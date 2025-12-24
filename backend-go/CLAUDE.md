# backend-go 模块文档

[← 根目录](../CLAUDE.md)

## 模块职责

Go 后端核心服务：HTTP API、多上游适配、协议转换、智能调度、会话管理、配置热重载。

## 启动命令

```bash
make dev          # 热重载开发
make test         # 运行测试
make test-cover   # 测试 + 覆盖率
make build        # 构建二进制
```

## API 端点

| 端点 | 方法 | 功能 |
|------|------|------|
| `/health` | GET | 健康检查（无需认证） |
| `/v1/messages` | POST | Claude Messages API |
| `/v1/messages/count_tokens` | POST | Token 计数 |
| `/v1/responses` | POST | Codex Responses API |
| `/v1/responses/compact` | POST | 精简版 Responses API |
| `/api/channels` | CRUD | Messages 渠道管理 |
| `/api/responses/channels` | CRUD | Responses 渠道管理 |
| `/api/ping/:id` | GET | 渠道连通性测试 |
| `/api/channels/metrics` | GET | 渠道指标 |
| `/api/channels/scheduler/stats` | GET | 调度器统计 |

## 指标历史数据聚合粒度

`/api/channels/:id/keys/metrics/history` 端点根据查询时间范围自动选择聚合间隔：

| 时间范围 | 聚合间隔 | 数据点数 |
|----------|----------|----------|
| 1h       | 1 分钟   | ~60 点   |
| 6h       | 5 分钟   | ~72 点   |
| 24h      | 15 分钟  | ~96 点   |

可通过 `interval` 参数手动指定（最小 1 分钟）。

## Provider 接口

所有上游服务实现 `internal/providers/Provider` 接口：

```go
type Provider interface {
    ConvertToProviderRequest(c *gin.Context, upstream *config.UpstreamConfig, apiKey string) (*http.Request, []byte, error)
    ConvertToClaudeResponse(providerResp *types.ProviderResponse) (*types.ClaudeResponse, error)
    HandleStreamResponse(body io.ReadCloser) (<-chan string, <-chan error, error)
}
```

**实现**: `ClaudeProvider`, `OpenAIProvider`, `GeminiProvider`

## 核心模块

| 模块 | 职责 |
|------|------|
| `handlers/` | HTTP 处理器（proxy.go, responses.go） |
| `providers/` | 上游适配器 |
| `converters/` | 协议转换器（工厂模式） |
| `scheduler/` | 多渠道调度（优先级、熔断） |
| `session/` | 会话管理（Trace 亲和性） |
| `config/` | 配置管理（热重载） |

## 日志规范

所有日志输出使用 `[Component-Action]` 标签格式，禁止使用 emoji 符号（确保跨平台兼容性）。

**格式规范**:
```go
// 标准格式
log.Printf("[Component-Action] 消息内容: %v", value)

// 警告信息
log.Printf("[Component] 警告: 消息内容")
```

**标签命名示例**:

| 组件 | 标签 | 用途 |
|------|------|------|
| 调度器 | `[Scheduler-Channel]` | 渠道选择 |
| 调度器 | `[Scheduler-Promotion]` | 促销渠道 |
| 调度器 | `[Scheduler-Affinity]` | Trace 亲和性 |
| 调度器 | `[Scheduler-Fallback]` | 降级选择 |
| 认证 | `[Auth-Failed]` | 认证失败 |
| 认证 | `[Auth-Success]` | 认证成功 |
| 指标 | `[Metrics-Store]` | 指标存储 |
| 会话 | `[Session-Manager]` | 会话管理 |
| 配置 | `[Config-Watcher]` | 配置热重载 |
| 压缩 | `[Gzip]` | Gzip 解压缩 |
| 流式 | `[Stream-Token]` | Token 注入 |

## 扩展指南

**添加新上游服务**:
1. 在 `internal/providers/` 创建新文件
2. 实现 `Provider` 接口
3. 在 `GetProvider()` 注册

**调度优先级规则**:
1. 促销期渠道优先
2. Priority 字段排序
3. Trace 亲和性绑定
4. 熔断状态过滤
