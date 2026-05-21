package textutil

import "testing"

func TestMediaURLFromPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// /media/ paths pass through unchanged.
		{"/media/abc123/image.png", "/media/abc123/image.png"},
		{"/media/foo.mp4", "/media/foo.mp4"},
		// Absolute paths get /api/files prefix.
		{"/tmp/x.png", "/api/files/tmp/x.png"},
		{"/home/user/photo.jpg", "/api/files/home/user/photo.jpg"},
		// Relative paths get /api/files/ prefix.
		{"out/img.png", "/api/files/out/img.png"},
		{"canvas/art.svg", "/api/files/canvas/art.svg"},
	}
	for _, tt := range tests {
		got := MediaURLFromPath(tt.input)
		if got != tt.want {
			t.Errorf("MediaURLFromPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestClassifyMediaExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".png", "image"},
		{".JPG", "image"},
		{".jpeg", "image"},
		{".gif", "image"},
		{".webp", "image"},
		{".svg", "image"},
		{".mp4", "video"},
		{".WEBM", "video"},
		{".mov", "video"},
		{".mp3", "audio"},
		{".wav", "audio"},
		{".ogg", "audio"},
		{".flac", "audio"},
		{".txt", ""},
		{".go", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := ClassifyMediaExt(tt.ext)
		if got != tt.want {
			t.Errorf("ClassifyMediaExt(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestClassifyMediaPath(t *testing.T) {
	if got := ClassifyMediaPath("/tmp/photo.PNG"); got != "image" {
		t.Errorf("ClassifyMediaPath /tmp/photo.PNG = %q, want image", got)
	}
	if got := ClassifyMediaPath("out/clip.mp4"); got != "video" {
		t.Errorf("ClassifyMediaPath out/clip.mp4 = %q, want video", got)
	}
	if got := ClassifyMediaPath("noext"); got != "" {
		t.Errorf("ClassifyMediaPath noext = %q, want empty", got)
	}
}
