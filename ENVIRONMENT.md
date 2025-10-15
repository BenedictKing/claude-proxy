# 环境变量配置指南

## 概述

本项目使用分层的环境变量配置系统，支持开发、生产等不同环境的端口和API配置。前端通过 Vite 的环境变量系统动态连接后端服务。

## 配置文件结构

```
claude-proxy/
├── frontend/
│   ├── .env                    # 前端默认配置
│   ├── .env.development        # 开发环境配置
│   ├── .env.production         # 生产环境配置
│   └── vite.config.ts          # Vite 构建配置
└── backend-go/
    └── .env                    # Go 后端环境配置
```

## 环境变量详解

### 前端配置变量

#### 通用变量（所有环境）
- `VITE_API_BASE_PATH` - API 基础路径，默认 `/api`
- `VITE_PROXY_API_PATH` - 代理 API 路径，默认 `/v1`
- `VITE_APP_ENV` - 应用环境标识

#### 开发环境专用变量
- `VITE_BACKEND_URL` - 完整后端URL，默认 `http://localhost:3000`
- `VITE_FRONTEND_PORT` - 前端开发服务器端口，默认 `5173`

### 后端配置 (Go)

后端支持以下环境变量：

```bash
# 服务器配置
PORT=3000                              # 服务器端口

# 运行环境
ENV=production                         # 运行环境: development | production
# NODE_ENV=production                  # 向后兼容 (已弃用，请使用 ENV)

# 访问控制
PROXY_ACCESS_KEY=your-secret-key       # 访问密钥 (必须设置!)

# Web UI
ENABLE_WEB_UI=true                     # 是否启用 Web 管理界面

# 日志配置
LOG_LEVEL=info                         # 日志级别: debug | info | warn | error
ENABLE_REQUEST_LOGS=true               # 是否记录请求日志
ENABLE_RESPONSE_LOGS=false             # 是否记录响应日志
```

#### 日志等级说明

项目采用标准的四级日志系统，等级从高到低：

| 等级 | 值 | 说明 | 典型场景 |
|------|----|----|---------|
| `error` | 0 | 错误日志（最高优先级） | 致命错误、异常情况 |
| `warn` | 1 | 警告日志 | 非致命问题、降级操作 |
| `info` | 2 | 信息日志（默认） | 常规操作、状态变化 |
| `debug` | 3 | 调试日志（最低优先级） | 详细调试信息、敏感数据 |

**等级控制规则**：设置 `LOG_LEVEL=info` 时，会输出 `error`、`warn`、`info` 级别的日志，但不输出 `debug` 级别。

#### 日志控制机制

项目使用三种机制来控制日志输出：

##### 1. 显式等级控制（推荐）
```go
// 代码示例
if envCfg.ShouldLog("info") {
    log.Printf("🎯 使用上游: %s", upstream.Name)
}
```
- **适用场景**：通用信息输出
- **控制变量**：`LOG_LEVEL`

##### 2. 开关控制（分类日志）
```go
// 代码示例
if envCfg.EnableRequestLogs {
    log.Printf("📥 收到请求: %s", c.Request.URL.Path)
}
```
- **适用场景**：请求/响应类日志
- **控制变量**：`ENABLE_REQUEST_LOGS`、`ENABLE_RESPONSE_LOGS`

##### 3. 环境门控（开发专用）
```go
// 代码示例
if envCfg.EnableRequestLogs && envCfg.IsDevelopment() {
    log.Printf("📄 原始请求体:\n%s", formattedBody)
}
```
- **适用场景**：敏感/详细信息（请求体、请求头等）
- **控制变量**：`ENV=development`

#### 日志输出对照表

| 日志内容 | 控制条件 | 等效等级 | 生产环境 | 开发环境 |
|---------|---------|---------|---------|---------|
| `📄 原始请求体` | `EnableRequestLogs && IsDevelopment()` | debug | ❌ 不输出 | ✅ 输出 |
| `📋 实际请求头` | `EnableRequestLogs && IsDevelopment()` | debug | ❌ 不输出 | ✅ 输出 |
| `📦 响应体` | `EnableResponseLogs && IsDevelopment()` | debug | ❌ 不输出 | ✅ 输出 |
| `📥 收到请求` | `EnableRequestLogs` | info | ⚙️ 可配置 | ✅ 输出 |
| `⏱️ 响应完成` | `EnableResponseLogs` | info | ⚙️ 可配置 | ✅ 输出 |
| `🎯 使用上游` | `ShouldLog("info")` | info | ⚙️ 可配置 | ✅ 输出 |
| `ℹ️ 客户端中断` | `ShouldLog("info")` | info | ⚙️ 可配置 | ✅ 输出 |
| `⚠️ API密钥失败` | 无条件 | warn | ✅ 输出 | ✅ 输出 |
| `💥 所有密钥失败` | 无条件 | error | ✅ 输出 | ✅ 输出 |

