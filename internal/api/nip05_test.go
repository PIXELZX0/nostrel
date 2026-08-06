package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nostrel/internal/billing"
	"nostrel/internal/config"
	"nostrel/internal/payments"
	"nostrel/internal/store"
)

const (
	buyerPubkey = "aa11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"
	otherPubkey = "bb11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"
)

// newNip05Server wires the NIP-05 routes. The admin handlers are mounted
// unwrapped: what they do with a request is the point here, and s.admin is
// covered by the auth tests.
func newNip05Server(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(st.Close)

	logger := log.New(io.Discard, "", 0)
	cfg := &config.Config{PanelURL: panelURL, ServiceURL: "wss://relay.example.com"}
	s := New(cfg, st, billing.New(st, payments.NewResolver(st, ""), logger), logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/nostr.json", s.handleWellKnownNostr)
	mux.HandleFunc("GET /api/nip05/domains", s.handleNip05Domains)
	mux.HandleFunc("GET /api/nip05/check", s.handleNip05Check)
	mux.HandleFunc("GET /api/nip05/names/{pubkey}", s.handleNip05NamesOf)
	mux.HandleFunc("GET /api/admin/nip05/domains", s.handleListNip05Domains)
	mux.HandleFunc("PUT /api/admin/nip05/domains/{domain}", s.handleSaveNip05Domain)
	mux.HandleFunc("DELETE /api/admin/nip05/domains/{domain}", s.handleDeleteNip05Domain)
	mux.HandleFunc("GET /api/admin/nip05/names", s.handleListNip05Names)
	mux.HandleFunc("PUT /api/admin/nip05/names/{domain}/{name}", s.handleSaveNip05Name)
	mux.HandleFunc("DELETE /api/admin/nip05/names/{domain}/{name}", s.handleDeleteNip05Name)
	mux.HandleFunc("GET /api/admin/nip05/verify/{domain}", s.handleVerifyNip05Domain)
	mux.HandleFunc("PUT /api/admin/nip05/blocked/{name}", s.handleBlockName)
	mux.HandleFunc("DELETE /api/admin/nip05/blocked/{name}", s.handleUnblockName)
	return mux, st
}

// do runs a request against the mux and returns the recorder plus the decoded
// body, which is nil for the empty 204 responses.
func do(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Body.Len() == 0 {
		return rec, nil
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: decoding %s: %v", method, path, rec.Body.String(), err)
	}
	return rec, out
}

func seedDomain(t *testing.T, st *store.Store, domain string, price int64) {
	t.Helper()
	if err := st.SaveNip05Domain(store.Nip05Domain{
		Domain: domain, Enabled: true, PriceSats: price, PeriodDays: 365,
	}); err != nil {
		t.Fatalf("saving domain %s: %v", domain, err)
	}
}

func wellKnown(t *testing.T, h http.Handler, host, name string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/nostr.json?name="+name, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	return rec, body
}

func TestWellKnownNostrJSON(t *testing.T) {
	h, st := newNip05Server(t)
	seedDomain(t, st, "example.com", 100)
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "bob", Pubkey: buyerPubkey,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	rec, body := wellKnown(t, h, "example.com:443", "bob")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// browsers fetch this cross-origin, so the header is not optional
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("CORS header = %q, want *", got)
	}
	names, _ := body["names"].(map[string]any)
	if names["bob"] != buyerPubkey {
		t.Errorf("names = %v, want bob -> %s", names, buyerPubkey)
	}
	relays, _ := body["relays"].(map[string]any)
	if relays[buyerPubkey] == nil {
		t.Errorf("relays = %v, want an entry for the owner", relays)
	}

	// a name nobody bought
	if _, body := wellKnown(t, h, "example.com", "nobody"); len(body["names"].(map[string]any)) != 0 {
		t.Errorf("unknown name answered %v, want an empty map", body["names"])
	}
	// a domain this relay does not serve must not leak names from another one
	if _, body := wellKnown(t, h, "elsewhere.example", "bob"); len(body["names"].(map[string]any)) != 0 {
		t.Errorf("foreign host answered %v, want an empty map", body["names"])
	}
	// no name at all: never a directory of every customer
	if _, body := wellKnown(t, h, "example.com", ""); len(body["names"].(map[string]any)) != 0 {
		t.Errorf("nameless query answered %v, want an empty map", body["names"])
	}
	// a name the regex rejects is answered empty, not with an error
	for _, bad := range []string{"bad+name", strings.Repeat("a", 31), "%20"} {
		rec, body := wellKnown(t, h, "example.com", bad)
		if rec.Code != http.StatusOK {
			t.Errorf("name %q: status = %d, want 200", bad, rec.Code)
		}
		if len(body["names"].(map[string]any)) != 0 {
			t.Errorf("name %q answered %v, want an empty map", bad, body["names"])
		}
	}
}

