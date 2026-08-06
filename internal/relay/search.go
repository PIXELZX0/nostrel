package relay

import (
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// NIP-50 lets a search string carry `key:value` extensions alongside the words
// being searched for. The spec says relays SHOULD ignore extensions they do not
// support — which means dropping them, not searching for them literally, or a
// client asking for `nsfw:false cats` would get no results at all.
//
// `domain:` is the one worth implementing here: this relay sells the NIP-05
// names under its own domains, so it can answer it exactly instead of guessing
// from profile metadata the way a general-purpose relay has to.

// searchExtensions are the tokens NIP-50 defines. Everything here is stripped
// from the query; only domain: changes the result.
var searchExtensions = []string{"include:", "domain:", "language:", "sentiment:", "nsfw:"}

// parseSearch splits a NIP-50 search string into the words to match and the
// domain filter, if one was asked for.
func parseSearch(search string) (words string, domain string) {
	var kept []string
	for _, token := range strings.Fields(search) {
		lower := strings.ToLower(token)
		matched := false
		for _, prefix := range searchExtensions {
			if !strings.HasPrefix(lower, prefix) {
				continue
			}
			matched = true
			if prefix == "domain:" {
				domain = strings.TrimPrefix(lower, prefix)
			}
			break
		}
		if !matched {
			kept = append(kept, token)
		}
	}
	return strings.Join(kept, " "), domain
}

// maxDomainAuthors caps how many customers one `domain:` search expands to.
// Beyond this the filter would be bigger than the query it is helping.
const maxDomainAuthors = 500

// applySearchExtensions rewrites a filter in place. The second result is false
// when the extensions can only match nothing, so the caller can skip the query
// entirely rather than answering as if no filter had been asked for.
func (r *Relay) applySearchExtensions(filter *nostr.Filter) bool {
	if filter.Search == "" {
		return true
	}

	words, domain := parseSearch(filter.Search)
	filter.Search = words
	if domain == "" {
		return true
	}

	holders, err := r.store.Nip05PubkeysFor(domain, maxDomainAuthors)
	if err != nil {
		r.Log.Printf("nip-50 domain lookup for %s: %v", domain, err)
		return false
	}
	if len(holders) == 0 {
		return false
	}

	if len(filter.Authors) == 0 {
		filter.Authors = holders
		return true
	}

	// both were asked for: only pubkeys satisfying each survive
	wanted := make(map[string]bool, len(holders))
	for _, holder := range holders {
		wanted[holder] = true
	}
	var narrowed []string
	for _, author := range filter.Authors {
		if wanted[author] {
			narrowed = append(narrowed, author)
		}
	}
	filter.Authors = narrowed
	return len(narrowed) > 0
}
