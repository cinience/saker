package agui

import (
	"bytes"
	"io"
)

// idOverrideWriter wraps an io.Writer and strips any "id: ..." lines produced
// by the AG-UI SDK's SSE writer. This allows the caller to prepend its own
// monotonic integer id: field for resumable streams.
type idOverrideWriter struct {
	w io.Writer
}

func (s *idOverrideWriter) Write(p []byte) (int, error) {
	filtered := stripIDLine(p)
	_, err := s.w.Write(filtered)
	// Return original len to satisfy io.Writer contract (caller wrote len(p) bytes).
	return len(p), err
}

// stripIDLine removes lines starting with "id: " from an SSE frame byte slice.
func stripIDLine(frame []byte) []byte {
	lines := bytes.Split(frame, []byte("\n"))
	var result []byte
	for i, line := range lines {
		if bytes.HasPrefix(line, []byte("id: ")) {
			continue
		}
		if i > 0 && len(result) > 0 {
			result = append(result, '\n')
		}
		result = append(result, line...)
	}
	return result
}
