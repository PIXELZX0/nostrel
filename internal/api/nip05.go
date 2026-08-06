package api

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nostrel/internal/billing"
	"nostrel/internal/store"
)

// NIP-05: identifiers of the form name@domain, sold per domain as a
// subscription. The relay can serve several domains at once — which one a
// request is for comes from the Host header, so pointing extra domains at this
// server is a DNS change plus a row in the panel.

var (
	nip05NameRE   = regexp.MustCompile(`^[a-z0-9_.-]{1,30}$`)
	nip05DomainRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
)

func (s *Server) registerNip05(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/nostr.json", s.handleWellKnownNostr)
	mux.HandleFunc("GET /api/nip05/domains", s.handleNip05Domains)
	mux.HandleFunc("GET /api/nip05/check", s.handleNip05Check)
	mux.HandleFunc("GET /api/nip05/names/{pubkey}", s.handleNip05NamesOf)

	mux.HandleFunc("GET /api/admin/nip05/domains", s.admin(s.handleListNip05Domains))
	mux.HandleFunc("PUT /api/admin/nip05/domains/{domain}", s.admin(s.handleSaveNip05Domain))
	mux.HandleFunc("DELETE /api/admin/nip05/domains/{domain}", s.admin(s.handleDeleteNip05Domain))
	mux.HandleFunc("GET /api/admin/nip05/names", s.admin(s.handleListNip05Names))
	mux.HandleFunc("PUT /api/admin/nip05/names/{domain}/{name}", s.admin(s.handleSaveNip05Name))
	mux.HandleFunc("DELETE /api/admin/nip05/names/{domain}/{name}", s.admin(s.handleDeleteNip05Name))
	mux.HandleFunc("GET /api/admin/nip05/verify/{domain}", s.admin(s.handleVerifyNip05Domain))
	mux.HandleFunc("GET /api/admin/nip05/blocked", s.admin(s.handleListBlockedNames))
	mux.HandleFunc("PUT /api/admin/nip05/blocked/{name}", s.admin(s.handleBlockName))
	mux.HandleFunc("DELETE /api/admin/nip05/blocked/{name}", s.admin(s.handleUnblockName))
}

// requestDomain is the domain a request was addressed to, lowercased and
// without the port. It is what decides which set of names answers.
func requestDomain(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

// handleWellKnownNostr is the NIP-05 endpoint itself. Clients fetch it from a
// browser, so it must be readable cross-origin.
//
// Only the requested name is answered: returning the whole list would hand
// anybody a directory of every customer.
func (s *Server) handleWellKnownNostr(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	names := map[string]string{}
	relays := map[string][]string{}

	name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
	if name != "" && nip05NameRE.MatchString(name) {
		pubkey, err := s.store.ResolveNip05(requestDomain(r), name)
		if err != nil {
			s.log.Printf("nip-05 lookup for %s: %v", name, err)
			writeErr(w, http.StatusInternalServerError, "could not look that name up")
			return
		}
		if pubkey != "" {
			names[name] = pubkey
			if s.cfg.ServiceURL != "" {
				relays[pubkey] = []string{s.cfg.ServiceURL}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"names": names, "relays": relays})
}

// handleNip05Domains is the price list the buy page renders.
func (s *Server) handleNip05Domains(w http.ResponseWriter, r *http.Request) {
	domains, err := s.store.ListNip05Domains(true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list domains")
		return
	}
	settings, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read settings")
		return
	}

	out := make([]map[string]any, 0, len(domains))
	for _, d := range domains {
		out = append(out, map[string]any{
			"domain":      d.Domain,
			"price_sats":  d.PriceSats,
			"period_days": d.PeriodDays,
			"note":        d.Note,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"domains":       out,
		"premium_tiers": settings.Nip05PremiumTiers,
	})
}

// handleNip05Check tells the buy page whether a name is free and what it costs,
// before an invoice is created for it.
func (s *Server) handleNip05Check(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := strings.ToLower(strings.TrimSpace(q.Get("name")))
	domain := strings.ToLower(strings.TrimSpace(q.Get("domain")))
	periods, _ := strconv.Atoi(q.Get("periods"))
	if periods < 1 {
		periods = 1
	}

	pubkey := strings.ToLower(strings.TrimSpace(q.Get("pubkey")))
	if pubkey != "" && !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "pubkey must be 64 hex characters")
		return
	}

	sats, _, _, err := s.billing.Quote(pubkey, billing.Order{
		Nip05Domain: domain, Nip05Name: name, Nip05Periods: periods,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false,
			"reason":    nip05Reason(err),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "sats": sats})
}

