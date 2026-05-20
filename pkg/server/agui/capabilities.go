package agui

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AG-UI Protocol Capabilities types per https://docs.ag-ui.com/concepts/capabilities

type IdentityCapabilities struct {
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Provider    string `json:"provider,omitempty"`
}

type TransportCapabilities struct {
	Streaming         bool `json:"streaming,omitempty"`
	Websocket         bool `json:"websocket,omitempty"`
	HTTPBinary        bool `json:"httpBinary,omitempty"`
	PushNotifications bool `json:"pushNotifications,omitempty"`
	Resumable         bool `json:"resumable,omitempty"`
}

type ToolsCapabilities struct {
	Supported      bool `json:"supported,omitempty"`
	ParallelCalls  bool `json:"parallelCalls,omitempty"`
	ClientProvided bool `json:"clientProvided,omitempty"`
}

type OutputCapabilities struct {
	StructuredOutput   bool     `json:"structuredOutput,omitempty"`
	SupportedMimeTypes []string `json:"supportedMimeTypes,omitempty"`
}

type StateCapabilities struct {
	Snapshots       bool `json:"snapshots,omitempty"`
	Deltas          bool `json:"deltas,omitempty"`
	Memory          bool `json:"memory,omitempty"`
	PersistentState bool `json:"persistentState,omitempty"`
}

type MultiAgentCapabilities struct {
	Supported  bool `json:"supported,omitempty"`
	Delegation bool `json:"delegation,omitempty"`
	Handoffs   bool `json:"handoffs,omitempty"`
}

type ReasoningCapabilities struct {
	Supported bool `json:"supported,omitempty"`
	Streaming bool `json:"streaming,omitempty"`
	Encrypted bool `json:"encrypted,omitempty"`
}

type MultimodalInputCapabilities struct {
	Image bool `json:"image,omitempty"`
	Audio bool `json:"audio,omitempty"`
	Video bool `json:"video,omitempty"`
	PDF   bool `json:"pdf,omitempty"`
	File  bool `json:"file,omitempty"`
}

type MultimodalOutputCapabilities struct {
	Image bool `json:"image,omitempty"`
	Audio bool `json:"audio,omitempty"`
}

type MultimodalCapabilities struct {
	Input  *MultimodalInputCapabilities  `json:"input,omitempty"`
	Output *MultimodalOutputCapabilities `json:"output,omitempty"`
}

type ExecutionCapabilities struct {
	CodeExecution    bool `json:"codeExecution,omitempty"`
	Sandboxed        bool `json:"sandboxed,omitempty"`
	MaxIterations    int  `json:"maxIterations,omitempty"`
	MaxExecutionTime int  `json:"maxExecutionTime,omitempty"`
}

type HumanInTheLoopCapabilities struct {
	Supported     bool `json:"supported,omitempty"`
	Approvals     bool `json:"approvals,omitempty"`
	Interventions bool `json:"interventions,omitempty"`
	Feedback      bool `json:"feedback,omitempty"`
	Interrupts    bool `json:"interrupts,omitempty"`
}

type AgentCapabilities struct {
	Identity       *IdentityCapabilities       `json:"identity,omitempty"`
	Transport      *TransportCapabilities      `json:"transport,omitempty"`
	Tools          *ToolsCapabilities          `json:"tools,omitempty"`
	Output         *OutputCapabilities         `json:"output,omitempty"`
	State          *StateCapabilities          `json:"state,omitempty"`
	MultiAgent     *MultiAgentCapabilities     `json:"multiAgent,omitempty"`
	Reasoning      *ReasoningCapabilities      `json:"reasoning,omitempty"`
	Multimodal     *MultimodalCapabilities     `json:"multimodal,omitempty"`
	Execution      *ExecutionCapabilities      `json:"execution,omitempty"`
	HumanInTheLoop *HumanInTheLoopCapabilities `json:"humanInTheLoop,omitempty"`
	Custom         map[string]any              `json:"custom,omitempty"`
}

// handleCapabilities implements GET/POST /v1/agents/run/capabilities —
// AG-UI capability discovery per protocol spec.
func (g *Gateway) handleCapabilities(c *gin.Context) {
	caps := &AgentCapabilities{
		Identity: &IdentityCapabilities{
			Name:        "saker",
			Type:        "saker",
			Description: "Saker AI Assistant — multi-model agent with tool execution",
			Version:     "1.0.0",
			Provider:    "saker-ai",
		},
		Transport: &TransportCapabilities{
			Streaming: true,
			Resumable: true,
		},
		Tools: &ToolsCapabilities{
			Supported:      true,
			ParallelCalls:  true,
			ClientProvided: false,
		},
		Output: &OutputCapabilities{
			StructuredOutput:   true,
			SupportedMimeTypes: []string{"text/plain", "application/json"},
		},
		State: &StateCapabilities{
			Snapshots:       true,
			Deltas:          true,
			Memory:          true,
			PersistentState: true,
		},
		MultiAgent: &MultiAgentCapabilities{
			Supported:  true,
			Delegation: true,
		},
		Reasoning: &ReasoningCapabilities{
			Supported: true,
			Streaming: true,
		},
		Multimodal: &MultimodalCapabilities{
			Input: &MultimodalInputCapabilities{
				Image: true,
				PDF:   true,
				File:  true,
			},
			Output: &MultimodalOutputCapabilities{
				Image: true,
			},
		},
		Execution: &ExecutionCapabilities{
			CodeExecution: true,
			Sandboxed:     true,
			MaxIterations: 25,
		},
		HumanInTheLoop: &HumanInTheLoopCapabilities{
			Supported:  true,
			Approvals:  true,
			Interrupts: true,
		},
	}
	c.JSON(http.StatusOK, caps)
}
