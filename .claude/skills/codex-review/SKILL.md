---
name: codex-review
description: 调用 codex 命令行进行代码审核，自动收集当前文件修改和任务状态一并发送 (user)
version: 1.2.0
author: https://github.com/BenedictKing/claude-proxy/
allowed-tools: Bash, Read, Glob, Write
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

## 执行步骤

### 1. 记录修改意图（关键步骤）

在审核前，先在 `CHANGELOG.md` 或 `docs/SCRATCHPAD.md` 中写下本次修改说明：

```markdown
## [Unreleased] - YYYY-MM-DD

### Changed
- 本次修改解决了 X 问题，采用了 Y 方案
- 具体改动：...
```

**为什么有效**：Codex 看到 diff 中包含人类可读的"修改说明"，就不需要猜测意图，而是直接验证："代码里的逻辑真的实现了 Changelog 里描述的吗？"

### 2. 预处理：Lint First（减少噪音）

在调用 Codex 前，先用静态分析工具扫一遍，不要让 Codex 浪费 token 在格式问题上：

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

**命令参数说明**：
- `--uncommitted`: 审核工作区中所有未提交的更改
- `--base <branch>`: 审核相对于指定分支的更改
- `--commit <sha>`: 审核指定的提交

### 4. 自我修正

如果 Codex 发现 Changelog 描述与代码逻辑不一致：
- **代码错误** → 修复代码
- **描述不准确** → 更新 Changelog

## 完整审核协议

```markdown
## 🕵️ Code Review Protocol

1. **Document Intent**:
   - 更新 CHANGELOG.md 或 docs/SCRATCHPAD.md
   - 说明本次修改解决什么问题、采用什么方案

2. **Clean Up**:
   - 运行格式化/lint 工具

3. **Execute Review**:
   - 运行: `codex review --uncommitted`
   - Codex 会同时看到意图描述和代码变更

4. **Self-Correction**:
   - 如发现意图与实现不一致，修复代码或更新描述
```

## 注意事项

- 确保在 git 仓库目录下执行
- 超时时间设置为 15 分钟 (`timeout: 900000`)
- codex 命令需要已正确配置并登录
- 大量修改时 codex 会自动分批处理
- CHANGELOG.md 也在未提交变更中时效果最佳
