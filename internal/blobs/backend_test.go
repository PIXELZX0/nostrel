package blobs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"nostrel/internal/store"
)

func TestBuildBackendSelection(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name     string
		settings store.Settings
		wantErr  bool
	}{
		{"empty means local", store.Settings{}, false},
		{"explicit local", store.Settings{StorageBackend: "local"}, false},
		{"local with its own path", store.Settings{StorageBackend: "local", LocalPath: filepath.Join(dir, "other")}, false},
		{"s3 fully configured", store.Settings{
			StorageBackend: "s3", S3Endpoint: "https://s3.example.com", S3Bucket: "media",
			S3AccessKey: "key", S3SecretKey: "secret",
		}, false},
		{"s3 without a bucket", store.Settings{
			StorageBackend: "s3", S3Endpoint: "https://s3.example.com", S3AccessKey: "key", S3SecretKey: "secret",
		}, true},
		{"s3 without credentials", store.Settings{
			StorageBackend: "s3", S3Endpoint: "https://s3.example.com", S3Bucket: "media",
		}, true},
		{"s3 without an endpoint", store.Settings{
			StorageBackend: "s3", S3Bucket: "media", S3AccessKey: "key", S3SecretKey: "secret",
		}, true},
		{"unknown backend", store.Settings{StorageBackend: "ftp"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildBackend(tc.settings, dir)
			if tc.wantErr && err == nil {
				t.Error("expected an error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseEndpoint(t *testing.T) {
	cases := []struct {
		raw        string
		host       string
		wantSecure bool
	}{
		{"https://s3.amazonaws.com", "s3.amazonaws.com", true},
		{"http://127.0.0.1:9000", "127.0.0.1:9000", false},
		{"minio.example.com:9000", "minio.example.com:9000", true},
		{"https://s3.example.com/", "s3.example.com", true},
	}
	for _, tc := range cases {
		host, secure := parseEndpoint(tc.raw)
		if host != tc.host || secure != tc.wantSecure {
			t.Errorf("parseEndpoint(%q) = %q/%v, want %q/%v", tc.raw, host, secure, tc.host, tc.wantSecure)
		}
	}
}

func TestStorageReloadSwitchesBackend(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)

	storage, err := New(st, filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	first := storage.Backend()
	if _, ok := first.(*Local); !ok {
		t.Fatalf("expected the local backend by default, got %T", first)
	}

	// unchanged settings keep the same instance
	if err := storage.Reload(); err != nil {
		t.Fatal(err)
	}
	if storage.Backend() != first {
		t.Error("the backend was rebuilt even though the settings did not change")
	}

	// an admin points the relay at a bucket
	settings, _ := st.Settings()
	settings.StorageBackend = "s3"
	settings.S3Endpoint = "https://s3.example.com"
	settings.S3Bucket = "media"
	settings.S3AccessKey = "key"
	settings.S3SecretKey = "secret"
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := storage.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := storage.Backend().(*S3); !ok {
		t.Errorf("backend = %T, want the S3 backend (no restart needed)", storage.Backend())
	}
}

func TestCopyAllSkipsNonBlobsAndDuplicates(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	ctx := context.Background()

	blobName := hash + ".txt"
	if err := os.WriteFile(filepath.Join(source, blobName), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	// things that are not blobs must be left behind
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".hidden"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	from, err := NewLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	to, err := NewLocal(target)
	if err != nil {
		t.Fatal(err)
	}

	copied, skipped, err := CopyAll(ctx, from, to, source)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if copied != 1 || skipped != 0 {
		t.Errorf("copied %d skipped %d, want 1 and 0", copied, skipped)
	}
	if _, ok := to.Exists(ctx, hash); !ok {
		t.Error("the blob did not arrive at the destination")
	}

	// running it again must not copy anything twice
	copied, skipped, err = CopyAll(ctx, from, to, source)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 0 || skipped != 1 {
		t.Errorf("second run copied %d skipped %d, want 0 and 1", copied, skipped)
	}
}
