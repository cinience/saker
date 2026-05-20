package agui

import (
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
	if err := validateMCPAllowList(servers, nil); err != nil {
		t.Fatalf("empty allow-list should permit anything: %v", err)
	}
}

func TestValidateMCPAllowList_ExactMatch(t *testing.T) {
	servers := []ClientMCPServer{{Name: "my-server", Type: "http", URL: "https://mcp.example.com/sse"}}
	allowList := []string{"https://mcp.example.com/sse"}
	if err := validateMCPAllowList(servers, allowList); err != nil {
		t.Fatalf("exact URL match should pass: %v", err)
	}
}

func TestValidateMCPAllowList_NameMatch(t *testing.T) {
	servers := []ClientMCPServer{{Name: "my-server", Type: "http", URL: "https://mcp.example.com/sse"}}
	allowList := []string{"my-server"}
	if err := validateMCPAllowList(servers, allowList); err != nil {
		t.Fatalf("name match should pass: %v", err)
	}
}

func TestValidateMCPAllowList_PrefixMatch(t *testing.T) {
	servers := []ClientMCPServer{{Name: "s", Type: "http", URL: "https://mcp.example.com/v2/sse"}}
	allowList := []string{"https://mcp.example.com/*"}
	if err := validateMCPAllowList(servers, allowList); err != nil {
		t.Fatalf("prefix match should pass: %v", err)
	}
}

func TestValidateMCPAllowList_Denied(t *testing.T) {
	servers := []ClientMCPServer{{Name: "evil", Type: "http", URL: "https://evil.com/mcp"}}
	allowList := []string{"https://mcp.example.com/*"}
	if err := validateMCPAllowList(servers, allowList); err == nil {
		t.Fatal("should deny server not in allow-list")
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
