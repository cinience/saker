package agui

import (
	"fmt"
	"testing"
)

func TestExtractMCPServers_Valid(t *testing.T) {
	props := map[string]any{
		"mcp_servers": []any{
			map[string]any{
				"name": "my-server",
				"type": "http",
				"url":  "https://mcp.example.com/sse",
			},
			map[string]any{
				"name":    "local-tool",
				"type":    "stdio",
				"command": "npx",
				"args":    []any{"-y", "@modelcontextprotocol/server-everything"},
			},
		},
	}

	servers, err := extractMCPServers(props)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}

	if servers[0].Name != "my-server" || servers[0].URL != "https://mcp.example.com/sse" {
		t.Errorf("server[0] mismatch: %+v", servers[0])
	}
	if servers[1].Name != "local-tool" || servers[1].Command != "npx" {
		t.Errorf("server[1] mismatch: %+v", servers[1])
	}
	if servers[1].Spec() != "stdio://npx -y @modelcontextprotocol/server-everything" {
		t.Errorf("server[1] spec mismatch: %s", servers[1].Spec())
	}
}

func TestExtractMCPServers_NoField(t *testing.T) {
	props := map[string]any{"timeout_seconds": 30}
	servers, err := extractMCPServers(props)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if servers != nil {
		t.Fatalf("expected nil, got %v", servers)
	}
}

