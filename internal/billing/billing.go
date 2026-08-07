// Package billing turns a price list plus a settled invoice into relay access.
package billing

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"nostrel/internal/payments"
	"nostrel/internal/store"
)

var (
	ErrNothingOrdered = errors.New("order is empty")
	ErrPlanRequired   = errors.New("a new account must buy a subscription period or lifetime storage")
	ErrTooLarge       = errors.New("order is too large")
	ErrNoSuchGroup    = errors.New("no such group")
	ErrNotGroupOwner  = errors.New("only the group owner can top it up")
	ErrBadName        = errors.New("a NIP-05 name uses a-z, 0-9, '-', '_' and '.' only")

	ErrNoSubscription    = errors.New("this relay does not sell subscriptions")
	ErrNoLifetime        = errors.New("this relay does not sell lifetime storage")
	ErrMixedPlans        = errors.New("subscription periods and lifetime storage cannot be bought in one order")
	ErrAlreadyPermanent  = errors.New("this account is on the lifetime plan; there is nothing to renew")
	ErrNoPermanentDomain = errors.New("this domain does not sell permanent names")
)

// maxOrder caps a single order so a typo (or a bot) can't ask for an invoice of
// a billion sats.
const maxPeriods, maxExtraMB = 24, 100_000

// nameRE is the NIP-05 local part. The spec allows a-z0-9-_. and nothing else;
// names are lowercased before they get here.
var nameRE = regexp.MustCompile(`^[a-z0-9_.-]{1,30}$`)

// reservationWindow is how long a name is held for an unpaid invoice: exactly
// as long as the invoice itself stays payable, so the two lapse together.
const reservationWindow = InvoiceTTL

type Service struct {
	store     *store.Store
	providers *payments.Resolver
	log       *log.Logger
}

func New(st *store.Store, providers *payments.Resolver, logger *log.Logger) *Service {
	return &Service{store: st, providers: providers, log: logger}
}

// Provider is the name of the configured lightning backend. It is read fresh
// every time: an admin can switch backends from the panel while the relay runs.
func (s *Service) Provider() string { return s.providers.Name() }

