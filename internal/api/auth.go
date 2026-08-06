package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"golang.org/x/crypto/bcrypt"
)

// nip98MaxAge is how far an HTTP auth event's created_at may be from now.
const nip98MaxAge = 60 * time.Second

var errNoAuth = errors.New("no credentials")

// VerifyNIP98 authenticates a request signed per NIP-98 and returns the signing
// pubkey. baseURL is the relay's public origin: the signed "u" tag must match
// it, so a signature captured by one site can't be replayed against another.
func (s *Server) VerifyNIP98(r *http.Request, body []byte) (string, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Nostr ") {
		return "", errNoAuth
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(header, "Nostr ")))
	if err != nil {
		return "", fmt.Errorf("auth event is not valid base64")
	}
	var evt nostr.Event
	if err := json.Unmarshal(raw, &evt); err != nil {
		return "", fmt.Errorf("auth event is not valid json")
	}

	if evt.Kind != nostr.KindHTTPAuth {
		return "", fmt.Errorf("auth event must be kind 27235")
	}
	if ok, err := evt.CheckSignature(); err != nil || !ok {
		return "", fmt.Errorf("auth event signature is invalid")
	}

	age := time.Since(evt.CreatedAt.Time())
	if age < -nip98MaxAge || age > nip98MaxAge {
		return "", fmt.Errorf("auth event timestamp is out of range")
	}

	// The browser signs the URL it actually opened, which is only PanelURL when
	// PANEL_URL is set to the address people really use. Accepting the request's
	// own URL too keeps a misconfigured PANEL_URL from locking the admin out.
	u := evt.Tags.GetFirst([]string{"u"})
	panel := s.cfg.PanelURL + r.URL.Path
	if u == nil {
		return "", fmt.Errorf("auth event has no 'u' tag")
	}
	if !sameURL(u.Value(), panel) && !sameURL(u.Value(), requestURL(r)) {
		return "", fmt.Errorf("auth event 'u' tag is %q, but this request arrived as %q (PANEL_URL is %q)",
			u.Value(), requestURL(r), s.cfg.PanelURL)
	}
	method := evt.Tags.GetFirst([]string{"method"})
	if method == nil || !strings.EqualFold(method.Value(), r.Method) {
		return "", fmt.Errorf("auth event 'method' tag does not match this request")
	}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		payload := evt.Tags.GetFirst([]string{"payload"})
		if payload == nil || !strings.EqualFold(payload.Value(), hex.EncodeToString(sum[:])) {
			return "", fmt.Errorf("auth event 'payload' tag does not match the request body")
		}
	}

	if !s.seen.use(evt.ID) {
		return "", fmt.Errorf("auth event was already used")
	}
	return evt.PubKey, nil
}

func sameURL(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(b, "/"))
}

// requestURL rebuilds the absolute URL the browser asked for, trusting the
// headers a TLS-terminating proxy sets in front of us.
func requestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := firstValue(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = strings.ToLower(proto)
	}
	host := r.Host
	if fwd := firstValue(r.Header.Get("X-Forwarded-Host")); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host + r.URL.Path
}

// firstValue takes the first entry of a comma-separated proxy header.
func firstValue(header string) string {
	return strings.TrimSpace(strings.Split(header, ",")[0])
}

// replayCache remembers recently accepted auth events so a captured header
// can't be used twice.
type replayCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newReplayCache() *replayCache { return &replayCache{seen: map[string]time.Time{}} }

// use records id and reports whether it is fresh (not seen before).
func (c *replayCache) use(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, t := range c.seen {
		if now.Sub(t) > nip98MaxAge {
			delete(c.seen, k)
		}
	}
	if _, dup := c.seen[id]; dup {
		return false
	}
	c.seen[id] = now
	return true
}

// --- password sessions (fallback when no NIP-07 signer is available) ---

const (
	sessionCookie = "nostrel_session"
	sessionTTL    = 12 * time.Hour
)

// issueSession returns a cookie value of "expiry.hmac".
func (s *Server) issueSession() string {
	exp := strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 10)
	return exp + "." + s.signSession(exp)
}

func (s *Server) signSession(payload string) string {
	mac := hmac.New(sha256.New, s.cfg.SessionSecret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	exp, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(sig), []byte(s.signSession(exp))) != 1 {
		return false
	}
	ts, err := strconv.ParseInt(exp, 10, 64)
	return err == nil && ts > time.Now().Unix()
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.issueSession(),
		Path:     "/",
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.cfg.PanelURL, "https://"),
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(sessionTTL),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
}

func (s *Server) checkPassword(password string) bool {
	if s.cfg.AdminPasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(s.cfg.AdminPasswordHash), []byte(password)) == nil
}

// loginLimiter throttles password guessing per IP.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

const (
	loginWindow      = 15 * time.Minute
	loginMaxAttempts = 5
)

func newLoginLimiter() *loginLimiter { return &loginLimiter{attempts: map[string][]time.Time{}} }

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-loginWindow)
	kept := l.attempts[ip][:0]
	for _, t := range l.attempts[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.attempts[ip] = kept
	if len(kept) >= loginMaxAttempts {
		return false
	}
	l.attempts[ip] = append(l.attempts[ip], time.Now())
	return true
}
