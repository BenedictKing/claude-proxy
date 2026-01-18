---
name: codex-runner
description: 执行 codex review 命令的独立子任务（内部使用）
version: 1.0.0
author: https://github.com/BenedictKing/claude-proxy/
allowed-tools: Bash
context: fork
---

# Codex Runner 子技能

> **注意**：这是一个内部子技能，由 `codex-review` 主技能通过 Task 工具调用。

## 用途

独立执行 `codex review` 命令，使用 `context: fork` 避免携带主对话的上下文，减少 Token 消耗。

## 接收参数

通过 Task 工具的 prompt 参数接收：

1. **审核模式**：`--uncommitted` 或 `--commit HEAD` 或 `--base <branch>`
2. **难度配置**：`--config model_reasoning_effort=high|xhigh`
3. **超时时间**：通过 Task 工具的 timeout 参数控制

## 执行命令示例

```bash
# 一般任务 - 审核未提交变更
codex review --uncommitted --config model_reasoning_effort=high

# 困难任务 - 审核未提交变更（深度推理）
codex review --uncommitted --config model_reasoning_effort=xhigh

# 工作区干净 - 审核最新提交
codex review --commit HEAD --config model_reasoning_effort=high

# 审核相对于 main 分支的变更
codex review --base main --config model_reasoning_effort=high
```

## 输出格式

直接返回 `codex review` 的输出结果，包括：

- 代码审核摘要
- 发现的问题列表
- 改进建议

## 注意事项

- 必须在 git 仓库目录下执行
- 确保 codex 命令已正确配置和登录
- 超时时间由调用方通过 Task timeout 参数控制
