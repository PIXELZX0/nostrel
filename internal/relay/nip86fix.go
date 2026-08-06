package relay

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// go-nostr's NIP-86 request decoder type-asserts the method list of
// grantadmin/revokeadmin straight to []string, which panics on the []any a JSON
// array decodes into — those two methods can never reach khatru's dispatcher.
// Handler intercepts them and runs them here; everything else goes to khatru
// untouched.
//
// Remove this once the upstream decoder checks that assertion.
func (r *Relay) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// BUD-09 blob reports: khatru reads the body into a nil slice, so its
		// handler can never parse one. Handle it here instead.
		if req.Method == http.MethodPut && req.URL.Path == "/report" {
			r.handleBlobReport(w, req)
			return
		}
		// BUD-04 mirroring: khatru fetches the client's URL with no address
		// checks at all, so we do it ourselves. See handleMirror.
		if req.Method == http.MethodPut && req.URL.Path == "/mirror" {
			r.handleMirror(w, req)
			return
		}

		// NIP-43 wants a `self` field khatru's document type cannot carry.
		if r.nip43Enabled() && req.Header.Get("Accept") == "application/nostr+json" {
			r.serveNIP11(w, req)
			return
		}

		if req.Method == http.MethodPost &&
			strings.Contains(req.Header.Get("Content-Type"), "nostr+json+rpc") {

			body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
			if err == nil {
				req.Body = io.NopCloser(bytes.NewReader(body))
				var parsed struct {
					Method string `json:"method"`
					Params []any  `json:"params"`
				}
				if json.Unmarshal(body, &parsed) == nil &&
					(parsed.Method == "grantadmin" || parsed.Method == "revokeadmin") {
					r.handleAdminGrant(w, req, body, parsed.Method, parsed.Params)
					return
				}
			}
		}
		r.Relay.ServeHTTP(w, req)
	})
}

func (r *Relay) handleAdminGrant(w http.ResponseWriter, req *http.Request, body []byte, method string, params []any) {
	w.Header().Set("Content-Type", "application/nostr+json+rpc")
	respond := func(result any, errMsg string) {
		out := map[string]any{}
		if errMsg != "" {
			out["error"] = errMsg
		} else {
			out["result"] = result
		}
		json.NewEncoder(w).Encode(out)
	}

	pubkey, err := r.verifyNIP86Auth(req, body)
	if err != nil {
		respond(nil, err.Error())
		return
	}
	if !r.IsAdmin(pubkey) {
		respond(nil, "you are not an admin of this relay")
		return
	}

	if len(params) < 1 {
		respond(nil, "invalid number of params for '"+method+"'")
		return
	}
	target, ok := params[0].(string)
	if !ok || len(target) != 64 {
		respond(nil, "first param must be a hex pubkey")
		return
	}

	if method == "grantadmin" {
		err = r.grantAdmin(req.Context(), target, nil)
	} else {
		err = r.revokeAdmin(req.Context(), target, nil)
	}
	if err != nil {
		respond(nil, err.Error())
		return
	}
	respond(true, "")
}

// verifyNIP86Auth applies the same rules khatru uses for the management API:
// a kind 27235 event signed for this exact URL and body, no older than 30s.
func (r *Relay) verifyNIP86Auth(req *http.Request, body []byte) (string, error) {
	header := req.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Nostr ") {
		return "", errMsg("missing auth")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(header, "Nostr ")))
	if err != nil {
		return "", errMsg("invalid base64 auth")
	}
	var evt nostr.Event
	if err := json.Unmarshal(raw, &evt); err != nil {
		return "", errMsg("invalid auth event json")
	}
	if evt.Kind != nostr.KindHTTPAuth {
		return "", errMsg("auth event must be kind 27235")
	}
	if ok, err := evt.CheckSignature(); err != nil || !ok {
		return "", errMsg("invalid auth event")
	}
	if evt.CreatedAt.Time().Before(time.Now().Add(-30 * time.Second)) {
		return "", errMsg("auth event is too old")
	}

	uTag := evt.Tags.GetFirst([]string{"u"})
	if uTag == nil || nostr.NormalizeURL(uTag.Value()) != nostr.NormalizeURL(r.baseURL(req)) {
		return "", errMsg("invalid 'u' tag")
	}
	sum := sha256.Sum256(body)
	if evt.Tags.FindWithValue("payload", hex.EncodeToString(sum[:])) == nil {
		return "", errMsg("invalid auth event payload hash")
	}
	return evt.PubKey, nil
}

// baseURL mirrors khatru's own guess so a client can sign one 'u' tag that
// works for every management method.
func (r *Relay) baseURL(req *http.Request) string {
	if r.ServiceURL != "" {
		return r.ServiceURL
	}
	host := req.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = req.Host
	}
	proto := req.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if strings.Contains(host, ":") || host == "localhost" {
			proto = "http"
		} else {
			proto = "https"
		}
	}
	return proto + "://" + host
}

type errMsg string

func (e errMsg) Error() string { return string(e) }
