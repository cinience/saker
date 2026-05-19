package server

import (
	"context"

	"github.com/gin-gonic/gin"
)

// Public re-exports of pkg/server internals that sibling subpackages (openai,
// agui) need. The original logic lives next to handler_turn.go / handler_media.go
// because the WebSocket handler depends on it; sibling gateways can't reach
// unexported symbols directly. Rather than move or duplicate, we expose minimal
// wrappers here so the gateways consume the same battle-tested logic.

// StreamArtifactFilter is the public surface of streamArtifactFilter.
// It strips function-call leakage (e.g. <tool_call>, Anthropic-style
// <invoke>, <|FunctionCallBegin|>) from streaming text deltas before
// they reach SSE subscribers.
//
// Use:
//
//	f := server.NewStreamArtifactFilter()
//	for chunk := range deltas {
//	    if safe := f.Push(chunk); safe != "" {
//	        emit(safe)
//	    }
//	}
//	if tail := f.Flush(); tail != "" {
//	    emit(tail)
//	}
type StreamArtifactFilter struct {
	inner *streamArtifactFilter
}

// NewStreamArtifactFilter builds a fresh filter with empty held state.
func NewStreamArtifactFilter() *StreamArtifactFilter {
	return &StreamArtifactFilter{inner: &streamArtifactFilter{}}
}

// Push consumes a delta chunk and returns whatever is safe to forward
// downstream. Returns "" when the whole chunk is held back; subscribers
// should treat that as "no-op, wait for the next delta".
func (f *StreamArtifactFilter) Push(chunk string) string {
	if f == nil || f.inner == nil {
		return chunk
	}
	return f.inner.Push(chunk)
}

// Flush releases any held-back bytes at end-of-stream.
func (f *StreamArtifactFilter) Flush() string {
	if f == nil || f.inner == nil {
		return ""
	}
	return f.inner.Flush()
}

// CleanAssistantReply is the canonical post-stream cleanup pass: trims
// streaming dot artifacts and strips leaked function-call syntax (Qwen-style
// XML, Claude-style invoke, etc.). Returns "" when nothing meaningful
// remains.
func CleanAssistantReply(raw string) string {
	return cleanAssistantReply(raw)
}

// DefaultTurnTimeout is the public alias for the in-package constant. The
// OpenAI gateway uses it as the upper bound for chat-completions runs so
// it matches the behavior of the WebSocket-driven turn handler.
// ExtractArtifacts is the public surface of extractArtifacts.
// It inspects a tool_execution_result output for media references via three
// paths: structured metadata, data metadata, and regex detection of URLs/paths.
func ExtractArtifacts(toolName string, output interface{}) []Artifact {
	return extractArtifacts(toolName, output)
}

// FormatToolResult is the public surface of formatToolResult.
// It extracts a readable summary from a tool result payload, truncating to
// 500 characters.
func FormatToolResult(toolName string, output interface{}) string {
	return formatToolResult(toolName, output)
}

// CacheArtifactMedia is the public wrapper for cacheArtifactMedia.
// It downloads remote media URLs and returns an artifact pointing to a locally
// cached copy under /api/files/ or /media/ paths. If caching fails or the URL
// is already local, returns the original artifact unchanged.
func (h *Handler) CacheArtifactMedia(ctx context.Context, a Artifact) Artifact {
	return h.cacheArtifactMedia(ctx, a)
}

const DefaultTurnTimeout = defaultTurnTimeout

// SessionValidatorFunc returns a gin.Context-typed callback that validates the
// saker_session cookie (or localhost loopback) and extracts identity. Intended
// for EngineHook-mounted gateways (AG-UI, etc.) that need browser auth but
// run outside the main auth middleware chain.
func (s *Server) SessionValidatorFunc() func(c *gin.Context) (string, string, bool) {
	return func(c *gin.Context) (string, string, bool) {
		if isLocalhost(c.Request) {
			if s.auth.cfg == nil || s.auth.cfg.Password == "" {
				return "localhost", "admin", true
			}
			adminUser := s.auth.cfg.Username
			if adminUser == "" {
				adminUser = "admin"
			}
			return adminUser, "admin", true
		}
		cookie, err := c.Request.Cookie(sessionCookieName)
		if err != nil || !s.auth.validToken(cookie.Value) {
			return "", "", false
		}
		username, role := s.auth.extractTokenInfo(cookie.Value)
		return username, role, true
	}
}
