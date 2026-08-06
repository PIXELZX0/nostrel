// Package relay assembles the khatru relay: storage, paid-whitelist policy and
// the NIP-11 document.
package relay

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip11"
	"nostrel/internal/relaycore"
	"nostrel/internal/relaycore/policies"

	"nostrel/internal/blobs"
	"nostrel/internal/config"
	"nostrel/internal/store"
)

const Version = "0.1.0"

// supportedNIPs is what this relay implements on the protocol side. Client-only
// NIPs (kinds it merely stores and serves) are deliberately absent — a relay
// does not "support" those, it just carries them.
//
//	01 core             04 encrypted dm     09 deletion
//	11 relay info       17 private dm       40 expiration
//	42 auth             45 count            50 search
//	56 reporting        59 gift wrap        62 request to vanish
//	70 protected events 77 negentropy sync  86 relay management
//	98 http auth
//
// 04, 17 and 59 are here because the relay enforces who may read those kinds,
// not merely because it stores them.
//
// The rest depend on configuration and are added in relayInformation, which
// runs per request against the live settings: 13 proof of work, 05 identifiers,
// 57 zap receipts, 58 badge awards, 96 http file storage.
var supportedNIPs = []int{1, 4, 9, 11, 17, 40, 42, 45, 50, 56, 59, 62, 70, 77, 86, 98}

type Relay struct {
	*relaycore.Relay
	cfg   *config.Config
	store *store.Store

	// media, set by EnableBlossom
	storage        *blobs.Storage
	blobIndex      *blobs.Index
	blobServiceURL string

	// NIP-43: the relay's own pubkey, and a nudge that the member list needs
	// rewriting. Buffered by one so a burst of signups collapses into one
	// republish.
	selfPubkey      string
	membershipDirty chan struct{}
	// okMessages holds the exact OK wording NIP-43 asks for, keyed by event id
	// and consumed once by overwriteOK.
	okMessages sync.Map
}

func New(cfg *config.Config, st *store.Store, logger *log.Logger) *Relay {
	kr := relaycore.NewRelay()
	if logger != nil {
		kr.Log = logger
	}
	r := &Relay{Relay: kr, cfg: cfg, store: st, membershipDirty: make(chan struct{}, 1)}

	// NIP-43 needs the relay to be able to sign as itself; without a key every
	// part of it stays off.
	if cfg.RelaySecretKey != "" {
		selfPubkey, err := nostr.GetPublicKey(cfg.RelaySecretKey)
		if err != nil {
			kr.Log.Printf("RELAY_SECRET_KEY is unusable, NIP-43 disabled: %v", err)
			cfg.RelaySecretKey = ""
		} else {
			r.selfPubkey = selfPubkey
		}
	}

	// The descriptive half of NIP-11 lives in the settings table and is filled
	// in per request by relayInformation, so an admin's edit is live at once.
	kr.Info.Software = "https://github.com/nostrel/nostrel"
	kr.Info.Version = Version
	kr.Info.SupportedNIPs = nil
	kr.Info.AddSupportedNIPs(supportedNIPs)
	if cfg.ServiceURL != "" {
		kr.ServiceURL = cfg.ServiceURL
	}

	// NIP-77: khatru builds the sync vector from QueryEvents, so this works with
	// any event store.
	kr.Negentropy = true

	policies.ApplySaneDefaults(kr)
	kr.RejectEvent = append(kr.RejectEvent,
		policies.ValidateKind,
		policies.PreventLargeTags(1024),
		policies.PreventTooManyIndexableTags(500, nil, nil),
	)
	kr.RejectFilter = append(kr.RejectFilter, policies.NoEmptyFilters)

	kr.StoreEvent = append(kr.StoreEvent, st.Events.SaveEvent)
	kr.QueryEvents = append(kr.QueryEvents, r.queryEvents)
	kr.CountEvents = append(kr.CountEvents, st.Events.CountEvents)
	kr.ReplaceEvent = append(kr.ReplaceEvent, st.Events.ReplaceEvent)
	kr.DeleteEvent = append(kr.DeleteEvent, st.Events.DeleteEvent, r.releaseUsage)

	// Registering this stops khatru turning an accepted ephemeral event into
	// "mute: no one was listening for this" when nobody is subscribed — a join
	// request is addressed to the relay, not to other clients.
	kr.OnEphemeralEvent = append(kr.OnEphemeralEvent, func(ctx context.Context, evt *nostr.Event) {})

	kr.RejectEvent = append(kr.RejectEvent, r.rejectEvent)
	kr.RejectFilter = append(kr.RejectFilter, r.rejectFilter)
	kr.RejectCountFilter = append(kr.RejectCountFilter, r.rejectFilter)
	kr.RejectConnection = append(kr.RejectConnection, r.rejectConnection)
	kr.OnEventSaved = append(kr.OnEventSaved, r.chargeUsage)

	kr.OverwriteRelayInformation = append(kr.OverwriteRelayInformation, r.relayInformation)
	kr.OverwriteOK = append(kr.OverwriteOK, r.overwriteOK)

	return r
}

