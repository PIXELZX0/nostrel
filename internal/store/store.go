// Package store owns the SQLite database: the event store (via
// eventstore/sqlite3) plus our own accounts / payments / usage tables, all in
// the same file so a backup is a single copy.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/jmoiron/sqlx"
)

var ErrNoAccount = errors.New("no such account")

const (
	StatusActive = "active"
	StatusBanned = "banned"

	KindAdmission    = "admission"
	KindSubscription = "subscription"
	KindLifetime     = "lifetime"
	KindStorage      = "storage"
	KindNip05        = "nip05"
	KindGroup        = "group"

	PayPending = "pending"
	PayPaid    = "paid"
	PayExpired = "expired"

	MB = int64(1024 * 1024)

	// BlobUsagePrefix namespaces uploaded files in usage_events so their rows
	// cannot collide with event ids — and so bookkeeping that walks the event
	// table knows to leave them alone.
	BlobUsagePrefix = "blob:"
)

type Store struct {
	Events *sqlite3.SQLite3Backend
	DB     *sqlx.DB
}

type Account struct {
	Pubkey     string `db:"pubkey" json:"pubkey"`
	Status     string `db:"status" json:"status"`
	ExpiresAt  int64  `db:"expires_at" json:"expires_at"` // unix seconds; 0 = never expires
	QuotaBytes int64  `db:"quota_bytes" json:"quota_bytes"`
	UsedBytes  int64  `db:"used_bytes" json:"used_bytes"`
	CreatedAt  int64  `db:"created_at" json:"created_at"`
	Note       string `db:"note" json:"note"`
	// Permanent is the lifetime plan: the quota was bought outright, so the
	// account never expires and there is nothing to renew. It is kept separate
	// from expires_at = 0 because that is also what a brand new row reads.
	Permanent bool `db:"permanent" json:"permanent"`
}

type Payment struct {
	ID          string `db:"id" json:"id"`
	Pubkey      string `db:"pubkey" json:"pubkey"`
	Kind        string `db:"kind" json:"kind"`
	Sats        int64  `db:"sats" json:"sats"`
	Provider    string `db:"provider" json:"provider"`
	PaymentHash string `db:"payment_hash" json:"payment_hash"`
	Bolt11      string `db:"bolt11" json:"bolt11"`
	Status      string `db:"status" json:"status"`
	Meta        string `db:"meta" json:"meta"` // JSON: {"period_days":30,"mb":100,"admission":true}
	CreatedAt   int64  `db:"created_at" json:"created_at"`
	PaidAt      int64  `db:"paid_at" json:"paid_at"`
}

// PaymentMeta is what gets stored in Payment.Meta: the entitlements to grant
// when the invoice is settled.
type PaymentMeta struct {
	Admission  bool `json:"admission,omitempty"`
	PeriodDays int  `json:"period_days,omitempty"`
	MB         int  `json:"mb,omitempty"`

	// Permanent turns the account (or group) this invoice credits onto the
	// lifetime plan: the megabytes above are kept forever and nothing expires.
	Permanent bool `json:"permanent,omitempty"`

	// a NIP-05 identifier bought with this invoice; Nip05Permanent means it was
	// bought outright, in which case Nip05Days is zero
	Nip05Domain    string `json:"nip05_domain,omitempty"`
	Nip05Name      string `json:"nip05_name,omitempty"`
	Nip05Days      int    `json:"nip05_days,omitempty"`
	Nip05Permanent bool   `json:"nip05_permanent,omitempty"`

	// a shared storage group topped up by this invoice; when set, PeriodDays
	// and MB go to the group instead of the buyer's own account
	GroupID string `json:"group_id,omitempty"`
}

