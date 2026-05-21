package agui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
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
	Timeout float64           `json:"timeout,omitempty"` // per-server timeout in seconds (0 = use global)
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
	return ParseMCPServersRaw(raw)
}

// ParseMCPServersRaw parses MCP server configs from a raw JSON value.
// Each element in the array can be either an object (ClientMCPServer) or a URI string.
func ParseMCPServersRaw(raw any) ([]ClientMCPServer, error) {
	arr, ok := raw.([]any)
	if !ok {
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal mcp_servers: %w", err)
		}
		var servers []ClientMCPServer
		if err := json.Unmarshal(data, &servers); err != nil {
			return nil, fmt.Errorf("unmarshal mcp_servers: %w", err)
		}
		return validateMCPEntries(servers)
	}

	servers := make([]ClientMCPServer, 0, len(arr))
	for i, elem := range arr {
		switch v := elem.(type) {
		case string:
			s, err := ParseMCPURI(v)
			if err != nil {
				return nil, fmt.Errorf("mcp_servers[%d]: %w", i, err)
			}
			servers = append(servers, *s)
		case map[string]any:
			data, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("mcp_servers[%d]: marshal: %w", i, err)
			}
			var s ClientMCPServer
			if err := json.Unmarshal(data, &s); err != nil {
				return nil, fmt.Errorf("mcp_servers[%d]: unmarshal: %w", i, err)
			}
			servers = append(servers, s)
		default:
			return nil, fmt.Errorf("mcp_servers[%d]: expected string or object, got %T", i, elem)
		}
	}

	return validateMCPEntries(servers)
}

func validateMCPEntries(servers []ClientMCPServer) ([]ClientMCPServer, error) {
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

// parseMCPURI parses an MCP server URI string into a ClientMCPServer.
//
// Supported formats:
//   - https://host/path?name=xxx&timeout=30  → type "http"
//   - http://host/path?name=xxx              → type "http"
//   - sse://host/path?name=xxx               → type "sse" (URL uses https)
//   - stdio:///command?args=a1&args=a2&name=xxx → type "stdio"
func ParseMCPURI(raw string) (*ClientMCPServer, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URI: %w", err)
	}

	query := u.Query()
	name := query.Get("name")
	if name == "" {
		return nil, fmt.Errorf("missing required query param: name")
	}

	var timeout float64
	if v := query.Get("timeout"); v != "" {
		if t, err := strconv.ParseFloat(v, 64); err == nil && t > 0 {
			timeout = t
		}
	}

	// Extract headers from header_* query params.
	headers := make(map[string]string)
	for key, vals := range query {
		if strings.HasPrefix(key, "header_") && len(vals) > 0 {
			headerName := strings.TrimPrefix(key, "header_")
			headers[headerName] = vals[0]
		}
	}

	// Clean query params that are metadata (not part of the server URL).
	cleanQuery := make(url.Values)
	for key, vals := range query {
		if key == "name" || key == "timeout" || key == "args" || strings.HasPrefix(key, "header_") {
			continue
		}
		cleanQuery[key] = vals
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https", "http":
		serverURL := scheme + "://" + u.Host + u.Path
		if len(cleanQuery) > 0 {
			serverURL += "?" + cleanQuery.Encode()
		}
		srv := &ClientMCPServer{
			Name:    name,
			Type:    "http",
			URL:     serverURL,
			Timeout: timeout,
		}
		if len(headers) > 0 {
			srv.Headers = headers
		}
		return srv, nil

	case "sse":
		serverURL := "https://" + u.Host + u.Path
		if len(cleanQuery) > 0 {
			serverURL += "?" + cleanQuery.Encode()
		}
		srv := &ClientMCPServer{
			Name:    name,
			Type:    "sse",
			URL:     serverURL,
			Timeout: timeout,
		}
		if len(headers) > 0 {
			srv.Headers = headers
		}
		return srv, nil

	case "stdio":
		command := strings.TrimPrefix(u.Path, "/")
		if command == "" {
			return nil, fmt.Errorf("stdio URI: missing command in path")
		}
		args := query["args"]
		srv := &ClientMCPServer{
			Name:    name,
			Type:    "stdio",
			Command: command,
			Args:    args,
			Timeout: timeout,
		}
		return srv, nil

	default:
		return nil, fmt.Errorf("unsupported MCP URI scheme: %q", scheme)
	}
}

// ValidateMCPSecurity enforces operator security limits on MCP server configs.
// maxServers caps the number of MCP servers per session (0 defaults to 5).
// allowStdio controls whether stdio-type servers are permitted.
func ValidateMCPSecurity(servers []ClientMCPServer, maxServers int, allowStdio bool) error {
	if maxServers == 0 {
		maxServers = 5
	}
	if len(servers) > maxServers {
		return fmt.Errorf("too many MCP servers: %d exceeds limit of %d", len(servers), maxServers)
	}

	if !allowStdio {
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
func ValidateMCPAllowList(servers []ClientMCPServer, allowList []string) error {
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
