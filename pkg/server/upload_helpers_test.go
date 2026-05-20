package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAllowedMedia(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mediaType string
		want      bool
	}{
		{"image/png", true},
		{"image/jpeg", true},
		{"video/mp4", true},
		{"audio/mpeg", true},
		{"application/pdf", true},
		{"text/plain", false},
		{"application/octet-stream", false},
		{"application/zip", false},
		{"", false},
	}
	for _, c := range cases {
		got := isAllowedMedia(c.mediaType)
		require.Equal(t, c.want, got, "isAllowedMedia(%q)", c.mediaType)
	}
}

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"hello.png", "hello.png"},
		{"/etc/passwd", "passwd"},
		{"..\\evil.exe", "evil.exe"},
		{"a/b/c.txt", "c.txt"},
		{"x\x00y.zip", "xy.zip"},
		{"..", "upload"},
		{"weird name with spaces.jpg", "weird name with spaces.jpg"},
	}
	for _, c := range cases {
		got := sanitizeFilename(c.in)
		require.Equal(t, c.want, got, "sanitizeFilename(%q)", c.in)
	}
}

func TestExtFromMime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mime string
		want string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"video/mp4", ".mp4"},
		{"audio/mpeg", ".mp3"},
		{"application/pdf", ".pdf"},
		{"text/plain", ""},
	}
	for _, c := range cases {
		got := extFromMime(c.mime)
		require.Equal(t, c.want, got, "extFromMime(%q)", c.mime)
	}
}
