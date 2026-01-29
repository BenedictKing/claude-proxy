# Claude / Codex / Gemini API Proxy - Go 版本

> 🚀 高性能的 Claude / Codex / Gemini API Proxy - Go 语言实现，支持多种上游AI服务提供商，内置前端管理界面

## 特性

- ✅ **完整的 TypeScript 后端功能移植**：所有原 TS 后端功能完整实现
- 🚀 **高性能**：Go 语言实现，性能优于 Node.js 版本
- 📦 **单文件部署**：前端资源嵌入二进制文件，无需额外配置
- 🔄 **协议转换**：自动转换 Claude 格式请求到不同上游服务商格式
- ⚖️ **故障转移**：支持多 API 密钥的智能分配和自动切换
- 🖥️ **Web 管理界面**：内置的前端管理界面（嵌入式）
- 🛡️ **高可用性**：健康检查、错误处理和优雅降级

## 支持的上游服务

- ✅ OpenAI (GPT-4, GPT-3.5 等)
- ✅ Gemini (Google AI)
- ✅ Claude (Anthropic)
- ✅ OpenAI Old (旧版兼容)

## 最新更新 (v2.0.1)

### 🐛 重要修复
- ✅ 修复前端资源加载问题（Vite base 路径配置）
- ✅ 修复静态文件 MIME 类型错误（favicon.ico 等）
- ✅ 修复 API 路由与前端不匹配问题
- ✅ 修复版本信息未注入问题

### ⚡ 性能优化
- ✅ 智能前端构建缓存（无变更时 0.07秒启动，提升 142 倍）
- ✅ 优化代码分割（vue-vendor 独立打包）

### 📝 改进
- ✅ ENV 环境变量标准化（替代 NODE_ENV，向后兼容）
- ✅ 添加 favicon 支持（SVG 格式）
- ✅ 完善文档和开发指南

---

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
# ============ 服务器配置 ============
PORT=3000

# 运行环境: development | production
# 影响:
#   - production: Gin ReleaseMode(高性能)、关闭/admin/dev/info、严格CORS
#   - development: Gin DebugMode(详细日志)、开启/admin/dev/info、宽松CORS
ENV=production

# ============ Web UI 配置 ============
ENABLE_WEB_UI=true

# ============ 访问控制 ============
# 代理访问密钥（必须修改！）
PROXY_ACCESS_KEY=your-secure-access-key

# ============ 日志配置 ============
# 日志级别: error | warn | info | debug
LOG_LEVEL=info

# 是否启用请求/响应日志
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true

# ============ 性能配置 ============
# 请求超时时间（毫秒）
REQUEST_TIMEOUT=30000

# ============ CORS 配置 ============
ENABLE_CORS=true
CORS_ORIGIN=*
```

### 环境模式详解

| 配置项 | development | production |
|--------|-------------|------------|
| **Gin 模式** | DebugMode (详细日志) | ReleaseMode (高性能) |
| **开发端点** | `/admin/dev/info` 开启 | `/admin/dev/info` 关闭 |
| **CORS 策略** | 自动允许所有 localhost 源 | 严格使用 CORS_ORIGIN 配置 |
| **日志输出** | 路由注册、请求详情 | 仅错误和警告 |
| **安全性** | 低（暴露调试信息） | 高（最小信息暴露） |

**建议**：
- 开发测试时使用 `ENV=development`
- 生产部署时务必使用 `ENV=production`

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
      "serviceType": "openai",
      "status": "active"
    }
  ],
  "loadBalance": "failover"
}
```

### 渠道状态自动变化

以下场景会触发渠道状态的自动变化：

| 场景 | 触发条件 | 自动行为 |
|------|----------|----------|
| **单 Key 更换自动激活** | 渠道只有 1 个 Key，且更新为不同的 Key | 1. 状态从 `suspended` 变为 `active`<br>2. 重置熔断状态（清除错误计数） |
| **熔断自动恢复** | 渠道熔断后超过恢复时间（默认 15 分钟） | 自动清除熔断标记，渠道恢复可用 |
| **无 Key 自动暂停** | 渠道配置为 `active` 但没有 API Key | 状态自动设为 `suspended` |

**设计说明：**
- 单 Key 更换时自动激活，因为用户明显想要使用新 Key
- 多 Key 场景不会自动激活，避免误操作（用户可能只是添加/删除部分 Key）
- `disabled` 状态不受影响，用户主动禁用的渠道不会被自动激活

### 渠道促销期（Promotion）

促销期机制用于临时提升某个渠道的优先级，让新渠道能够快速获得流量进行测试。

**促销期特性：**
- 处于促销期的渠道会被**优先选择**，忽略 trace 亲和性
- 同一时间**只能有一个渠道**处于促销期（设置新渠道会自动清除旧渠道的促销期）
- 促销期有**时间限制**，到期后自动失效
- 促销渠道如果**不健康**（熔断/无可用密钥），会自动跳过

