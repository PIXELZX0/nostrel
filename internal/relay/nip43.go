package relay

import (
	"context"
	"errors"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/store"
)

// NIP-43 lets a standard client discover how to get in and ask for access,
// instead of having to find this relay's web panel. The relay publishes what
// roles exist and who is a member, and answers three requests: give me a code,
// let me in with this code, let me out.
//
// Everything here is off unless RELAY_SECRET_KEY is set — the relay has to be
// able to sign as itself before it can say anything about itself.

const (
	KindRoleDefinition = 33534
	KindMembershipList = 13534
	KindMemberAdded    = 8000
	KindMemberRemoved  = 8001
	KindJoinRequest    = 28934
	KindInviteRequest  = 28935
	KindLeaveRequest   = 28936
)

const (
	// requestDrift is how far a join or leave request's created_at may be from
	// now. The spec says "now, plus or minus a few minutes".
	requestDrift = 5 * time.Minute
	// autoInviteTTL is how long a code handed out over 28935 stays usable.
	// Short, because anyone may ask for one.
	autoInviteTTL = 10 * time.Minute
	// maxMembersPublished caps the membership list. The spec calls the list
	// non-authoritative, so truncating it is allowed.
	maxMembersPublished = 1000
	// membershipDebounce keeps a burst of signups from rewriting the list once
	// per signup.
	membershipDebounce = 30 * time.Second
)

// roles are the only two this relay actually has. Inventing more would be
// describing a permission model we do not implement.
var roles = []struct{ id, label, description, color, order string }{
	{"member", "member", "can write to this relay", "37", "1"},
	{"admin", "admin", "can manage this relay", "0", "0"},
}

func (r *Relay) nip43Enabled() bool { return r.cfg.RelaySecretKey != "" }

// sign produces an event authored by the relay itself, always carrying the
// NIP-70 "-" tag the spec requires.
func (r *Relay) sign(kind int, tags nostr.Tags, content string) (*nostr.Event, error) {
	if !r.nip43Enabled() {
		return nil, errors.New("relay has no signing key")
	}
	evt := &nostr.Event{
		Kind:      kind,
		CreatedAt: nostr.Now(),
		Tags:      append(nostr.Tags{{"-"}}, tags...),
		Content:   content,
	}
	if err := evt.Sign(r.cfg.RelaySecretKey); err != nil {
		return nil, err
	}
	return evt, nil
}

// publish saves one of the relay's own events and hands it to subscribers. It
// goes straight to the store: the paid-whitelist gate exists to decide whether
// somebody else may write here, and the relay is not somebody else.
func (r *Relay) publish(ctx context.Context, evt *nostr.Event) {
	if err := r.store.Events.SaveEvent(ctx, evt); err != nil {
		r.Log.Printf("nip-43: saving kind %d: %v", evt.Kind, err)
		return
	}
	r.BroadcastEvent(evt)
}

// PublishRoles writes the role definitions. Called once at startup.
func (r *Relay) PublishRoles(ctx context.Context) {
	if !r.nip43Enabled() {
		return
	}
	for _, role := range roles {
		evt, err := r.sign(KindRoleDefinition, nostr.Tags{
			{"d", role.id},
			{"label", role.label},
			{"description", role.description},
			{"color", role.color},
			{"order", role.order},
		}, "")
		if err != nil {
			r.Log.Printf("nip-43: signing role %s: %v", role.id, err)
			return
		}
		r.publish(ctx, evt)
	}
}

// PublishMembership rewrites the member list. Admins are tagged with the admin
// role; everyone else with an active account is a plain member.
func (r *Relay) PublishMembership(ctx context.Context) {
	if !r.nip43Enabled() {
		return
	}
	accounts, err := r.store.ListAccounts("", maxMembersPublished, 0)
	if err != nil {
		r.Log.Printf("nip-43: listing members: %v", err)
		return
	}

	now := time.Now().Unix()
	tags := nostr.Tags{}
	for _, account := range accounts {
		if account.Status != store.StatusActive {
			continue
		}
		if account.ExpiresAt != 0 && account.ExpiresAt < now {
			continue
		}
		tag := nostr.Tag{"member", account.Pubkey}
		if r.store.IsAdmin(account.Pubkey, r.cfg.AdminPubkeys) {
			tag = append(tag, "admin")
		}
		tags = append(tags, tag)
	}

	evt, err := r.sign(KindMembershipList, tags, "")
	if err != nil {
		r.Log.Printf("nip-43: signing membership list: %v", err)
		return
	}
	r.publish(ctx, evt)
}

// membershipChanged asks for the list to be republished soon. Signups arrive in
// bursts and the list is a replaceable event, so rewriting it per signup would
// be pure churn.
func (r *Relay) membershipChanged() {
	select {
	case r.membershipDirty <- struct{}{}:
	default: // already pending
	}
}

// WatchMembership republishes the member list at most once per debounce window,
// until ctx is cancelled.
func (r *Relay) WatchMembership(ctx context.Context) {
	if !r.nip43Enabled() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.membershipDirty:
			select {
			case <-ctx.Done():
				return
			case <-time.After(membershipDebounce):
				r.PublishMembership(ctx)
			}
		}
	}
}

func (r *Relay) announceMember(ctx context.Context, kind int, pubkey string) {
	evt, err := r.sign(kind, nostr.Tags{{"p", pubkey}}, "")
	if err != nil {
		r.Log.Printf("nip-43: signing kind %d: %v", kind, err)
		return
	}
	r.publish(ctx, evt)
	r.membershipChanged()
}

// --- client requests ---

// freshRequest rejects a join or leave whose timestamp drifted, which is what
// stops one being captured and replayed later.
func freshRequest(evt *nostr.Event) bool {
	age := time.Since(evt.CreatedAt.Time())
	return age < requestDrift && age > -requestDrift
}

