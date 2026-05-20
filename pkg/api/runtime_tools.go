package api

import (
	"github.com/saker-ai/saker/pkg/message"
	"github.com/saker-ai/saker/pkg/tool"
	toolbuiltin "github.com/saker-ai/saker/pkg/tool/builtin"
)

type runtimeToolExecutor struct {
	executor  *tool.Executor
	hooks     *runtimeHookAdapter
	history   *message.History
	allow     map[string]struct{}
	root      string
	host      string
	sessionID string
	yolo      bool // skip all whitelist and permission checks

	permissionResolver tool.PermissionResolver

	// dynamicSource provides per-request tools (e.g. AG-UI client MCP servers)
	// that are tried when the primary registry does not contain the tool.
	dynamicSource tool.DynamicToolSource

	// tracer optionally emits a span around each tool dispatch. nil tracer
	// is the common case (no OTLP wired up) and skips span allocation.
	tracer Tracer
}

type registeredToolRefs struct {
	taskTool            *toolbuiltin.TaskTool
	spawnAgentTool      *toolbuiltin.SpawnAgentTool
	sendInputTool       *toolbuiltin.SendInputTool
	waitAgentTool       *toolbuiltin.WaitAgentTool
	closeAgentTool      *toolbuiltin.CloseAgentTool
	spawnAgentsBatchTool *toolbuiltin.SpawnAgentsBatchTool
	streamMonitor       *toolbuiltin.StreamMonitorTool
}
