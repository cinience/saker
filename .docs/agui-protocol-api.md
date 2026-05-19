# AG-UI Protocol Implementation

Saker's AG-UI gateway (`pkg/server/agui/`) implements the [AG-UI Protocol](https://docs.ag-ui.com) — an event-driven streaming standard for agent-user interaction over Server-Sent Events (SSE).

## Compliance Requirement

The AG-UI implementation **MUST** remain compliant with the official specification at https://docs.ag-ui.com. Any deviation from the protocol is a bug. Protocol compliance is enforced by the test suite.

## SDK

Uses the community Go SDK:

```
github.com/ag-ui-protocol/ag-ui/sdks/community/go v0.0.0-20260514010037-6a4efc6202fe
```

## Endpoints

| Method | Path | Description |
|---|---|---|
| POST | `/v1/agents/run` | CopilotKit v2 envelope transport (multiplexes methods) |
| GET | `/v1/agents/info` | Agent metadata |
| POST | `/v1/agents/run/agent/run` | Start streaming run |
| POST | `/v1/agents/run/agent/connect` | Connect to thread (history replay) |
| GET/POST | `/v1/agents/run/capabilities` | Capabilities discovery |
| GET | `/v1/agents/run/threads` | List threads |
| POST | `/v1/agents/run/threads` | Create thread |
| PATCH | `/v1/agents/run/threads/:threadId` | Update thread |
| DELETE | `/v1/agents/run/threads/:threadId` | Delete thread |
| POST | `/v1/agents/run/threads/:threadId/archive` | Archive thread |
| POST | `/v1/agents/run/stop` | Stop active run |

## Event Types

### Lifecycle Events

| Event | When emitted |
|---|---|
| `RUN_STARTED` | Start of every run (streaming and connect) |
| `RUN_FINISHED` | End of every run |
| `RUN_ERROR` | Unrecoverable error during run |

### Text Message Events

| Event | When emitted |
|---|---|
| `TEXT_MESSAGE_START` | First text content delta in a run |
| `TEXT_MESSAGE_CONTENT` | Each text content delta |
| `TEXT_MESSAGE_END` | Finalization (after all text emitted) |

Text deltas pass through a `textFilter` that strips XML function-call artifacts from streaming output.

### Reasoning Events

| Event | When emitted |
|---|---|
| `REASONING_START` | First reasoning delta (outer wrapper) |
| `REASONING_MESSAGE_START` | Start of reasoning message |
| `REASONING_MESSAGE_CONTENT` | Each reasoning text delta |
| `REASONING_MESSAGE_END` | End of reasoning message |
| `REASONING_END` | End of reasoning block (outer wrapper) |

Reasoning events are emitted before text content. When text content starts, the reasoning phase is auto-closed.

### Tool Call Events

| Event | When emitted |
|---|---|
| `TOOL_CALL_START` | Tool execution begins |
| `TOOL_CALL_ARGS` | Tool arguments (JSON string) |
| `TOOL_CALL_END` | Tool execution finishes |
| `TOOL_CALL_RESULT` | Formatted tool result content |

Lifecycle guarantees:
- A new TOOL_CALL_START auto-closes any open tool call (emits TOOL_CALL_END first).
- `finalize()` closes any dangling open tool call.
- Tool call IDs are tracked in `streamState.toolCalls` for dedup and ordering.

### State Management Events

| Event | When emitted |
|---|---|
| `STATE_SNAPSHOT` | On connect (establishes artifacts schema) |
| `STATE_DELTA` | After tool result with new artifacts (JSON Patch RFC 6902) |

State deltas use JSON Patch operations to append artifacts:
```json
[{"op": "add", "path": "/artifacts/-", "value": {"type": "image", "url": "...", "name": "..."}}]
```

### Activity Events

| Event | When emitted |
|---|---|
| `ACTIVITY_SNAPSHOT` | Tool execution start, iteration start |
| `ACTIVITY_DELTA` | Tool execution complete (status patch) |

Activity types: `TOOL_EXECUTION`, `ITERATION`.

### Step Events

| Event | When emitted |
|---|---|
| `STEP_STARTED` | Each agent iteration start |
| `STEP_FINISHED` | Each agent iteration end |

A new STEP_STARTED auto-closes any open step.

### Custom Events

| Event type string | When emitted |
|---|---|
| `timeline` | EventTimeline received from runtime |
| `skill_activation` | EventSkillActivation received from runtime |

### Messages Snapshot

| Event | When emitted |
|---|---|
| `MESSAGES_SNAPSHOT` | On connect — replays thread history |

History filtering rules (`shouldSkipHistoryMessage`):
- Role `"tool"` messages are excluded
- Assistant messages with empty content + tool calls only are excluded
- Duplicate consecutive messages (same role, content, toolCallID) are deduped
- System injected messages (e.g. `[System] You asked questions...`) are excluded

## Capabilities Discovery

`GET /v1/agents/run/capabilities` returns the agent's declared capabilities:

```json
{
  "identity": {"name": "saker", "type": "saker", "description": "..."},
  "transport": {"streaming": true},
  "tools": {"supported": true, "parallelCalls": true},
  "output": {"streaming": true, "supportedFormats": ["text", "markdown"]},
  "state": {"supported": true, "format": "json_patch"},
  "reasoning": {"supported": true},
  "multimodal": {"supported": true, "inputFormats": ["image/png", "image/jpeg", "image/gif", "image/webp", "application/pdf"]},
  "execution": {"interruptible": true, "resumable": true},
  "humanInTheLoop": {"supported": true, "methods": ["tool_call"]}
}
```