**自动触发场景：**

| 场景 | 触发条件 | 自动行为 |
|------|----------|----------|
| **快速添加渠道** | 通过 Web UI 快速添加新渠道 | 1. 新渠道排序到第一位<br>2. 设置 5 分钟促销期 |

**API 使用：**
```bash
# 设置渠道促销期（600秒 = 10分钟）
curl -X POST http://localhost:3000/api/channels/0/promotion \
  -H "x-api-key: your-proxy-access-key" \
  -H "Content-Type: application/json" \
  -d '{"duration": 600}'

# 清除渠道促销期
curl -X POST http://localhost:3000/api/channels/0/promotion \
  -H "x-api-key: your-proxy-access-key" \
  -H "Content-Type: application/json" \
  -d '{"duration": 0}'
```

**适用场景：**
- 新增渠道后，临时提升优先级进行测试
- 更换 Key 后，验证新 Key 是否正常工作
- 临时将流量切换到特定渠道

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
│   │   └── claude.go       # Claude 适配器
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

### 🔥 热重载开发模式（新增）

Go 版本现在支持代码热重载，修改代码后自动重新编译和重启！

#### 安装热重载工具

```bash
# 方式一：使用 make（推荐）
make install-air

# 方式二：使用 npm/bun
npm run dev:go:install

# 方式三：直接安装
go install github.com/air-verse/air@latest
```

#### 启动热重载开发模式

```bash
# 方式一：使用 make（推荐）
make dev              # 自动检测并安装 Air，启动热重载

# 方式二：使用 npm/bun
npm run dev:go        # 或 bun run dev:go

# 方式三：直接使用 air
cd backend-go && air
```

**热重载特性：**
- ✅ **自动重启** - 修改 `.go` 文件后自动重新编译和重启
- ✅ **配置监听** - 修改 `.yaml`, `.toml`, `.env` 文件也会触发重启
- ✅ **错误恢复** - 编译错误时保持运行，修复后自动恢复
- ✅ **彩色日志** - 不同类型日志使用不同颜色，便于调试
- ✅ **性能优化** - 1秒延迟编译，避免频繁重启

### 推荐开发流程（智能缓存）

```bash
# 使用 Makefile - 自动管理前端构建缓存
make dev              # 🔥 热重载开发模式（推荐）
make run              # 首次构建前端，后续仅在源文件变更时重新编译
make build            # 构建生产版本
make clean            # 清除所有构建缓存和临时文件

# 手动控制
make build-local      # 构建本地版本
make test             # 运行测试
make test-cover       # 生成测试覆盖率报告
make fmt              # 格式化代码
make lint             # 代码检查
make deps             # 更新依赖
```

**智能缓存机制：**
- ✅ `make run` 自动检测 `frontend/src` 目录文件变更
- ✅ 未变更时跳过编译，**秒级启动**服务器
- ✅ 首次运行或源文件修改后自动重新编译
- ✅ 使用标记文件 `.build-marker` 追踪构建状态

### Air 配置说明

`.air.toml` 文件定义了热重载行为：

```toml
# 监听的文件类型
include_ext = ["go", "tpl", "tmpl", "html", "yaml", "yml", "toml", "env"]

# 忽略的目录
exclude_dir = ["assets", "tmp", "vendor", "frontend", "dist"]

# 编译延迟（毫秒）
delay = 1000

# 编译错误时是否停止
stop_on_error = true
```

### 传统开发方式

```bash
# 直接运行（不推荐 - 无版本信息）
go run main.go

# 运行测试
go test ./...

# 格式化代码
go fmt ./...

# 静态检查
go vet ./...
```

### 开发技巧

1. **使用热重载**：`make dev` 启动后，专注于代码编写，无需手动重启
2. **查看日志**：热重载模式下日志有颜色区分，更易阅读
3. **错误处理**：编译错误会显示在控制台，修复后自动重新编译
4. **配置更新**：修改 `.env` 或配置文件也会触发重启

## 版本管理

### 升级版本

只需修改根目录的 `VERSION` 文件：

```bash
# 编辑 VERSION 文件
echo "v1.1.0" > ../VERSION

# 重新构建即可
make build
```

所有构建产物会自动包含新版本号，无需修改代码！

### 查看版本信息

```bash
# 查看项目版本信息
make info

# 启动服务器后查看版本
curl http://localhost:3000/health | jq '.version'

# 输出示例：
# {
#   "version": "v1.0.0",
#   "buildTime": "2025-01-15_10:30:45_UTC",
#   "gitCommit": "abc1234"
# }
```

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

---

**注意**: 这是 Claude Proxy 的 Go 语言重写版本，完整实现了原 TypeScript 版本的所有功能，并提供了更好的性能和部署体验。
