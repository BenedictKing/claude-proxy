# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

Claude / Codex / Gemini API Proxy - 支持多上游 AI 服务的协议转换代理，提供 Web 管理界面和统一 API 入口。

**技术栈**: Go 1.22 (后端) + Vue 3 + Vuetify (前端) + Docker

## 常用命令

```bash
# 根目录（推荐）
make dev              # Go 后端热重载开发（不含前端）
make run              # 构建前端并运行 Go 后端
make frontend-dev     # 前端开发服务器
make build            # 构建前端并编译 Go 后端
make clean            # 清理构建文件
docker-compose up -d  # Docker 部署

# Go 后端开发 (backend-go/)
make dev              # 热重载开发模式
make test             # 运行所有测试
make test-cover       # 测试 + 覆盖率报告（生成 coverage.html）
make build            # 构建生产版本
make lint             # 代码检查（需要 golangci-lint）
make fmt              # 格式化代码
make deps             # 更新依赖

# 运行特定测试
go test -v ./internal/converters/...       # 运行单个包测试
go test -v -run TestName ./internal/...    # 运行单个测试

# 前端开发 (frontend/)
bun install && bun run dev    # 开发服务器
bun run build                 # 生产构建
```

## 架构概览

```
claude-proxy/
├── backend-go/                 # Go 后端（主程序）
│   ├── main.go                # 入口、路由配置
│   └── internal/
│       ├── handlers/          # HTTP 处理器 (proxy.go, responses.go, config.go)
│       ├── providers/         # 上游适配器 (openai.go, gemini.go, claude.go)
│       ├── converters/        # 协议转换器（工厂模式）
│       ├── scheduler/         # 多渠道调度器（优先级、熔断）
│       ├── session/           # 会话管理 + Trace 亲和性
│       ├── metrics/           # 渠道指标（滑动窗口算法）
│       ├── config/            # 配置管理（fsnotify 热重载）
│       └── middleware/        # 认证、CORS、日志过滤
├── frontend/                   # Vue 3 + Vuetify 前端
│   └── src/
│       ├── components/        # Vue 组件
│       └── services/          # API 服务封装
└── .config/                    # 运行时配置（热重载）
```

## 核心设计模式

1. **Provider Pattern** - `internal/providers/`: 所有上游实现统一 `Provider` 接口
2. **Converter Pattern** - `internal/converters/`: 协议转换，工厂模式创建转换器
3. **Session Manager** - `internal/session/`: 基于 `previous_response_id` 的多轮对话跟踪
4. **Scheduler Pattern** - `internal/scheduler/`: 优先级调度、Trace 亲和性、自动熔断

## API 端点

**代理端点**:
- `POST /v1/messages` - Claude Messages API（支持 OpenAI/Gemini 协议转换）
- `POST /v1/messages/count_tokens` - Token 计数
- `POST /v1/responses` - Codex Responses API（支持会话管理）
- `POST /v1/responses/compact` - 精简版 Responses API
- `GET /health` - 健康检查（无需认证）

**管理 API** (`/api/`):
- `/api/messages/channels` - Messages 渠道 CRUD
- `/api/responses/channels` - Responses 渠道 CRUD
- `/api/messages/channels/metrics` - 渠道指标
- `/api/messages/channels/scheduler/stats` - 调度器统计
- `/api/messages/ping/:id` - 渠道连通性测试

## 关键配置

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `PORT` | 3000 | 服务器端口 |
| `ENV` | production | 运行环境 |
| `PROXY_ACCESS_KEY` | - | **必须设置** 访问密钥 |
| `QUIET_POLLING_LOGS` | true | 静默轮询日志 |
| `MAX_REQUEST_BODY_SIZE_MB` | 50 | 请求体最大大小 |

**注意**: 负载均衡策略通过 Web UI 或 `config.json` 配置，不再使用环境变量。

完整配置参考 `backend-go/.env.example`

## 常见任务

1. **添加新的上游服务**: 在 `internal/providers/` 实现 `Provider` 接口，在 `GetProvider()` 注册
2. **修改协议转换**: 编辑 `internal/converters/` 中的转换器
3. **调整调度策略**: 修改 `internal/scheduler/channel_scheduler.go`
4. **前端界面调整**: 编辑 `frontend/src/components/` 中的 Vue 组件

## 重要提示

- **Git 操作**: 未经用户明确要求，不要执行 git commit/push/branch 操作
- **配置热重载**: `backend-go/.config/config.json` 修改后自动生效，无需重启
- **环境变量变更**: 修改 `.env` 后需要重启服务
- **认证**: 所有端点（除 `/health`）需要 `x-api-key` 头或 `PROXY_ACCESS_KEY`

## 模块文档

- [backend-go/CLAUDE.md](backend-go/CLAUDE.md) - Go 后端详细文档
- [frontend/CLAUDE.md](frontend/CLAUDE.md) - Vue 前端详细文档
- [ARCHITECTURE.md](ARCHITECTURE.md) - 详细架构设计
- [ENVIRONMENT.md](ENVIRONMENT.md) - 完整环境变量配置
