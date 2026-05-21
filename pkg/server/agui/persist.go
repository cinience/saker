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
	seen := make(map[string]bool)
	for _, evt := range events {
		if evt.Kind != string(conversation.EventKindAssistantText) && evt.Kind != string(conversation.EventKindToolResult) {
			continue
		}
		// Path 1: structured artifacts in ContentJSON.
		if len(evt.ContentJSON) > 0 {
			var payload struct {
				Artifacts []server.Artifact `json:"artifacts"`
			}
			if json.Unmarshal(evt.ContentJSON, &payload) == nil && len(payload.Artifacts) > 0 {
				for _, a := range payload.Artifacts {
					if !seen[a.URL] {
						seen[a.URL] = true
						arts = append(arts, a)
					}
				}
				continue
			}
		}
		// Path 2: extract media URLs from plain-text tool results.
		if evt.Kind == string(conversation.EventKindToolResult) && evt.ContentText != "" {
			extracted := server.ExtractArtifacts("", map[string]any{"output": evt.ContentText})
			for _, a := range extracted {
				if !seen[a.URL] {
					seen[a.URL] = true
					arts = append(arts, a)
				}
			}
		}
	}
	return arts
}
