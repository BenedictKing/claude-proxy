[根目录](../CLAUDE.md) > **backend-go**

# backend-go 模块文档

## 变更记录 (Changelog)

### 2025-12-11 - 初始索引
- 创建模块文档
- 记录核心子模块结构
- 整理入口与 API 端点

---

## 模块职责

Go 语言实现的 Claude API 代理服务器核心后端，提供：
- HTTP API 路由和处理
- 多上游服务适配（OpenAI/Gemini/Claude）
- 协议转换（Messages API / Responses API）
- 多渠道智能调度和负载均衡
- 会话管理和 Trace 亲和性
- 配置热重载和指标监控

## 入口与启动

**主入口**: `main.go`

核心初始化流程：
1. 加载环境变量 (`.env`)
2. 初始化日志系统 (`internal/logger`)
3. 初始化配置管理器 (`internal/config`)
4. 初始化会话管理器 (`internal/session`)
5. 初始化多渠道调度器 (`internal/scheduler`)
6. 启动 Gin HTTP 服务器

**启动命令**:
```bash
# 开发模式（热重载）
make dev

# 生产运行
make run

# 构建二进制
make build
```

## 对外接口

### API 端点

| 端点 | 方法 | 认证 | 功能 |
|------|------|------|------|
| `/health` | GET | 否 | 健康检查 |
| `/v1/messages` | POST | 是 | Claude Messages API 代理 |
| `/v1/responses` | POST | 是 | Codex Responses API 代理 |
| `/api/channels` | GET | 是 | 获取渠道列表 |
| `/api/channels` | POST | 是 | 添加渠道 |
| `/api/channels/:id` | PUT | 是 | 更新渠道 |
| `/api/channels/:id` | DELETE | 是 | 删除渠道 |
| `/api/channels/metrics` | GET | 是 | 渠道指标 |
| `/api/ping/:id` | GET | 是 | 测试渠道连通性 |
| `/admin/config/reload` | POST | 是 | 重载配置 |

### Provider 接口

所有上游服务需实现 `internal/providers/Provider` 接口：

```go
type Provider interface {
    ConvertToProviderRequest(c *gin.Context, upstream *config.UpstreamConfig, apiKey string) (*http.Request, []byte, error)
    ConvertToClaudeResponse(providerResp *types.ProviderResponse) (*types.ClaudeResponse, error)
    HandleStreamResponse(body io.ReadCloser) (<-chan string, <-chan error, error)
}
```

**实现列表**:
- `ClaudeProvider` - Claude 原生协议
- `OpenAIProvider` - OpenAI 协议转换
- `OpenAIOldProvider` - OpenAI 旧版协议
- `GeminiProvider` - Gemini 协议转换

## 关键依赖与配置

### 外部依赖
```go
github.com/gin-gonic/gin v1.10.0       // HTTP 框架
github.com/fsnotify/fsnotify v1.7.0    // 文件监控（热重载）
github.com/joho/godotenv v1.5.1        // 环境变量加载
gopkg.in/natefinch/lumberjack.v2       // 日志滚动
```

### 配置文件
- **环境配置**: `.env` - 服务器基础配置
- **渠道配置**: `.config/config.json` - 上游服务配置（支持热重载）
- **配置备份**: `.config/backups/` - 自动备份历史配置

### 环境变量
参考根目录 [ENVIRONMENT.md](../ENVIRONMENT.md)

## 数据模型

### 核心类型

#### ClaudeRequest / ClaudeResponse
```go
// internal/types/types.go
type ClaudeRequest struct {
    Model       string         `json:"model"`
    MaxTokens   int            `json:"max_tokens"`
    Messages    []Message      `json:"messages"`
    Stream      bool           `json:"stream"`
    Temperature float64        `json:"temperature,omitempty"`
    Tools       []Tool         `json:"tools,omitempty"`
}
```

#### ResponsesRequest / ResponsesResponse
```go
// internal/types/responses.go
type ResponsesRequest struct {
    Model              string      `json:"model"`
    Input              interface{} `json:"input"`
    PreviousResponseID string      `json:"previous_response_id,omitempty"`
    Stream             bool        `json:"stream"`
    Store              bool        `json:"store"`
}
```

#### UpstreamConfig
```go
// internal/config/config.go
type UpstreamConfig struct {
    BaseURL            string            `json:"baseUrl"`
    APIKeys            []string          `json:"apiKeys"`
    ServiceType        string            `json:"serviceType"`
    Priority           int               `json:"priority"`
    Status             string            `json:"status"`
    PromotionUntil     *time.Time        `json:"promotionUntil,omitempty"`
}
```

## 测试与质量

### 单元测试文件
- `internal/converters/converter_test.go` - 协议转换测试
- `internal/converters/chat_to_responses_test.go` - Chat/Responses 转换测试
- `internal/middleware/auth_test.go` - 认证中间件测试
- `internal/utils/json_test.go` - JSON 工具测试
- `internal/utils/json_compact_test.go` - JSON 压缩测试
- `internal/utils/headers_test.go` - HTTP 头处理测试

### 运行测试
```bash
# 运行所有测试
make test

# 生成覆盖率报告
make test-cover

# 查看覆盖率 HTML
open coverage.html
```

