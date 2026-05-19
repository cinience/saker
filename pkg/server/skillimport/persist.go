package skillimport

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ResolveTargetRoot returns the directory where imported skills should be placed
// based on the scope.
func ResolveTargetRoot(cfg ConfigProvider, scope Scope) (string, error) {
	switch scope {
	case ScopeLocal:
		return filepath.Join(cfg.ConfigRoot(), "skills"), nil
	case ScopeGlobal:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".agents", "skills"), nil
	default:
		return "", fmt.Errorf("invalid target scope %q", scope)
	}
}

// PrepareTargetDir handles conflict resolution for an existing target directory.
// Returns an action string ("created", "overwritten", "skipped") or an error.
func PrepareTargetDir(targetDir string, mode ConflictMode) (string, error) {
	if _, err := os.Stat(targetDir); err != nil {
		if os.IsNotExist(err) {
			return "created", nil
		}
		return "", err
	}
	switch NormalizeConflictStrategy(mode) {
	case ConflictOverwrite:
		if err := os.RemoveAll(targetDir); err != nil {
			return "", err
		}
		return "overwritten", nil
	case ConflictSkip:
		return "skipped", nil
	case ConflictError:
		return "", fmt.Errorf("target skill already exists: %s", filepath.Base(targetDir))
	default:
		return "", fmt.Errorf("unsupported conflict strategy %q", mode)
	}
}

// CopyDir recursively copies source to target, replacing target if it exists.
func CopyDir(source, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		return writeCopiedFile(dst, srcFile, info.Mode())
	})
}

func writeCopiedFile(target string, reader io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, reader); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