// Settings holds panel-editable relay configuration and the price list.
// Stored as a single JSON row so changes are atomic and schema-free.
type Settings struct {
	RelayName        string `json:"relay_name"`
	RelayDescription string `json:"relay_description"`
	RelayIcon        string `json:"relay_icon"`
	RelayBanner      string `json:"relay_banner"`
	RelayContact     string `json:"relay_contact"`
	// RelayPubkey is the operator's pubkey in NIP-11. Empty falls back to the
	// pubkey of RELAY_SECRET_KEY, when there is one.
	RelayPubkey string `json:"relay_pubkey"`

	// Panel look, applied by the browser as CSS variables. RelayIcon doubles as
	// the logo. Colours are "#rrggbb"; empty means the stylesheet's own value.
	// The rest of the palette (surfaces, borders, text) is derived from
	// ThemeBgColor so a light background still reads.
	ThemeBgColor string `json:"theme_bg_color"`
	ThemeAccent  string `json:"theme_accent"`
	ThemeBgImage string `json:"theme_bg_image"`

	// The rest of NIP-11's descriptive fields, which relay directories index
	// and a paying customer expects to be able to read. The list-shaped ones
	// are comma separated in the panel.
	RelayRetentionDays int    `json:"relay_retention_days"` // 0 = kept indefinitely
	RelayPostingPolicy string `json:"relay_posting_policy"` // URL; defaults to the panel
	RelayCountries     string `json:"relay_countries"`      // ISO 3166-1 alpha-2
	RelayLanguages     string `json:"relay_languages"`      // ISO 639-1
	RelayTopics        string `json:"relay_topics"`

	// NsiteDomains host NIP-5A static websites at <nip05 name>.<domain>, comma
	// separated. The name is one sold here, so a customer's site address falls
	// out of the identifier they already bought.
	NsiteDomains string `json:"nsite_domains,omitempty"`

	// AutoInvite hands out a fresh single-use code to anyone who asks for one
	// over NIP-43 (kind 28935). Off means codes only come from an admin, which
	// is the point of an invite on a paid relay — turn it on for an open beta.
	AutoInvite           bool `json:"auto_invite"`
	AutoInvitePeriodDays int  `json:"auto_invite_period_days"`
	AutoInviteQuotaMB    int  `json:"auto_invite_quota_mb"`

	AdmissionSats int64 `json:"admission_sats"`

	// Two plans, either of which the operator may switch off. Subscription
	// rents access by the period and includes storage with it; lifetime sells
	// storage outright, once, and the account never expires afterwards. The
	// admission fee is charged either way, to a pubkey with no account yet.
	SubscriptionEnabled bool  `json:"subscription_enabled"`
	SubscriptionSats    int64 `json:"subscription_sats"`
	PeriodDays          int   `json:"period_days"`
	IncludedMB          int   `json:"included_mb"`
	PricePerMBSats      int64 `json:"price_per_mb_sats"`

	LifetimeEnabled        bool  `json:"lifetime_enabled"`
	LifetimePricePerMBSats int64 `json:"lifetime_price_per_mb_sats"`

	// Short NIP-05 names cost a multiple of the domain price. Written as
	// "1:20,2:10,3:5" — see ParsePremiumTiers. Empty means no premium.
	Nip05PremiumTiers string `json:"nip05_premium_tiers,omitempty"`

	// Relays the panel offers a remote signer for NIP-46 login, comma
	// separated. It cannot be this relay: signing happens before the user has
	// an account, and unknown pubkeys are not allowed to publish here.
	Nip46Relays string `json:"nip46_relays,omitempty"`

	// Kind policy (NIP-86). An allow list, when non-empty, wins over everything
	// else; otherwise anything not in the block list is accepted.
	AllowedKinds    []int `json:"allowed_kinds,omitempty"`
	DisallowedKinds []int `json:"disallowed_kinds,omitempty"`

	// AcceptZapReceipts and AcceptBadgeAwards let events written *about* a
	// customer by somebody without an account through: NIP-57 receipts (kind
	// 9735) from the LNURL server, and NIP-58 awards (kind 8) from the badge
	// issuer. They are billed to the customer they name, so a stranger can
	// spend that customer's quota: off by default, and abusive authors are
	// banned through NIP-86.
	AcceptZapReceipts bool `json:"accept_zap_receipts"`
	AcceptBadgeAwards bool `json:"accept_badge_awards"`

	// AcceptDirectMessages does the same for a message addressed to a customer
	// by somebody without an account: a NIP-04 kind 4, or the NIP-59 gift wrap
	// (kind 1059) NIP-17 private chat is built on. A wrap is always signed by a
	// throwaway key, so turning this off stops NIP-17 dead — including between
	// two customers. On by default; the quota it spends is the recipient's, so
	// a spammer is banned through NIP-86 like any other.
	AcceptDirectMessages bool `json:"accept_direct_messages"`

	// ReadAuthRequired makes readers authenticate (NIP-42) and be whitelisted
	// too, turning the relay private in both directions.
	ReadAuthRequired bool `json:"read_auth_required"`

	// MinPoW is the NIP-13 difficulty an event must commit to. 0 disables it.
	MinPoW int `json:"min_pow"`
	// CreatedAtMaxPast/Future bound created_at, in seconds (0 = unbounded).
	// Advertised through the NIP-11 created_at limits.
	CreatedAtMaxPast   int `json:"created_at_max_past"`
	CreatedAtMaxFuture int `json:"created_at_max_future"`

	// ExtraAdmins are pubkeys granted admin rights at runtime via NIP-86
	// grantadmin, on top of the ADMIN_PUBKEYS environment variable.
	ExtraAdmins []string `json:"extra_admins,omitempty"`

	// Lightning backend, editable from the admin panel. Seeded from the
	// environment on first run. The key and the connection URI are secrets:
	// never hand these to a client, use Redacted().
	PaymentProvider  string `json:"payment_provider"`
	LNbitsURL        string `json:"lnbits_url"`
	LNbitsInvoiceKey string `json:"lnbits_invoice_key"`
	NWCURI           string `json:"nwc_uri"`

	// Where uploaded media is stored: "local" (a directory on this machine) or
	// "s3" (any S3-compatible bucket). S3SecretKey is a secret.
	StorageBackend string `json:"storage_backend"`
	LocalPath      string `json:"local_path"`
	S3Endpoint     string `json:"s3_endpoint"`
	S3Region       string `json:"s3_region"`
	S3Bucket       string `json:"s3_bucket"`
	S3Prefix       string `json:"s3_prefix"`
	S3AccessKey    string `json:"s3_access_key"`
	S3SecretKey    string `json:"s3_secret_key"`
	// S3PublicURL, when set, is the address clients are redirected to for
	// downloads so the bytes never pass through the relay.
	S3PublicURL string `json:"s3_public_url"`
	// MaxBlobSizeMB caps a single upload.
	MaxBlobSizeMB int `json:"max_blob_size_mb"`
}