## Human-in-the-Loop (HITL)

Saker uses the AG-UI protocol's tool-call-based HITL mechanism:

1. The `ask_user_question` tool is suppressed from the raw tool lifecycle events
2. Instead, it triggers a side-channel AG-UI action event for the frontend
3. The tool result carries the user's response back to the agent

The `suppressedToolCalls` map in `streamState` tracks these IDs to avoid emitting TOOL_CALL_START/END for HITL interactions.

## Interrupt Protocol

Types defined in `interrupt.go` prepare for the AG-UI interrupt-aware run lifecycle:

- `Interrupt` — pause point requiring human input
- `ResumeItem` — client response to an interrupt
- `RunOutcome` — discriminator ("success" or "interrupt")

Interrupt reasons: `tool_call`, `input_required`, `confirmation`.

> Note: The current AG-UI Go SDK does not include native Interrupt/Resume types in RunAgentInput. Types are defined locally for forward compatibility.

## Multimodal Input

AG-UI `InputContent` (binary type) is converted to saker `model.ContentBlock`:

| MIME prefix | Block type |
|---|---|
| `image/*` | `ContentBlockImage` |
| `application/pdf` | `ContentBlockDocument` |
| `application/*` | `ContentBlockDocument` |

The SDK's `msg.ContentInputContents()` helper handles type-safe extraction of multimodal arrays.

## Artifact Extraction

Tool results are inspected for media artifacts via `server.ExtractArtifacts()`:

1. Structured metadata path: `output.metadata.structured.{media_type, media_url}`
2. URL regex detection in string output
3. Deduplication via `seenArtifactURLs` within a stream

Artifacts are emitted as STATE_DELTA events (JSON Patch append to `/artifacts/-`).

## Stream Lifecycle

```
RUN_STARTED
├── STEP_STARTED (per iteration)
│   ├── ACTIVITY_SNAPSHOT (iteration)
│   ├── REASONING_START → REASONING_MESSAGE_START → CONTENT* → MESSAGE_END → REASONING_END
│   ├── TEXT_MESSAGE_START → CONTENT* → TEXT_MESSAGE_END
│   ├── ACTIVITY_SNAPSHOT (tool) → TOOL_CALL_START → ARGS → END → RESULT
│   │   └── ACTIVITY_DELTA (tool status)
│   │   └── STATE_DELTA (artifacts)
│   └── ...
├── STEP_FINISHED
└── RUN_FINISHED
```

`finalize()` ensures all open resources are closed in order:
1. Close open tool call (TOOL_CALL_END)
2. Close reasoning phase if text never started (REASONING_MESSAGE_END + REASONING_END)
3. Flush text filter tail (TEXT_MESSAGE_CONTENT)
4. Close text message (TEXT_MESSAGE_END)
5. Close open step (STEP_FINISHED)
6. Emit RUN_FINISHED

## CopilotKit v2 Envelope Transport

The `/v1/agents/run` endpoint accepts a JSON envelope:

```json
{"method": "<method>", "body": { ... }}
```

Supported methods: `agent/run`, `agent/connect`, `info`, `threads`, `capabilities`.

## Thread Management

| Operation | Endpoint | Notes |
|---|---|---|
| List | GET `/threads?source=agui` | Filtered by source |
| Create | POST `/threads` | Returns formatted thread |
| Update | PATCH `/threads/:threadId` | Accepts `name` or `title` field |
| Delete | DELETE `/threads/:threadId` | Soft delete |
| Archive | POST `/threads/:threadId/archive` | Treated as soft delete |

## Client State Forwarding

`RunAgentInput.State` is forwarded to `req.Metadata["_agui_state"]`. This enables agents to access frontend shared state (e.g. CopilotKit's `useCoAgent` state) during execution.

## Frontend Tools

`RunAgentInput.Tools` (frontend-defined actions from CopilotKit) are converted to `model.ToolDefinition` and injected as `req.ExtraTools`. These are sent to the LLM for awareness but NOT executed by saker's tool executor — the model's tool calls for these are passed through to the client.

## ForwardedProps

`RunAgentInput.ForwardedProps` is injected into `req.Metadata["_agui_forwarded_props"]` for downstream access by tools and middleware.

## Run Outcome

`RUN_FINISHED` events include a `result` field with the run outcome per protocol spec:

```json
{"type": "RUN_FINISHED", "threadId": "...", "runId": "...", "result": {"type": "success"}}
```

## Message ID Stability

Message IDs follow the deterministic format `msg_<threadID>_<runID>`, ensuring:
- Same thread + run always produces the same message ID
- CopilotKit can reliably reference messages for updates
- IDs are stable across reconnects within the same run

## Artifact Persistence

Artifacts extracted from tool results are cached per-thread in memory. On reconnect (`/agent/connect`), the STATE_SNAPSHOT replays all previously generated artifacts:

```json
{"type": "STATE_SNAPSHOT", "snapshot": {"artifacts": [{"type": "image", "url": "...", "name": "..."}]}}
```

## Error Handling

`RUN_ERROR` events include:
- `message`: Human-readable error description
- `code` (optional): Machine-readable error code extracted from structured error output

Context cancellation (client disconnect) is checked before every SSE write via `ctx.Err()`.
