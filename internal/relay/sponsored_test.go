package relay

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/store"
)

// zapReceipt builds a kind 9735 receipt for `to`, embedding a zap request the
// sender signed. Anything left empty is deliberately broken for the rejection
// cases.
func zapReceipt(t *testing.T, to string, mangle func(*nostr.Event, *nostr.Event)) *nostr.Event {
	t.Helper()
	senderKey := nostr.GeneratePrivateKey()
	request := &nostr.Event{
		Kind:      nostr.KindZapRequest,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{{"p", to}, {"amount", "21000"}},
		Content:   "thanks",
	}
	if err := request.Sign(senderKey); err != nil {
		t.Fatalf("signing zap request: %v", err)
	}

	zapperKey := nostr.GeneratePrivateKey()
	receipt := &nostr.Event{
		Kind:      nostr.KindZap,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{{"p", to}, {"bolt11", "lnbc210n1..."}},
	}
	if mangle != nil {
		mangle(receipt, request)
	}
	if receipt.Tags.Find("description").Value() == "" {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		receipt.Tags = append(receipt.Tags, nostr.Tag{"description", string(encoded)})
	}
	if err := receipt.Sign(zapperKey); err != nil {
		t.Fatalf("signing receipt: %v", err)
	}
	return receipt
}

func fundedAccount(t *testing.T, st *store.Store, pubkey string) {
	t.Helper()
	if _, err := st.EnsureAccount(pubkey); err != nil {
		t.Fatalf("account: %v", err)
	}
	if err := st.UpdateAccount(pubkey, store.StatusActive, 0, 100_000, ""); err != nil {
		t.Fatalf("funding: %v", err)
	}
}

func enableZaps(t *testing.T, st *store.Store, on bool) {
	t.Helper()
	settings, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.AcceptZapReceipts = on
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
}

func enableBadges(t *testing.T, st *store.Store, on bool) {
	t.Helper()
	settings, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.AcceptBadgeAwards = on
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
}