func TestExtractMCPServers_MissingName(t *testing.T) {
	props := map[string]any{
		"mcp_servers": []any{
			map[string]any{"type": "http", "url": "https://example.com"},
		},
	}
	_, err := extractMCPServers(props)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestExtractMCPServers_MissingSpec(t *testing.T) {
	props := map[string]any{
		"mcp_servers": []any{
			map[string]any{"name": "bad", "type": "http"},
		},
	}
	_, err := extractMCPServers(props)
	if err == nil {
		t.Fatal("expected error for missing url/command")
	}
}

func TestValidateMCPAllowList_Empty(t *testing.T) {
	servers := []ClientMCPServer{{Name: "any", Type: "http", URL: "https://anything.com"}}
	if err := ValidateMCPAllowList(servers, nil); err != nil {
		t.Fatalf("empty allow-list should permit anything: %v", err)
	}
}

func TestValidateMCPAllowList_ExactMatch(t *testing.T) {
	servers := []ClientMCPServer{{Name: "my-server", Type: "http", URL: "https://mcp.example.com/sse"}}
	allowList := []string{"https://mcp.example.com/sse"}
	if err := ValidateMCPAllowList(servers, allowList); err != nil {
		t.Fatalf("exact URL match should pass: %v", err)
	}
}

func TestValidateMCPAllowList_NameMatch(t *testing.T) {
	servers := []ClientMCPServer{{Name: "my-server", Type: "http", URL: "https://mcp.example.com/sse"}}
	allowList := []string{"my-server"}
	if err := ValidateMCPAllowList(servers, allowList); err != nil {
		t.Fatalf("name match should pass: %v", err)
	}
}

func TestValidateMCPAllowList_PrefixMatch(t *testing.T) {
	servers := []ClientMCPServer{{Name: "s", Type: "http", URL: "https://mcp.example.com/v2/sse"}}
	allowList := []string{"https://mcp.example.com/*"}
	if err := ValidateMCPAllowList(servers, allowList); err != nil {
		t.Fatalf("prefix match should pass: %v", err)
	}
}

func TestValidateMCPAllowList_Denied(t *testing.T) {
	servers := []ClientMCPServer{{Name: "evil", Type: "http", URL: "https://evil.com/mcp"}}
	allowList := []string{"https://mcp.example.com/*"}
	if err := ValidateMCPAllowList(servers, allowList); err == nil {
		t.Fatal("should deny server not in allow-list")
	}
}

func TestValidateMCPSecurity_MaxServers(t *testing.T) {
	servers := make([]ClientMCPServer, 6)
	for i := range servers {
		servers[i] = ClientMCPServer{Name: fmt.Sprintf("srv%d", i), Type: "http", URL: fmt.Sprintf("https://s%d.com", i)}
	}
	// Default limit is 5.
	if err := ValidateMCPSecurity(servers, 0, false); err == nil {
		t.Fatal("expected error for 6 servers exceeding default limit of 5")
	}
	// Custom limit.
	if err := ValidateMCPSecurity(servers, 10, false); err != nil {
		t.Fatalf("should allow 6 servers with limit 10: %v", err)
	}
}

func TestValidateMCPSecurity_StdioBlocked(t *testing.T) {
	servers := []ClientMCPServer{{Name: "local", Type: "stdio", Command: "mcp-server"}}
	// Default: stdio not allowed.
	if err := ValidateMCPSecurity(servers, 0, false); err == nil {
		t.Fatal("expected error for stdio server when AllowMCPStdio is false")
	}
	// Explicitly allowed.
	if err := ValidateMCPSecurity(servers, 0, true); err != nil {
		t.Fatalf("should allow stdio when AllowMCPStdio is true: %v", err)
	}
}

func TestValidateMCPSecurity_HTTPAllowed(t *testing.T) {
	servers := []ClientMCPServer{{Name: "remote", Type: "http", URL: "https://mcp.example.com"}}
	if err := ValidateMCPSecurity(servers, 0, false); err != nil {
		t.Fatalf("http servers should always be allowed: %v", err)
	}
}

func TestClientMCPServer_Spec(t *testing.T) {
	cases := []struct {
		server ClientMCPServer
		want   string
	}{
		{ClientMCPServer{Type: "http", URL: "https://x.com/mcp"}, "https://x.com/mcp"},
		{ClientMCPServer{Type: "sse", URL: "https://x.com/sse"}, "https://x.com/sse"},
		{ClientMCPServer{Type: "stdio", Command: "node", Args: []string{"server.js"}}, "stdio://node server.js"},
		{ClientMCPServer{Type: "stdio", Command: "mcp-server"}, "stdio://mcp-server"},
		{ClientMCPServer{Type: "unknown", URL: "https://fallback.com"}, "https://fallback.com"},
		{ClientMCPServer{Type: "unknown"}, ""},
	}
	for _, tc := range cases {
		got := tc.server.Spec()
		if got != tc.want {
			t.Errorf("Spec() for %+v = %q, want %q", tc.server, got, tc.want)
		}
	}
}

func TestParseMCPURI_HTTP(t *testing.T) {
	s, err := ParseMCPURI("https://mcp.example.com/tools?name=weather&timeout=30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "weather" {
		t.Errorf("name = %q, want %q", s.Name, "weather")
	}
	if s.Type != "http" {
		t.Errorf("type = %q, want %q", s.Type, "http")
	}
	if s.URL != "https://mcp.example.com/tools" {
		t.Errorf("url = %q, want %q", s.URL, "https://mcp.example.com/tools")
	}
	if s.Timeout != 30 {
		t.Errorf("timeout = %v, want 30", s.Timeout)
	}
}

func TestParseMCPURI_SSE(t *testing.T) {
	s, err := ParseMCPURI("sse://mcp.example.com/sse?name=code-tools")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "code-tools" {
		t.Errorf("name = %q, want %q", s.Name, "code-tools")
	}
	if s.Type != "sse" {
		t.Errorf("type = %q, want %q", s.Type, "sse")
	}
	if s.URL != "https://mcp.example.com/sse" {
		t.Errorf("url = %q, want %q", s.URL, "https://mcp.example.com/sse")
	}
}

func TestParseMCPURI_Stdio(t *testing.T) {
	s, err := ParseMCPURI("stdio:///npx?args=-y&args=@modelcontextprotocol/server-memory&name=memory")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "memory" {
		t.Errorf("name = %q, want %q", s.Name, "memory")
	}
	if s.Type != "stdio" {
		t.Errorf("type = %q, want %q", s.Type, "stdio")
	}
	if s.Command != "npx" {
		t.Errorf("command = %q, want %q", s.Command, "npx")
	}
	if len(s.Args) != 2 || s.Args[0] != "-y" || s.Args[1] != "@modelcontextprotocol/server-memory" {
		t.Errorf("args = %v, want [-y @modelcontextprotocol/server-memory]", s.Args)
	}
}

func TestParseMCPURI_WithHeaders(t *testing.T) {
	s, err := ParseMCPURI("https://mcp.example.com/api?name=auth-server&header_Authorization=Bearer+tok123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Headers["Authorization"] != "Bearer tok123" {
		t.Errorf("headers = %v, want Authorization=Bearer tok123", s.Headers)
	}
}

func TestParseMCPURI_Invalid(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"missing name", "https://mcp.example.com/tools"},
		{"unsupported scheme", "ftp://mcp.example.com?name=x"},
		{"stdio no command", "stdio:///?name=x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseMCPURI(tc.uri)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestExtractMCPServers_MixedArray(t *testing.T) {
	props := map[string]any{
		"mcp_servers": []any{
			"https://mcp.example.com/tools?name=weather&timeout=10",
			map[string]any{
				"name":    "local-db",
				"type":    "stdio",
				"command": "mcp-server-sqlite",
				"args":    []any{"--db", "test.db"},
			},
		},
	}

	servers, err := extractMCPServers(props)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}

	// First: parsed from URI
	if servers[0].Name != "weather" || servers[0].Type != "http" {
		t.Errorf("server[0] = %+v, want weather/http", servers[0])
	}
	if servers[0].URL != "https://mcp.example.com/tools" {
		t.Errorf("server[0].URL = %q", servers[0].URL)
	}

	// Second: parsed from object
	if servers[1].Name != "local-db" || servers[1].Type != "stdio" {
		t.Errorf("server[1] = %+v, want local-db/stdio", servers[1])
	}
	if servers[1].Command != "mcp-server-sqlite" {
		t.Errorf("server[1].Command = %q", servers[1].Command)
	}
}
