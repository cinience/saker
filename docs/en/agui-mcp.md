# AG-UI Dynamic MCP Server Integration

This document describes how AG-UI clients can dynamically connect MCP (Model
Context Protocol) servers on a per-session basis, enabling the agent to use
client-provided tools during a run.

## Overview

The AG-UI protocol allows clients to forward MCP server configurations via
`ForwardedProps.mcp_servers` in the `RunAgentInput` payload. The gateway
establishes connections to these servers, registers their tools as a
`DynamicToolSource`, and injects any server-provided instructions into the
system prompt. Connections are cached at the thread level for cross-turn reuse.

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

## Client Configuration

Clients include MCP servers in `ForwardedProps`:

```json
{
  "threadId": "thread_abc",
  "runId": "run_123",
  "messages": [...],
  "forwardedProps": {
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
}
```

### ClientMCPServer fields

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
