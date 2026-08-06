package relay

import (
	"context"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// NIP-62 request to vanish: the user asks a relay to forget everything it
// holds about them. A paid relay is explicitly not exempt, so this is honoured
// whether or not the author ever had an account.

// KindRequestToVanish is NIP-62's event; go-nostr has no constant for it.
const KindRequestToVanish = 62

// allRelays is the wildcard a client uses to ask every relay at once.
const allRelays = "ALL_RELAYS"

// maxVanishReason caps the note a user may attach, so an event we accept from
// somebody with no account cannot be used as free storage.
const maxVanishReason = 1024

// addressesUs reports whether a vanish request names this relay.
func (r *Relay) addressedToUs(evt *nostr.Event) bool {
	for _, tag := range evt.Tags {
		if len(tag) < 2 || tag[0] != "relay" {
			continue
		}
		if tag[1] == allRelays || sameRelayURL(tag[1], r.cfg.ServiceURL) {
			return true
		}
	}
	return false
}

// sameRelayURL compares two relay addresses the way operators write them:
// scheme and a trailing slash are noise.
func sameRelayURL(a, b string) bool {
	strip := func(url string) string {
		url = strings.ToLower(strings.TrimSpace(url))
		url = strings.TrimPrefix(strings.TrimPrefix(url, "wss://"), "ws://")
		url = strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
		return strings.TrimSuffix(url, "/")
	}
	return b != "" && strip(a) == strip(b)
}

// vanish carries out a request that has already been accepted and stored. It
// runs after the event is saved so the request itself survives as the record
// of what was asked and when.
func (r *Relay) vanish(ctx context.Context, evt *nostr.Event) {
	if evt.Kind != KindRequestToVanish || !r.addressedToUs(evt) {
		return
	}

	cutoff := int64(evt.CreatedAt)
	deleted, err := r.store.Vanish(evt.PubKey, cutoff, evt.Content)
	if err != nil {
		r.Log.Printf("vanish %s: %v", evt.PubKey, err)
		return
	}

	forgotten := 0
	if r.blobIndex != nil {
		if forgotten, err = r.blobIndex.Forget(ctx, evt.PubKey); err != nil {
			r.Log.Printf("vanish %s: forgetting blobs: %v", evt.PubKey, err)
		}
	}
	r.Log.Printf("vanish: %s asked to be forgotten — %d events, %d files removed",
		evt.PubKey, deleted, forgotten)
}
