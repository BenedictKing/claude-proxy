---
name: codex-review
description: 调用 codex 命令行进行代码审核，自动收集当前文件修改和任务状态一并发送；工作区干净时自动审核最新提交 (user)
version: 1.5.0
author: https://github.com/BenedictKing/claude-proxy/
allowed-tools: Bash, Read, Glob, Write, Edit
context: fork
---

# Codex 代码审核技能

## 触发条件

当用户输入包含以下关键词时触发：

- "代码审核"、"代码审查"、"审查代码"、"审核代码"
- "review"、"code review"、"review code"
- "帮我审核"、"检查代码"、"审一下"、"看看代码"

## 核心理念：意图 vs 实现

单纯运行 `codex review --uncommitted` 只让 AI 看"做了什么 (Implementation)"。
通过先记录意图，是在告诉 AI "想做什么 (Intention)"。

**"代码变更 + 意图描述"同时作为输入，是提升 AI 代码审查质量的最高效手段。**

## Codex Review 命令完整用法

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

## 执行步骤

### 0. 【首先】检查工作区状态

```bash
git diff --name-only && git status --short
```

**根据输出决定审核模式：**

- **有未提交变更** → 继续执行步骤 1-4（常规流程）
- **工作区干净** → 直接审核最新提交：`codex review --commit HEAD`

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

### 3. 调用 codex review

```bash
# 审核所有未提交的更改（推荐）
codex review --uncommitted

# 超时时间设置为 15 分钟 (900000ms)
```

### 4. 自我修正

如果 Codex 发现 Changelog 描述与代码逻辑不一致：

- **代码错误** → 修复代码
- **描述不准确** → 更新 Changelog

## 完整审核协议

1. **[GATE] Check CHANGELOG** - 未更新则自动生成并写入
2. **[PREP] Lint & Format** - go fmt / npm lint / black
3. **[EXEC] codex review --uncommitted** - Codex 同时看到意图 + 实现
4. **[FIX] Self-Correction** - 意图 ≠ 实现时修复代码或更新描述

## 注意事项

- 确保在 git 仓库目录下执行
- 超时时间设置为 15 分钟 (`timeout: 900000`)
- codex 命令需要已正确配置并登录
- 大量修改时 codex 会自动分批处理
- **CHANGELOG.md 必须在未提交变更中，否则 Codex 无法看到意图描述**
