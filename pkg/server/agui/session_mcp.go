package agui

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/saker-ai/saker/pkg/model"
	"github.com/saker-ai/saker/pkg/sandbox"
	"github.com/saker-ai/saker/pkg/tool"
)

// SessionMCPRegistry manages per-session MCP server connections at per-server
// granularity. It implements tool.DynamicToolSource so the runtime can look up
// and execute dynamic tools.
type SessionMCPRegistry struct {
	mu      sync.Mutex
	entries map[string]*mcpEntry // keyed by server name
	closed  bool
	logger  *slog.Logger

	connectTimeout time.Duration // operator-configurable; 0 → default 10s
	maxParallel    int           // max concurrent connections; 0 → default 4
}

// mcpEntry holds a single MCP server's connection and tool registry.
type mcpEntry struct {
	server   ClientMCPServer
	registry *tool.Registry
}

func NewSessionMCPRegistry(logger *slog.Logger) *SessionMCPRegistry {
	return &SessionMCPRegistry{
		entries: make(map[string]*mcpEntry),
		logger:  logger,
	}
}

// SetConnectTimeout sets the per-server connect timeout.
func (s *SessionMCPRegistry) SetConnectTimeout(d time.Duration) {
	s.connectTimeout = d
}

// EnsureServers performs an incremental diff against the current server set:
//   - Unchanged servers (same name + spec) are kept as-is.
//   - Removed servers are disconnected.
//   - Added servers are connected in parallel.
//
// If the server list is identical to what was previously registered, this is a no-op.
func (s *SessionMCPRegistry) EnsureServers(ctx context.Context, servers []ClientMCPServer, _ *sandbox.Manager) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("session MCP registry is closed")
	}

	// Build lookup of desired state.
	desired := make(map[string]ClientMCPServer, len(servers))
	for _, srv := range servers {
		desired[srv.Name] = srv
	}

	// Compute diff.
	var toRemove []string
	var toHealthCheck []string
	for name, entry := range s.entries {
		want, ok := desired[name]
		if !ok {
			toRemove = append(toRemove, name)
		} else if entry.server.Spec() != want.Spec() {
			toRemove = append(toRemove, name)
		} else {
			toHealthCheck = append(toHealthCheck, name)
		}
	}

	var toAdd []ClientMCPServer
	for _, srv := range servers {
		existing, ok := s.entries[srv.Name]
		if !ok {
			toAdd = append(toAdd, srv)
		} else if existing.server.Spec() != srv.Spec() {
			toAdd = append(toAdd, srv)
		}
	}

	// Health-check retained entries: ping to detect dead connections.
	if len(toHealthCheck) > 0 {
		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		for _, name := range toHealthCheck {
			entry := s.entries[name]
			if err := entry.registry.PingMCP(pingCtx); err != nil {
				s.logger.Warn("session MCP: health check failed, will reconnect",
					"name", name, "error", err)
				aguiMCPHealthCheckFailures.Inc()
				toRemove = append(toRemove, name)
				toAdd = append(toAdd, entry.server)
			}
		}
		pingCancel()
	}

	// Short-circuit: nothing changed.
	if len(toRemove) == 0 && len(toAdd) == 0 {
		s.mu.Unlock()
		return nil
	}

	// Remove stale entries (under lock).
	for _, name := range toRemove {
		if entry, ok := s.entries[name]; ok {
			entry.registry.Close()
			delete(s.entries, name)
			s.logger.Info("session MCP: disconnected server", "name", name)
		}
	}
	s.mu.Unlock()

	// Connect new servers in parallel.
	if len(toAdd) > 0 {
		if err := s.connectServers(ctx, toAdd); err != nil {
			return err
		}
	}

	return nil
}

// connectServers connects multiple MCP servers concurrently using errgroup.
func (s *SessionMCPRegistry) connectServers(ctx context.Context, servers []ClientMCPServer) error {
	start := time.Now()
	defer func() {
		aguiMCPConnectDuration.Observe(time.Since(start).Seconds())
	}()

	timeout := s.connectTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	maxPar := s.maxParallel
	if maxPar <= 0 {
		maxPar = 4
	}

	type connResult struct {
		server   ClientMCPServer
		registry *tool.Registry
	}

	var (
		mu      sync.Mutex
		results []connResult
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxPar)

	for _, srv := range servers {
		g.Go(func() error {
			srvTimeout := timeout
			if srv.Timeout > 0 {
				srvTimeout = time.Duration(srv.Timeout * float64(time.Second))
				if srvTimeout > timeout {
					srvTimeout = timeout
				}
			}
			reg, err := s.connectOneServer(gctx, srv, srvTimeout)
			if err != nil {
				return err
			}
			mu.Lock()
			results = append(results, connResult{server: srv, registry: reg})
			mu.Unlock()
			s.logger.Info("session MCP: connected server", "name", srv.Name, "spec", srv.Spec())
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		// Clean up any successfully connected registries on failure.
		mu.Lock()
		for _, r := range results {
			r.registry.Close()
		}
		mu.Unlock()
		return err
	}

	// Register all successful entries.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		for _, r := range results {
			r.registry.Close()
		}
		return fmt.Errorf("session MCP registry closed during connect")
	}
	for _, r := range results {
		s.entries[r.server.Name] = &mcpEntry{
			server:   r.server,
			registry: r.registry,
		}
		aguiMCPActiveConnections.Inc()
	}
	return nil
}

