# Claude API 代理服务器

一个高性能的 Claude API 代理服务器，支持多种上游 AI 服务提供商（OpenAI、Gemini、自定义 API），提供负载均衡、多 API 密钥管理和统一入口访问。

## 🚀 功能特性

- **统一入口**: 所有请求通过单一端点 `http://localhost:3000/v1/messages` 访问
- **多上游支持**: 支持 OpenAI (及兼容 API)、Gemini 和 Claude 等多种上游服务
- **负载均衡**: 支持轮询、随机、故障转移策略
- **多 API 密钥**: 每个上游可配置多个 API 密钥，自动轮换使用
- **配置管理**: 命令行工具轻松管理上游配置
- **环境变量**: 通过 `.env` 文件灵活配置服务器参数
- **健康检查**: 内置健康检查端点
- **日志系统**: 完整的请求/响应日志记录
- **📡 支持流式和非流式响应**
- **🛠️ 支持工具调用**

## 🏁 快速开始

### 前置要求

- Node.js 18+ 或 Bun
- pnpm 包管理器

### 安装步骤

1. 克隆项目

```bash
git clone https://github.com/BenedictKing/claude-proxy
cd claude-proxy
```

2. 安装依赖

```bash
bun install
```

3. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件，设置你的配置
```

4. 启动服务器

```bash
# 开发环境 (文件修改后自动重启)
bun run dev

# 生产环境
bun run start
```

5. 配置客户端 (以 Claude Code 为例)

现在代理服务器已在本地运行，您需要配置您的客户端来使用它。

编辑 `~/.claude/settings.json` 文件：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:3000",
    "ANTHROPIC_AUTH_TOKEN": "your-proxy-access-key",
    "DISABLE_TELEMETRY": "1",
    "DISABLE_ERROR_REPORTING": "1",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

> **重要说明**: `your-proxy-access-key` 是您在 `.env` 文件中设置的 `PROXY_ACCESS_KEY`，用于验证您对代理服务器的访问权限，并非上游服务商的 API key。

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
bun run config add <name> <type> <url>

# 示例
bun run config add openai-api openai https://api.openai.com/v1
bun run config add gemini-api gemini https://generativelanguage.googleapis.com/v1beta
bun run config add claude-api claude https://api.anthropic.com/v1

# 添加 API 密钥 (支持索引或名称，一次添加一个)
bun run config key <index|name> add <apiKey>

# 列出 API 密钥（输出已脱敏）
bun run config key <index|name> list

# 示例
bun run config key openai-api add sk-1234567890abcdef
bun run config key openai-api add sk-0987654321fedcba

# 查看当前配置
bun run config show

# 删除上游 (支持索引或名称)
bun run config remove <index|name>

# 设置负载均衡策略
bun run config balance <strategy>

# 开启/关闭跳过TLS证书验证（用于处理证书问题）
bun run config update <index|name> --insecureSkipVerify <true|false>
```

### 🔧 详细配置示例

#### 1. OpenAI 配置

```bash
# 添加 OpenAI 上游
bun run config add openai-main https://api.openai.com openai

# 添加多个 API 密钥（支持负载均衡）
bun run config key openai-main add \
  sk-proj-abc123def456... \
  sk-proj-xyz789uvw456...

# 设置为当前使用的上游
bun run config use openai-main
```

#### 2. Gemini 配置

```bash
# 添加 Gemini 上游
bun run config add gemini-main https://generativelanguage.googleapis.com/v1beta gemini

# 添加 Gemini API 密钥
bun run config key gemini-main add AIzaSyC1234567890abcdef...

# 切换到 Gemini
bun run config use gemini-main
```

#### 3. Claude 配置

```bash
# 添加 Claude 官方上游
bun run config add claude-main https://api.anthropic.com/v1 claude

# 添加 API 密钥
bun run config key claude-main add sk-ant-your-api-key...

# 切换到 Claude
bun run config use claude-main
```

#### 4. 第三方 API 服务配置

