package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/store"
)

// newNip43 wires a relay that can sign as itself.
func newNip43(t *testing.T) (*Relay, *store.Store, string) {
	t.Helper()
	r, st := newGate(t)
	key := nostr.GeneratePrivateKey()
	self, err := nostr.GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	r.cfg.RelaySecretKey = key
	r.cfg.ServiceURL = serviceURL
	r.selfPubkey = self
	return r, st, self
}

func joinRequest(t *testing.T, key, code string, at nostr.Timestamp) *nostr.Event {
	t.Helper()
	tags := nostr.Tags{{"-"}}
	if code != "" {
		tags = append(tags, nostr.Tag{"claim", code})
	}
	evt := &nostr.Event{Kind: KindJoinRequest, CreatedAt: at, Tags: tags}
	if err := evt.Sign(key); err != nil {
		t.Fatal(err)
	}
	return evt
}

func hasProtectedTag(evt *nostr.Event) bool {
	for _, tag := range evt.Tags {
		if len(tag) > 0 && tag[0] == "-" {
			return true
		}
	}
	return false
}

func newInvite(t *testing.T, st *store.Store, inv store.Invite) *store.Invite {
	t.Helper()
	created, err := st.CreateInvite(inv)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func TestJoinRequiresASigningKey(t *testing.T) {
	r, st := newGate(t) // no RELAY_SECRET_KEY
	invite := newInvite(t, st, store.Invite{PeriodDays: 30, QuotaMB: 100, MaxUses: 1})

	request := joinRequest(t, nostr.GeneratePrivateKey(), invite.Code, nostr.Now())
	reject, msg := r.rejectEvent(context.Background(), request)
	if !reject || msg != "restricted: this relay does not accept invite codes" {
		t.Errorf("without a key: reject=%v msg=%q", reject, msg)
	}
	if slices.Contains(advertised(t, r), 43) {
		t.Error("NIP-43 advertised without a signing key")
	}
}

func TestJoinWithAnInviteCreatesAnAccount(t *testing.T) {
	r, st, self := newNip43(t)
	ctx := context.Background()
	invite := newInvite(t, st, store.Invite{PeriodDays: 30, QuotaMB: 100, MaxUses: 1})

	key := nostr.GeneratePrivateKey()
	author, err := nostr.GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}

	if reject, msg := r.rejectEvent(ctx, joinRequest(t, key, invite.Code, nostr.Now())); reject {
		t.Fatalf("valid join rejected: %s", msg)
	}

	account, err := st.Account(author)
	if err != nil {
		t.Fatalf("no account was created: %v", err)
	}
	if account.QuotaBytes != 100*store.MB {
		t.Errorf("quota = %d, want %d", account.QuotaBytes, 100*store.MB)
	}
	if account.ExpiresAt == 0 {
		t.Error("the account has no expiry")
	}
	// an invite waives the admission fee, so no payment should exist
	payments, _ := st.ListPayments(author, 10)
	if len(payments) != 0 {
		t.Errorf("%d payments recorded for an invited user", len(payments))
	}

	// the relay announced it, signed as itself
	events, err := st.Events.QueryEvents(ctx, nostr.Filter{Kinds: []int{KindMemberAdded}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for evt := range events {
		found = true
		if evt.PubKey != self {
			t.Errorf("kind 8000 signed by %s, want the relay key", evt.PubKey)
		}
		if evt.Tags.Find("p").Value() != author {
			t.Errorf("kind 8000 names %q, want %s", evt.Tags.Find("p").Value(), author)
		}
		// Tags.Find ignores single-element tags, so the NIP-70 marker is
		// looked for by hand
		if !hasProtectedTag(evt) {
			t.Error("kind 8000 is missing the NIP-70 - tag")
		}
	}
	if !found {
		t.Error("no kind 8000 was published")
	}
}

func TestJoinRejections(t *testing.T) {
	r, st, _ := newNip43(t)
	ctx := context.Background()
	key := nostr.GeneratePrivateKey()

	spent := newInvite(t, st, store.Invite{PeriodDays: 30, MaxUses: 1})
	if _, err := st.ClaimInvite(spent.Code, "ff"+me[2:]); err != nil {
		t.Fatal(err)
	}
	expired := newInvite(t, st, store.Invite{
		PeriodDays: 30, MaxUses: 1, ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	})

	cases := []struct {
		name    string
		request *nostr.Event
		want    string
	}{
		{"no claim tag", joinRequest(t, key, "", nostr.Now()),
			"invalid: a join request needs a claim tag"},
		{"unknown code", joinRequest(t, key, "deadbeef", nostr.Now()),
			"restricted: that is an invalid invite code."},
		{"expired code", joinRequest(t, key, expired.Code, nostr.Now()),
			"restricted: that invite code is expired."},
		{"used up code", joinRequest(t, key, spent.Code, nostr.Now()),
			"restricted: that invite code has been used up."},
		{"stale timestamp", joinRequest(t, key, "deadbeef", nostr.Now()-600),
			"invalid: created_at must be within a few minutes of now"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reject, msg := r.rejectEvent(ctx, tc.request)
			if !reject || msg != tc.want {
				t.Errorf("reject=%v msg=%q, want true %q", reject, msg, tc.want)
			}
		})
	}
}

// Joining twice must not spend a second code.
func TestJoinTwiceIsANoOp(t *testing.T) {
	r, st, _ := newNip43(t)
	ctx := context.Background()
	invite := newInvite(t, st, store.Invite{PeriodDays: 30, QuotaMB: 100, MaxUses: 5})

	key := nostr.GeneratePrivateKey()
	if reject, msg := r.rejectEvent(ctx, joinRequest(t, key, invite.Code, nostr.Now())); reject {
		t.Fatalf("first join: %s", msg)
	}
	if reject, msg := r.rejectEvent(ctx, joinRequest(t, key, invite.Code, nostr.Now())); reject {
		t.Fatalf("second join should be accepted as a duplicate: %s", msg)
	}

	after, _ := st.Invite(invite.Code)
	if after.Used != 1 {
		t.Errorf("used = %d after joining twice, want 1", after.Used)
	}
}

func TestLeaveRevokesButKeepsData(t *testing.T) {
	r, st, self := newNip43(t)
	ctx := context.Background()

	key := nostr.GeneratePrivateKey()
	author, err := nostr.GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	fundedAccount(t, st, author)

	note := &nostr.Event{Kind: nostr.KindTextNote, CreatedAt: nostr.Now(), Content: "mine"}
	if err := note.Sign(key); err != nil {
		t.Fatal(err)
	}
	if err := st.Events.SaveEvent(ctx, note); err != nil {
		t.Fatal(err)
	}

	leave := &nostr.Event{Kind: KindLeaveRequest, CreatedAt: nostr.Now(), Tags: nostr.Tags{{"-"}}}
	if err := leave.Sign(key); err != nil {
		t.Fatal(err)
	}
	if reject, msg := r.rejectEvent(ctx, leave); reject {
		t.Fatalf("leave rejected: %s", msg)
	}

	account, err := st.Account(author)
	if err != nil {
		t.Fatal(err)
	}
	if account.Status != store.StatusBanned {
		t.Errorf("status = %q after leaving, want banned", account.Status)
	}

	// the events they wrote are still there — leaving is not vanishing
	var remaining int
	if err := st.DB.Get(&remaining, `SELECT count(*) FROM event WHERE pubkey = ?`, author); err != nil {
		t.Fatal(err)
	}
	if remaining == 0 {
		t.Error("leaving deleted the user's events")
	}

	events, _ := st.Events.QueryEvents(ctx, nostr.Filter{Kinds: []int{KindMemberRemoved}})
	found := false
	for evt := range events {
		found = evt.PubKey == self && evt.Tags.Find("p").Value() == author
	}
	if !found {
		t.Error("no kind 8001 was published for the departing member")
	}
}

func TestInviteRequestOnlyWhenAutoInviteIsOn(t *testing.T) {
	r, st, self := newNip43(t)
	ctx := context.Background()
	filter := nostr.Filter{Kinds: []int{KindInviteRequest}}

	drain := func() []*nostr.Event {
		ch, err := r.queryEvents(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		var out []*nostr.Event
		for evt := range ch {
			out = append(out, evt)
		}
		return out
	}

	if got := drain(); len(got) != 0 {
		t.Errorf("%d invites handed out while auto-invite is off", len(got))
	}

	settings, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.AutoInvite = true
	settings.AutoInvitePeriodDays = 7
	settings.AutoInviteQuotaMB = 50
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	events := drain()
	if len(events) != 1 {
		t.Fatalf("%d invites, want 1", len(events))
	}
	if events[0].PubKey != self {
		t.Errorf("invite signed by %s, want the relay key", events[0].PubKey)
	}
	code := events[0].Tags.Find("claim").Value()
	if code == "" {
		t.Fatal("the invite has no claim tag")
	}

	// the code it handed out actually works, and only once
	key := nostr.GeneratePrivateKey()
	if reject, msg := r.rejectEvent(ctx, joinRequest(t, key, code, nostr.Now())); reject {
		t.Fatalf("the relay's own code was refused: %s", msg)
	}
	if reject, _ := r.rejectEvent(ctx, joinRequest(t, nostr.GeneratePrivateKey(), code, nostr.Now())); !reject {
		t.Error("a single-use auto invite was accepted twice")
	}

	// each request mints a different code
	second := drain()
	if len(second) == 1 && second[0].Tags.Find("claim").Value() == code {
		t.Error("the same code was handed out twice")
	}
}

func TestMembershipListMarksAdmins(t *testing.T) {
	r, st, self := newNip43(t)
	ctx := context.Background()

	fundedAccount(t, st, me)
	fundedAccount(t, st, other)
	r.cfg.AdminPubkeys = []string{other}

	r.PublishMembership(ctx)

	events, err := st.Events.QueryEvents(ctx, nostr.Filter{Kinds: []int{KindMembershipList}})
	if err != nil {
		t.Fatal(err)
	}
	var list *nostr.Event
	for evt := range events {
		list = evt
	}
	if list == nil {
		t.Fatal("no membership list was published")
	}
	if list.PubKey != self {
		t.Errorf("list signed by %s, want the relay key", list.PubKey)
	}

	roles := map[string]string{}
	for _, tag := range list.Tags {
		if len(tag) >= 2 && tag[0] == "member" {
			roles[tag[1]] = ""
			if len(tag) >= 3 {
				roles[tag[1]] = tag[2]
			}
		}
	}
	if _, ok := roles[me]; !ok {
		t.Error("a paying member is missing from the list")
	}
	if roles[other] != "admin" {
		t.Errorf("admin role = %q, want admin", roles[other])
	}
}

// The OK wording is the part of NIP-43 the vendored khatru exists for: upstream
// replaces it with "broadcasted to N listeners" for every ephemeral event.
func TestOKMessagesMatchTheSpec(t *testing.T) {
	r, st, _ := newNip43(t)
	ctx := context.Background()
	invite := newInvite(t, st, store.Invite{PeriodDays: 30, QuotaMB: 100, MaxUses: 5})
	key := nostr.GeneratePrivateKey()

	// what khatru would have said on its own, for an accepted ephemeral event
	const khatruSays = "broadcasted to 0 listeners"

	join := joinRequest(t, key, invite.Code, nostr.Now())
	if reject, _ := r.rejectEvent(ctx, join); reject {
		t.Fatal("join rejected")
	}
	if _, reason := r.overwriteOK(ctx, join, true, khatruSays); reason != "info: welcome to "+serviceURL+"!" {
		t.Errorf("welcome = %q", reason)
	}

	duplicate := joinRequest(t, key, invite.Code, nostr.Now())
	if reject, _ := r.rejectEvent(ctx, duplicate); reject {
		t.Fatal("duplicate join rejected")
	}
	if _, reason := r.overwriteOK(ctx, duplicate, true, khatruSays); reason != "duplicate: you are already a member of this relay." {
		t.Errorf("duplicate = %q", reason)
	}

	// each message is consumed once, so a later event cannot inherit it
	if _, reason := r.overwriteOK(ctx, duplicate, true, khatruSays); reason != khatruSays {
		t.Errorf("a used message came back: %q", reason)
	}

	// and an unrelated event keeps whatever khatru decided
	note := event(me, "hello")
	note.ID = "note1"
	if ok, reason := r.overwriteOK(ctx, note, false, "restricted: nope"); ok || reason != "restricted: nope" {
		t.Errorf("ordinary event: ok=%v reason=%q", ok, reason)
	}
}

func TestNIP11CarriesSelf(t *testing.T) {
	r, _, self := newNip43(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/nostr+json")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	var document map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	if document["self"] != self {
		t.Errorf("self = %v, want %s", document["self"], self)
	}
	nips, _ := document["supported_nips"].([]any)
	if !slices.ContainsFunc(nips, func(v any) bool { return v == float64(43) }) {
		t.Errorf("supported_nips = %v, want it to include 43", nips)
	}
}