### 代码质量工具
```bash
# 格式化代码
make fmt

# 静态检查
make lint
```

## 常见问题 (FAQ)

### Q1: 如何添加新的上游服务类型？
1. 在 `internal/providers/` 创建新文件（如 `custom.go`）
2. 实现 `Provider` 接口的三个方法
3. 在 `GetProvider()` 函数中注册新类型
4. 更新 `UpstreamConfig.ServiceType` 文档

### Q2: 如何调试配置热重载？
- 查看日志输出中的 "配置文件已更新" 消息
- 检查 `.config/backups/` 目录下的备份文件
- 使用 `POST /admin/config/reload` 手动触发重载

### Q3: 如何处理大请求/响应体？
- 后端已实现 gzip 压缩支持（`internal/utils/compression.go`）
- 流式响应自动处理（`internal/providers/*Provider.HandleStreamResponse`）
- 日志中大 body 会被截断（参考 `internal/handlers/proxy.go`）

### Q4: 多渠道调度的优先级规则？
1. 促销期渠道（PromotionUntil 未过期）优先
2. 按 Priority 字段排序（数字越小越高）
3. Trace 亲和性绑定（同 user_id 倾向同渠道）
4. 熔断状态过滤（失败率过高自动跳过）

### Q5: 会话管理的清理策略？
- 默认 24 小时过期（可配置）
- 最多保存 100 条消息（可配置）
- 最多 100k tokens（可配置）
- 定期清理任务自动运行

## 相关文件清单

### 核心代码
```
backend-go/
├── main.go                           # 主入口
├── version.go                        # 版本信息
├── Makefile                          # 构建工具
├── go.mod                            # Go 模块定义
└── internal/
    ├── handlers/                     # HTTP 处理器
    │   ├── proxy.go                  # Messages API 代理
    │   ├── responses.go              # Responses API 代理
    │   ├── config.go                 # 配置管理 API
    │   ├── health.go                 # 健康检查
    │   ├── frontend.go               # 前端资源服务
    │   └── channel_metrics_handler.go # 渠道指标 API
    ├── providers/                    # 上游适配器
    │   ├── provider.go               # Provider 接口
    │   ├── claude.go                 # Claude 实现
    │   ├── openai.go                 # OpenAI 实现
    │   ├── gemini.go                 # Gemini 实现
    │   └── responses.go              # Responses 专用
    ├── converters/                   # 协议转换器
    │   ├── converter.go              # 转换器接口
    │   ├── factory.go                # 转换器工厂
    │   ├── openai_converter.go       # OpenAI 转换
    │   ├── claude_converter.go       # Claude 转换
    │   ├── responses_passthrough.go  # 直通模式
    │   ├── chat_to_responses.go      # Chat→Responses
    │   └── responses_to_chat.go      # Responses→Chat
    ├── config/                       # 配置管理
    │   ├── config.go                 # 配置管理器
    │   └── env.go                    # 环境变量
    ├── session/                      # 会话管理
    │   ├── manager.go                # 会话管理器
    │   └── trace_affinity.go         # Trace 亲和性
    ├── scheduler/                    # 调度器
    │   └── channel_scheduler.go      # 多渠道调度
    ├── metrics/                      # 指标监控
    │   └── channel_metrics.go        # 渠道指标
    ├── middleware/                   # 中间件
    │   ├── auth.go                   # 认证中间件
    │   └── cors.go                   # CORS 中间件
    ├── logger/                       # 日志系统
    │   └── logger.go                 # 日志配置
    ├── httpclient/                   # HTTP 客户端
    │   └── client.go                 # 自定义客户端
    ├── types/                        # 类型定义
    │   ├── types.go                  # Messages 类型
    │   └── responses.go              # Responses 类型
    └── utils/                        # 工具函数
        ├── json.go                   # JSON 处理
        ├── compression.go            # 压缩工具
        ├── headers.go                # HTTP 头处理
        └── stream_synthesizer.go     # 流式响应合成
```

### 配置和构建
```
backend-go/
├── .env.example                      # 环境变量示例
├── .air.toml                         # Air 热重载配置
└── .config/
    ├── config.json                   # 渠道配置（热重载）
    └── backups/                      # 配置备份
```

## 子模块索引

| 子模块 | 职责 | 关键文件 |
|--------|------|----------|
| `handlers/` | HTTP 请求处理 | `proxy.go`, `responses.go`, `config.go` |
| `providers/` | 上游服务适配 | `provider.go`, `openai.go`, `gemini.go`, `claude.go` |
| `converters/` | 协议转换 | `factory.go`, `openai_converter.go`, `claude_converter.go` |
| `config/` | 配置管理 | `config.go`, `env.go` |
| `session/` | 会话管理 | `manager.go`, `trace_affinity.go` |
| `scheduler/` | 多渠道调度 | `channel_scheduler.go` |
| `metrics/` | 指标监控 | `channel_metrics.go` |
| `middleware/` | HTTP 中间件 | `auth.go`, `cors.go` |
| `logger/` | 日志系统 | `logger.go` |
| `types/` | 数据类型 | `types.go`, `responses.go` |
| `utils/` | 工具函数 | `json.go`, `compression.go`, `headers.go` |
