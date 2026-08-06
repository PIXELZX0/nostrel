package relay

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/config"
	"nostrel/internal/store"
)

func newGate(t *testing.T) (*Relay, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(st.Close)

	cfg := &config.Config{PanelURL: "https://relay.example.com"}
	return New(cfg, st, log.New(io.Discard, "", 0)), st
}

func event(pubkey, content string) *nostr.Event {
	return &nostr.Event{
		Kind: nostr.KindTextNote, PubKey: pubkey, Content: content,
		CreatedAt: nostr.Now(), Tags: nostr.Tags{},
	}
}

// A group member with no subscription of their own still writes, on the
// group's budget — that is the whole point of sharing storage.
func TestGroupCoversAMemberWithoutAnAccount(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()

	if _, err := st.CreateGroup("g1", "team", me); err != nil {
		t.Fatalf("creating group: %v", err)
	}
	if err := st.AddMember("g1", other); err != nil {
		t.Fatalf("adding member: %v", err)
	}

	// an unfunded group covers nobody
	if reject, msg := r.rejectEvent(ctx, event(other, "hello")); !reject {
		t.Errorf("empty group allowed a write (%s)", msg)
	}

	if err := st.UpdateGroup("g1", "team", me, store.StatusActive, 0, 10_000, ""); err != nil {
		t.Fatalf("funding group: %v", err)
	}
	if reject, msg := r.rejectEvent(ctx, event(other, "hello")); reject {
		t.Errorf("funded group rejected its member: %s", msg)
	}

	// a pubkey in no group is still a stranger
	if reject, _ := r.rejectEvent(ctx, event("cc"+other[2:], "hello")); !reject {
		t.Error("a stranger was allowed to write")
	}
}

func TestPersonalQuotaIsChargedBeforeTheGroup(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()

	if _, err := st.CreateGroup("g1", "team", me); err != nil {
		t.Fatalf("creating group: %v", err)
	}
	if err := st.UpdateGroup("g1", "team", me, store.StatusActive, 0, 100_000, ""); err != nil {
		t.Fatalf("funding group: %v", err)
	}
	if err := st.AddMember("g1", other); err != nil {
		t.Fatalf("adding member: %v", err)
	}
	if _, err := st.EnsureAccount(other); err != nil {
		t.Fatalf("account: %v", err)
	}

	// a small personal quota: the first note fits, the second does not
	evt := event(other, strings.Repeat("x", 100))
	evt.ID = "event1"
	size := eventSize(evt)
	if err := st.UpdateAccount(other, store.StatusActive, 0, size, ""); err != nil {
		t.Fatalf("funding account: %v", err)
	}

	if reject, msg := r.rejectEvent(ctx, evt); reject {
		t.Fatalf("first note rejected: %s", msg)
	}
	r.chargeUsage(ctx, evt)

	acct, _ := st.Account(other)
	group, _ := st.Group("g1")
	if acct.UsedBytes != size || group.UsedBytes != 0 {
		t.Fatalf("first note billed account=%d group=%d, want %d/0", acct.UsedBytes, group.UsedBytes, size)
	}

	// personal quota is spent, so the group picks the next one up
	second := event(other, strings.Repeat("y", 100))
	second.ID = "event2"
	if reject, msg := r.rejectEvent(ctx, second); reject {
		t.Fatalf("second note rejected: %s", msg)
	}
	r.chargeUsage(ctx, second)

	acct, _ = st.Account(other)
	group, _ = st.Group("g1")
	if acct.UsedBytes != size || group.UsedBytes != eventSize(second) {
		t.Errorf("second note billed account=%d group=%d, want %d/%d",
			acct.UsedBytes, group.UsedBytes, size, eventSize(second))
	}

	// deleting it refunds the pot that paid
	if err := r.releaseUsage(ctx, second); err != nil {
		t.Fatalf("release: %v", err)
	}
	group, _ = st.Group("g1")
	if group.UsedBytes != 0 {
		t.Errorf("group used = %d after the delete, want 0", group.UsedBytes)
	}
}

func TestExpiredGroupStopsCoveringMembers(t *testing.T) {
	r, st := newGate(t)
	ctx := context.Background()

	if _, err := st.CreateGroup("g1", "team", me); err != nil {
		t.Fatalf("creating group: %v", err)
	}
	expired := time.Now().Add(-time.Minute).Unix()
	if err := st.UpdateGroup("g1", "team", me, store.StatusActive, expired, 10_000, ""); err != nil {
		t.Fatalf("expiring group: %v", err)
	}
	if err := st.AddMember("g1", other); err != nil {
		t.Fatalf("adding member: %v", err)
	}

	reject, msg := r.rejectEvent(ctx, event(other, "hello"))
	if !reject {
		t.Fatal("an expired group still allowed a write")
	}
	if !strings.Contains(msg, "expired") {
		t.Errorf("message = %q, want it to mention the expiry", msg)
	}
}
