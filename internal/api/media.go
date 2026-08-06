package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	"nostrel/internal/relaycore/blossom"

	"nostrel/internal/store"
)

// NIP-96 HTTP file storage. Blobs live in the same store Blossom uses, so a
// file uploaded through either protocol is downloadable through both and is
// billed to the uploader's storage quota once.

func (s *Server) registerMedia(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/nostr/nip96.json", s.handleNIP96Info)
	mux.HandleFunc("POST /nip96", s.handleNIP96Upload)
	mux.HandleFunc("DELETE /nip96/{sha256}", s.handleNIP96Delete)

	mux.HandleFunc("POST /api/admin/storage/test", s.admin(s.handleStorageTest))
	mux.HandleFunc("GET /api/admin/blobs", s.admin(s.handleListBlobs))
	mux.HandleFunc("DELETE /api/admin/blobs/{sha256}", s.admin(s.handleAdminDeleteBlob))
	mux.HandleFunc("GET /api/admin/blobs/reports", s.admin(s.handleListBlobReports))
	mux.HandleFunc("DELETE /api/admin/blobs/reports/{sha256}", s.admin(s.handleDismissBlobReport))
}

// handleListBlobs lists stored media, newest first, from the blob index events.
func (s *Server) handleListBlobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	events, err := s.store.Events.QueryEvents(r.Context(), nostr.Filter{
		Kinds: []int{24242},
		Limit: limit,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list media")
		return
	}

	type blob struct {
		SHA256   string `json:"sha256"`
		Pubkey   string `json:"pubkey"`
		Size     int64  `json:"size"`
		Type     string `json:"type"`
		Uploaded int64  `json:"uploaded"`
		Reported bool   `json:"reported"`
		URL      string `json:"url"`
	}

	blobs := []blob{}
	for evt := range events {
		b := blob{Pubkey: evt.PubKey, Uploaded: int64(evt.CreatedAt)}
		for _, tag := range evt.Tags {
			if len(tag) < 2 {
				continue
			}
			switch tag[0] {
			case "x":
				b.SHA256 = tag[1]
			case "type":
				b.Type = tag[1]
			case "size":
				b.Size, _ = strconv.ParseInt(tag[1], 10, 64)
			}
		}
		if b.SHA256 == "" {
			continue
		}
		ext, _ := s.blobs.Exists(r.Context(), b.SHA256)
		b.URL = s.cfg.PanelURL + "/" + b.SHA256
		if ext != "" {
			b.URL += "." + ext
		}
		b.Reported = s.store.ModHas(store.ModReportedBlob, b.SHA256)
		blobs = append(blobs, b)
	}
	writeJSON(w, http.StatusOK, blobs)
}

// handleAdminDeleteBlob removes a blob from every owner and from disk.
func (s *Server) handleAdminDeleteBlob(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("sha256")
	if len(hash) != 64 {
		writeErr(w, http.StatusBadRequest, "expected a sha256 hex digest")
		return
	}
	if err := s.blobIndex.Purge(r.Context(), hash); err != nil {
		s.log.Printf("purging blob %s: %v", hash, err)
		writeErr(w, http.StatusInternalServerError, "could not delete the file")
		return
	}
	if err := s.store.ModRemove(store.ModReportedBlob, hash); err != nil {
		s.log.Printf("clearing blob report %s: %v", hash, err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListBlobReports(w http.ResponseWriter, r *http.Request) {
	reports, err := s.store.ModList(store.ModReportedBlob)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list reports")
		return
	}
	writeJSON(w, http.StatusOK, reports)
}

// handleDismissBlobReport clears a report without touching the file.
func (s *Server) handleDismissBlobReport(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ModRemove(store.ModReportedBlob, r.PathValue("sha256")); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not clear the report")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNIP96Info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"api_url":        s.cfg.PanelURL + "/nip96",
		"download_url":   s.cfg.PanelURL,
		"supported_nips": []int{96, 98},
		"tos_url":        s.cfg.PanelURL,
		"content_types":  []string{"image/*", "video/*", "audio/*", "application/pdf"},
		"plans": map[string]any{
			"free": map[string]any{
				"name":              "member",
				"is_nip98_required": true,
				"url":               s.cfg.PanelURL,
				"max_byte_size":     s.blobs.MaxSize(),
			},
		},
	})
}

