package billing

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"nostrel/internal/payments"
	"nostrel/internal/store"
)

func newService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(st.Close)
	return New(st, payments.NewResolver(st, ""), log.New(io.Discard, "", 0)), st
}

const testPubkey = "aa11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"

func TestQuotePricesAdmissionOnlyForNewAccounts(t *testing.T) {
	s, st := newService(t)
	settings := store.DefaultSettings() // 1000 admission, 2000/period, 100MB included, 10/MB

	sats, meta, kind, err := s.Quote(testPubkey, Order{Periods: 1, ExtraMB: 50})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	want := settings.AdmissionSats + settings.SubscriptionSats + 50*settings.PricePerMBSats
	if sats != want {
		t.Errorf("new account price = %d, want %d", sats, want)
	}
	if kind != store.KindAdmission || !meta.Admission {
		t.Errorf("new account should be billed admission, got kind=%q admission=%v", kind, meta.Admission)
	}
	if meta.MB != settings.IncludedMB+50 {
		t.Errorf("granted MB = %d, want %d", meta.MB, settings.IncludedMB+50)
	}

	// once the account exists the admission fee is not charged again
	if _, err := st.EnsureAccount(testPubkey); err != nil {
		t.Fatalf("ensure account: %v", err)
	}
	sats, _, kind, err = s.Quote(testPubkey, Order{Periods: 1})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if sats != settings.SubscriptionSats || kind != store.KindSubscription {
		t.Errorf("renewal = %d sats (%s), want %d sats (subscription)", sats, kind, settings.SubscriptionSats)
	}
}

func TestQuoteRejectsBadOrders(t *testing.T) {
	s, st := newService(t)

	if _, _, _, err := s.Quote(testPubkey, Order{ExtraMB: 10}); err != ErrPeriodRequired {
		t.Errorf("storage-only order for a new account: err = %v, want ErrPeriodRequired", err)
	}
	if _, err := st.EnsureAccount(testPubkey); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Quote(testPubkey, Order{}); err != ErrNothingOrdered {
		t.Errorf("empty order: err = %v, want ErrNothingOrdered", err)
	}
	if _, _, _, err := s.Quote(testPubkey, Order{Periods: maxPeriods + 1}); err != ErrTooLarge {
		t.Errorf("huge order: err = %v, want ErrTooLarge", err)
	}
	if _, _, _, err := s.Quote(testPubkey, Order{Periods: -1}); err != ErrNothingOrdered {
		t.Errorf("negative order: err = %v, want ErrNothingOrdered", err)
	}
}

func TestSettleIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, st := newService(t)
	settings := store.DefaultSettings()

	p, err := s.CreateOrder(ctx, testPubkey, Order{Periods: 1, ExtraMB: 50})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := s.Settle(ctx, p.PaymentHash); err != nil {
			t.Fatalf("settle %d: %v", i, err)
		}
	}

	acct, err := st.Account(testPubkey)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	wantQuota := int64(settings.IncludedMB+50) * store.MB
	if acct.QuotaBytes != wantQuota {
		t.Errorf("quota after 3 settles = %d, want %d (credited once)", acct.QuotaBytes, wantQuota)
	}

	wantExpiry := time.Now().Add(time.Duration(settings.PeriodDays) * 24 * time.Hour).Unix()
	if diff := acct.ExpiresAt - wantExpiry; diff < -5 || diff > 5 {
		t.Errorf("expiry = %d, want ~%d (credited once)", acct.ExpiresAt, wantExpiry)
	}
}

func TestSettleExtendsFromExistingExpiry(t *testing.T) {
	ctx := context.Background()
	s, st := newService(t)
	settings := store.DefaultSettings()

	// an account with a month left on the clock
	if _, err := st.EnsureAccount(testPubkey); err != nil {
		t.Fatal(err)
	}
	existing := time.Now().Add(30 * 24 * time.Hour).Unix()
	if err := st.UpdateAccount(testPubkey, store.StatusActive, existing, 0, ""); err != nil {
		t.Fatal(err)
	}

	p, err := s.CreateOrder(ctx, testPubkey, Order{Periods: 2})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := s.Settle(ctx, p.PaymentHash); err != nil {
		t.Fatalf("settle: %v", err)
	}

	acct, err := st.Account(testPubkey)
	if err != nil {
		t.Fatal(err)
	}
	want := existing + int64(2*settings.PeriodDays)*86400
	if acct.ExpiresAt != want {
		t.Errorf("expiry = %d, want %d (stacked on top of the remaining time)", acct.ExpiresAt, want)
	}
}

func TestCanWriteBoundaries(t *testing.T) {
	now := time.Now().Unix()
	acct := &store.Account{Status: store.StatusActive, QuotaBytes: 1000, UsedBytes: 900}

	if ok, msg := acct.CanWrite(now, 100); !ok {
		t.Errorf("exactly filling the quota should be allowed, got %q", msg)
	}
	if ok, _ := acct.CanWrite(now, 101); ok {
		t.Error("one byte over the quota should be rejected")
	}

	acct.ExpiresAt = now - 1
	if ok, _ := acct.CanWrite(now, 1); ok {
		t.Error("expired account should be rejected")
	}

	acct.ExpiresAt = 0
	acct.Status = store.StatusBanned
	if ok, _ := acct.CanWrite(now, 1); ok {
		t.Error("banned account should be rejected")
	}
}

func TestUsageAccounting(t *testing.T) {
	_, st := newService(t)
	if _, err := st.EnsureAccount(testPubkey); err != nil {
		t.Fatal(err)
	}

	if err := st.AddUsage("event1", testPubkey, 300); err != nil {
		t.Fatal(err)
	}
	if err := st.AddUsage("event1", testPubkey, 300); err != nil { // duplicate save
		t.Fatal(err)
	}
	if err := st.AddUsage("event2", testPubkey, 200); err != nil {
		t.Fatal(err)
	}

	acct, _ := st.Account(testPubkey)
	if acct.UsedBytes != 500 {
		t.Errorf("used = %d, want 500 (duplicate event billed once)", acct.UsedBytes)
	}

	if err := st.RemoveUsage("event1"); err != nil {
		t.Fatal(err)
	}
	acct, _ = st.Account(testPubkey)
	if acct.UsedBytes != 200 {
		t.Errorf("used after delete = %d, want 200", acct.UsedBytes)
	}

	// drift repair
	if _, err := st.DB.Exec(`UPDATE accounts SET used_bytes = 99999`); err != nil {
		t.Fatal(err)
	}
	if err := st.RecomputeUsage(); err != nil {
		t.Fatal(err)
	}
	acct, _ = st.Account(testPubkey)
	if acct.UsedBytes != 200 {
		t.Errorf("used after recompute = %d, want 200", acct.UsedBytes)
	}
}
