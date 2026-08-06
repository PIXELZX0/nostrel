package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nostrel/internal/store"
)

// Invite codes, the web half of NIP-43. A client that speaks the protocol sends
// a kind 28934 to the relay; everyone else pastes the code into the panel and
// ends up in exactly the same place.

func (s *Server) registerInvites(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/invite/claim", s.handleClaimInvite)

	mux.HandleFunc("GET /api/admin/invites", s.admin(s.handleListInvites))
	mux.HandleFunc("POST /api/admin/invites", s.admin(s.handleCreateInvite))
	mux.HandleFunc("DELETE /api/admin/invites/{code}", s.admin(s.handleDeleteInvite))
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	invites, err := s.store.ListInvites(limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list invite codes")
		return
	}
	writeJSON(w, http.StatusOK, invites)
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Note       string `json:"note"`
		PeriodDays int    `json:"period_days"`
		QuotaMB    int    `json:"quota_mb"`
		MaxUses    int    `json:"max_uses"`
		ExpiresIn  int    `json:"expires_in_days"` // 0 = never
	}
	if _, err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.PeriodDays < 0 || req.QuotaMB < 0 || req.MaxUses < 0 || req.ExpiresIn < 0 {
		writeErr(w, http.StatusBadRequest, "values must not be negative")
		return
	}
	if req.PeriodDays == 0 && req.QuotaMB == 0 {
		writeErr(w, http.StatusBadRequest, "an invite must grant a period or some storage")
		return
	}

	var expiresAt int64
	if req.ExpiresIn > 0 {
		expiresAt = time.Now().AddDate(0, 0, req.ExpiresIn).Unix()
	}

	invite, err := s.store.CreateInvite(store.Invite{
		Note: req.Note, PeriodDays: req.PeriodDays, QuotaMB: req.QuotaMB,
		MaxUses: req.MaxUses, ExpiresAt: expiresAt,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create the invite code")
		return
	}
	writeJSON(w, http.StatusOK, invite)
}

func (s *Server) handleDeleteInvite(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteInvite(r.PathValue("code")); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete the invite code")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleClaimInvite is the panel's version of a kind 28934. The claim is signed
// with NIP-98 so the code is spent for whoever actually holds the key, not for
// whatever pubkey the request body happens to name.
func (s *Server) handleClaimInvite(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read request body")
		return
	}
	pubkey, err := s.VerifyNIP98(r, body)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "sign this request with the key you want to register")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if len(body) > 0 {
		if err := decodeBytes(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	if strings.TrimSpace(req.Code) == "" {
		writeErr(w, http.StatusBadRequest, "an invite code is required")
		return
	}

	invite, err := s.store.ClaimInvite(req.Code, pubkey)
	switch {
	case errors.Is(err, store.ErrInviteUnknown),
		errors.Is(err, store.ErrInviteExpired),
		errors.Is(err, store.ErrInviteExhausted),
		errors.Is(err, store.ErrInviteAlreadyUsed):
		writeErr(w, http.StatusForbidden, err.Error())
		return
	case err != nil:
		s.log.Printf("claiming invite for %s: %v", pubkey, err)
		writeErr(w, http.StatusInternalServerError, "could not check that invite code")
		return
	}

	if err := s.grantInvite(pubkey, invite); err != nil {
		s.log.Printf("granting invite to %s: %v", pubkey, err)
		writeErr(w, http.StatusInternalServerError, "could not create your account")
		return
	}

	account, err := s.store.Account(pubkey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read your account back")
		return
	}
	writeJSON(w, http.StatusOK, account)
}

// grantInvite mirrors the relay's own version: an invite waives the admission
// fee, which is the whole point of handing one out.
func (s *Server) grantInvite(pubkey string, invite *store.Invite) error {
	account, err := s.store.EnsureAccount(pubkey)
	if err != nil {
		return err
	}
	expiresAt := account.ExpiresAt
	if invite.PeriodDays > 0 {
		from := time.Now().Unix()
		if expiresAt > from {
			from = expiresAt
		}
		expiresAt = from + int64(invite.PeriodDays)*86400
	}
	quota := account.QuotaBytes + int64(invite.QuotaMB)*store.MB
	return s.store.UpdateAccount(pubkey, store.StatusActive, expiresAt, quota, "invite:"+invite.Code)
}
