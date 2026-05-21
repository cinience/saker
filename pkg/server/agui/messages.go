package agui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/message"
	"github.com/saker-ai/saker/pkg/model"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// convertFrontendTools converts AG-UI Tool definitions (from CopilotKit frontend
// actions) into saker model.ToolDefinition for LLM awareness.
func convertFrontendTools(tools []aguitypes.Tool) []model.ToolDefinition {
	out := make([]model.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		params, _ := t.Parameters.(map[string]any)
		out = append(out, model.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  params,
		})
	}
	return out
}

// messagesToRequest converts AG-UI RunAgentInput into a saker api.Request.
// Uses the ThreadID as SessionID so saker's runtime manages conversation
// history across turns. The latest user message becomes the Prompt.
// All preceding messages are converted to PreloadHistory so the runtime
// can restore context after worker failover without requiring a shared DB.
// Multimodal content (images, documents) is converted to ContentBlocks.
func messagesToRequest(input aguitypes.RunAgentInput, identity Identity) api.Request {
	var prompt string
	var contentBlocks []model.ContentBlock
	var lastUserIdx int = -1

	for i := len(input.Messages) - 1; i >= 0; i-- {
		if input.Messages[i].Role == aguitypes.RoleUser {
			prompt, contentBlocks = extractMultimodalContent(input.Messages[i])
			lastUserIdx = i
			break
		}
	}

	if len(input.Context) > 0 {
		var parts []string
		for _, ctx := range input.Context {
			parts = append(parts, fmt.Sprintf("%s: %s", ctx.Description, ctx.Value))
		}
		prompt = strings.Join(parts, "\n") + "\n\n" + prompt
	}

	// Convert all messages preceding the last user message into PreloadHistory.
	// This allows the runtime to seed session history on a fresh worker.
	var preload []message.Message
	if lastUserIdx > 0 {
		preload = aguiMessagesToHistory(input.Messages[:lastUserIdx])
	}

	req := api.Request{
		Prompt:         prompt,
		ContentBlocks:  contentBlocks,
		SessionID:      input.ThreadID,
		Ephemeral:      true,
		PreloadHistory: preload,
	}
	if identity.Username != "" {
		req.User = identity.Username
	}

	// Forward frontend tools as ExtraTools for model awareness and mark them
	// as passthrough so the agent loop exits instead of trying to execute them.
	if len(input.Tools) > 0 {
		req.ExtraTools = convertFrontendTools(input.Tools)
		names := make([]string, len(input.Tools))
		for i, t := range input.Tools {
			names[i] = t.Name
		}
		req.PassthroughTools = names
	}

	// Forward ParentRunID for subagent tracing.
	if input.ParentRunID != nil && *input.ParentRunID != "" {
		req.ParentSessionID = *input.ParentRunID
	}

	// Forward state and props to metadata for downstream access.
	meta := make(map[string]any)
	if input.ForwardedProps != nil {
		meta["_agui_forwarded_props"] = input.ForwardedProps
	}
	if input.State != nil {
		meta["_agui_state"] = input.State
	}
	if len(meta) > 0 {
		req.Metadata = meta
	}

	return req
}

// extractMultimodalContent extracts text prompt and multimodal content blocks
// from an AG-UI message. Uses the SDK's ContentInputContents() helper for
// proper type handling of multimodal arrays.
// When multiple files are attached, a manifest line is prepended to the prompt
// so the LLM can reference files by ordinal (e.g. "第2个素材").
func extractMultimodalContent(msg aguitypes.Message) (string, []model.ContentBlock) {
	// Try multimodal array first ([]InputContent).
	if parts, ok := msg.ContentInputContents(); ok {
		var texts []string
		var blocks []model.ContentBlock
		for _, part := range parts {
			switch part.Type {
			case aguitypes.InputContentTypeText:
				if part.Text != "" {
					texts = append(texts, part.Text)
				}
			case aguitypes.InputContentTypeBinary:
				block := inputContentToBlock(part)
				if block.Type != "" {
					blocks = append(blocks, block)
				}
			default:
				// Unknown types with text fallback.
				if part.Text != "" {
					texts = append(texts, part.Text)
				}
			}
		}
		prompt := strings.Join(texts, "\n")
		if len(blocks) > 0 {
			prompt = prependFileManifest(prompt, blocks)
		}
		return prompt, blocks
	}

	// Fall back to string content.
	text := extractTextContent(msg.Content)
	return text, nil
}

