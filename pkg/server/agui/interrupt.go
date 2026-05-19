package agui

// AG-UI Protocol Interrupt types per https://docs.ag-ui.com/concepts/interrupts
//
// The interrupt-aware run lifecycle allows agents to pause execution for
// human input. The run ends with an interrupt outcome, and the client starts
// a new run carrying per-interrupt responses via the resume array.
//
// NOTE: The current AG-UI Go SDK (v0.0.0-20260514) does not include native
// Interrupt/Resume types in RunAgentInput. These are defined here to prepare
// for future SDK upgrades and to document the protocol contract.

// Interrupt represents a pause point where the agent needs human input.
type Interrupt struct {
	ID             string         `json:"id"`
	Reason         string         `json:"reason"`
	Message        string         `json:"message,omitempty"`
	ToolCallID     string         `json:"toolCallId,omitempty"`
	ResponseSchema any            `json:"responseSchema,omitempty"`
	ExpiresAt      string         `json:"expiresAt,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// ResumeItem is the client's response to a single interrupt.
type ResumeItem struct {
	InterruptID string `json:"interruptId"`
	Status      string `json:"status"` // "resolved" or "cancelled"
	Payload     any    `json:"payload,omitempty"`
}

// RunOutcome represents the outcome of a finished run.
// The outcome discriminator is either "success" or "interrupt".
type RunOutcome struct {
	Type       string      `json:"type"`                 // "success" or "interrupt"
	Interrupts []Interrupt `json:"interrupts,omitempty"` // non-empty when type == "interrupt"
}

// InterruptReasons defines the core reason taxonomy per AG-UI spec.
const (
	InterruptReasonToolCall      = "tool_call"
	InterruptReasonInputRequired = "input_required"
	InterruptReasonConfirmation  = "confirmation"
)