// An expired name must stop resolving, and the /.well-known answer is the only
// place a client ever looks.
func TestWellKnownDropsExpiredNames(t *testing.T) {
	h, st := newNip05Server(t)
	seedDomain(t, st, "example.com", 100)
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "bob", Pubkey: buyerPubkey,
		ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	if _, body := wellKnown(t, h, "example.com", "bob"); len(body["names"].(map[string]any)) != 0 {
		t.Errorf("expired name answered %v, want an empty map", body["names"])
	}
}

func TestNip05DomainsListsOnlyWhatIsForSale(t *testing.T) {
	h, st := newNip05Server(t)
	seedDomain(t, st, "example.com", 100)
	if err := st.SaveNip05Domain(store.Nip05Domain{
		Domain: "hidden.example", Enabled: false, PriceSats: 100, PeriodDays: 365,
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Nip05PremiumTiers = "1:20,3:5"
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	_, body := do(t, h, http.MethodGet, "/api/nip05/domains", "")
	domains, _ := body["domains"].([]any)
	if len(domains) != 1 {
		t.Fatalf("domains = %v, want only the enabled one", domains)
	}
	if d := domains[0].(map[string]any); d["domain"] != "example.com" {
		t.Errorf("domain = %v, want example.com", d["domain"])
	}
	// the buy page prices short names off this
	if body["premium_tiers"] != "1:20,3:5" {
		t.Errorf("premium_tiers = %v, want the configured spec", body["premium_tiers"])
	}

	// the admin list shows disabled domains too, or they could never be turned on
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/nip05/domains", nil))
	var all []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &all); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	if len(all) != 2 {
		t.Errorf("admin domains = %v, want both", all)
	}
}

func TestNip05Check(t *testing.T) {
	h, st := newNip05Server(t)
	seedDomain(t, st, "example.com", 100)

	rec, body := do(t, h, http.MethodGet, "/api/nip05/check?domain=example.com&name=bob&periods=2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["available"] != true {
		t.Errorf("available = %v, want true", body["available"])
	}
	if body["sats"] != float64(200) {
		t.Errorf("sats = %v, want 200 (two periods)", body["sats"])
	}

	// somebody else's name: a reason, not an error status, so the buy page can
	// show it inline
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "taken", Pubkey: otherPubkey,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	rec, body = do(t, h, http.MethodGet, "/api/nip05/check?domain=example.com&name=taken&pubkey="+buyerPubkey, "")
	if rec.Code != http.StatusOK || body["available"] != false {
		t.Errorf("taken name: status %d body %v, want 200 and available=false", rec.Code, body)
	}
	if body["reason"] == "" || body["reason"] == nil {
		t.Errorf("taken name came back without a reason: %v", body)
	}

	// the holder may renew
	if _, body := do(t, h, http.MethodGet, "/api/nip05/check?domain=example.com&name=taken&pubkey="+otherPubkey, ""); body["available"] != true {
		t.Errorf("renewal check = %v, want available", body)
	}

	// unknown domain and reserved name each get a reason of their own
	for _, path := range []string{
		"/api/nip05/check?domain=nowhere.example&name=bob",
		"/api/nip05/check?domain=example.com&name=NOT%20VALID",
	} {
		if _, body := do(t, h, http.MethodGet, path, ""); body["available"] != false {
			t.Errorf("%s = %v, want available=false", path, body)
		}
	}

	if rec, _ := do(t, h, http.MethodGet, "/api/nip05/check?domain=example.com&name=bob&pubkey=nothex", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad pubkey: status = %d, want 400", rec.Code)
	}
}

func TestNip05NamesOfSkipsUnpaidAndLapsedNames(t *testing.T) {
	h, st := newNip05Server(t)
	seedDomain(t, st, "example.com", 100)
	now := time.Now()

	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "live", Pubkey: buyerPubkey,
		ExpiresAt: now.Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "lapsed", Pubkey: buyerPubkey,
		ExpiresAt: now.Add(-time.Hour).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	// an invoice nobody paid holds the name but must not be listed as owned
	if err := st.ReserveNip05("example.com", "checkout", buyerPubkey, now.Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nip05/names/"+buyerPubkey, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var names []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &names); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	if len(names) != 1 || names[0]["name"] != "live" {
		t.Errorf("names = %v, want only the live one", names)
	}

	if rec, _ := do(t, h, http.MethodGet, "/api/nip05/names/nothex", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad pubkey: status = %d, want 400", rec.Code)
	}
}

func TestAdminSaveNip05Domain(t *testing.T) {
	h, _ := newNip05Server(t)

	rec, body := do(t, h, http.MethodPut, "/api/admin/nip05/domains/Example.COM",
		`{"enabled":true,"price_sats":100,"period_days":365,"note":"hi"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %v, want 200", rec.Code, body)
	}
	// stored lowercased, or the Host-header lookup would never match it
	if body["domain"] != "example.com" {
		t.Errorf("domain = %v, want example.com", body["domain"])
	}

	for _, tc := range []struct{ path, body, why string }{
		{"/api/admin/nip05/domains/ex@mple.com", `{"period_days":365}`, "a hostname has no @ in it"},
		{"/api/admin/nip05/domains/-example.com", `{"period_days":365}`, "a label cannot start with a dash"},
		{"/api/admin/nip05/domains/example.com:8080", `{"period_days":365}`, "a port is not part of the domain"},
		{"/api/admin/nip05/domains/example.com", `{"price_sats":-1,"period_days":365}`, "negative price"},
		{"/api/admin/nip05/domains/example.com", `{"price_sats":100,"period_days":0}`, "a term of zero days"},
	} {
		if rec, _ := do(t, h, http.MethodPut, tc.path, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.why, rec.Code)
		}
	}

	if rec, _ := do(t, h, http.MethodDelete, "/api/admin/nip05/domains/example.com", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete: status = %d, want 204", rec.Code)
	}
}

func TestAdminSaveNip05Name(t *testing.T) {
	h, st := newNip05Server(t)
	seedDomain(t, st, "example.com", 100)

	rec, body := do(t, h, http.MethodPut, "/api/admin/nip05/names/example.com/Bob",
		`{"pubkey":"`+buyerPubkey+`","permanent":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %v, want 200", rec.Code, body)
	}
	if pubkey, _ := st.ResolveNip05("example.com", "bob"); pubkey != buyerPubkey {
		t.Errorf("granted name resolves to %q, want %q", pubkey, buyerPubkey)
	}

	// a name under a domain the relay does not serve would never resolve
	if rec, _ := do(t, h, http.MethodPut, "/api/admin/nip05/names/nowhere.example/bob",
		`{"pubkey":"`+buyerPubkey+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown domain: status = %d, want 400", rec.Code)
	}
	for _, tc := range []struct{ path, body, why string }{
		{"/api/admin/nip05/names/example.com/NOT%20VALID", `{"pubkey":"` + buyerPubkey + `"}`, "malformed name"},
		{"/api/admin/nip05/names/example.com/bob", `{"pubkey":"nothex"}`, "malformed pubkey"},
		{"/api/admin/nip05/names/example.com/bob", `{"pubkey":"` + buyerPubkey + `","expires_at":-1}`, "negative expiry"},
	} {
		if rec, _ := do(t, h, http.MethodPut, tc.path, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.why, rec.Code)
		}
	}

	if rec, _ := do(t, h, http.MethodDelete, "/api/admin/nip05/names/example.com/bob", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete: status = %d, want 204", rec.Code)
	}
	if pubkey, _ := st.ResolveNip05("example.com", "bob"); pubkey != "" {
		t.Errorf("deleted name still resolves to %q", pubkey)
	}
}

// probeTo points the outbound verify fetch at a local test server, so the check
// can be exercised without touching DNS.
func probeTo(t *testing.T, h http.Handler) {
	t.Helper()
	remote := httptest.NewServer(h)
	t.Cleanup(remote.Close)

	old := nip05Probe.Transport
	nip05Probe.Transport = rewriteTo(remote.URL)
	t.Cleanup(func() { nip05Probe.Transport = old })
}

type rewriteTo string

func (base rewriteTo) RoundTrip(r *http.Request) (*http.Response, error) {
	to, err := url.Parse(string(base))
	if err != nil {
		return nil, err
	}
	out := r.Clone(r.Context())
	out.URL.Scheme, out.URL.Host, out.Host = to.Scheme, to.Host, r.URL.Host
	return http.DefaultTransport.RoundTrip(out)
}

// The verify endpoint is what tells an admin whether DNS and the reverse proxy
// actually reach this relay for a domain — the two things that cannot be seen
// from inside the process.
func TestAdminVerifyNip05Domain(t *testing.T) {
	h, st := newNip05Server(t)
	seedDomain(t, st, "example.com", 100)
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "bob", Pubkey: buyerPubkey, Permanent: true,
	}); err != nil {
		t.Fatal(err)
	}

	// the domain really does point here: the relay's own handler answers
	probeTo(t, h)
	_, body := do(t, h, http.MethodGet, "/api/admin/nip05/verify/example.com", "")
	if body["ok"] != true {
		t.Fatalf("verify = %v, want ok", body)
	}
	// with a name sold, the probe checks a pubkey rather than just the shape
	if body["name"] != "bob" || body["got"] != buyerPubkey || body["expected"] != buyerPubkey {
		t.Errorf("verify = %v, want bob answered by the owner", body)
	}
	if body["cors"] != true {
		t.Errorf("cors = %v, want true", body["cors"])
	}

	for _, tc := range []struct {
		why     string
		handler http.HandlerFunc
	}{
		{"another server answers for the domain", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			writeJSON(w, http.StatusOK, map[string]any{"names": map[string]string{"bob": otherPubkey}})
		}},
		{"the proxy drops the Host header, so nothing matches", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			writeJSON(w, http.StatusOK, map[string]any{"names": map[string]string{}})
		}},
		{"the domain serves something else entirely", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("<html>hello</html>"))
		}},
		{"the domain does not serve the endpoint", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusNotFound)
		}},
		{"browsers cannot read an answer without the CORS header", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"names": map[string]string{"bob": buyerPubkey}})
		}},
	} {
		t.Run(tc.why, func(t *testing.T) {
			probeTo(t, tc.handler)
			_, body := do(t, h, http.MethodGet, "/api/admin/nip05/verify/example.com", "")
			if body["ok"] != false {
				t.Errorf("verify = %v, want ok=false", body)
			}
			if body["problem"] == nil || body["problem"] == "" {
				t.Errorf("verify = %v, want a problem an admin can act on", body)
			}
		})
	}

	// a domain nobody bought a name under can still be checked for reachability
	seedDomain(t, st, "empty.example", 100)
	probeTo(t, h)
	_, body = do(t, h, http.MethodGet, "/api/admin/nip05/verify/empty.example", "")
	if body["ok"] != true || body["name"] != "_" {
		t.Errorf("empty domain verify = %v, want ok with the _ probe", body)
	}

	if rec, _ := do(t, h, http.MethodGet, "/api/admin/nip05/verify/ex@mple.com", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed domain: status = %d, want 400", rec.Code)
	}
}