// prependFileManifest injects a concise file list at the start of the prompt
// so the LLM knows which ordinal maps to which file.
func prependFileManifest(prompt string, blocks []model.ContentBlock) string {
	if len(blocks) == 0 {
		return prompt
	}
	var items []string
	for i, b := range blocks {
		name := b.Filename
		if name == "" {
			name = fmt.Sprintf("%s_file", b.Type)
		}
		items = append(items, fmt.Sprintf("附件%d: %s (%s)", i+1, name, b.MediaType))
	}
	manifest := "[" + strings.Join(items, ", ") + "]"
	if prompt == "" {
		return manifest
	}
	return manifest + "\n" + prompt
}

// inputContentToBlock converts an AG-UI InputContent (binary type) into a
// saker model.ContentBlock for multimodal processing.
func inputContentToBlock(ic aguitypes.InputContent) model.ContentBlock {
	blockType := mimeToBlockType(ic.MimeType)
	if blockType == "" {
		return model.ContentBlock{}
	}
	return model.ContentBlock{
		Type:      blockType,
		MediaType: ic.MimeType,
		Data:      ic.Data,
		URL:       ic.URL,
		Filename:  ic.Filename,
	}
}

// mimeToBlockType maps MIME types to saker ContentBlockType.
func mimeToBlockType(mimeType string) model.ContentBlockType {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return model.ContentBlockImage
	case strings.HasPrefix(mimeType, "video/"):
		return model.ContentBlockVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return model.ContentBlockAudio
	case mimeType == "application/pdf":
		return model.ContentBlockDocument
	case strings.HasPrefix(mimeType, "application/"):
		return model.ContentBlockDocument
	default:
		return ""
	}
}

// aguiMessagesToHistory converts AG-UI messages into saker message.Message
// slice suitable for seeding runtime history (PreloadHistory). This enables
// stateless worker failover: synapse injects stored conversation history into
// the AG-UI body, and saker uses it to rebuild context.
func aguiMessagesToHistory(msgs []aguitypes.Message) []message.Message {
	out := make([]message.Message, 0, len(msgs))
	for _, m := range msgs {
		role := mapAGUIRoleToHistory(m.Role)
		if role == "" {
			continue
		}
		msg := message.Message{
			Role:    role,
			Content: extractTextContent(m.Content),
		}
		for _, tc := range m.ToolCalls {
			args := parseToolCallArgs(tc.Function.Arguments)
			msg.ToolCalls = append(msg.ToolCalls, message.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
		// Tool result messages: store the content as a tool call result
		// linked back to the original call via ToolCallID.
		if m.ToolCallID != "" && role == "tool" {
			msg.ToolCalls = []message.ToolCall{{
				ID:     m.ToolCallID,
				Result: msg.Content,
			}}
		}
		out = append(out, msg)
	}
	return out
}

// mapAGUIRoleToHistory maps AG-UI roles to saker message roles.
func mapAGUIRoleToHistory(role aguitypes.Role) string {
	switch role {
	case aguitypes.RoleUser:
		return "user"
	case aguitypes.RoleAssistant:
		return "assistant"
	case aguitypes.RoleSystem, aguitypes.RoleDeveloper:
		return "system"
	case aguitypes.RoleTool:
		return "tool"
	default:
		return ""
	}
}

// parseToolCallArgs parses a JSON arguments string into a map.
func parseToolCallArgs(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var args map[string]any
	if json.Unmarshal([]byte(raw), &args) != nil {
		return map[string]any{"_raw": raw}
	}
	return args
}

// extractTextContent coerces AG-UI message content (typed as any) into a
// plain string. Handles: string, nil, []InputContent (joins text parts),
// and arbitrary JSON.
func extractTextContent(content any) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		var s string
		if json.Unmarshal(b, &s) == nil {
			return s
		}
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(b, &parts) == nil {
			var texts []string
			for _, p := range parts {
				if p.Type == "text" && p.Text != "" {
					texts = append(texts, p.Text)
				}
			}
			if len(texts) > 0 {
				return strings.Join(texts, "\n")
			}
		}
		return string(b)
	}
}
