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
	// textStarted is true after the first TEXT_MESSAGE_START has been emitted.
	textStarted bool
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
	// artifacts accumulates media artifacts extracted from tool results.
	artifacts []server.Artifact
	// seenArtifactURLs deduplicates artifact URLs within a single stream.
	seenArtifactURLs map[string]bool
}

func newStreamState(threadID, runID string) *streamState {
	return &streamState{
		threadID:            threadID,
		runID:               runID,
		msgID:               fmt.Sprintf("msg_%s", runID),
		toolCalls:           make(map[string]bool),
		toolNames:           make(map[string]string),
		suppressedToolCalls: make(map[string]bool),
		seenArtifactURLs:    make(map[string]bool),
	}
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
		if !s.reasoningStarted {
			s.reasoningStarted = true
			reasoningMsgID := "reasoning_" + s.msgID
			if err := writeSSE(ctx, w, sseW, aguievents.NewReasoningMessageStartEvent(reasoningMsgID, "assistant")); err != nil {
				return nil, err
			}
		}
		return nil, writeSSE(ctx, w, sseW, aguievents.NewReasoningMessageContentEvent("reasoning_"+s.msgID, evt.Delta.Text))

	case api.EventContentBlockDelta:
		if evt.Delta == nil || evt.Delta.Text == "" {
			return nil, nil
		}
		// Close reasoning phase when text content starts arriving.
		if s.reasoningStarted && !s.textStarted {
			if err := writeSSE(ctx, w, sseW, aguievents.NewReasoningMessageEndEvent("reasoning_"+s.msgID)); err != nil {
				return nil, err
			}
		}
		safe := filter.Push(evt.Delta.Text)
		if safe == "" {
			return nil, nil
		}
		if !s.textStarted {
			s.textStarted = true
			if err := writeSSE(ctx, w, sseW, aguievents.NewTextMessageStartEvent(s.msgID, aguievents.WithRole("assistant"))); err != nil {
				return nil, err
			}
		}
		return nil, writeSSE(ctx, w, sseW, aguievents.NewTextMessageContentEvent(s.msgID, safe))

	case api.EventToolExecutionStart:
		if evt.Name == "ask_user_question" {
			if evt.ToolUseID != "" {
				s.suppressedToolCalls[evt.ToolUseID] = true
			}
			return nil, nil
		}
		if s.lastToolID != "" {
			if err := writeSSE(ctx, w, sseW, aguievents.NewToolCallEndEvent(s.lastToolID)); err != nil {
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
		if err := writeSSE(ctx, w, sseW, aguievents.NewToolCallStartEvent(toolID, evt.Name, aguievents.WithParentMessageID(s.msgID))); err != nil {
			return nil, err
		}
		args := inputToJSON(evt.Input)
		if args != "" && args != "{}" {
			return nil, writeSSE(ctx, w, sseW, aguievents.NewToolCallArgsEvent(toolID, args))
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
			if err := writeSSE(ctx, w, sseW, aguievents.NewToolCallEndEvent(toolID)); err != nil {
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
			if err := writeSSE(ctx, w, sseW, aguievents.NewToolCallResultEvent(s.msgID, toolID, content)); err != nil {
				return newArtifacts, err
			}
		}
		return newArtifacts, nil

	case api.EventIterationStart:
		s.iterCount++
		stepName := fmt.Sprintf("iteration_%d", s.iterCount)
		if s.lastStep != "" {
			if err := writeSSE(ctx, w, sseW, aguievents.NewStepFinishedEvent(s.lastStep)); err != nil {
				return nil, err
			}
		}
		s.lastStep = stepName
		return nil, writeSSE(ctx, w, sseW, aguievents.NewStepStartedEvent(stepName))

	case api.EventIterationStop:
		if s.lastStep != "" {
			if err := writeSSE(ctx, w, sseW, aguievents.NewStepFinishedEvent(s.lastStep)); err != nil {
				return nil, err
			}
			s.lastStep = ""
		}
		return nil, nil

	case api.EventError:
		msg := "runtime error"
		if evt.Output != nil {
			if s, ok := evt.Output.(string); ok && s != "" {
				msg = s
			}
		}
		return nil, writeSSE(ctx, w, sseW, aguievents.NewRunErrorEvent(msg, aguievents.WithRunID(s.runID)))
	}
	return nil, nil
}

// finalize emits closing events for any open tool calls, text message,
// steps, and the RUN_FINISHED event.
func (s *streamState) finalize(ctx context.Context, w io.Writer, sseW sseWriter, filter textFilter) error {
	if s.lastToolID != "" {
		if err := writeSSE(ctx, w, sseW, aguievents.NewToolCallEndEvent(s.lastToolID)); err != nil {
			return err
		}
		delete(s.toolNames, s.lastToolID)
		s.lastToolID = ""
	}
	if s.reasoningStarted && !s.textStarted {
		if err := writeSSE(ctx, w, sseW, aguievents.NewReasoningMessageEndEvent("reasoning_"+s.msgID)); err != nil {
			return err
		}
	}
	if tail := filter.Flush(); tail != "" && s.textStarted {
		if err := writeSSE(ctx, w, sseW, aguievents.NewTextMessageContentEvent(s.msgID, tail)); err != nil {
			return err
		}
	}
	if s.textStarted {
		if err := writeSSE(ctx, w, sseW, aguievents.NewTextMessageEndEvent(s.msgID)); err != nil {
			return err
		}
	}
	if s.lastStep != "" {
		if err := writeSSE(ctx, w, sseW, aguievents.NewStepFinishedEvent(s.lastStep)); err != nil {
			return err
		}
		s.lastStep = ""
	}
	return writeSSE(ctx, w, sseW, aguievents.NewRunFinishedEvent(s.threadID, s.runID))
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
// SDK writer's network timeout.
func writeSSE(ctx context.Context, w io.Writer, sseW sseWriter, event aguievents.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return sseW.WriteEventWithType(ctx, w, event, string(event.Type()))
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