// connectOneServer connects a single MCP server with retry and exponential backoff.
func (s *SessionMCPRegistry) connectOneServer(ctx context.Context, srv ClientMCPServer, timeout time.Duration) (*tool.Registry, error) {
	const maxRetries = 2
	backoff := 250 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(backoff)))
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("connect MCP server %q: %w", srv.Name, ctx.Err())
			case <-time.After(backoff + jitter):
			}
			backoff = min(backoff*2, 5*time.Second)
		}
		reg := tool.NewRegistry()
		opts := tool.MCPServerOptions{
			Headers: srv.Headers,
			Env:     srv.Env,
			Timeout: timeout,
		}
		if err := reg.RegisterMCPServerWithOptions(ctx, srv.Spec(), srv.Name, opts); err != nil {
			reg.Close()
			lastErr = err
			s.logger.Debug("session MCP: connect attempt failed",
				"name", srv.Name, "attempt", attempt+1, "error", err)
			continue
		}
		return reg, nil
	}
	return nil, fmt.Errorf("connect MCP server %q: %w", srv.Name, lastErr)
}

// LookupTool finds a tool by name across all connected MCP server registries.
// It supports both raw names and prefixed names (serverName__toolName) for
// conflict-resolved tools.
func (s *SessionMCPRegistry) LookupTool(name string) (tool.Tool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, false
	}
	// Try direct lookup first.
	for _, entry := range s.entries {
		if t, err := entry.registry.Get(name); err == nil {
			return t, true
		}
	}
	// Try prefixed lookup: strip serverName__ prefix and look in that server.
	if idx := strings.Index(name, "__"); idx > 0 {
		serverName := name[:idx]
		rawName := name[idx+2:]
		if entry, ok := s.entries[serverName]; ok {
			if t, err := entry.registry.Get(rawName); err == nil {
				return t, true
			}
		}
	}
	return nil, false
}

// ListToolDefs returns tool definitions aggregated from all connected MCP servers.
// When multiple servers expose the same tool name, conflicting names are prefixed
// with serverName__ to disambiguate (matching the main registry convention).
func (s *SessionMCPRegistry) ListToolDefs() []model.ToolDefinition {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	// First pass: detect name conflicts across servers.
	nameCount := make(map[string]int)
	for _, entry := range s.entries {
		for _, t := range entry.registry.List() {
			name := strings.TrimSpace(t.Name())
			if name != "" {
				nameCount[name]++
			}
		}
	}

	// Second pass: build definitions, prefixing conflicting names.
	var defs []model.ToolDefinition
	for serverName, entry := range s.entries {
		for _, t := range entry.registry.List() {
			name := strings.TrimSpace(t.Name())
			if name == "" {
				continue
			}
			exportName := name
			if nameCount[name] > 1 {
				exportName = serverName + "__" + name
			}
			defs = append(defs, model.ToolDefinition{
				Name:        exportName,
				Description: strings.TrimSpace(t.Description()),
				Parameters:  schemaToMap(t.Schema()),
			})
		}
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

// MCPInstructions returns server name → instructions for all connected servers
// that provided instructions in their InitializeResult.
func (s *SessionMCPRegistry) MCPInstructions() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	result := make(map[string]string)
	for name, entry := range s.entries {
		instrs := entry.registry.MCPServerInstructions()
		for _, instr := range instrs {
			if instr != "" {
				result[name] = instr
				break
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// PingAll pings all connected MCP servers and returns the names of those that
// failed. Useful for health checking before reuse from cache.
func (s *SessionMCPRegistry) PingAll(ctx context.Context) []string {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	// Snapshot entries under lock.
	type pingTarget struct {
		name     string
		registry *tool.Registry
	}
	targets := make([]pingTarget, 0, len(s.entries))
	for name, entry := range s.entries {
		targets = append(targets, pingTarget{name: name, registry: entry.registry})
	}
	s.mu.Unlock()

	var (
		mu     sync.Mutex
		failed []string
	)
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := t.registry.PingMCP(ctx); err != nil {
				mu.Lock()
				failed = append(failed, t.name)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return failed
}

// ServerNames returns the names of currently connected servers.
func (s *SessionMCPRegistry) ServerNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.entries))
	for name := range s.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Close shuts down all MCP connections managed by this session registry.
func (s *SessionMCPRegistry) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true
	for name, entry := range s.entries {
		entry.registry.Close()
		aguiMCPActiveConnections.Dec()
		s.logger.Debug("session MCP: closed server", "name", name)
	}
	s.entries = nil
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
