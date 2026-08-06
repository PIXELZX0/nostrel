// Package config loads runtime configuration from environment variables.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host   string
	Port   int
	DBPath string

	// PanelURL is the public https address of the web panel, used in NIP-11
	// payments_url and in rejection messages shown to clients.
	PanelURL string
	// ServiceURL is the public wss:// address of the relay. Leave empty to let
	// khatru guess it from request headers.
	ServiceURL string

	RelayName        string
	RelayDescription string
	RelayPubkey      string
	RelayContact     string
	RelayIcon        string

	// RelaySecretKey is the relay's own signing key, used for the NIP-43 events
	// it publishes about itself (roles, membership, join/leave notices). It is
	// the relay's identity: anyone holding it can forge those lists, so it lives
	// in the environment only and is never written to the database or an API
	// response. Empty disables NIP-43 entirely.
	RelaySecretKey string

	AdminPubkeys      []string
	AdminPasswordHash string
	SessionSecret     []byte

	PaymentProvider  string // lnbits | nwc | mock
	LNbitsURL        string
	LNbitsInvoiceKey string
	NWCURI           string

	ReadAuthRequired bool

	// MinPoW is the NIP-13 difficulty an event must commit to. 0 disables it.
	MinPoW int
	// MaxPastDrift/MaxFutureDrift bound created_at (0 = unbounded), advertised
	// through the NIP-11 created_at limits.
	MaxPastDrift   time.Duration
	MaxFutureDrift time.Duration

	// Media storage seeds: "local" or "s3". After the first start the admin
	// panel owns the storage configuration.
	BlobBackend string
	BlobPath    string
	MaxBlobSize int64
}

func Load() (*Config, error) {
	c := &Config{
		Host:              env("HOST", ""),
		DBPath:            env("DB_PATH", "./nostrel.db"),
		PanelURL:          strings.TrimRight(env("PANEL_URL", "http://localhost:3334"), "/"),
		ServiceURL:        env("SERVICE_URL", ""),
		RelayName:         env("RELAY_NAME", "nostrel"),
		RelayDescription:  env("RELAY_DESCRIPTION", "a paid whitelist relay"),
		RelayPubkey:       env("RELAY_PUBKEY", ""),
		RelayContact:      env("RELAY_CONTACT", ""),
		RelayIcon:         env("RELAY_ICON", ""),
		RelaySecretKey:    strings.ToLower(env("RELAY_SECRET_KEY", "")),
		AdminPasswordHash: env("ADMIN_PASSWORD_HASH", ""),
		PaymentProvider:   strings.ToLower(env("PAYMENT_PROVIDER", "mock")),
		LNbitsURL:         strings.TrimRight(env("LNBITS_URL", ""), "/"),
		LNbitsInvoiceKey:  env("LNBITS_INVOICE_KEY", ""),
		NWCURI:            env("NWC_URI", ""),
		ReadAuthRequired:  env("READ_AUTH_REQUIRED", "false") == "true",
		BlobBackend:       strings.ToLower(env("BLOB_BACKEND", "local")),
		BlobPath:          env("BLOB_PATH", "./blobs"),
	}

	port, err := strconv.Atoi(env("RELAY_PORT", "3334"))
	if err != nil {
		return nil, fmt.Errorf("RELAY_PORT: %w", err)
	}
	c.Port = port

	if c.MinPoW, err = intEnv("MIN_POW", 0); err != nil {
		return nil, err
	}
	past, err := intEnv("CREATED_AT_MAX_PAST", 0)
	if err != nil {
		return nil, err
	}
	future, err := intEnv("CREATED_AT_MAX_FUTURE", 900) // 15 minutes
	if err != nil {
		return nil, err
	}
	c.MaxPastDrift = time.Duration(past) * time.Second
	c.MaxFutureDrift = time.Duration(future) * time.Second

	maxBlob, err := intEnv("MAX_BLOB_SIZE_MB", 25)
	if err != nil {
		return nil, err
	}
	c.MaxBlobSize = int64(maxBlob) << 20

	for _, pk := range strings.Split(env("ADMIN_PUBKEYS", ""), ",") {
		pk = strings.TrimSpace(pk)
		if pk == "" {
			continue
		}
		if !isHex32(pk) {
			return nil, fmt.Errorf("ADMIN_PUBKEYS: %q is not a 64-char hex pubkey", pk)
		}
		c.AdminPubkeys = append(c.AdminPubkeys, strings.ToLower(pk))
	}

	if s := env("SESSION_SECRET", ""); s != "" {
		c.SessionSecret = []byte(s)
	} else {
		// Ephemeral secret: password sessions simply don't survive a restart.
		c.SessionSecret = make([]byte, 32)
		if _, err := rand.Read(c.SessionSecret); err != nil {
			return nil, fmt.Errorf("generating session secret: %w", err)
		}
	}

	switch c.PaymentProvider {
	case "lnbits":
		if c.LNbitsURL == "" || c.LNbitsInvoiceKey == "" {
			return nil, fmt.Errorf("PAYMENT_PROVIDER=lnbits requires LNBITS_URL and LNBITS_INVOICE_KEY")
		}
	case "nwc":
		if c.NWCURI == "" {
			return nil, fmt.Errorf("PAYMENT_PROVIDER=nwc requires NWC_URI")
		}
	case "mock":
	default:
		return nil, fmt.Errorf("PAYMENT_PROVIDER must be lnbits, nwc or mock (got %q)", c.PaymentProvider)
	}

	if c.RelayPubkey != "" && !isHex32(c.RelayPubkey) {
		return nil, fmt.Errorf("RELAY_PUBKEY must be a 64-char hex pubkey")
	}
	if c.RelaySecretKey != "" && !isHex32(c.RelaySecretKey) {
		return nil, fmt.Errorf("RELAY_SECRET_KEY must be a 64-char hex private key")
	}

	return c, nil
}

// IsAdmin reports whether pubkey is in ADMIN_PUBKEYS.
func (c *Config) IsAdmin(pubkey string) bool {
	pubkey = strings.ToLower(pubkey)
	for _, pk := range c.AdminPubkeys {
		if pk == pubkey {
			return true
		}
	}
	return false
}

func intEnv(key string, def int) (int, error) {
	raw := env(key, "")
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if v < 0 {
		return 0, fmt.Errorf("%s must not be negative", key)
	}
	return v, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func isHex32(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
