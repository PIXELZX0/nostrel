package relay

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// NIP-43 requires the relay to advertise the pubkey it signs its own events
// with, in a NIP-11 field called `self`. go-nostr's RelayInformationDocument has
// no such field and khatru marshals that struct directly, so there is no hook
// to add one.
//
// Rather than fork either library, the rendered document is decoded, the field
// is inserted, and it is written out again. NIP-11 is a small document fetched
// rarely, so the extra round trip does not matter.

// serveNIP11 renders khatru's document with `self` added. It is only used when
// NIP-43 is on; otherwise there is nothing to add and khatru answers directly.
func (r *Relay) serveNIP11(w http.ResponseWriter, req *http.Request) {
	buffered := &captureWriter{header: http.Header{}}
	r.Relay.HandleNIP11(buffered, req)

	var document map[string]any
	if err := json.Unmarshal(buffered.body.Bytes(), &document); err != nil {
		r.Log.Printf("nip-11: could not re-read our own document: %v", err)
		copyHeader(w.Header(), buffered.header)
		w.WriteHeader(buffered.status())
		w.Write(buffered.body.Bytes())
		return
	}
	document["self"] = r.selfPubkey

	copyHeader(w.Header(), buffered.header)
	w.WriteHeader(buffered.status())
	json.NewEncoder(w).Encode(document)
}

// captureWriter buffers a handler's response so it can be edited.
type captureWriter struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (c *captureWriter) Header() http.Header         { return c.header }
func (c *captureWriter) Write(p []byte) (int, error) { return c.body.Write(p) }
func (c *captureWriter) WriteHeader(code int)        { c.code = code }
func (c *captureWriter) status() int {
	if c.code == 0 {
		return http.StatusOK
	}
	return c.code
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		dst[key] = values
	}
}
