package relay

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
	"github.com/nbd-wtf/go-nostr/nip40"
	"nostrel/internal/relaycore"

	"nostrel/internal/store"
)

// privateKinds are only readable by the pubkey they are addressed to: NIP-04
// direct messages and the NIP-59 gift wraps used by NIP-17 private chats.
// Reading them requires NIP-42 auth as the sender or the recipient.
var privateKinds = []int{
	nostr.KindEncryptedDirectMessage, // 4
	nostr.KindGiftWrap,               // 1059
	nostr.KindSeal,                   // 13
}

// rejectEvent is the paid-whitelist gate plus the relay-wide event policy:
// banned events, kind policy, proof of work and timestamp limits.
func (r *Relay) rejectEvent(ctx context.Context, evt *nostr.Event) (bool, string) {
	if r.store.ModHas(store.ModBannedEvent, evt.ID) {
		return true, "blocked: this event was banned by a relay admin"
	}

	// Nothing the author already asked us to forget may come back.
	if cutoff := r.store.VanishedAt(evt.PubKey); cutoff > 0 && int64(evt.CreatedAt) <= cutoff {
		return true, "blocked: this pubkey asked to be forgotten here"
	}

	// NIP-43 requests are addressed to the relay itself, so they are answered
	// rather than charged for. They are ephemeral and never stored.
	switch evt.Kind {
	case KindJoinRequest:
		return r.handleJoin(ctx, evt)
	case KindLeaveRequest:
		return r.handleLeave(ctx, evt)
	}

	// A vanish request is honoured whether or not its author is a customer, so
	// it skips the paywall entirely. The size cap keeps that from being a way
	// to store data for free.
	if evt.Kind == KindRequestToVanish {
		if !r.addressedToUs(evt) {
			return true, "invalid: this request to vanish does not name this relay"
		}
		if len(evt.Content) > maxVanishReason {
			return true, "invalid: request to vanish is too long"
		}
		return false, ""
	}

	settings, err := r.store.Settings()
	if err != nil {
		r.Log.Printf("settings read failed: %v", err)
		return true, "error: could not read relay settings"
	}
	if !settings.KindAllowed(evt.Kind) {
		return true, "blocked: this relay does not accept kind " + strconv.Itoa(evt.Kind)
	}

	if r.cfg.MinPoW > 0 {
		// CommittedDifficulty only counts leading zeroes that the author
		// committed to in the nonce tag, so a lucky id can't pass for work.
		if nip13.CommittedDifficulty(evt) < r.cfg.MinPoW {
			return true, "pow: need at least " + strconv.Itoa(r.cfg.MinPoW) + " bits of difficulty"
		}
	}

	now := time.Now()
	if r.cfg.MaxFutureDrift > 0 && evt.CreatedAt.Time().After(now.Add(r.cfg.MaxFutureDrift)) {
		return true, "invalid: created_at is too far in the future"
	}
	// NIP-59 gift wraps deliberately backdate created_at by up to two days to
	// hide when the message was really sent, so the past limit must not apply
	// to them or private messaging breaks whenever an admin sets one.
	if r.cfg.MaxPastDrift > 0 && evt.Kind != nostr.KindGiftWrap &&
		evt.CreatedAt.Time().Before(now.Add(-r.cfg.MaxPastDrift)) {
		return true, "invalid: created_at is too far in the past"
	}

	// Zap receipts and badge awards are written by a stranger about a customer,
	// so they are checked and billed against that customer instead.
	payer := evt.PubKey
	if kind, ok := classify(evt); ok {
		if !kind.enabled(settings) {
			return true, "blocked: this relay does not accept a " + kind.noun + " from a third party"
		}
		if !kind.valid(evt, kind.candidates) {
			return true, "invalid: malformed " + kind.noun
		}
		if payer = r.payer(kind.candidates); payer == "" {
			return true, "restricted: this " + kind.noun + " does not name anybody with an account here"
		}
	}

	// the author's own account first, then the group they share storage with
	allowance, err := r.store.Allowance(payer)
	if err != nil {
		r.Log.Printf("account lookup failed for %s: %v", payer, err)
		return true, "error: could not verify your account"
	}
	if allowance.Account == nil && allowance.Group == nil {
		return true, "restricted: not whitelisted — get access at " + r.cfg.PanelURL
	}
	if ok, _, msg := allowance.CanWrite(now.Unix(), eventSize(evt)); !ok {
		return true, msg + " — " + r.cfg.PanelURL
	}
	return false, ""
}

