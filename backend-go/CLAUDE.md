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
| `/v1/responses` | POST | Codex Responses API |
| `/api/channels` | CRUD | 渠道管理 |
| `/api/ping/:id` | GET | 渠道连通性测试 |

## Provider 接口

所有上游服务实现 `internal/providers/Provider` 接口：

```go
type Provider interface {
    ConvertToProviderRequest(c *gin.Context, upstream *config.UpstreamConfig, apiKey string) (*http.Request, []byte, error)
    ConvertToClaudeResponse(providerResp *types.ProviderResponse) (*types.ClaudeResponse, error)
    HandleStreamResponse(body io.ReadCloser) (<-chan string, <-chan error, error)
}
```

**实现**: `ClaudeProvider`, `OpenAIProvider`, `OpenAIOldProvider`, `GeminiProvider`

## 核心模块

| 模块 | 职责 |
|------|------|
| `handlers/` | HTTP 处理器（proxy.go, responses.go） |
| `providers/` | 上游适配器 |
| `converters/` | 协议转换器（工厂模式） |
| `scheduler/` | 多渠道调度（优先级、熔断） |
| `session/` | 会话管理（Trace 亲和性） |
| `config/` | 配置管理（热重载） |

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
