package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/server"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// streamState tracks per-stream lifecycle for translating saker StreamEvents
// into AG-UI typed events. One instance per SSE stream.
type streamState struct {
	threadID string
	runID    string
	msgID    string
	// textMsgSeq counts text message segments (incremented when text resumes after tool calls).
	textMsgSeq int
	// eventSeq is the monotonically increasing SSE event ID counter.
	eventSeq int
	// textStarted is true after the first TEXT_MESSAGE_START has been emitted.
	textStarted bool
	// reasoningBlockStarted is true after REASONING_START (outer wrapper) has been emitted.
	reasoningBlockStarted bool
	// reasoningStarted is true after REASONING_MESSAGE_START has been emitted.
	reasoningStarted bool
	// toolCalls tracks which tool call IDs have been started.
	toolCalls map[string]bool
	// toolNames maps tool call IDs to their tool names for artifact extraction.
	toolNames map[string]string
	// suppressedToolCalls tracks runtime tool call IDs that are represented by
	// side-channel AG-UI action events instead of raw tool lifecycle events.
	suppressedToolCalls map[string]bool
	// lastToolID is the ID of the currently open tool call.
	lastToolID string
	// lastStep is the name of the currently open STEP.
	lastStep string
	// iterCount tracks how many iterations have started.
	iterCount int
	// activeActivityID tracks the current activity message ID for ACTIVITY events.
	activeActivityID string
	// artifacts accumulates media artifacts extracted from tool results.
	artifacts []server.Artifact
	// seenArtifactURLs deduplicates artifact URLs within a single stream.
	seenArtifactURLs map[string]bool
	// ring buffers serialized SSE frames for future reconnect replay.
	ring *eventRing
	// errorSent is true after a RUN_ERROR event has been emitted.
	errorSent bool
}

func newStreamState(threadID, runID string) *streamState {
	return &streamState{
		threadID:            threadID,
		runID:               runID,
		msgID:               fmt.Sprintf("msg_%s_%s", threadID, runID),
		toolCalls:           make(map[string]bool),
		toolNames:           make(map[string]string),
		suppressedToolCalls: make(map[string]bool),
		seenArtifactURLs:    make(map[string]bool),
		ring:                newEventRing(defaultEventRingSize),
	}
}

// currentTextMsgID returns the message ID for the current text segment.
func (s *streamState) currentTextMsgID() string {
	if s.textMsgSeq <= 1 {
		return s.msgID
	}
	return fmt.Sprintf("%s_%d", s.msgID, s.textMsgSeq)
}

// writeEvent emits an AG-UI event with a monotonic SSE id for resumability.
func (s *streamState) writeEvent(ctx context.Context, w io.Writer, sseW sseWriter, event aguievents.Event) error {
	return writeSSEWithID(ctx, w, sseW, event, s)
}

