package relay

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/nbd-wtf/go-nostr/nip11"

	"nostrel/internal/store"
)

// advertised renders NIP-11 the way a client fetching the relay would see it.
// SupportedNIPs is []any because the document also allows string entries.
func advertised(t *testing.T, r *Relay) []int {
	t.Helper()
	info := nip11.RelayInformationDocument{}
	info.AddSupportedNIPs(supportedNIPs)

	var nips []int
	for _, entry := range r.relayInformation(context.Background(), &http.Request{}, info).SupportedNIPs {
		if nip, ok := entry.(int); ok {
			nips = append(nips, nip)
		}
	}
	return nips
}

// Anything the relay always does must be in supported_nips: a client that
// checks the list before trying negentropy or private messaging would
// otherwise never use features that work.
func TestAlwaysSupportedNIPsAreAdvertised(t *testing.T) {
	r, _ := newGate(t)
	nips := advertised(t, r)

	for _, nip := range []int{1, 4, 9, 11, 17, 40, 42, 45, 50, 56, 59, 70, 77, 86, 98} {
		if !slices.Contains(nips, nip) {
			t.Errorf("NIP-%02d is implemented but not advertised", nip)
		}
	}
}

// The rest are only true once they are switched on, and claiming them while
// off would be a lie in the other direction.
func TestConditionalNIPsFollowTheConfiguration(t *testing.T) {
	r, st := newGate(t)

	for _, nip := range []int{5, 13, 57, 58, 96} {
		if slices.Contains(advertised(t, r), nip) {
			t.Errorf("NIP-%02d advertised while switched off", nip)
		}
	}

	settings, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.AcceptZapReceipts = true
	settings.AcceptBadgeAwards = true
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveNip05Domain(store.Nip05Domain{
		Domain: "example.com", Enabled: true, PriceSats: 100, PeriodDays: 365,
	}); err != nil {
		t.Fatal(err)
	}

	nips := advertised(t, r)
	for _, nip := range []int{5, 57, 58} {
		if !slices.Contains(nips, nip) {
			t.Errorf("NIP-%02d switched on but not advertised", nip)
		}
	}

	// a domain that is not for sale does not make this a NIP-05 provider
	if err := st.SaveNip05Domain(store.Nip05Domain{
		Domain: "example.com", Enabled: false, PriceSats: 100, PeriodDays: 365,
	}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(advertised(t, r), 5) {
		t.Error("NIP-05 advertised with every domain disabled")
	}
}
