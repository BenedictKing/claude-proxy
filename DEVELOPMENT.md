# 开发指南

本文档为开发者提供开发环境配置、工作流程、调试技巧和最佳实践。

> 📚 **相关文档**
> - 架构设计和技术选型: [ARCHITECTURE.md](ARCHITECTURE.md)
> - 环境变量配置: [ENVIRONMENT.md](ENVIRONMENT.md)
> - 贡献规范: [CONTRIBUTING.md](CONTRIBUTING.md)

---

## 🎯 推荐开发方式

| 开发方式 | 启动速度 | 热重载 | 适用场景 |
|---------|---------|-------|---------|
| **🚀 根目录 Make 命令** | ⚡ 极快 | ✅ 支持 | **推荐：日常开发** |
| **🔧 backend-go Make** | ⚡ 极快 | ✅ 支持 | Go 后端专项开发 |
| **🐳 Docker** | 🔄 中等 | ❌ 需重启 | 生产环境测试 |

---

## 方式一：🚀 根目录开发（推荐）

**适合日常开发，自动处理前端构建和后端启动**

### 快速开始

```bash
# 在项目根目录执行

# 查看所有可用命令
make help

# 开发模式（后端热重载）
make dev

# 构建前端并运行后端
make run

# 前端独立开发服务器
make frontend-dev

# 完整构建（前端 + 后端）
make build

# 清理构建产物
make clean
```

### 开发环境要求

- Go 1.22+
- Make（构建工具）
- Bun（前端构建）

---

## 方式二：🔧 backend-go 目录开发

**适合专注 Go 后端开发和调试**

```bash
cd backend-go

# 查看所有可用命令
make help

# 开发模式（支持热重载）
make dev

# 运行测试
make test

# 测试 + 覆盖率
make test-cover

# 构建当前平台二进制
make build-current

# 构建并运行
make build-run
```

---

## 🪟 Windows 环境配置

Windows 用户在开发本项目时可能遇到一些工具缺失的问题，以下是常见问题的解决方案。

### 问题 1: 没有 `make` 命令

Windows 默认不包含 `make` 工具，有以下几种解决方案：

#### 方案 A: 安装 Make (推荐)

```powershell
# 使用 Chocolatey (推荐)
choco install make

# 或使用 Scoop
scoop install make

# 或使用 winget
winget install GnuWin32.Make
```

#### 方案 B: 直接使用 Go 命令 (无需安装 make)

```powershell
cd backend-go

# 替代 make dev (需要先安装 air: go install github.com/air-verse/air@latest)
air

# 替代 make build
go build -o claude-proxy.exe .

# 替代 make run
go run main.go

# 替代 make test
go test ./...

# 替代 make fmt
go fmt ./...
```

### 问题 2: 没有 `vite` 命令

这是因为前端依赖未安装，`vite` 是项目的开发依赖。

#### 解决步骤

```powershell
cd frontend

# 使用 bun 安装依赖 (推荐)
bun install

# 或使用 npm
npm install

# 安装完成后运行开发服务器
bun run dev    # 或 npm run dev
```

### Windows 完整开发环境配置

#### 1. 安装包管理器 (可选但推荐)

```powershell
# 安装 Scoop (无需管理员权限)
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
irm get.scoop.sh | iex

# 或安装 Chocolatey (需要管理员权限)
Set-ExecutionPolicy Bypass -Scope Process -Force
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
```

#### 2. 安装开发工具

```powershell
# 使用 Scoop
scoop install git go bun make

# 或使用 Chocolatey
choco install git golang bun make -y
```

#### 3. 验证安装

```powershell
go version      # 应显示 go1.22+
bun --version   # 应显示版本号
make --version  # 应显示 GNU Make 版本
git --version   # 应显示 git 版本
```

### Windows 快速启动流程

