package toolbuiltin

import (
	"context"
	"time"
)

// AgentRunner abstracts the subagent lifecycle for the spawn/send/wait/close tools.
// The Runtime implements this interface and injects it at startup.
type AgentRunner interface {
	SpawnAgent(ctx context.Context, req SpawnAgentRequest) (*SpawnAgentResult, error)
	SendInput(ctx context.Context, agentID string, message string) error
	WaitAgent(ctx context.Context, agentID string, timeout time.Duration) (*WaitAgentResult, error)
	CloseAgent(ctx context.Context, agentID string) (*CloseAgentResult, error)
	ListAgents(ctx context.Context, parentSession string) ([]AgentInfo, error)
}

type SpawnAgentRequest struct {
	Prompt       string
	SubagentType string
	Model        string
	ForkContext  bool
	ForkTurns    int
	Background   bool
}

type SpawnAgentResult struct {
	AgentID  string
	Nickname string
}

type WaitAgentResult struct {
	AgentID  string
	Output   string
	Status   string
	TimedOut bool
	Profile  string        // subagent type (e.g. "general-purpose", "explore")
	Model    string        // model tier requested
	Elapsed  time.Duration // wall-clock from start to finish
}

type CloseAgentResult struct {
	AgentID string
	Output  string
	Status  string
}

type AgentInfo struct {
	ID       string
	Nickname string
	Profile  string
	Status   string
}
