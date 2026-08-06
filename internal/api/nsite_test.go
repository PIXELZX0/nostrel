package api

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/blobs"
	"nostrel/internal/config"
	"nostrel/internal/store"
)

func TestSplitNsiteHost(t *testing.T) {
	domains := []string{"sites.example.com", "nsite.test"}
	cases := []struct {
		host   string
		name   string
		domain string
		ok     bool
	}{
		{"alice.sites.example.com", "alice", "sites.example.com", true},
		{"bob.nsite.test", "bob", "nsite.test", true},
		{"sites.example.com", "", "", false},       // the bare host is not a site
		{"a.b.sites.example.com", "", "", false},   // only one label is a name
		{"alice.elsewhere.example", "", "", false}, // not a registered nsite domain
	}
	for _, tc := range cases {
		name, domain, ok := splitNsiteHost(tc.host, domains)
		if ok != tc.ok || name != tc.name || domain != tc.domain {
			t.Errorf("splitNsiteHost(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.host, name, domain, ok, tc.name, tc.domain, tc.ok)
		}
	}
}

func TestRequestedPath(t *testing.T) {
	cases := map[string]string{
		"/":           "/index.html",
		"":            "/index.html",
		"/about/":     "/about/index.html",
		"/about":      "/about/index.html",
		"/style.css":  "/style.css",
		"/a/b/app.js": "/a/b/app.js",
	}
	for in, want := range cases {
		if got := requestedPath(in); got != want {
			t.Errorf("requestedPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// newNsiteServer wires a relay that hosts sites at <name>.sites.example.com.
func newNsiteServer(t *testing.T) (http.Handler, *store.Store, *blobs.Storage) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)

	settings, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.NsiteDomains = "sites.example.com"
	settings.StorageBackend = "local"
	settings.LocalPath = filepath.Join(dir, "blobs")
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	storage, err := blobs.New(st, filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatal(err)
	}

	s := New(&config.Config{PanelURL: panelURL}, st, nil, log.New(io.Discard, "", 0))
	s.EnableMedia(storage, storage.Index(st, "http://localhost"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.orNsite(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("panel index"))
	}))
	mux.HandleFunc("GET /", s.orNsite(http.NotFound))
	return mux, st, storage
}

func get(t *testing.T, h http.Handler, host, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNsiteServesAPublishedSite(t *testing.T) {
	h, st, storage := newNsiteServer(t)
	ctx := context.Background()

	index := []byte("<h1>hello</h1>")
	indexHash := "1111111111111111111111111111111111111111111111111111111111111111"
	missing := "2222222222222222222222222222222222222222222222222222222222222222"
	notFound := []byte("gone")
	notFoundHash := "3333333333333333333333333333333333333333333333333333333333333333"
	for hash, body := range map[string][]byte{indexHash: index, notFoundHash: notFound} {
		if err := storage.Write(ctx, hash, ".html", body); err != nil {
			t.Fatal(err)
		}
	}

	if err := st.SaveNip05Domain(store.Nip05Domain{
		Domain: "sites.example.com", Enabled: true, PriceSats: 1, PeriodDays: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "sites.example.com", Name: "alice", Pubkey: buyerPubkey, Permanent: true,
	}); err != nil {
		t.Fatal(err)
	}

	manifest := &nostr.Event{
		ID: "site1", PubKey: buyerPubkey, Kind: kindRootSite, CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"path", "/index.html", indexHash},
			{"path", "/404.html", notFoundHash},
			{"path", "/broken.html", missing},
		},
	}
	if err := st.Events.SaveEvent(ctx, manifest); err != nil {
		t.Fatal(err)
	}

	// the panel is untouched on its own host
	if rec := get(t, h, "relay.example.com", "/"); rec.Body.String() != "panel index" {
		t.Errorf("panel index = %q, want it served unchanged", rec.Body.String())
	}

	// "/" resolves to index.html
	rec := get(t, h, "alice.sites.example.com", "/")
	if rec.Code != http.StatusOK || rec.Body.String() != string(index) {
		t.Errorf("site index = %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content type = %q", got)
	}

	// an unknown path falls back to the site's own 404 page
	rec = get(t, h, "alice.sites.example.com", "/nope.html")
	if rec.Code != http.StatusNotFound || rec.Body.String() != string(notFound) {
		t.Errorf("404 fallback = %d %q", rec.Code, rec.Body.String())
	}

	// a path in the manifest whose blob is gone is an honest 404, not a panic
	if rec = get(t, h, "alice.sites.example.com", "/broken.html"); rec.Code != http.StatusNotFound {
		t.Errorf("missing blob = %d, want 404", rec.Code)
	}

	// a name nobody bought has no site
	if rec = get(t, h, "carol.sites.example.com", "/"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown name = %d, want 404", rec.Code)
	}
}