// Order is what a user asked to buy. It is one of three things: relay access
// for the buyer, a top-up for a shared storage group, or a NIP-05 name.
//
// Access comes in two shapes, and an order buys one or the other, never both:
// Periods rents it by the period, LifetimeMB buys the storage outright and the
// account stops expiring.
type Order struct {
	Periods int `json:"periods"`  // subscription periods
	ExtraMB int `json:"extra_mb"` // storage on top of what the periods include

	// LifetimeMB is storage bought once and kept forever.
	LifetimeMB int `json:"lifetime_mb"`

	// GroupID or GroupName make the periods and megabytes go to a shared group
	// instead of the buyer's own account. GroupName creates a new group.
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`

	// A NIP-05 identifier, priced per domain and independent of the above.
	// Nip05Permanent buys the name outright instead of for Nip05Periods terms.
	Nip05Domain    string `json:"nip05_domain"`
	Nip05Name      string `json:"nip05_name"`
	Nip05Periods   int    `json:"nip05_periods"`
	Nip05Permanent bool   `json:"nip05_permanent"`
}

func (o Order) forGroup() bool { return o.GroupID != "" || o.GroupName != "" }
func (o Order) forNip05() bool { return o.Nip05Name != "" }

// forAccess reports whether the order buys relay storage for somebody, as
// opposed to being a bare NIP-05 purchase.
func (o Order) forAccess() bool { return o.Periods > 0 || o.ExtraMB > 0 || o.LifetimeMB > 0 }

// Quote prices an order for a pubkey, including the one-off admission fee when
// the pubkey is buying relay access and has no account yet.
func (s *Service) Quote(pubkey string, o Order) (sats int64, meta store.PaymentMeta, kind string, err error) {
	if o.Periods < 0 || o.ExtraMB < 0 || o.LifetimeMB < 0 || o.Nip05Periods < 0 {
		return 0, meta, "", ErrNothingOrdered
	}
	if o.Periods > maxPeriods || o.ExtraMB > maxExtraMB ||
		o.LifetimeMB > maxExtraMB || o.Nip05Periods > maxPeriods {
		return 0, meta, "", ErrTooLarge
	}
	// the two plans are alternatives, not a menu to combine: an order that
	// mixed them would leave it ambiguous whether the pot still expires
	if o.LifetimeMB > 0 && (o.Periods > 0 || o.ExtraMB > 0) {
		return 0, meta, "", ErrMixedPlans
	}

	st, err := s.store.Settings()
	if err != nil {
		return 0, meta, "", err
	}
	if o.Periods > 0 && !st.SubscriptionEnabled {
		return 0, meta, "", ErrNoSubscription
	}
	if o.LifetimeMB > 0 && (!st.LifetimeEnabled || st.LifetimePricePerMBSats <= 0) {
		return 0, meta, "", ErrNoLifetime
	}

	if o.forNip05() {
		nameSats, days, err := s.quoteNip05(st, pubkey, o)
		if err != nil {
			return 0, meta, "", err
		}
		sats += nameSats
		meta.Nip05Domain = strings.ToLower(o.Nip05Domain)
		meta.Nip05Name = strings.ToLower(o.Nip05Name)
		meta.Nip05Days = days
		meta.Nip05Permanent = o.Nip05Permanent
		kind = store.KindNip05
	}

	if o.forGroup() {
		groupID, err := s.quoteGroup(pubkey, o)
		if err != nil {
			return 0, meta, "", err
		}
		meta.GroupID = groupID
		if kind == "" {
			kind = store.KindGroup
		}
	} else if o.forAccess() {
		// only relay access for the buyer themselves carries the admission fee
		acct, accErr := s.store.Account(pubkey)
		isNew := errors.Is(accErr, store.ErrNoAccount)
		if accErr != nil && !isNew {
			return 0, meta, "", accErr
		}
		if isNew {
			// storage alone buys nothing without a plan behind it
			if o.Periods < 1 && o.LifetimeMB < 1 {
				return 0, meta, "", ErrPlanRequired
			}
			sats += st.AdmissionSats
			meta.Admission = true
			kind = store.KindAdmission
		} else if acct.Permanent && (o.Periods > 0 || o.ExtraMB > 0) {
			// a lifetime account has no term to extend, and its storage is
			// topped up at the lifetime price
			return 0, meta, "", ErrAlreadyPermanent
		}
	}

	if o.Periods > 0 {
		sats += st.SubscriptionSats * int64(o.Periods)
		meta.PeriodDays = st.PeriodDays * o.Periods
		meta.MB += st.IncludedMB * o.Periods
		if kind == "" {
			kind = store.KindSubscription
		}
	}
	if o.ExtraMB > 0 {
		sats += st.PricePerMBSats * int64(o.ExtraMB)
		meta.MB += o.ExtraMB
		if kind == "" {
			kind = store.KindStorage
		}
	}
	if o.LifetimeMB > 0 {
		sats += st.LifetimePricePerMBSats * int64(o.LifetimeMB)
		meta.MB += o.LifetimeMB
		meta.Permanent = true
		if kind == "" {
			kind = store.KindLifetime
		}
	}

	if sats <= 0 {
		return 0, meta, "", ErrNothingOrdered
	}
	return sats, meta, kind, nil
}

// quoteNip05 prices a name under one of the relay's domains and returns the
// term it buys, refusing names that are malformed, blocked or somebody else's.
func (s *Service) quoteNip05(st store.Settings, pubkey string, o Order) (sats int64, days int, err error) {
	name := strings.ToLower(strings.TrimSpace(o.Nip05Name))
	if !nameRE.MatchString(name) {
		return 0, 0, ErrBadName
	}
	if !o.Nip05Permanent && o.Nip05Periods < 1 {
		return 0, 0, ErrNothingOrdered
	}

	domain, err := s.store.Nip05Domain(o.Nip05Domain)
	if err != nil {
		return 0, 0, err
	}
	if !domain.Enabled {
		return 0, 0, store.ErrNoDomain
	}
	if o.Nip05Permanent && domain.PermanentPriceSats <= 0 {
		return 0, 0, ErrNoPermanentDomain
	}
	if err := s.store.Nip05Available(domain.Domain, name, pubkey); err != nil {
		return 0, 0, err
	}

	tiers, err := store.ParsePremiumTiers(st.Nip05PremiumTiers)
	if err != nil {
		return 0, 0, err
	}
	multiplier := store.PremiumMultiplier(tiers, name)
	if o.Nip05Permanent {
		// bought outright: no term, so no days to grant
		return domain.PermanentPriceSats * int64(multiplier), 0, nil
	}
	return domain.PriceSats * int64(o.Nip05Periods) * int64(multiplier),
		domain.PeriodDays * o.Nip05Periods, nil
}

// quoteGroup resolves which group an order tops up, creating it when the buyer
// asked for a new one. Only the owner may pay into an existing group, so
// nobody can extend a stranger's subscription and claim it later.
func (s *Service) quoteGroup(pubkey string, o Order) (string, error) {
	if !o.forAccess() {
		return "", ErrNothingOrdered
	}
	if o.GroupID == "" {
		return "", nil // a new group; the id is minted in CreateOrder
	}

	group, err := s.store.Group(o.GroupID)
	if errors.Is(err, store.ErrNoGroup) {
		return "", ErrNoSuchGroup
	}
	if err != nil {
		return "", err
	}
	if group.Owner != pubkey {
		return "", ErrNotGroupOwner
	}
	if group.Permanent && (o.Periods > 0 || o.ExtraMB > 0) {
		return "", ErrAlreadyPermanent
	}
	return group.ID, nil
}

// CreateOrder prices the order, asks the provider for an invoice and records it
// as pending.
func (s *Service) CreateOrder(ctx context.Context, pubkey string, o Order) (*store.Payment, error) {
	sats, meta, kind, err := s.Quote(pubkey, o)
	if err != nil {
		return nil, err
	}

	provider, err := s.providers.Provider()
	if err != nil {
		return nil, fmt.Errorf("payment backend: %w", err)
	}

	// Hold the name before asking for an invoice: without this, two people can
	// both pay for the same name and only one of them can be given it.
	if meta.Nip05Name != "" {
		until := time.Now().Add(reservationWindow).Unix()
		if err := s.store.ReserveNip05(meta.Nip05Domain, meta.Nip05Name, pubkey, until); err != nil {
			return nil, err
		}
	}
	if o.forGroup() && meta.GroupID == "" {
		group, err := s.store.CreateGroup(randomID(), strings.TrimSpace(o.GroupName), pubkey)
		if err != nil {
			s.releaseNip05(meta, pubkey)
			return nil, err
		}
		meta.GroupID = group.ID
	}

	memo := fmt.Sprintf("%s: %s… (%d days, %d MB)", kind, pubkey[:8], meta.PeriodDays, meta.MB)
	inv, err := provider.CreateInvoice(ctx, sats, memo)
	if err != nil {
		s.releaseNip05(meta, pubkey)
		return nil, fmt.Errorf("creating invoice: %w", err)
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	p := &store.Payment{
		ID:          randomID(),
		Pubkey:      pubkey,
		Kind:        kind,
		Sats:        sats,
		Provider:    provider.Name(),
		PaymentHash: inv.PaymentHash,
		Bolt11:      inv.Bolt11,
		Status:      store.PayPending,
		Meta:        string(metaJSON),
		CreatedAt:   time.Now().Unix(),
	}
	if err := s.store.CreatePayment(p); err != nil {
		s.releaseNip05(meta, pubkey)
		return nil, err
	}
	return p, nil
}

// releaseNip05 gives an unpaid reservation back when the order could not be
// placed after all, so a failed checkout doesn't park the name for an hour.
func (s *Service) releaseNip05(meta store.PaymentMeta, pubkey string) {
	if meta.Nip05Name == "" {
		return
	}
	if err := s.store.ReleaseNip05(meta.Nip05Domain, meta.Nip05Name, pubkey); err != nil {
		s.log.Printf("releasing reservation %s@%s: %v", meta.Nip05Name, meta.Nip05Domain, err)
	}
}

// Settle asks the lightning backend whether the invoice was really paid and, if
// so, grants the entitlements. Safe to call repeatedly and from several places
// at once (webhook, poller, panel polling) — only the first call that flips the
// row from pending to paid grants anything.
func (s *Service) Settle(ctx context.Context, paymentHash string) (*store.Payment, error) {
	p, err := s.store.PaymentByHash(paymentHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	if p.Status == store.PayPaid {
		return p, nil
	}

	provider, err := s.providers.Provider()
	if err != nil {
		return p, fmt.Errorf("payment backend: %w", err)
	}
	paid, err := provider.IsPaid(ctx, paymentHash)
	if err != nil {
		return p, err
	}
	if !paid {
		return p, nil
	}

	if err := s.grant(p); err != nil {
		return p, err
	}
	p.Status = store.PayPaid
	p.PaidAt = time.Now().Unix()
	s.log.Printf("payment settled: %s %d sats for %s", p.Kind, p.Sats, p.Pubkey)
	return p, nil
}

// grant applies a paid invoice in one transaction. The status flip and the
// entitlement update share the transaction, so a payment can never be credited
// twice.
func (s *Service) grant(p *store.Payment) error {
	var meta store.PaymentMeta
	if err := json.Unmarshal([]byte(p.Meta), &meta); err != nil {
		return fmt.Errorf("payment %s: bad meta: %w", p.PaymentHash, err)
	}

	tx, err := s.store.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	res, err := tx.Exec(
		`UPDATE payments SET status = 'paid', paid_at = ? WHERE payment_hash = ? AND status != 'paid'`,
		now, p.PaymentHash)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// somebody else already settled it
		return tx.Rollback()
	}

	if meta.Nip05Name != "" {
		// the name was reserved for this buyer at order time, so the update can
		// only miss if an admin took the name away in the meantime
		set := `expires_at = max(expires_at, ?) + ?, reserved_until = 0`
		args := []any{now, int64(meta.Nip05Days) * 86400}
		if meta.Nip05Permanent {
			set, args = `permanent = 1, reserved_until = 0`, nil
		}
		args = append(args, meta.Nip05Domain, meta.Nip05Name, p.Pubkey)
		res, err := tx.Exec(
			`UPDATE nip05_names SET `+set+` WHERE domain = ? AND name = ? AND pubkey = ?`, args...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			s.log.Printf("payment %s: %s@%s is no longer reserved for %s — name not granted",
				p.PaymentHash, meta.Nip05Name, meta.Nip05Domain, p.Pubkey)
		}
	}

	// Periods and megabytes go to the group when the order was for one, and to
	// the buyer's own account otherwise. A pure NIP-05 purchase touches neither,
	// and must not create an account row: that would silently waive the
	// admission fee the buyer still owes if they ever join the relay.
	target, id := "accounts", p.Pubkey
	if meta.GroupID != "" {
		target, id = "groups", meta.GroupID
	} else if meta.Admission || meta.PeriodDays > 0 || meta.MB > 0 || meta.Permanent {
		if _, err := tx.Exec(
			`INSERT INTO accounts (pubkey, status, created_at) VALUES (?, 'active', ?)
			 ON CONFLICT(pubkey) DO NOTHING`, p.Pubkey, now); err != nil {
			return err
		}
	}
	key := "pubkey"
	if target == "groups" {
		key = "id"
	}

	if meta.PeriodDays > 0 {
		// extend from whichever is later: now, or an unexpired subscription.
		// A pot already on the lifetime plan is left alone: it has no term, and
		// writing one would take away what was bought outright.
		if _, err := tx.Exec(
			`UPDATE `+target+` SET expires_at = max(expires_at, ?) + ?
			 WHERE `+key+` = ? AND permanent = 0`,
			now, int64(meta.PeriodDays)*86400, id); err != nil {
			return err
		}
	}
	if meta.MB > 0 {
		if _, err := tx.Exec(
			`UPDATE `+target+` SET quota_bytes = quota_bytes + ? WHERE `+key+` = ?`,
			int64(meta.MB)*store.MB, id); err != nil {
			return err
		}
	}
	if meta.Permanent {
		// the lifetime plan: the megabytes above are kept, and the term the pot
		// may have had before is cleared rather than left to look expired
		if _, err := tx.Exec(
			`UPDATE `+target+` SET permanent = 1, expires_at = 0 WHERE `+key+` = ?`,
			id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func randomID() string {
	buf := make([]byte, 16)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}