func (s *Server) handleNIP96Upload(w http.ResponseWriter, r *http.Request) {
	if s.blobs == nil {
		writeErr(w, http.StatusNotFound, "media storage is disabled")
		return
	}

	// NIP-98 covers the body hash, which multipart bodies can't reproduce
	// meaningfully, so the signature is checked over the request line only.
	pubkey, err := s.VerifyNIP98(r, nil)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "nip-98 authorization required")
		return
	}

	if err := r.ParseMultipartForm(s.blobs.MaxSize()); err != nil {
		writeErr(w, http.StatusBadRequest, "expected a multipart form with a 'file' field")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "expected a multipart form with a 'file' field")
		return
	}
	defer file.Close()

	if ok, reason := s.blobs.CheckQuota(pubkey, header.Size); !ok {
		writeErr(w, http.StatusPaymentRequired, reason)
		return
	}

	body, err := io.ReadAll(io.LimitReader(file, s.blobs.MaxSize()+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read the uploaded file")
		return
	}
	if int64(len(body)) > s.blobs.MaxSize() {
		writeErr(w, http.StatusRequestEntityTooLarge, "file is too large")
		return
	}
	// re-check with the real size: Content-Length is whatever the client claimed
	if ok, reason := s.blobs.CheckQuota(pubkey, int64(len(body))); !ok {
		writeErr(w, http.StatusPaymentRequired, reason)
		return
	}

	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	ext := strings.ToLower(filepath.Ext(header.Filename))
	mimeType := header.Header.Get("Content-Type")
	if ext == "" && mimeType != "" {
		if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(ext)
	}

	if err := s.blobs.Write(r.Context(), hash, ext, body); err != nil {
		s.log.Printf("storing blob %s: %v", hash, err)
		writeErr(w, http.StatusInternalServerError, "could not store the file")
		return
	}
	// index it the same way Blossom does, which also books the quota
	if err := s.blobIndex.Keep(r.Context(), blossom.BlobDescriptor{
		URL:      s.cfg.PanelURL + "/" + hash + ext,
		SHA256:   hash,
		Size:     len(body),
		Type:     mimeType,
		Uploaded: nostr.Now(),
	}, pubkey); err != nil {
		s.log.Printf("indexing blob %s: %v", hash, err)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":  "success",
		"message": "uploaded",
		"nip94_event": map[string]any{
			"tags": [][]string{
				{"url", s.cfg.PanelURL + "/" + hash + ext},
				{"ox", hash},
				{"x", hash},
				{"m", mimeType},
				{"size", strconv.Itoa(len(body))},
			},
			"content": "",
		},
	})
}

func (s *Server) handleNIP96Delete(w http.ResponseWriter, r *http.Request) {
	if s.blobs == nil {
		writeErr(w, http.StatusNotFound, "media storage is disabled")
		return
	}
	pubkey, err := s.VerifyNIP98(r, nil)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "nip-98 authorization required")
		return
	}

	hash := r.PathValue("sha256")
	if len(hash) != 64 {
		writeErr(w, http.StatusBadRequest, "expected a sha256 hex digest")
		return
	}

	// only the uploader (or an admin) may delete; ownership lives in the
	// blossom blob index, which is keyed by pubkey
	owned, err := s.blobIndex.OwnedBy(r.Context(), pubkey, hash)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not check blob ownership")
		return
	}
	if !owned && !s.store.IsAdmin(pubkey, s.cfg.AdminPubkeys) {
		writeErr(w, http.StatusForbidden, "you did not upload this file")
		return
	}

	ext, _ := s.blobs.Exists(r.Context(), hash)
	if err := s.blobs.Remove(r.Context(), hash, ext); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete the file")
		return
	}
	// drops the index event and refunds the quota to whoever uploaded it
	if err := s.blobIndex.Delete(r.Context(), hash, pubkey); err != nil {
		s.log.Printf("de-indexing blob %s: %v", hash, err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "message": "deleted"})
}
