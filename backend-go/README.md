# Claude Proxy - Go 版本

> 🚀 高性能的 Claude API 代理服务器 - Go 语言实现，支持多种上游AI服务提供商，内置前端管理界面

## 特性

- ✅ **完整的 TypeScript 后端功能移植**：所有原 TS 后端功能完整实现
- 🚀 **高性能**：Go 语言实现，性能优于 Node.js 版本
- 📦 **单文件部署**：前端资源嵌入二进制文件，无需额外配置
- 🔄 **协议转换**：自动转换 Claude 格式请求到不同上游服务商格式
- ⚖️ **负载均衡**：支持多 API 密钥的智能分配和故障转移
- 🖥️ **Web 管理界面**：内置的前端管理界面（嵌入式）
- 🛡️ **高可用性**：健康检查、错误处理和优雅降级

## 支持的上游服务

- ✅ OpenAI (GPT-4, GPT-3.5 等)
- ✅ Gemini (Google AI)
- ✅ Claude (Anthropic)
- ✅ OpenAI Old (旧版兼容)

## 快速开始

### 方式1：下载预编译二进制文件（推荐）

1. 从 [Releases](https://github.com/yourusername/claude-proxy/releases) 下载对应平台的二进制文件
2. 创建 `.env` 文件：

```bash
# 复制示例配置
cp .env.example .env

# 编辑配置
nano .env
```

3. 运行服务器：

```bash
# Linux / macOS
./claude-proxy-linux-amd64

# Windows
claude-proxy-windows-amd64.exe
```

### 方式2：从源码构建

#### 前置要求

- Go 1.22 或更高版本
- Node.js 18+ (用于构建前端)

#### 构建步骤

```bash
# 1. 克隆项目
git clone https://github.com/yourusername/claude-proxy.git
cd claude-proxy

# 2. 构建前端
cd frontend
npm install
npm run build
cd ..

# 3. 构建 Go 后端（包含前端资源）
cd backend-go
./build.sh

# 构建产物位于 dist/ 目录
```

## 配置说明

### 环境变量配置 (.env)

```env
# 服务器配置
PORT=3000
NODE_ENV=production

# Web UI
ENABLE_WEB_UI=true

# 访问控制
PROXY_ACCESS_KEY=your-secure-access-key

# 负载均衡
LOAD_BALANCE_STRATEGY=failover  # failover | round-robin | random

# 日志配置
LOG_LEVEL=info  # error | warn | info | debug
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true

# 其他配置
REQUEST_TIMEOUT=30000
MAX_CONCURRENT_REQUESTS=100
HEALTH_CHECK_PATH=/health
```

### 渠道配置

服务启动后，通过 Web 管理界面 (http://localhost:3000) 配置上游渠道和 API 密钥。

或者直接编辑配置文件 `.config/config.json`：

```json
{
  "upstream": [
    {
      "name": "OpenAI",
      "baseUrl": "https://api.openai.com/v1",
      "apiKeys": ["sk-your-api-key"],
      "serviceType": "openai"
    }
  ],
  "currentUpstream": 0,
  "loadBalance": "failover"
}
```

## 使用方法

### 访问 Web 管理界面

打开浏览器访问: http://localhost:3000

首次访问需要输入 `PROXY_ACCESS_KEY`

### API 调用

```bash
curl -X POST http://localhost:3000/v1/messages \
  -H "x-api-key: your-proxy-access-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, Claude!"}
    ]
  }'
```

## 架构对比

| 特性 | TypeScript 版本 | Go 版本 |
|------|----------------|---------|
| 运行时 | Node.js/Bun | Go (编译型) |
| 性能 | 中等 | 高 |
| 内存占用 | 较高 | 较低 |
| 部署 | 需要 Node.js 环境 | 单文件可执行 |
| 启动速度 | 较慢 | 快速 |
| 并发处理 | 事件循环 | Goroutine（原生并发）|

## 目录结构

```
backend-go/
├── main.go                 # 主程序入口
├── go.mod                  # Go 模块定义
├── build.sh                # 构建脚本
├── internal/
│   ├── config/             # 配置管理
│   │   ├── env.go          # 环境变量配置
│   │   └── config.go       # 配置文件管理
│   ├── providers/          # 上游服务适配器
│   │   ├── provider.go     # Provider 接口
│   │   ├── openai.go       # OpenAI 适配器
│   │   ├── gemini.go       # Gemini 适配器
│   │   ├── claude.go       # Claude 适配器
│   │   └── openaiold.go    # OpenAI Old 适配器
│   ├── middleware/         # HTTP 中间件
│   │   ├── cors.go         # CORS 中间件
│   │   └── auth.go         # 认证中间件
│   ├── handlers/           # HTTP 处理器
│   │   ├── health.go       # 健康检查
│   │   ├── config.go       # 配置管理 API
│   │   ├── proxy.go        # 代理处理逻辑
│   │   └── frontend.go     # 前端资源服务
│   └── types/              # 类型定义
│       └── types.go        # 请求/响应类型
└── frontend/dist/          # 嵌入的前端资源（构建时生成）
```

## 性能优化

Go 版本相比 TypeScript 版本的性能优势：

1. **更低的内存占用**：Go 的垃圾回收机制更高效
2. **更快的启动速度**：编译型语言，无需运行时解析
3. **更好的并发性能**：原生 Goroutine 支持
4. **更小的部署包**：单文件可执行，无需 node_modules

## 常见问题

### 1. 如何更新前端资源？

重新构建前端后，运行 `./build.sh` 重新打包。

### 2. 如何禁用 Web UI？

在 `.env` 文件中设置 `ENABLE_WEB_UI=false`

### 3. 支持热重载配置吗？

支持！配置文件（`.config/config.json`）变更会自动重载，无需重启服务器。

### 4. 如何添加自定义上游服务？

实现 `providers.Provider` 接口并在 `providers.GetProvider` 中注册即可。

## 开发

```bash
# 开发模式运行
go run main.go

# 运行测试
go test ./...

# 格式化代码
go fmt ./...

# 静态检查
go vet ./...
```

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

---

**注意**: 这是 Claude Proxy 的 Go 语言重写版本，完整实现了原 TypeScript 版本的所有功能，并提供了更好的性能和部署体验。