#### 配置组合效果

**开发环境 + 完整日志**：
```env
ENV=development
LOG_LEVEL=debug
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true
```
- ✅ 输出所有日志，包括完整请求体、请求头、响应体
- ✅ 适合本地开发调试
- ⚠️ 可能包含敏感信息，不要在生产环境使用

**生产环境 + 最小日志**：
```env
ENV=production
LOG_LEVEL=warn
ENABLE_REQUEST_LOGS=false
ENABLE_RESPONSE_LOGS=false
```
- ✅ 只输出警告和错误
- ✅ 最小性能影响
- ✅ 不输出敏感信息
- ⚠️ 排查问题时信息较少

**生产环境 + 适度日志**（推荐）：
```env
ENV=production
LOG_LEVEL=info
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=false
```
- ✅ 输出基本请求信息（如 `📥 收到请求`）
- ✅ 不输出详细内容（请求体、响应体等）
- ✅ 平衡了可观测性和性能
- ✅ 不泄露敏感信息

**调试模式**：
```env
ENV=development
LOG_LEVEL=debug
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true
```
- ✅ 最详细的日志输出
- ✅ 查看完整的请求/响应数据流
- ⚠️ 仅用于故障排查，排查完成后应恢复正常配置

#### 性能影响说明

| 配置 | CPU 影响 | 内存影响 | 磁盘 I/O |
|-----|---------|---------|----------|
| `LOG_LEVEL=error` | 极低 | 极低 | 极低 |
| `LOG_LEVEL=warn` | 极低 | 极低 | 低 |
| `LOG_LEVEL=info` | 低 | 低 | 中 |
| `LOG_LEVEL=debug` | 中 | 中 | 高 |
| `ENABLE_REQUEST_LOGS=true` | 低 | 低 | 中 |
| `ENABLE_RESPONSE_LOGS=true` | 低-中 | 中-高 | 高 |

**生产环境建议**：
- 日常运行：`LOG_LEVEL=info`，`ENABLE_RESPONSE_LOGS=false`
- 故障排查：临时开启 `ENABLE_RESPONSE_LOGS=true`
- 高负载场景：使用 `LOG_LEVEL=warn` 减少开销

### ENV 变量影响

| 配置项 | `development` | `production` |
|--------|---------------|--------------|
| Gin 模式 | DebugMode | ReleaseMode |
| `/admin/dev/info` | ✅ 开启 | ❌ 关闭 |
| CORS | 宽松（localhost自动允许）| 严格 |
| 日志 | 详细 | 最小 |

## 配置文件内容

### frontend/.env
```env
# 前端环境配置

# 后端API服务器配置
VITE_BACKEND_URL=http://localhost:3000

# 前端开发服务器配置
VITE_FRONTEND_PORT=5173

# API路径配置
VITE_API_BASE_PATH=/api
VITE_PROXY_API_PATH=/v1
```

### frontend/.env.development
```env
# 开发环境配置

# 后端API服务器配置
VITE_BACKEND_URL=http://localhost:3000

# 前端开发服务器配置
VITE_FRONTEND_PORT=5173

# API路径配置
VITE_API_BASE_PATH=/api
VITE_PROXY_API_PATH=/v1

# 开发模式标识
VITE_APP_ENV=development
```

### frontend/.env.production
```env
# 生产环境配置
VITE_API_BASE_PATH=/api
VITE_PROXY_API_PATH=/v1
VITE_APP_ENV=production
```

### backend-go/.env.example
```env
# 服务器配置
PORT=3000

# 运行环境
ENV=production

# 访问控制 (必须修改!)
PROXY_ACCESS_KEY=your-super-strong-secret-key

# Web UI
ENABLE_WEB_UI=true

# 日志配置
LOG_LEVEL=info
ENABLE_REQUEST_LOGS=false
ENABLE_RESPONSE_LOGS=false
```

## API 基础URL 生成逻辑

前端通过以下逻辑动态确定API基础URL：

```typescript
const getApiBase = () => {
  // 生产环境：直接使用当前域名
  if (import.meta.env.PROD) {
    return '/api'
  }

  // 开发环境：使用配置的后端URL
  const backendUrl = import.meta.env.VITE_BACKEND_URL
  const apiBasePath = import.meta.env.VITE_API_BASE_PATH || '/api'

  if (backendUrl) {
    return `${backendUrl}${apiBasePath}`
  }

  // 回退到默认配置
  return '/api'
}
```

## 开发服务器代理配置

Vite 开发服务器自动配置代理，将前端请求转发到后端：

```typescript
// vite.config.ts
server: {
  port: Number(env.VITE_FRONTEND_PORT) || 5173,
  proxy: {
    '/api': {
      target: backendUrl,
      changeOrigin: true,
      secure: false
    }
  }
}
```

