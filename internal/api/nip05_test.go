package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"nostrel/internal/config"
	"nostrel/internal/store"
)

const buyerPubkey = "aa11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"

// newNip05Server wires enough of the panel to answer /.well-known/nostr.json.
func newNip05Server(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(st.Close)

	cfg := &config.Config{PanelURL: panelURL, ServiceURL: "wss://relay.example.com"}
	s := New(cfg, st, nil, log.New(io.Discard, "", 0))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/nostr.json", s.handleWellKnownNostr)
	return mux, st
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
	if err := st.SaveNip05Domain(store.Nip05Domain{
		Domain: "example.com", Enabled: true, PriceSats: 100, PeriodDays: 365,
	}); err != nil {
		t.Fatal(err)
	}
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
}
