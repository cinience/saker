package agui

import (
	"context"
	"encoding/json"

	"github.com/saker-ai/saker/pkg/conversation"
	"github.com/saker-ai/saker/pkg/server"
)

const aguiClient = "agui"

// loadArtifactsFromDB queries the conversation store for persisted artifacts
// as a fallback when the in-memory cache has expired.
func (g *Gateway) loadArtifactsFromDB(ctx context.Context, threadID string) []server.Artifact {
	cs := g.deps.ConversationStore
	if cs == nil {
		return nil
	}
	events, err := cs.GetEvents(ctx, threadID, conversation.GetEventsOpts{})
	if err != nil {
		return nil
	}
	var arts []server.Artifact
	for _, evt := range events {
		if (evt.Kind != string(conversation.EventKindAssistantText) && evt.Kind != string(conversation.EventKindToolResult)) || len(evt.ContentJSON) == 0 {
			continue
		}
		var payload struct {
			Artifacts []server.Artifact `json:"artifacts"`
		}
		if json.Unmarshal(evt.ContentJSON, &payload) == nil && len(payload.Artifacts) > 0 {
			arts = append(arts, payload.Artifacts...)
		}
	}
	return arts
}
