// Package blobs stores uploaded media and bills it to the uploader's storage
// quota, the same pot events are charged against. Where the bytes physically
// live is a swappable Backend: a local directory or an S3-compatible bucket.
package blobs

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"nostrel/internal/relaycore/blossom"

	"nostrel/internal/store"
)

// usagePrefix namespaces blob rows in usage_events so they can't collide with
// event ids.
const usagePrefix = "blob:"

type Storage struct {
	store    *store.Store
	localDir string // used when the local backend has no path of its own

	mu          sync.RWMutex
	backend     Backend
	fingerprint string
	maxSize     int64
}

// New builds storage from the settings in the database, falling back to
// localDir when the local backend is configured without a path of its own.
func New(st *store.Store, localDir string) (*Storage, error) {
	s := &Storage{store: st, localDir: localDir}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// Reload picks up storage settings an admin changed. Callers invoke it after
// saving settings; the backend is otherwise left alone so normal traffic costs
// no extra queries.
func (s *Storage) Reload() error {
	settings, err := s.store.Settings()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.maxSize = settings.MaxBlobSize()
	fingerprint := settings.StorageFingerprint()
	if s.backend != nil && fingerprint == s.fingerprint {
		return nil
	}
	backend, err := BuildBackend(settings, s.localDir)
	if err != nil {
		return err
	}
	s.backend, s.fingerprint = backend, fingerprint
	return nil
}

func (s *Storage) Backend() Backend {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.backend
}

func (s *Storage) MaxSize() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxSize
}

func (s *Storage) Write(ctx context.Context, sha256, ext string, body []byte) error {
	return s.Backend().Write(ctx, sha256, ext, body)
}

func (s *Storage) Read(ctx context.Context, sha256, ext string) (io.ReadSeekCloser, error) {
	return s.Backend().Read(ctx, sha256, ext)
}

func (s *Storage) Remove(ctx context.Context, sha256, ext string) error {
	return s.Backend().Remove(ctx, sha256, ext)
}

func (s *Storage) Exists(ctx context.Context, sha256 string) (string, bool) {
	return s.Backend().Exists(ctx, sha256)
}

func (s *Storage) PublicURL(sha256, ext string) string {
	return s.Backend().PublicURL(sha256, ext)
}

// Charge books the blob against the uploader's quota. Blobs are stored once per
// content hash, so if two people upload the same file only the first is
// charged — that matches what the storage actually holds.
func (s *Storage) Charge(pubkey, sha256 string, size int64) error {
	return s.store.AddUsage(usagePrefix+sha256, pubkey, size)
}

// Refund releases the quota a blob was holding.
func (s *Storage) Refund(sha256 string) error {
	return s.store.RemoveUsage(usagePrefix + sha256)
}

// CheckQuota is the gate for uploads: the same rules as writing an event of
// that size.
func (s *Storage) CheckQuota(pubkey string, size int64) (bool, string) {
	if max := s.MaxSize(); size > max {
		return false, fmt.Sprintf("file is larger than the %d MB limit", max>>20)
	}
	allowance, err := s.store.Allowance(pubkey)
	if err != nil {
		return false, "could not verify your account"
	}
	if allowance.Account == nil && allowance.Group == nil {
		return false, "not whitelisted — pay for access first"
	}
	ok, _, msg := allowance.CanWrite(time.Now().Unix(), size)
	return ok, msg
}

// Index is the blob ownership index. It reuses khatru's event-based index (a
// kind 24242 event per blob) and adds quota accounting, so Blossom and NIP-96
// uploads land in exactly the same place and are billed once.
type Index struct {
	blossom.EventStoreBlobIndexWrapper
	storage *Storage
	events  *store.Store
}

func (s *Storage) Index(st *store.Store, serviceURL string) *Index {
	return &Index{
		EventStoreBlobIndexWrapper: blossom.EventStoreBlobIndexWrapper{
			Store:      st.Events,
			ServiceURL: serviceURL,
		},
		storage: s,
		events:  st,
	}
}

