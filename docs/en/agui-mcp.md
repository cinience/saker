# AG-UI ForwardedProps — Client Configuration Protocol

This document describes how AG-UI clients can configure the agent runtime via
`ForwardedProps` in the `RunAgentInput` payload.

Supported capabilities:
- **Dynamic MCP servers** — connect external tool providers per-session
- **LLM endpoint override** — select model, provider, base URL, with ordered failover
- **System prompt injection** — prepend, append, or replace the built-in prompt
- **Tool control** — whitelist internal tools or declare passthrough tools

## Quick Reference

```json
{
  "threadId": "thread_abc",
  "messages": [{"role": "user", "content": "..."}],
  "forwardedProps": {
    "model_uri": [
      "openai://sk-xxx@api.openai.com/v1?model=gpt-4o&temperature=0.7",
      "anthropic://sk-ant-xxx@api.anthropic.com?model=claude-sonnet-4-20250514"
    ],
    "system_prompt": "You are a specialized assistant for data analysis.",
    "system_prompt_mode": "replace",
    "allowed_tools": ["bash", "file_read", "file_write", "web_search"],
    "passthrough_tools": ["custom_ui_action"],
    "mcp_servers": [
      "https://mcp.example.com/tools?name=weather&timeout=30",
      {"name": "local-db", "type": "stdio", "command": "mcp-server-sqlite", "args": ["db.sqlite"]}
    ],
    "timeout_seconds": 120
  }
}
```

---

## Model Endpoint (`model_uri`)

Specify which LLM backend the agent should use. Supports a single URI string
or an ordered array for automatic failover.

### URI Format

```
provider://api_key@host[:port]/path?model=name&temperature=0.7&max_tokens=4096&...
```

| Component | Maps to | Example |
|-----------|---------|---------|
| scheme | Provider | `openai`, `anthropic`, `dashscope` |
| userinfo | API key | `sk-xxx` |
| host+path | Base URL | `api.openai.com/v1` |
| `?model=` | Model name (required) | `gpt-4o` |
| `?temperature=` | Sampling temperature | `0.7` |
| `?top_p=` | Top-P | `0.9` |
| `?max_tokens=` | Max output tokens | `4096` |
| `?stop=` | Stop sequences (comma-separated) | `END,STOP` |
| `?seed=` | Random seed | `42` |
| `?tool_choice=` | Tool choice strategy | `auto` |
| `?parallel_tool_calls=` | Parallel tool calls | `true` |

### Failover Chain

When `model_uri` is an array, the first entry is the primary model. Subsequent
entries form an ordered failover chain — if the primary fails, the runtime
automatically falls back to the next model via Bifrost SDK-level routing.

```json
"model_uri": [
  "openai://sk-primary@api.openai.com/v1?model=gpt-4o&temperature=0.7",
  "anthropic://sk-backup@api.anthropic.com?model=claude-sonnet-4-20250514",
  "openai://sk-china@dashscope.aliyuncs.com/compatible-mode/v1?model=qwen-max"
]
```

Sampling parameters (temperature, top_p, etc.) are extracted only from the
first URI and apply to the entire failover chain.

### Localhost Detection

URIs targeting `localhost` or `127.x.x.x` automatically use `http://` instead
of `https://`:

```
openai://ollama@localhost:11434/v1?model=llama3
→ base_url: http://localhost:11434/v1
```

---

## System Prompt (`system_prompt`, `system_prompt_mode`)

Inject a custom system prompt that composes with the built-in agent prompt.

| Fi Descrn|-------|------|-------------|
| `system_prompt` | string | The prompt text to inject |
| `system_prompt_mode` | string | How to compose: `"prepend"` (default), `"append"`, or `"replace"` |

Modes:
- **prepend** — Insert client text BEFORE the built-in system prompt
- **append** — Insert client text AFTER the built-in system prompt
- **replace** — Completely replace the built-in system prompt with client text

---

## Tool Control (`allowed_tools`, `passthrough_tools`)

| Field | Type | Description |
|-------|------|-------------|
| `allowed_tools` | []string | Restrict which internal tools are available (whitelist) |
| `passthrough_tools` | []string | Tools the agent should NOT execute — relayed back to client |

```json
"allowed_tools": ["bash", "file_read", "file_write"],
"passthrough_tools": ["ask_user_question", "custom_action"]
```

---

## Dynamic MCP Servers (`mcp_servers`)

The AG-UI protocol allows clients to forward MCP server configurations via
`ForwardedProps.mcp_servers`. The gateway establishes connections to these
servers, registers their tools as a `DynamicToolSource`, and injects any
server-provided instructions into the system prompt. Connections are cached at
the thread level for cross-turn reuse.

