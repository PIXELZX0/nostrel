// Package api serves the web panel and its JSON endpoints.
package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"nostrel/internal/billing"
	"nostrel/internal/blobs"
	"nostrel/internal/config"
	"nostrel/internal/store"
	"nostrel/web"
)

const maxBodySize = 1 << 20

type Server struct {
	cfg     *config.Config
	store   *store.Store
	billing *billing.Service
	log     *log.Logger

	seen   *replayCache
	logins *loginLimiter

	// media storage, nil when BLOB_PATH could not be prepared
	blobs     *blobs.Storage
	blobIndex *blobs.Index

	checkMu   sync.Mutex
	lastCheck map[string]time.Time // payment hash -> last provider query
}

func New(cfg *config.Config, st *store.Store, b *billing.Service, logger *log.Logger) *Server {
	return &Server{
		cfg: cfg, store: st, billing: b, log: logger,
		seen:      newReplayCache(),
		logins:    newLoginLimiter(),
		lastCheck: map[string]time.Time{},
	}
}

// EnableMedia turns on the NIP-96 endpoints, backed by the same blob storage
// Blossom uses.
func (s *Server) EnableMedia(storage *blobs.Storage, index *blobs.Index) {
	s.blobs, s.blobIndex = storage, index
}

// Register mounts the panel and API on the relay's mux.
func (s *Server) Register(mux *http.ServeMux) {
	if s.blobs != nil {
		s.registerMedia(mux)
	}
	s.registerNip05(mux)
	s.registerGroups(mux)
	s.registerInvites(mux)

	// public
	mux.HandleFunc("GET /api/info", s.handleInfo)
	mux.HandleFunc("POST /api/order", s.handleOrder)
	mux.HandleFunc("GET /api/invoice/{hash}", s.handleInvoice)
	mux.HandleFunc("GET /api/invoice/{hash}/qr.png", s.handleInvoiceQR)
	mux.HandleFunc("GET /api/account/{pubkey}", s.handleAccount)
	mux.HandleFunc("POST /webhook/lnbits", s.handleWebhook)

	// admin
	mux.HandleFunc("POST /api/admin/login", s.handleLogin)
	mux.HandleFunc("POST /api/admin/logout", s.handleLogout)
	mux.HandleFunc("GET /api/admin/session", s.admin(s.handleSession))
	mux.HandleFunc("GET /api/admin/stats", s.admin(s.handleStats))
	mux.HandleFunc("GET /api/admin/accounts", s.admin(s.handleListAccounts))
	mux.HandleFunc("PUT /api/admin/accounts/{pubkey}", s.admin(s.handleSaveAccount))
	mux.HandleFunc("DELETE /api/admin/accounts/{pubkey}", s.admin(s.handleDeleteAccount))
	mux.HandleFunc("GET /api/admin/payments", s.admin(s.handleListPayments))
	mux.HandleFunc("GET /api/admin/settings", s.admin(s.handleGetSettings))
	mux.HandleFunc("PUT /api/admin/settings", s.admin(s.handleSaveSettings))
	mux.HandleFunc("POST /api/admin/payments/test", s.admin(s.handlePaymentTest))

	// static panel
	mux.Handle("GET /static/", http.FileServerFS(web.Files))
	mux.HandleFunc("GET /admin", web.Page("admin.html"))
	mux.HandleFunc("GET /{$}", s.orNsite(web.Page("index.html")))
	// NIP-5A sites live on their own hosts, so everything else is either a page
	// of somebody's site or a genuine 404
	mux.HandleFunc("GET /", s.orNsite(http.NotFound))
}

// orNsite serves a NIP-5A site when the request came in on an nsite host, and
// otherwise falls through to the panel. Sites are matched on Host, so they
// cannot shadow the panel's own address.
func (s *Server) orNsite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.handleNsite(w, r) {
			return
		}
		next(w, r)
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, maxBodySize))
}

// decodeBytes parses a body that was already read, for handlers that had to
// verify a NIP-98 signature over the exact bytes first.
func decodeBytes(body []byte, v any) error {
	return json.NewDecoder(bytes.NewReader(body)).Decode(v)
}

// decode reads and parses a JSON body, returning the raw bytes too so NIP-98
// payload hashes can be checked against exactly what was received.
func decode(r *http.Request, v any) ([]byte, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return body, nil
	}
	return body, json.NewDecoder(bytes.NewReader(body)).Decode(v)
}

// admin wraps a handler so it only runs for an authenticated admin: either a
// NIP-98 signature from a pubkey in ADMIN_PUBKEYS, or a valid password session.
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.validSession(r) {
			next(w, r)
			return
		}

		var body []byte
		if r.Method != http.MethodGet && r.Method != http.MethodDelete {
			var err error
			if body, err = readBody(r); err != nil {
				writeErr(w, http.StatusBadRequest, "could not read request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		pubkey, err := s.VerifyNIP98(r, body)
		if err != nil {
			// Why the signature was refused is not a secret, and hiding it makes
			// a misconfigured PANEL_URL or a skewed clock impossible to diagnose
			// from the login screen.
			if errors.Is(err, errNoAuth) {
				writeErr(w, http.StatusUnauthorized, "authentication required")
				return
			}
			writeErr(w, http.StatusUnauthorized, err.Error())
			return
		}
		if !s.store.IsAdmin(pubkey, s.cfg.AdminPubkeys) {
			writeErr(w, http.StatusForbidden, "not an admin pubkey")
			return
		}
		next(w, r)
	}
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func validPubkey(pk string) bool {
	if len(pk) != 64 {
		return false
	}
	_, err := hex.DecodeString(pk)
	return err == nil
}
