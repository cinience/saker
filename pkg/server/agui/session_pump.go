package agui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/saker-ai/saker/pkg/server"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	aguisse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
)

// pump runs the event loop for a session, independent of any HTTP connection.
// It reads from the runtime event channel, translates to AG-UI events,
// buffers in the ring, and forwards to the attached client (if any).
func (s *runSession) pump(finishRun func()) {
	defer func() {
		finishRun()
		s.gateway.clearThreadRun(s.threadID, s.runID)
		if s.mcpRegistry != nil {
			// Return registry to thread-level cache for cross-turn reuse
			// instead of closing it immediately.
			s.gateway.mcpCache.put(s.threadID, s.mcpRegistry)
		}
		// Grace period: keep session alive so late reconnects can still replay.
		time.AfterFunc(sessionGracePeriod, func() {
			s.gateway.sessions.remove(s.runID)
		})
		close(s.doneCh)
	}()

	ctx := s.runtimeCtx
	state := newStreamState(s.threadID, s.runID)
	state.ring = s.ring // Share ring with session for writeSSEWithID

	sseW := aguisse.NewSSEWriter().WithLogger(s.logger)
	filter := server.NewStreamArtifactFilter()
	var accumulated strings.Builder
	eventCounts := make(map[string]int)
	streamStart := time.Now()
	aguiActiveStreams.Inc()
	defer func() {
		aguiActiveStreams.Dec()
		aguiRunDuration.Observe(time.Since(streamStart).Seconds())
	}()

	s.logger.Info("agui session pump started",
		"thread_id", s.threadID,
		"run_id", s.runID,
		"turn_id", s.turnID,
	)

	// Write initial events.
	w := s.clientWriter()
	if err := writeSSEWithID(ctx, w, sseW, aguievents.NewRunStartedEvent(s.threadID, s.runID), state); err != nil {
		s.logger.Warn("agui pump: run_started write failed", "run_id", s.runID, "error", err)
		s.handleWriteError(err)
		w = s.clientWriter()
	}
	if err := writeSSEWithID(ctx, w, sseW, aguievents.NewStateSnapshotEvent(
		map[string]any{"artifacts": []any{}},
	), state); err != nil {
		s.logger.Warn("agui pump: state_snapshot write failed", "run_id", s.runID, "error", err)
		s.handleWriteError(err)
	}
	s.flushClient()

	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case evt, ok := <-s.eventCh:
			if !ok {
				// Runtime completed — persist and finalize.
				if len(state.artifacts) > 0 {
					s.gateway.storeArtifacts(s.threadID, state.artifacts)
				}
				s.gateway.persistAssistantWithArtifacts(context.Background(), s.threadID, s.turnID, s.projectID, accumulated.String(), state.artifacts)
				if s.gateway.deps.ConversationStore != nil {
					_ = s.gateway.deps.ConversationStore.CloseTurn(context.Background(), s.turnID, "completed")
				}
				s.gateway.clearThreadRun(s.threadID, s.runID)
				w = s.clientWriter()
				if err := state.finalize(ctx, w, sseW, filter); err != nil {
					s.logger.Warn("agui pump: finalize failed", "run_id", s.runID, "error", err)
					s.handleWriteError(err)
				}
				s.flushClient()
				s.markFinished()
				s.logger.Info("agui session pump completed",
					"run_id", s.runID,
					"thread_id", s.threadID,
					"turn_id", s.turnID,
					"assistant_len", accumulated.Len(),
					"event_counts", eventCounts,
				)
				return
			}

			eventCounts[string(evt.Type)]++
			if evt.Type == "content_block_delta" && evt.Delta != nil && evt.Delta.Text != "" {
				accumulated.WriteString(evt.Delta.Text)
			}

			w = s.clientWriter()
			newArtifacts, err := state.translateEvent(ctx, w, sseW, evt, filter)
			if err != nil {
				if s.handleWriteError(err) {
					// Client disconnected, pump continues.
					continue
				}
				// Context cancelled or fatal error.
				s.logger.Warn("agui pump: translate failed fatally",
					"run_id", s.runID, "error", err)
				return
			}

			if len(newArtifacts) > 0 {
				w = s.clientWriter()
				s.emitArtifacts(ctx, w, sseW, state, newArtifacts)
			}
			s.flushClient()

		case se := <-s.sideCh:
			if len(se.events) == 0 {
				continue
			}
			w = s.clientWriter()
			for _, event := range se.events {
				if event == nil {
					continue
				}
				if err := writeSSEWithID(ctx, w, sseW, event, state); err != nil {
					s.handleWriteError(err)
					break
				}
			}
			s.flushClient()

		case <-keepalive.C:
			client := s.getClient()
			if client == nil {
				continue
			}
			if _, err := fmt.Fprintf(client.writer, ": keepalive\n\n"); err != nil {
				s.handleWriteError(err)
				continue
			}
			if client.flusher != nil {
				client.flusher.Flush()
			}

		case <-ctx.Done():
			s.gateway.clearThreadRun(s.threadID, s.runID)
			aguiErrorsTotal.WithLabelValues("cancelled").Inc()
			bgCtx := context.Background()
			w = s.clientWriter()
			_ = writeSSEWithID(bgCtx, w, sseW, aguievents.NewRunErrorEvent("stream cancelled",
				aguievents.WithRunID(s.runID), aguievents.WithErrorCode("cancelled")), state)
			s.flushClient()
			s.markFinished()
			s.logger.Info("agui session pump cancelled",
				"run_id", s.runID,
				"thread_id", s.threadID,
				"error", ctx.Err(),
			)
			return
		}
	}
}