```
┌─────────┐   POST /v1/agents/run    ┌──────────────┐
│  Client │ ──── (mcp_servers) ────▶ │  AG-UI GW    │
└─────────┘                          └──────┬───────┘
                                            │
                          ┌─────────────────┼─────────────────┐
                          ▼                 ▼                  ▼
                   ┌────────────┐   ┌────────────┐   ┌────────────┐
                   │ MCP Srv A  │   │ MCP Srv B  │   │ MCP Srv C  │
                   └────────────┘   └────────────┘   └────────────┘
```

## MCP Client Configuration

Each element in `mcp_servers` can be either a **JSON object** or a **URI string**.
Both formats can be mixed in the same array.

### Object Format

```json
{
  "mcp_servers": [
    {
      "name": "my-tools",
      "type": "http",
      "url": "https://mcp.example.com/sse",
      "headers": {"Authorization": "Bearer <token>"},
      "timeout": 5.0
    },
    {
      "name": "local-db",
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-sqlite", "db.sqlite"],
      "env": {"DB_PATH": "/data/app.db"}
    }
  ]
}
```

#### ClientMCPServer fields

| Field     | Type              | Required | Description                                      |
|-----------|-------------------|----------|--------------------------------------------------|
| `name`    | string            | yes      | Unique identifier for this server                |
| `type`    | string            | yes      | Transport: `"http"`, `"sse"`, or `"stdio"`       |
| `url`     | string            | http/sse | Endpoint URL for HTTP-based transports           |
| `command` | string            | stdio    | Command to execute for stdio transport           |
| `args`    | []string          | no       | Arguments passed to the stdio command            |
| `headers` | map[string]string | no       | HTTP headers sent with every request             |
| `env`     | map[string]string | no       | Environment variables for stdio processes        |
| `timeout` | float64           | no       | Per-server connection timeout in seconds (0 = use global default) |

### URI Format

MCP servers can also be specified as URI strings for a more compact notation:

| Transport | URI Format | Example |
|-----------|-----------|---------|
| HTTP (StreamableHTTP) | `https://host/path?name=xxx&timeout=30` | `https://mcp.example.com/tools?name=weather` |
| SSE (legacy) | `sse://host/path?name=xxx` | `sse://mcp.example.com/sse?name=code-tools` |
| Stdio | `stdio:///command?args=a1&args=a2&name=xxx` | `stdio:///npx?args=-y&args=@mcp/server-memory&name=memory` |

URI query parameters:

| Param | Required | Description |
|-------|----------|-------------|
| `name` | yes | Server name (unique identifier) |
| `timeout` | no | Connection timeout in seconds |
| `args` | stdio only | Command arguments (repeatable) |
| `header_*` | no | Custom HTTP headers (e.g., `header_Authorization=Bearer+tok`) |

### Mixed Format Example

```json
{
  "mcp_servers": [
    "https://mcp.example.com/tools?name=weather&timeout=30",
    "sse://mcp.internal/sse?name=code-tools&header_Authorization=Bearer+tok",
    {"name": "local-db", "type": "stdio", "command": "mcp-server-sqlite", "args": ["db.sqlite"]}
  ]
}
```

## Architecture

### Key Components

```
pkg/server/agui/
├── mcp.go            # ClientMCPServer type, parsing, security validation
├── session_mcp.go    # sessionMCPRegistry — per-session connection manager
├── mcp_cache.go      # threadMCPCache — cross-turn registry cache
├── handler_run.go    # Orchestration: parse → validate → connect → run
├── capabilities.go   # /capabilities endpoint with MCP status
└── metrics.go        # Prometheus metrics for MCP subsystem

pkg/tool/
├── dynamic.go        # DynamicToolSource / DynamicInstructionSource interfaces
└── registry_mcp.go   # PingMCP health check method
```

### sessionMCPRegistry

The `sessionMCPRegistry` manages per-session MCP server connections at
per-server granularity. It implements `tool.DynamicToolSource` and
`tool.DynamicInstructionSource`.

Key behaviors:

- **Incremental diff**: `EnsureServers()` compares desired vs. current state.
  Unchanged servers (same name + spec) are retained; removed servers are
  disconnected; new servers are connected in parallel.
- **Health check**: Retained servers are pinged before reuse. A failed ping
  triggers automatic reconnection.
- **Parallel connections**: New servers connect concurrently via `errgroup`
  (default limit: 4 parallel connections).
- **Retry with backoff**: Each server connection retries up to 2 times with
  exponential backoff (250ms → 500ms, capped at 5s) plus full jitter.
- **Tool namespace resolution**: When multiple servers expose a tool with the
  same name, conflicting tools are prefixed with `serverName__toolName`.

### threadMCPCache

The `threadMCPCache` caches `sessionMCPRegistry` instances at the thread level.
Consecutive turns on the same thread reuse existing connections.

| Parameter            | Default | Description                       |
|----------------------|---------|-----------------------------------|
| TTL                  | 10 min  | Idle time before eviction         |
| Max threads          | 200     | Maximum cached thread registries  |
| Cleanup interval     | 2 min   | Background goroutine sweep period |

