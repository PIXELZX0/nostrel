package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

const (
	alice = "aa11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"
	bob   = "bb11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func seedDomain(t *testing.T, s *Store, domain string) {
	t.Helper()
	if err := s.SaveNip05Domain(Nip05Domain{
		Domain: domain, Enabled: true, PriceSats: 100, PeriodDays: 365,
	}); err != nil {
		t.Fatalf("saving domain: %v", err)
	}
}

// A reservation is what stops two buyers paying for the same name, so it has to
// hold until it is either settled or lapses.
func TestReservationBlocksASecondBuyer(t *testing.T) {
	s := newStore(t)
	seedDomain(t, s, "example.com")
	hour := time.Now().Add(time.Hour).Unix()

	if err := s.ReserveNip05("example.com", "bob", alice, hour); err != nil {
		t.Fatalf("first reservation: %v", err)
	}
	if err := s.ReserveNip05("example.com", "bob", bob, hour); !errors.Is(err, ErrNameTaken) {
		t.Errorf("second buyer got %v, want ErrNameTaken", err)
	}
	// the holder may re-reserve: that is a retried checkout, not a conflict
	if err := s.ReserveNip05("example.com", "bob", alice, hour); err != nil {
		t.Errorf("same buyer again: %v", err)
	}

	// nothing resolves until the invoice is paid
	if pubkey, err := s.ResolveNip05("example.com", "bob"); err != nil || pubkey != "" {
		t.Errorf("unpaid name resolved to %q (err %v), want empty", pubkey, err)
	}
}

func TestNameResolvesUntilItExpires(t *testing.T) {
	s := newStore(t)
	seedDomain(t, s, "example.com")

	if err := s.SaveNip05Name(Nip05Name{
		Domain: "example.com", Name: "bob", Pubkey: alice,
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("granting name: %v", err)
	}

	pubkey, err := s.ResolveNip05("example.com", "bob")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pubkey != alice {
		t.Errorf("resolved to %q, want %q", pubkey, alice)
	}
	// case is not significant in a NIP-05 lookup
	if pubkey, _ := s.ResolveNip05("EXAMPLE.com", "BOB"); pubkey != alice {
		t.Errorf("uppercase lookup resolved to %q, want %q", pubkey, alice)
	}

	// expired names disappear from nostr.json without anything being deleted
	if err := s.SaveNip05Name(Nip05Name{
		Domain: "example.com", Name: "bob", Pubkey: alice,
		ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("expiring name: %v", err)
	}
	if pubkey, _ := s.ResolveNip05("example.com", "bob"); pubkey != "" {
		t.Errorf("expired name resolved to %q, want empty", pubkey)
	}

	// and the name goes back on sale for somebody else
	if err := s.Nip05Available("example.com", "bob", bob); err != nil {
		t.Errorf("lapsed name should be available: %v", err)
	}
}

func TestBlockedNamesCannotBeBought(t *testing.T) {
	s := newStore(t)
	seedDomain(t, s, "example.com")
	if err := s.ModAdd(ModBlockedName, "admin", "reserved"); err != nil {
		t.Fatalf("blocking: %v", err)
	}

	if err := s.Nip05Available("example.com", "admin", alice); !errors.Is(err, ErrNameBlocked) {
		t.Errorf("availability = %v, want ErrNameBlocked", err)
	}
	if err := s.ReserveNip05("example.com", "admin", alice, time.Now().Add(time.Hour).Unix()); !errors.Is(err, ErrNameBlocked) {
		t.Errorf("reservation = %v, want ErrNameBlocked", err)
	}
}

func TestPurgeKeepsLiveAndReservedNames(t *testing.T) {
	s := newStore(t)
	seedDomain(t, s, "example.com")
	now := time.Now()

	if err := s.SaveNip05Name(Nip05Name{
		Domain: "example.com", Name: "live", Pubkey: alice,
		ExpiresAt: now.Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("live name: %v", err)
	}
	if err := s.SaveNip05Name(Nip05Name{
		Domain: "example.com", Name: "forever", Pubkey: alice, Permanent: true,
	}); err != nil {
		t.Fatalf("permanent name: %v", err)
	}
	if err := s.SaveNip05Name(Nip05Name{
		Domain: "example.com", Name: "lapsed", Pubkey: alice,
		ExpiresAt: now.Add(-time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("lapsed name: %v", err)
	}
	if err := s.ReserveNip05("example.com", "checkout", bob, now.Add(time.Hour).Unix()); err != nil {
		t.Fatalf("reservation: %v", err)
	}

	n, err := s.PurgeNip05Names()
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d rows, want 1 (only the lapsed name)", n)
	}
	for _, name := range []string{"live", "forever", "checkout"} {
		if _, err := s.Nip05Name("example.com", name); err != nil {
			t.Errorf("%s was purged: %v", name, err)
		}
	}
}

func TestDeletingADomainDropsItsNames(t *testing.T) {
	s := newStore(t)
	seedDomain(t, s, "example.com")
	if err := s.SaveNip05Name(Nip05Name{
		Domain: "example.com", Name: "bob", Pubkey: alice, Permanent: true,
	}); err != nil {
		t.Fatalf("granting name: %v", err)
	}

	if err := s.DeleteNip05Domain("example.com"); err != nil {
		t.Fatalf("deleting domain: %v", err)
	}
	if pubkey, _ := s.ResolveNip05("example.com", "bob"); pubkey != "" {
		t.Errorf("name survived its domain: resolved to %q", pubkey)
	}
}

func TestPremiumTiers(t *testing.T) {
	tiers, err := ParsePremiumTiers("1:20, 2:10,3:5")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	for _, tc := range []struct {
		name string
		want int
	}{
		{"a", 20},
		{"ab", 10},
		{"abc", 5},
		{"abcd", 1},
		{"averylongname", 1},
	} {
		if got := PremiumMultiplier(tiers, tc.name); got != tc.want {
			t.Errorf("multiplier(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}

	empty, err := ParsePremiumTiers("")
	if err != nil {
		t.Fatalf("empty spec: %v", err)
	}
	if got := PremiumMultiplier(empty, "a"); got != 1 {
		t.Errorf("no tiers should mean no premium, got %d", got)
	}

	for _, bad := range []string{"nonsense", "0:5", "2:0", "2:x"} {
		if _, err := ParsePremiumTiers(bad); err == nil {
			t.Errorf("ParsePremiumTiers(%q) accepted a broken spec", bad)
		}
	}
}
