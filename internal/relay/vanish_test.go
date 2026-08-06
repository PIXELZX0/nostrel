package relay

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

const serviceURL = "wss://relay.example.com"

func vanishRequest(t *testing.T, key string, tags nostr.Tags, content string) *nostr.Event {
	t.Helper()
	evt := &nostr.Event{
		Kind:      KindRequestToVanish,
		CreatedAt: nostr.Now(),
		Tags:      tags,
		Content:   content,
	}
	if err := evt.Sign(key); err != nil {
		t.Fatal(err)
	}
	return evt
}

// A vanish request must work for somebody who never paid — NIP-62 says a paid
// relay complies "regardless of user status".
func TestVanishIsAcceptedWithoutAnAccount(t *testing.T) {
	r, _ := newGate(t)
	r.cfg.ServiceURL = serviceURL
	ctx := context.Background()

	key := nostr.GeneratePrivateKey()
	request := vanishRequest(t, key, nostr.Tags{{"relay", serviceURL}}, "")
	if reject, msg := r.rejectEvent(ctx, request); reject {
		t.Errorf("vanish request from a non-customer rejected: %s", msg)
	}

	// ALL_RELAYS is the broadcast form
	wildcard := vanishRequest(t, key, nostr.Tags{{"relay", allRelays}}, "")
	if reject, msg := r.rejectEvent(ctx, wildcard); reject {
		t.Errorf("ALL_RELAYS request rejected: %s", msg)
	}

	// but a request naming somebody else's relay is not ours to act on
	elsewhere := vanishRequest(t, key, nostr.Tags{{"relay", "wss://elsewhere.example"}}, "")
	if reject, _ := r.rejectEvent(ctx, elsewhere); !reject {
		t.Error("a request naming another relay was accepted")
	}
	if reject, _ := r.rejectEvent(ctx, vanishRequest(t, key, nostr.Tags{}, "")); !reject {
		t.Error("a request naming no relay at all was accepted")
	}

	// and it cannot be used as free storage
	long := make([]byte, maxVanishReason+1)
	for i := range long {
		long[i] = 'x'
	}
	if reject, _ := r.rejectEvent(ctx, vanishRequest(t, key, nostr.Tags{{"relay", serviceURL}}, string(long))); !reject {
		t.Error("an oversized vanish request was accepted")
	}
}

func TestVanishDeletesEventsAndRefundsQuota(t *testing.T) {
	r, st := newGate(t)
	r.cfg.ServiceURL = serviceURL
	ctx := context.Background()

	key := nostr.GeneratePrivateKey()
	author, err := nostr.GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	fundedAccount(t, st, author)

	note := &nostr.Event{Kind: nostr.KindTextNote, CreatedAt: nostr.Now(), Content: "hello"}
	if err := note.Sign(key); err != nil {
		t.Fatal(err)
	}
	if err := st.Events.SaveEvent(ctx, note); err != nil {
		t.Fatal(err)
	}
	r.chargeUsage(ctx, note)

	acct, _ := st.Account(author)
	if acct.UsedBytes == 0 {
		t.Fatal("the note was not billed, so the refund cannot be observed")
	}

	request := vanishRequest(t, key, nostr.Tags{{"relay", "https://relay.example.com/"}}, "지워주세요")
	r.vanish(ctx, request)

	if got := st.VanishedAt(author); got == 0 {
		t.Error("no cutoff was recorded")
	}
	acct, _ = st.Account(author)
	if acct.UsedBytes != 0 {
		t.Errorf("used = %d after vanishing, want 0", acct.UsedBytes)
	}

	var remaining int
	if err := st.DB.Get(&remaining, `SELECT count(*) FROM event WHERE pubkey = ?`, author); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("%d events survived the vanish", remaining)
	}
}

// "MUST ensure the deleted events cannot be re-broadcasted into the relay."
func TestVanishedEventsCannotComeBack(t *testing.T) {
	r, st := newGate(t)
	r.cfg.ServiceURL = serviceURL
	ctx := context.Background()

	key := nostr.GeneratePrivateKey()
	author, err := nostr.GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	fundedAccount(t, st, author)

	old := &nostr.Event{Kind: nostr.KindTextNote, CreatedAt: nostr.Now() - 60, Content: "old"}
	if err := old.Sign(key); err != nil {
		t.Fatal(err)
	}
	r.vanish(ctx, vanishRequest(t, key, nostr.Tags{{"relay", serviceURL}}, ""))

	if reject, _ := r.rejectEvent(ctx, old); !reject {
		t.Error("a vanished event was re-accepted")
	}

	// the user may keep using the relay afterwards; only the past is sealed
	fresh := &nostr.Event{Kind: nostr.KindTextNote, CreatedAt: nostr.Now() + 60, Content: "new"}
	if err := fresh.Sign(key); err != nil {
		t.Fatal(err)
	}
	if reject, msg := r.rejectEvent(ctx, fresh); reject {
		t.Errorf("a new event after vanishing was rejected: %s", msg)
	}

	// an admin can lift it
	if err := st.Unvanish(author); err != nil {
		t.Fatal(err)
	}
	if reject, msg := r.rejectEvent(ctx, old); reject {
		t.Errorf("unvanish did not lift the block: %s", msg)
	}
}
