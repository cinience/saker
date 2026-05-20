package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mojatter/s2"

	storagecfg "github.com/saker-ai/saker/pkg/storage"
)

const maxUploadSize = 50 << 20 // 50 MB

// allowedMediaPrefixes lists the MIME type prefixes accepted for upload.
var allowedMediaPrefixes = []string{"image/", "video/", "audio/", "application/pdf"}

// uploadResponse is returned on successful upload.
type uploadResponse struct {
	Path      string `json:"path"`       // URL path to access the file
	Name      string `json:"name"`       // original filename
	MediaType string `json:"media_type"` // detected MIME type
	Size      int64  `json:"size"`       // file size in bytes
}

// handleUpload accepts a multipart file upload and persists it to the
// configured object store (osfs/S3/OSS). The file is stored under a
// content-addressed key (SHA-256), providing automatic deduplication and
// seamless backend switching via configuration.
//
// POST /api/upload  —  form field "file".
//
// @Summary Upload file
// @Description Accepts a multipart file upload (max 50 MB) for media files (images, video, audio, PDF). The file is persisted to the configured object store and a response with the URL path is returned.
// @Tags files
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "File to upload"
// @Success 200 {object} uploadResponse "Upload successful"
// @Failure 400 {string} string "missing file field or unsupported file type"
// @Failure 413 {string} string "file too large (max 50MB)"
// @Failure 500 {string} string "internal error"
// @Router /api/upload [post]
func (s *Server) handleUpload(c *gin.Context) {
	r := c.Request
	w := c.Writer

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+1024)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "file too large (max 50MB)", http.StatusRequestEntityTooLarge)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Stream hash: compute SHA-256 during the initial read instead of a
	// separate pass over the buffer afterwards.
	hasher := sha256.New()
	data, err := io.ReadAll(io.TeeReader(file, hasher))
	if err != nil {
		s.logger.Error("failed to read upload", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	mediaType := http.DetectContentType(data[:min(512, len(data))])
	if mediaType == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		mediaType = mime.TypeByExtension(ext)
	}
	if mediaType == "" {
		mediaType = header.Header.Get("Content-Type")
	}
	if !isAllowedMedia(mediaType) {
		http.Error(w, fmt.Sprintf("unsupported file type: %s", mediaType), http.StatusBadRequest)
		return
	}

	safeFilename := sanitizeFilename(header.Filename)

	store, cfg := s.handler.objectStoreSnapshot()
	if store == nil {
		s.logger.Error("object store not initialized")
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	url, err := s.uploadToStore(r.Context(), store, cfg, data, hasher, safeFilename, mediaType)
	if err != nil {
		s.logger.Error("upload to store failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.logger.Info("file uploaded", "name", header.Filename, "size", len(data), "media_type", mediaType, "path", url)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(uploadResponse{
		Path:      url,
		Name:      header.Filename,
		MediaType: mediaType,
		Size:      int64(len(data)),
	})
}

// uploadToStore persists file data to the object store using content-addressed
// key (sha256). Deduplicates automatically — identical files share the same key.
// The hasher must already contain the complete digest of data (via TeeReader).
func (s *Server) uploadToStore(ctx context.Context, store s2.Storage, cfg storagecfg.Config, data []byte, hasher hash.Hash, filename, mediaType string) (string, error) {
	sha := hex.EncodeToString(hasher.Sum(nil))
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = extFromMime(mediaType)
	}
	bucket := classifyMedia(mediaType, "")
	key := cfg.Key("_default", bucket, sha, ext)

	exists, _ := store.Exists(ctx, key)
	if !exists {
		md := s2.Metadata{}
		md.Set("Content-Type", mediaType)
		md.Set("X-Original-Name", filename)
		obj := s2.NewObjectBytes(key, data, s2.WithMetadata(md))
		if err := store.Put(ctx, obj); err != nil {
			return "", fmt.Errorf("storage put %s: %w", key, err)
		}
	}

	return cfg.PublicURL(key), nil
}

// extFromMime returns a file extension for common MIME types when the
// original filename lacks one.
func extFromMime(mediaType string) string {
	switch {
	case strings.HasPrefix(mediaType, "image/png"):
		return ".png"
	case strings.HasPrefix(mediaType, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mediaType, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mediaType, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mediaType, "video/mp4"):
		return ".mp4"
	case strings.HasPrefix(mediaType, "audio/mpeg"):
		return ".mp3"
	case strings.HasPrefix(mediaType, "audio/wav"):
		return ".wav"
	case mediaType == "application/pdf":
		return ".pdf"
	default:
		return ""
	}
}

func isAllowedMedia(mediaType string) bool {
	for _, prefix := range allowedMediaPrefixes {
		if strings.HasPrefix(mediaType, prefix) {
			return true
		}
	}
	return false
}

// sanitizeFilename removes path separators and other dangerous characters.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "..", "")
	var b strings.Builder
	for _, r := range name {
		if r == '/' || r == '\\' || r == '\x00' {
			continue
		}
		b.WriteRune(r)
	}
	result := b.String()
	if result == "" {
		result = "upload"
	}
	return result
}
