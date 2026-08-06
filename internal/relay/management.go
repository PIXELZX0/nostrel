package relay

import (
	"context"
	"errors"
	"net"
	"slices"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip86"
	"nostrel/internal/relaycore"

	"nostrel/internal/store"
)

// moderationQueueSize caps how many reports the moderation queue returns.
const moderationQueueSize = 100

// EnableManagementAPI implements the full NIP-86 method set so standard
// relay-management clients can run this relay. Only admins may call it; khatru
// has already verified the NIP-98 signature by the time RejectAPICall runs.
func (r *Relay) EnableManagementAPI() {
	r.ManagementAPI.RejectAPICall = append(r.ManagementAPI.RejectAPICall,
		func(ctx context.Context, mp nip86.MethodParams) (bool, string) {
			if !r.IsAdmin(relaycore.GetAuthed(ctx)) {
				return true, "you are not an admin of this relay"
			}
			return false, ""
		})

	m := &r.ManagementAPI

	// pubkeys
	m.AllowPubKey = r.allowPubKey
	m.BanPubKey = r.banPubKey
	m.ListAllowedPubKeys = r.listPubKeys(store.StatusActive)
	m.ListBannedPubKeys = r.listPubKeys(store.StatusBanned)

	// events
	m.BanEvent = r.banEvent
	m.AllowEvent = r.allowEvent
	m.ListBannedEvents = r.listModeration(store.ModBannedEvent)
	m.ListAllowedEvents = r.listModeration(store.ModAllowedEvent)
	m.ListEventsNeedingModeration = r.listReportedEvents

	// kinds
	m.AllowKind = r.changeKindPolicy(true)
	m.DisallowKind = r.changeKindPolicy(false)
	m.ListAllowedKinds = func(ctx context.Context) ([]int, error) { return r.kindList(true) }
	m.ListDisAllowedKinds = func(ctx context.Context) ([]int, error) { return r.kindList(false) }

	// ips
	m.BlockIP = r.blockIP
	m.UnblockIP = r.unblockIP
	m.ListBlockedIPs = r.listBlockedIPs

	// relay metadata
	m.ChangeRelayName = r.changeSetting(func(s *store.Settings, v string) { s.RelayName = v })
	m.ChangeRelayDescription = r.changeSetting(func(s *store.Settings, v string) { s.RelayDescription = v })
	m.ChangeRelayIcon = r.changeSetting(func(s *store.Settings, v string) { s.RelayIcon = v })

	// admins
	m.GrantAdmin = r.grantAdmin
	m.RevokeAdmin = r.revokeAdmin

	m.Stats = r.stats
}

// --- pubkeys ---

// allowPubKey comps an account one subscription period at the current price
// list — the admin equivalent of a paid signup.
func (r *Relay) allowPubKey(ctx context.Context, pubkey string, reason string) error {
	settings, err := r.store.Settings()
	if err != nil {
		return err
	}
	acct, err := r.store.EnsureAccount(pubkey)
	if err != nil {
		return err
	}

	expires := acct.ExpiresAt
	if expires < time.Now().Unix() {
		expires = time.Now().Unix()
	}
	expires += int64(settings.PeriodDays) * 86400

	note := reason
	if note == "" {
		note = "granted via nip-86"
	}
	return r.store.UpdateAccount(pubkey, store.StatusActive, expires,
		acct.QuotaBytes+int64(settings.IncludedMB)*store.MB, note)
}

func (r *Relay) banPubKey(ctx context.Context, pubkey string, reason string) error {
	acct, err := r.store.EnsureAccount(pubkey)
	if err != nil {
		return err
	}
	if reason == "" {
		reason = "banned via nip-86"
	}
	return r.store.UpdateAccount(pubkey, store.StatusBanned, acct.ExpiresAt, acct.QuotaBytes, reason)
}

func (r *Relay) listPubKeys(status string) func(context.Context) ([]nip86.PubKeyReason, error) {
	return func(ctx context.Context) ([]nip86.PubKeyReason, error) {
		accounts, err := r.store.ListAccounts("", 500, 0)
		if err != nil {
			return nil, err
		}
		out := make([]nip86.PubKeyReason, 0, len(accounts))
		for _, a := range accounts {
			if a.Status != status {
				continue
			}
			out = append(out, nip86.PubKeyReason{PubKey: a.Pubkey, Reason: a.Note})
		}
		return out, nil
	}
}

// --- events ---

// banEvent both blocks the id from coming back and deletes the copy we hold.
func (r *Relay) banEvent(ctx context.Context, id string, reason string) error {
	if err := r.store.ModAdd(store.ModBannedEvent, id, reason); err != nil {
		return err
	}
	if err := r.store.ModRemove(store.ModAllowedEvent, id); err != nil {
		return err
	}

	ch, err := r.store.Events.QueryEvents(ctx, nostr.Filter{IDs: []string{id}})
	if err != nil {
		return err
	}
	for evt := range ch {
		for _, del := range r.DeleteEvent {
			if err := del(ctx, evt); err != nil {
				return err
			}
		}
	}
	return nil
}

// allowEvent lifts a ban and marks the event as reviewed, taking it out of the
// moderation queue.
func (r *Relay) allowEvent(ctx context.Context, id string, reason string) error {
	if err := r.store.ModRemove(store.ModBannedEvent, id); err != nil {
		return err
	}
	return r.store.ModAdd(store.ModAllowedEvent, id, reason)
}