## 环境切换

### 开发环境启动
```bash
# 方式 1: 同时启动前后端 (推荐)
bun run dev

# 方式 2: 分别启动
# 启动后端 (端口 3000)
cd backend-go && make dev

# 启动前端 (端口 5173)
cd frontend && bun run dev
```

### 生产环境构建
```bash
# 构建完整项目
bun run build

# 启动生产服务器
bun run start

# 或使用 Docker
docker-compose up -d
```

## 端口配置优先级

1. **环境变量** - 从 `.env.*` 文件读取
2. **默认值** - 代码中定义的回退值
3. **系统环境变量** - `PORT` （后端）

## 常见配置场景

### 场景1：更改后端端口到 8080
```env
# backend-go/.env
PORT=8080

# frontend/.env.development
VITE_BACKEND_URL=http://localhost:8080
```

### 场景2：使用远程后端服务
```env
# frontend/.env.development
VITE_BACKEND_URL=https://api.example.com
```

### 场景3：自定义前端开发端口
```env
# frontend/.env.development
VITE_FRONTEND_PORT=3000
```

### 场景4：生产环境配置

#### 4.1 高性能模式（最小日志）
```env
# backend-go/.env
ENV=production
PORT=3000
PROXY_ACCESS_KEY=$(openssl rand -base64 32)

# 最小日志输出
LOG_LEVEL=warn
ENABLE_REQUEST_LOGS=false
ENABLE_RESPONSE_LOGS=false

ENABLE_WEB_UI=true
```
- ✅ 适合：高并发场景、性能敏感应用
- ✅ 特点：最低资源消耗，只记录警告和错误
- ⚠️ 注意：排查问题时信息较少

#### 4.2 标准模式（推荐）
```env
# backend-go/.env
ENV=production
PORT=3000
PROXY_ACCESS_KEY=$(openssl rand -base64 32)

# 适度日志输出
LOG_LEVEL=info
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=false

ENABLE_WEB_UI=true
```
- ✅ 适合：大多数生产环境
- ✅ 特点：平衡可观测性和性能，不泄露敏感信息
- ✅ 优势：足够的信息用于监控和问题排查

#### 4.3 调试模式（临时排查）
```env
# backend-go/.env
ENV=production
PORT=3000
PROXY_ACCESS_KEY=$(openssl rand -base64 32)

# 详细日志输出（临时使用）
LOG_LEVEL=info
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true

ENABLE_WEB_UI=true
```
- ⚠️ 适合：故障排查时临时启用
- ⚠️ 注意：会输出完整响应内容，增加日志量
- 🔄 建议：问题解决后立即恢复标准配置

#### 4.4 开发环境配置
```env
# backend-go/.env
ENV=development
PORT=3000
PROXY_ACCESS_KEY=dev-test-key

# 完整日志输出
LOG_LEVEL=debug
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true

ENABLE_WEB_UI=true
```
- ✅ 适合：本地开发和调试
- ✅ 特点：输出所有详细信息，包括请求体、响应体
- ⚠️ 警告：包含敏感信息，仅限开发环境使用

## 调试配置

开发环境下，前端会在控制台输出当前API配置：

```javascript
console.log('🔗 API Configuration:', {
  API_BASE: '/api',
  BACKEND_URL: 'http://localhost:3000',
  IS_DEV: true,
  IS_PROD: false
})
```

## 注意事项

1. **变量前缀**：前端环境变量必须以 `VITE_` 开头才能在浏览器中访问
2. **构建时解析**：Vite 在构建时静态替换环境变量，运行时无法修改
3. **生产环境**：生产环境不需要指定后端URL，通过反向代理或一体化部署处理
4. **类型安全**：使用 `Number()` 转换端口号确保类型正确
5. **密钥安全**：切勿在版本控制中提交 `.env` 文件，使用 `.env.example` 作为模板

## 安全最佳实践

### 生成强密钥
```bash
# 生成随机密钥
PROXY_ACCESS_KEY=$(openssl rand -base64 32)
echo "生成的密钥: $PROXY_ACCESS_KEY"
```

### 生产环境配置清单
```bash
# 1. 强密钥 (必须!)
PROXY_ACCESS_KEY=<strong-random-key>

# 2. 生产模式
ENV=production

# 3. 适度日志（推荐）
LOG_LEVEL=info
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=false

# 4. 启用 Web UI (可选)
ENABLE_WEB_UI=true
```

### 日志安全建议

#### 敏感信息保护
项目已自动对以下信息进行脱敏处理：
- ✅ API密钥：只显示前4位和后4位（如 `sk-a***b`）
- ✅ Authorization 请求头：完全隐藏
- ✅ x-api-key 请求头：完全隐藏

