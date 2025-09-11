# Claude API 代理服务器

一个高性能的 Claude API 代理服务器，支持多种上游 AI 服务提供商（OpenAI、Gemini、自定义 API），提供负载均衡、多 API 密钥管理和统一入口访问。

## 🚀 功能特性

- **统一入口**: 所有请求通过单一端点 `http://localhost:3000/v1/messages` 访问
- **多上游支持**: 支持 OpenAI、Gemini、自定义 API 服务商
- **负载均衡**: 支持轮询、随机、故障转移策略
- **多 API 密钥**: 每个上游可配置多个 API 密钥，自动轮换使用
- **配置管理**: 命令行工具轻松管理上游配置
- **环境变量**: 通过 `.env` 文件灵活配置服务器参数
- **健康检查**: 内置健康检查端点
- **日志系统**: 完整的请求/响应日志记录
- **🔄 兼容 Claude Code**: 配合 [One-Balance](https://github.com/glidea/one-balance) 低成本使用 Claude Code
- **📡 支持流式和非流式响应**
- **🛠️ 支持工具调用**

## 📦 安装

### 前置要求

- Node.js 18+ 或 Bun
- pnpm 包管理器

### 安装步骤

1. 克隆项目

```bash
git clone https://github.com/glidea/claude-worker-proxy
cd claude-worker-proxy
```

2. 安装依赖

```bash
pnpm install
```

3. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件，设置你的配置
```

4. 启动服务器

```bash
# 生产环境
pnpm start

# 开发环境（热重载）
pnpm dev:local
```

## ⚙️ 配置

### 代理访问密钥配置

代理服务器需要一个访问密钥来验证客户端请求。这个密钥通过环境变量 `PROXY_ACCESS_KEY` 配置：

```env
PROXY_ACCESS_KEY=your-proxy-access-key
```

**密钥说明**：

- **代理访问密钥**: 在 `.env` 文件中配置，用于验证客户端对代理服务器的访问权限
- **上游 API 密钥**: 通过 `bun run config key` 命令配置，用于代理服务器访问上游 AI 服务商

### 环境变量配置

创建 `.env` 文件（参考 `.env.example`）：

```env
# 服务器配置
PORT=3000
NODE_ENV=development

# 代理访问密钥 - 用于验证客户端对代理服务器的访问权限
PROXY_ACCESS_KEY=your-proxy-access-key

# 负载均衡策略 (round-robin, random, failover)
LOAD_BALANCE_STRATEGY=failover

# 日志级别 (error, warn, info, debug)
LOG_LEVEL=debug

# 是否启用请求/响应日志
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true

# 请求超时时间（毫秒）
REQUEST_TIMEOUT=30000

# 最大并发请求数
MAX_CONCURRENT_REQUESTS=100

# CORS配置
ENABLE_CORS=true
CORS_ORIGIN=*

# 安全配置
ENABLE_RATE_LIMIT=false
RATE_LIMIT_WINDOW=60000
RATE_LIMIT_MAX_REQUESTS=100

# 健康检查配置
HEALTH_CHECK_ENABLED=true
HEALTH_CHECK_PATH=/health
```

### 上游配置管理

使用命令行工具管理上游配置：

```bash
# 添加上游
bun run config add <name> <baseUrl> <serviceType>

# 示例
bun run config add openai-api https://api.openai.com openai
bun run config add gemini-api https://generativelanguage.googleapis.com gemini
bun run config add custom-api https://your-api.com custom

# 添加 API 密钥
bun run config key <upstream-name> add <apiKey1> <apiKey2> ...

# 示例
bun run config key openai-api add sk-1234567890abcdef sk-0987654321fedcba

# 查看当前配置
bun run config show

# 删除上游
bun run config remove <upstream-name>

# 设置负载均衡策略
bun run config balance <strategy>

# 清除所有配置
bun run config clear
```

### 配置文件格式

配置存储在 `config.json` 中：

```json
{
    "upstream": [
        {
            "baseUrl": "https://api.openai.com",
            "apiKeys": ["sk-1234567890abcdef", "sk-0987654321fedcba"],
            "serviceType": "openai",
            "name": "openai-api"
        },
        {
            "baseUrl": "https://generativelanguage.googleapis.com",
            "apiKeys": ["your-gemini-api-key"],
            "serviceType": "gemini",
            "name": "gemini-api"
        }
    ],
    "currentUpstream": 0,
    "loadBalance": "failover"
}
```

## 🔧 API 使用

### 统一入口端点

```
POST http://localhost:3000/v1/messages
```

### 请求头

需要在请求头中包含代理服务器的访问密钥：

```bash
x-api-key: your-proxy-access-key
```

### 工作原理

1. **客户端请求**: 发送请求到代理服务器，包含代理访问密钥
2. **代理验证**: 代理服务器验证访问密钥
3. **上游路由**: 代理服务器根据配置选择上游服务商和 API 密钥
4. **协议转换**: 代理服务器将 Claude API 格式转换为目标服务商格式
5. **响应返回**: 代理服务器将响应转换回 Claude API 格式返回给客户端

### 请求格式

发送标准的 Claude API 格式请求：

```json
{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 1000,
    "messages": [
        {
            "role": "user",
            "content": "Hello, how are you?"
        }
    ]
}
```

### 响应格式

返回标准的 Claude API 格式响应：

```json
{
    "id": "msg_123456789",
    "type": "message",
    "role": "assistant",
    "content": [
        {
            "type": "text",
            "text": "I'm doing well, thank you for asking!"
        }
    ],
    "model": "claude-sonnet-4-20250514",
    "stop_reason": "end_turn",
    "stop_sequence": null,
    "usage": {
        "input_tokens": 15,
        "output_tokens": 12
    }
}
```

### 实际使用示例

使用 cURL 测试代理服务器：

```bash
curl -X POST http://localhost:3000/v1/messages \
  -H "x-api-key: your-proxy-access-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-haiku-20241022",
    "max_tokens": 100,
    "messages": [
      {
        "role": "user",
        "content": "Hello, how are you?"
      }
    ]
  }'
