package agui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ClientMCPServer describes an MCP server configuration sent by the frontend
// via ForwardedProps.mcp_servers.
type ClientMCPServer struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"` // "http", "sse", "stdio"
	URL     string            `json:"url,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Spec returns the connection spec string used by the tool registry.
func (c ClientMCPServer) Spec() string {
	switch c.Type {
	case "http", "sse":
		return c.URL
	case "stdio":
		if len(c.Args) > 0 {
			return "stdio://" + c.Command + " " + strings.Join(c.Args, " ")
		}
		return "stdio://" + c.Command
	default:
		if c.URL != "" {
			return c.URL
		}
		return ""
	}
}

// extractMCPServers parses MCP server configs from ForwardedProps.
func extractMCPServers(props map[string]any) ([]ClientMCPServer, error) {
	raw, ok := props["mcp_servers"]
	if !ok {
		return nil, nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp_servers: %w", err)
	}

	var servers []ClientMCPServer
	if err := json.Unmarshal(data, &servers); err != nil {
		return nil, fmt.Errorf("unmarshal mcp_servers: %w", err)
	}

	for i, s := range servers {
		if s.Name == "" {
			return nil, fmt.Errorf("mcp_servers[%d]: name is required", i)
		}
		if s.Spec() == "" {
			return nil, fmt.Errorf("mcp_servers[%d] (%s): url or command is required", i, s.Name)
		}
	}

	return servers, nil
}

// validateMCPSecurity enforces operator security limits on MCP server configs.
func validateMCPSecurity(servers []ClientMCPServer, opts Options) error {
	maxServers := opts.MaxMCPServersPerSession
	if maxServers == 0 {
		maxServers = 5
	}
	if len(servers) > maxServers {
		return fmt.Errorf("too many MCP servers: %d exceeds limit of %d", len(servers), maxServers)
	}

	if !opts.AllowMCPStdio {
		for _, s := range servers {
			if s.Type == "stdio" {
				return fmt.Errorf("mcp server %q: stdio transport is not permitted", s.Name)
			}
		}
	}

	return nil
}

// validateMCPAllowList checks whether the given servers are permitted by the
// operator's allow-list patterns. An empty allowList permits everything.
func validateMCPAllowList(servers []ClientMCPServer, allowList []string) error {
	if len(allowList) == 0 {
		return nil
	}

	allowed := make(map[string]struct{}, len(allowList))
	for _, p := range allowList {
		allowed[p] = struct{}{}
	}

	for _, s := range servers {
		if !matchesAllowList(s, allowed, allowList) {
			return fmt.Errorf("mcp server %q (%s) is not in the allowed list", s.Name, s.Spec())
		}
	}
	return nil
}

func matchesAllowList(s ClientMCPServer, _ map[string]struct{}, patterns []string) bool {
	spec := s.Spec()
	for _, p := range patterns {
		if p == s.Name {
			return true
		}
		if p == spec {
			return true
		}
		if strings.HasSuffix(p, "*") && strings.HasPrefix(spec, strings.TrimSuffix(p, "*")) {
			return true
		}
	}
	return false
}
