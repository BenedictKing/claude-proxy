# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Claude API 代理服务器 - AI 助手指南

> 🚀 一个高性能的Claude API代理服务器，支持多种上游AI服务提供商，提供Web管理界面和统一API入口。

---

## 📚 文档导航

在回答问题前，**优先查阅相关文档**：

| 文档 | 用途 | 链接 |
|------|------|------|
| **README.md** | 快速入门、部署指南 | [README.md](README.md) |
| **ARCHITECTURE.md** | 技术架构、设计模式 | [ARCHITECTURE.md](ARCHITECTURE.md) |
| **DEVELOPMENT.md** | 开发流程、调试技巧 | [DEVELOPMENT.md](DEVELOPMENT.md) |
| **ENVIRONMENT.md** | 环境变量配置 | [ENVIRONMENT.md](ENVIRONMENT.md) |
| **CONTRIBUTING.md** | 贡献规范、提交标准 | [CONTRIBUTING.md](CONTRIBUTING.md) |
| **CHANGELOG.md** | 版本历史、升级指南 | [CHANGELOG.md](CHANGELOG.md) |

---

## 变更记录 (最近3次)

### 2025-10-13

- **功能增强**: 故障转移与降级、上游请求超时与重试、健康检查优化
- **Bug修复**: 流式响应优化、代理逻辑改进
- **代码重构**: JSON处理性能优化

### 2025-10-12

- **添加API密钥复制功能**：渠道卡片和编辑弹框支持一键复制
- **实现自动登录功能**：页面刷新时自动验证访问密钥
- **修复双重登录框问题**：移除Go后端HTML登录页面

> 📚 完整变更历史请参考 [CHANGELOG.md](CHANGELOG.md)

---

## 项目愿景

本项目是一个现代化的AI API代理服务器，核心目标：

- 🔄 **协议转换**: Claude格式 ↔ 各上游服务格式
- ⚖️ **负载均衡**: 多API密钥智能分配和故障转移
- 🖥️ **可视化管理**: 现代化Web管理界面
- 🛡️ **高可用性**: 健康检查、错误处理、优雅降级

---

## 核心架构速查

### 技术栈

**后端 (backend-go/)**
- Go 1.22+ + Gin Framework
- 启动时间 < 100ms，内存占用 ~20MB

**前端 (frontend/)**
- Vue 3 + Vuetify 3 + Vite

> 📚 详细架构设计请参考 [ARCHITECTURE.md](ARCHITECTURE.md)

### 项目结构

```
claude-proxy/
├── backend-go/              # Go 后端服务 (主要)
│   ├── cmd/                # 主程序入口
│   ├── internal/           # 内部实现
│   │   ├── config/        # 配置管理
│   │   ├── handlers/      # HTTP 处理器
│   │   ├── middleware/    # 中间件
│   │   └── providers/     # 上游服务适配器
│   └── .config/           # 运行时配置
├── frontend/               # Vue 3 前端
│   └── dist/              # 构建产物
└── backend/                # Node.js/Bun 后端 (备用)
```

### 关键命令

```bash
# 开发模式（前后端热重载）
bun run dev

# 生产构建和启动
bun run build
bun run start

# Go后端配置工具
cd backend-go && make help

# Docker部署
docker-compose up -d
```

> 📚 完整开发指南请参考 [DEVELOPMENT.md](DEVELOPMENT.md)

---

## 编码规范速查

### SOLID + KISS + DRY + YAGNI

- **KISS**: 保持代码简洁，优先直观方案
- **DRY**: 消除重复代码，提取共享函数
- **YAGNI**: 仅实现当前所需功能
- **函数式优先**: 使用 `map`/`reduce`/`filter`

### 命名规范

- 文件名: `kebab-case` (例: `config-manager.ts`)
- 类名: `PascalCase` (例: `ConfigManager`)
- 函数名: `camelCase` (例: `getNextApiKey`)
- 常量: `SCREAMING_SNAKE_CASE` (例: `DEFAULT_CONFIG`)

> 📚 完整编码规范请参考 [CONTRIBUTING.md](CONTRIBUTING.md)

---

## ⚠️ 重要提示

### Git 操作限制
**（重要：如果用户没有主动要求，绝对不要计划和执行git提交和分支等操作）**
- 仅在用户明确要求时才执行 `git commit`、`git push`
- 任何涉及版本控制的操作都需要用户确认

### 环境变量安全
- 生产环境必须修改 `PROXY_ACCESS_KEY`
- 使用 `openssl rand -base64 32` 生成强密钥

> 📚 环境配置详见 [ENVIRONMENT.md](ENVIRONMENT.md)

---

## 快速参考

### 常用API端点

```bash
# 健康检查 (无需认证)
GET /health

# Web管理界面 (需要密钥)
GET /

# Claude API代理 (需要密钥)
POST /v1/messages

# 管理API (需要密钥)
GET /api/channels
```

### 环境变量核心配置

```bash
# 服务器配置
PORT=3000
ENV=production

# 访问控制 (必须修改!)
PROXY_ACCESS_KEY=your-super-strong-secret-key

# Web UI
ENABLE_WEB_UI=true

# 日志配置
LOG_LEVEL=info
```

---

> 💡 **提示**: 本项目遵循Monorepo结构，前后端代码共存但相对独立。开发时建议使用`bun run dev`以获得最佳开发体验。
