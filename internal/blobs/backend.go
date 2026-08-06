package blobs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"nostrel/internal/store"
)

// Backend is where blob bytes actually live. The quota accounting and the
// ownership index sit above it, so swapping the backend does not change how
// media is billed or who owns what.
type Backend interface {
	Name() string
	Write(ctx context.Context, sha256, ext string, body []byte) error
	// Read must return a seekable reader: http.ServeContent needs to seek to
	// answer range requests.
	Read(ctx context.Context, sha256, ext string) (io.ReadSeekCloser, error)
	Remove(ctx context.Context, sha256, ext string) error
	// Exists reports whether the blob is stored and under which extension.
	Exists(ctx context.Context, sha256 string) (string, bool)
	// Check verifies the backend is usable: it writes, reads back and deletes a
	// probe object.
	Check(ctx context.Context) (string, error)
	// PublicURL returns a URL clients can be redirected to, or "" when blobs
	// must be streamed through the relay.
	PublicURL(sha256, ext string) string
}

// BuildBackend makes the storage backend described by settings. localDir is the
// filesystem path from the environment, used when the backend is "local".
func BuildBackend(settings store.Settings, localDir string) (Backend, error) {
	switch settings.StorageBackend {
	case "", "local":
		dir := settings.LocalPath
		if dir == "" {
			dir = localDir
		}
		return NewLocal(dir)
	case "s3":
		return NewS3(settings)
	default:
		return nil, fmt.Errorf("unknown storage backend %q", settings.StorageBackend)
	}
}

// Local keeps blobs as files in one directory, named by their hash.
type Local struct {
	dir string
}

func NewLocal(dir string) (*Local, error) {
	if dir == "" {
		return nil, fmt.Errorf("local storage needs a directory")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("blob directory: %w", err)
	}
	return &Local{dir: dir}, nil
}

func (l *Local) Name() string { return "local:" + l.dir }

// path confines the blob to the storage directory: the hash and the extension
// both come from requests, so neither is trusted as a path.
func (l *Local) path(sha256, ext string) string {
	name := filepath.Base(sha256)
	if ext = cleanExt(ext); ext != "" {
		name += "." + ext
	}
	return filepath.Join(l.dir, name)
}

func (l *Local) Write(_ context.Context, sha256, ext string, body []byte) error {
	return os.WriteFile(l.path(sha256, ext), body, 0o600)
}

// Read opens a blob, falling back to whatever extension it was stored under
// when the caller asks for a different one.
func (l *Local) Read(ctx context.Context, sha256, ext string) (io.ReadSeekCloser, error) {
	f, err := os.Open(l.path(sha256, ext))
	if err == nil {
		return f, nil
	}
	if stored, ok := l.Exists(ctx, sha256); ok {
		if f, fallbackErr := os.Open(l.path(sha256, stored)); fallbackErr == nil {
			return f, nil
		}
	}
	return nil, err // a nil interface, never a nil *os.File
}

func (l *Local) Remove(ctx context.Context, sha256, ext string) error {
	if err := os.Remove(l.path(sha256, ext)); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if stored, ok := l.Exists(ctx, sha256); ok {
		if err := os.Remove(l.path(sha256, stored)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (l *Local) Exists(_ context.Context, sha256 string) (string, bool) {
	matches, err := filepath.Glob(l.path(sha256, "") + "*")
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return strings.TrimPrefix(filepath.Ext(matches[0]), "."), true
}

func (l *Local) Check(ctx context.Context) (string, error) {
	probe := filepath.Join(l.dir, ".nostrel-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return "", fmt.Errorf("cannot write to %s: %w", l.dir, err)
	}
	if _, err := os.ReadFile(probe); err != nil {
		return "", fmt.Errorf("cannot read from %s: %w", l.dir, err)
	}
	if err := os.Remove(probe); err != nil {
		return "", fmt.Errorf("cannot delete from %s: %w", l.dir, err)
	}
	return "writing to " + l.dir, nil
}

func (l *Local) PublicURL(string, string) string { return "" }

// CopyAll copies every blob in a local directory into another backend,
// skipping ones already present. It is what the migrate-blobs command runs.
func CopyAll(ctx context.Context, source *Local, target Backend, dir string) (copied, skipped int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		hash := strings.TrimSuffix(name, filepath.Ext(name))
		if len(hash) != 64 {
			continue // not a blob
		}
		ext := filepath.Ext(name)

		if _, exists := target.Exists(ctx, hash); exists {
			skipped++
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return copied, skipped, fmt.Errorf("reading %s: %w", name, err)
		}
		if err := target.Write(ctx, hash, ext, body); err != nil {
			return copied, skipped, fmt.Errorf("writing %s: %w", name, err)
		}
		copied++
	}
	return copied, skipped, nil
}

// cleanExt returns a lowercase alphanumeric extension without its dot, or ""
// if there isn't a usable one. Callers pass extensions both with and without a
// leading dot, which is why this is shared by every backend.
func cleanExt(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if ext == "" || len(ext) > 10 {
		return ""
	}
	for _, c := range ext {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return ""
		}
	}
	return ext
}

// objectKey is the name a blob is stored under on object storage.
func objectKey(sha256, ext string) string {
	if ext = cleanExt(ext); ext != "" {
		return sha256 + "." + ext
	}
	return sha256
}
