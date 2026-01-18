---
name: codex-review
description: 调用 codex 命令行进行代码审核，自动收集当前文件修改和任务状态一并发送；工作区干净时自动审核最新提交 (user)
version: 2.0.0
author: https://github.com/BenedictKing/claude-proxy/
allowed-tools: Bash, Read, Glob, Write, Edit, Task
---

# Codex 代码审核技能

## 触发条件

当用户输入包含以下关键词时触发：

- "代码审核"、"代码审查"、"审查代码"、"审核代码"
- "review"、"code review"、"review code"、"codex 审核"
- "帮我审核"、"检查代码"、"审一下"、"看看代码"

## 核心理念：意图 vs 实现

单纯运行 `codex review --uncommitted` 只让 AI 看"做了什么 (Implementation)"。
通过先记录意图，是在告诉 AI "想做什么 (Intention)"。

**"代码变更 + 意图描述"同时作为输入，是提升 AI 代码审查质量的最高效手段。**

## 技能架构

本技能分为两个阶段：

1. **准备阶段**（当前上下文）：检查工作区、更新 CHANGELOG、执行 Lint
2. **审核阶段**（独立上下文）：调用 Task 工具执行 codex review（使用 context: fork 减少上下文浪费）

## 执行步骤

### 0. 【首先】检查工作区状态

```bash
git diff --name-only && git status --short
```

**根据输出决定审核模式：**

- **有未提交变更** → 继续执行步骤 1-4（常规流程）
- **工作区干净** → 直接调用 codex-runner 执行：`codex review --commit HEAD`

### 1. 【强制】检查 CHANGELOG 是否已更新

**在执行任何审核前，必须先检查 CHANGELOG.md 是否包含本次修改的说明。**

```bash
# 检查 CHANGELOG.md 是否在未提交变更中
git diff --name-only | grep -E "(CHANGELOG|changelog)"
```

**如果 CHANGELOG 未更新，你必须自动执行以下操作（不要让用户手动操作）：**

1. **分析变更内容**：运行 `git diff --stat` 和 `git diff` 获取完整变更
2. **自动生成 CHANGELOG 条目**：根据代码变更内容，生成符合规范的条目
3. **写入 CHANGELOG.md**：使用 Edit 工具将条目插入到文件顶部的 `[Unreleased]` 区域
4. **继续审核流程**：CHANGELOG 更新后立即继续执行后续步骤

**自动生成的 CHANGELOG 条目格式：**

```markdown
## [Unreleased]

### Added（新功能）/ Changed（修改）/ Fixed（修复）

- 功能描述：解决了什么问题或实现了什么功能
- 涉及文件：主要修改的文件/模块
```

**示例 - 自动生成流程：**

```
1. 检测到 CHANGELOG 未更新
2. 运行 git diff --stat 发现修改了 handlers/responses.go (+88 lines)
3. 运行 git diff 分析具体内容：新增了 CompactHandler 函数
4. 自动生成条目：
   ### Added
   - 新增 `/v1/responses/compact` 端点，支持对话上下文压缩
   - 支持多渠道故障转移和请求体大小限制
5. 使用 Edit 工具写入 CHANGELOG.md
6. 继续执行 lint 和 codex review
```

### 2. 预处理：Lint First（减少噪音）

在调用 Codex 前，先用静态分析工具扫一遍：

```bash
# Go 项目
go fmt ./... && go vet ./...

# Node 项目
npm run lint:fix

# Python 项目
black . && ruff check --fix .
```

### 3. 评估任务难度并调用 codex-runner

**统计变更规模：**

```bash
# 统计变更文件数量和代码行数
git diff --stat | tail -1
```

**难度评估标准：**

**困难任务**（满足任一条件）：
- 修改文件 ≥ 10 个
- 代码变更 ≥ 500 行
- 涉及核心架构/算法修改
- 跨模块重构
- 配置：`model_reasoning_effort=xhigh`，超时 30 分钟

