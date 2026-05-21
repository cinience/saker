package textutil

import (
	"path/filepath"
	"strings"
)

// MediaURLFromPath builds a routable HTTP URL from a local file path.
//
//   - /media/* paths are served directly by the media handler (returned as-is).
//   - Absolute paths get the /api/files prefix (e.g. /tmp/x.png → /api/files/tmp/x.png).
//   - Relative paths get /api/files/ (e.g. out/img.png → /api/files/out/img.png).
//
// Consolidates the URL-building logic previously duplicated across
// pkg/server/handler_media.go and pkg/api/conversation_persist.go.
func MediaURLFromPath(path string) string {
	if strings.HasPrefix(path, "/media/") {
		return path
	}
	if strings.HasPrefix(path, "/") {
		return "/api/files" + path
	}
	return "/api/files/" + path
}

// Media extension classification tables.
var (
	ImageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true}
	VideoExts = map[string]bool{".mp4": true, ".webm": true, ".mov": true}
	AudioExts = map[string]bool{".mp3": true, ".wav": true, ".ogg": true, ".flac": true}
)

// ClassifyMediaExt returns "image", "video", "audio", or "" for a given
// file extension (including the dot, e.g. ".png"). Case-insensitive.
func ClassifyMediaExt(ext string) string {
	ext = strings.ToLower(ext)
	switch {
	case ImageExts[ext]:
		return "image"
	case VideoExts[ext]:
		return "video"
	case AudioExts[ext]:
		return "audio"
	default:
		return ""
	}
}

// ClassifyMediaPath returns the media type for a file path based on its
// extension, or "" if the extension is not a known media type.
func ClassifyMediaPath(path string) string {
	return ClassifyMediaExt(filepath.Ext(path))
}