// commaList turns a panel field into the string array NIP-11 expects, dropping
// blanks so an empty setting stays absent from the document.
func commaList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// IsAdmin reports whether a pubkey may use the admin APIs: either it is in
// ADMIN_PUBKEYS or it was granted admin through NIP-86.
func (r *Relay) IsAdmin(pubkey string) bool {
	return r.store.IsAdmin(pubkey, r.cfg.AdminPubkeys)
}

// readAuthRequired reports whether reads are restricted to whitelisted pubkeys.
// A settings read that fails is treated as "restricted": on a private relay
// that is the only safe answer, and the same broken database would fail the
// query itself a moment later anyway.
func (r *Relay) readAuthRequired() bool {
	settings, err := r.store.Settings()
	if err != nil {
		r.Log.Printf("settings read failed: %v", err)
		return true
	}
	return settings.ReadAuthRequired
}

// relayInformation renders NIP-11 from the live settings so price and policy
// changes show up immediately.
func (r *Relay) relayInformation(ctx context.Context, req *http.Request, info nip11.RelayInformationDocument) nip11.RelayInformationDocument {
	settings, err := r.store.Settings()
	if err != nil {
		r.Log.Printf("settings read failed: %v", err)
		return info
	}
	info.Name = settings.RelayName
	info.Description = settings.RelayDescription
	info.Icon = settings.RelayIcon
	info.Banner = settings.RelayBanner
	info.Contact = settings.RelayContact
	// No operator pubkey configured: the relay's own NIP-43 identity is the
	// closest thing to one, and it is at least verifiable.
	if info.PubKey = settings.RelayPubkey; info.PubKey == "" {
		info.PubKey = r.selfPubkey
	}
	info.RelayCountries = commaList(settings.RelayCountries)
	info.LanguageTags = commaList(settings.RelayLanguages)
	info.Tags = commaList(settings.RelayTopics)
	// A relay that charges money should say how long it keeps things; leaving
	// the field out means "indefinitely", which is also our default.
	if settings.RelayRetentionDays > 0 {
		info.Retention = []*nip11.RelayRetentionDocument{
			{Time: int64(settings.RelayRetentionDays) * 86400},
		}
	}
	// the configuration-dependent half of supported_nips
	//
	// Clients "MUST only request kind 28935 events from and send kind 28934
	// events to relays which include this NIP", so 43 must not appear before
	// the relay can sign as itself.
	if r.nip43Enabled() {
		info.AddSupportedNIP(43)
	}
	if settings.MinPoW > 0 {
		info.AddSupportedNIP(13)
	}
	if settings.AcceptZapReceipts {
		info.AddSupportedNIP(57)
	}
	if settings.AcceptBadgeAwards {
		info.AddSupportedNIP(58)
	}
	if r.storage != nil {
		info.AddSupportedNIP(96)
	}
	// NIP-05 is only true of this host once a domain is actually on sale
	if domains, err := r.store.ListNip05Domains(true); err != nil {
		r.Log.Printf("listing nip-05 domains: %v", err)
	} else if len(domains) > 0 {
		info.AddSupportedNIP(5)
	}

	info.PaymentsURL = r.cfg.PanelURL
	info.PostingPolicy = r.cfg.PanelURL
	if settings.RelayPostingPolicy != "" {
		info.PostingPolicy = settings.RelayPostingPolicy
	}
	info.Limitation = &nip11.RelayLimitationDocument{
		MaxMessageLength: int(r.MaxMessageSize),
		MaxSubscriptions: 20,
		MaxLimit:         r.store.Events.QueryLimit,
		DefaultLimit:     r.store.Events.QueryLimit,
		MaxEventTags:     500,
		MinPowDifficulty: settings.MinPoW,
		PaymentRequired:  true,
		RestrictedWrites: true,
		AuthRequired:     settings.ReadAuthRequired,
	}
	now := time.Now().Unix()
	if settings.CreatedAtMaxPast > 0 {
		info.Limitation.CreatedAtLowerLimit = now - int64(settings.CreatedAtMaxPast)
	}
	if settings.CreatedAtMaxFuture > 0 {
		info.Limitation.CreatedAtUpperLimit = now + int64(settings.CreatedAtMaxFuture)
	}

	fees := &nip11.RelayFeesDocument{}
	if settings.AdmissionSats > 0 {
		fees.Admission = append(fees.Admission, struct {
			Amount int    `json:"amount"`
			Unit   string `json:"unit"`
		}{Amount: int(settings.AdmissionSats) * 1000, Unit: "msats"})
	}
	if settings.SubscriptionSats > 0 {
		fees.Subscription = append(fees.Subscription, struct {
			Amount int    `json:"amount"`
			Unit   string `json:"unit"`
			Period int    `json:"period"`
		}{Amount: int(settings.SubscriptionSats) * 1000, Unit: "msats", Period: settings.PeriodDays * 86400})
	}
	info.Fees = fees

	return info
}
