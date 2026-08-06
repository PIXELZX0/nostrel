package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nostrel/internal/blobs"
	"nostrel/internal/payments"
	"nostrel/internal/store"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.logins.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, wait a few minutes")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if _, err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if !s.checkPassword(req.Password) {
		writeErr(w, http.StatusUnauthorized, "wrong password")
		return
	}

	s.setSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"admin": true})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read stats")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	accounts, err := s.store.ListAccounts(q.Get("q"), limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list accounts")
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

// handleSaveAccount creates or edits an account by hand: the manual whitelist
// path, and the way an admin grants, extends or bans access without payment.
func (s *Server) handleSaveAccount(w http.ResponseWriter, r *http.Request) {
	pubkey := r.PathValue("pubkey")
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "pubkey must be 64 hex characters")
		return
	}

	var req struct {
		Status     string `json:"status"`
		ExpiresAt  int64  `json:"expires_at"`
		QuotaBytes int64  `json:"quota_bytes"`
		Note       string `json:"note"`
	}
	if _, err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Status != store.StatusActive && req.Status != store.StatusBanned {
		writeErr(w, http.StatusBadRequest, "status must be active or banned")
		return
	}
	if req.ExpiresAt < 0 || req.QuotaBytes < 0 {
		writeErr(w, http.StatusBadRequest, "expires_at and quota_bytes must not be negative")
		return
	}

	if _, err := s.store.EnsureAccount(pubkey); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create account")
		return
	}
	if err := s.store.UpdateAccount(pubkey, req.Status, req.ExpiresAt, req.QuotaBytes, req.Note); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not update account")
		return
	}

	acct, err := s.store.Account(pubkey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read account back")
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	pubkey := r.PathValue("pubkey")
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "pubkey must be 64 hex characters")
		return
	}
	if err := s.store.DeleteAccount(pubkey); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListPayments(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	payments, err := s.store.ListPayments(r.URL.Query().Get("pubkey"), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list payments")
		return
	}
	writeJSON(w, http.StatusOK, payments)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read settings")
		return
	}
	writeJSON(w, http.StatusOK, st.Redacted())
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	current, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read settings")
		return
	}

	// start from what is stored so a partial form can't wipe fields it didn't
	// show, and so masked secrets stay as they are
	incoming := current
	if _, err := decode(r, &incoming); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	incoming.LNbitsInvoiceKey = keepSecret(incoming.LNbitsInvoiceKey, current.LNbitsInvoiceKey)
	incoming.NWCURI = keepSecret(incoming.NWCURI, current.NWCURI)
	incoming.S3SecretKey = keepSecret(incoming.S3SecretKey, current.S3SecretKey)

	if err := validateSettings(incoming); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SaveSettings(incoming); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	// pick up a changed storage backend without a restart
	if s.blobs != nil {
		if err := s.blobs.Reload(); err != nil {
			s.log.Printf("reloading media storage: %v", err)
			writeErr(w, http.StatusBadRequest, "settings saved, but the storage backend failed to start: "+err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, incoming.Redacted())
}

// handleStorageTest writes, reads back and deletes a probe object so an admin
// can prove a bucket works before pointing the relay at it.
func (s *Server) handleStorageTest(w http.ResponseWriter, r *http.Request) {
	current, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read settings")
		return
	}

	candidate := current
	if _, err := decode(r, &candidate); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	candidate.S3SecretKey = keepSecret(candidate.S3SecretKey, current.S3SecretKey)

	backend, err := blobs.BuildBackend(candidate, store.DefaultBlobPath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	message, err := backend.Check(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": message})
}

// handlePaymentTest exercises a lightning backend without saving it, so an
// admin can find out whether credentials work before switching the relay over.
func (s *Server) handlePaymentTest(w http.ResponseWriter, r *http.Request) {
	current, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read settings")
		return
	}

	candidate := current
	if _, err := decode(r, &candidate); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	candidate.LNbitsInvoiceKey = keepSecret(candidate.LNbitsInvoiceKey, current.LNbitsInvoiceKey)
	candidate.NWCURI = keepSecret(candidate.NWCURI, current.NWCURI)

	provider, err := payments.Build(candidate)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	checker, ok := provider.(payments.Checker)
	if !ok {
		writeErr(w, http.StatusBadRequest, "this backend cannot be tested")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	message, err := checker.Check(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": message})
}

// keepSecret returns the stored secret when the panel echoed back the masked
// form (meaning the admin didn't touch that field).
func keepSecret(submitted, stored string) string {
	if submitted == "" || store.IsMasked(submitted) {
		return stored
	}
	return strings.TrimSpace(submitted)
}

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// hexColor accepts the empty string, meaning "leave the stylesheet alone".
func hexColor(v string) bool { return v == "" || hexColorRe.MatchString(v) }

// imageURL keeps the background image to an absolute http(s) address: it ends
// up inside a CSS url(), where a quote or a bracket would break out.
func imageURL(v string) bool {
	if v == "" {
		return true
	}
	if strings.ContainsAny(v, "\"'()\\ \t\r\n") {
		return false
	}
	u, err := url.Parse(v)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func validateSettings(st store.Settings) error {
	switch {
	case st.AdmissionSats < 0, st.SubscriptionSats < 0, st.PricePerMBSats < 0:
		return errors.New("prices must not be negative")
	case st.PeriodDays <= 0:
		return errors.New("period_days must be at least 1")
	case st.IncludedMB < 0:
		return errors.New("included_mb must not be negative")
	case st.MinPoW < 0:
		return errors.New("min_pow must not be negative")
	case st.CreatedAtMaxPast < 0, st.CreatedAtMaxFuture < 0:
		return errors.New("the created_at limits must not be negative")
	case st.MaxBlobSizeMB < 1:
		return errors.New("max_blob_size_mb must be at least 1")
	case st.RelayPubkey != "" && !validPubkey(st.RelayPubkey):
		return errors.New("relay_pubkey must be 64 hex characters")
	}
	// a malformed premium spec would make every NIP-05 quote fail
	if _, err := store.ParsePremiumTiers(st.Nip05PremiumTiers); err != nil {
		return err
	}
	// theme values are written straight into CSS by the panel, so only accept
	// shapes that cannot carry a second declaration
	if !hexColor(st.ThemeBgColor) || !hexColor(st.ThemeAccent) {
		return errors.New("colours must be written as #rrggbb")
	}
	if !imageURL(st.ThemeBgImage) {
		return errors.New("the background image must be an http(s) URL")
	}
	// a backend that can't be built would leave the relay unable to sell anything
	if _, err := payments.Build(st); err != nil {
		return err
	}
	// same for storage: refuse a configuration uploads would fail on
	switch st.StorageBackend {
	case "", "local":
	case "s3":
		if st.S3Bucket == "" || st.S3Endpoint == "" || st.S3AccessKey == "" || st.S3SecretKey == "" {
			return errors.New("s3 storage needs an endpoint, a bucket, an access key and a secret key")
		}
	default:
		return errors.New("storage_backend must be local or s3")
	}
	return nil
}