// Redacted returns a copy safe to send to a browser: secrets become a short
// fingerprint so an admin can tell whether something is configured, and which
// value it is, without the value leaving the server.
func (s Settings) Redacted() Settings {
	s.LNbitsInvoiceKey = mask(s.LNbitsInvoiceKey)
	s.NWCURI = mask(s.NWCURI)
	s.S3SecretKey = mask(s.S3SecretKey)
	return s
}

// MaskedValue is what the panel sends back for a secret it didn't change.
const MaskedValue = "••••"

func mask(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return MaskedValue
	}
	return MaskedValue + secret[len(secret)-4:]
}

// IsMasked reports whether a submitted value is the redacted form of an
// existing secret rather than a new one.
func IsMasked(value string) bool {
	return strings.HasPrefix(value, MaskedValue)
}

// PaymentFingerprint changes whenever the lightning backend configuration
// changes, which is how a running relay notices it must rebuild its provider.
func (s Settings) PaymentFingerprint() string {
	return strings.Join([]string{s.PaymentProvider, s.LNbitsURL, s.LNbitsInvoiceKey, s.NWCURI}, "\x00")
}

// StorageFingerprint does the same for the media storage backend.
func (s Settings) StorageFingerprint() string {
	return strings.Join([]string{
		s.StorageBackend, s.LocalPath, s.S3Endpoint, s.S3Region,
		s.S3Bucket, s.S3Prefix, s.S3AccessKey, s.S3SecretKey, s.S3PublicURL,
	}, "\x00")
}

// MaxBlobSize is the upload cap in bytes.
func (s Settings) MaxBlobSize() int64 { return int64(s.MaxBlobSizeMB) << 20 }

// KindAllowed applies the kind policy.
func (s Settings) KindAllowed(kind int) bool {
	if len(s.AllowedKinds) > 0 {
		return slices.Contains(s.AllowedKinds, kind)
	}
	return !slices.Contains(s.DisallowedKinds, kind)
}

