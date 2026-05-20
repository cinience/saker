# AG-UI 动态 MCP 服务器集成

本文档描述 AG-UI 客户端如何在每次会话中动态连接 MCP（Model Context
Protocol）服务器，使 agent 能够在运行期间使用客户端提供的工具。

## 概述

AG-UI 协议允许客户端通过 `RunAgentInput` 负载中的
`ForwardedProps.mcp_servers` 转发 MCP 服务器配置。网关会建立到这些服务器的
连接，将其工具注册为 `DynamicToolSource`，并将服务器提供的指令注入系统提示。
连接在 thread 级别缓存，支持跨 turn 复用。

```
┌─────────┐   POST /v1/agents/run    ┌──────────────┐
│  客户端  │ ──── (mcp_servers) ────▶ │  AG-UI 网关   │
└─────────┘                          └──────┬───────┘
                                            │
                          ┌─────────────────┼─────────────────┐
                          ▼                 ▼                  ▼
                   ┌────────────┐   ┌────────────┐   ┌────────────┐
                   │ MCP 服务 A  │   │ MCP 服务 B  │   │ MCP 服务 C  │
                   └────────────┘   └────────────┘   └────────────┘
```

## 客户端配置

客户端在 `ForwardedProps` 中包含 MCP 服务器配置：

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

### ClientMCPServer 字段

| 字段      | 类型              | 必填   | 说明                                          |
|-----------|-------------------|--------|-----------------------------------------------|
| `name`    | string            | 是     | 服务器的唯一标识符                            |
| `type`    | string            | 是     | 传输方式：`"http"`、`"sse"` 或 `"stdio"`      |
| `url`     | string            | http/sse | HTTP 传输的端点 URL                         |
| `command` | string            | stdio  | stdio 传输要执行的命令                        |
| `args`    | []string          | 否     | 传递给 stdio 命令的参数                       |
| `headers` | map[string]string | 否     | 每次请求携带的 HTTP 头                        |
| `env`     | map[string]string | 否     | stdio 进程的环境变量                          |
| `timeout` | float64           | 否     | 单服务器连接超时（秒），0 表示使用全局默认值  |

## 架构

### 关键组件

```
pkg/server/agui/
├── mcp.go            # ClientMCPServer 类型、解析、安全验证
├── session_mcp.go    # sessionMCPRegistry — 会话级连接管理器
├── mcp_cache.go      # threadMCPCache — 跨 turn 注册表缓存
├── handler_run.go    # 编排：解析 → 验证 → 连接 → 运行
├── capabilities.go   # /capabilities 端点暴露 MCP 状态
└── metrics.go        # MCP 子系统的 Prometheus 指标

pkg/tool/
├── dynamic.go        # DynamicToolSource / DynamicInstructionSource 接口
└── registry_mcp.go   # PingMCP 健康检查方法
```

### sessionMCPRegistry

`sessionMCPRegistry` 以 per-server 粒度管理会话级 MCP 服务器连接，
实现了 `tool.DynamicToolSource` 和 `tool.DynamicInstructionSource` 接口。

关键行为：

- **增量 diff**：`EnsureServers()` 比较期望状态与当前状态。未变更的服务器
  （相同 name + spec）保留；已移除的服务器断开；新增服务器并行连接。
- **健康检查**：复用的服务器在使用前进行 ping 检测，ping 失败时自动重连。
- **并行连接**：新服务器通过 `errgroup` 并发连接（默认限制：4 个并行连接）。
- **重试退避**：每个服务器连接最多重试 2 次，使用指数退避
  （250ms → 500ms，上限 5s）加全随机抖动。
- **工具命名空间解析**：当多个服务器暴露同名工具时，冲突的工具以
  `serverName__toolName` 格式前缀化。

### threadMCPCache

`threadMCPCache` 在 thread 级别缓存 `sessionMCPRegistry` 实例，
使同一 thread 的连续 turn 复用已有连接。

| 参数               | 默认值  | 说明                         |
|--------------------|---------|------------------------------|
| TTL                | 10 分钟 | 空闲后驱逐时间               |
| 最大 thread 数     | 200     | 缓存的最大 thread 注册表数   |
| 清理间隔           | 2 分钟  | 后台 goroutine 扫描周期      |

驱逐策略：
1. 优先移除过期条目（超过 TTL）。
2. 仍超出容量时，驱逐 LRU（`lastAccess` 最早）条目。

### DynamicToolSource 接口

