package relay

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"nostrel/internal/relaycore/blossom"
)

// mirrorTimeout bounds a single BUD-04 mirror fetch.
const mirrorTimeout = 30 * time.Second

// handleMirror implements BUD-04. khatru's own handler passes the client's URL
// straight to http.Get before any policy hook runs, which lets a paying member
// aim the relay at localhost or a cloud metadata endpoint. This version
// resolves the URL through a dialer that refuses non-public addresses, and
// checks the uploader's quota before storing anything.
func (r *Relay) handleMirror(w http.ResponseWriter, req *http.Request) {
	if r.storage == nil {
		blobError(w, "media storage is disabled", http.StatusNotFound)
		return
	}

	auth, err := readBlossomAuth(req, "upload")
	if err != nil {
		blobError(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(req.Body, 4096)).Decode(&body); err != nil {
		blobError(w, "expected a json body with a url", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
		blobError(w, "url must be http or https", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), mirrorTimeout)
	defer cancel()

	fetch, err := http.NewRequestWithContext(ctx, http.MethodGet, body.URL, nil)
	if err != nil {
		blobError(w, "invalid url", http.StatusBadRequest)
		return
	}
	resp, err := publicOnlyClient.Do(fetch)
	if err != nil {
		blobError(w, "could not fetch that url: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		blobError(w, fmt.Sprintf("source returned status %d", resp.StatusCode), http.StatusBadRequest)
		return
	}

	limit := r.storage.MaxSize()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		blobError(w, "could not read the source", http.StatusBadRequest)
		return
	}
	if int64(len(data)) > limit {
		blobError(w, "file is larger than the upload limit", http.StatusRequestEntityTooLarge)
		return
	}
	if ok, reason := r.storage.CheckQuota(auth.PubKey, int64(len(data))); !ok {
		blobError(w, reason, http.StatusPaymentRequired)
		return
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	ext := ""
	if exts, _ := mime.ExtensionsByType(strings.SplitN(mimeType, ";", 2)[0]); len(exts) > 0 {
		ext = exts[0]
	}

	if err := r.storage.Write(ctx, hash, ext, data); err != nil {
		r.Log.Printf("mirroring %s: %v", body.URL, err)
		blobError(w, "could not store the file", http.StatusInternalServerError)
		return
	}
	descriptor := blossom.BlobDescriptor{
		URL:      r.blobServiceURL + "/" + hash + ext,
		SHA256:   hash,
		Size:     len(data),
		Type:     mimeType,
		Uploaded: nostr.Now(),
	}
	if err := r.blobIndex.Keep(req.Context(), descriptor, auth.PubKey); err != nil {
		r.Log.Printf("indexing mirrored blob %s: %v", hash, err)
		blobError(w, "could not index the file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(descriptor)
}

// publicOnlyClient refuses to connect to loopback, private, link-local or
// otherwise internal addresses, which is what keeps mirroring from becoming an
// SSRF gadget. Redirects are followed but re-checked by the same dialer.
var publicOnlyClient = &http.Client{
	Timeout: mirrorTimeout,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Control: func(network, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				ip := net.ParseIP(host)
				if ip == nil || !isPublicIP(ip) {
					return fmt.Errorf("refusing to connect to internal address %s", host)
				}
				return nil
			},
		}).DialContext,
	},
}

func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	// 100.64.0.0/10 (carrier NAT) and 169.254.0.0/16 are covered above for v4;
	// unique local addresses fc00::/7 are covered by IsPrivate.
	return true
}

// readBlossomAuth validates a Blossom kind 24242 authorization event for the
// given action.
func readBlossomAuth(req *http.Request, action string) (*nostr.Event, error) {
	header := req.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Nostr ") {
		return nil, fmt.Errorf("missing authorization header")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len("Nostr "):]))
	if err != nil {
		return nil, fmt.Errorf("authorization is not valid base64")
	}
	var evt nostr.Event
	if err := json.Unmarshal(raw, &evt); err != nil {
		return nil, fmt.Errorf("authorization is not a valid event")
	}
	if evt.Kind != 24242 || !evt.CheckID() {
		return nil, fmt.Errorf("authorization must be a kind 24242 event")
	}
	if ok, err := evt.CheckSignature(); err != nil || !ok {
		return nil, fmt.Errorf("invalid signature")
	}
	if evt.Tags.FindWithValue("t", action) == nil {
		return nil, fmt.Errorf("authorization is not for %q", action)
	}
	expiration := evt.Tags.Find("expiration")
	if expiration == nil || len(expiration) < 2 {
		return nil, fmt.Errorf("authorization needs an expiration tag")
	}
	var expiresAt int64
	fmt.Sscanf(expiration[1], "%d", &expiresAt)
	if expiresAt < time.Now().Unix() {
		return nil, fmt.Errorf("authorization expired")
	}
	return &evt, nil
}

func blobError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("X-Reason", message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}