// handleJoin processes a kind 28934. The bool result is khatru's reject flag,
// so "false" means the request was accepted.
func (r *Relay) handleJoin(ctx context.Context, evt *nostr.Event) (bool, string) {
	if !r.nip43Enabled() {
		return true, "restricted: this relay does not accept invite codes"
	}
	if !freshRequest(evt) {
		return true, "invalid: created_at must be within a few minutes of now"
	}
	code := evt.Tags.Find("claim").Value()
	if code == "" {
		return true, "invalid: a join request needs a claim tag"
	}

	// already in? say so without spending a use
	if account, err := r.store.Account(evt.PubKey); err == nil && account.Status == store.StatusActive {
		r.say(evt, "duplicate: you are already a member of this relay.")
		return false, ""
	}

	invite, err := r.store.ClaimInvite(code, evt.PubKey)
	switch {
	case errors.Is(err, store.ErrInviteUnknown):
		return true, "restricted: that is an invalid invite code."
	case errors.Is(err, store.ErrInviteExpired):
		return true, "restricted: that invite code is expired."
	case errors.Is(err, store.ErrInviteExhausted):
		return true, "restricted: that invite code has been used up."
	case errors.Is(err, store.ErrInviteAlreadyUsed):
		return true, "restricted: you already used that invite code."
	case err != nil:
		r.Log.Printf("nip-43: claiming %s: %v", code, err)
		return true, "error: could not check that invite code"
	}

	if err := r.grantInvite(evt.PubKey, invite); err != nil {
		r.Log.Printf("nip-43: granting %s to %s: %v", code, evt.PubKey, err)
		return true, "error: could not create your account"
	}

	r.announceMember(ctx, KindMemberAdded, evt.PubKey)
	r.say(evt, "info: welcome to "+r.cfg.ServiceURL+"!")
	return false, ""
}

// grantInvite turns a claimed code into relay access. An invite deliberately
// skips the admission fee — waiving it is what an invite is for.
func (r *Relay) grantInvite(pubkey string, invite *store.Invite) error {
	account, err := r.store.EnsureAccount(pubkey)
	if err != nil {
		return err
	}
	expiresAt := account.ExpiresAt
	if invite.PeriodDays > 0 {
		from := time.Now().Unix()
		if expiresAt > from {
			from = expiresAt
		}
		expiresAt = from + int64(invite.PeriodDays)*86400
	}
	quota := account.QuotaBytes + int64(invite.QuotaMB)*store.MB
	return r.store.UpdateAccount(pubkey, store.StatusActive, expiresAt, quota, "invite:"+invite.Code)
}

// handleLeave processes a kind 28936. Access is revoked, but nothing is
// deleted: a paying customer's data is not ours to throw away on a request that
// only asks for access to stop. NIP-62 is the one that erases things.
func (r *Relay) handleLeave(ctx context.Context, evt *nostr.Event) (bool, string) {
	if !r.nip43Enabled() {
		return true, "restricted: this relay does not manage membership this way"
	}
	if !freshRequest(evt) {
		return true, "invalid: created_at must be within a few minutes of now"
	}

	account, err := r.store.Account(evt.PubKey)
	if errors.Is(err, store.ErrNoAccount) {
		return true, "restricted: you are not a member of this relay"
	}
	if err != nil {
		return true, "error: could not read your account"
	}
	if err := r.store.UpdateAccount(evt.PubKey, store.StatusBanned,
		account.ExpiresAt, account.QuotaBytes, account.Note); err != nil {
		r.Log.Printf("nip-43: revoking %s: %v", evt.PubKey, err)
		return true, "error: could not revoke your access"
	}

	r.announceMember(ctx, KindMemberRemoved, evt.PubKey)
	r.say(evt, "info: your access has been revoked. your events were left in place.")
	return false, ""
}

// inviteEvents answers a subscription for kind 28935 by minting a code on the
// spot. Nothing is stored: the code lives in the invites table, the event that
// carries it does not.
func (r *Relay) inviteEvents(ctx context.Context) []*nostr.Event {
	if !r.nip43Enabled() {
		return nil
	}
	settings, err := r.store.Settings()
	if err != nil || !settings.AutoInvite {
		return nil
	}

	invite, err := r.store.CreateInvite(store.Invite{
		Note:       "auto",
		PeriodDays: settings.AutoInvitePeriodDays,
		QuotaMB:    settings.AutoInviteQuotaMB,
		MaxUses:    1,
		ExpiresAt:  time.Now().Add(autoInviteTTL).Unix(),
	})
	if err != nil {
		r.Log.Printf("nip-43: minting an invite: %v", err)
		return nil
	}

	evt, err := r.sign(KindInviteRequest, nostr.Tags{{"claim", invite.Code}}, "")
	if err != nil {
		r.Log.Printf("nip-43: signing an invite: %v", err)
		return nil
	}
	return []*nostr.Event{evt}
}

// say records the OK message this event should be answered with. khatru decides
// the reason for an accepted ephemeral event on its own ("broadcasted to N
// listeners"), so the wording NIP-43 specifies is handed over through the
// OverwriteOK hook the vendored copy adds — see third_party/khatru/FORK.md.
func (r *Relay) say(evt *nostr.Event, message string) {
	r.okMessages.Store(evt.ID, message)
}

// overwriteOK is that hook. It only speaks for events say() left a message for,
// and each message is used once.
func (r *Relay) overwriteOK(ctx context.Context, evt *nostr.Event, ok bool, reason string) (bool, string) {
	if message, found := r.okMessages.LoadAndDelete(evt.ID); found {
		return ok, message.(string)
	}
	return ok, reason
}