```bash
# 添加第三方 Claude 兼容 API
bun run config add anthropic-proxy https://api.your-provider.com openai

# 添加 API 密钥
bun run config key anthropic-proxy add your-api-key-here

# 切换到第三方服务
bun run config use anthropic-proxy
```

#### 4. 多渠道配置与切换

```bash
# 配置多个上游服务
bun run config add openai-primary https://api.openai.com openai
bun run config add openai-backup https://api.openai.com openai
bun run config add gemini-backup https://generativelanguage.googleapis.com/v1beta gemini

# 为每个上游添加密钥
bun run config key openai-primary add sk-primary-key...
bun run config key openai-backup add sk-backup-key...
bun run config key gemini-backup add AIza-backup-key...

# 查看所有配置
bun run config show

# 根据需要切换上游
bun run config use openai-primary    # 使用主要 OpenAI
bun run config use gemini-backup     # 切换到备用 Gemini
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

## 🚀 部署指南

除了通过 `bun run start` 直接启动，您还可以选择更强大的生产环境部署方式。

#### 生产环境 (使用 PM2)

PM2 是一个带有负载均衡器的 Node.js 生产流程管理器，可以帮助您保持应用7x24小时在线。

```bash
# 1. 全局安装 PM2 (如果尚未安装)
npm install -g pm2

# 2. 使用 PM2 启动应用
# 这会使用 bun 来执行 start 脚本
pm2 start bun --name "claude-proxy" -- run start

# 3. 将应用列表保存到硬盘
pm2 save

# 4. 生成并配置启动脚本，使应用在服务器重启后自动启动
pm2 startup
```

#### Docker 部署

您也可以使用 Docker 来容器化和部署此应用。

1.  **在项目根目录创建 `Dockerfile`**

    ```dockerfile
    FROM oven/bun:1

    WORKDIR /app

    # 仅复制必要的文件
    COPY package.json bun.lockb ./

    # 安装生产依赖
    RUN bun install --production --frozen-lockfile

    # 复制源代码
    COPY . .

    # 暴露端口
    EXPOSE 3000

    # 启动命令
    CMD ["bun", "run", "start"]
    ```

2.  **构建和运行 Docker 容器**

    ```bash
    # 构建镜像
    docker build -t claude-api-proxy .

    # 运行容器
    # -d: 后台运行
    # --restart always: 容器退出时总是自动重启
    # -v: 挂载配置文件和环境变量文件，方便修改
    docker run -d -p 3000:3000 \
      -v $(pwd)/config.json:/app/config.json \
      -v $(pwd)/.env:/app/.env \
      --name claude-proxy-container \
      --restart always \
      claude-api-proxy
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

### 🏗️ 工作原理

```mermaid
sequenceDiagram
    participant Client as 客户端
    participant Proxy as 代理服务器
    participant Upstream as 上游API

    Client->>Proxy: POST /v1/messages
    Note over Client,Proxy: 包含代理访问密钥

    Proxy->>Proxy: 验证访问密钥
    Proxy->>Proxy: 获取API密钥 (轮询/随机)

    Proxy->>Proxy: 协议转换 (Claude→上游格式)
    Proxy->>Upstream: 转发请求
    Upstream-->>Proxy: 上游响应

    Proxy->>Proxy: 协议转换 (上游格式→Claude)
    Proxy-->>Client: 返回Claude格式响应
```

### 请求格式

#### 基础文本对话

```json
{
  "model": "claude-3-5-sonnet-20241022",
  "max_tokens": 1000,
  "messages": [
    {
      "role": "user",
      "content": "Hello, how are you?"
    }
  ]
}
```

#### 流式响应

```json
{
  "model": "claude-3-5-sonnet-20241022",
  "max_tokens": 1000,
  "stream": true,
  "messages": [
    {
      "role": "user",
      "content": "Tell me a story"
    }
  ]
}
```

#### 工具调用

```json
{
  "model": "claude-3-5-sonnet-20241022",
  "max_tokens": 1000,
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather information",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The city name"
            }
          }
        }
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather like in Shanghai?"
    }
  ]
}
```

