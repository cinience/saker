package skillimport

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSSHRepoURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"ssh://git@github.com/foo/bar.git", true},
		{"git@github.com:foo/bar.git", true},
		{"https://github.com/foo/bar.git", false},
		{"http://example.com/foo.git", false},
		{"file:///tmp/repo", false},
		{"plainstring", false},
	}
	for _, c := range cases {
		require.Equal(t, c.want, isSSHRepoURL(c.in), "isSSHRepoURL(%q)", c.in)
	}
}

func TestSafeArchivePath(t *testing.T) {
	t.Parallel()
	root := "/dest"
	good, err := safeArchivePath(root, "skill/inner/file.md")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skill", "inner", "file.md"), good)

	_, err = safeArchivePath(root, "../escape")
	require.Error(t, err)

	_, err = safeArchivePath(root, "/absolute")
	require.Error(t, err)

	_, err = safeArchivePath(root, ".")
	require.Error(t, err)
}

func TestExtractArchive_Zip(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	zipPath := filepath.Join(src, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"skill/SKILL.md": "---\nname: z\n---\nbody",
		"skill/lib.go":   "package lib",
	})

	dest := filepath.Join(t.TempDir(), "out")
	require.NoError(t, os.MkdirAll(dest, 0o755))
	require.NoError(t, ExtractArchive(zipPath, dest))

	data, err := os.ReadFile(filepath.Join(dest, "skill", "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "name: z")
}

func TestExtractArchive_TarGz(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	tgzPath := filepath.Join(src, "test.tar.gz")
	createTestTarGz(t, tgzPath, map[string]string{
		"skill/SKILL.md": "---\nname: tgz\n---\nbody",
	})

	dest := filepath.Join(t.TempDir(), "out")
	require.NoError(t, os.MkdirAll(dest, 0o755))
	require.NoError(t, ExtractArchive(tgzPath, dest))

	data, err := os.ReadFile(filepath.Join(dest, "skill", "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "name: tgz")
}

func TestDetectSingleRootDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "only-child", "sub"), 0o755))

	got, err := detectSingleRootDir(root)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "only-child"), got)

	multiRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(multiRoot, "a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(multiRoot, "b"), 0o755))
	_, err = detectSingleRootDir(multiRoot)
	require.Error(t, err)
}

func TestDownloadArchive_HappyPath(t *testing.T) {
	t.Parallel()
	zipBuf := createTestZipBytes(t, map[string]string{
		"skill/SKILL.md": "---\nname: dl\n---\nbody",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipBuf)
	}))
	defer srv.Close()

	root, cleanup, err := DownloadArchive(srv.URL + "/test.zip")
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	_, err = os.Stat(filepath.Join(root, "SKILL.md"))
	require.NoError(t, err)
}

func TestDownloadArchive_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, _, err := DownloadArchive(srv.URL + "/missing.zip")
	require.Error(t, err)
	require.Contains(t, err.Error(), "status 404")
}

func TestPrepareSource_PathNoop(t *testing.T) {
	t.Parallel()
	root, cleanup, err := PrepareSource(SourcePath, Params{})
	require.NoError(t, err)
	require.Equal(t, "", root)
	require.Nil(t, cleanup)
}

func TestPrepareSource_BadType(t *testing.T) {
	t.Parallel()
	_, _, err := PrepareSource(SourceType("bad"), Params{})
	require.Error(t, err)
}

// --- test helpers ---

func createTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	data := createTestZipBytes(t, files)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func createTestZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		parts := strings.Split(name, "/")
		for i := range len(parts) - 1 {
			dirName := strings.Join(parts[:i+1], "/") + "/"
			if _, err := w.Create(dirName); err != nil {
				t.Fatalf("zip create dir %s: %v", dirName, err)
			}
		}
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func createTestTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
}
