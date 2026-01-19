# Codex Review Skill

专业的代码审核技能，集成 Codex AI 审核工具，自动收集变更上下文并生成 CHANGELOG。

## ✨ 功能特性

- **智能上下文收集**：自动分析 Git 变更，收集完整的代码修改上下文
- **自动 CHANGELOG 生成**：检测到 CHANGELOG 未更新时，自动分析变更并生成规范条目
- **双模式审核**：
  - 有未提交变更 → 审核工作区所有修改
  - 工作区干净 → 审核最新提交
- **智能难度评估**：根据变更规模自动调整审核深度和超时时间
- **Lint 集成**：自动执行代码格式化和静态检查
- **意图驱动审核**：结合 CHANGELOG 描述和代码变更，提供更准确的审核建议

## 🚀 使用方法

### 通过斜杠命令

```bash
/codex-review
```

### 通过自然语言

```
"代码审核"
"帮我审查一下代码"
"review 一下"
"检查代码"
```

## 📋 工作流程

### 1. 检查工作区状态

自动检测是否有未提交的变更，决定审核模式。

### 2. CHANGELOG 检查与自动生成

如果 CHANGELOG.md 未更新，技能会：
1. 分析 `git diff` 获取完整变更
2. 自动生成符合规范的 CHANGELOG 条目
3. 使用 Edit 工具写入文件
4. 继续执行审核流程

**自动生成的 CHANGELOG 格式**：

```markdown
## [Unreleased]

### Added（新功能）/ Changed（修改）/ Fixed（修复）

- 功能描述：解决了什么问题或实现了什么功能
- 涉及文件：主要修改的文件/模块
```

### 3. 暂存新增文件（v2.1.0 新增）

**自动将所有新增文件加入 git 暂存区，避免 codex 报 P1 错误。**

```bash
# 安全地暂存所有新增文件（处理空列表和特殊文件名）
git ls-files --others --exclude-standard -z | while IFS= read -r -d '' f; do git add -- "$f"; done
```

**特点：**
- 使用 null 字符分隔，正确处理包含空格/换行的文件名
- 使用 `--` 分隔符，正确处理以 `-` 开头的文件名
- 当没有新增文件时，循环体不执行，安全跳过
- 只处理新增文件，不暂存已修改的文件
- 自动排除 .gitignore 中的文件

### 4. 智能难度评估

根据变更规模自动选择审核配置：

| 难度级别 | 触发条件 | 配置 | 超时时间 |
|---------|---------|------|---------|
| **困难任务** | 修改文件 ≥ 10 个<br>代码变更 ≥ 500 行<br>核心架构修改 | `model_reasoning_effort=xhigh` | 30 分钟 |
| **一般任务** | 其他情况 | `model_reasoning_effort=high` | 10 分钟 |

### 5. Lint + Codex 审核

自动执行项目对应的 Lint 工具：

- **Go 项目**：`go fmt ./... && go vet ./...`
- **Node 项目**：`npm run lint:fix`
- **Python 项目**：`black . && ruff check --fix .`

然后调用 `codex review` 进行 AI 代码审核。

### 6. 自我修正

如果 Codex 发现 CHANGELOG 描述与代码逻辑不一致：
- **代码错误** → 修复代码
- **描述不准确** → 更新 CHANGELOG

## 📦 依赖要求

### 必需

- **Git 仓库**：必须在 Git 仓库目录下执行
- **Codex CLI**：需要安装并配置 [Codex](https://codex.ai/) 命令行工具
- **CHANGELOG.md**：项目根目录需要有 CHANGELOG.md 文件

### 可选（根据项目类型）

- **Go 项目**：`go fmt`、`go vet`
- **Node 项目**：`npm run lint`
- **Python 项目**：`black`、`ruff`

## 🎯 使用场景

### 场景 1：日常开发审核

```
用户：修改了几个文件，想审核一下
技能：
1. 检测到 3 个文件修改，200 行变更
2. 检查 CHANGELOG 未更新，自动生成条目
3. 执行 go fmt && go vet
4. 调用 codex review --uncommitted --config model_reasoning_effort=high
5. 返回审核结果和改进建议
```

### 场景 2：大规模重构审核

```
用户：重构了整个模块，需要全面审核
技能：
1. 检测到 15 个文件修改，800 行变更
2. 判定为困难任务
3. 自动生成 CHANGELOG 条目
4. 执行 Lint
5. 调用 codex review --uncommitted --config model_reasoning_effort=xhigh
6. 30 分钟超时，深度审核
```

### 场景 3：审核最新提交

```
用户：工作区干净，想审核刚才的提交
技能：
1. 检测到工作区干净
2. 直接调用 codex review --commit HEAD
3. 返回审核结果
```

## 🔧 配置说明

### Codex Review 命令参考

```bash
# 审核所有未提交的更改
codex review --uncommitted

# 审核最新提交
codex review --commit HEAD

# 审核指定提交
codex review --commit abc1234

# 审核相对于 main 分支的所有更改
codex review --base main

# 使用特定模型
codex review --uncommitted -c model="o3"

# 调整推理深度
codex review --uncommitted -c model_reasoning_effort=xhigh
```

### 重要限制

- `--uncommitted`、`--base`、`--commit` 三者互斥
- 必须在 Git 仓库目录下执行
- CHANGELOG.md 必须在未提交变更中，否则 Codex 无法看到意图描述

## 🏗️ 架构设计

### 为什么分离上下文？

本技能采用两阶段架构：

1. **准备阶段**（当前上下文）：
   - 检查工作区状态
   - 更新 CHANGELOG（需要理解用户意图和对话历史）
   - 暂存新增文件（避免 codex P1 错误）

2. **审核阶段**（独立上下文，`context: fork`）：
   - 执行 Lint + codex review
   - 不携带无关的对话上下文，减少 Token 消耗

### 核心理念：意图 vs 实现

单纯运行 `codex review --uncommitted` 只让 AI 看"做了什么 (Implementation)"。

通过先记录意图（CHANGELOG），是在告诉 AI "想做什么 (Intention)"。

**"代码变更 + 意图描述"同时作为输入，是提升 AI 代码审查质量的最高效手段。**

## 📝 CHANGELOG 规范

推荐使用 [Keep a Changelog](https://keepachangelog.com/) 格式：

```markdown
# Changelog

## [Unreleased]

### Added
- 新增功能描述

### Changed
- 修改内容描述

### Fixed
- 修复问题描述

## [1.0.0] - 2026-01-19

### Added
- 初始版本发布
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

## 🔗 相关链接

- [Codex CLI 文档](https://codex.ai/docs)
- [Claude Code 技能文档](https://code.claude.com/docs/en/skills)
- [项目仓库](https://github.com/BenedictKing/claude-proxy)
