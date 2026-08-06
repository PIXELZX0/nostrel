package relay

import (
	"slices"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"nostrel/internal/store"
)

func TestParseSearchStripsExtensions(t *testing.T) {
	cases := []struct {
		in     string
		words  string
		domain string
	}{
		{"cats", "cats", ""},
		{"domain:example.com", "", "example.com"},
		{"domain:Example.COM cats", "cats", "example.com"},
		{"nsfw:false include:spam cats dogs", "cats dogs", ""},
		{"language:ko sentiment:positive 고양이", "고양이", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		words, domain := parseSearch(tc.in)
		if words != tc.words || domain != tc.domain {
			t.Errorf("parseSearch(%q) = (%q, %q), want (%q, %q)", tc.in, words, domain, tc.words, tc.domain)
		}
	}
}

func TestDomainSearchUsesTheNamesWeSold(t *testing.T) {
	r, st := newGate(t)

	if err := st.SaveNip05Domain(store.Nip05Domain{
		Domain: "example.com", Enabled: true, PriceSats: 100, PeriodDays: 365,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "alice", Pubkey: me, Permanent: true,
	}); err != nil {
		t.Fatal(err)
	}

	// the extension is stripped from the words and turned into an author filter
	filter := nostr.Filter{Search: "domain:example.com hello"}
	if !r.applySearchExtensions(&filter) {
		t.Fatal("a domain with a customer matched nothing")
	}
	if filter.Search != "hello" {
		t.Errorf("search = %q, want the extension stripped", filter.Search)
	}
	if !slices.Equal(filter.Authors, []string{me}) {
		t.Errorf("authors = %v, want [%s]", filter.Authors, me)
	}

	// a domain nobody holds a name under can only match nothing
	empty := nostr.Filter{Search: "domain:nowhere.example hello"}
	if r.applySearchExtensions(&empty) {
		t.Error("an unknown domain was treated as no filter at all")
	}

	// combined with an explicit author list, both must agree
	narrowed := nostr.Filter{Search: "domain:example.com", Authors: []string{me, other}}
	if !r.applySearchExtensions(&narrowed) {
		t.Fatal("the intersection was reported empty")
	}
	if !slices.Equal(narrowed.Authors, []string{me}) {
		t.Errorf("authors = %v, want only the name holder", narrowed.Authors)
	}

	excluded := nostr.Filter{Search: "domain:example.com", Authors: []string{other}}
	if r.applySearchExtensions(&excluded) {
		t.Error("an author who holds no name under the domain still matched")
	}

	// an expired name is not a customer of that domain any more
	if err := st.SaveNip05Name(store.Nip05Name{
		Domain: "example.com", Name: "alice", Pubkey: me, ExpiresAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	lapsed := nostr.Filter{Search: "domain:example.com"}
	if r.applySearchExtensions(&lapsed) {
		t.Error("a lapsed name still counted towards the domain")
	}
}
