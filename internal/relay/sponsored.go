package relay

import (
	"encoding/json"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/store"
)

// Some events are written *about* a customer by somebody who is not one.
//
//   - a NIP-57 zap receipt (kind 9735) is written by the LNURL server that took
//     the payment
//   - a NIP-58 badge award (kind 8) is written by the badge issuer
//
// Neither author has an account here, so on a paid whitelist relay every one of
// them bounces and the customer never sees the zap or the badge. These are let
// through only when they are well formed and name a customer, and they are
// billed to that customer — the person whose storage they occupy.
//
// The relay cannot verify that the payment or the award really happened; only
// the LNURL server or the issuer knows. So an admin who turns these on accepts
// that a stranger can spend a customer's quota, and bans abusers with NIP-86.
// Both switches are off by default.

// KindBadgeAward is NIP-58's award event; go-nostr has no constant for it.
const KindBadgeAward = 8

// sponsored describes an event written on a customer's behalf.
type sponsored struct {
	// candidates are the pubkeys the event is addressed to, in tag order.
	candidates []string
	// enabled reports whether the admin turned this kind of event on.
	enabled func(store.Settings) bool
	// valid checks the shape the NIP requires.
	valid func(*nostr.Event, []string) bool
	// noun names the event in the rejection message.
	noun string
}

// maxRecipients bounds how many pubkeys we look up for one event; a badge award
// may legitimately name several.
const maxRecipients = 20

// classify recognises an event written on somebody else's behalf. The second
// result is false for ordinary events, which are billed to their author.
func classify(evt *nostr.Event) (sponsored, bool) {
	recipients := pTags(evt)
	switch evt.Kind {
	case nostr.KindZap:
		return sponsored{
			candidates: recipients,
			enabled:    func(s store.Settings) bool { return s.AcceptZapReceipts },
			valid:      validZapReceipt,
			noun:       "zap receipt",
		}, true
	case KindBadgeAward:
		return sponsored{
			candidates: recipients,
			enabled:    func(s store.Settings) bool { return s.AcceptBadgeAwards },
			valid:      validBadgeAward,
			noun:       "badge award",
		}, true
	}
	return sponsored{}, false
}

func pTags(evt *nostr.Event) []string {
	var out []string
	for _, tag := range evt.Tags {
		if len(tag) < 2 || tag[0] != "p" || len(tag[1]) != 64 {
			continue
		}
		out = append(out, tag[1])
		if len(out) == maxRecipients {
			break
		}
	}
	return out
}

// payer picks which of the named pubkeys the event is billed to: the first one
// the relay already knows. Existence rather than remaining quota is the test,
// so the write gate and the accounting that follows it always agree.
func (r *Relay) payer(candidates []string) string {
	for _, candidate := range candidates {
		allowance, err := r.store.Allowance(candidate)
		if err != nil {
			r.Log.Printf("account lookup failed for %s: %v", candidate, err)
			return ""
		}
		if allowance.Account != nil || allowance.Group != nil {
			return candidate
		}
	}
	return ""
}

// billedPubkey is whose quota an event costs: its author, unless it was written
// on a customer's behalf.
func (r *Relay) billedPubkey(evt *nostr.Event) string {
	if kind, ok := classify(evt); ok {
		if payer := r.payer(kind.candidates); payer != "" {
			return payer
		}
	}
	return evt.PubKey
}

// validZapReceipt requires a payment reference and the original zap request,
// correctly signed and naming the same recipient.
func validZapReceipt(evt *nostr.Event, recipients []string) bool {
	// a receipt is for exactly one person; several would make billing arbitrary
	if len(recipients) != 1 {
		return false
	}
	if evt.Tags.Find("bolt11").Value() == "" {
		return false
	}

	description := evt.Tags.Find("description").Value()
	if description == "" {
		return false
	}
	var request nostr.Event
	if err := json.Unmarshal([]byte(description), &request); err != nil {
		return false
	}
	if request.Kind != nostr.KindZapRequest {
		return false
	}
	if request.Tags.Find("p").Value() != recipients[0] {
		return false
	}
	if !request.CheckID() {
		return false
	}
	ok, err := request.CheckSignature()
	return ok && err == nil
}

// validBadgeAward requires the award to point at a badge definition the author
// published themselves, so nobody can hand out somebody else's badge.
func validBadgeAward(evt *nostr.Event, recipients []string) bool {
	if len(recipients) == 0 {
		return false
	}
	parts := strings.SplitN(evt.Tags.Find("a").Value(), ":", 3)
	return len(parts) == 3 && parts[0] == "30009" && parts[1] == evt.PubKey && parts[2] != ""
}
