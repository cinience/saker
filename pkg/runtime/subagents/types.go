package subagents

import (
	"context"
	"errors"
	"maps"
	"strings"
	"time"

	"github.com/saker-ai/saker/pkg/runtime/skills"
)

var (
	ErrUnknownInstance = errors.New("subagents: unknown instance")
	ErrInstanceExists  = errors.New("subagents: instance already exists")
	ErrExecutorClosed  = errors.New("subagents: executor runner is not configured")
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Instance struct {
	ID              string
	Profile         string
	ParentSessionID string
	SessionID       string
	Status          Status
	CreatedAt       time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
	Result          *Result
	Error           string
	Metadata        map[string]any
	Background      bool               // true if spawned in background mode
	cancelFunc      context.CancelFunc // for abort propagation (unexported to avoid cloning issues)
	mailbox         chan string        // inter-agent message channel (cap=10)
}

// Mailbox returns the receive end of the instance's message channel.
// Returns nil if the instance has no mailbox (e.g. retrieved from store clone).
func (i *Instance) Mailbox() <-chan string {
	if i.mailbox == nil {
		return nil
	}
	return i.mailbox
}

// SendMessage delivers a message to the running instance's mailbox.
// Returns ErrMailboxFull if the channel is at capacity; never blocks.
func (i *Instance) SendMessage(msg string) error {
	if i.mailbox == nil {
		return errors.New("subagents: instance has no mailbox")
	}
	select {
	case i.mailbox <- msg:
		return nil
	default:
		return ErrMailboxFull
	}
}

var ErrMailboxFull = errors.New("subagents: mailbox is full")

func (i Instance) clone() Instance {
	if len(i.Metadata) > 0 {
		i.Metadata = maps.Clone(i.Metadata)
	}
	if i.Result != nil {
		res := i.Result.clone()
		i.Result = &res
	}
	if i.StartedAt != nil {
		started := *i.StartedAt
		i.StartedAt = &started
	}
	if i.FinishedAt != nil {
		finished := *i.FinishedAt
		i.FinishedAt = &finished
	}
	// mailbox and cancelFunc are shared references — intentionally NOT copied.
	return i
}

type SpawnRequest struct {
	Target        string
	Instruction   string
	Activation    skills.ActivationContext
	ToolWhitelist []string
	Metadata      map[string]any
	ParentContext Context
	Background    bool // when true, Spawn returns immediately without blocking the caller
}

type SpawnHandle struct {
	ID string
}

type WaitRequest struct {
	ID      string
	Timeout time.Duration
}

type WaitResult struct {
	Instance Instance
	TimedOut bool
}

type RunRequest struct {
	InstanceID    string
	Target        string
	Instruction   string
	Activation    skills.ActivationContext
	ToolWhitelist []string
	Metadata      map[string]any
	ParentContext Context
	ResumeFrom    string // transcript/agent ID to resume from
}

type Runner interface {
	RunSubagent(context.Context, RunRequest) (Result, error)
}

func childSessionID(parent Context, instanceID string) string {
	parentID := strings.TrimSpace(parent.SessionID)
	if parentID == "" {
		return instanceID
	}
	return parentID + ":" + instanceID
}
