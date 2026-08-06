package payments

import (
	"context"
	"fmt"
	"sync"

	"nostrel/internal/store"
)

// Checker is implemented by providers that can verify their credentials
// without moving money, so the admin panel can test a backend before saving it.
type Checker interface {
	Check(ctx context.Context) (string, error)
}

// Resolver builds the lightning provider from the settings in the database and
// rebuilds it whenever an admin changes them, so the backend can be swapped
// from the panel without restarting the relay.
type Resolver struct {
	store   *store.Store
	webhook string

	mu          sync.Mutex
	current     Provider
	fingerprint string
}

// NewResolver reads the backend from st. webhook is the URL LNbits should call
// when an invoice settles.
func NewResolver(st *store.Store, webhook string) *Resolver {
	return &Resolver{store: st, webhook: webhook}
}

// Provider returns the provider for the current settings.
func (r *Resolver) Provider() (Provider, error) {
	settings, err := r.store.Settings()
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	fingerprint := settings.PaymentFingerprint()
	if r.current != nil && fingerprint == r.fingerprint {
		return r.current, nil
	}

	provider, err := BuildWithWebhook(settings, r.webhook)
	if err != nil {
		return nil, err
	}
	r.current, r.fingerprint = provider, fingerprint
	return provider, nil
}

// Name is the configured backend, or "unconfigured" when it can't be built.
func (r *Resolver) Name() string {
	provider, err := r.Provider()
	if err != nil {
		return "unconfigured"
	}
	return provider.Name()
}

// Build makes a provider from settings without caching it — used by the
// panel's connection test, which must exercise values that aren't saved yet.
func Build(settings store.Settings) (Provider, error) {
	switch settings.PaymentProvider {
	case "lnbits":
		if settings.LNbitsURL == "" || settings.LNbitsInvoiceKey == "" {
			return nil, fmt.Errorf("lnbits needs a server URL and an invoice key")
		}
		return NewLNbits(settings.LNbitsURL, settings.LNbitsInvoiceKey, ""), nil
	case "nwc":
		if settings.NWCURI == "" {
			return nil, fmt.Errorf("nwc needs a connection URI")
		}
		return NewNWC(settings.NWCURI)
	case "mock":
		return NewMock(), nil
	case "":
		return nil, fmt.Errorf("no payment backend configured")
	default:
		return nil, fmt.Errorf("unknown payment backend %q", settings.PaymentProvider)
	}
}

// BuildWithWebhook is Build plus the webhook URL LNbits should call.
func BuildWithWebhook(settings store.Settings, webhook string) (Provider, error) {
	provider, err := Build(settings)
	if err != nil {
		return nil, err
	}
	if lnbits, ok := provider.(*LNbits); ok {
		lnbits.webhook = webhook
	}
	return provider, nil
}