// badgeAward builds a kind 8 award from an issuer with no account here.
func badgeAward(t *testing.T, tags nostr.Tags) *nostr.Event {
	t.Helper()
	issuerKey := nostr.GeneratePrivateKey()
	issuer, err := nostr.GetPublicKey(issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	award := &nostr.Event{Kind: KindBadgeAward, CreatedAt: nostr.Now()}
	for _, tag := range tags {
		// "a" tags are rewritten to point at this issuer's own definition
		// unless the case under test supplied a full address itself
		if len(tag) == 2 && tag[0] == "a" && tag[1] == "self" {
			tag = nostr.Tag{"a", "30009:" + issuer + ":bravery"}
		}
		award.Tags = append(award.Tags, tag)
	}
	if err := award.Sign(issuerKey); err != nil {
		t.Fatal(err)
	}
	return award
}

// The whole point: the zapper has no account, so without the exception every
// zap a paying customer receives would bounce.
func TestZapReceiptFromAStrangerIsAcceptedForACustomer(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	fundedAccount(t, st, me)

	receipt := zapReceipt(t, me, nil)

	enableZaps(t, st, false)
	if reject, msg := r.rejectEvent(ctx, receipt); !reject {
		t.Errorf("receipts are off but one was accepted (%s)", msg)
	}

	enableZaps(t, st, true)
	if reject, msg := r.rejectEvent(ctx, receipt); reject {
		t.Errorf("receipt for a paying customer rejected: %s", msg)
	}
}

func TestZapReceiptIsBilledToTheRecipient(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	enableZaps(t, st, true)
	fundedAccount(t, st, me)

	receipt := zapReceipt(t, me, nil)
	receipt.ID = "zap1"
	if reject, msg := r.rejectEvent(ctx, receipt); reject {
		t.Fatalf("rejected: %s", msg)
	}
	r.chargeUsage(ctx, receipt)

	recipient, err := st.Account(me)
	if err != nil {
		t.Fatal(err)
	}
	if recipient.UsedBytes != eventSize(receipt) {
		t.Errorf("recipient used = %d, want %d", recipient.UsedBytes, eventSize(receipt))
	}
	// the zapper must not have been given an account by writing a receipt
	if _, err := st.Account(receipt.PubKey); err != store.ErrNoAccount {
		t.Error("the zapper ended up with an account of their own")
	}
}

func TestZapReceiptRejections(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	enableZaps(t, st, true)
	fundedAccount(t, st, me)

	cases := []struct {
		name   string
		mangle func(receipt, request *nostr.Event)
	}{
		{"no bolt11", func(receipt, _ *nostr.Event) {
			receipt.Tags = nostr.Tags{{"p", me}}
		}},
		{"description is not a zap request", func(receipt, _ *nostr.Event) {
			receipt.Tags = append(receipt.Tags, nostr.Tag{"description", `{"kind":1,"content":"nope"}`})
		}},
		{"description is not json", func(receipt, _ *nostr.Event) {
			receipt.Tags = append(receipt.Tags, nostr.Tag{"description", "not json"})
		}},
		{"zap request names somebody else", func(receipt, request *nostr.Event) {
			request.Tags = nostr.Tags{{"p", other}}
			_ = request.Sign(nostr.GeneratePrivateKey())
		}},
		{"zap request signature is forged", func(receipt, request *nostr.Event) {
			request.Sig = "00" + request.Sig[2:]
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if reject, _ := r.rejectEvent(ctx, zapReceipt(t, me, tc.mangle)); !reject {
				t.Error("a malformed receipt was accepted")
			}
		})
	}

	// a receipt for somebody who is not a customer is still refused
	if reject, _ := r.rejectEvent(ctx, zapReceipt(t, other, nil)); !reject {
		t.Error("a receipt for a stranger was accepted")
	}
}

// A badge award has the same shape of problem as a zap receipt: the issuer is
// a stranger, the recipient is the customer.
func TestBadgeAwardFromAStranger(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	fundedAccount(t, st, me)

	award := badgeAward(t, nostr.Tags{{"a", "self"}, {"p", me}})

	enableBadges(t, st, false)
	if reject, msg := r.rejectEvent(ctx, award); !reject {
		t.Errorf("awards are off but one was accepted (%s)", msg)
	}

	enableBadges(t, st, true)
	if reject, msg := r.rejectEvent(ctx, award); reject {
		t.Fatalf("award for a paying customer rejected: %s", msg)
	}

	award.ID = "badge1"
	r.chargeUsage(ctx, award)
	acct, err := st.Account(me)
	if err != nil {
		t.Fatal(err)
	}
	if acct.UsedBytes != eventSize(award) {
		t.Errorf("recipient used = %d, want %d", acct.UsedBytes, eventSize(award))
	}
}

func TestBadgeAwardRejections(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	enableBadges(t, st, true)
	fundedAccount(t, st, me)

	stranger := "cc11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"
	cases := []struct {
		name string
		tags nostr.Tags
	}{
		{"no recipient", nostr.Tags{{"a", "self"}}},
		{"no badge definition", nostr.Tags{{"p", me}}},
		{"definition is not kind 30009", nostr.Tags{{"a", "30023:x:y"}, {"p", me}}},
		{"awarding somebody else's badge", nostr.Tags{{"a", "30009:" + stranger + ":bravery"}, {"p", me}}},
		{"nobody named has an account", nostr.Tags{{"a", "self"}, {"p", stranger}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if reject, _ := r.rejectEvent(ctx, badgeAward(t, tc.tags)); !reject {
				t.Error("a malformed award was accepted")
			}
		})
	}

	// several recipients are allowed; the first customer named pays
	award := badgeAward(t, nostr.Tags{{"a", "self"}, {"p", stranger}, {"p", me}})
	if reject, msg := r.rejectEvent(ctx, award); reject {
		t.Fatalf("multi-recipient award rejected: %s", msg)
	}
	if got := r.billedPubkey(award); got != me {
		t.Errorf("billed %s, want the customer %s", got, me)
	}
}

// NIP-59 gift wraps backdate created_at on purpose, so a configured past limit
// must not throw them away.
func TestGiftWrapIgnoresThePastLimit(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	fundedAccount(t, st, me)
	settings, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.CreatedAtMaxPast = 3600
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	old := nostr.Timestamp(time.Now().Add(-47 * time.Hour).Unix())

	note := event(me, "an ordinary note")
	note.CreatedAt = old
	if reject, _ := r.rejectEvent(ctx, note); !reject {
		t.Error("an old regular event slipped past the created_at limit")
	}

	wrap := event(me, "sealed")
	wrap.Kind = nostr.KindGiftWrap
	wrap.Tags = nostr.Tags{{"p", me}}
	wrap.CreatedAt = old
	if reject, msg := r.rejectEvent(ctx, wrap); reject {
		t.Errorf("backdated gift wrap rejected: %s", msg)
	}
}

// Everything else on the user's list is an ordinary event kind: the relay only
// has to not stand in the way.
func TestClientKindsAreCarried(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	fundedAccount(t, st, me)

	for name, kind := range map[string]int{
		"zap goal (NIP-75)":          9041,
		"badge definition (NIP-58)":  30009,
		"profile badges (NIP-58)":    10008,
		"badge set (NIP-58)":         30008,
		"forum thread (NIP-7D)":      11,
		"highlight (NIP-84)":         9802,
		"trusted assertion (NIP-85)": 30382,
		"blossom server list (B7)":   10063,
	} {
		evt := event(me, "x")
		evt.Kind = kind
		if reject, msg := r.rejectEvent(ctx, evt); reject {
			t.Errorf("%s (kind %d) rejected: %s", name, kind, msg)
		}
	}
}
