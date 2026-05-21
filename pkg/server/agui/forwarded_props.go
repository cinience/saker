package agui

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/config"
)

// applyForwardedProps extracts client-side LLM/tool/prompt configuration from
// the AG-UI ForwardedProps map and applies them to the saker request.
// Security switches in opts control which overrides are honored.
func applyForwardedProps(props map[string]any, req *api.Request, opts Options) error {
	// --- Model endpoint (URI string or []string for failover) ---
	if !opts.DenyModelEndpoint {
		if raw, ok := props["model_uri"]; ok {
			uris, err := CoerceModelURIs(raw)
			if err != nil {
				return fmt.Errorf("model_uri: %w", err)
			}
			var entries []config.FailoverModelEntry
			for i, uri := range uris {
				entry, overrides, err := ParseModelURI(uri)
				if err != nil {
					return fmt.Errorf("model_uri[%d]: %w", i, err)
				}
				entries = append(entries, *entry)
				if i == 0 && overrides != nil {
					req.ModelOverrides = overrides
				}
			}
			if len(entries) > 0 {
				req.ModelEndpoint = entries
			}
		}
	}

	// --- System prompt ---
	if text, ok := props["system_prompt"].(string); ok && text != "" {
		mode := api.SystemPromptModePrepend
		if raw, ok := props["system_prompt_mode"].(string); ok && raw != "" {
			m := api.SystemPromptMode(strings.ToLower(strings.TrimSpace(raw)))
			switch m {
			case api.SystemPromptModePrepend, api.SystemPromptModeAppend, api.SystemPromptModeReplace:
				mode = m
			default:
				return fmt.Errorf("system_prompt_mode: unknown %q", raw)
			}
		}
		if mode == api.SystemPromptModeReplace && opts.DenySystemPromptReplace {
			mode = api.SystemPromptModePrepend
		}
		req.SystemPromptOverride = &api.SystemPromptOverride{Text: text, Mode: mode}
	}

	// --- Tool configuration ---
	if !opts.DenyToolOverride {
		if raw, ok := props["allowed_tools"]; ok {
			list, err := coerceStringSlice(raw)
			if err != nil {
				return fmt.Errorf("allowed_tools: %w", err)
			}
			if len(list) > 0 {
				req.ToolWhitelist = list
			}
		}
		if raw, ok := props["passthrough_tools"]; ok {
			list, err := coerceStringSlice(raw)
			if err != nil {
				return fmt.Errorf("passthrough_tools: %w", err)
			}
			req.PassthroughTools = append(req.PassthroughTools, list...)
		}
	}

	return nil
}

// parseModelURI parses a model endpoint URI into a FailoverModelEntry and optional ModelOverrides.
//
// Format: provider://api_key@host[:port]/path?model=name&temperature=0.7&...
//
// Examples:
//   - openai://sk-xxx@api.openai.com/v1?model=gpt-4o&temperature=0.7
//   - anthropic://sk-ant-xxx@api.anthropic.com?model=claude-sonnet-4-20250514
//   - openai://ollama@localhost:11434/v1?model=llama3
func ParseModelURI(raw string) (*config.FailoverModelEntry, *api.ModelOverrides, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid URI: %w", err)
	}

	provider := strings.ToLower(u.Scheme)
	if provider == "" {
		return nil, nil, fmt.Errorf("missing provider scheme")
	}
	switch provider {
	case "openai", "anthropic", "dashscope":
	default:
		return nil, nil, fmt.Errorf("unsupported provider: %q", provider)
	}

	var apiKey string
	if u.User != nil {
		apiKey = u.User.Username()
	}

	baseURL := buildBaseURL(u)

	query := u.Query()
	modelName := query.Get("model")
	if modelName == "" {
		return nil, nil, fmt.Errorf("missing required query param: model")
	}

	entry := &config.FailoverModelEntry{
		Provider: provider,
		Model:    modelName,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}

	overrides := parseModelOverrides(query)

	return entry, overrides, nil
}

// buildBaseURL reconstructs the base URL from the parsed URI.
// Uses http:// for localhost/loopback, https:// otherwise.
func buildBaseURL(u *url.URL) string {
	scheme := "https"
	host := u.Hostname()
	if isLoopback(host) {
		scheme = "http"
	}

	base := scheme + "://" + u.Host
	if u.Path != "" && u.Path != "/" {
		base += u.Path
	}
	return base
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// parseModelOverrides extracts sampling parameters from URI query params.
// Returns nil if no override params are present.
func parseModelOverrides(query url.Values) *api.ModelOverrides {
	var ov api.ModelOverrides
	set := false

	if v := query.Get("temperature"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			ov.Temperature = &f
			set = true
		}
	}
	if v := query.Get("top_p"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			ov.TopP = &f
			set = true
		}
	}
	if v := query.Get("max_tokens"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ov.MaxTokens = &n
			set = true
		}
	}
	if v := query.Get("stop"); v != "" {
		ov.Stop = strings.Split(v, ",")
		set = true
	}
	if v := query.Get("seed"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			ov.Seed = &n
			set = true
		}
	}
	if v := query.Get("tool_choice"); v != "" {
		ov.ToolChoice = v
		set = true
	}
	if v := query.Get("parallel_tool_calls"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			ov.ParallelToolCalls = &b
			set = true
		}
	}

	if !set {
		return nil
	}
	return &ov
}

// coerceModelURIs accepts a string or []string (or []any of strings) for model_uri.
func CoerceModelURIs(raw any) ([]string, error) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, fmt.Errorf("empty URI")
		}
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string array, got %T element", item)
			}
			if s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("empty URI array")
		}
		return out, nil
	case []string:
		if len(v) == 0 {
			return nil, fmt.Errorf("empty URI array")
		}
		return v, nil
	default:
		return nil, fmt.Errorf("expected string or string array, got %T", raw)
	}
}

// coerceStringSlice converts an any value (expected []any or []string from JSON) to []string.
func coerceStringSlice(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string array, got %T element", item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("expected string array: %w", err)
		}
		var out []string
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("expected string array: %w", err)
		}
		return out, nil
	}
}
