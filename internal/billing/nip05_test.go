package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"nostrel/internal/store"
)

const otherPubkey = "bb11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"

func seedDomain(t *testing.T, st *store.Store, price int64) {
	t.Helper()
	if err := st.SaveNip05Domain(store.Nip05Domain{
		Domain: "example.com", Enabled: true, PriceSats: price, PeriodDays: 365,
	}); err != nil {
		t.Fatalf("saving domain: %v", err)
	}
}

// Buying only a name must not drag in the relay admission fee, and must not
// leave an account row behind: that row would waive the fee the buyer still
// owes if they ever join the relay itself.
func TestNip05OnlyOrderSkipsAdmission(t *testing.T) {
	ctx := context.Background()
	s, st := newService(t)
	seedDomain(t, st, 500)

	sats, meta, kind, err := s.Quote(testPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "bob", Nip05Periods: 1,
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if sats != 500 {
		t.Errorf("price = %d, want 500 (no admission fee)", sats)
	}
	if kind != store.KindNip05 || meta.Admission {
		t.Errorf("kind = %q admission = %v, want nip05/false", kind, meta.Admission)
	}
	if meta.Nip05Days != 365 {
		t.Errorf("term = %d days, want 365", meta.Nip05Days)
	}

	p, err := s.CreateOrder(ctx, testPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "bob", Nip05Periods: 1,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := s.Settle(ctx, p.PaymentHash); err != nil {
		t.Fatalf("settle: %v", err)
	}

	pubkey, err := st.ResolveNip05("example.com", "bob")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pubkey != testPubkey {
		t.Errorf("name resolves to %q, want %q", pubkey, testPubkey)
	}
	if _, err := st.Account(testPubkey); !errors.Is(err, store.ErrNoAccount) {
		t.Error("buying a name created a relay account, which would waive the admission fee")
	}
}

func TestNip05PremiumPricing(t *testing.T) {
	s, st := newService(t)
	seedDomain(t, st, 100)

	settings, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Nip05PremiumTiers = "1:20,3:5"
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		periods int
		want    int64
	}{
		{"a", 1, 2000},   // 100 * 20
		{"abc", 1, 500},  // 100 * 5
		{"abcd", 1, 100}, // no tier matches
		{"abcd", 3, 300},
	} {
		sats, _, _, err := s.Quote(testPubkey, Order{
			Nip05Domain: "example.com", Nip05Name: tc.name, Nip05Periods: tc.periods,
		})
		if err != nil {
			t.Fatalf("quote %q: %v", tc.name, err)
		}
		if sats != tc.want {
			t.Errorf("%q x%d = %d sats, want %d", tc.name, tc.periods, sats, tc.want)
		}
	}
}

func TestNip05OrderRejections(t *testing.T) {
	ctx := context.Background()
	s, st := newService(t)
	seedDomain(t, st, 100)

	if _, _, _, err := s.Quote(testPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "NOT VALID", Nip05Periods: 1,
	}); !errors.Is(err, ErrBadName) {
		t.Errorf("malformed name: err = %v, want ErrBadName", err)
	}
	if _, _, _, err := s.Quote(testPubkey, Order{
		Nip05Domain: "nowhere.example", Nip05Name: "bob", Nip05Periods: 1,
	}); !errors.Is(err, store.ErrNoDomain) {
		t.Errorf("unknown domain: err = %v, want ErrNoDomain", err)
	}

	if err := st.ModAdd(store.ModBlockedName, "admin", "reserved"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Quote(testPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "admin", Nip05Periods: 1,
	}); !errors.Is(err, store.ErrNameBlocked) {
		t.Errorf("blocked name: err = %v, want ErrNameBlocked", err)
	}

	// an unpaid order still holds the name against everybody else
	if _, err := s.CreateOrder(ctx, testPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "bob", Nip05Periods: 1,
	}); err != nil {
		t.Fatalf("first order: %v", err)
	}
	if _, err := s.CreateOrder(ctx, otherPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "bob", Nip05Periods: 1,
	}); !errors.Is(err, store.ErrNameTaken) {
		t.Errorf("second buyer: err = %v, want ErrNameTaken", err)
	}
}