// Blocking reserves a name across every domain, but must not take back one that
// was already sold.
func TestAdminBlockName(t *testing.T) {
	h, st := newNip05Server(t)
	seedDomain(t, st, "example.com", 100)
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "admin", Pubkey: buyerPubkey, Permanent: true,
	}); err != nil {
		t.Fatal(err)
	}

	if rec, _ := do(t, h, http.MethodPut, "/api/admin/nip05/blocked/Admin", `{"reason":"reserved"}`); rec.Code != http.StatusOK {
		t.Fatalf("block: status = %d, want 200", rec.Code)
	}
	if _, body := do(t, h, http.MethodGet, "/api/nip05/check?domain=example.com&name=admin", ""); body["available"] != false {
		t.Errorf("blocked name = %v, want available=false", body)
	}
	if pubkey, _ := st.ResolveNip05("example.com", "admin"); pubkey != buyerPubkey {
		t.Errorf("blocking took back a sold name: resolves to %q", pubkey)
	}

	if rec, _ := do(t, h, http.MethodPut, "/api/admin/nip05/blocked/NOT%20VALID", `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed name: status = %d, want 400", rec.Code)
	}

	if rec, _ := do(t, h, http.MethodDelete, "/api/admin/nip05/blocked/admin", ""); rec.Code != http.StatusNoContent {
		t.Errorf("unblock: status = %d, want 204", rec.Code)
	}
	if _, body := do(t, h, http.MethodGet, "/api/nip05/check?domain=example.com&name=free", ""); body["available"] != true {
		t.Errorf("after unblocking = %v, want available=true", body)
	}
}
