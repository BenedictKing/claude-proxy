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
```env
# backend-go/.env
ENV=production
PORT=3000
PROXY_ACCESS_KEY=$(openssl rand -base64 32)
LOG_LEVEL=warn
ENABLE_REQUEST_LOGS=false
ENABLE_RESPONSE_LOGS=false
ENABLE_WEB_UI=true
```

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

# 3. 最小日志
LOG_LEVEL=warn
ENABLE_REQUEST_LOGS=false
ENABLE_RESPONSE_LOGS=false

# 4. 启用 Web UI (可选)
ENABLE_WEB_UI=true
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

## 文档资源

- **项目架构**: 参见 [ARCHITECTURE.md](ARCHITECTURE.md)
- **快速开始**: 参见 [README.md](README.md)
- **贡献指南**: 参见 [CONTRIBUTING.md](CONTRIBUTING.md)
