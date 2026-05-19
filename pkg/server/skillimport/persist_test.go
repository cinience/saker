package skillimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockConfigProvider struct {
	root string
}

func (m *mockConfigProvider) ConfigRoot() string { return m.root }

func TestResolveTargetRoot_Local(t *testing.T) {
	t.Parallel()
	cfg := &mockConfigProvider{root: "/project/.saker"}
	got, err := ResolveTargetRoot(cfg, ScopeLocal)
	require.NoError(t, err)
	require.Equal(t, "/project/.saker/skills", got)
}

func TestResolveTargetRoot_Global(t *testing.T) {
	t.Parallel()
	cfg := &mockConfigProvider{root: "/ignored"}
	got, err := ResolveTargetRoot(cfg, ScopeGlobal)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(got, filepath.Join(".agents", "skills")))
}

func TestResolveTargetRoot_InvalidScope(t *testing.T) {
	t.Parallel()
	cfg := &mockConfigProvider{root: "/x"}
	_, err := ResolveTargetRoot(cfg, Scope("bad"))
	require.Error(t, err)
}

func TestPrepareTargetDir_NewDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "fresh")
	action, err := PrepareTargetDir(dir, ConflictSkip)
	require.NoError(t, err)
	require.Equal(t, "created", action)
}

func TestPrepareTargetDir_OverwriteRemoves(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "leftover"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "leftover", "f"), []byte("x"), 0o644))

	action, err := PrepareTargetDir(dir, ConflictOverwrite)
	require.NoError(t, err)
	require.Equal(t, "overwritten", action)

	_, err = os.Stat(dir)
	require.True(t, os.IsNotExist(err), "dir must be removed after overwrite")
}

func TestPrepareTargetDir_Skip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	action, err := PrepareTargetDir(dir, ConflictSkip)
	require.NoError(t, err)
	require.Equal(t, "skipped", action)
}

func TestPrepareTargetDir_Error(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := PrepareTargetDir(dir, ConflictError)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestCopyDir(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("world"), 0o644))

	dst := filepath.Join(t.TempDir(), "target")
	require.NoError(t, CopyDir(src, dst))

	data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))

	data, err = os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	require.NoError(t, err)
	require.Equal(t, "world", string(data))
}
