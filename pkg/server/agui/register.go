package agui

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/conversation"
	"github.com/saker-ai/saker/pkg/project"
	"github.com/saker-ai/saker/pkg/server"
)

// Runner is the narrow stream-execution interface the gateway needs.
// *api.Runtime satisfies it directly.
type Runner interface {
	RunStream(ctx context.Context, req api.Request) (<-chan api.StreamEvent, error)
}

// MediaCacher downloads remote media URLs and returns artifacts pointing to
// locally cached copies. *server.Handler satisfies this interface via its
// public CacheArtifactMedia method.
type MediaCacher interface {
	CacheArtifactMedia(ctx context.Context, a server.Artifact) server.Artifact
}

// Deps bundles the runtime dependencies for the AG-UI gateway.
type Deps struct {
	Runtime           Runner
	ProjectStore      *project.Store
	ConversationStore *conversation.Store
	Logger            *slog.Logger
	Options           Options
	SessionValidator  func(c *gin.Context) (username, role string, ok bool)
	MediaCacher       MediaCacher
	// CtxEnricher, when non-nil, is called to inject additional context values
	// (e.g. object-store callbacks) into the runtime context before a run starts.
	CtxEnricher func(context.Context) context.Context
}

// Options holds operator-configurable settings for the AG-UI gateway.
type Options struct {
	Enabled          bool
	DevBypassAuth    bool
	RPS              float64       // Rate limit requests per second per identity (default 10)
	Burst            int           // Rate limit burst size (default 20)
	DrainTimeout     time.Duration // Graceful shutdown drain period (default 3s)
	MaxActiveStreams  int           // Load shedding: max concurrent runs (default 100, 0 = unlimited)
	TurnTimeout      time.Duration // Max run duration (default: server.DefaultTurnTimeout)
	DetachTimeout    time.Duration // How long a detached session survives before cancellation (default 30s)
	AllowedMCPPatterns []string    // Optional: restrict which MCP server URLs clients may connect to

	// MCP security settings
	MaxMCPServersPerSession int           // Max MCP servers per session (default 5, 0 = unlimited)
	AllowMCPStdio           bool          // Whether stdio-type MCP servers are permitted (default false)
	MCPConnectTimeout       time.Duration // Timeout for connecting to MCP servers (default 10s)

	// Client override security switches. When true, the corresponding client
	// capability is blocked. All default to false (= allowed).
	DenyModelEndpoint       bool // Block client from specifying custom LLM endpoint via model_uri
	DenySystemPromptReplace bool // Block client from using "replace" mode for system prompt
	DenyToolOverride        bool // Block client from controlling tools via allowed_tools/passthrough_tools
}

// Gateway carries the runtime dependencies for AG-UI HTTP handlers.
type Gateway struct {
	deps          Deps
	hitl          *hitlRegistry
	mu            sync.Mutex
	activeCancels map[string]context.CancelFunc
	activeWg      sync.WaitGroup
	// threadRuns maps threadID → runID for concurrent run mutual exclusion.
	threadRuns         map[string]string
	shuttingDown       bool
	rateLimiterCleanup func()
	// artifactCache stores per-thread artifacts so connect can replay them.
	artifactCache artifactCache
	// sessions manages active run sessions for SSE reconnect support.
	sessions *runSessionManager
	// mcpCache caches per-thread MCP registries across turns.
	mcpCache *threadMCPCache
}

