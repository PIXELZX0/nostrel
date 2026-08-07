package store

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// NIP-05 identifiers sold under one of the relay's domains, either for a term
// (valid until expires_at) or outright (permanent, never expires).
//
// A name is taken while it resolves (expires_at in the future, or permanent),
// and also while somebody is paying for it (reserved_until in the future). The
// reservation is what stops two people buying the same name at once: whoever
// asks for the invoice first holds the name until that invoice expires.

var (
	ErrNoDomain    = errors.New("no such domain")
	ErrNameTaken   = errors.New("that name is already taken")
	ErrNameBlocked = errors.New("that name is not available")
)

// ModBlockedName is a moderation entry type: names an admin reserved so nobody
// can buy them.
const ModBlockedName = "blocked_name"

type Nip05Domain struct {
	Domain     string `db:"domain" json:"domain"`
	Enabled    bool   `db:"enabled" json:"enabled"`
	PriceSats  int64  `db:"price_sats" json:"price_sats"`
	PeriodDays int    `db:"period_days" json:"period_days"`
	Note       string `db:"note" json:"note"`
	CreatedAt  int64  `db:"created_at" json:"created_at"`
	// PermanentPriceSats is the one-off price of a name that never expires,
	// the NIP-05 side of the lifetime plan. 0 means this domain sells terms
	// only. The premium multiplier applies to it just the same.
	PermanentPriceSats int64 `db:"permanent_price_sats" json:"permanent_price_sats"`
}

type Nip05Name struct {
	Domain        string `db:"domain" json:"domain"`
	Name          string `db:"name" json:"name"`
	Pubkey        string `db:"pubkey" json:"pubkey"`
	ExpiresAt     int64  `db:"expires_at" json:"expires_at"`
	ReservedUntil int64  `db:"reserved_until" json:"reserved_until"`
	Permanent     bool   `db:"permanent" json:"permanent"`
	CreatedAt     int64  `db:"created_at" json:"created_at"`
}

// Live reports whether the name currently resolves.
func (n *Nip05Name) Live(now int64) bool {
	return n.Permanent || n.ExpiresAt > now
}

// held reports whether the name is unavailable to anyone else.
func (n *Nip05Name) held(now int64) bool {
	return n.Live(now) || n.ReservedUntil > now
}

// --- domains ---

func (s *Store) Nip05Domain(domain string) (*Nip05Domain, error) {
	var d Nip05Domain
	err := s.DB.Get(&d, `SELECT * FROM nip05_domains WHERE domain = ?`, strings.ToLower(domain))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoDomain
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) ListNip05Domains(enabledOnly bool) ([]Nip05Domain, error) {
	domains := []Nip05Domain{}
	query := `SELECT * FROM nip05_domains ORDER BY domain`
	if enabledOnly {
		query = `SELECT * FROM nip05_domains WHERE enabled = 1 ORDER BY domain`
	}
	err := s.DB.Select(&domains, query)
	return domains, err
}

// SaveNip05Domain creates or updates a domain.
func (s *Store) SaveNip05Domain(d Nip05Domain) error {
	_, err := s.DB.Exec(
		`INSERT INTO nip05_domains (domain, enabled, price_sats, period_days, note, created_at, permanent_price_sats)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(domain) DO UPDATE SET
		   enabled = excluded.enabled, price_sats = excluded.price_sats,
		   period_days = excluded.period_days, note = excluded.note,
		   permanent_price_sats = excluded.permanent_price_sats`,
		strings.ToLower(d.Domain), d.Enabled, d.PriceSats, d.PeriodDays, d.Note,
		time.Now().Unix(), d.PermanentPriceSats)
	return err
}

// DeleteNip05Domain drops a domain and every name under it.
func (s *Store) DeleteNip05Domain(domain string) error {
	domain = strings.ToLower(domain)
	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM nip05_names WHERE domain = ?`, domain); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM nip05_domains WHERE domain = ?`, domain); err != nil {
		return err
	}
	return tx.Commit()
}

// --- names ---

