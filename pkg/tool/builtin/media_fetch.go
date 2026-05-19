package toolbuiltin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type mediaDirKey struct{}

type mediaDir struct {
	abs string // absolute path for writing files
	rel string // project-relative path for return values
}

// WithMediaDir injects a media storage directory into the context.
// absDir is the absolute write path; relDir is the project-relative prefix
// used in returned paths (e.g. ".saker/media").
func WithMediaDir(ctx context.Context, absDir, relDir string) context.Context {
	if absDir == "" {
		return ctx
	}
	return context.WithValue(ctx, mediaDirKey{}, mediaDir{abs: absDir, rel: relDir})
}

// mediaDirFromContext retrieves the media directory config from context.
func mediaDirFromContext(ctx context.Context) (absDir, relDir string) {
	if ctx == nil {
		return "", ""
	}
	md, _ := ctx.Value(mediaDirKey{}).(mediaDir)
	return md.abs, md.rel
}

const (
	mediaFetchTimeout  = 2 * time.Minute
	mediaFetchMaxBytes = 200 << 20 // 200 MB

	defaultMediaMaxFiles      = 200
	defaultMediaMaxTotalBytes = 500 << 20 // 500 MB
	defaultMediaMaxAge        = 7 * 24 * time.Hour
)

// CleanupMediaDir removes stale files from the media directory.
// Cleanup order: files older than maxAge, then oldest files when count exceeds
// maxFiles, then largest-first when total size exceeds maxTotalBytes.
func CleanupMediaDir(dir string) {
	cleanupMedia(dir, defaultMediaMaxFiles, defaultMediaMaxTotalBytes, defaultMediaMaxAge)
}

func cleanupMedia(dir string, maxFiles int, maxTotalBytes int64, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type fileEntry struct {
		path  string
		size  int64
		mtime time.Time
	}

	var files []fileEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{
			path:  filepath.Join(dir, e.Name()),
			size:  info.Size(),
			mtime: info.ModTime(),
		})
	}

	// Phase 1: delete files older than maxAge.
	cutoff := time.Now().Add(-maxAge)
	var remaining []fileEntry
	for _, f := range files {
		if f.mtime.Before(cutoff) {
			os.Remove(f.path)
		} else {
			remaining = append(remaining, f)
		}
	}
	files = remaining

	// Phase 2: delete oldest files when count exceeds maxFiles.
	if len(files) > maxFiles {
		sort.Slice(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })
		excess := len(files) - maxFiles
		for i := 0; i < excess; i++ {
			os.Remove(files[i].path)
		}
		files = files[excess:]
	}

	// Phase 3: delete largest files when total size exceeds maxTotalBytes.
	var totalSize int64
	for _, f := range files {
		totalSize += f.size
	}
	if totalSize > maxTotalBytes {
		sort.Slice(files, func(i, j int) bool { return files[i].size > files[j].size })
		for i := 0; i < len(files) && totalSize > maxTotalBytes; i++ {
			totalSize -= files[i].size
			os.Remove(files[i].path)
		}
	}
}

// resolveMediaPath checks whether path is an HTTP(S) URL. If so it downloads
// the resource to a content-addressed temp file and returns the local path.
// Otherwise the input is returned unchanged. Files are named by content hash
// so repeated downloads of the same resource are no-ops and the file remains
// available for subsequent tool calls within the same session.
func ResolveMediaPath(ctx context.Context, path string) (localPath string, err error) {
	trimmed := strings.TrimSpace(path)
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return path, nil
	}

	data, mediaType, err := fetchMediaURL(ctx, trimmed)
	if err != nil {
		return "", fmt.Errorf("fetch media URL: %w", err)
	}

	ext := extensionForMediaType(mediaType, trimmed)
	hash := sha256.Sum256(data)
	fileName := "saker-media-" + hex.EncodeToString(hash[:8]) + ext

	absDir, relDir := mediaDirFromContext(ctx)
	if absDir == "" {
		absDir = os.TempDir()
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return "", fmt.Errorf("create media dir: %w", err)
	}
	diskPath := filepath.Join(absDir, fileName)

	// Content-addressed: skip write if file already exists with same hash.
	if _, err := os.Stat(diskPath); err == nil {
		if relDir != "" {
			return filepath.Join(relDir, fileName), nil
		}
		return diskPath, nil
	}

	if err := os.WriteFile(diskPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write temp media: %w", err)
	}

	if relDir != "" {
		return filepath.Join(relDir, fileName), nil
	}
	return diskPath, nil
}

func fetchMediaURL(ctx context.Context, rawURL string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, mediaFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download media: status %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(mediaFetchMaxBytes)+1))
	if err != nil {
		return nil, "", fmt.Errorf("read media body: %w", err)
	}
	if len(data) > mediaFetchMaxBytes {
		return nil, "", fmt.Errorf("media too large (>200MB)")
	}

	mediaType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if semi := strings.Index(mediaType, ";"); semi >= 0 {
		mediaType = strings.TrimSpace(mediaType[:semi])
	}
	if mediaType == "" {
		mediaType = http.DetectContentType(data)
	}

	return data, mediaType, nil
}

func extensionForMediaType(mediaType, rawURL string) string {
	switch mediaType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "video/mp4":
		return ".mp4"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav":
		return ".wav"
	}
	// Fallback: try to extract from URL path.
	if idx := strings.LastIndex(rawURL, "."); idx >= 0 {
		ext := strings.ToLower(rawURL[idx:])
		if qm := strings.Index(ext, "?"); qm >= 0 {
			ext = ext[:qm]
		}
		if len(ext) <= 5 {
			return ext
		}
	}
	return ".bin"
}
