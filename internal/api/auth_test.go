package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/config"
)

const panelURL = "https://relay.example.com"

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{PanelURL: panelURL, SessionSecret: []byte("test-secret")}
	return New(cfg, nil, nil, log.New(io.Discard, "", 0))
}

// signAuth builds a NIP-98 Authorization header value.
func signAuth(t *testing.T, sk, url, method string, body []byte, createdAt time.Time) string {
	t.Helper()
	tags := nostr.Tags{{"u", url}, {"method", method}}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		tags = append(tags, nostr.Tag{"payload", hex.EncodeToString(sum[:])})
	}
	evt := nostr.Event{
		Kind:      nostr.KindHTTPAuth,
		CreatedAt: nostr.Timestamp(createdAt.Unix()),
		Tags:      tags,
	}
	if err := evt.Sign(sk); err != nil {
		t.Fatalf("signing auth event: %v", err)
	}
	raw, _ := json.Marshal(evt)
	return "Nostr " + base64.StdEncoding.EncodeToString(raw)
}

func request(t *testing.T, header, method, path string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Authorization", header)
	return r
}

func TestVerifyNIP98(t *testing.T) {
	s := newTestServer(t)
	sk := nostr.GeneratePrivateKey()
	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("valid signature is accepted", func(t *testing.T) {
		header := signAuth(t, sk, panelURL+"/api/admin/stats", "GET", nil, time.Now())
		got, err := s.VerifyNIP98(request(t, header, "GET", "/api/admin/stats", nil), nil)
		if err != nil {
			t.Fatalf("valid header rejected: %v", err)
		}
		if got != pk {
			t.Errorf("pubkey = %s, want %s", got, pk)
		}
	})

	t.Run("replayed header is rejected", func(t *testing.T) {
		// a separate key, so this event differs from the one signed above:
		// identical events in the same second share an id, which the replay
		// cache would reject on its own
		sk := nostr.GeneratePrivateKey()
		header := signAuth(t, sk, panelURL+"/api/admin/stats", "GET", nil, time.Now())
		if _, err := s.VerifyNIP98(request(t, header, "GET", "/api/admin/stats", nil), nil); err != nil {
			t.Fatalf("first use rejected: %v", err)
		}
		if _, err := s.VerifyNIP98(request(t, header, "GET", "/api/admin/stats", nil), nil); err == nil {
			t.Error("second use of the same auth event was accepted")
		}
	})

	t.Run("stale timestamp is rejected", func(t *testing.T) {
		header := signAuth(t, sk, panelURL+"/api/admin/stats", "GET", nil, time.Now().Add(-5*time.Minute))
		if _, err := s.VerifyNIP98(request(t, header, "GET", "/api/admin/stats", nil), nil); err == nil {
			t.Error("stale auth event was accepted")
		}
	})

	t.Run("url signed for another site is rejected", func(t *testing.T) {
		header := signAuth(t, sk, "https://evil.example.com/api/admin/stats", "GET", nil, time.Now())
		if _, err := s.VerifyNIP98(request(t, header, "GET", "/api/admin/stats", nil), nil); err == nil {
			t.Error("auth event for a different origin was accepted")
		}
	})

	// PANEL_URL is the address the operator declared; the browser can only sign
	// the one it actually opened. When they differ the admin must still get in.
	t.Run("url the request actually arrived on is accepted", func(t *testing.T) {
		header := signAuth(t, sk, "http://192.0.2.10:3334/api/admin/stats", "GET", nil, time.Now())
		r := request(t, header, "GET", "/api/admin/stats", nil)
		r.Host = "192.0.2.10:3334"
		if _, err := s.VerifyNIP98(r, nil); err != nil {
			t.Fatalf("auth event signed for this very request was rejected: %v", err)
		}
	})

	t.Run("url behind a tls proxy is accepted", func(t *testing.T) {
		sk := nostr.GeneratePrivateKey()
		header := signAuth(t, sk, "https://panel.example.org/api/admin/stats", "GET", nil, time.Now())
		r := request(t, header, "GET", "/api/admin/stats", nil)
		r.Host = "panel.example.org"
		r.Header.Set("X-Forwarded-Proto", "https")
		if _, err := s.VerifyNIP98(r, nil); err != nil {
			t.Fatalf("auth event matching the proxied URL was rejected: %v", err)
		}
	})

	t.Run("url signed for another path is rejected", func(t *testing.T) {
		header := signAuth(t, sk, panelURL+"/api/admin/settings", "GET", nil, time.Now())
		if _, err := s.VerifyNIP98(request(t, header, "GET", "/api/admin/stats", nil), nil); err == nil {
			t.Error("auth event for a different path was accepted")
		}
	})

	t.Run("method mismatch is rejected", func(t *testing.T) {
		header := signAuth(t, sk, panelURL+"/api/admin/stats", "GET", nil, time.Now())
		if _, err := s.VerifyNIP98(request(t, header, "DELETE", "/api/admin/stats", nil), nil); err == nil {
			t.Error("auth event signed for GET was accepted on DELETE")
		}
	})

	t.Run("tampered body is rejected", func(t *testing.T) {
		body := []byte(`{"status":"active"}`)
		header := signAuth(t, sk, panelURL+"/api/admin/accounts/x", "PUT", body, time.Now())
		tampered := []byte(`{"status":"banned"}`)
		if _, err := s.VerifyNIP98(request(t, header, "PUT", "/api/admin/accounts/x", tampered), tampered); err == nil {
			t.Error("auth event with a mismatched payload hash was accepted")
		}
	})

	t.Run("wrong kind is rejected", func(t *testing.T) {
		evt := nostr.Event{Kind: 1, CreatedAt: nostr.Now(), Tags: nostr.Tags{
			{"u", panelURL + "/api/admin/stats"}, {"method", "GET"}}}
		evt.Sign(sk)
		raw, _ := json.Marshal(evt)
		header := "Nostr " + base64.StdEncoding.EncodeToString(raw)
		if _, err := s.VerifyNIP98(request(t, header, "GET", "/api/admin/stats", nil), nil); err == nil {
			t.Error("non-27235 event was accepted")
		}
	})

	t.Run("forged signature is rejected", func(t *testing.T) {
		header := signAuth(t, sk, panelURL+"/api/admin/stats", "GET", nil, time.Now())
		raw, _ := base64.StdEncoding.DecodeString(header[len("Nostr "):])
		var evt nostr.Event
		json.Unmarshal(raw, &evt)
		evt.PubKey = "00" + evt.PubKey[2:] // claim to be somebody else
		forged, _ := json.Marshal(evt)
		bad := "Nostr " + base64.StdEncoding.EncodeToString(forged)
		if _, err := s.VerifyNIP98(request(t, bad, "GET", "/api/admin/stats", nil), nil); err == nil {
			t.Error("event with a mismatched pubkey was accepted")
		}
	})

	t.Run("missing header is rejected", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/api/admin/stats", nil)
		if _, err := s.VerifyNIP98(r, nil); err == nil {
			t.Error("request without an Authorization header was accepted")
		}
	})
}

func TestSessionCookie(t *testing.T) {
	s := newTestServer(t)

	w := httptest.NewRecorder()
	s.setSessionCookie(w, httptest.NewRequest("POST", "/api/admin/login", nil))
	cookie := w.Result().Cookies()[0]

	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Error("session cookie must be HttpOnly and SameSite=Strict")
	}

	r := httptest.NewRequest("GET", "/api/admin/stats", nil)
	r.AddCookie(cookie)
	if !s.validSession(r) {
		t.Error("freshly issued session was rejected")
	}

	r = httptest.NewRequest("GET", "/api/admin/stats", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie.Value + "0"})
	if s.validSession(r) {
		t.Error("session with a tampered signature was accepted")
	}

	// an expiry in the past, signed with the right key, must still fail
	past := "1000000000"
	r = httptest.NewRequest("GET", "/api/admin/stats", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: past + "." + s.signSession(past)})
	if s.validSession(r) {
		t.Error("expired session was accepted")
	}
}
