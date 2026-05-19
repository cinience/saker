package agui

import (
	"context"
	"errors"
	"log/slog"
	"sync"

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
}

// Options holds operator-configurable settings for the AG-UI gateway.
type Options struct {
	Enabled       bool
	DevBypassAuth bool
}

// Gateway carries the runtime dependencies for AG-UI HTTP handlers.
type Gateway struct {
	deps          Deps
	hitl          *hitlRegistry
	mu            sync.Mutex
	activeCancels map[string]context.CancelFunc
	shuttingDown  bool
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

	g := &Gateway{
		deps:          deps,
		hitl:          newHITLRegistry(),
		activeCancels: make(map[string]context.CancelFunc),
	}

	agents := engine.Group("/v1/agents")
	agents.Use(g.authMiddleware())
	{
		agents.POST("/run", g.handleRun)
		agents.POST("/run/agent/:agentId/run", g.handleRun)
		agents.POST("/run/agent/:agentId/connect", g.handleConnectRoute)
		agents.POST("/run/agent/:agentId/stop/:threadId", g.handleStop)
		agents.GET("/run/info", g.handleInfo)
		agents.POST("/run/info", g.handleInfo)
		agents.GET("/run/threads", g.handleThreads)
		agents.PATCH("/run/threads/:threadId", g.handleThreadUpdate)
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
	return ctx, func() {
		cancel()
		g.mu.Lock()
		delete(g.activeCancels, runID)
		g.mu.Unlock()
	}, true
}

// Shutdown cancels all active AG-UI runs so SSE handlers can return before
// http.Server.Shutdown reaches its deadline. It is safe to call repeatedly.
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
	cancels := make([]context.CancelFunc, 0, len(g.activeCancels))
	for _, cancel := range g.activeCancels {
		cancels = append(cancels, cancel)
	}
	g.activeCancels = make(map[string]context.CancelFunc)
	g.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	g.deps.Logger.Info("agui gateway shutdown", "active_runs_cancelled", len(cancels))
}
