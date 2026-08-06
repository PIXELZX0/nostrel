package blobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"nostrel/internal/store"
)

// S3 stores blobs on any S3-compatible service: AWS S3, MinIO, Cloudflare R2,
// Backblaze B2 and so on.
type S3 struct {
	client    *minio.Client
	bucket    string
	prefix    string
	publicURL string
	endpoint  string
}

func NewS3(settings store.Settings) (*S3, error) {
	if settings.S3Bucket == "" {
		return nil, errors.New("s3 storage needs a bucket")
	}
	if settings.S3AccessKey == "" || settings.S3SecretKey == "" {
		return nil, errors.New("s3 storage needs an access key and a secret key")
	}

	endpoint, secure := parseEndpoint(settings.S3Endpoint)
	if endpoint == "" {
		return nil, errors.New("s3 storage needs an endpoint")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(settings.S3AccessKey, settings.S3SecretKey, ""),
		Secure: secure,
		Region: settings.S3Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	return &S3{
		client:    client,
		bucket:    settings.S3Bucket,
		prefix:    strings.Trim(settings.S3Prefix, "/"),
		publicURL: strings.TrimRight(settings.S3PublicURL, "/"),
		endpoint:  endpoint,
	}, nil
}

// parseEndpoint accepts "https://host", "host:9000" or "host" and reports
// whether TLS should be used. Plain hosts default to https.
func parseEndpoint(raw string) (host string, secure bool) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "https://"):
		return strings.TrimRight(strings.TrimPrefix(raw, "https://"), "/"), true
	case strings.HasPrefix(raw, "http://"):
		return strings.TrimRight(strings.TrimPrefix(raw, "http://"), "/"), false
	case raw == "":
		return "", true
	default:
		return strings.TrimRight(raw, "/"), true
	}
}

func (s *S3) Name() string { return "s3:" + s.endpoint + "/" + s.bucket }

func (s *S3) key(sha256, ext string) string {
	key := objectKey(sha256, ext)
	if s.prefix != "" {
		return s.prefix + "/" + key
	}
	return key
}

func (s *S3) Write(ctx context.Context, sha256, ext string, body []byte) error {
	_, err := s.client.PutObject(ctx, s.bucket, s.key(sha256, ext),
		bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{})
	return err
}

func (s *S3) Read(ctx context.Context, sha256, ext string) (io.ReadSeekCloser, error) {
	obj, err := s.open(ctx, s.key(sha256, ext))
	if err == nil {
		return obj, nil
	}
	// the caller's extension may not be the one it was stored under
	if stored, ok := s.Exists(ctx, sha256); ok {
		if obj, fallbackErr := s.open(ctx, s.key(sha256, stored)); fallbackErr == nil {
			return obj, nil
		}
	}
	return nil, err
}

// open fetches an object and confirms it exists: minio only reports a missing
// object once you touch it.
func (s *S3) open(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		return nil, err
	}
	return obj, nil
}

func (s *S3) Remove(ctx context.Context, sha256, ext string) error {
	stored, ok := s.Exists(ctx, sha256)
	if !ok {
		return nil
	}
	return s.client.RemoveObject(ctx, s.bucket, s.key(sha256, stored), minio.RemoveObjectOptions{})
}

// Exists lists by hash prefix, which is how the extension a blob was stored
// with is recovered.
func (s *S3) Exists(ctx context.Context, sha256 string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	prefix := s.key(sha256, "")
	for object := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:  prefix,
		MaxKeys: 1,
	}) {
		if object.Err != nil {
			return "", false
		}
		name := strings.TrimPrefix(object.Key, prefix)
		return strings.TrimPrefix(name, "."), true
	}
	return "", false
}

// Check proves the credentials can write, read and delete in the bucket, which
// is what actually matters for uploads.
func (s *S3) Check(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	key := s.key("nostrel-probe", "txt")
	payload := []byte("nostrel storage probe")

	if _, err := s.client.PutObject(ctx, s.bucket, key,
		bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{}); err != nil {
		return "", fmt.Errorf("cannot write to the bucket: %w", err)
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot read from the bucket: %w", err)
	}
	got, err := io.ReadAll(obj)
	obj.Close()
	if err != nil {
		return "", fmt.Errorf("cannot read from the bucket: %w", err)
	}
	if !bytes.Equal(got, payload) {
		return "", errors.New("the bucket returned different bytes than were written")
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return "", fmt.Errorf("cannot delete from the bucket: %w", err)
	}
	return fmt.Sprintf("read, wrote and deleted a probe object in %q", s.bucket), nil
}

// PublicURL points at the bucket's own public address when one is configured,
// letting downloads skip the relay entirely.
func (s *S3) PublicURL(sha256, ext string) string {
	if s.publicURL == "" {
		return ""
	}
	return s.publicURL + "/" + s.key(sha256, ext)
}
