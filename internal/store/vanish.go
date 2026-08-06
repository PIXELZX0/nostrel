package store

import (
	"database/sql"
	"errors"
	"time"
)

// NIP-62: a user can ask a relay to forget them entirely. Paid relays are not
// exempt — the request has to be honoured whether or not they are a customer.
//
// "Forget" has to survive a re-upload, so the cutoff is remembered: anything
// the pubkey wrote at or before the moment they asked stays refused forever.
// Newer events are fine, because the user may keep using the relay afterwards.

// Vanished is a pubkey that asked to be forgotten.
type Vanished struct {
	Pubkey     string `db:"pubkey" json:"pubkey"`
	VanishedAt int64  `db:"vanished_at" json:"vanished_at"` // cutoff: events at or before are refused
	Reason     string `db:"reason" json:"reason"`
	CreatedAt  int64  `db:"created_at" json:"created_at"`
}

// VanishedAt returns the cutoff for a pubkey, or 0 when they never asked.
func (s *Store) VanishedAt(pubkey string) int64 {
	var cutoff int64
	err := s.DB.Get(&cutoff, `SELECT vanished_at FROM vanished WHERE pubkey = ?`, pubkey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// failing open would let deleted events back in, so treat an unreadable
		// table as "everything from this pubkey is refused"
		return time.Now().Unix()
	}
	return cutoff
}

func (s *Store) ListVanished(limit int) ([]Vanished, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows := []Vanished{}
	err := s.DB.Select(&rows, `SELECT * FROM vanished ORDER BY created_at DESC LIMIT ?`, limit)
	return rows, err
}

// Vanish deletes everything a pubkey wrote up to upTo and records the cutoff so
// the same events cannot be published again. It returns how many events went.
//
// Blobs are not touched here: their bytes may be shared with other uploaders,
// so ownership is unwound separately (see blobs.Index.Forget).
func (s *Store) Vanish(pubkey string, upTo int64, reason string) (int64, error) {
	tx, err := s.DB.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	if _, err := tx.Exec(
		`INSERT INTO vanished (pubkey, vanished_at, reason, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(pubkey) DO UPDATE SET
		   vanished_at = max(vanished.vanished_at, excluded.vanished_at),
		   reason = excluded.reason`,
		pubkey, upTo, reason, now); err != nil {
		return 0, err
	}

	res, err := tx.Exec(`DELETE FROM event WHERE pubkey = ? AND created_at <= ?`, pubkey, upTo)
	if err != nil {
		return 0, err
	}
	deleted, _ := res.RowsAffected()

	// the storage those events held goes back to the account
	if _, err := tx.Exec(
		`DELETE FROM usage_events
		 WHERE pubkey = ? AND id NOT LIKE ? AND id NOT IN (SELECT id FROM event WHERE pubkey = ?)`,
		pubkey, BlobUsagePrefix+"%", pubkey); err != nil {
		return 0, err
	}
	if err := recomputeUsage(tx, pubkey); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// Unvanish lifts the cutoff. NIP-62 says nobody is obliged to offer this, but
// an admin needs a way to undo a mistake; the deleted events stay deleted.
func (s *Store) Unvanish(pubkey string) error {
	_, err := s.DB.Exec(`DELETE FROM vanished WHERE pubkey = ?`, pubkey)
	return err
}
