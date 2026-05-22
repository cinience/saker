package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/runhub"
	"github.com/saker-ai/saker/pkg/server"
)

// runChatProducer drains the saker stream, translates each event into
// OpenAI chat.completion.chunk envelopes, JSON-marshals them, and
// publishes onto the hub's ring + subscribers. On stream end (channel
// closed), the run is marked terminal so subscribers see chan-close.
//
// producerCancel is the WithTimeout cancel returned alongside producerCtx;
// the producer defers it so the timer goroutine is reclaimed promptly when
// the saker stream finishes naturally instead of waiting out the 45-min
// turn timeout.
func (g *Gateway) runChatProducer(eventCh <-chan api.StreamEvent, hubRun *runhub.Run, producerCancel context.CancelFunc, chunkID, model string, exposeTools, includeUsage bool) {
	defer producerCancel()
	builder := newChatChunkBuilder(chunkID, hubRun.ID, model, g.deps.Options.ErrorDetailMode)
	filter := server.NewStreamArtifactFilter()
	var assistantText strings.Builder

	var jsonBuf bytes.Buffer
	jsonEnc := json.NewEncoder(&jsonBuf)
	marshalToBytes := func(v any) ([]byte, error) {
		jsonBuf.Reset()
		if err := jsonEnc.Encode(v); err != nil {
			return nil, err
		}
		b := jsonBuf.Bytes()
		if len(b) > 0 && b[len(b)-1] == '\n' {
			b = b[:len(b)-1]
		}
		return append([]byte(nil), b...), nil // copy because hubRun.Publish may retain
	}

	finalStatus := runhub.RunStatusCompleted
	for evt := range eventCh {
		if evt.Type == api.EventError {
			finalStatus = runhub.RunStatusFailed
		}
		if evt.Type == api.EventContentBlockDelta && evt.Delta != nil && evt.Delta.Text != "" {
			assistantText.WriteString(evt.Delta.Text)
		}
		chunks, _ := builder.translate(evt, exposeTools, filter)
		for _, ch := range chunks {
			data, err := marshalToBytes(ch)
			if err != nil {
				continue
			}
			hubRun.Publish("chunk", data)
		}
	}

	// If the saker stream closed without ever firing a finish-bearing
	// chunk, synthesize a "stop" so SDKs see a clean end-of-stream.
	if builder.finish == "" {
		chunk := builder.envelope(ChatChoice{
			Index:        0,
			Delta:        &ChatMessageOut{},
			FinishReason: "stop",
		})
		if data, err := marshalToBytes(chunk); err == nil {
			hubRun.Publish("chunk", data)
		}
	}

	// Always emit a usage envelope when we observed any token counts, but
	// publish it with type="usage" so subscribers can decide whether to
	// forward it. SSE forwards only when stream_options.include_usage=true
	// (OpenAI spec requires the empty-choices frame to be opt-in); the sync
	// path always consumes it for the response.usage field.
	_ = includeUsage // SSE path filters by event Type, not by this flag
	if chunk, ok := builder.usageChunk(); ok {
		if data, err := marshalToBytes(chunk); err == nil {
			hubRun.Publish("usage", data)
		}
	}

	// Clean up any pending ask that was never answered (e.g. context cancelled).
	if g.pendingAsks != nil {
		if pa := g.pendingAsks.Lookup(hubRun.ID); pa != nil {
			select {
			case pa.AnswerCh <- askAnswer{Action: "cancel"}:
			default:
			}
			g.pendingAsks.Remove(hubRun.ID)
		}
	}

	g.hub.Finish(hubRun.ID, finalStatus)
}
