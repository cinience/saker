package agui

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/saker-ai/saker/pkg/conversation"
	"github.com/saker-ai/saker/pkg/server"
)

const aguiClient = "agui"

func (g *Gateway) ensureThread(ctx context.Context, threadID string, identity Identity) {
	cs := g.deps.ConversationStore
	if cs == nil {
		return
	}
	projectID := identity.ProjectID
	if projectID == "" {
		projectID = "default"
	}
	owner := identity.UserID
	if owner == "" {
		owner = identity.Username
	}
	if owner == "" {
		owner = "anonymous"
	}
	existing, err := cs.GetThread(ctx, threadID)
	if err == nil {
		if existing.Client != aguiClient {
			_ = cs.UpdateThreadClient(ctx, threadID, aguiClient)
		}
		return
	}
	if _, err := cs.CreateThreadWithID(ctx, threadID, projectID, owner, "", aguiClient); err != nil {
		g.deps.Logger.Warn("agui: failed to create thread", "thread_id", threadID, "error", err)
	}
}

func (g *Gateway) persistUserMessage(ctx context.Context, threadID, turnID, projectID, text string) {
	cs := g.deps.ConversationStore
	if cs == nil || strings.TrimSpace(text) == "" {
		return
	}
	if projectID == "" {
		projectID = "default"
	}
	if _, err := cs.AppendEvent(ctx, conversation.AppendEventInput{
		ThreadID:    threadID,
		ProjectID:   projectID,
		TurnID:      turnID,
		Kind:        conversation.EventKindUserMessage,
		ContentText: text,
	}); err != nil {
		g.deps.Logger.Warn("agui: failed to persist user message", "thread_id", threadID, "error", err)
	}
}

func (g *Gateway) persistAssistantMessage(ctx context.Context, threadID, turnID, projectID, text string) {
	g.persistAssistantWithArtifacts(ctx, threadID, turnID, projectID, text, nil)
}

func (g *Gateway) persistAssistantWithArtifacts(ctx context.Context, threadID, turnID, projectID, text string, arts []server.Artifact) {
	cs := g.deps.ConversationStore
	if cs == nil {
		return
	}
	if projectID == "" {
		projectID = "default"
	}

	fullText := text

	if strings.TrimSpace(fullText) == "" {
		return
	}

	var contentJSON any
	if len(arts) > 0 {
		contentJSON = map[string]any{"artifacts": arts}
	}

	if _, err := cs.AppendEvent(ctx, conversation.AppendEventInput{
		ThreadID:    threadID,
		ProjectID:   projectID,
		TurnID:      turnID,
		Kind:        conversation.EventKindAssistantText,
		ContentText: fullText,
		ContentJSON: contentJSON,
	}); err != nil {
		g.deps.Logger.Warn("agui: failed to persist assistant message", "thread_id", threadID, "error", err)
	}
}

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
		if evt.Kind != string(conversation.EventKindAssistantText) || len(evt.ContentJSON) == 0 {
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
