package blobs

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"nostrel/internal/relaycore/blossom"

	"nostrel/internal/store"
)

const pubkey = "aa11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"
const hash = "7bb44abb2303a881ba292674df0eeeff2648bd001d4cf157443a5777e0cdb000"

func newStorage(t *testing.T) (*Storage, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(st.Close)

	s, err := New(st, filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatalf("creating storage: %v", err)
	}
	return s, st
}

// local returns the filesystem backend behind a test Storage.
func local(t *testing.T, s *Storage) *Local {
	t.Helper()
	backend, ok := s.Backend().(*Local)
	if !ok {
		t.Fatalf("expected the local backend, got %T", s.Backend())
	}
	return backend
}

func TestExtensionsAreNormalised(t *testing.T) {
	storage, _ := newStorage(t)
	s := local(t, storage)

	// Blossom hands the extension over with a dot on upload and without one on
	// download; both must land on the same file.
	if withDot, without := s.path(hash, ".txt"), s.path(hash, "txt"); withDot != without {
		t.Errorf("path(.txt) = %q, path(txt) = %q, want the same file", withDot, without)
	}
	// path traversal and junk extensions are dropped
	for _, ext := range []string{"../../etc/passwd", ".exe/../..", "verylongextension", ".t x t"} {
		if got := s.path(hash, ext); got != s.path(hash, "") {
			t.Errorf("path(%q) = %q, want the extension dropped", ext, got)
		}
	}
	if got := s.path("../../etc/passwd", ""); filepath.Base(got) != "passwd" {
		t.Errorf("path with a traversing hash = %q, want it confined to the blob directory", got)
	}
}

func TestReadFallsBackToTheStoredExtension(t *testing.T) {
	s, _ := newStorage(t)
	ctx := context.Background()
	if err := s.Write(ctx, hash, ".conf", []byte("payload")); err != nil {
		t.Fatal(err)
	}

	// the uploader's guessed extension (.conf) differs from what a client asks
	// for (.txt), which must still resolve
	f, err := s.Read(ctx, hash, ".txt")
	if err != nil {
		t.Fatalf("read with a mismatched extension: %v", err)
	}
	defer f.Close()
	body, _ := io.ReadAll(f)
	if string(body) != "payload" {
		t.Errorf("read %q, want %q", body, "payload")
	}
}

func TestReadMissingBlobReturnsNilReader(t *testing.T) {
	s, _ := newStorage(t)

	// a typed nil here makes blossom's `reader != nil` check pass and blows up
	// inside http.ServeContent
	f, err := s.Read(context.Background(), hash, ".txt")
	if err == nil {
		t.Fatal("expected an error for a missing blob")
	}
	if f != nil {
		t.Error("reader must be a nil interface when the blob is missing")
	}
}

func TestQuotaAndRefund(t *testing.T) {
	s, st := newStorage(t)
	if _, err := st.EnsureAccount(pubkey); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateAccount(pubkey, store.StatusActive, 0, 1000, ""); err != nil {
		t.Fatal(err)
	}

	if ok, reason := s.CheckQuota(pubkey, 900); !ok {
		t.Errorf("900 bytes into a 1000 byte quota was rejected: %s", reason)
	}
	if ok, _ := s.CheckQuota(pubkey, 1001); ok {
		t.Error("an upload over the quota was accepted")
	}
	if ok, _ := s.CheckQuota(pubkey, s.MaxSize()+1); ok {
		t.Error("an upload over the file size limit was accepted")
	}
	if ok, _ := s.CheckQuota("unknown"+pubkey[7:], 10); ok {
		t.Error("an upload from a pubkey with no account was accepted")
	}

	if err := s.Charge(pubkey, hash, 900); err != nil {
		t.Fatal(err)
	}
	if acct, _ := st.Account(pubkey); acct.UsedBytes != 900 {
		t.Errorf("used = %d, want 900", acct.UsedBytes)
	}
	if ok, _ := s.CheckQuota(pubkey, 200); ok {
		t.Error("an upload that would exceed the remaining quota was accepted")
	}

	if err := s.Refund(hash); err != nil {
		t.Fatal(err)
	}
	if acct, _ := st.Account(pubkey); acct.UsedBytes != 0 {
		t.Errorf("used after refund = %d, want 0", acct.UsedBytes)
	}
}

func TestRemoveDeletesWhicheverExtensionIsStored(t *testing.T) {
	s, _ := newStorage(t)
	ctx := context.Background()
	if err := s.Write(ctx, hash, ".conf", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(ctx, hash, ".txt"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if ext, ok := s.Exists(ctx, hash); ok {
		t.Errorf("blob still present with extension %q", ext)
	}
	if err := s.Remove(ctx, hash, ".txt"); err != nil {
		t.Errorf("removing an absent blob should be a no-op, got %v", err)
	}
}

func TestDeleteOnlyRefundsTheOwner(t *testing.T) {
	s, st := newStorage(t)
	if _, err := st.EnsureAccount(pubkey); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateAccount(pubkey, store.StatusActive, 0, 1<<20, ""); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	index := s.Index(st, "https://relay.example.com")
	descriptor := blossom.BlobDescriptor{SHA256: hash, Size: 500, Type: "text/plain", Uploaded: nostr.Now()}
	if err := index.Keep(ctx, descriptor, pubkey); err != nil {
		t.Fatalf("keep: %v", err)
	}
	if acct, _ := st.Account(pubkey); acct.UsedBytes != 500 {
		t.Fatalf("used = %d, want 500 after upload", acct.UsedBytes)
	}

	// a stranger calling DELETE must not hand the uploader's quota back
	stranger := "cc11bb22cc33dd44ee55ff6600778899aabbccddeeff00112233445566778899"
	if err := index.Delete(ctx, hash, stranger); err != nil {
		t.Fatalf("delete by a stranger: %v", err)
	}
	if acct, _ := st.Account(pubkey); acct.UsedBytes != 500 {
		t.Errorf("used = %d after a stranger's delete, want 500 (no refund)", acct.UsedBytes)
	}

	if err := index.Delete(ctx, hash, pubkey); err != nil {
		t.Fatalf("delete by the owner: %v", err)
	}
	if acct, _ := st.Account(pubkey); acct.UsedBytes != 0 {
		t.Errorf("used = %d after the owner's delete, want 0", acct.UsedBytes)
	}
}

func TestPurgeRemovesEverything(t *testing.T) {
	s, st := newStorage(t)
	if _, err := st.EnsureAccount(pubkey); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateAccount(pubkey, store.StatusActive, 0, 1<<20, ""); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	index := s.Index(st, "https://relay.example.com")
	if err := s.Write(ctx, hash, ".txt", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := index.Keep(ctx, blossom.BlobDescriptor{SHA256: hash, Size: 7, Uploaded: nostr.Now()}, pubkey); err != nil {
		t.Fatal(err)
	}

	if err := index.Purge(ctx, hash); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if _, ok := s.Exists(ctx, hash); ok {
		t.Error("the file is still on disk after a purge")
	}
	if acct, _ := st.Account(pubkey); acct.UsedBytes != 0 {
		t.Errorf("used = %d after a purge, want 0", acct.UsedBytes)
	}
	if owners, _ := index.Owners(ctx, hash); len(owners) != 0 {
		t.Errorf("owners = %v after a purge, want none", owners)
	}
}
