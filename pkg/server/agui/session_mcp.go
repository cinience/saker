package agui

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/saker-ai/saker/pkg/model"
	"github.com/saker-ai/saker/pkg/sandbox"
	"github.com/saker-ai/saker/pkg/tool"
)

// sessionMCPRegistry manages per-session MCP server connections. It implements
// tool.DynamicToolSource so the runtime can look up and execute dynamic tools.
type sessionMCPRegistry struct {
	mu       sync.Mutex
	registry *tool.Registry
	servers  []ClientMCPServer
	closed   bool
	logger   *slog.Logger
}

func newSessionMCPRegistry(logger *slog.Logger) *sessionMCPRegistry {
	return &sessionMCPRegistry{
		registry: tool.NewRegistry(),
		logger:   logger,
	}
}

// EnsureServers connects to MCP servers described by the client. It performs an
// incremental diff: new servers are connected, removed servers are disconnected.
// If the server list is identical to what was previously registered, this is a no-op.
func (s *sessionMCPRegistry) EnsureServers(ctx context.Context, servers []ClientMCPServer, sb *sandbox.Manager) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session MCP registry is closed")
	}

	if serversEqual(s.servers, servers) {
		return nil
	}

	// Rebuild: close old registry and create a fresh one.
	// Full rebuild is simpler and safer than incremental diff for MCP sessions
	// because MCP connections are stateful and we can't partially reconfigure.
	s.registry.Close()
	s.registry = tool.NewRegistry()

	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for _, srv := range servers {
		opts := tool.MCPServerOptions{
			Headers: srv.Headers,
			Env:     srv.Env,
		}
		if err := s.registry.RegisterMCPServerWithOptions(connectCtx, srv.Spec(), srv.Name, opts); err != nil {
			s.logger.Warn("session MCP: failed to connect server",
				"name", srv.Name, "spec", srv.Spec(), "error", err)
			return fmt.Errorf("connect MCP server %q: %w", srv.Name, err)
		}
		s.logger.Info("session MCP: connected server", "name", srv.Name, "spec", srv.Spec())
	}

	s.servers = append([]ClientMCPServer(nil), servers...)
	return nil
}

// LookupTool finds a tool by name in the dynamic MCP registry.
func (s *sessionMCPRegistry) LookupTool(name string) (tool.Tool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.registry == nil {
		return nil, false
	}
	t, err := s.registry.Get(name)
	if err != nil {
		return nil, false
	}
	return t, true
}

// ListToolDefs returns tool definitions for all dynamically registered MCP tools.
func (s *sessionMCPRegistry) ListToolDefs() []model.ToolDefinition {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.registry == nil {
		return nil
	}

	tools := s.registry.List()
	defs := make([]model.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		name := strings.TrimSpace(t.Name())
		if name == "" {
			continue
		}
		defs = append(defs, model.ToolDefinition{
			Name:        name,
			Description: strings.TrimSpace(t.Description()),
			Parameters:  schemaToMap(t.Schema()),
		})
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

// Close shuts down all MCP connections managed by this session registry.
func (s *sessionMCPRegistry) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true
	if s.registry != nil {
		s.registry.Close()
	}
}

func serversEqual(a, b []ClientMCPServer) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Spec() != b[i].Spec() {
			return false
		}
	}
	return true
}

// schemaToMap converts a tool JSONSchema to map[string]any for ToolDefinition.
func schemaToMap(schema *tool.JSONSchema) map[string]any {
	if schema == nil {
		return nil
	}
	payload := map[string]any{}
	if schema.Type != "" {
		payload["type"] = schema.Type
	}
	if len(schema.Properties) > 0 {
		payload["properties"] = schema.Properties
	}
	if len(schema.Required) > 0 {
		payload["required"] = append([]string(nil), schema.Required...)
	}
	return payload
}