#### 推荐配置
```bash
# 生产环境：不输出详细内容
ENV=production
ENABLE_REQUEST_LOGS=true    # ✅ 基本请求信息
ENABLE_RESPONSE_LOGS=false  # ❌ 不输出响应体

# 开发环境：可以输出详细内容
ENV=development
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true
```

#### 日志存储注意事项
1. **日志轮转**：定期清理旧日志，避免磁盘空间耗尽
2. **访问控制**：限制日志文件的访问权限
   ```bash
   chmod 600 /var/log/claude-proxy/*.log
   ```
3. **敏感数据**：即使有脱敏，也应定期审查日志内容
4. **合规要求**：根据数据保护法规（GDPR、CCPA等）管理日志

#### 故障排查时的安全做法
```bash
# ✅ 推荐：临时开启详细日志，排查完成后恢复
ENABLE_RESPONSE_LOGS=true  # 临时启用

# 🔄 排查完成后立即恢复
ENABLE_RESPONSE_LOGS=false

# ❌ 不推荐：在生产环境长期开启 debug 级别
LOG_LEVEL=debug  # 可能泄露敏感信息
```

## 故障排除

### 问题：前端无法连接后端
1. 检查后端是否在正确端口启动
   ```bash
   curl http://localhost:3000/health
   ```
2. 确认 `VITE_BACKEND_URL` 配置正确
3. 查看浏览器控制台的API配置输出

### 问题：构建后API请求失败
1. 确认生产环境配置了正确的反向代理或使用一体化部署
2. 检查 `VITE_API_BASE_PATH` 设置
3. 验证后端API路径匹配

### 问题：环境变量不生效
1. 确认变量名以 `VITE_` 开头 (前端) 或在后端代码中正确读取
2. 重启开发服务器
3. 检查 `.env` 文件语法正确 (无多余空格、引号等)

### 问题：认证失败
```bash
# 检查密钥设置
echo $PROXY_ACCESS_KEY

# 测试认证
curl -H "x-api-key: $PROXY_ACCESS_KEY" http://localhost:3000/health
```

### 问题：日志输出过多或过少

#### 日志过多（影响性能）
**症状**：日志文件快速增长，磁盘空间不足，或系统性能下降

**解决方案**：
1. 降低日志等级
   ```env
   LOG_LEVEL=warn  # 从 info 或 debug 降级
   ```

2. 关闭详细日志
   ```env
   ENABLE_REQUEST_LOGS=false
   ENABLE_RESPONSE_LOGS=false
   ```

3. 使用日志轮转（推荐）
   ```bash
   # 使用 systemd 日志轮转
   journalctl --vacuum-time=7d

   # 或使用 logrotate
   # /etc/logrotate.d/claude-proxy
   /var/log/claude-proxy/*.log {
       daily
       rotate 7
       compress
       delaycompress
       missingok
       notifempty
   }
   ```

#### 日志过少（排查困难）
**症状**：出现问题时没有足够的日志信息

**解决方案**：
1. 提高日志等级
   ```env
   LOG_LEVEL=info  # 从 warn 提升
   ```

2. 临时开启详细日志
   ```env
   ENABLE_REQUEST_LOGS=true
   ENABLE_RESPONSE_LOGS=true
   ```

3. 使用开发模式（仅限测试环境）
   ```env
   ENV=development
   LOG_LEVEL=debug
   ```

#### 看不到请求体/响应体
**症状**：日志中没有详细的请求/响应内容

**原因**：详细内容只在开发环境 (`ENV=development`) 输出

**解决方案**：
```env
# 方案1：临时切换到开发模式（不推荐生产环境）
ENV=development
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true

# 方案2：查看是否开启了日志开关
ENABLE_REQUEST_LOGS=true   # 必须为 true
ENABLE_RESPONSE_LOGS=true  # 必须为 true

# 方案3：检查当前环境
echo $ENV  # 必须是 development
```

**安全提醒**：
- ⚠️ 请求体和响应体可能包含敏感信息（API密钥、用户数据等）
- ⚠️ 生产环境建议关闭 `ENABLE_RESPONSE_LOGS`
- ⚠️ 排查完成后立即恢复安全配置

### 问题：日志格式混乱
**症状**：日志输出格式不统一或难以阅读

**检查项**：
1. 确认是否混用了多个日志系统
2. 检查是否有第三方库输出了额外日志
3. 验证环境变量是否正确加载
   ```bash
   # 打印当前日志配置
   curl -H "x-api-key: $PROXY_ACCESS_KEY" http://localhost:3000/health
   ```

## 文档资源

- **项目架构**: 参见 [ARCHITECTURE.md](ARCHITECTURE.md)
- **快速开始**: 参见 [README.md](README.md)
- **贡献指南**: 参见 [CONTRIBUTING.md](CONTRIBUTING.md)
