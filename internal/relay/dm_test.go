package relay

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/store"
)

// giftWrap is a NIP-59 wrap for `to`, signed by the throwaway key NIP-59
// requires — never by the sender's own.
func giftWrap(t *testing.T, to string) *nostr.Event {
	t.Helper()
	wrap := &nostr.Event{
		Kind:      nostr.KindGiftWrap,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{{"p", to}},
		Content:   "nip44 ciphertext",
	}
	if err := wrap.Sign(nostr.GeneratePrivateKey()); err != nil {
		t.Fatalf("signing gift wrap: %v", err)
	}
	return wrap
}

func enableDirectMessages(t *testing.T, st *store.Store, on bool) {
	t.Helper()
	settings, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.AcceptDirectMessages = on
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
}

// The whole point: a gift wrap is signed by a key that exists for one message,
// so without the exception NIP-17 chat bounces here — even between two paying
// customers.
func TestGiftWrapForACustomerIsAccepted(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	fundedAccount(t, st, me)

	wrap := giftWrap(t, me)
	if reject, msg := r.rejectEvent(ctx, wrap); reject {
		t.Errorf("gift wrap for a paying customer rejected: %s", msg)
	}

	// and it is the recipient's storage it occupies
	wrap.ID = "wrap1"
	r.chargeUsage(ctx, wrap)
	recipient, err := st.Account(me)
	if err != nil {
		t.Fatal(err)
	}
	if recipient.UsedBytes != eventSize(wrap) {
		t.Errorf("recipient used = %d, want %d", recipient.UsedBytes, eventSize(wrap))
	}
	if _, err := st.Account(wrap.PubKey); err != store.ErrNoAccount {
		t.Error("the throwaway wrapping key ended up with an account")
	}
}

func TestGiftWrapForAStrangerIsRefused(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	fundedAccount(t, st, me)

	if reject, _ := r.rejectEvent(ctx, giftWrap(t, other)); !reject {
		t.Error("a gift wrap addressed to nobody with an account was accepted")
	}
}

func TestMalformedGiftWrapIsRefused(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	fundedAccount(t, st, me)

	noRecipient := giftWrap(t, me)
	noRecipient.Tags = nostr.Tags{}
	if reject, _ := r.rejectEvent(ctx, noRecipient); !reject {
		t.Error("a gift wrap naming nobody was accepted")
	}

	twoRecipients := giftWrap(t, me)
	twoRecipients.Tags = nostr.Tags{{"p", me}, {"p", other}}
	if reject, _ := r.rejectEvent(ctx, twoRecipients); !reject {
		t.Error("a gift wrap with two recipients was accepted")
	}

	empty := giftWrap(t, me)
	empty.Content = ""
	if reject, _ := r.rejectEvent(ctx, empty); !reject {
		t.Error("an empty gift wrap was accepted")
	}
}

func TestDirectMessagesCanBeTurnedOff(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	fundedAccount(t, st, me)
	enableDirectMessages(t, st, false)

	if reject, _ := r.rejectEvent(ctx, giftWrap(t, me)); !reject {
		t.Error("direct messages are off but a gift wrap was accepted")
	}

	// a stranger's legacy DM goes with it
	fromOutside := event(other, "encrypted")
	fromOutside.Kind = nostr.KindEncryptedDirectMessage
	fromOutside.Tags = nostr.Tags{{"p", me}}
	if reject, _ := r.rejectEvent(ctx, fromOutside); !reject {
		t.Error("direct messages are off but a stranger's kind 4 was accepted")
	}

	// the customer's own outgoing message is not third-party traffic, so the
	// switch must not touch it
	mine := event(me, "encrypted")
	mine.Kind = nostr.KindEncryptedDirectMessage
	mine.Tags = nostr.Tags{{"p", other}}
	if reject, msg := r.rejectEvent(ctx, mine); reject {
		t.Errorf("a customer's own kind 4 was rejected: %s", msg)
	}
}

// A customer writing a kind 4 pays for it themselves, the way they pay for
// every other event they write.
func TestOwnDirectMessageIsBilledToItsAuthor(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()
	fundedAccount(t, st, me)
	fundedAccount(t, st, other)

	dm := event(me, "encrypted")
	dm.Kind = nostr.KindEncryptedDirectMessage
	dm.Tags = nostr.Tags{{"p", other}}
	dm.ID = "dm1"
	if reject, msg := r.rejectEvent(ctx, dm); reject {
		t.Fatalf("rejected: %s", msg)
	}
	r.chargeUsage(ctx, dm)

	author, err := st.Account(me)
	if err != nil {
		t.Fatal(err)
	}
	if author.UsedBytes != eventSize(dm) {
		t.Errorf("author used = %d, want %d", author.UsedBytes, eventSize(dm))
	}
	recipient, err := st.Account(other)
	if err != nil {
		t.Fatal(err)
	}
	if recipient.UsedBytes != 0 {
		t.Errorf("recipient used = %d, want 0", recipient.UsedBytes)
	}
}