### 响应格式

#### 标准响应

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
  "model": "claude-3-5-sonnet-20241022",
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 15,
    "output_tokens": 12
  }
}
```

#### 流式响应

```json
data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","content":[],"model":"claude-3-5-sonnet-20241022","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":15,"output_tokens":0}}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}

data: {"type":"content_block_stop","index":0}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn","usage":{"output_tokens":1}}}

data: {"type":"message_stop"}
```

### 实际使用示例

#### cURL 示例

```bash
# 基础对话
curl -X POST http://localhost:3000/v1/messages \
  -H "x-api-key: your-proxy-access-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 100,
    "messages": [
      {
        "role": "user",
        "content": "Hello, how are you?"
      }
    ]
  }'

# 流式响应
curl -X POST http://localhost:3000/v1/messages \
  -H "x-api-key: your-proxy-access-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 100,
    "stream": true,
    "messages": [
      {
        "role": "user",
        "content": "Tell me a short story"
      }
    ]
  }'
```

#### Python 示例

```python
import requests
import json

# 配置
base_url = "http://localhost:3000"
api_key = "your-proxy-access-key"

# 发送请求
response = requests.post(
    f"{base_url}/v1/messages",
    headers={
        "x-api-key": api_key,
        "Content-Type": "application/json"
    },
    json={
        "model": "claude-3-5-sonnet-20241022",
        "max_tokens": 1000,
        "messages": [
            {
                "role": "user",
                "content": "Explain quantum computing in simple terms"
            }
        ]
    }
)

print(response.json())
```

#### JavaScript 示例

```javascript
// 使用 fetch API
async function sendMessage(content) {
  const response = await fetch('http://localhost:3000/v1/messages', {
    method: 'POST',
    headers: {
      'x-api-key': 'your-proxy-access-key',
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      model: 'claude-3-5-sonnet-20241022',
      max_tokens: 1000,
      messages: [
        {
          role: 'user',
          content: content
        }
      ]
    })
  })

  const data = await response.json()
  return data
}