// translateEvent converts a single saker StreamEvent into zero or more AG-UI
// events written directly to the SSE writer. The text filter strips XML
// function-call artifacts from text deltas.
// Returns extracted artifacts from tool results for the caller to cache and
// emit as StateDeltaEvents.
func (s *streamState) translateEvent(ctx context.Context, w io.Writer, sseW sseWriter, evt api.StreamEvent, filter textFilter) ([]server.Artifact, error) {
	switch evt.Type {
	case api.EventReasoningDelta:
		if evt.Delta == nil || evt.Delta.Text == "" {
			return nil, nil
		}
		reasoningMsgID := "reasoning_" + s.msgID
		if !s.reasoningBlockStarted {
			s.reasoningBlockStarted = true
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewReasoningStartEvent(reasoningMsgID)); err != nil {
				return nil, err
			}
		}
		if !s.reasoningStarted {
			s.reasoningStarted = true
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewReasoningMessageStartEvent(reasoningMsgID, "reasoning")); err != nil {
				return nil, err
			}
		}
		return nil, s.writeEvent(ctx, w, sseW, aguievents.NewReasoningMessageContentEvent(reasoningMsgID, evt.Delta.Text))

	case api.EventContentBlockDelta:
		if evt.Delta == nil || evt.Delta.Text == "" {
			return nil, nil
		}
		// Close reasoning phase when text content starts arriving.
		if s.reasoningStarted && !s.textStarted {
			reasoningMsgID := "reasoning_" + s.msgID
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewReasoningMessageEndEvent(reasoningMsgID)); err != nil {
				return nil, err
			}
			if s.reasoningBlockStarted {
				if err := s.writeEvent(ctx, w, sseW, aguievents.NewReasoningEndEvent(reasoningMsgID)); err != nil {
					return nil, err
				}
			}
		}
		safe := filter.Push(evt.Delta.Text)
		if safe == "" {
			return nil, nil
		}
		if !s.textStarted {
			s.textStarted = true
			s.textMsgSeq++
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewTextMessageStartEvent(s.currentTextMsgID(), aguievents.WithRole("assistant"))); err != nil {
				return nil, err
			}
		}
		return nil, s.writeEvent(ctx, w, sseW, aguievents.NewTextMessageContentEvent(s.currentTextMsgID(), safe))

	case api.EventToolExecutionStart:
		if evt.Name == "ask_user_question" {
			if evt.ToolUseID != "" {
				s.suppressedToolCalls[evt.ToolUseID] = true
			}
			return nil, nil
		}
		// Close open text message before starting tool call — AG-UI requires
		// TEXT_MESSAGE to be fully closed before TOOL_CALL events.
		if s.textStarted {
			if tail := filter.Flush(); tail != "" {
				if err := s.writeEvent(ctx, w, sseW, aguievents.NewTextMessageContentEvent(s.currentTextMsgID(), tail)); err != nil {
					return nil, err
				}
			}
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewTextMessageEndEvent(s.currentTextMsgID())); err != nil {
				return nil, err
			}
			s.textStarted = false
		} else if s.textMsgSeq == 0 {
			// No text message started yet — emit an empty text message to create
			// the assistant message container that tool calls reference via parentMessageId.
			s.textMsgSeq++
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewTextMessageStartEvent(s.currentTextMsgID(), aguievents.WithRole("assistant"))); err != nil {
				return nil, err
			}
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewTextMessageEndEvent(s.currentTextMsgID())); err != nil {
				return nil, err
			}
		}
		if s.lastToolID != "" {
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewToolCallEndEvent(s.lastToolID)); err != nil {
				return nil, err
			}
		}
		toolID := evt.ToolUseID
		if toolID == "" {
			toolID = fmt.Sprintf("tc_%s_%s", s.runID, evt.Name)
		}
		s.toolCalls[toolID] = true
		s.toolNames[toolID] = evt.Name
		s.lastToolID = toolID

		// Emit ACTIVITY_SNAPSHOT for tool execution progress.
		s.activeActivityID = "activity_" + toolID
		activityContent := map[string]any{
			"tool":   evt.Name,
			"status": "running",
		}
		if err := s.writeEvent(ctx, w, sseW, aguievents.NewActivitySnapshotEvent(s.activeActivityID, "TOOL_EXECUTION", activityContent)); err != nil {
			return nil, err
		}

		if err := s.writeEvent(ctx, w, sseW, aguievents.NewToolCallStartEvent(toolID, evt.Name, aguievents.WithParentMessageID(s.msgID))); err != nil {
			return nil, err
		}
		args := inputToJSON(evt.Input)
		if args != "" && args != "{}" {
			return nil, s.writeEvent(ctx, w, sseW, aguievents.NewToolCallArgsEvent(toolID, args))
		}
		return nil, nil

	case api.EventToolExecutionResult:
		toolID := evt.ToolUseID
		if toolID == "" {
			toolID = s.lastToolID
		}
		if toolID != "" && s.suppressedToolCalls[toolID] {
			delete(s.suppressedToolCalls, toolID)
			return nil, nil
		}
		// Emit ACTIVITY_DELTA to mark tool execution completed.
		if s.activeActivityID != "" {
			status := "completed"
			if evt.IsError != nil && *evt.IsError {
				status = "error"
			}
			patch := []aguievents.JSONPatchOperation{
				{Op: "replace", Path: "/status", Value: status},
			}
			_ = s.writeEvent(ctx, w, sseW, aguievents.NewActivityDeltaEvent(s.activeActivityID, "TOOL_EXECUTION", patch))
			s.activeActivityID = ""
		}
		// Resolve tool name for artifact extraction.
		toolName := ""
		if toolID != "" {
			toolName = s.toolNames[toolID]
		}
		if toolName == "" {
			toolName = evt.Name
		}
		// Extract artifacts from tool result (non-error only).
		var newArtifacts []server.Artifact
		if evt.IsError == nil || !*evt.IsError {
			for _, a := range server.ExtractArtifacts(toolName, evt.Output) {
				if !s.seenArtifactURLs[a.URL] {
					s.seenArtifactURLs[a.URL] = true
					newArtifacts = append(newArtifacts, a)
				}
			}
		}
		s.artifacts = append(s.artifacts, newArtifacts...)
		// Emit TOOL_CALL_END.
		if toolID != "" {
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewToolCallEndEvent(toolID)); err != nil {
				return newArtifacts, err
			}
			if s.lastToolID == toolID {
				s.lastToolID = ""
			}
			delete(s.toolCalls, toolID)
			delete(s.toolNames, toolID)
		}
		// Emit TOOL_CALL_RESULT with formatted content.
		content := server.FormatToolResult(toolName, evt.Output)
		if content != "" && toolID != "" {
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewToolCallResultEvent(s.msgID, toolID, content)); err != nil {
				return newArtifacts, err
			}
		}
		return newArtifacts, nil

	case api.EventIterationStart:
		s.iterCount++
		stepName := fmt.Sprintf("iteration_%d", s.iterCount)
		if s.lastStep != "" {
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewStepFinishedEvent(s.lastStep)); err != nil {
				return nil, err
			}
		}
		s.lastStep = stepName
		// Emit ACTIVITY_SNAPSHOT for iteration progress.
		iterActivityID := fmt.Sprintf("activity_iter_%d_%s", s.iterCount, s.runID)
		iterContent := map[string]any{
			"step":      stepName,
			"iteration": s.iterCount,
			"status":    "running",
		}
		_ = s.writeEvent(ctx, w, sseW, aguievents.NewActivitySnapshotEvent(iterActivityID, "ITERATION", iterContent))
		return nil, s.writeEvent(ctx, w, sseW, aguievents.NewStepStartedEvent(stepName))

	case api.EventIterationStop:
		if s.lastStep != "" {
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewStepFinishedEvent(s.lastStep)); err != nil {
				return nil, err
			}
			s.lastStep = ""
		}
		return nil, nil

	case api.EventError:
		msg := "runtime error"
		code := ""
		if evt.Output != nil {
			switch v := evt.Output.(type) {
			case string:
				if v != "" {
					msg = v
				}
			case map[string]any:
				if m, ok := v["message"].(string); ok && m != "" {
					msg = m
				}
				if c, ok := v["code"].(string); ok && c != "" {
					code = c
				}
			}
		}
		errorLabel := code
		if errorLabel == "" {
			errorLabel = "unknown"
		}
		aguiErrorsTotal.WithLabelValues(errorLabel).Inc()
		opts := []aguievents.RunErrorOption{aguievents.WithRunID(s.runID)}
		if code != "" {
			opts = append(opts, aguievents.WithErrorCode(code))
		}
		s.errorSent = true
		return nil, s.writeEvent(ctx, w, sseW, aguievents.NewRunErrorEvent(msg, opts...))

	case api.EventTimeline:
		if evt.Timeline == nil {
			return nil, nil
		}
		return nil, s.writeEvent(ctx, w, sseW, aguievents.NewCustomEvent("timeline", aguievents.WithValue(evt.Timeline)))

	case api.EventSkillActivation:
		return nil, s.writeEvent(ctx, w, sseW, aguievents.NewCustomEvent("skill_activation", aguievents.WithValue(map[string]any{"name": evt.Name})))
	}
	return nil, nil
}

