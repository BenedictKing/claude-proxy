---
name: codex-runner
description: 执行 Lint 和 codex review 的独立子任务（内部使用）
version: 1.0.0
author: https://github.com/BenedictKing/claude-proxy/
allowed-tools: Bash
context: fork
---

# Codex Runner 子技能

> **注意**：这是一个内部子技能，由 `codex-review` 主技能通过 Task 工具调用。

## 用途

独立执行 Lint 和 `codex review` 命令，使用 `context: fork` 避免携带主对话的上下文，减少 Token 消耗。

## 接收参数

通过 Task 工具的 prompt 参数接收完整命令链：

1. **Lint 命令**：根据项目类型自动选择（go fmt、npm lint、black 等）
2. **审核模式**：`--uncommitted` 或 `--commit HEAD` 或 `--base <branch>`
3. **难度配置**：`--config model_reasoning_effort=high|xhigh`
4. **超时时间**：通过 Task 工具的 timeout 参数控制

## 执行命令示例

```bash
# Go 项目 - 一般任务
go fmt ./... && go vet ./... && codex review --uncommitted --config model_reasoning_effort=high

# Go 项目 - 困难任务（深度推理）
go fmt ./... && go vet ./... && codex review --uncommitted --config model_reasoning_effort=xhigh

# Node 项目
npm run lint:fix && codex review --uncommitted --config model_reasoning_effort=high

# Python 项目
black . && ruff check --fix . && codex review --uncommitted --config model_reasoning_effort=high

# 工作区干净 - 审核最新提交
codex review --commit HEAD --config model_reasoning_effort=high

# 审核相对于 main 分支的变更
codex review --base main --config model_reasoning_effort=high
```

## 执行流程

1. **Lint First**：先执行静态分析工具修复格式问题
2. **Codex Review**：然后执行代码审核

## 输出格式

直接返回完整输出，包括：

- Lint 工具的修复结果
- 代码审核摘要
- 发现的问题列表
- 改进建议

## 注意事项

- 必须在 git 仓库目录下执行
- 确保 codex 命令已正确配置和登录
- 超时时间由调用方通过 Task timeout 参数控制
- Lint 失败不会阻止 codex review 执行（使用 `&&` 连接）
