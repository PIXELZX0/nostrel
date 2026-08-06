// Command nostrel runs a paid-whitelist Nostr relay with a web panel.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"nostrel/internal/api"
	"nostrel/internal/billing"
	"nostrel/internal/blobs"
	"nostrel/internal/config"
	"nostrel/internal/payments"
	"nostrel/internal/relay"
	"nostrel/internal/store"
)

func main() {
	logger := log.New(os.Stderr, "[nostrel] ", log.LstdFlags|log.Lmsgprefix)

	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		if err := hashPassword(os.Args[2:]); err != nil {
			logger.Fatal(err)
		}
		return
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("config: %v", err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Fatalf("database: %v", err)
	}
	defer st.Close()

	if len(os.Args) > 1 {
		if err := runCommand(os.Args[1], st, cfg, logger); err != nil {
			logger.Fatalf("%s: %v", os.Args[1], err)
		}
		return
	}

	logConfig(cfg, logger)

	// the lightning backend is panel-owned, so it can be changed without a restart
	providers := payments.NewResolver(st, cfg.PanelURL+"/webhook/lnbits")
	switch providers.Name() {
	case "mock":
		logger.Print("WARNING: payment backend is 'mock' — invoices settle themselves, never use this in production")
	case "unconfigured":
		logger.Print("WARNING: no working payment backend — configure one in the admin panel")
	}
	if len(cfg.AdminPubkeys) == 0 && cfg.AdminPasswordHash == "" {
		logger.Print("WARNING: no ADMIN_PUBKEYS and no ADMIN_PASSWORD_HASH — the admin panel is unreachable")
	}
	if cfg.AdminPasswordHash != "" && !strings.HasPrefix(cfg.AdminPasswordHash, "$2") {
		logger.Print("WARNING: ADMIN_PASSWORD_HASH is not a bcrypt hash — password login will always fail. " +
			"It must be the output of `nostrel hash-password`, single-quoted so the shell keeps the $ signs")
	}

	r := relay.New(cfg, st, logger)
	r.EnableManagementAPI()

	bill := billing.New(st, providers, logger)
	panel := api.New(cfg, st, bill, logger)

	// media storage powers both Blossom and NIP-96; without it the relay still
	// works, it just stores no files
	storage, err := blobs.New(st, store.DefaultBlobPath)
	if err != nil {
		logger.Printf("media storage disabled: %v", err)
		storage = nil
	}
	var index *blobs.Index
	if storage != nil {
		index = storage.Index(st, cfg.PanelURL)
		panel.EnableMedia(storage, index)
	}

	panel.Register(r.Router())

	// blossom must wrap the mux last: it takes ownership of the router and
	// falls back to whatever was registered before it
	if storage != nil {
		r.EnableBlossom(storage, index, cfg.PanelURL)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bill.Poll(ctx)

	// NIP-43: say who we are and who is in, then keep the member list current
	r.PublishRoles(ctx)
	r.PublishMembership(ctx)
	go r.WatchMembership(ctx)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           r.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Printf("listening on %s (panel: %s, payments: %s)", srv.Addr, cfg.PanelURL, providers.Name())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Print("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}

// logConfig prints the settings the process actually resolved. An environment
// variable that never reached the process (host env not passed into the
// container, a typo, a missing env_file) is invisible otherwise: the only clue
// is a warning about a value being empty. Secrets are reported as set/unset.
func logConfig(cfg *config.Config, logger *log.Logger) {
	logger.Printf("config: RELAY_PORT=%d DB_PATH=%q PANEL_URL=%q SERVICE_URL=%s",
		cfg.Port, cfg.DBPath, cfg.PanelURL, quoteOr(cfg.ServiceURL, "(unset, guessed from request headers)"))

	admins := "(none)"
	if n := len(cfg.AdminPubkeys); n > 0 {
		short := make([]string, n)
		for i, pk := range cfg.AdminPubkeys {
			short[i] = pk[:8] + "…"
		}
		admins = fmt.Sprintf("%d (%s)", n, strings.Join(short, ", "))
	}
	session := "set"
	if os.Getenv("SESSION_SECRET") == "" {
		session = "unset (password sessions drop on restart)"
	}
	logger.Printf("config: ADMIN_PUBKEYS=%s ADMIN_PASSWORD_HASH=%s SESSION_SECRET=%s RELAY_SECRET_KEY=%s",
		admins, setOrNot(cfg.AdminPasswordHash != ""), session, setOrNot(cfg.RelaySecretKey != ""))
}

func quoteOr(v, empty string) string {
	if v == "" {
		return empty
	}
	return strconv.Quote(v)
}

func setOrNot(ok bool) string {
	if ok {
		return "set"
	}
	return "unset"
}

func runCommand(name string, st *store.Store, cfg *config.Config, logger *log.Logger) error {
	switch name {
	case "recompute-usage":
		if err := st.RecomputeUsage(); err != nil {
			return err
		}
		logger.Print("usage recomputed from usage_events")
		return nil
	case "migrate-blobs":
		return migrateBlobs(st, cfg, logger, os.Args[2:])
	default:
		return fmt.Errorf("unknown command (available: recompute-usage, migrate-blobs, hash-password)")
	}
}

// migrateBlobs copies every file from a local directory into the storage
// backend the relay is configured with. Switching backends does not move
// anything by itself, so this is how "local → S3" is finished.
func migrateBlobs(st *store.Store, cfg *config.Config, logger *log.Logger, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: nostrel migrate-blobs <source-directory>")
	}
	source, err := blobs.NewLocal(args[0])
	if err != nil {
		return err
	}
	storage, err := blobs.New(st, store.DefaultBlobPath)
	if err != nil {
		return err
	}
	target := storage.Backend()
	if target.Name() == source.Name() {
		return fmt.Errorf("source and destination are the same (%s)", target.Name())
	}

	ctx := context.Background()
	copied, skipped, err := blobs.CopyAll(ctx, source, target, args[0])
	if err != nil {
		return err
	}
	logger.Printf("migrated %d file(s) to %s, %d already there", copied, target.Name(), skipped)
	return nil
}

// hashPassword prints a bcrypt hash for ADMIN_PASSWORD_HASH.
func hashPassword(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: nostrel hash-password <password>")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(args[0]), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	fmt.Println(string(hash))
	return nil
}
