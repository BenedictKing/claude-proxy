# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

Claude API 代理服务器 - 支持多上游 AI 服务（OpenAI/Gemini/Claude）的协议转换代理，提供 Web 管理界面和统一 API 入口。

## 常用命令

```bash
# Go 后端开发（推荐）
cd backend-go
make dev              # 热重载开发模式
make test             # 运行测试
make test-cover       # 测试 + 覆盖率
make build            # 构建生产版本
make lint             # 代码检查
make fmt              # 格式化代码

# 前端开发
cd frontend
bun install && bun run dev

# 根目录快捷命令
bun run dev           # 前后端联合开发
bun run build         # 生产构建
bun run start         # 启动生产服务

# Docker
docker-compose up -d
```

## 架构概览

```
claude-proxy/
├── backend-go/                 # Go 后端 (主要)
│   ├── main.go                # 入口
│   └── internal/
│       ├── handlers/          # HTTP 处理器 (proxy.go, responses.go, config.go)
│       ├── providers/         # 上游适配器 (openai.go, gemini.go, claude.go)
│       ├── converters/        # Responses API 协议转换器
│       ├── config/            # 配置管理 + 热重载
│       ├── session/           # Responses API 会话管理
│       └── middleware/        # 认证、CORS
├── frontend/                   # Vue 3 + Vuetify 前端
└── backend/                    # Node.js 备用实现
```

### 核心设计模式

1. **Provider Pattern** - `internal/providers/`: 所有上游实现统一 `Provider` 接口
2. **Converter Pattern** - `internal/converters/`: Responses API 的协议转换，通过工厂模式创建
3. **Session Manager** - `internal/session/`: 基于 `previous_response_id` 的多轮对话跟踪

### 双 API 支持

- `/v1/messages` - Claude Messages API（支持 OpenAI/Gemini 协议转换）
- `/v1/responses` - Codex Responses API（支持会话管理）

## 编码规范

- **KISS/DRY/YAGNI** - 保持简洁，消除重复，只实现当前所需
- **命名**: 文件 `kebab-case`，类 `PascalCase`，函数 `camelCase`，常量 `SCREAMING_SNAKE_CASE`
- **Go**: 使用 `go fmt`，遵循标准 Go 项目布局
- **TypeScript**: 严格类型，避免 `any`

## 重要提示

- **Git 操作**: 未经用户明确要求，不要执行 git commit/push/branch 操作
- **配置热重载**: `backend-go/.config/config.json` 修改后自动生效，无需重启
- **认证**: 所有端点（除 `/health`）需要 `x-api-key` 头或 `PROXY_ACCESS_KEY`

## 文档索引

| 文档 | 内容 |
|------|------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | 详细架构、设计模式、数据流 |
| [DEVELOPMENT.md](DEVELOPMENT.md) | 开发流程、调试技巧 |
| [ENVIRONMENT.md](ENVIRONMENT.md) | 环境变量配置 |
| [CONTRIBUTING.md](CONTRIBUTING.md) | 贡献规范 |
