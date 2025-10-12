# 升级到 v2.0.1 指南

> 从 v2.0.0-go 升级到 v2.0.1 的完整指南

## 🎯 升级概述

v2.0.1 主要修复了前端资源加载问题和性能优化，强烈建议所有 v2.0.0 用户升级。

### 主要改进

- ✅ **修复前端无法加载** - 解决 Vite base 路径配置问题
- ✅ **性能提升 142 倍** - 智能缓存机制，开发时启动仅需 0.07 秒
- ✅ **API 路由修复** - 前后端路由完全匹配
- ✅ **ENV 标准化** - 更通用的环境变量命名

## 📋 升级步骤

### 1. 备份配置（可选但推荐）

```bash
# 备份配置文件
cp backend-go/.config/config.json backend-go/.config/config.json.backup
cp backend-go/.env backend-go/.env.backup
```

### 2. 更新代码

```bash
# 拉取最新代码
git pull origin main

# 或下载最新 Release
# https://github.com/yourusername/claude-proxy/releases/tag/v2.0.1
```

### 3. 更新环境变量（推荐）

编辑 `backend-go/.env`：

```diff
# 服务器配置
PORT=3001

- NODE_ENV=development
+ # 运行环境: development | production
+ ENV=development

# ... 其他配置保持不变
```

**注意**：旧的 `NODE_ENV` 仍然有效（向后兼容），但建议迁移到 `ENV`。

### 4. 重新构建

#### 方式 1: 使用 Makefile（推荐）

```bash
# 清除旧的构建缓存
make clean

# 重新构建前端和后端
make build-frontend-internal

# 启动服务器（会自动使用缓存）
make run
```

#### 方式 2: 手动构建

```bash
# 构建前端
cd frontend
npm install
npm run build
cd ..

# 构建 Go 后端
cd backend-go
./build.sh

# 运行
./dist/claude-proxy-darwin-arm64  # 根据你的平台选择
```

### 5. 验证升级

```bash
# 检查版本信息
make info

# 应该显示:
# Version: v2.0.1
# Build Time: 2025-10-12_XX:XX:XX_UTC
# Git Commit: <commit-hash>

# 测试服务器
curl http://localhost:3001/health | jq '.version'

# 访问 Web 界面
# http://localhost:3001/?key=your-access-key
```

## 🔍 变更详情

### 前端资源修复

**问题**：v2.0.0 中前端资源无法加载，显示 MIME 类型错误

**修复内容**：
1. Vite 配置添加 `base: '/'`
2. Go 后端 NoRoute 处理器优化
3. 添加完整的 Content-Type 检测
4. 添加 favicon 支持

**影响**：前端界面现在能正常加载了

### 性能优化

**智能缓存**：
- 首次构建：~10 秒
- 无变更重启：**0.07 秒**（提升 142 倍）
- 有变更重新构建：~8.5 秒

**使用方法**：
```bash
# 第一次运行（会构建前端）
make run

# 修改后端代码后重启（跳过前端构建）
Ctrl+C
make run  # 0.07 秒启动！

# 修改前端代码后重启（自动重新构建）
Ctrl+C
make run  # 自动检测到变更，重新构建
```

### API 路由变更

**变更**：
- `/api/upstreams` → `/api/channels`
- 新增 `/api/loadbalance` (PUT)
- 新增 `/api/ping/:id` (GET)
- 新增 `/api/ping` (GET)

**影响**：前端 API 调用现在能正常工作

**迁移**：无需修改，新路由自动生效

### 环境变量变更

**变更**：`NODE_ENV` → `ENV`

**向后兼容**：
```go
// 优先读取 ENV，回退到 NODE_ENV
env := getEnv("ENV", "")
if env == "" {
    env = getEnv("NODE_ENV", "development")
}
```

**迁移建议**：
- ✅ 立即迁移：修改 `.env` 文件使用 `ENV`
- ✅ 延迟迁移：保持使用 `NODE_ENV` 也能正常工作
- ⚠️ 未来版本：v3.0.0 可能移除 `NODE_ENV` 支持

## 🆕 新功能使用

### 1. 智能缓存

```bash
# 查看缓存状态
ls -la backend-go/frontend/dist/.build-marker

# 强制重新构建
make build-frontend

# 清除缓存
make clean
```

### 2. ENV 变量详细配置

```bash
# 开发环境（详细日志、开发端点、宽松 CORS）
ENV=development

# 生产环境（高性能、严格安全）
ENV=production
```

**影响说明**：

| 配置项 | development | production |
|--------|-------------|------------|
| Gin 模式 | DebugMode | ReleaseMode |
| `/admin/dev/info` | ✅ 开启 | ❌ 关闭 |
| CORS | 宽松（localhost自动允许）| 严格 |
| 日志 | 详细 | 最小 |

### 3. favicon 支持

现在自动包含 favicon，浏览器标签页会显示 "C" 图标。

自定义 favicon：
```bash
# 替换 frontend/public/favicon.svg
# 重新构建
make build-frontend-internal
```

## ❌ 破坏性变更

**无破坏性变更**。所有 v2.0.0 配置和 API 完全兼容。

## 🐛 已知问题

### 问题：前端资源 404（已修复）

**症状**：访问 Web 界面时资源加载失败

**原因**：Vite base 路径配置错误

**状态**：✅ v2.0.1 已修复

### 问题：favicon.ico 返回 HTML（已修复）

**症状**：浏览器控制台显示 MIME 类型错误

**原因**：NoRoute 处理器逻辑问题

**状态**：✅ v2.0.1 已修复

## 📞 支持

遇到升级问题？

1. **查看日志**：`make run` 启动服务查看错误信息
2. **清除缓存**：`make clean && make build-frontend-internal`
3. **验证版本**：`make info` 确认版本为 v2.0.1
4. **提交 Issue**：https://github.com/yourusername/claude-proxy/issues

## 🔄 回滚到 v2.0.0

如果升级遇到问题，可以回滚：

```bash
# 切换到 v2.0.0 标签
git checkout v2.0.0-go

# 恢复配置
cp backend-go/.config/config.json.backup backend-go/.config/config.json
cp backend-go/.env.backup backend-go/.env

# 重新构建
make clean
cd frontend && npm install && npm run build && cd ..
cd backend-go && ./build.sh
```

## ✅ 升级检查清单

- [ ] 备份配置文件
- [ ] 更新代码到 v2.0.1
- [ ] 更新 `.env` 文件（可选，推荐）
- [ ] 清除旧构建缓存 (`make clean`)
- [ ] 重新构建前端 (`make build-frontend-internal`)
- [ ] 启动服务器 (`make run`)
- [ ] 验证版本信息 (`make info`)
- [ ] 测试 Web 界面访问
- [ ] 测试 API 调用

---

**升级成功！** 🎉

现在你拥有了更快、更稳定的 Claude Proxy v2.0.1！