// finalize emits closing events for any open tool calls, text message,
// steps, and the RUN_FINISHED event with appropriate outcome.
func (s *streamState) finalize(ctx context.Context, w io.Writer, sseW sseWriter, filter textFilter) error {
	if s.errorSent {
		return nil
	}
	interrupted := s.lastToolID != "" && len(s.toolCalls) > 0
	if s.lastToolID != "" {
		if err := s.writeEvent(ctx, w, sseW, aguievents.NewToolCallEndEvent(s.lastToolID)); err != nil {
			return err
		}
		delete(s.toolNames, s.lastToolID)
		s.lastToolID = ""
	}
	if s.reasoningStarted && !s.textStarted {
		reasoningMsgID := "reasoning_" + s.msgID
		if err := s.writeEvent(ctx, w, sseW, aguievents.NewReasoningMessageEndEvent(reasoningMsgID)); err != nil {
			return err
		}
		if s.reasoningBlockStarted {
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewReasoningEndEvent(reasoningMsgID)); err != nil {
				return err
			}
		}
	}
	if tail := filter.Flush(); tail != "" {
		if !s.textStarted {
			s.textStarted = true
			s.textMsgSeq++
			if err := s.writeEvent(ctx, w, sseW, aguievents.NewTextMessageStartEvent(s.currentTextMsgID(), aguievents.WithRole("assistant"))); err != nil {
				return err
			}
		}
		if err := s.writeEvent(ctx, w, sseW, aguievents.NewTextMessageContentEvent(s.currentTextMsgID(), tail)); err != nil {
			return err
		}
	}
	if s.textStarted {
		if err := s.writeEvent(ctx, w, sseW, aguievents.NewTextMessageEndEvent(s.currentTextMsgID())); err != nil {
			return err
		}
	}
	if s.lastStep != "" {
		if err := s.writeEvent(ctx, w, sseW, aguievents.NewStepFinishedEvent(s.lastStep)); err != nil {
			return err
		}
		s.lastStep = ""
	}
	outcome := map[string]any{"type": "success"}
	if interrupted {
		outcome = map[string]any{"type": "interrupt"}
	}
	return s.writeEvent(ctx, w, sseW, aguievents.NewRunFinishedEventWithOptions(s.threadID, s.runID, aguievents.WithResult(outcome)))
}

