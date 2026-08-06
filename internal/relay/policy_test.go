package relay

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

const me = "aa11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"
const other = "bb11bb22cc33dd44ee55ff6600778899aabbccddeeff001122334455667788aa"

func TestPrivateFilterAccess(t *testing.T) {
	tests := []struct {
		name   string
		filter nostr.Filter
		allow  bool
	}{
		{
			name:   "recipient reading their own gift wraps",
			filter: nostr.Filter{Kinds: []int{1059}, Tags: nostr.TagMap{"p": {me}}},
			allow:  true,
		},
		{
			name:   "sender reading what they sent",
			filter: nostr.Filter{Kinds: []int{1059}, Authors: []string{me}},
			allow:  true,
		},
		{
			name:   "reading somebody else's mail",
			filter: nostr.Filter{Kinds: []int{1059}, Tags: nostr.TagMap{"p": {other}}},
			allow:  false,
		},
		{
			name:   "sweeping up every gift wrap",
			filter: nostr.Filter{Kinds: []int{1059}},
			allow:  false,
		},
		{
			name:   "hiding a wide net behind one's own name",
			filter: nostr.Filter{Kinds: []int{1059}, Authors: []string{me, other}},
			allow:  false,
		},
		{
			name:   "kind 4 direct messages are private too",
			filter: nostr.Filter{Kinds: []int{4}},
			allow:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !wantsPrivateKinds(tc.filter) {
				t.Fatal("filter should have been treated as private")
			}
			if got := addressedTo(tc.filter, me); got != tc.allow {
				t.Errorf("addressedTo = %v, want %v", got, tc.allow)
			}
		})
	}
}

func TestPublicFiltersAreNotPrivate(t *testing.T) {
	for _, filter := range []nostr.Filter{
		{Kinds: []int{1}},
		{Kinds: []int{1, 7, 30023}},
		{Authors: []string{me}},
	} {
		if wantsPrivateKinds(filter) {
			t.Errorf("%v should not be treated as a private-kind filter", filter)
		}
	}
}

func TestExpired(t *testing.T) {
	past := nostr.Now() - 10
	future := nostr.Now() + 3600

	cases := []struct {
		name string
		tags nostr.Tags
		want bool
	}{
		{"no expiration tag", nostr.Tags{{"t", "hello"}}, false},
		{"expiration in the future", nostr.Tags{{"expiration", itoa(int64(future))}}, false},
		{"expiration in the past", nostr.Tags{{"expiration", itoa(int64(past))}}, true},
		{"garbage expiration", nostr.Tags{{"expiration", "soon"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := expired(&nostr.Event{Tags: tc.tags}); got != tc.want {
				t.Errorf("expired = %v, want %v", got, tc.want)
			}
		})
	}
}

func itoa(n int64) string {
	digits := ""
	if n == 0 {
		return "0"
	}
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
