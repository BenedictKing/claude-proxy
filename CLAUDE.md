# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

Claude API 代理服务器 - 支持多上游 AI 服务（OpenAI/Gemini/Claude）的协议转换代理，提供 Web 管理界面和统一 API 入口。

**技术栈**: Go 1.22 (后端) + Vue 3 + Vuetify (前端) + Docker

## 常用命令

```bash
# Go 后端开发
cd backend-go
make dev              # 热重载开发模式
make test             # 运行所有测试
make test-cover       # 测试 + 覆盖率报告
go test -v ./internal/converters/...  # 运行单个包测试
go test -v -run TestName ./internal/...  # 运行单个测试
make build            # 构建生产版本
make lint             # 代码检查
make fmt              # 格式化代码

# 前端开发
cd frontend
bun install && bun run dev

# 根目录（推荐）
make dev              # Go 后端热重载开发（不含前端）
make run              # 构建前端并运行 Go 后端
make frontend-dev     # 前端开发服务器
make build            # 构建前端并编译 Go 后端
make clean            # 清理构建文件
docker-compose up -d  # Docker 部署
```

## 架构概览

```
claude-proxy/
├── backend-go/                 # Go 后端
│   ├── main.go                # 入口
│   └── internal/
│       ├── handlers/          # HTTP 处理器 (proxy.go, responses.go)
│       ├── providers/         # 上游适配器 (openai.go, gemini.go, claude.go)
│       ├── converters/        # Responses API 协议转换器
│       ├── config/            # 配置管理 + 热重载
│       ├── session/           # Responses API 会话管理
│       ├── scheduler/         # 多渠道调度器
│       └── metrics/           # 渠道指标监控
└── frontend/                   # Vue 3 + Vuetify 前端
    └── src/
        ├── components/        # Vue 组件
        └── services/          # API 服务
```

## 核心设计模式

1. **Provider Pattern** - `internal/providers/`: 所有上游实现统一 `Provider` 接口
2. **Converter Pattern** - `internal/converters/`: Responses API 协议转换，工厂模式创建转换器
3. **Session Manager** - `internal/session/`: 基于 `previous_response_id` 的多轮对话跟踪
4. **Scheduler Pattern** - `internal/scheduler/`: 优先级调度、健康检查、自动熔断

## 双 API 支持

- `/v1/messages` - Claude Messages API（支持 OpenAI/Gemini 协议转换）
- `/v1/responses` - Codex Responses API（支持会话管理）

## 常见任务

1. **添加新的上游服务**: 在 `internal/providers/` 实现 `Provider` 接口，在 `GetProvider()` 注册
2. **修改协议转换**: 编辑 `internal/converters/` 中的转换器
3. **调整调度策略**: 修改 `internal/scheduler/channel_scheduler.go`
4. **前端界面调整**: 编辑 `frontend/src/components/` 中的 Vue 组件

## 重要提示

- **Git 操作**: 未经用户明确要求，不要执行 git commit/push/branch 操作
- **配置热重载**: `backend-go/.config/config.json` 修改后自动生效，无需重启
- **认证**: 所有端点（除 `/health`）需要 `x-api-key` 头或 `PROXY_ACCESS_KEY`
- **环境变量**: 通过 `.env` 文件配置，参考 `backend-go/.env.example`

## 模块文档

- [backend-go/CLAUDE.md](backend-go/CLAUDE.md) - Go 后端详细文档（API 端点、Provider 接口、数据模型）
- [frontend/CLAUDE.md](frontend/CLAUDE.md) - Vue 前端详细文档（组件、API 服务）