// ResolveNip05 returns the pubkey behind name@domain, or "" when nothing
// resolves. This is what /.well-known/nostr.json answers with.
func (s *Store) ResolveNip05(domain, name string) (string, error) {
	var pubkey string
	err := s.DB.Get(&pubkey,
		`SELECT pubkey FROM nip05_names
		 WHERE domain = ? AND name = ? AND (permanent = 1 OR expires_at > ?)`,
		strings.ToLower(domain), strings.ToLower(name), time.Now().Unix())
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return pubkey, err
}

func (s *Store) Nip05Name(domain, name string) (*Nip05Name, error) {
	var n Nip05Name
	err := s.DB.Get(&n, `SELECT * FROM nip05_names WHERE domain = ? AND name = ?`,
		strings.ToLower(domain), strings.ToLower(name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// ListNip05Names lists sold names for the admin panel, newest first. domain ""
// means every domain; query matches the name or the pubkey.
func (s *Store) ListNip05Names(query, domain string, limit, offset int) ([]Nip05Name, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	names := []Nip05Name{}
	like := "%" + strings.ToLower(query) + "%"
	sqlText := `SELECT * FROM nip05_names WHERE (name LIKE ? OR pubkey LIKE ?)`
	args := []any{like, like}
	if domain != "" {
		sqlText += ` AND domain = ?`
		args = append(args, strings.ToLower(domain))
	}
	sqlText += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	err := s.DB.Select(&names, sqlText, args...)
	return names, err
}

// Nip05Available reports whether pubkey may buy name@domain. Renewing a name
// you already hold is allowed; a name somebody else holds is not.
func (s *Store) Nip05Available(domain, name, pubkey string) error {
	if s.ModHas(ModBlockedName, strings.ToLower(name)) {
		return ErrNameBlocked
	}
	existing, err := s.Nip05Name(domain, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !existing.held(time.Now().Unix()) {
		return nil
	}
	if existing.Pubkey == pubkey {
		return nil // renewal
	}
	return ErrNameTaken
}

// ReserveNip05 holds name@domain for pubkey until an invoice is paid or lapses.
// It is the availability check and the claim in one statement, so two buyers
// racing for the same name cannot both win.
func (s *Store) ReserveNip05(domain, name, pubkey string, until int64) error {
	domain, name = strings.ToLower(domain), strings.ToLower(name)
	if s.ModHas(ModBlockedName, name) {
		return ErrNameBlocked
	}

	// One statement does the check and the claim: a losing racer's update
	// matches no rows instead of overwriting the winner.
	now := time.Now().Unix()
	res, err := s.DB.Exec(
		`INSERT INTO nip05_names (domain, name, pubkey, expires_at, reserved_until, permanent, created_at)
		 VALUES (?, ?, ?, 0, ?, 0, ?)
		 ON CONFLICT(domain, name) DO UPDATE SET
		   pubkey = excluded.pubkey,
		   reserved_until = excluded.reserved_until,
		   -- taking over a lapsed name starts a fresh term; a renewal keeps its own
		   expires_at = CASE WHEN nip05_names.pubkey = excluded.pubkey
		                     THEN nip05_names.expires_at ELSE 0 END
		 WHERE nip05_names.pubkey = excluded.pubkey
		    OR (nip05_names.permanent = 0
		        AND nip05_names.expires_at <= ?
		        AND nip05_names.reserved_until <= ?)`,
		domain, name, pubkey, until, now, now, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNameTaken
	}
	return nil
}

// ReleaseNip05 drops an unpaid reservation, used when the invoice could not be
// created after all.
func (s *Store) ReleaseNip05(domain, name, pubkey string) error {
	_, err := s.DB.Exec(
		`DELETE FROM nip05_names
		 WHERE domain = ? AND name = ? AND pubkey = ? AND permanent = 0 AND expires_at = 0`,
		strings.ToLower(domain), strings.ToLower(name), pubkey)
	return err
}

// SaveNip05Name is the admin path: hand a name to a pubkey, or edit one.
func (s *Store) SaveNip05Name(n Nip05Name) error {
	_, err := s.DB.Exec(
		`INSERT INTO nip05_names (domain, name, pubkey, expires_at, reserved_until, permanent, created_at)
		 VALUES (?, ?, ?, ?, 0, ?, ?)
		 ON CONFLICT(domain, name) DO UPDATE SET
		   pubkey = excluded.pubkey, expires_at = excluded.expires_at,
		   permanent = excluded.permanent, reserved_until = 0`,
		strings.ToLower(n.Domain), strings.ToLower(n.Name), n.Pubkey,
		n.ExpiresAt, n.Permanent, time.Now().Unix())
	return err
}

func (s *Store) DeleteNip05Name(domain, name string) error {
	_, err := s.DB.Exec(`DELETE FROM nip05_names WHERE domain = ? AND name = ?`,
		strings.ToLower(domain), strings.ToLower(name))
	return err
}

// ListNip05NamesOf returns the names a pubkey holds, unpaid reservations
// excluded.
func (s *Store) ListNip05NamesOf(pubkey string) ([]Nip05Name, error) {
	names := []Nip05Name{}
	err := s.DB.Select(&names,
		`SELECT * FROM nip05_names
		 WHERE pubkey = ? AND (permanent = 1 OR expires_at > ?)
		 ORDER BY domain, name`, pubkey, time.Now().Unix())
	return names, err
}

// Nip05PubkeysFor lists the pubkeys holding a live name under a domain. It
// backs NIP-50's `domain:` search extension, which this relay can answer from
// its own records instead of trusting profile metadata.
func (s *Store) Nip05PubkeysFor(domain string, limit int) ([]string, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	pubkeys := []string{}
	err := s.DB.Select(&pubkeys,
		`SELECT DISTINCT pubkey FROM nip05_names
		 WHERE domain = ? AND (permanent = 1 OR expires_at > ?) LIMIT ?`,
		strings.ToLower(domain), time.Now().Unix(), limit)
	return pubkeys, err
}

func (s *Store) CountNip05Names() (int, error) {
	var n int
	err := s.DB.Get(&n,
		`SELECT count(*) FROM nip05_names WHERE permanent = 1 OR expires_at > ?`, time.Now().Unix())
	return n, err
}

// PurgeNip05Names deletes rows that neither resolve nor hold a reservation:
// lapsed names and abandoned checkouts.
func (s *Store) PurgeNip05Names() (int64, error) {
	now := time.Now().Unix()
	res, err := s.DB.Exec(
		`DELETE FROM nip05_names WHERE permanent = 0 AND expires_at <= ? AND reserved_until <= ?`,
		now, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- premium pricing ---

// PremiumTier makes short names cost more: a name of at most MaxLen characters
// is charged Multiplier times the domain price.
type PremiumTier struct {
	MaxLen     int `json:"max_len"`
	Multiplier int `json:"multiplier"`
}

// ParsePremiumTiers reads the panel's compact form, "1:20,2:10,3:5" — one
// character costs twenty times the base price, two characters ten, three five.
// An empty string means no premium at all.
func ParsePremiumTiers(spec string) ([]PremiumTier, error) {
	tiers := []PremiumTier{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		length, multiplier, ok := strings.Cut(part, ":")
		if !ok {
			return nil, errors.New("premium tiers look like \"1:20,2:10\" (length:multiplier)")
		}
		maxLen, err := strconv.Atoi(strings.TrimSpace(length))
		if err != nil || maxLen < 1 {
			return nil, errors.New("premium tier length must be a positive number: " + part)
		}
		mult, err := strconv.Atoi(strings.TrimSpace(multiplier))
		if err != nil || mult < 1 {
			return nil, errors.New("premium tier multiplier must be at least 1: " + part)
		}
		tiers = append(tiers, PremiumTier{MaxLen: maxLen, Multiplier: mult})
	}
	return tiers, nil
}

// PremiumMultiplier returns what name costs relative to the domain price. The
// tightest matching tier wins, so "1:20,3:5" charges 20x for one character and
// 5x for two or three.
func PremiumMultiplier(tiers []PremiumTier, name string) int {
	best, bestLen := 1, 0
	for _, t := range tiers {
		if len([]rune(name)) > t.MaxLen {
			continue
		}
		if bestLen == 0 || t.MaxLen < bestLen {
			best, bestLen = t.Multiplier, t.MaxLen
		}
	}
	return best
}