```go
type DynamicToolSource interface {
    LookupTool(name string) (Tool, bool)
    ListToolDefs() []model.ToolDefinition
}

type DynamicInstructionSource interface {
    MCPInstructions() map[string]string
}
```

当主工具注册表中未找到请求的工具时，runtime 回退到 `DynamicToolSource`。
`DynamicInstructionSource` 提供 MCP 服务器指令，注入到系统提示中。

## 连接生命周期

```
1. 客户端发送包含 mcp_servers 的 RunAgentInput
2. 网关解析并验证（允许列表、安全限制）
3. 检查 threadMCPCache 是否有已缓存的注册表
   ├── 缓存命中 → EnsureServers（增量 diff + 健康检查）
   └── 缓存未命中 → 创建新的 sessionMCPRegistry
4. 并行连接服务器（带重试 + 退避）
5. 将注册表设置为 request.DynamicExecutor
6. Runtime 使用 ListToolDefs() 获取工具声明
7. Runtime 调用 LookupTool(name) 执行工具
8. MCPInstructions() 注入系统提示
9. 运行完成 → 注册表缓存回 threadMCPCache
10. 后台 goroutine 每 2 分钟驱逐过期注册表
```

## 安全机制

在建立任何连接之前，执行三层验证：

### 1. 允许列表（`AllowedMCPPatterns`）

运营商配置的模式，限制客户端可以连接的服务器。匹配规则：
- 精确名称匹配：`"my-tools"`
- 精确 spec 匹配：`"https://mcp.example.com/sse"`
- 前缀通配符：`"https://mcp.example.com/*"`

空允许列表表示允许所有服务器（默认行为）。

### 2. 安全限制（`Options`）

| 选项                       | 默认值 | 说明                           |
|----------------------------|--------|--------------------------------|
| `MaxMCPServersPerSession`  | 5      | 每次请求的最大服务器数         |
| `AllowMCPStdio`            | false  | 是否允许 stdio 传输            |
| `MCPConnectTimeout`        | 10s    | 全局连接超时上限               |

### 3. 单服务器超时上限

客户端指定的 `timeout` 字段受运营商全局 `MCPConnectTimeout` 限制，
客户端不能请求比运营商允许的更长超时。

## 工具命名空间冲突解决

当多个 MCP 服务器暴露同名工具时，网关通过前缀化解决冲突：

```
服务器 "alpha" 暴露：search, query
服务器 "beta"  暴露：search, list

结果：
  alpha__search  （前缀化 — 冲突）
  query          （唯一 — 无前缀）
  beta__search   （前缀化 — 冲突）
  list           （唯一 — 无前缀）
```

`LookupTool` 同时支持原始名称和前缀名称：
1. 直接在所有注册表中查找（对唯一名称生效）。
2. 如果名称包含 `__`，剥离前缀并在指定服务器中查找。

## Capabilities 端点

`GET /v1/agents/run/capabilities?threadId=<id>` 返回 MCP 状态：

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

未提供 `threadId` 时，`connected` 为 `null`。

## 可观测性

Prometheus 指标（namespace: `saker`，subsystem: `agui`）：

| 指标                                         | 类型      | 说明                              |
|----------------------------------------------|-----------|-----------------------------------|
| `saker_agui_mcp_cache_hits_total`            | Counter   | 跨 turn 缓存命中                  |
| `saker_agui_mcp_cache_misses_total`          | Counter   | 缓存未命中（创建新注册表）        |
| `saker_agui_mcp_connect_duration_seconds`    | Histogram | MCP 服务器连接耗时（桶：0.1–10s） |
| `saker_agui_mcp_health_check_failures_total` | Counter   | 健康检查 ping 失败次数            |
| `saker_agui_mcp_active_connections`          | Gauge     | 所有 thread 的活跃 MCP 连接数     |

## 错误响应

| HTTP 状态码           | 错误码                  | 条件                                   |
|-----------------------|-------------------------|----------------------------------------|
| 400 Bad Request       | `invalid_request_error` | `mcp_servers` JSON 格式错误            |
| 403 Forbidden         | `permission_error`      | 服务器不在允许列表或 stdio 被禁止      |
| 502 Bad Gateway       | `mcp_connection_error`  | 所有连接尝试均失败                     |

## 示例：完整请求流程

```bash
curl -X POST https://saker.example.com/v1/agents/run \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "threadId": "thread_abc",
    "messages": [{"role": "user", "content": "查询数据库"}],
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

Agent 在本次运行期间可以访问 `sqlite` MCP 服务器暴露的所有工具。
在后续使用相同 `threadId` 的 turn 中，连接从缓存复用，无需重新建立。