**一般任务**（其他情况）：
- 配置：`model_reasoning_effort=high`，超时 10 分钟

**调用 codex-runner 子任务：**

使用 Task 工具调用 codex-runner，传入以下参数：

```
Task 参数:
- subagent_type: Bash
- description: "执行 codex review"
- prompt: 根据难度选择对应命令

困难任务:
  codex review --uncommitted --config model_reasoning_effort=xhigh

一般任务:
  codex review --uncommitted --config model_reasoning_effort=high

工作区干净时:
  codex review --commit HEAD --config model_reasoning_effort=high
```

### 4. 自我修正

如果 Codex 发现 Changelog 描述与代码逻辑不一致：

- **代码错误** → 修复代码
- **描述不准确** → 更新 Changelog

## 完整审核协议

1. **[GATE] Check CHANGELOG** - 未更新则自动生成并写入（利用当前上下文理解变更意图）
2. **[PREP] Lint & Format** - go fmt / npm lint / black
3. **[EXEC] Task → codex review** - 调用 Task 工具执行 codex（独立上下文，减少浪费）
4. **[FIX] Self-Correction** - 意图 ≠ 实现时修复代码或更新描述

## Codex Review 命令参考

### 基本语法

```bash
codex review [OPTIONS] [PROMPT]
```

**注意**: `[PROMPT]` 参数不能与 `--uncommitted`、`--base`、`--commit` 同时使用。

### 常用选项

| 选项                       | 说明                                                        | 示例                                                         |
| -------------------------- | ----------------------------------------------------------- | ------------------------------------------------------------ |
| `--uncommitted`            | 审核工作区所有未提交的更改（staged + unstaged + untracked） | `codex review --uncommitted`                                 |
| `--base <BRANCH>`          | 审核相对于指定基准分支的更改                                | `codex review --base main`                                   |
| `--commit <SHA>`           | 审核指定提交引入的更改                                      | `codex review --commit HEAD`                                 |
| `--title <TITLE>`          | 可选的提交标题，显示在审核摘要中                            | `codex review --uncommitted --title "feat: add JSON parser"` |
| `-c, --config <key=value>` | 覆盖配置值                                                  | `codex review --uncommitted -c model="o3"`                   |

### 使用场景示例

```bash
# 1. 审核所有未提交的更改（最常用）
codex review --uncommitted

# 2. 审核最新提交
codex review --commit HEAD

# 3. 审核指定提交
codex review --commit abc1234

# 4. 审核当前分支相对于 main 的所有更改
codex review --base main

# 5. 审核当前分支相对于 develop 的更改
codex review --base develop

# 6. 带标题的审核（标题会显示在审核摘要中）
codex review --uncommitted --title "fix: resolve JSON parsing errors"

# 7. 使用特定模型进行审核
codex review --uncommitted -c model="o3"
```

### 重要限制

- `--uncommitted`、`--base`、`--commit` 三者互斥，不能同时使用
- `[PROMPT]` 参数与上述三个选项互斥
- 必须在 git 仓库目录下执行

## 注意事项

- 确保在 git 仓库目录下执行
- **超时时间根据任务难度自动调整：**
  - 困难任务：30 分钟 (`timeout: 1800000`)
  - 一般任务：10 分钟 (`timeout: 600000`)
- codex 命令需要已正确配置并登录
- 大量修改时 codex 会自动分批处理
- **CHANGELOG.md 必须在未提交变更中，否则 Codex 无法看到意图描述**

## 设计说明

**为什么分离上下文？**

1. **CHANGELOG 更新需要当前上下文**：理解用户之前的对话、任务意图，才能生成准确的变更描述
2. **Codex 审核不需要对话历史**：只需要代码变更和 CHANGELOG，独立运行更高效
3. **减少 Token 消耗**：codex review 作为独立子任务，不携带无关的对话上下文
