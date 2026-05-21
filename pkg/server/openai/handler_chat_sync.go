package openai

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/saker-ai/saker/pkg/runhub"
	"github.com/saker-ai/saker/pkg/server"
)

// streamChatSync collapses every chunk into a single chat.completion
// response and writes it as JSON. Mirrors the OpenAI non-streaming
// shape so SDKs can call this path interchangeably with stream=true.
func (g *Gateway) streamChatSync(c *gin.Context, hubRun *runhub.Run, extra ExtraBody, modelID, chunkID string) {
	eventsCh, backfill, unsub := hubRun.Subscribe()
	defer unsub()

	var (
		contentBuf   strings.Builder
		toolCallMap  = map[int]*ChatToolCall{}
		finishReason string
		usage        *ChatUsage
	)

	consume := func(e runhub.Event) {
		var chunk ChatCompletionChunk
		if err := json.Unmarshal(e.Data, &chunk); err != nil {
			return
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, ch := range chunk.Choices {
			if ch.Delta == nil {
				continue
			}
			if ch.Delta.Content != "" {
				contentBuf.WriteString(ch.Delta.Content)
			}
			for _, tc := range ch.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				if existing, ok := toolCallMap[idx]; ok {
					existing.Function.Arguments += tc.Function.Arguments
					if tc.ID != "" {
						existing.ID = tc.ID
					}
					if tc.Function.Name != "" {
						existing.Function.Name = tc.Function.Name
					}
				} else {
					clone := tc
					clone.Index = nil
					toolCallMap[idx] = &clone
				}
			}
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
		}
	}

	for _, e := range backfill {
		consume(e)
	}

	timer := time.NewTimer(server.DefaultTurnTimeout)
	defer timer.Stop()

	clientCtx := c.Request.Context()

loop:
	for {
		select {
		case e, ok := <-eventsCh:
			if !ok {
				break loop
			}
			consume(e)
		case <-timer.C:
			ServerError(c, "timeout waiting for completion")
			return
		case <-clientCtx.Done():
			if extra.EffectiveCancelOnDisconnect() {
				_ = g.hub.Cancel(hubRun.ID)
			}
			return
		}
	}

	if finishReason == "" {
		finishReason = "stop"
	}
	msg := &ChatMessageOut{
		Role:    "assistant",
		Content: server.CleanAssistantReply(contentBuf.String()),
	}
	if len(toolCallMap) > 0 {
		maxIdx := 0
		for idx := range toolCallMap {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		toolCalls := make([]ChatToolCall, 0, len(toolCallMap))
		for i := 0; i <= maxIdx; i++ {
			if tc, ok := toolCallMap[i]; ok {
				toolCalls = append(toolCalls, *tc)
			}
		}
		msg.ToolCalls = toolCalls
	}
	resp := ChatCompletionResponse{
		ID:      chunkID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelID,
		Choices: []ChatChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
	c.JSON(http.StatusOK, resp)
}

// makeChatChunkID returns a stable chat.completion id derived from the
// hub run id. The "chatcmpl-" prefix mirrors OpenAI's wire format so
// SDKs that prefix-match (e.g. for telemetry) keep working.
func makeChatChunkID(runID string) string {
	return "chatcmpl-" + strings.TrimPrefix(runID, "run_")
}