// textFilter abstracts the stream artifact filter used to strip XML
// function-call artifacts from streaming text deltas.
type textFilter interface {
	Push(chunk string) string
	Flush() string
}

// sseWriter abstracts the AG-UI SDK SSE writer for testability.
type sseWriter interface {
	WriteEventWithType(ctx context.Context, w io.Writer, event aguievents.Event, eventType string) error
}

// writeSSE writes an AG-UI event as a typed SSE frame using the caller's
// request/run context so shutdown and client disconnects do not wait for the
// SDK writer's network timeout. Events are validated before sending.
// When state is provided, an incremental SSE `id:` field is prepended for
// resumable streams via Last-Event-ID.
func writeSSE(ctx context.Context, w io.Writer, sseW sseWriter, event aguievents.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("agui event validation: %w", err)
	}
	aguiEventsTotal.WithLabelValues(string(event.Type())).Inc()
	return sseW.WriteEventWithType(ctx, w, event, string(event.Type()))
}

// writeSSEWithID writes an event with an SSE id: field for resumability.
// After successful write, the serialized frame is pushed to the ring buffer.
func writeSSEWithID(ctx context.Context, w io.Writer, sseW sseWriter, event aguievents.Event, state *streamState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("agui event validation: %w", err)
	}
	aguiEventsTotal.WithLabelValues(string(event.Type())).Inc()
	state.eventSeq++
	if err := sseW.WriteEventWithType(ctx, w, event, string(event.Type())); err != nil {
		return err
	}
	if state.ring != nil {
		if frame, err := json.Marshal(event); err == nil {
			state.ring.Push(state.eventSeq, frame)
		}
	}
	return nil
}

// inputToJSON serializes a StreamEvent.Input (typed as interface{}) to a
// compact JSON string suitable for TOOL_CALL_ARGS delta.
func inputToJSON(input interface{}) string {
	if input == nil {
		return "{}"
	}
	if s, ok := input.(string); ok {
		return s
	}
	b, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}
	return string(b)
}