// RegisterAGUIGateway mounts the AG-UI protocol endpoints on the supplied
// Gin engine and returns the Gateway handle.
//
// Returns (nil, nil) when Options.Enabled is false.
func RegisterAGUIGateway(engine *gin.Engine, deps Deps) (*Gateway, error) {
	if !deps.Options.Enabled {
		return nil, nil
	}
	if engine == nil {
		return nil, errors.New("agui-gw: gin engine is nil")
	}
	if deps.Runtime == nil {
		return nil, errors.New("agui-gw: runtime is required")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	rps := deps.Options.RPS
	if rps == 0 {
		rps = 10
	}
	burst := deps.Options.Burst
	if burst == 0 {
		burst = 20
	}
	rateLimiter, rateLimiterCleanup := newAGUIRateLimiter(rps, burst, deps.Logger)

	g := &Gateway{
		deps:               deps,
		hitl:               newHITLRegistry(),
		activeCancels:      make(map[string]context.CancelFunc),
		threadRuns:         make(map[string]string),
		artifactCache:      newArtifactCache(),
		rateLimiterCleanup: rateLimiterCleanup,
		sessions:           newRunSessionManager(),
		mcpCache:           newThreadMCPCache(deps.Logger),
	}

	agents := engine.Group("/v1/agents")
	agents.Use(g.authMiddleware())
	agents.Use(rateLimiter)
	{
		agents.POST("/run", g.handleRun)
		agents.POST("/run/agent/:agentId/run", g.handleRun)
		agents.POST("/run/agent/:agentId/connect", g.handleConnectRoute)
		agents.POST("/run/agent/:agentId/stop/:threadId", g.handleStop)
		agents.GET("/run/info", g.handleInfo)
		agents.POST("/run/info", g.handleInfo)
		agents.GET("/run/capabilities", g.handleCapabilities)
		agents.POST("/run/capabilities", g.handleCapabilities)
		agents.GET("/run/threads", g.handleThreads)
		agents.PATCH("/run/threads/:threadId", g.handleThreadUpdate)
		agents.POST("/run/threads/:threadId/archive", g.handleThreadArchive)
		agents.DELETE("/run/threads/:threadId", g.handleThreadDelete)
		agents.POST("/run/:runId/approval", g.handleApprovalRespond)
		agents.POST("/run/:runId/answer", g.handleQuestionRespond)
	}

	deps.Logger.Info("agui gateway mounted",
		"dev_bypass", deps.Options.DevBypassAuth,
	)

	return g, nil
}

func (g *Gateway) runContext(parent context.Context, runID string) (context.Context, context.CancelFunc, bool) {
	ctx, cancel := context.WithCancel(parent)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.shuttingDown {
		cancel()
		return ctx, func() {}, false
	}
	g.activeCancels[runID] = cancel
	g.activeWg.Add(1)
	return ctx, func() {
		cancel()
		g.mu.Lock()
		delete(g.activeCancels, runID)
		g.mu.Unlock()
		g.activeWg.Done()
	}, true
}

// Shutdown gracefully drains active AG-UI runs before force-cancelling.
// Phase 1: reject new runs, wait for active runs to finish naturally.
// Phase 2: after drain timeout, force cancel remaining runs.
// It is safe to call repeatedly.
func (g *Gateway) Shutdown() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if g.shuttingDown {
		g.mu.Unlock()
		return
	}
	g.shuttingDown = true
	activeCount := len(g.activeCancels)
	g.mu.Unlock()

	// Cancel all run sessions (detached ones included).
	g.sessions.cancelAll()

	if activeCount == 0 {
		g.cleanup()
		return
	}

	drain := g.deps.Options.DrainTimeout
	if drain == 0 {
		drain = 3 * time.Second
	}

	done := make(chan struct{})
	go func() {
		g.activeWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		g.deps.Logger.Info("agui gateway drained gracefully", "active_runs", activeCount)
	case <-time.After(drain):
		g.mu.Lock()
		cancels := make([]context.CancelFunc, 0, len(g.activeCancels))
		for _, cancel := range g.activeCancels {
			cancels = append(cancels, cancel)
		}
		g.activeCancels = make(map[string]context.CancelFunc)
		g.mu.Unlock()
		for _, cancel := range cancels {
			cancel()
		}
		g.deps.Logger.Info("agui gateway force shutdown", "cancelled_runs", len(cancels))
	}

	g.cleanup()
}

func (g *Gateway) cleanup() {
	if g.rateLimiterCleanup != nil {
		g.rateLimiterCleanup()
	}
	if g.mcpCache != nil {
		g.mcpCache.closeAll()
	}
}