// 使用示例
sendMessage('What is the meaning of life?')
  .then(response => console.log(response))
  .catch(error => console.error(error))
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
  "mode": "development",
  "config": {
    "upstreamCount": 2,
    "currentUpstream": 0,
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

## ❓ 常见问题解答 (FAQ)

### Q1: 代理服务器支持哪些上游 AI 服务商？

**A:** 目前支持以下服务商：

- **OpenAI**: 支持 OpenAI 官方 API 以及任何兼容 OpenAI 格式的第三方服务 (使用 `openai` 或 `openaiold` 类型)。
- **Gemini**: Google 的 Gemini API。
- **Claude**: Anthropic 的官方 Claude API。

### Q2: 如何实现 API 密钥的负载均衡？

**A:** 代理服务器支持三种负载均衡策略：

1. **轮询 (round-robin)**: 按顺序轮流使用每个 API 密钥
2. **随机 (random)**: 随机选择一个 API 密钥
3. **故障转移 (failover)**: 总是优先使用第一个密钥

```bash
# 设置负载均衡策略
bun run config balance round-robin
```

### Q3: 可以同时配置多个上游服务商吗？

**A:** 可以！你可以配置多个上游，但同时只能使用一个。通过以下命令切换：

```bash
# 查看所有上游
bun run config show

# 按索引切换
bun run config use 0

# 按名称切换
bun run config use openai-main
```

### Q4: 系统是否需要外部依赖？

**A:** 不需要。系统已经简化，移除了Redis依赖：

- **API密钥轮询**: 使用内存计数器实现
- **配置管理**: 基于本地文件，支持热重载
- **部署简单**: 无需配置外部数据库或缓存

### Q5: 如何在 Claude Code 中使用这个代理？

**A:** 请参考 **[🏁 快速开始](#-快速开始)** 章节中的“5. 配置客户端”部分。该部分提供了详细的步骤来修改 `~/.claude/settings.json` 文件以正确接入本代理服务。

### Q6: 支持流式响应吗？

**A:** 完全支持！在请求中添加 `"stream": true` 即可：

```json
{
    "model": "claude-3-5-sonnet-20241022",
    "stream": true,
    "messages": [...]
}
```

### Q7: 如何监控代理服务器的状态？

**A:** 使用健康检查端点：

```bash
curl http://localhost:3000/health
```

开发模式下还有额外的监控端点：

```bash
# 开发环境信息
curl http://localhost:3000/admin/dev/info

# 重载配置
curl -X POST http://localhost:3000/admin/config/reload
```

## 🐛 故障排除

### 启动问题

#### 1. 端口被占用

**现象**: `Error: listen EADDRINUSE: address already in use :::3000`

**解决方案**:

```bash
# 查看端口占用 (macOS/Linux)
lsof -i :3000

# 强制终止进程
kill -9 <PID>

# 或修改 .env 文件中的端口
PORT=3001
```

#### 2. 配置文件损坏

**现象**: `SyntaxError: Unexpected token in JSON`

**解决方案**:

```bash
# 检查配置文件语法
cat config.json | jq .

# 或直接删除损坏的配置文件，程序会自动重新生成
rm config.json
bun run config show
```

### API 调用问题

#### 1. 401 Unauthorized (未授权)

**可能原因**:

- 客户端发来的 `x-api-key` (代理访问密钥) 不正确。
- 上游服务的 API 密钥无效或已过期。

**解决方案**:

- 确认客户端请求头中的 `x-api-key` 与 `.env` 文件里的 `PROXY_ACCESS_KEY` 一致。
- 使用 `bun run config show` 检查当前上游的密钥是否正确配置。
- 直接用上游密钥测试，以验证其有效性。

#### 2. 429 Too Many Requests (请求过多)

**可能原因**:

- 单个 API 密钥的请求频率或额度已达上限。

**解决方案**:

- 为当前上游添加更多可用的 API 密钥。
  ```bash
  bun run config key your-upstream add sk-new-key
  ```
- 将负载均衡策略设置为 `round-robin` 以分散请求。
  ```bash
  bun run config balance round-robin
  ```

#### 3. 500 Internal Server Error (服务器内部错误)

**现象**: 客户端收到 500 错误，或日志中出现 `ERR_TLS_CERT_ALTNAME_INVALID` 等证书错误。

**可能原因**:

- 上游服务暂时不可用或返回了错误。
- 代理服务器配置错误。
- 上游服务使用了自签名或不匹配的 SSL/TLS 证书。

**解决方案**:

- 首先检查服务器日志，定位问题根源。
- 启用 `debug` 模式以获取最详细的日志：
  ```bash
  # 在 .env 文件中修改
  LOG_LEVEL=debug
  ```
- 如果日志显示为 TLS 证书问题，并且你信任该上游，可以为特定上游开启“跳过 TLS 验证”：
  ```bash
  # 警告：这会降低安全性，仅在必要时使用
  bun run config update your-upstream --insecureSkipVerify true
  ```
- 重启服务器以应用 `.env` 文件的更改。

### 性能问题

#### 1. 响应缓慢

**解决方案**:

- 检查网络到上游服务器的延迟。
- 确认上游服务本身没有性能问题。
- 在 `.env` 文件中适当调整 `MAX_CONCURRENT_REQUESTS` 和 `REQUEST_TIMEOUT`。

#### 2. 内存使用过高

**解决方案**:

- 在生产环境中，将日志级别设置为 `info` 或 `warn` 以减少日志输出带来的开销。
  ```bash
  # 在 .env 文件中修改
  LOG_LEVEL=info
  ENABLE_REQUEST_LOGS=false
  ENABLE_RESPONSE_LOGS=false
  ```
- 重启服务器以应用更改。

## 📝 许可证

MIT License

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📞 支持

如有问题，请查看故障排除部分或提交 Issue。