// DefaultSettings is what a relay runs on before an admin has touched the
// panel. Settings() layers the stored row on top of it, so a field added in a
// later release gets its default on an existing database too.
func DefaultSettings() Settings {
	return Settings{
		RelayName:        "nostrel",
		RelayDescription: "a paid whitelist relay",
		AdmissionSats: 1000,
		// the subscription is the plan a fresh relay sells; lifetime is opt-in,
		// priced so buying it outright costs about two years of the same storage
		SubscriptionEnabled:    true,
		SubscriptionSats:       2000,
		PeriodDays:             30,
		IncludedMB:             100,
		PricePerMBSats:         10,
		LifetimeEnabled:        false,
		LifetimePricePerMBSats: 500,
		Nip46Relays:            "wss://relay.nsec.app,wss://nos.lol",
		ThemeBgColor:     "#121417",
		ThemeAccent:      "#f7931a",
		// private chat works out of the box; see AcceptDirectMessages
		AcceptDirectMessages: true,
		// mock settles its own invoices: harmless for a relay nobody has
		// configured yet, and main() warns loudly about it on every start.
		// LocalPath is deliberately left empty: that means DefaultBlobPath, the
		// directory the caller of blobs.New passes in.
		PaymentProvider: "mock",
		StorageBackend:  "local",
		MaxBlobSizeMB:   25,
		// 15 minutes: enough for a client with a wrong clock, not enough to
		// backdate anything meaningfully.
		CreatedAtMaxFuture: 900,
	}
}

// DefaultBlobPath is where uploads land until an admin points storage
// somewhere else. Relative to the working directory (/data in the image).
const DefaultBlobPath = "./blobs"

