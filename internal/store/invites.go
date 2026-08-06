package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Invite codes are how somebody joins without paying: a beta invitation, a
// friend's referral, a conference coupon. They sit beside the lightning
// checkout rather than replacing it — NIP-43 is neutral about pricing.

var (
	ErrInviteUnknown     = errors.New("that is an invalid invite code")
	ErrInviteExpired     = errors.New("that invite code is expired")
	ErrInviteExhausted   = errors.New("that invite code has been used up")
	ErrInviteAlreadyUsed = errors.New("you already used that invite code")
)

type Invite struct {
	Code       string `db:"code" json:"code"`
	Note       string `db:"note" json:"note"`
	PeriodDays int    `db:"period_days" json:"period_days"`
	QuotaMB    int    `db:"quota_mb" json:"quota_mb"`
	MaxUses    int    `db:"max_uses" json:"max_uses"` // 0 = unlimited
	Used       int    `db:"used" json:"used"`
	ExpiresAt  int64  `db:"expires_at" json:"expires_at"` // 0 = never
	CreatedAt  int64  `db:"created_at" json:"created_at"`
}

// NewInviteCode returns a short, unguessable, case-insensitive code.
func NewInviteCode() string {
	buf := make([]byte, 8)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

func normalizeCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func (s *Store) CreateInvite(inv Invite) (*Invite, error) {
	if inv.Code == "" {
		inv.Code = NewInviteCode()
	}
	inv.Code = normalizeCode(inv.Code)
	_, err := s.DB.Exec(
		`INSERT INTO invites (code, note, period_days, quota_mb, max_uses, used, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		inv.Code, inv.Note, inv.PeriodDays, inv.QuotaMB, inv.MaxUses, inv.ExpiresAt, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return s.Invite(inv.Code)
}

func (s *Store) Invite(code string) (*Invite, error) {
	var inv Invite
	err := s.DB.Get(&inv, `SELECT * FROM invites WHERE code = ?`, normalizeCode(code))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteUnknown
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *Store) ListInvites(limit int) ([]Invite, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	invites := []Invite{}
	err := s.DB.Select(&invites, `SELECT * FROM invites ORDER BY created_at DESC LIMIT ?`, limit)
	return invites, err
}

func (s *Store) DeleteInvite(code string) error {
	_, err := s.DB.Exec(`DELETE FROM invites WHERE code = ?`, normalizeCode(code))
	return err
}

// ClaimInvite spends one use of a code for a pubkey. Checking and spending
// happen in one transaction, so two people racing for the last seat cannot both
// win, and one person cannot burn a multi-use code by claiming it twice.
//
// The returned invite says what the code grants; the caller applies it.
func (s *Store) ClaimInvite(code, pubkey string) (*Invite, error) {
	code = normalizeCode(code)

	tx, err := s.DB.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var inv Invite
	err = tx.Get(&inv, `SELECT * FROM invites WHERE code = ?`, code)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteUnknown
	}
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	if inv.ExpiresAt != 0 && inv.ExpiresAt <= now {
		return nil, ErrInviteExpired
	}

	// Claiming again is checked before the cap so somebody who already used a
	// code is told that, rather than the less useful "it is used up".
	res, err := tx.Exec(
		`INSERT INTO invite_uses (code, pubkey, used_at) VALUES (?, ?, ?)
		 ON CONFLICT(code, pubkey) DO NOTHING`, code, pubkey, now)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrInviteAlreadyUsed
	}
	if inv.MaxUses > 0 && inv.Used >= inv.MaxUses {
		return nil, ErrInviteExhausted
	}

	// the WHERE repeats the cap so a concurrent claim cannot push us over it
	res, err = tx.Exec(
		`UPDATE invites SET used = used + 1
		 WHERE code = ? AND (max_uses = 0 OR used < max_uses)`, code)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrInviteExhausted
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	inv.Used++
	return &inv, nil
}

// InviteUsedBy reports whether a pubkey already claimed this code.
func (s *Store) InviteUsedBy(code, pubkey string) bool {
	var n int
	err := s.DB.Get(&n, `SELECT count(*) FROM invite_uses WHERE code = ? AND pubkey = ?`,
		normalizeCode(code), pubkey)
	return err == nil && n > 0
}
