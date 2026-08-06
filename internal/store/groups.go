package store

import (
	"database/sql"
	"errors"
	"time"
)

// Groups let several pubkeys share one pot of storage. A group buys periods and
// megabytes exactly like a personal account does, but every member may spend
// from it.
//
// Only storage is shared. Whether a member may write at all is decided by
// Allowance: their own account first, the group second — so a member who
// already paid for a personal subscription spends their own quota before
// touching the shared one.

var ErrNoGroup = errors.New("no such group")

type Group struct {
	ID         string `db:"id" json:"id"`
	Name       string `db:"name" json:"name"`
	Owner      string `db:"owner" json:"owner"`
	Status     string `db:"status" json:"status"`
	ExpiresAt  int64  `db:"expires_at" json:"expires_at"` // unix seconds; 0 = never expires
	QuotaBytes int64  `db:"quota_bytes" json:"quota_bytes"`
	UsedBytes  int64  `db:"used_bytes" json:"used_bytes"`
	CreatedAt  int64  `db:"created_at" json:"created_at"`
	Note       string `db:"note" json:"note"`
}

type Member struct {
	Pubkey  string `db:"pubkey" json:"pubkey"`
	GroupID string `db:"group_id" json:"group_id"`
	AddedAt int64  `db:"added_at" json:"added_at"`
}

// CanSpend reports whether the group may absorb size more bytes.
func (g *Group) CanSpend(now, size int64) (bool, string) {
	if g.Status == StatusBanned {
		return false, "blocked: this group is banned"
	}
	if g.ExpiresAt != 0 && g.ExpiresAt < now {
		return false, "restricted: the group subscription expired"
	}
	if g.UsedBytes+size > g.QuotaBytes {
		return false, "blocked: the group storage quota is exhausted"
	}
	return true, ""
}

// --- groups ---