// handleNip05NamesOf lists the names a pubkey holds so the buy page can show
// what somebody already owns and when it runs out. It is public for the same
// reason /api/account is: the answer is already visible in nostr.json to
// anybody who guesses the name.
func (s *Server) handleNip05NamesOf(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.ToLower(r.PathValue("pubkey"))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "pubkey must be 64 hex characters")
		return
	}

	names, err := s.store.ListNip05NamesOf(pubkey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list names")
		return
	}
	writeJSON(w, http.StatusOK, names)
}

// nip05Reason turns the quoting errors into something a buyer can act on.
func nip05Reason(err error) string {
	switch {
	case errors.Is(err, store.ErrNameTaken):
		return "somebody already has that name"
	case errors.Is(err, store.ErrNameBlocked):
		return "that name is reserved"
	case errors.Is(err, store.ErrNoDomain):
		return "that domain is not for sale here"
	case errors.Is(err, billing.ErrBadName):
		return billing.ErrBadName.Error()
	default:
		return err.Error()
	}
}

// --- admin ---

func (s *Server) handleListNip05Domains(w http.ResponseWriter, r *http.Request) {
	domains, err := s.store.ListNip05Domains(false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list domains")
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (s *Server) handleSaveNip05Domain(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if !nip05DomainRE.MatchString(domain) {
		writeErr(w, http.StatusBadRequest, "domain must be a bare hostname, without scheme or port")
		return
	}

	var req struct {
		Enabled    bool   `json:"enabled"`
		PriceSats  int64  `json:"price_sats"`
		PeriodDays int    `json:"period_days"`
		Note       string `json:"note"`
	}
	if _, err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.PriceSats < 0 {
		writeErr(w, http.StatusBadRequest, "price must not be negative")
		return
	}
	if req.PeriodDays <= 0 {
		writeErr(w, http.StatusBadRequest, "period_days must be at least 1")
		return
	}

	if err := s.store.SaveNip05Domain(store.Nip05Domain{
		Domain: domain, Enabled: req.Enabled, PriceSats: req.PriceSats,
		PeriodDays: req.PeriodDays, Note: req.Note,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save the domain")
		return
	}

	saved, err := s.store.Nip05Domain(domain)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read the domain back")
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteNip05Domain(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteNip05Domain(r.PathValue("domain")); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete the domain")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// nip05Probe fetches a domain's own nostr.json. Its own client so the timeout
// is short: an admin is waiting on the answer.
var nip05Probe = &http.Client{Timeout: 10 * time.Second}

// handleVerifyNip05Domain fetches https://domain/.well-known/nostr.json from
// the outside and reports what came back. DNS and the reverse proxy are the two
// things that break a domain, and neither is visible from inside this process —
// a name that resolves here still answers nothing if the proxy rewrites Host.
//
// This makes the server fetch a URL an admin chose, which is the point: the
// scheme is fixed to https and the host has to look like a hostname.
func (s *Server) handleVerifyNip05Domain(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if !nip05DomainRE.MatchString(domain) {
		writeErr(w, http.StatusBadRequest, "domain must be a bare hostname, without scheme or port")
		return
	}

	// Probe with a name that actually resolves here, so the answer is checked
	// against a pubkey and not merely parsed. Failing that, "_" still proves the
	// endpoint is reachable and well-formed.
	name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
	if name == "" {
		name = s.probeNameFor(domain)
	}
	if !nip05NameRE.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "invalid name")
		return
	}
	expected, err := s.store.ResolveNip05(domain, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not look that name up")
		return
	}

	url := "https://" + domain + "/.well-known/nostr.json?name=" + name
	out := map[string]any{"url": url, "name": name, "expected": expected, "ok": false}
	fail := func(problem string) {
		out["problem"] = problem
		writeJSON(w, http.StatusOK, out)
	}

	resp, err := nip05Probe.Get(url)
	if err != nil {
		fail("could not reach the domain: " + err.Error())
		return
	}
	defer resp.Body.Close()

	out["status"] = resp.StatusCode
	out["cors"] = resp.Header.Get("Access-Control-Allow-Origin") == "*"
	if resp.StatusCode != http.StatusOK {
		fail("the domain answered " + strconv.Itoa(resp.StatusCode) + ", not 200")
		return
	}

	var doc struct {
		Names map[string]string `json:"names"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		fail("could not read the answer: " + err.Error())
		return
	}
	if err := json.Unmarshal(body, &doc); err != nil || doc.Names == nil {
		fail("the answer was not a NIP-05 document — something else is serving this domain")
		return
	}

	got := doc.Names[name]
	out["got"] = got
	switch {
	case expected == "" && got == "":
		// nothing is sold under this domain yet, so there is nothing to match
		out["ok"] = true
		out["problem"] = "the endpoint works, but no name is sold here yet to check against"
	case got == "":
		fail("the endpoint answered, but not with " + name + "@" + domain + " — Host is probably not passed through")
		return
	case got != expected:
		fail("another server answers for this domain: it returned a different pubkey")
		return
	default:
		out["ok"] = true
	}
	if !out["cors"].(bool) {
		out["ok"] = false
		out["problem"] = "the answer has no Access-Control-Allow-Origin: * header, so browser clients cannot read it"
	}
	writeJSON(w, http.StatusOK, out)
}

// probeNameFor picks a live name under domain to check the answer against, or
// "_" when the domain has sold none.
func (s *Server) probeNameFor(domain string) string {
	names, err := s.store.ListNip05Names("", domain, 50, 0)
	if err != nil {
		return "_"
	}
	now := time.Now().Unix()
	for _, n := range names {
		if n.Live(now) {
			return n.Name
		}
	}
	return "_"
}

func (s *Server) handleListNip05Names(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	names, err := s.store.ListNip05Names(q.Get("q"), q.Get("domain"), limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list names")
		return
	}
	writeJSON(w, http.StatusOK, names)
}

// handleSaveNip05Name is the manual path: hand a name to a pubkey, extend one,
// or make it permanent — all without a payment.
func (s *Server) handleSaveNip05Name(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	if !nip05DomainRE.MatchString(domain) || !nip05NameRE.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "invalid domain or name")
		return
	}

	var req struct {
		Pubkey    string `json:"pubkey"`
		ExpiresAt int64  `json:"expires_at"`
		Permanent bool   `json:"permanent"`
	}
	if _, err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.Pubkey = strings.ToLower(strings.TrimSpace(req.Pubkey))
	if !validPubkey(req.Pubkey) {
		writeErr(w, http.StatusBadRequest, "pubkey must be 64 hex characters")
		return
	}
	if req.ExpiresAt < 0 {
		writeErr(w, http.StatusBadRequest, "expires_at must not be negative")
		return
	}
	if _, err := s.store.Nip05Domain(domain); errors.Is(err, store.ErrNoDomain) {
		writeErr(w, http.StatusBadRequest, "add the domain first")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read the domain")
		return
	}

	if err := s.store.SaveNip05Name(store.Nip05Name{
		Domain: domain, Name: name, Pubkey: req.Pubkey,
		ExpiresAt: req.ExpiresAt, Permanent: req.Permanent,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save the name")
		return
	}

	saved, err := s.store.Nip05Name(domain, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read the name back")
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteNip05Name(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteNip05Name(r.PathValue("domain"), r.PathValue("name")); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete the name")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListBlockedNames(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ModList(store.ModBlockedName)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list blocked names")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleBlockName reserves a name across every domain. Names already sold keep
// working — taking one back is a deliberate delete, not a side effect.
func (s *Server) handleBlockName(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	if !nip05NameRE.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "invalid name")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if _, err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := s.store.ModAdd(store.ModBlockedName, name, req.Reason); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not block the name")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "reason": req.Reason})
}

func (s *Server) handleUnblockName(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	if err := s.store.ModRemove(store.ModBlockedName, name); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not unblock the name")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
