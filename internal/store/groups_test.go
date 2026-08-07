package store

import (
	"path/filepath"
	"testing"
	"time"
)

// seedGroup makes a group with quota bytes of storage and no expiry.
func seedGroup(t *testing.T, s *Store, id, owner string, quota int64) *Group {
	t.Helper()
	g, err := s.CreateGroup(id, "test group", owner)
	if err != nil {
		t.Fatalf("creating group: %v", err)
	}
	if err := s.UpdateGroup(id, g.Name, owner, StatusActive, 0, quota, "", false); err != nil {
		t.Fatalf("funding group: %v", err)
	}
	g, err = s.Group(id)
	if err != nil {
		t.Fatalf("reading group back: %v", err)
	}
	return g
}

// The whole point of the personal-first rule: a member who already paid for
// their own subscription spends it before touching the shared pot.
func TestPersonalQuotaIsSpentBeforeTheGroup(t *testing.T) {
	s := newStore(t)
	seedGroup(t, s, "g1", alice, 10_000)
	if err := s.AddMember("g1", bob); err != nil {
		t.Fatalf("adding member: %v", err)
	}
	if _, err := s.EnsureAccount(bob); err != nil {
		t.Fatalf("account: %v", err)
	}
	if err := s.UpdateAccount(bob, StatusActive, 0, 500, ""); err != nil {
		t.Fatalf("funding account: %v", err)
	}

	if err := s.AddUsage("event1", bob, 400); err != nil {
		t.Fatalf("first write: %v", err)
	}
	acct, _ := s.Account(bob)
	group, _ := s.Group("g1")
	if acct.UsedBytes != 400 || group.UsedBytes != 0 {
		t.Fatalf("first write billed account=%d group=%d, want 400/0", acct.UsedBytes, group.UsedBytes)
	}

	// 400 of 500 personal bytes are gone; a 300 byte write no longer fits, so
	// it falls through to the shared pot
	if err := s.AddUsage("event2", bob, 300); err != nil {
		t.Fatalf("second write: %v", err)
	}
	acct, _ = s.Account(bob)
	group, _ = s.Group("g1")
	if acct.UsedBytes != 400 || group.UsedBytes != 300 {
		t.Fatalf("second write billed account=%d group=%d, want 400/300", acct.UsedBytes, group.UsedBytes)
	}

	// deleting refunds whichever pot actually paid
	if err := s.RemoveUsage("event2"); err != nil {
		t.Fatalf("refund: %v", err)
	}
	acct, _ = s.Account(bob)
	group, _ = s.Group("g1")
	if acct.UsedBytes != 400 || group.UsedBytes != 0 {
		t.Errorf("refund left account=%d group=%d, want 400/0", acct.UsedBytes, group.UsedBytes)
	}
}

func TestAllowanceGate(t *testing.T) {
	s := newStore(t)
	now := time.Now().Unix()
	seedGroup(t, s, "g1", alice, 1000)
	if err := s.AddMember("g1", bob); err != nil {
		t.Fatalf("adding member: %v", err)
	}

	// a member with no account of their own writes on the group's budget
	allowance, err := s.Allowance(bob)
	if err != nil {
		t.Fatalf("allowance: %v", err)
	}
	ok, groupID, msg := allowance.CanWrite(now, 900)
	if !ok || groupID != "g1" {
		t.Errorf("group member: ok=%v group=%q (%s), want true/g1", ok, groupID, msg)
	}
	if ok, _, _ := allowance.CanWrite(now, 1001); ok {
		t.Error("a write past the group quota was allowed")
	}

	// an expired group stops covering its members
	if err := s.UpdateGroup("g1", "", alice, StatusActive, now-1, 1000, "", false); err != nil {
		t.Fatalf("expiring group: %v", err)
	}
	allowance, _ = s.Allowance(bob)
	if ok, _, _ := allowance.CanWrite(now, 10); ok {
		t.Error("an expired group still allowed a write")
	}

	// a stranger has neither pot
	allowance, err = s.Allowance("cc" + bob[2:])
	if err != nil {
		t.Fatalf("allowance for a stranger: %v", err)
	}
	if allowance.Account != nil || allowance.Group != nil {
		t.Error("a stranger should have no account and no group")
	}
	if allowance.Whitelisted() {
		t.Error("a stranger should not be whitelisted")
	}
}

func TestOnePubkeyBelongsToOneGroup(t *testing.T) {
	s := newStore(t)
	carol := "cc11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"
	seedGroup(t, s, "g1", alice, 1000)
	seedGroup(t, s, "g2", carol, 1000)

	if err := s.AddMember("g1", bob); err != nil {
		t.Fatalf("first group: %v", err)
	}
	if err := s.AddMember("g2", bob); err != nil {
		t.Fatalf("second group: %v", err)
	}

	group, err := s.GroupOf(bob)
	if err != nil {
		t.Fatalf("group of: %v", err)
	}
	if group.ID != "g2" {
		t.Errorf("member landed in %q, want g2 — joining a group must leave the old one", group.ID)
	}
	members, err := s.ListMembers("g1")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(members) != 1 || members[0].Pubkey != alice {
		t.Errorf("old group still lists %+v, want only the owner", members)
	}
}

func TestRecomputeUsageRebuildsBothPots(t *testing.T) {
	s := newStore(t)
	seedGroup(t, s, "g1", alice, 10_000)
	if _, err := s.EnsureAccount(alice); err != nil {
		t.Fatalf("account: %v", err)
	}
	if err := s.UpdateAccount(alice, StatusActive, 0, 500, ""); err != nil {
		t.Fatalf("funding account: %v", err)
	}
	if err := s.AddUsage("personal", alice, 400); err != nil {
		t.Fatalf("personal write: %v", err)
	}
	if err := s.AddUsage("shared", alice, 800); err != nil {
		t.Fatalf("shared write: %v", err)
	}

	// simulate the drift a crash mid-write leaves behind
	if _, err := s.DB.Exec(`UPDATE accounts SET used_bytes = 99999`); err != nil {
		t.Fatalf("corrupting account: %v", err)
	}
	if _, err := s.DB.Exec(`UPDATE groups SET used_bytes = 0`); err != nil {
		t.Fatalf("corrupting group: %v", err)
	}

	if err := s.RecomputeUsage(); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	acct, _ := s.Account(alice)
	group, _ := s.Group("g1")
	if acct.UsedBytes != 400 {
		t.Errorf("account used = %d, want 400", acct.UsedBytes)
	}
	if group.UsedBytes != 800 {
		t.Errorf("group used = %d, want 800", group.UsedBytes)
	}
}

// A database created before groups existed has no usage_events.group_id, and
// opening it must add the column instead of failing — and must stay happy on
// every open after that.
func TestOpenAddsMissingColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := s.DB.Exec(`INSERT INTO usage_events (id, pubkey, size, created_at, group_id)
		VALUES ('e1', ?, 10, 1, '')`, alice); err != nil {
		t.Fatalf("seeding usage: %v", err)
	}
	// pretend the column was never there
	if _, err := s.DB.Exec(`ALTER TABLE usage_events DROP COLUMN group_id`); err != nil {
		t.Fatalf("dropping column: %v", err)
	}
	s.Close()

	for i := range 2 {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i+1, err)
		}
		var count int
		if err := s.DB.Get(&count,
			`SELECT count(*) FROM pragma_table_info('usage_events') WHERE name = 'group_id'`); err != nil {
			t.Fatalf("checking column: %v", err)
		}
		if count != 1 {
			t.Fatalf("open %d left %d group_id columns, want 1", i+1, count)
		}
		s.Close()
	}
}