func (s *Store) Group(id string) (*Group, error) {
	var g Group
	err := s.DB.Get(&g, `SELECT * FROM groups WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoGroup
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// CreateGroup registers a group owned by pubkey with nothing bought yet. The
// owner is its first member.
func (s *Store) CreateGroup(id, name, owner string) (*Group, error) {
	tx, err := s.DB.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	if _, err := tx.Exec(
		`INSERT INTO groups (id, name, owner, status, created_at) VALUES (?, ?, ?, 'active', ?)`,
		id, name, owner, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`INSERT INTO group_members (pubkey, group_id, added_at) VALUES (?, ?, ?)
		 ON CONFLICT(pubkey) DO UPDATE SET group_id = excluded.group_id, added_at = excluded.added_at`,
		owner, id, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Group(id)
}

// ListGroups returns groups whose id, name, owner or note matches query (empty
// = all), newest first.
func (s *Store) ListGroups(query string, limit, offset int) ([]Group, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	groups := []Group{}
	like := "%" + query + "%"
	err := s.DB.Select(&groups,
		`SELECT * FROM groups
		 WHERE id LIKE ? OR name LIKE ? OR owner LIKE ? OR note LIKE ?
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`, like, like, like, like, limit, offset)
	return groups, err
}

// UpdateGroup applies an admin edit. expiresAt/quotaBytes are absolute values.
func (s *Store) UpdateGroup(id, name, owner, status string, expiresAt, quotaBytes int64, note string) error {
	res, err := s.DB.Exec(
		`UPDATE groups SET name = ?, owner = ?, status = ?, expires_at = ?, quota_bytes = ?, note = ?
		 WHERE id = ?`, name, owner, status, expiresAt, quotaBytes, note, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoGroup
	}
	return nil
}

// DeleteGroup removes the group and its membership rows. Usage rows keep
// pointing at the dead group id; RecomputeUsage ignores them, and the members
// fall back to their personal accounts.
func (s *Store) DeleteGroup(id string) error {
	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM group_members WHERE group_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM groups WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- members ---

// AddMember puts pubkey in a group, moving it out of any other group: one
// person belongs to at most one group so there is never a question about which
// pot a write is billed to.
func (s *Store) AddMember(groupID, pubkey string) error {
	if _, err := s.Group(groupID); err != nil {
		return err
	}
	_, err := s.DB.Exec(
		`INSERT INTO group_members (pubkey, group_id, added_at) VALUES (?, ?, ?)
		 ON CONFLICT(pubkey) DO UPDATE SET group_id = excluded.group_id, added_at = excluded.added_at`,
		pubkey, groupID, time.Now().Unix())
	return err
}

func (s *Store) RemoveMember(pubkey string) error {
	_, err := s.DB.Exec(`DELETE FROM group_members WHERE pubkey = ?`, pubkey)
	return err
}

func (s *Store) ListMembers(groupID string) ([]Member, error) {
	members := []Member{}
	err := s.DB.Select(&members,
		`SELECT * FROM group_members WHERE group_id = ? ORDER BY added_at`, groupID)
	return members, err
}

func (s *Store) CountMembers(groupID string) (int, error) {
	var n int
	err := s.DB.Get(&n, `SELECT count(*) FROM group_members WHERE group_id = ?`, groupID)
	return n, err
}

// GroupOf returns the group a pubkey belongs to, or ErrNoGroup.
func (s *Store) GroupOf(pubkey string) (*Group, error) {
	var g Group
	err := s.DB.Get(&g,
		`SELECT groups.* FROM groups
		 JOIN group_members ON group_members.group_id = groups.id
		 WHERE group_members.pubkey = ?`, pubkey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoGroup
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// --- allowance ---

// Allowance is everything that decides whether a pubkey may write: their own
// account and the group they belong to, either of which may be missing.
type Allowance struct {
	Account *Account
	Group   *Group
}

// Allowance loads both pots for a pubkey. A missing account or group is not an
// error — it is simply nil.
func (s *Store) Allowance(pubkey string) (*Allowance, error) {
	a := &Allowance{}

	acct, err := s.Account(pubkey)
	if err != nil && !errors.Is(err, ErrNoAccount) {
		return nil, err
	}
	a.Account = acct

	group, err := s.GroupOf(pubkey)
	if err != nil && !errors.Is(err, ErrNoGroup) {
		return nil, err
	}
	a.Group = group

	return a, nil
}

// CanWrite decides whether size bytes may be stored and which pot pays for it.
// The personal account goes first so a member who bought their own subscription
// spends it before the shared one. groupID is "" when the personal account pays.
func (a *Allowance) CanWrite(now, size int64) (ok bool, groupID string, msg string) {
	if a.Account != nil {
		ok, personalMsg := a.Account.CanWrite(now, size)
		if ok {
			return true, "", ""
		}
		msg = personalMsg
	}
	if a.Group != nil {
		ok, groupMsg := a.Group.CanSpend(now, size)
		if ok {
			return true, a.Group.ID, ""
		}
		if msg == "" {
			msg = groupMsg
		}
	}
	if msg == "" {
		msg = "restricted: not whitelisted"
	}
	return false, "", msg
}

// Whitelisted reports whether the pubkey is known to the relay at all, through
// an account or a group. Banned accounts and banned groups do not count.
func (a *Allowance) Whitelisted() bool {
	if a.Account != nil && a.Account.Status != StatusBanned {
		return true
	}
	return a.Group != nil && a.Group.Status != StatusBanned
}

// payer picks the pot a write should be billed to, without judging whether it
// is allowed: the gate already ran, and refusing to bill a stored event would
// let it be stored for free.
func (a *Allowance) payer(now, size int64) string {
	if ok, groupID, _ := a.CanWrite(now, size); ok {
		return groupID
	}
	// over budget already (a quota can be lowered under what is stored): bill
	// whichever pot exists, preferring the personal one.
	if a.Account == nil && a.Group != nil {
		return a.Group.ID
	}
	return ""
}
