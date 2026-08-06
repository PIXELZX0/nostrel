package payments

import (
	"path/filepath"
	"testing"

	"nostrel/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func TestBuildValidatesCredentials(t *testing.T) {
	cases := []struct {
		name     string
		settings store.Settings
		wantErr  bool
	}{
		{"mock", store.Settings{PaymentProvider: "mock"}, false},
		{"lnbits with url and key", store.Settings{
			PaymentProvider: "lnbits", LNbitsURL: "https://lnbits.example.com", LNbitsInvoiceKey: "key",
		}, false},
		{"lnbits missing key", store.Settings{
			PaymentProvider: "lnbits", LNbitsURL: "https://lnbits.example.com",
		}, true},
		{"lnbits missing url", store.Settings{PaymentProvider: "lnbits", LNbitsInvoiceKey: "key"}, true},
		{"nwc missing uri", store.Settings{PaymentProvider: "nwc"}, true},
		{"nwc malformed uri", store.Settings{PaymentProvider: "nwc", NWCURI: "https://example.com"}, true},
		{"nothing configured", store.Settings{}, true},
		{"unknown backend", store.Settings{PaymentProvider: "paypal"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build(tc.settings)
			if tc.wantErr && err == nil {
				t.Error("expected an error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolverRebuildsWhenSettingsChange(t *testing.T) {
	st := newStore(t)
	if err := st.EnsurePaymentDefaults("mock", "", "", ""); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(st, "https://relay.example.com/webhook/lnbits")

	first, err := r.Provider()
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if first.Name() != "mock" {
		t.Fatalf("provider = %s, want mock", first.Name())
	}

	// unchanged settings hand back the same instance
	again, _ := r.Provider()
	if again != first {
		t.Error("provider was rebuilt even though settings did not change")
	}

	// an admin switches the backend from the panel
	settings, _ := st.Settings()
	settings.PaymentProvider = "lnbits"
	settings.LNbitsURL = "https://lnbits.example.com"
	settings.LNbitsInvoiceKey = "key"
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	switched, err := r.Provider()
	if err != nil {
		t.Fatalf("provider after switch: %v", err)
	}
	if switched.Name() != "lnbits" {
		t.Errorf("provider = %s, want lnbits (no restart needed)", switched.Name())
	}
	if lnbits, ok := switched.(*LNbits); !ok || lnbits.webhook == "" {
		t.Error("the rebuilt LNbits client lost its webhook URL")
	}
}

func TestSecretsAreRedacted(t *testing.T) {
	settings := store.Settings{
		PaymentProvider:  "lnbits",
		LNbitsURL:        "https://lnbits.example.com",
		LNbitsInvoiceKey: "supersecretinvoicekey1234",
		NWCURI:           "nostr+walletconnect://pubkey?relay=wss://r&secret=abcdef",
	}
	redacted := settings.Redacted()

	if redacted.LNbitsInvoiceKey == settings.LNbitsInvoiceKey {
		t.Error("the invoice key was sent out in the clear")
	}
	if redacted.NWCURI == settings.NWCURI {
		t.Error("the NWC connection string was sent out in the clear")
	}
	if redacted.LNbitsInvoiceKey != store.MaskedValue+"1234" {
		t.Errorf("masked key = %q, want the last four characters kept", redacted.LNbitsInvoiceKey)
	}
	if redacted.LNbitsURL != settings.LNbitsURL {
		t.Error("the server URL is not a secret and should stay readable")
	}
	if !store.IsMasked(redacted.LNbitsInvoiceKey) || store.IsMasked("a-real-key") {
		t.Error("IsMasked must tell a redacted value from a new one")
	}
}
