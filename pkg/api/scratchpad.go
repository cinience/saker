package api

import (
	"fmt"
	"os"
	"path/filepath"
)

// ScratchpadDir returns the per-session scratchpad directory path.
// Creates the directory if it doesn't exist.
func ScratchpadDir(projectRoot, sessionID string) (string, error) {
	dir := filepath.Join(projectRoot, ".saker", "scratchpad", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create scratchpad dir: %w", err)
	}
	return dir, nil
}

func sectionScratchpad(dir string) string {
	if dir == "" {
		return ""
	}
	return fmt.Sprintf(`# Scratchpad Directory

Use this scratchpad directory for temporary files instead of /tmp or other system temp directories:
%s

Use this directory for ALL temporary file needs:
- Storing intermediate results or data during multi-step tasks
- Writing temporary scripts or configuration files
- Saving outputs that don't belong in the user's project
- Creating working files during analysis or processing

The scratchpad directory is session-specific, isolated from the user's project, and can be used freely.`, dir)
}
