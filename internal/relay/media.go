package relay

import (
	"context"
	"io"
	"net/http"

	"github.com/nbd-wtf/go-nostr"
	"nostrel/internal/relaycore/blossom"

	"nostrel/internal/blobs"
)

// EnableBlossom serves media uploads over the Blossom protocol (BUD-01/02/04),
// authenticated with kind 24242 events. Uploads consume the uploader's storage
// quota, so media and events share one budget.
//
// It must be called after every other route is registered: blossom takes over
// the relay's mux and falls back to the previous one for paths it doesn't own.
func (r *Relay) EnableBlossom(storage *blobs.Storage, index *blobs.Index, serviceURL string) {
	r.storage, r.blobIndex, r.blobServiceURL = storage, index, serviceURL

	bs := blossom.New(r.Relay, serviceURL)
	bs.Store = index

	// media storage also backs the NIP-96 endpoints served by the panel
	r.Info.AddSupportedNIP(96)

	bs.StoreBlob = append(bs.StoreBlob, func(ctx context.Context, sha256 string, ext string, body []byte) error {
		return storage.Write(ctx, sha256, ext, body)
	})
	bs.LoadBlob = append(bs.LoadBlob, func(ctx context.Context, sha256 string, ext string) (io.ReadSeeker, error) {
		f, err := storage.Read(ctx, sha256, ext)
		if err != nil {
			return nil, err // never hand back a typed nil: blossom nil-checks this
		}
		return f, nil
	})
	bs.DeleteBlob = append(bs.DeleteBlob, func(ctx context.Context, sha256 string, ext string) error {
		return storage.Remove(ctx, sha256, ext)
	})

	// when the backend can serve blobs itself (an S3 bucket with a public URL),
	// send clients straight there instead of streaming through the relay
	bs.RedirectGet = append(bs.RedirectGet,
		func(ctx context.Context, sha256 string, ext string) (string, int, error) {
			if url := storage.PublicURL(sha256, ext); url != "" {
				return url, http.StatusFound, nil
			}
			return "", 0, nil
		})

	bs.RejectUpload = append(bs.RejectUpload,
		func(ctx context.Context, auth *nostr.Event, size int, ext string) (bool, string, int) {
			if auth == nil {
				return true, "authorization required", 401
			}
			if ok, reason := storage.CheckQuota(auth.PubKey, int64(size)); !ok {
				return true, reason, 402
			}
			return false, "", 0
		})

	// only the uploader (or an admin) may delete a blob
	bs.RejectDelete = append(bs.RejectDelete,
		func(ctx context.Context, auth *nostr.Event, sha256 string, ext string) (bool, string, int) {
			if auth == nil {
				return true, "authorization required", 401
			}
			if r.IsAdmin(auth.PubKey) {
				return false, "", 0
			}
			owned, err := index.OwnedBy(ctx, auth.PubKey, sha256)
			if err != nil {
				return true, "could not check who owns this blob", 500
			}
			if !owned {
				return true, "you did not upload this blob", 403
			}
			return false, "", 0
		})

	if r.cfg.ReadAuthRequired {
		isMember := func(auth *nostr.Event) (bool, string, int) {
			if auth == nil || !r.whitelisted(auth.PubKey) {
				return true, "this relay only serves media to its members", 401
			}
			return false, "", 0
		}
		bs.RejectGet = append(bs.RejectGet,
			func(ctx context.Context, auth *nostr.Event, _ string, _ string) (bool, string, int) {
				return isMember(auth)
			})
		bs.RejectList = append(bs.RejectList,
			func(ctx context.Context, auth *nostr.Event, _ string) (bool, string, int) {
				return isMember(auth)
			})
	}
}

// whitelisted reports whether the relay knows this pubkey at all — through
// their own account or through a group they share storage with.
func (r *Relay) whitelisted(pubkey string) bool {
	allowance, err := r.store.Allowance(pubkey)
	return err == nil && allowance.Whitelisted()
}