```powershell
# 1. 克隆项目
git clone https://github.com/BenedictKing/claude-proxy
cd claude-proxy

# 2. 安装前端依赖
cd frontend
bun install    # 或 npm install

# 3. 配置环境变量
cd ../backend-go
copy .env.example .env
# 编辑 .env 文件设置 PROXY_ACCESS_KEY

# 4. 启动后端 (选择以下方式之一)

# 方式 A: 使用 make (如果已安装)
make dev

# 方式 B: 直接使用 Go
go run main.go

# 5. 另开终端，启动前端开发服务器 (如需单独开发前端)
cd frontend
bun run dev
```

### Windows 常见问题

#### PowerShell 执行策略限制

```powershell
# 如果遇到脚本执行限制，以管理员身份运行
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

#### 端口被占用

```powershell
# 查看端口占用
netstat -ano | findstr :3000

# 终止占用进程 (替换 PID 为实际进程 ID)
taskkill /PID <PID> /F
```

#### 路径包含空格

确保项目路径不包含空格和中文字符，推荐使用如 `C:\projects\claude-proxy` 这样的路径。

---

## 方式三：🐳 Docker 开发

**适合测试生产环境或隔离开发**

```bash
# 使用 docker-compose 启动
docker-compose up -d

# 查看日志
docker-compose logs -f

# 重新构建并启动
docker-compose up -d --build

# 停止服务
docker-compose down
```

---

## 前端独立开发

前端使用 Vue 3 + Vuetify + Vite，可独立开发：

```bash
cd frontend

# 安装依赖
bun install

# 启动开发服务器（端口 5173）
bun run dev

# 构建生产版本
bun run build

# 预览构建结果
bun run preview
```

**开发服务器代理配置**:

Vite 开发服务器会自动将 `/api` 和 `/v1` 请求代理到后端（默认 `http://localhost:3000`）：

```typescript
// frontend/vite.config.ts
server: {
  port: 5173,
  proxy: {
    '/api': { target: backendUrl, changeOrigin: true },
    '/v1': { target: backendUrl, changeOrigin: true }
  }
}
```

**环境变量**:
- `VITE_PROXY_TARGET` - 后端代理目标（默认 `http://localhost:3000`）
- `VITE_FRONTEND_PORT` - 前端开发服务器端口（默认 `5173`）

---

## 文件监听策略

### 配置文件（无需重启）

- `backend-go/.config/config.json` - 主配置文件

**变化时**: 自动重载配置，服务保持运行

### 环境变量文件（需要重启）

- `backend-go/.env` - 环境变量配置

**变化时**: 需要重启服务以加载新的环境变量

## 开发模式特性

### 1. 热重载开发 (`make dev`)

- ✅ Go 源码变化自动重新编译
- ✅ 配置文件变化自动重载（不重启）
- ✅ 优雅关闭处理
- ✅ 详细的开发日志

### 2. 配置热重载

- ✅ 配置文件变化自动重载
- ✅ 无需重启服务器
- ✅ 自动备份配置（最多 10 个）

---

## 🎯 代码质量标准

> 📚 完整的编码规范和设计模式请参考 [ARCHITECTURE.md](ARCHITECTURE.md)

### 编程原则

项目严格遵循以下软件工程原则：

#### 1. KISS 原则 (Keep It Simple, Stupid)
- 追求代码和设计的极致简洁
- 优先选择最直观的解决方案
- 使用正则表达式替代复杂的字符串处理逻辑

#### 2. DRY 原则 (Don't Repeat Yourself)  
- 消除重复代码，提取共享函数
- 统一相似功能的实现方式
- 例：`normalizeClaudeRole` 函数的提取和共享

#### 3. YAGNI 原则 (You Aren't Gonna Need It)
- 仅实现当前明确所需的功能
- 删除未使用的代码和依赖
- 避免过度设计和未来特性预留

#### 4. 函数式编程优先
- 使用 `map`、`reduce`、`filter` 等函数式方法
- 优先使用不可变数据操作
- 例：命令行参数解析使用 `reduce()` 替代传统循环

### 代码优化检查清单

在提交代码前，请确保：

- [ ] Go 代码通过 `make lint` 检查
- [ ] 通过 `make test` 测试
- [ ] 前端代码通过 `bun run build` 构建验证

### Go 代码规范

- 使用 `gofmt` 格式化代码
- 遵循 Go 官方代码规范
- 错误处理要完整
- 适当添加注释

