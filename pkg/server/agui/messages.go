package agui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/model"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// messagesToRequest converts AG-UI RunAgentInput into a saker api.Request.
// Uses the ThreadID as SessionID so saker's runtime manages conversation
// history across turns. The latest user message becomes the Prompt.
// Multimodal content (images, documents) is converted to ContentBlocks.
func messagesToRequest(input aguitypes.RunAgentInput, identity Identity) api.Request {
	var prompt string
	var contentBlocks []model.ContentBlock

	for i := len(input.Messages) - 1; i >= 0; i-- {
		if input.Messages[i].Role == aguitypes.RoleUser {
			prompt, contentBlocks = extractMultimodalContent(input.Messages[i])
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

	req := api.Request{
		Prompt:        prompt,
		ContentBlocks: contentBlocks,
		SessionID:     input.ThreadID,
		Ephemeral:     true,
	}
	if identity.Username != "" {
		req.User = identity.Username
	}

	// Forward props to metadata for downstream access.
	if input.ForwardedProps != nil {
		req.Metadata = map[string]any{
			"_agui_forwarded_props": input.ForwardedProps,
		}
	}

	return req
}

// extractMultimodalContent extracts text prompt and multimodal content blocks
// from an AG-UI message. Uses the SDK's ContentInputContents() helper for
// proper type handling of multimodal arrays.
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
		return strings.Join(texts, "\n"), blocks
	}

	// Fall back to string content.
	text := extractTextContent(msg.Content)
	return text, nil
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
	}
}

// mimeToBlockType maps MIME types to saker ContentBlockType.
func mimeToBlockType(mimeType string) model.ContentBlockType {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return model.ContentBlockImage
	case mimeType == "application/pdf":
		return model.ContentBlockDocument
	case strings.HasPrefix(mimeType, "application/"):
		return model.ContentBlockDocument
	default:
		return ""
	}
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