func TestNip05RenewalStacksOnRemainingTime(t *testing.T) {
	ctx := context.Background()
	s, st := newService(t)
	seedDomain(t, st, 100)

	existing := time.Now().Add(30 * 24 * time.Hour).Unix()
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "bob", Pubkey: testPubkey, ExpiresAt: existing,
	}); err != nil {
		t.Fatal(err)
	}

	p, err := s.CreateOrder(ctx, testPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "bob", Nip05Periods: 1,
	})
	if err != nil {
		t.Fatalf("renewal order: %v", err)
	}
	if _, err := s.Settle(ctx, p.PaymentHash); err != nil {
		t.Fatalf("settle: %v", err)
	}

	name, err := st.Nip05Name("example.com", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if want := existing + 365*86400; name.ExpiresAt != want {
		t.Errorf("expiry = %d, want %d (stacked on the remaining time)", name.ExpiresAt, want)
	}
	if name.ReservedUntil != 0 {
		t.Errorf("reservation survived settlement: %d", name.ReservedUntil)
	}
}

func TestGroupOrderFundsTheGroupNotTheBuyer(t *testing.T) {
	ctx := context.Background()
	s, st := newService(t)
	settings := store.DefaultSettings()

	p, err := s.CreateOrder(ctx, testPubkey, Order{GroupName: "team", Periods: 1, ExtraMB: 50})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if p.Kind != store.KindGroup {
		t.Errorf("kind = %q, want group", p.Kind)
	}
	// a group is not a relay account, so there is no admission fee to pay
	want := settings.SubscriptionSats + 50*settings.PricePerMBSats
	if p.Sats != want {
		t.Errorf("price = %d, want %d", p.Sats, want)
	}

	if _, err := s.Settle(ctx, p.PaymentHash); err != nil {
		t.Fatalf("settle: %v", err)
	}

	group, err := st.GroupOf(testPubkey)
	if err != nil {
		t.Fatalf("group of buyer: %v", err)
	}
	wantQuota := int64(settings.IncludedMB+50) * store.MB
	if group.QuotaBytes != wantQuota {
		t.Errorf("group quota = %d, want %d", group.QuotaBytes, wantQuota)
	}
	if group.ExpiresAt == 0 {
		t.Error("group subscription has no expiry")
	}
	if _, err := st.Account(testPubkey); !errors.Is(err, store.ErrNoAccount) {
		t.Error("funding a group created a personal account for the buyer")
	}

	// only the owner may top a group up
	if _, _, _, err := s.Quote(otherPubkey, Order{GroupID: group.ID, Periods: 1}); !errors.Is(err, ErrNotGroupOwner) {
		t.Errorf("stranger top-up: err = %v, want ErrNotGroupOwner", err)
	}
	if _, _, _, err := s.Quote(testPubkey, Order{GroupID: "nope", Periods: 1}); !errors.Is(err, ErrNoSuchGroup) {
		t.Errorf("unknown group: err = %v, want ErrNoSuchGroup", err)
	}
}

func TestNip05PermanentPurchase(t *testing.T) {
	ctx := context.Background()
	s, st := newService(t)

	// a domain that only sells terms refuses a permanent order
	seedDomain(t, st, 500)
	if _, _, _, err := s.Quote(testPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "bob", Nip05Permanent: true,
	}); !errors.Is(err, ErrNoPermanentDomain) {
		t.Fatalf("permanent on a term-only domain: err = %v, want ErrNoPermanentDomain", err)
	}

	if err := st.SaveNip05Domain(store.Nip05Domain{
		Domain: "example.com", Enabled: true, PriceSats: 500, PeriodDays: 365,
		PermanentPriceSats: 20_000,
	}); err != nil {
		t.Fatal(err)
	}

	sats, meta, _, err := s.Quote(testPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "bob", Nip05Permanent: true,
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if sats != 20_000 || !meta.Nip05Permanent || meta.Nip05Days != 0 {
		t.Errorf("quote = %d sats, meta %+v; want 20000 with a permanent, termless grant", sats, meta)
	}

	p, err := s.CreateOrder(ctx, testPubkey, Order{
		Nip05Domain: "example.com", Nip05Name: "bob", Nip05Permanent: true,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := s.Settle(ctx, p.PaymentHash); err != nil {
		t.Fatalf("settle: %v", err)
	}

	name, err := st.Nip05Name("example.com", "bob")
	if err != nil {
		t.Fatalf("reading the name: %v", err)
	}
	if !name.Permanent || name.ReservedUntil != 0 {
		t.Errorf("name = %+v, want permanent with the reservation cleared", name)
	}
	if !name.Live(time.Now().AddDate(10, 0, 0).Unix()) {
		t.Error("a permanent name stopped resolving")
	}
}