func (r *Relay) listModeration(typ string) func(context.Context) ([]nip86.IDReason, error) {
	return func(ctx context.Context) ([]nip86.IDReason, error) {
		entries, err := r.store.ModList(typ)
		if err != nil {
			return nil, err
		}
		out := make([]nip86.IDReason, 0, len(entries))
		for _, e := range entries {
			out = append(out, nip86.IDReason{ID: e.Value, Reason: e.Reason})
		}
		return out, nil
	}
}

// listReportedEvents turns NIP-56 reports into a moderation queue: every event
// somebody reported that an admin has not banned or cleared yet.
func (r *Relay) listReportedEvents(ctx context.Context) ([]nip86.IDReason, error) {
	ch, err := r.store.Events.QueryEvents(ctx, nostr.Filter{
		Kinds: []int{nostr.KindReporting},
		Limit: moderationQueueSize * 5,
	})
	if err != nil {
		return nil, err
	}

	out := make([]nip86.IDReason, 0, moderationQueueSize)
	seen := map[string]bool{}
	for report := range ch {
		for _, tag := range report.Tags {
			if len(tag) < 2 || tag[0] != "e" {
				continue
			}
			target := tag[1]
			if len(target) != 64 { // reports can carry junk; ignore it
				continue
			}
			if seen[target] || r.store.ModHas(store.ModBannedEvent, target) ||
				r.store.ModHas(store.ModAllowedEvent, target) {
				continue
			}
			seen[target] = true

			reason := report.Content
			if len(tag) >= 3 && tag[2] != "" {
				reason = tag[2] + ": " + reason
			}
			out = append(out, nip86.IDReason{ID: target, Reason: reason})
			if len(out) >= moderationQueueSize {
				return out, nil
			}
		}
	}
	return out, nil
}

// --- kinds ---

func (r *Relay) changeKindPolicy(allow bool) func(context.Context, int) error {
	return func(ctx context.Context, kind int) error {
		settings, err := r.store.Settings()
		if err != nil {
			return err
		}
		if allow {
			settings.DisallowedKinds = remove(settings.DisallowedKinds, kind)
			if len(settings.AllowedKinds) > 0 && !slices.Contains(settings.AllowedKinds, kind) {
				settings.AllowedKinds = append(settings.AllowedKinds, kind)
			}
		} else {
			settings.AllowedKinds = remove(settings.AllowedKinds, kind)
			if !slices.Contains(settings.DisallowedKinds, kind) {
				settings.DisallowedKinds = append(settings.DisallowedKinds, kind)
			}
		}
		return r.store.SaveSettings(settings)
	}
}

func (r *Relay) kindList(allowed bool) ([]int, error) {
	settings, err := r.store.Settings()
	if err != nil {
		return nil, err
	}
	if allowed {
		return settings.AllowedKinds, nil
	}
	return settings.DisallowedKinds, nil
}

// --- ips ---

func (r *Relay) blockIP(ctx context.Context, ip net.IP, reason string) error {
	return r.store.ModAdd(store.ModBlockedIP, ip.String(), reason)
}

func (r *Relay) unblockIP(ctx context.Context, ip net.IP, reason string) error {
	return r.store.ModRemove(store.ModBlockedIP, ip.String())
}

func (r *Relay) listBlockedIPs(ctx context.Context) ([]nip86.IPReason, error) {
	entries, err := r.store.ModList(store.ModBlockedIP)
	if err != nil {
		return nil, err
	}
	out := make([]nip86.IPReason, 0, len(entries))
	for _, e := range entries {
		out = append(out, nip86.IPReason{IP: e.Value, Reason: e.Reason})
	}
	return out, nil
}

// --- admins & metadata ---

// grantAdmin adds a runtime admin. The methods argument is ignored: this relay
// has one admin level, not per-method grants.
func (r *Relay) grantAdmin(ctx context.Context, pubkey string, methods []string) error {
	settings, err := r.store.Settings()
	if err != nil {
		return err
	}
	if !slices.Contains(settings.ExtraAdmins, pubkey) {
		settings.ExtraAdmins = append(settings.ExtraAdmins, pubkey)
	}
	return r.store.SaveSettings(settings)
}

// revokeAdmin can only take back grants made here; ADMIN_PUBKEYS is owned by
// whoever controls the environment.
func (r *Relay) revokeAdmin(ctx context.Context, pubkey string, methods []string) error {
	if slices.Contains(r.cfg.AdminPubkeys, pubkey) {
		return errors.New("this admin comes from ADMIN_PUBKEYS and must be removed there")
	}
	settings, err := r.store.Settings()
	if err != nil {
		return err
	}
	settings.ExtraAdmins = remove(settings.ExtraAdmins, pubkey)
	return r.store.SaveSettings(settings)
}

func (r *Relay) changeSetting(apply func(*store.Settings, string)) func(context.Context, string) error {
	return func(ctx context.Context, value string) error {
		if value == "" {
			return errors.New("value must not be empty")
		}
		settings, err := r.store.Settings()
		if err != nil {
			return err
		}
		apply(&settings, value)
		return r.store.SaveSettings(settings)
	}
}

func (r *Relay) stats(ctx context.Context) (nip86.Response, error) {
	st, err := r.store.Stats()
	if err != nil {
		return nip86.Response{}, err
	}
	return nip86.Response{Result: map[string]any{
		"accounts":        st.Accounts,
		"active_accounts": st.ActiveAccounts,
		"events":          st.Events,
		"stored_bytes":    st.StoredBytes,
		"paid_payments":   st.PaidPayments,
		"sats_collected":  st.SatsCollected,
	}}, nil
}

func remove[T comparable](list []T, value T) []T {
	return slices.DeleteFunc(list, func(v T) bool { return v == value })
}
