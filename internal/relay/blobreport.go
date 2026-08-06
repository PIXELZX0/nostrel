package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/store"
)

// handleBlobReport implements BUD-09: a signed kind 1984 event naming the blobs
// somebody wants moderated. Reports are queued for an admin, never acted on
// automatically — anyone can report anything.
func (r *Relay) handleBlobReport(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		blobReportError(w, "could not read the request body", http.StatusBadRequest)
		return
	}

	var evt nostr.Event
	if err := json.Unmarshal(body, &evt); err != nil {
		blobReportError(w, "expected a nostr event", http.StatusBadRequest)
		return
	}
	if evt.Kind != nostr.KindReporting {
		blobReportError(w, "report must be a kind 1984 event", http.StatusBadRequest)
		return
	}
	if ok, err := evt.CheckSignature(); err != nil || !ok {
		blobReportError(w, "invalid signature", http.StatusForbidden)
		return
	}

	reason := strings.TrimSpace(evt.Content)
	reported := 0
	for _, tag := range evt.Tags {
		if len(tag) < 2 || tag[0] != "x" {
			continue
		}
		hash := tag[1]
		if len(hash) != 64 {
			continue
		}
		if len(tag) >= 3 && tag[2] != "" {
			reason = tag[2] + ": " + reason
		}
		note := strings.TrimSpace("reported by " + evt.PubKey[:8] + "… " + reason)
		if err := r.store.ModAdd(store.ModReportedBlob, hash, note); err != nil {
			r.Log.Printf("storing blob report for %s: %v", hash, err)
			blobReportError(w, "could not store the report", http.StatusInternalServerError)
			return
		}
		reported++
	}

	if reported == 0 {
		blobReportError(w, "report must name at least one blob in an 'x' tag", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func blobReportError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}