// rejectFilter keeps private-kind events readable only by their participants,
// and (when READ_AUTH_REQUIRED is set) restricts all reads to whitelisted
// pubkeys.
func (r *Relay) rejectFilter(ctx context.Context, filter nostr.Filter) (bool, string) {
	authed := relaycore.GetAuthed(ctx)

	if wantsPrivateKinds(filter) {
		if authed == "" {
			return true, "auth-required: authenticate to read direct messages"
		}
		if !addressedTo(filter, authed) {
			return true, "restricted: you can only read direct messages addressed to you"
		}
	}

	if !r.cfg.ReadAuthRequired {
		return false, ""
	}
	if authed == "" {
		return true, "auth-required: authenticate to read from this relay"
	}
	if !r.whitelisted(authed) {
		return true, "restricted: not whitelisted — get access at " + r.cfg.PanelURL
	}
	return false, ""
}

func wantsPrivateKinds(filter nostr.Filter) bool {
	for _, kind := range filter.Kinds {
		if slices.Contains(privateKinds, kind) {
			return true
		}
	}
	return false
}

// addressedTo reports whether every event the filter can match is one the
// pubkey is party to: they wrote all of them, or all of them are p-tagged to
// them. Listing themselves alongside somebody else is not enough — that filter
// would return the other party's messages too.
func addressedTo(filter nostr.Filter, pubkey string) bool {
	return onlyValue(filter.Authors, pubkey) || onlyValue(filter.Tags["p"], pubkey)
}

func onlyValue(list []string, value string) bool {
	if len(list) == 0 {
		return false
	}
	for _, v := range list {
		if v != value {
			return false
		}
	}
	return true
}

// rejectConnection drops connections from IPs blocked through NIP-86.
func (r *Relay) rejectConnection(req *http.Request) bool {
	return r.store.ModHas(store.ModBlockedIP, relaycore.GetIPFromRequest(req))
}

// queryEvents serves stored events, hiding NIP-40 events whose expiration has
// passed. khatru's expiration manager only sweeps them off disk once an hour,
// and until it does the event store still returns them.
//
// Internal calls (the sweeper itself, deletion handling) see everything —
// otherwise the sweeper could never find the events it is supposed to delete.
func (r *Relay) queryEvents(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
	// NIP-43 invite requests are minted on demand rather than stored, so they
	// are answered here instead of coming out of the event table.
	if !relaycore.IsInternalCall(ctx) && slices.Contains(filter.Kinds, KindInviteRequest) {
		out := make(chan *nostr.Event, 1)
		for _, evt := range r.inviteEvents(ctx) {
			out <- evt
		}
		close(out)
		return out, nil
	}

	if !relaycore.IsInternalCall(ctx) && !r.applySearchExtensions(&filter) {
		// the NIP-50 extensions cannot match anything; say so immediately
		// instead of running a query that would ignore them
		empty := make(chan *nostr.Event)
		close(empty)
		return empty, nil
	}

	src, err := r.store.Events.QueryEvents(ctx, filter)
	if err != nil || relaycore.IsInternalCall(ctx) {
		return src, err
	}

	out := make(chan *nostr.Event)
	go func() {
		defer close(out)
		for evt := range src {
			if expired(evt) {
				continue
			}
			select {
			case out <- evt:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func expired(evt *nostr.Event) bool {
	expiresAt := nip40.GetExpiration(evt.Tags)
	return expiresAt != -1 && expiresAt <= nostr.Now()
}

// chargeUsage bills the stored bytes to the author's quota.
func (r *Relay) chargeUsage(ctx context.Context, evt *nostr.Event) {
	if nostr.IsEphemeralKind(evt.Kind) {
		return
	}
	// a vanish request is accepted from anyone, so there may be no account to
	// bill; carry it out instead
	if evt.Kind == KindRequestToVanish {
		r.vanish(ctx, evt)
		return
	}
	// zap receipts and badge awards land in the recipient's quota
	payer := r.billedPubkey(evt)
	if err := r.store.AddUsage(evt.ID, payer, eventSize(evt)); err != nil {
		r.Log.Printf("usage accounting failed for %s: %v", evt.ID, err)
	}
	if !nostr.IsRegularKind(evt.Kind) {
		// a replaceable event superseded an older one: drop its usage row
		if err := r.store.ReconcileUsage(payer); err != nil {
			r.Log.Printf("usage reconcile failed for %s: %v", payer, err)
		}
	}
}

// releaseUsage gives quota back when an event is deleted.
func (r *Relay) releaseUsage(ctx context.Context, evt *nostr.Event) error {
	if err := r.store.RemoveUsage(evt.ID); err != nil {
		r.Log.Printf("usage release failed for %s: %v", evt.ID, err)
	}
	return nil
}

// eventSize is what an event costs against a quota: the bytes of its wire JSON.
func eventSize(evt *nostr.Event) int64 {
	return int64(len(evt.String()))
}
