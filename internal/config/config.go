// Package config loads the handful of settings that cannot live in the
// database: where the database is, which port to listen on, and the credentials
// that guard the admin panel itself. Everything else — relay identity, prices,
// policy, payment and storage backends — is owned by the panel and stored in
// the settings table.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port   int
	DBPath string

	// PanelURL is the public https address of the web panel, used in NIP-11
	// payments_url and in rejection messages shown to clients.
	PanelURL string
	// ServiceURL is the public wss:// address of the relay. Leave empty to let
	// khatru guess it from request headers.
	ServiceURL string

	// RelaySecretKey is the relay's own signing key, used for the NIP-43 events
	// it publishes about itself (roles, membership, join/leave notices). It is
	// the relay's identity: anyone holding it can forge those lists, so it lives
	// in the environment only and is never written to the database or an API
	// response. Empty disables NIP-43 entirely.
	RelaySecretKey string

	AdminPubkeys      []string
	AdminPasswordHash string
	SessionSecret     []byte
}

func Load() (*Config, error) {
	c := &Config{
		DBPath:            env("DB_PATH", "./nostrel.db"),
		PanelURL:          strings.TrimRight(env("PANEL_URL", "http://localhost:3334"), "/"),
		ServiceURL:        env("SERVICE_URL", ""),
		RelaySecretKey:    strings.ToLower(env("RELAY_SECRET_KEY", "")),
		AdminPasswordHash: env("ADMIN_PASSWORD_HASH", ""),
	}

	port, err := strconv.Atoi(env("RELAY_PORT", "3334"))
	if err != nil {
		return nil, fmt.Errorf("RELAY_PORT: %w", err)
	}
	c.Port = port

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
