package api

import (
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// NIP-5A static websites. A site is a manifest event mapping paths to blob
// hashes; the bytes are ordinary Blossom blobs.
//
// This relay is both the relay holding the manifest and the Blossom server
// holding the files, so a request is answered from local storage — no fetching
// from other relays or `server` hints needed.
//
// Sites are addressed as <name>.<nsite domain>, where <name> is a NIP-05 name
// sold here. That reuses the identifiers customers already bought instead of
// asking them to remember an npub.

const (
	kindRootSite  = 15128
	kindNamedSite = 35128
)

// handleNsite serves one file of a customer's site, or nothing at all when the
// host is not an nsite address — in which case the caller falls through to the
// normal panel routes.
func (s *Server) handleNsite(w http.ResponseWriter, r *http.Request) bool {
	if s.blobs == nil {
		return false
	}
	name, domain, ok := splitNsiteHost(requestDomain(r), s.nsiteDomains())
	if !ok {
		return false
	}

	pubkey, err := s.store.ResolveNip05(domain, name)
	if err != nil {
		s.log.Printf("nsite: resolving %s@%s: %v", name, domain, err)
		http.Error(w, "could not look that site up", http.StatusInternalServerError)
		return true
	}
	if pubkey == "" {
		http.Error(w, "no such site", http.StatusNotFound)
		return true
	}

	manifest := s.siteManifest(r, pubkey)
	if manifest == nil {
		http.Error(w, "this name has no site published", http.StatusNotFound)
		return true
	}

	paths := sitePaths(manifest)
	wanted := requestedPath(r.URL.Path)
	hash, found := paths[wanted]
	if !found {
		// NIP-5A: fall back to the site's own 404 page before giving up
		if hash, found = paths["/404.html"]; found {
			s.serveBlob(w, r, hash, "/404.html", http.StatusNotFound)
			return true
		}
		http.Error(w, "not found", http.StatusNotFound)
		return true
	}
	s.serveBlob(w, r, hash, wanted, http.StatusOK)
	return true
}

// nsiteDomains is the list an admin registered for site hosting.
func (s *Server) nsiteDomains() []string {
	settings, err := s.store.Settings()
	if err != nil {
		s.log.Printf("nsite: reading settings: %v", err)
		return nil
	}
	var domains []string
	for _, domain := range strings.Split(settings.NsiteDomains, ",") {
		if domain = strings.ToLower(strings.TrimSpace(domain)); domain != "" {
			domains = append(domains, domain)
		}
	}
	return domains
}

// splitNsiteHost pulls "<name>.<nsite domain>" apart. The name that comes back
// is looked up against the NIP-05 names sold under that same domain.
func splitNsiteHost(host string, nsiteDomains []string) (name, domain string, ok bool) {
	for _, nsite := range nsiteDomains {
		suffix := "." + nsite
		if !strings.HasSuffix(host, suffix) {
			continue
		}
		name = strings.TrimSuffix(host, suffix)
		// only a single label: a.sites.example.com, not a.b.sites.example.com
		if name == "" || strings.Contains(name, ".") {
			continue
		}
		return name, nsite, true
	}
	return "", "", false
}

// siteManifest finds the site to serve: a named site when the host asked for
// one, otherwise the pubkey's root site.
func (s *Server) siteManifest(r *http.Request, pubkey string) *nostr.Event {
	filter := nostr.Filter{
		Authors: []string{pubkey},
		Kinds:   []int{kindRootSite, kindNamedSite},
		Limit:   20,
	}
	events, err := s.store.Events.QueryEvents(r.Context(), filter)
	if err != nil {
		s.log.Printf("nsite: querying manifests for %s: %v", pubkey, err)
		return nil
	}

	var root *nostr.Event
	for evt := range events {
		if evt.Kind == kindRootSite && (root == nil || evt.CreatedAt > root.CreatedAt) {
			root = evt
		}
	}
	return root
}

// sitePaths turns the manifest's path tags into a lookup table.
func sitePaths(manifest *nostr.Event) map[string]string {
	paths := map[string]string{}
	for _, tag := range manifest.Tags {
		if len(tag) < 3 || tag[0] != "path" || tag[1] == "" || tag[2] == "" {
			continue
		}
		paths[tag[1]] = tag[2]
	}
	return paths
}

// requestedPath applies NIP-5A's directory rule: a path with no filename gets
// index.html appended.
func requestedPath(raw string) string {
	if raw == "" {
		raw = "/"
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	if strings.HasSuffix(raw, "/") {
		return raw + "index.html"
	}
	if path.Ext(raw) == "" {
		return raw + "/index.html"
	}
	return raw
}

// serveBlob streams a file out of the same storage Blossom uses.
func (s *Server) serveBlob(w http.ResponseWriter, r *http.Request, hash, name string, status int) {
	ext, exists := s.blobs.Exists(r.Context(), hash)
	if !exists {
		http.Error(w, "this site references a file the relay does not hold", http.StatusNotFound)
		return
	}

	body, err := s.blobs.Read(r.Context(), hash, ext)
	if err != nil {
		s.log.Printf("nsite: reading %s: %v", hash, err)
		http.Error(w, "could not read that file", http.StatusInternalServerError)
		return
	}
	defer body.Close()

	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	// blobs are content addressed, so a hit can be cached hard
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	io.Copy(w, body)
}