```

## 🏥 健康检查

健康检查端点：

```
GET http://localhost:3000/health
```

响应示例：

```json
{
    "status": "healthy",
    "timestamp": "2024-01-01T00:00:00.000Z",
    "uptime": 120.5,
    "config": {
        "upstreamCount": 2,
        "currentUpstream": "openai-api",
        "loadBalance": "failover"
    }
}
```

## 📊 监控和日志

### 日志级别

- `error`: 仅错误日志
- `warn`: 警告和错误日志
- `info`: 一般信息、警告和错误日志
- `debug`: 所有日志（包括调试信息）

### 日志输出

服务器会输出详细的运行日志：

```
🚀 Claude API代理服务器已启动
📍 本地地址: http://localhost:3000
📋 统一入口: POST /v1/messages
💚 健康检查: GET /health
⚙️  当前配置: openai-api - https://api.openai.com
🔧 使用 'bun run config --help' 查看配置选项
📊 环境: development
🔍 开发模式 - 详细日志已启用
```

## 🔄 负载均衡策略

负载均衡策略应用于**当前选定上游内的多个 API 密钥**，而不是在多个上游之间切换。你可以通过 `bun run config use <index>` 来选择要使用的上游。

### 1. 轮询 (round-robin)

按顺序轮流使用当前上游配置的每个 API 密钥。

### 2. 随机 (random)

在当前上游配置的 API 密钥中随机选择一个使用。

### 3. 故障转移 (failover)

总是优先使用当前上游配置的第一个 API 密钥。这种策略适用于主备密钥场景。

## 🛡️ 安全特性

- API 密钥安全存储和管理
- CORS 跨域请求控制
- 请求频率限制（可选）
- 请求超时保护
- 错误处理和日志记录

## 🚀 部署

### 本地开发

```bash
# 开发模式（热重载）
pnpm dev:local

# 生产模式
pnpm start
```

### Cloudflare Workers 部署

```bash
# 部署到 Cloudflare Workers
pnpm deploycf
```

## 在 Claude Code 中使用

配置 Claude Code 使用本地代理：

```bash
# 编辑 ~/.claude/settings.json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:3000",
    "ANTHROPIC_CUSTOM_HEADERS": "x-api-key: your-proxy-access-key",
    "ANTHROPIC_MODEL": "claude-3-5-sonnet-20241022",
    "ANTHROPIC_SMALL_FAST_MODEL": "claude-3-haiku-20240307",
    "API_TIMEOUT_MS": "600000"
  }
}

claude
```

> **重要说明**: `your-proxy-access-key` 是你访问代理服务器的授权密钥，不是上游服务商的 API key。这个 key 用于验证你对代理服务器的访问权限。

## 🐛 故障排除

### 常见问题

1. **端口被占用**
    - 修改 `.env` 文件中的 `PORT` 配置
    - 或停止占用端口的进程

2. **API 密钥无效**
    - 检查 `config.json` 中的 API 密钥是否正确
    - 确认上游服务商的 API 密钥格式

3. **上游连接失败**
    - 检查网络连接
    - 确认上游服务的 baseUrl 是否正确
    - 检查防火墙设置

4. **配置文件错误**
    - 删除 `config.json` 重新配置
    - 使用 `bun run config show` 检查配置

### 调试模式

在 `.env` 文件中设置：

```env
LOG_LEVEL=debug
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true
```

## 📝 许可证

MIT License

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📞 支持

如有问题，请查看故障排除部分或提交 Issue。
