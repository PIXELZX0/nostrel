package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"nostrel/internal/billing"
	"nostrel/internal/store"
)

// invoiceCheckCooldown keeps panel polling from hammering the lightning backend.
const invoiceCheckCooldown = 2 * time.Second

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read settings")
		return
	}
	name, desc := st.RelayName, st.RelayDescription
	if name == "" {
		name = s.cfg.RelayName
	}
	if desc == "" {
		desc = s.cfg.RelayDescription
	}

	domains, err := s.store.ListNip05Domains(true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list domains")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"nip05_domains":      len(domains),
		"nip46_relays":       st.Nip46Relays,
		"name":               name,
		"description":        desc,
		"icon":               st.RelayIcon,
		"theme_bg_color":     st.ThemeBgColor,
		"theme_accent":       st.ThemeAccent,
		"theme_bg_image":     st.ThemeBgImage,
		"relay_url":          s.cfg.ServiceURL,
		"payment_provider":   s.billing.Provider(),
		"admission_sats":     st.AdmissionSats,
		"subscription_sats":  st.SubscriptionSats,
		"period_days":        st.PeriodDays,
		"included_mb":        st.IncludedMB,
		"price_per_mb_sats":  st.PricePerMBSats,
		"read_auth_required": s.cfg.ReadAuthRequired,
	})
}

type orderRequest struct {
	Pubkey  string `json:"pubkey"`
	Periods int    `json:"periods"`
	ExtraMB int    `json:"extra_mb"`

	// a shared storage group to create or top up instead of a personal account
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`

	// a NIP-05 identifier
	Nip05Domain  string `json:"nip05_domain"`
	Nip05Name    string `json:"nip05_name"`
	Nip05Periods int    `json:"nip05_periods"`
}

func (s *Server) handleOrder(w http.ResponseWriter, r *http.Request) {
	var req orderRequest
	if _, err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.Pubkey = strings.ToLower(strings.TrimSpace(req.Pubkey))
	if !validPubkey(req.Pubkey) {
		writeErr(w, http.StatusBadRequest, "pubkey must be 64 hex characters")
		return
	}

	order := billing.Order{
		Periods: req.Periods, ExtraMB: req.ExtraMB,
		GroupID: req.GroupID, GroupName: req.GroupName,
		Nip05Domain: req.Nip05Domain, Nip05Name: req.Nip05Name, Nip05Periods: req.Nip05Periods,
	}
	p, err := s.billing.CreateOrder(r.Context(), req.Pubkey, order)
	switch {
	case errors.Is(err, billing.ErrPeriodRequired):
		writeErr(w, http.StatusBadRequest, "a new account needs at least one subscription period")
		return
	case errors.Is(err, billing.ErrNothingOrdered), errors.Is(err, billing.ErrTooLarge),
		errors.Is(err, billing.ErrBadName), errors.Is(err, billing.ErrNoSuchGroup),
		errors.Is(err, billing.ErrNotGroupOwner):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, store.ErrNameTaken), errors.Is(err, store.ErrNameBlocked),
		errors.Is(err, store.ErrNoDomain):
		writeErr(w, http.StatusConflict, nip05Reason(err))
		return
	case err != nil:
		s.log.Printf("order failed for %s: %v", req.Pubkey, err)
		writeErr(w, http.StatusBadGateway, "could not create an invoice, try again shortly")
		return
	}

	var meta store.PaymentMeta
	json.Unmarshal([]byte(p.Meta), &meta)

	writeJSON(w, http.StatusOK, map[string]any{
		"payment_hash": p.PaymentHash,
		"bolt11":       p.Bolt11,
		"sats":         p.Sats,
		"kind":         p.Kind,
		"group_id":     meta.GroupID,
		"qr":           "/api/invoice/" + p.PaymentHash + "/qr.png",
	})
}

// handleInvoice reports invoice status, asking the lightning backend at most
// once every invoiceCheckCooldown.
func (s *Server) handleInvoice(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")

	p, err := s.store.PaymentByHash(hash)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "unknown invoice")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read invoice")
		return
	}

	if p.Status == store.PayPending && s.shouldCheck(hash) {
		if settled, err := s.billing.Settle(r.Context(), hash); err != nil {
			s.log.Printf("settling %s: %v", hash, err)
		} else {
			p = settled
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"payment_hash": p.PaymentHash,
		"status":       p.Status,
		"paid":         p.Status == store.PayPaid,
		"sats":         p.Sats,
		"pubkey":       p.Pubkey,
	})
}

func (s *Server) shouldCheck(hash string) bool {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	if time.Since(s.lastCheck[hash]) < invoiceCheckCooldown {
		return false
	}
	s.lastCheck[hash] = time.Now()
	return true
}

func (s *Server) handleInvoiceQR(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.PaymentByHash(r.PathValue("hash"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "unknown invoice")
		return
	}
	png, err := qrcode.Encode("lightning:"+p.Bolt11, qrcode.Medium, 320)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not render qr code")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.Write(png)
}

// handleAccount exposes the subscription state of a pubkey. It is public
// because everything in it is already observable from the relay's behaviour,
// and the panel needs it before the user has any way to authenticate.
func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	pubkey := r.PathValue("pubkey")
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "pubkey must be 64 hex characters")
		return
	}

	allowance, err := s.store.Allowance(pubkey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read account")
		return
	}

	out := map[string]any{"pubkey": pubkey, "exists": allowance.Account != nil}
	if acct := allowance.Account; acct != nil {
		out["status"] = acct.Status
		out["expires_at"] = acct.ExpiresAt
		out["quota_bytes"] = acct.QuotaBytes
		out["used_bytes"] = acct.UsedBytes
	}
	// the shared pot the pubkey can fall back on, if any
	if group := allowance.Group; group != nil {
		out["group"] = map[string]any{
			"id":          group.ID,
			"name":        group.Name,
			"owner":       group.Owner,
			"expires_at":  group.ExpiresAt,
			"quota_bytes": group.QuotaBytes,
			"used_bytes":  group.UsedBytes,
		}
	}
	// a zero-byte write is the cheapest question there is: may this pubkey
	// write at all, from either pot?
	out["can_write"], _, _ = allowance.CanWrite(time.Now().Unix(), 0)

	writeJSON(w, http.StatusOK, out)
}

// handleWebhook is the LNbits payment notification. The payload is unsigned, so
// it is treated purely as a hint: the invoice is re-checked against the LNbits
// API before anything is granted.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PaymentHash string `json:"payment_hash"`
		CheckingID  string `json:"checking_id"`
	}
	if _, err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	hash := body.PaymentHash
	if hash == "" {
		hash = body.CheckingID
	}
	if hash == "" {
		writeErr(w, http.StatusBadRequest, "payment_hash missing")
		return
	}

	if _, err := s.billing.Settle(r.Context(), hash); err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.log.Printf("webhook settle %s: %v", hash, err)
	}
	w.WriteHeader(http.StatusNoContent)
}