var ddls = []string{
	`CREATE TABLE IF NOT EXISTS accounts (
		pubkey      TEXT PRIMARY KEY,
		status      TEXT NOT NULL DEFAULT 'active',
		expires_at  INTEGER NOT NULL DEFAULT 0,
		quota_bytes INTEGER NOT NULL DEFAULT 0,
		used_bytes  INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL,
		note        TEXT NOT NULL DEFAULT '',
		permanent   INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS payments (
		id           TEXT PRIMARY KEY,
		pubkey       TEXT NOT NULL,
		kind         TEXT NOT NULL,
		sats         INTEGER NOT NULL,
		provider     TEXT NOT NULL,
		payment_hash TEXT NOT NULL UNIQUE,
		bolt11       TEXT NOT NULL DEFAULT '',
		status       TEXT NOT NULL DEFAULT 'pending',
		meta         TEXT NOT NULL DEFAULT '{}',
		created_at   INTEGER NOT NULL,
		paid_at      INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS payments_status ON payments(status, created_at)`,
	`CREATE INDEX IF NOT EXISTS payments_pubkey ON payments(pubkey, created_at DESC)`,
	// group_id names the pot the bytes were billed to: empty for the author's
	// own account, otherwise a shared group.
	`CREATE TABLE IF NOT EXISTS usage_events (
		id         TEXT PRIMARY KEY,
		pubkey     TEXT NOT NULL,
		size       INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		group_id   TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS usage_pubkey ON usage_events(pubkey)`,
	`CREATE TABLE IF NOT EXISTS groups (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL DEFAULT '',
		owner       TEXT NOT NULL,
		status      TEXT NOT NULL DEFAULT 'active',
		expires_at  INTEGER NOT NULL DEFAULT 0,
		quota_bytes INTEGER NOT NULL DEFAULT 0,
		used_bytes  INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL,
		note        TEXT NOT NULL DEFAULT '',
		permanent   INTEGER NOT NULL DEFAULT 0
	)`,
	// one pubkey belongs to at most one group, so there is never a question
	// about which pot a write is billed to
	`CREATE TABLE IF NOT EXISTS group_members (
		pubkey   TEXT PRIMARY KEY,
		group_id TEXT NOT NULL,
		added_at INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS group_members_group ON group_members(group_id)`,
	`CREATE TABLE IF NOT EXISTS nip05_domains (
		domain      TEXT PRIMARY KEY,
		enabled     INTEGER NOT NULL DEFAULT 1,
		price_sats  INTEGER NOT NULL DEFAULT 0,
		period_days INTEGER NOT NULL DEFAULT 365,
		note        TEXT NOT NULL DEFAULT '',
		created_at  INTEGER NOT NULL,
		-- 0 means this domain does not sell permanent names at all
		permanent_price_sats INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS nip05_names (
		domain         TEXT NOT NULL,
		name           TEXT NOT NULL,
		pubkey         TEXT NOT NULL,
		expires_at     INTEGER NOT NULL DEFAULT 0,
		reserved_until INTEGER NOT NULL DEFAULT 0,
		permanent      INTEGER NOT NULL DEFAULT 0,
		created_at     INTEGER NOT NULL,
		PRIMARY KEY (domain, name)
	)`,
	`CREATE INDEX IF NOT EXISTS nip05_names_pubkey ON nip05_names(pubkey)`,
	// NIP-43 invite codes: access granted without a payment
	`CREATE TABLE IF NOT EXISTS invites (
		code        TEXT PRIMARY KEY,
		note        TEXT NOT NULL DEFAULT '',
		period_days INTEGER NOT NULL DEFAULT 0,
		quota_mb    INTEGER NOT NULL DEFAULT 0,
		max_uses    INTEGER NOT NULL DEFAULT 1,
		used        INTEGER NOT NULL DEFAULT 0,
		expires_at  INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL
	)`,
	// one row per (code, pubkey) so a single person cannot burn a multi-use code
	`CREATE TABLE IF NOT EXISTS invite_uses (
		code    TEXT NOT NULL,
		pubkey  TEXT NOT NULL,
		used_at INTEGER NOT NULL,
		PRIMARY KEY (code, pubkey)
	)`,
	// NIP-62: pubkeys that asked to be forgotten, and the cutoff that keeps
	// their old events from coming back
	`CREATE TABLE IF NOT EXISTS vanished (
		pubkey      TEXT PRIMARY KEY,
		vanished_at INTEGER NOT NULL,
		reason      TEXT NOT NULL DEFAULT '',
		created_at  INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	// moderation decisions made through NIP-86: banned/allowed event ids and
	// blocked IPs, one row per (type, value)
	`CREATE TABLE IF NOT EXISTS moderation (
		type       TEXT NOT NULL,
		value      TEXT NOT NULL,
		reason     TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		PRIMARY KEY (type, value)
	)`,
}

// Open initialises the event store and our tables in the file at path.
func Open(path string) (*Store, error) {
	events := &sqlite3.SQLite3Backend{DatabaseURL: dsn(path)}
	if err := events.Init(); err != nil {
		return nil, fmt.Errorf("opening event store: %w", err)
	}
	s := &Store{Events: events, DB: events.DB}
	for _, ddl := range ddls {
		if _, err := s.DB.Exec(ddl); err != nil {
			events.Close()
			return nil, fmt.Errorf("schema: %w", err)
		}
	}
	// columns added to tables that already existed before the feature landed
	for _, c := range [][3]string{
		{"usage_events", "group_id", "TEXT NOT NULL DEFAULT ''"},
		{"accounts", "permanent", "INTEGER NOT NULL DEFAULT 0"},
		{"groups", "permanent", "INTEGER NOT NULL DEFAULT 0"},
		{"nip05_domains", "permanent_price_sats", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.addColumn(c[0], c[1], c[2]); err != nil {
			events.Close()
			return nil, fmt.Errorf("schema: %w", err)
		}
	}
	return s, nil
}

// addColumn is the whole migration story: CREATE TABLE IF NOT EXISTS covers new
// installs, and this covers databases created before a column existed. ALTER
// TABLE has no IF NOT EXISTS, so the column list is checked first.
func (s *Store) addColumn(table, column, definition string) error {
	var columns []struct {
		Name string `db:"name"`
	}
	if err := s.DB.Select(&columns, `SELECT name FROM pragma_table_info(?)`, table); err != nil {
		return err
	}
	for _, c := range columns {
		if c.Name == column {
			return nil
		}
	}
	_, err := s.DB.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

// dsn adds WAL + a busy timeout so relay writes and panel writes don't collide
// on "database is locked".
func dsn(path string) string {
	if strings.HasPrefix(path, "file:") {
		return path
	}
	return "file:" + path + "?" + url.Values{
		"_journal_mode": {"WAL"},
		"_busy_timeout": {"5000"},
		"_foreign_keys": {"on"},
		// Take the write lock when a transaction begins. Without this a
		// transaction that reads before it writes fails with "database is
		// locked" the moment two of them overlap — SQLite refuses to upgrade a
		// deferred read lock and does not wait out the busy timeout for it.
		"_txlock": {"immediate"},
	}.Encode()
}

func (s *Store) Close() { s.Events.Close() }

// --- settings ---

func (s *Store) Settings() (Settings, error) {
	var raw string
	err := s.DB.Get(&raw, `SELECT value FROM settings WHERE key = 'config'`)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	st := DefaultSettings()
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return Settings{}, fmt.Errorf("settings json: %w", err)
	}
	return st, nil
}

func (s *Store) SaveSettings(st Settings) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(
		`INSERT INTO settings (key, value) VALUES ('config', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(raw))
	return err
}