### 命名规范

- **文件名**: snake_case (例: `config_manager.go`)
- **函数名**: PascalCase 导出 / camelCase 私有 (例: `GetProvider` / `parseRequest`)
- **常量名**: PascalCase 或 SCREAMING_SNAKE_CASE
- **接口名**: PascalCase，通常以 -er 结尾 (例: `Provider`)

### 错误处理

```go
result, err := riskyOperation()
if err != nil {
    log.Printf("Operation failed: %v", err)
    return fmt.Errorf("specific error: %w", err)
}
```

### 日志规范

使用 Go 标准日志或结构化日志：

```go
log.Printf("🎯 使用上游: %s", upstream.Name)
log.Printf("⚠️ 警告: %s", message)
log.Printf("💥 错误: %v", err)
```

## 🧪 测试策略

### 手动测试

#### 1. 基础功能测试

```bash
# 测试健康检查
curl http://localhost:3000/health

# 测试基础对话
curl -X POST http://localhost:3000/v1/messages \
  -H "x-api-key: test-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-3-5-sonnet-20241022","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}'

# 测试流式响应
curl -X POST http://localhost:3000/v1/messages \
  -H "x-api-key: test-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-3-5-sonnet-20241022","stream":true,"max_tokens":100,"messages":[{"role":"user","content":"Count to 10"}]}'
```

#### 2. 负载均衡测试

```bash
# 添加多个 API 密钥
bun run config key test-upstream add key1 key2 key3

# 设置轮询策略
bun run config balance round-robin

# 发送多个请求观察密钥轮换
for i in {1..5}; do
  curl -X POST http://localhost:3000/v1/messages \
    -H "x-api-key: test-key" \
    -H "Content-Type: application/json" \
    -d '{"model":"claude-3-5-sonnet-20241022","max_tokens":10,"messages":[{"role":"user","content":"Test '$i'"}]}'
done
```

### 集成测试

#### Claude Code 集成测试

1. 配置 Claude Code 使用本地代理
2. 测试基础对话功能
3. 测试工具调用功能
4. 测试流式响应
5. 验证错误处理

#### 压力测试

```bash
# 使用 ab (Apache Bench) 进行压力测试
ab -n 100 -c 10 -p request.json -T application/json \
  -H "x-api-key: test-key" \
  http://localhost:3000/v1/messages
```

## 🔧 调试技巧

### 1. 日志分析

```bash
# 实时查看日志
tail -f server.log

# 过滤错误日志
grep -i "error" server.log

# 分析请求模式
grep -o "POST /v1/messages" server.log | wc -l
```

### 2. 配置调试

```bash
# 验证配置文件
cat config.json | jq .

# 检查环境变量
env | grep -E "(PORT|LOG_LEVEL)"
```

### 3. 网络调试

```bash
# 测试上游连接
curl -I https://api.openai.com

# 检查 DNS 解析
nslookup api.openai.com

# 测试端口连通性
telnet localhost 3000
```

## 🚀 部署指南

### 开发环境部署

```bash
# 1. 克隆项目
git clone https://github.com/BenedictKing/claude-proxy
cd claude-proxy

# 2. 配置环境变量
cp backend-go/.env.example backend-go/.env
vim backend-go/.env

# 3. 启动开发服务器
make dev
```

### 生产环境部署

```bash
# 1. 构建生产版本
make build

# 2. 配置环境变量
cp backend-go/.env.example backend-go/.env
# 修改 ENV=production 和 PROXY_ACCESS_KEY

# 3. 运行服务
./backend-go/dist/claude-proxy
```

### Docker 部署

```bash
# 使用预构建镜像
docker-compose up -d

# 或本地构建
docker-compose build
docker-compose up -d
```

## 🤝 贡献与发布

### 贡献指南

欢迎提交 Issue 和 Pull Request！

> 📚 详细的贡献规范和提交指南请参考 [CONTRIBUTING.md](CONTRIBUTING.md)

### 版本发布

> 📚 维护者版本发布流程请参考 [RELEASE.md](RELEASE.md)