func (i *Index) Keep(ctx context.Context, blob blossom.BlobDescriptor, pubkey string) error {
	if err := i.EventStoreBlobIndexWrapper.Keep(ctx, blob, pubkey); err != nil {
		return err
	}
	return i.storage.Charge(pubkey, blob.SHA256, int64(blob.Size))
}

// Delete drops one owner's claim on a blob. The quota is only released when
// that owner really held the blob — otherwise a stranger calling DELETE would
// refund storage to the actual uploader while the file stays put.
func (i *Index) Delete(ctx context.Context, sha256 string, pubkey string) error {
	owned, err := i.OwnedBy(ctx, pubkey, sha256)
	if err != nil {
		return err
	}
	if err := i.EventStoreBlobIndexWrapper.Delete(ctx, sha256, pubkey); err != nil {
		return err
	}
	if !owned {
		return nil
	}
	return i.storage.Refund(sha256)
}

// OwnedBy reports whether pubkey uploaded this blob.
func (i *Index) OwnedBy(ctx context.Context, pubkey, sha256 string) (bool, error) {
	ch, err := i.events.Events.QueryEvents(ctx, nostr.Filter{
		Authors: []string{pubkey},
		Kinds:   []int{24242},
		Tags:    nostr.TagMap{"x": []string{sha256}},
		Limit:   1,
	})
	if err != nil {
		return false, err
	}
	found := false
	for range ch {
		found = true
	}
	return found, nil
}

// Owners lists the pubkeys that uploaded a blob.
func (i *Index) Owners(ctx context.Context, sha256 string) ([]string, error) {
	ch, err := i.events.Events.QueryEvents(ctx, nostr.Filter{
		Kinds: []int{24242},
		Tags:  nostr.TagMap{"x": []string{sha256}},
		Limit: 100,
	})
	if err != nil {
		return nil, err
	}
	owners := []string{}
	for evt := range ch {
		owners = append(owners, evt.PubKey)
	}
	return owners, nil
}

// Forget drops every blob a pubkey uploaded, for NIP-62. Bytes that another
// uploader still claims stay on disk — the file is theirs too, and only the
// vanishing user's ownership and quota are unwound.
func (i *Index) Forget(ctx context.Context, pubkey string) (int, error) {
	events, err := i.events.Events.QueryEvents(ctx, nostr.Filter{
		Authors: []string{pubkey},
		Kinds:   []int{24242},
		Limit:   10_000,
	})
	if err != nil {
		return 0, err
	}

	hashes := []string{}
	for evt := range events {
		if hash := evt.Tags.Find("x").Value(); hash != "" {
			hashes = append(hashes, hash)
		}
	}

	forgotten := 0
	for _, hash := range hashes {
		if err := i.Delete(ctx, hash, pubkey); err != nil {
			return forgotten, err
		}
		forgotten++

		owners, err := i.Owners(ctx, hash)
		if err != nil {
			return forgotten, err
		}
		if len(owners) > 0 {
			continue
		}
		ext, exists := i.storage.Exists(ctx, hash)
		if !exists {
			continue
		}
		if err := i.storage.Remove(ctx, hash, ext); err != nil {
			return forgotten, err
		}
	}
	return forgotten, nil
}

// Purge removes a blob from every owner, refunds the quota and deletes the
// stored bytes. This is the admin hammer, used from the panel.
func (i *Index) Purge(ctx context.Context, sha256 string) error {
	owners, err := i.Owners(ctx, sha256)
	if err != nil {
		return err
	}
	for _, owner := range owners {
		if err := i.EventStoreBlobIndexWrapper.Delete(ctx, sha256, owner); err != nil {
			return err
		}
	}
	if err := i.storage.Refund(sha256); err != nil {
		return err
	}
	ext, _ := i.storage.Exists(ctx, sha256)
	return i.storage.Remove(ctx, sha256, ext)
}