// --- accounts ---

func (s *Store) Account(pubkey string) (*Account, error) {
	var a Account
	err := s.DB.Get(&a, `SELECT * FROM accounts WHERE pubkey = ?`, pubkey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoAccount
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAccounts returns accounts whose pubkey or note matches query (empty =
// all), newest first.
func (s *Store) ListAccounts(query string, limit, offset int) ([]Account, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	accounts := []Account{}
	like := "%" + query + "%"
	err := s.DB.Select(&accounts,
		`SELECT * FROM accounts WHERE pubkey LIKE ? OR note LIKE ?
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`, like, like, limit, offset)
	return accounts, err
}

func (s *Store) CountAccounts() (total int, active int, err error) {
	now := time.Now().Unix()
	err = s.DB.Get(&total, `SELECT count(*) FROM accounts`)
	if err != nil {
		return
	}
	err = s.DB.Get(&active,
		`SELECT count(*) FROM accounts
		 WHERE status = 'active' AND (permanent = 1 OR expires_at = 0 OR expires_at > ?)`, now)
	return
}

// EnsureAccount creates an account row if missing and returns it.
func (s *Store) EnsureAccount(pubkey string) (*Account, error) {
	_, err := s.DB.Exec(
		`INSERT INTO accounts (pubkey, status, created_at) VALUES (?, 'active', ?)
		 ON CONFLICT(pubkey) DO NOTHING`, pubkey, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return s.Account(pubkey)
}

// SetAccountPermanent switches the lifetime plan on or off for an account. It
// is separate from UpdateAccount so that the paths which only renew or ban a
// pubkey (invites, NIP-86) leave the plan alone.
func (s *Store) SetAccountPermanent(pubkey string, permanent bool) error {
	res, err := s.DB.Exec(`UPDATE accounts SET permanent = ? WHERE pubkey = ?`, permanent, pubkey)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoAccount
	}
	return nil
}

// UpdateAccount applies an admin edit. expiresAt/quotaBytes are absolute values.
func (s *Store) UpdateAccount(pubkey, status string, expiresAt, quotaBytes int64, note string) error {
	res, err := s.DB.Exec(
		`UPDATE accounts SET status = ?, expires_at = ?, quota_bytes = ?, note = ? WHERE pubkey = ?`,
		status, expiresAt, quotaBytes, note, pubkey)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoAccount
	}
	return nil
}

func (s *Store) DeleteAccount(pubkey string) error {
	_, err := s.DB.Exec(`DELETE FROM accounts WHERE pubkey = ?`, pubkey)
	return err
}

// CanWrite reports whether the account may store an event of size bytes.
func (a *Account) CanWrite(now, size int64) (bool, string) {
	if a.Status == StatusBanned {
		return false, "blocked: this pubkey is banned"
	}
	if !a.Permanent && a.ExpiresAt != 0 && a.ExpiresAt < now {
		return false, "restricted: subscription expired"
	}
	if a.UsedBytes+size > a.QuotaBytes {
		return false, "blocked: storage quota exceeded"
	}
	return true, ""
}

// --- payments ---

func (s *Store) CreatePayment(p *Payment) error {
	_, err := s.DB.NamedExec(
		`INSERT INTO payments (id, pubkey, kind, sats, provider, payment_hash, bolt11, status, meta, created_at, paid_at)
		 VALUES (:id, :pubkey, :kind, :sats, :provider, :payment_hash, :bolt11, :status, :meta, :created_at, :paid_at)`, p)
	return err
}

func (s *Store) PaymentByHash(hash string) (*Payment, error) {
	var p Payment
	err := s.DB.Get(&p, `SELECT * FROM payments WHERE payment_hash = ?`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &p, err
}

// PendingPayments returns unsettled invoices created within maxAge, for the poller.
func (s *Store) PendingPayments(maxAge time.Duration) ([]Payment, error) {
	payments := []Payment{}
	err := s.DB.Select(&payments,
		`SELECT * FROM payments WHERE status = 'pending' AND created_at > ? ORDER BY created_at`,
		time.Now().Add(-maxAge).Unix())
	return payments, err
}

// ExpireStalePayments marks pending invoices older than maxAge as expired so
// the poller stops checking them.
func (s *Store) ExpireStalePayments(maxAge time.Duration) (int64, error) {
	res, err := s.DB.Exec(`UPDATE payments SET status = 'expired' WHERE status = 'pending' AND created_at <= ?`,
		time.Now().Add(-maxAge).Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) ListPayments(pubkey string, limit int) ([]Payment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	payments := []Payment{}
	var err error
	if pubkey == "" {
		err = s.DB.Select(&payments, `SELECT * FROM payments ORDER BY created_at DESC LIMIT ?`, limit)
	} else {
		err = s.DB.Select(&payments,
			`SELECT * FROM payments WHERE pubkey = ? ORDER BY created_at DESC LIMIT ?`, pubkey, limit)
	}
	return payments, err
}

// --- usage ---

// AddUsage records the storage cost of a saved event. Duplicate event ids are
// ignored so a re-saved event isn't billed twice.
//
// Which pot pays is decided here, by the same rule the write gate used: the
// author's own account when it can still absorb the bytes, otherwise the group
// they belong to. The pot is written into the usage row so RemoveUsage refunds
// the one that actually paid.
func (s *Store) AddUsage(eventID, pubkey string, size int64) error {
	allowance, err := s.Allowance(pubkey)
	if err != nil {
		return err
	}
	groupID := allowance.payer(time.Now().Unix(), size)

	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO usage_events (id, pubkey, size, created_at, group_id) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`, eventID, pubkey, size, time.Now().Unix(), groupID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return tx.Rollback()
	}
	if groupID != "" {
		if _, err := tx.Exec(`UPDATE groups SET used_bytes = used_bytes + ? WHERE id = ?`, size, groupID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err := tx.Exec(`UPDATE accounts SET used_bytes = used_bytes + ? WHERE pubkey = ?`, size, pubkey); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveUsage releases the storage of a deleted or replaced event.
func (s *Store) RemoveUsage(eventID string) error {
	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// the event store maps columns by the json tag (see eventstore's
	// sqlite3.Init), so a field whose name doesn't lowercase to the column
	// needs one
	var row struct {
		Pubkey  string `json:"pubkey"`
		Size    int64  `json:"size"`
		GroupID string `json:"group_id"`
	}
	err = tx.Get(&row, `SELECT pubkey, size, group_id FROM usage_events WHERE id = ?`, eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Rollback()
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM usage_events WHERE id = ?`, eventID); err != nil {
		return err
	}
	if row.GroupID != "" {
		if _, err := tx.Exec(
			`UPDATE groups SET used_bytes = max(0, used_bytes - ?) WHERE id = ?`, row.Size, row.GroupID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err := tx.Exec(
		`UPDATE accounts SET used_bytes = max(0, used_bytes - ?) WHERE pubkey = ?`, row.Size, row.Pubkey); err != nil {
		return err
	}
	return tx.Commit()
}

// ReconcileUsage drops usage rows for events of this pubkey that are no longer
// in the event table (replaceable events superseded by a newer version) and
// recomputes the account's used_bytes.
func (s *Store) ReconcileUsage(pubkey string) error {
	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Uploaded files live in the same table but not in the event table, so they
	// must be excluded here or every replaceable event a user posts would hand
	// their whole media quota back.
	if _, err := tx.Exec(
		`DELETE FROM usage_events WHERE pubkey = ? AND id NOT LIKE ?
		 AND id NOT IN (SELECT id FROM event WHERE pubkey = ?)`,
		pubkey, BlobUsagePrefix+"%", pubkey); err != nil {
		return err
	}
	if err := recomputeUsage(tx, pubkey); err != nil {
		return err
	}
	return tx.Commit()
}

// recomputeUsage rebuilds the counters for one pubkey and whichever group they
// share storage with, from the usage rows that are left.
func recomputeUsage(tx *sqlx.Tx, pubkey string) error {
	if _, err := tx.Exec(
		`UPDATE accounts SET used_bytes =
		 coalesce((SELECT sum(size) FROM usage_events WHERE pubkey = ? AND group_id = ''), 0)
		 WHERE pubkey = ?`, pubkey, pubkey); err != nil {
		return err
	}
	// the same events may have been billed to a shared group
	_, err := tx.Exec(
		`UPDATE groups SET used_bytes =
		 coalesce((SELECT sum(size) FROM usage_events WHERE group_id = groups.id), 0)
		 WHERE id IN (SELECT group_id FROM group_members WHERE pubkey = ?)`, pubkey)
	return err
}

// RecomputeUsage rebuilds used_bytes for every account and group from
// usage_events, fixing any drift caused by crashes mid-write.
func (s *Store) RecomputeUsage() error {
	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE accounts SET used_bytes = coalesce((
		SELECT sum(size) FROM usage_events
		WHERE usage_events.pubkey = accounts.pubkey AND usage_events.group_id = ''), 0)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE groups SET used_bytes = coalesce((
		SELECT sum(size) FROM usage_events WHERE usage_events.group_id = groups.id), 0)`); err != nil {
		return err
	}
	return tx.Commit()
}

// --- moderation (NIP-86) ---

const (
	ModBannedEvent  = "banned_event"
	ModAllowedEvent = "allowed_event"
	ModBlockedIP    = "blocked_ip"
	ModReportedBlob = "reported_blob" // BUD-09 report awaiting an admin
)

type ModEntry struct {
	Type      string `db:"type" json:"type"`
	Value     string `db:"value" json:"value"`
	Reason    string `db:"reason" json:"reason"`
	CreatedAt int64  `db:"created_at" json:"created_at"`
}

// ModAdd records a moderation decision, replacing any previous one of the same
// type for that value.
func (s *Store) ModAdd(typ, value, reason string) error {
	_, err := s.DB.Exec(
		`INSERT INTO moderation (type, value, reason, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(type, value) DO UPDATE SET reason = excluded.reason, created_at = excluded.created_at`,
		typ, value, reason, time.Now().Unix())
	return err
}

func (s *Store) ModRemove(typ, value string) error {
	_, err := s.DB.Exec(`DELETE FROM moderation WHERE type = ? AND value = ?`, typ, value)
	return err
}

func (s *Store) ModList(typ string) ([]ModEntry, error) {
	entries := []ModEntry{}
	err := s.DB.Select(&entries,
		`SELECT * FROM moderation WHERE type = ? ORDER BY created_at DESC LIMIT 1000`, typ)
	return entries, err
}

// ModHas reports whether a moderation entry exists.
func (s *Store) ModHas(typ, value string) bool {
	var n int
	if err := s.DB.Get(&n, `SELECT count(*) FROM moderation WHERE type = ? AND value = ?`, typ, value); err != nil {
		return false
	}
	return n > 0
}

// IsAdmin combines the admins from the environment with those granted at
// runtime through NIP-86.
func (s *Store) IsAdmin(pubkey string, envAdmins []string) bool {
	pubkey = strings.ToLower(pubkey)
	if pubkey == "" {
		return false
	}
	if slices.Contains(envAdmins, pubkey) {
		return true
	}
	settings, err := s.Settings()
	if err != nil {
		return false
	}
	return slices.Contains(settings.ExtraAdmins, pubkey)
}

// Stats for the admin dashboard.
type Stats struct {
	Accounts       int   `json:"accounts"`
	ActiveAccounts int   `json:"active_accounts"`
	Events         int   `json:"events"`
	StoredBytes    int64 `json:"stored_bytes"`
	PaidPayments   int   `json:"paid_payments"`
	SatsCollected  int64 `json:"sats_collected"`
	Groups         int   `json:"groups"`
	Nip05Names     int   `json:"nip05_names"`
}

func (s *Store) Stats() (Stats, error) {
	var st Stats
	var err error
	if st.Accounts, st.ActiveAccounts, err = s.CountAccounts(); err != nil {
		return st, err
	}
	if err = s.DB.Get(&st.Events, `SELECT count(*) FROM event`); err != nil {
		return st, err
	}
	if err = s.DB.Get(&st.StoredBytes, `SELECT coalesce(sum(size), 0) FROM usage_events`); err != nil {
		return st, err
	}
	if err = s.DB.Get(&st.PaidPayments, `SELECT count(*) FROM payments WHERE status = 'paid'`); err != nil {
		return st, err
	}
	if err = s.DB.Get(&st.SatsCollected, `SELECT coalesce(sum(sats), 0) FROM payments WHERE status = 'paid'`); err != nil {
		return st, err
	}
	if err = s.DB.Get(&st.Groups, `SELECT count(*) FROM groups`); err != nil {
		return st, err
	}
	st.Nip05Names, err = s.CountNip05Names()
	return st, err
}
