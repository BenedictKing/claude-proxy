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
└── backend/
    └── src/server.ts           # 后端服务器启动配置
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

### 后端配置

后端端口通过以下方式配置：
```typescript
const PORT = process.env.PORT || 3000
```

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
# 启动后端（端口 3000）
cd backend && bun run dev

# 启动前端（端口 5173）
cd frontend && bun run dev
```

### 生产环境构建
```bash
# 构建前端静态资源
cd frontend && bun run build

# 启动后端服务器
cd backend && bun run start
```

## 端口配置优先级

1. **环境变量** - 从 `.env.*` 文件读取
2. **默认值** - 代码中定义的回退值
3. **系统环境变量** - `process.env.PORT` （后端）

## 常见配置场景

### 场景1：更改后端端口到 8080
```env
# frontend/.env.development
VITE_BACKEND_URL=http://localhost:8080
```

```bash
# backend/.env 或直接设置环境变量
PORT=8080
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
3. **生产环境**：生产环境不需要指定后端URL，通过反向代理处理
4. **类型安全**：使用 `Number()` 转换端口号确保类型正确

## 故障排除

### 问题：前端无法连接后端
1. 检查后端是否在正确端口启动
2. 确认 `VITE_BACKEND_URL` 配置正确
3. 查看浏览器控制台的API配置输出

### 问题：构建后API请求失败
1. 确认生产环境配置了正确的反向代理
2. 检查 `VITE_API_BASE_PATH` 设置
3. 验证后端API路径匹配

### 问题：环境变量不生效
1. 确认变量名以 `VITE_` 开头
2. 重启开发服务器
3. 检查 `.env` 文件语法正确