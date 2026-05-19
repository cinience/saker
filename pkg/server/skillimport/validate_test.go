package skillimport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeConflictStrategy(t *testing.T) {
	t.Parallel()
	require.Equal(t, ConflictOverwrite, NormalizeConflictStrategy(""))
	require.Equal(t, ConflictOverwrite, NormalizeConflictStrategy(ConflictOverwrite))
	require.Equal(t, ConflictSkip, NormalizeConflictStrategy(ConflictSkip))
	require.Equal(t, ConflictError, NormalizeConflictStrategy(ConflictError))
	require.Equal(t, ConflictMode(""), NormalizeConflictStrategy(ConflictMode("nonsense")))
}

func TestNormalizeParams_PathHappyPath(t *testing.T) {
	t.Parallel()
	src, paths, err := NormalizeParams(Params{
		SourcePath:  "/abs/skill",
		TargetScope: ScopeLocal,
	})
	require.NoError(t, err)
	require.Equal(t, SourcePath, src)
	require.Equal(t, []string{"/abs/skill"}, paths)
}

func TestNormalizeParams_PathUsesSourcePathsArray(t *testing.T) {
	t.Parallel()
	_, paths, err := NormalizeParams(Params{
		SourcePaths: []string{"/a", "/b"},
		TargetScope: ScopeLocal,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"/a", "/b"}, paths)
}

func TestNormalizeParams_PathEmptyEntryRejected(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeParams(Params{
		SourcePaths: []string{"   "},
		TargetScope: ScopeLocal,
	})
	require.Error(t, err)
}

func TestNormalizeParams_PathMissing(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeParams(Params{TargetScope: ScopeLocal})
	require.Error(t, err)
	require.Contains(t, err.Error(), "source_path is required")
}

func TestNormalizeParams_BadConflict(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeParams(Params{
		SourcePath:       "/x",
		TargetScope:      ScopeLocal,
		ConflictStrategy: ConflictMode("garbage"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "conflict_strategy")
}

func TestNormalizeParams_BadScope(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeParams(Params{SourcePath: "/x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "target_scope")
}

func TestNormalizeParams_GitMissingURL(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeParams(Params{
		SourceType:  SourceGit,
		TargetScope: ScopeLocal,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "repo_url")
}

func TestNormalizeParams_ArchiveMissingURL(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeParams(Params{
		SourceType:  SourceArchive,
		TargetScope: ScopeLocal,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "archive_url")
}

func TestNormalizeParams_GitWithBadScope(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeParams(Params{
		SourceType: SourceGit,
		RepoURL:    "https://x/y.git",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "target_scope")
}

func TestNormalizeParams_GitWithRelativeSourcePaths(t *testing.T) {
	t.Parallel()
	_, paths, err := NormalizeParams(Params{
		SourceType:  SourceGit,
		RepoURL:     "https://x/y.git",
		SourcePaths: []string{"alpha", "beta"},
		TargetScope: ScopeLocal,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta"}, paths)
}

func TestNormalizeParams_GitRejectsAbsoluteSourcePath(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeParams(Params{
		SourceType:  SourceGit,
		RepoURL:     "https://x/y.git",
		SourcePaths: []string{"/etc"},
		TargetScope: ScopeLocal,
	})
	require.Error(t, err)
}

func TestNormalizeParams_BadSourceType(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeParams(Params{
		SourceType:  SourceType("garbage"),
		TargetScope: ScopeLocal,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid source_type")
}

func TestValidateRelativePath(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateRelativePath("alpha/beta"))
	require.NoError(t, ValidateRelativePath("alpha"))

	require.Error(t, ValidateRelativePath(""))
	require.Error(t, ValidateRelativePath("   "))
	require.Error(t, ValidateRelativePath("."))
	require.Error(t, ValidateRelativePath("/abs"))
	require.Error(t, ValidateRelativePath("../escape"))
}

func TestParseSkillName(t *testing.T) {
	t.Parallel()
	name, err := parseSkillName("---\nname: alpha\ndescription: x\n---\nbody\n")
	require.NoError(t, err)
	require.Equal(t, "alpha", name)

	name, err = parseSkillName("\uFEFF---\nname: bom-skill\n---\nbody")
	require.NoError(t, err)
	require.Equal(t, "bom-skill", name)

	_, err = parseSkillName("body without frontmatter")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing YAML frontmatter")

	_, err = parseSkillName("---\nname: alpha\nstill going\n")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unterminated")

	_, err = parseSkillName("---\nname: : :\n---\nbody")
	require.Error(t, err)

	name, err = parseSkillName("---\ndescription: only\n---\nbody")
	require.NoError(t, err)
	require.Empty(t, name)
}

func TestIsValidSkillDir(t *testing.T) {
	t.Parallel()
	require.True(t, IsValidSkillDir("alpha"))
	require.True(t, IsValidSkillDir("alpha-beta_1"))

	require.False(t, IsValidSkillDir(""))
	require.False(t, IsValidSkillDir("with/slash"))
	require.False(t, IsValidSkillDir(`with\backslash`))
	require.False(t, IsValidSkillDir(".."))
	require.False(t, IsValidSkillDir("."))
	require.False(t, IsValidSkillDir("/abs"))
}

func TestValidateSkill_Happy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: hi\n---\nbody\n"), 0o644))

	name, err := ValidateSkill(dir)
	require.NoError(t, err)
	require.Equal(t, "my-skill", name)
}

func TestValidateSkill_NameFromDirWhenMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "fallback-name")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\ndescription: no name\n---\nbody\n"), 0o644))

	name, err := ValidateSkill(dir)
	require.NoError(t, err)
	require.Equal(t, "fallback-name", name)
}

func TestValidateSkill_MissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := ValidateSkill(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing SKILL.md")
}

func TestValidateSkill_InvalidName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: with/slash\n---\nbody\n"), 0o644))

	_, err := ValidateSkill(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid skill name")
}

func TestValidateSkill_BadFrontmatter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("no frontmatter at all"), 0o644))

	_, err := ValidateSkill(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid SKILL.md")
}

func TestDiscoverPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mk := func(p, body string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, p), 0o755))
		if body != "" {
			require.NoError(t, os.WriteFile(filepath.Join(root, p, "SKILL.md"), []byte(body), 0o644))
		}
	}
	mk("alpha", "x")
	mk("nested/beta", "x")
	mk(".git/hooks", "")
	mk("node_modules/pkg", "")

	paths, err := DiscoverPaths(root)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"alpha", filepath.Join("nested", "beta")}, paths)
}

func TestDiscoverPaths_NothingFound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "x"), 0o755))

	_, err := DiscoverPaths(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no skills discovered")
}

func TestDiscoverPaths_BadRoot(t *testing.T) {
	t.Parallel()
	_, err := DiscoverPaths(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}
