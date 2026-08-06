package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nostrel/internal/store"
)

// Groups share one pot of storage between several pubkeys. Members are managed
// by the group's owner — signing with the owner's key is the proof — or by a
// relay admin from the panel.

func (s *Server) registerGroups(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/group/{id}", s.handleGroup)
	mux.HandleFunc("GET /api/group/{id}/members", s.groupOwner(s.handleListGroupMembers))
	mux.HandleFunc("PUT /api/group/{id}/members/{pubkey}", s.groupOwner(s.handleAddGroupMember))
	mux.HandleFunc("DELETE /api/group/{id}/members/{pubkey}", s.groupOwner(s.handleRemoveGroupMember))

	mux.HandleFunc("GET /api/admin/groups", s.admin(s.handleListGroups))
	mux.HandleFunc("PUT /api/admin/groups/{id}", s.admin(s.handleSaveGroup))
	mux.HandleFunc("DELETE /api/admin/groups/{id}", s.admin(s.handleDeleteGroup))
	mux.HandleFunc("GET /api/admin/groups/{id}/members", s.admin(s.handleListGroupMembers))
	mux.HandleFunc("PUT /api/admin/groups/{id}/members/{pubkey}", s.admin(s.handleAddGroupMember))
	mux.HandleFunc("DELETE /api/admin/groups/{id}/members/{pubkey}", s.admin(s.handleRemoveGroupMember))
}

// groupOwner wraps a handler so it only runs for the group's owner, proven with
// a NIP-98 signature, or for a relay admin. It is the same shape as admin(),
// including the password-session shortcut an admin already has.
func (s *Server) groupOwner(next http.HandlerFunc) http.HandlerFunc {
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
			writeErr(w, http.StatusUnauthorized, "sign this request with the group owner's key")
			return
		}
		if s.store.IsAdmin(pubkey, s.cfg.AdminPubkeys) {
			next(w, r)
			return
		}

		group, err := s.store.Group(r.PathValue("id"))
		if errors.Is(err, store.ErrNoGroup) {
			writeErr(w, http.StatusNotFound, "no such group")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not read the group")
			return
		}
		if group.Owner != pubkey {
			writeErr(w, http.StatusForbidden, "only the group owner can do that")
			return
		}
		next(w, r)
	}
}

// handleGroup reports what a group has left. Public for the same reason
// handleAccount is: a member needs it before they can authenticate anywhere.
func (s *Server) handleGroup(w http.ResponseWriter, r *http.Request) {
	group, err := s.store.Group(r.PathValue("id"))
	if errors.Is(err, store.ErrNoGroup) {
		writeErr(w, http.StatusNotFound, "no such group")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read the group")
		return
	}
	members, err := s.store.CountMembers(group.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not count members")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          group.ID,
		"name":        group.Name,
		"owner":       group.Owner,
		"status":      group.Status,
		"expires_at":  group.ExpiresAt,
		"quota_bytes": group.QuotaBytes,
		"used_bytes":  group.UsedBytes,
		"members":     members,
		"active": group.Status == store.StatusActive &&
			(group.ExpiresAt == 0 || group.ExpiresAt > time.Now().Unix()),
	})
}

func (s *Server) handleListGroupMembers(w http.ResponseWriter, r *http.Request) {
	members, err := s.store.ListMembers(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list members")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (s *Server) handleAddGroupMember(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.ToLower(r.PathValue("pubkey"))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "pubkey must be 64 hex characters")
		return
	}

	err := s.store.AddMember(r.PathValue("id"), pubkey)
	if errors.Is(err, store.ErrNoGroup) {
		writeErr(w, http.StatusNotFound, "no such group")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not add the member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveGroupMember drops a member. The owner stays: removing them would
// leave a group nobody can top up or manage.
func (s *Server) handleRemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.ToLower(r.PathValue("pubkey"))
	group, err := s.store.Group(r.PathValue("id"))
	if errors.Is(err, store.ErrNoGroup) {
		writeErr(w, http.StatusNotFound, "no such group")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read the group")
		return
	}
	if group.Owner == pubkey {
		writeErr(w, http.StatusBadRequest, "the owner cannot leave their own group")
		return
	}

	if err := s.store.RemoveMember(pubkey); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not remove the member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- admin ---

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	groups, err := s.store.ListGroups(q.Get("q"), limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list groups")
		return
	}

	type row struct {
		store.Group
		Members int `json:"members"`
	}
	out := make([]row, 0, len(groups))
	for _, g := range groups {
		members, err := s.store.CountMembers(g.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not count members")
			return
		}
		out = append(out, row{Group: g, Members: members})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSaveGroup creates or edits a group by hand: the way an admin grants
// shared storage without a payment.
func (s *Server) handleSaveGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Owner      string `json:"owner"`
		Status     string `json:"status"`
		ExpiresAt  int64  `json:"expires_at"`
		QuotaBytes int64  `json:"quota_bytes"`
		Note       string `json:"note"`
	}
	if _, err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.Owner = strings.ToLower(strings.TrimSpace(req.Owner))
	if !validPubkey(req.Owner) {
		writeErr(w, http.StatusBadRequest, "owner must be 64 hex characters")
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

	id := r.PathValue("id")
	if _, err := s.store.Group(id); errors.Is(err, store.ErrNoGroup) {
		if _, err := s.store.CreateGroup(id, req.Name, req.Owner); err != nil {
			writeErr(w, http.StatusInternalServerError, "could not create the group")
			return
		}
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read the group")
		return
	}

	if err := s.store.UpdateGroup(id, req.Name, req.Owner, req.Status,
		req.ExpiresAt, req.QuotaBytes, req.Note); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not update the group")
		return
	}
	// an owner handed the group by an admin has to be a member of it
	if err := s.store.AddMember(id, req.Owner); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not add the owner as a member")
		return
	}

	group, err := s.store.Group(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read the group back")
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteGroup(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete the group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