// clientWriter returns the current client writer, or io.Discard if detached.
func (s *runSession) clientWriter() io.Writer {
	client := s.getClient()
	if client == nil {
		return io.Discard
	}
	return client.writer
}

// flushClient flushes the attached client, if any.
func (s *runSession) flushClient() {
	client := s.getClient()
	if client != nil && client.flusher != nil {
		client.flusher.Flush()
	}
}

// handleWriteError determines if the error is a client disconnect (returns true)
// or a fatal error (returns false). On client disconnect, detaches the client.
func (s *runSession) handleWriteError(err error) bool {
	if err == nil {
		return false
	}
	if s.runtimeCtx.Err() != nil {
		return false // context cancelled, not a client error
	}
	if isSlowClientErr(err) || isClientWriteErr(err) {
		aguiSlowClientDisconnects.Inc()
		s.detach()
		return true
	}
	return false
}

// isClientWriteErr returns true if the error likely indicates a client disconnect.
func isClientWriteErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "write: connection refused") ||
		strings.Contains(msg, "client disconnected") ||
		strings.Contains(msg, "http2: stream closed")
}

// emitArtifacts emits StateDelta events for new artifacts immediately (non-blocking),
// then caches remote media asynchronously. When caching completes, a follow-up
// StateDelta is sent via sideCh to update the URL to the local cached copy.
func (s *runSession) emitArtifacts(ctx context.Context, w io.Writer, sseW sseWriter, state *streamState, newArtifacts []server.Artifact) {
	baseIdx := len(state.artifacts) - len(newArtifacts)

	for i, a := range newArtifacts {
		delta := []aguievents.JSONPatchOperation{
			{
				Op:   "add",
				Path: "/artifacts/-",
				Value: map[string]string{
					"type": a.Type,
					"url":  a.URL,
					"name": a.Name,
				},
			},
		}
		if err := writeSSEWithID(ctx, w, sseW, aguievents.NewStateDeltaEvent(delta), state); err != nil {
			s.logger.Warn("agui pump: state delta write failed",
				"run_id", s.runID, "artifact_url", a.URL, "error", err)
			s.handleWriteError(err)
			break
		}

		// Start async caching for remote URLs.
		cacher := s.gateway.deps.MediaCacher
		if cacher == nil {
			continue
		}
		if !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
			continue
		}
		artifactIdx := baseIdx + i
		original := a
		go func() {
			cached := cacher.CacheArtifactMedia(context.Background(), original)
			if cached.URL == original.URL {
				return
			}
			// Update in-memory artifact for future connect replays.
			s.mu.Lock()
			if artifactIdx >= 0 && artifactIdx < len(state.artifacts) {
				state.artifacts[artifactIdx] = cached
			}
			s.mu.Unlock()
			s.gateway.storeArtifacts(s.threadID, state.artifacts)

			// Notify attached client of URL update.
			patch := []aguievents.JSONPatchOperation{
				{Op: "replace", Path: fmt.Sprintf("/artifacts/%d/url", artifactIdx), Value: cached.URL},
			}
			select {
			case s.sideCh <- sideEvent{events: []aguievents.Event{aguievents.NewStateDeltaEvent(patch)}}:
			default:
			}
		}()
	}
}