Eviction strategy:
1. Expired entries (TTL exceeded) are removed first.
2. If still over capacity, LRU (oldest `lastAccess`) entries are evicted.

### DynamicToolSource Interface

```go
type DynamicToolSource interface {
    LookupTool(name string) (Tool, bool)
    ListToolDefs() []model.ToolDefinition
}

type DynamicInstructionSource interface {
    MCPInstructions() map[string]string
}
```

The runtime falls back to `DynamicToolSource` when the primary tool registry
does not contain a requested tool. `DynamicInstructionSource` provides MCP
server instructions that are injected into the system prompt.

## Connection Lifecycle

```
1. Client sends RunAgentInput with mcp_servers
2. Gateway parses and validates (allow-list, security limits)
3. Check threadMCPCache for existing registry
   ├── Cache HIT  → EnsureServers (incremental diff + health check)
   └── Cache MISS → Create new sessionMCPRegistry
4. Connect servers in parallel (with retry + backoff)
5. Set registry as request.DynamicExecutor
6. Runtime uses ListToolDefs() for tool declarations
7. Runtime calls LookupTool(name) for tool execution
8. MCPInstructions() injected into system prompt
9. On run completion → registry cached back to threadMCPCache
10. Background goroutine evicts expired registries every 2 min
```

## Security

Three layers of validation are applied before any connection is established:

### 1. Allow-list (`AllowedMCPPatterns`)

Operator-configured patterns that restrict which servers clients can connect to.
Matching rules:
- Exact name match: `"my-tools"`
- Exact spec match: `"https://mcp.example.com/sse"`
- Prefix wildcard: `"https://mcp.example.com/*"`

An empty allow-list permits all servers (default).

### 2. Security limits (`Options`)

| Option                     | Default | Description                                |
|----------------------------|---------|--------------------------------------------|
| `MaxMCPServersPerSession`  | 5       | Maximum servers per run request            |
| `AllowMCPStdio`            | false   | Whether stdio transport is permitted       |
| `MCPConnectTimeout`        | 10s     | Global connection timeout cap              |

### 3. Per-server timeout cap

A client-specified `timeout` field is capped by the operator's global
`MCPConnectTimeout`. A client cannot request a longer timeout than the operator
allows.

## Tool Namespace Conflict Resolution

When multiple MCP servers expose tools with the same name, the gateway resolves
conflicts by prefixing:

```
Server "alpha" exposes: search, query
Server "beta"  exposes: search, list

Result:
  alpha__search  (prefixed — conflict)
  query          (unique — no prefix)
  beta__search   (prefixed — conflict)
  list           (unique — no prefix)
```

`LookupTool` supports both raw and prefixed names:
1. Direct lookup across all registries (succeeds for unique names).
2. If name contains `__`, strip the prefix and look in the specific server.

## Capabilities Endpoint

`GET /v1/agents/run/capabilities?threadId=<id>` returns MCP status:

```json
{
  "custom": {
    "mcpServers": {
      "dynamic": true,
      "connected": ["my-tools", "local-db"]
    }
  }
}
```

When no `threadId` is provided, `connected` is `null`.

## Observability

Prometheus metrics (namespace: `saker`, subsystem: `agui`):

| Metric                                    | Type      | Description                                    |
|-------------------------------------------|-----------|------------------------------------------------|
| `saker_agui_mcp_cache_hits_total`         | Counter   | Cross-turn cache hits                          |
| `saker_agui_mcp_cache_misses_total`       | Counter   | Cache misses (new registry created)            |
| `saker_agui_mcp_connect_duration_seconds` | Histogram | Time to connect MCP servers (buckets: 0.1–10s) |
| `saker_agui_mcp_health_check_failures_total` | Counter | Health check ping failures                  |
| `saker_agui_mcp_active_connections`       | Gauge     | Active MCP connections across all threads      |

## Error Responses

| HTTP Status         | Code                    | Condition                                |
|---------------------|-------------------------|------------------------------------------|
| 400 Bad Request     | `invalid_request_error` | Malformed `mcp_servers` JSON             |
| 403 Forbidden       | `permission_error`      | Server not in allow-list or stdio denied |
| 502 Bad Gateway     | `mcp_connection_error`  | All connection attempts failed           |

## Example: Full Request Flow

```bash
curl -X POST https://saker.example.com/v1/agents/run \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "threadId": "thread_abc",
    "messages": [{"role": "user", "content": "Query the database"}],
    "forwardedProps": {
      "mcp_servers": [
        {
          "name": "sqlite",
          "type": "http",
          "url": "https://mcp.internal/sqlite",
          "timeout": 3.0
        }
      ]
    }
  }'
```

The agent will have access to all tools exposed by the `sqlite` MCP server
during this run. On subsequent turns with the same `threadId`, the connection
is reused from cache without re-establishing.
