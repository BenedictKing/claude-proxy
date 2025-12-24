---
name: github-release
description: 发布 GitHub Release，从 CHANGELOG 生成发布公告并更新 Draft Release
version: 1.0.0
author: https://github.com/BenedictKing/claude-proxy/
allowed-tools: Bash, Read
---

# GitHub Release 发布技能

## 触发条件

当用户输入包含以下关键词时触发：
- "发布公告"、"发布说明"、"release notes"
- "发布 release"、"publish release"
- "更新 draft"、"编辑 release"

## 执行步骤

### 1. 获取最新 tag 和上次公开发布的 tag

```bash
# 获取最新 tag
git describe --tags --abbrev=0

# 获取所有 tag 列表
git tag --sort=-v:refname | head -10
```

询问用户：上次公开发布的版本是哪个？（如果用户已在对话中提及则直接使用）

### 2. 获取版本间的变更日志

```bash
# 从 CHANGELOG.md 中提取相关版本的内容
cat CHANGELOG.md
```

解析 CHANGELOG.md，提取从上次发布版本到当前版本的所有变更内容。

### 3. 生成发布公告

根据 CHANGELOG 内容生成简洁的发布公告，格式：

```markdown
## 主要更新

### ✨ 新功能
- 功能点1
- 功能点2

### 🐛 修复
- 修复点1
- 修复点2

### ⚡ 优化
- 优化点1
```

**注意事项**：
- 合并多个小版本的内容
- 保持简洁，每个点一行
- 移除技术实现细节，保留用户可感知的变化

### 4. 检查 Draft Release 状态

```bash
# 查看是否有 draft release
gh release list --limit 5

# 查看特定 release 详情
gh release view <tag> --json isDraft,name,body
```

### 5. 更新 Draft Release 并发布

```bash
# 编辑 release 内容并发布
gh release edit <tag> \
  --title "<tag>" \
  --notes "发布公告内容" \
  --draft=false
```

或者如果没有 draft，直接创建：

```bash
gh release create <tag> \
  --title "<tag>" \
  --notes "发布公告内容" \
  --latest
```

### 6. 确认发布成功

```bash
gh release view <tag> --json url,publishedAt
```

输出发布链接供用户确认。

## 输出格式

```
📦 Release 发布完成！

版本: v2.3.7
状态: ✅ 已发布
链接: https://github.com/BenedictKing/claude-proxy/releases/tag/v2.3.7

发布内容:
---
[发布公告内容]
---
```

## 注意事项

- 确保 `gh` CLI 已登录并有仓库权限
- 发布前会显示完整公告内容供用户确认
- 支持多版本合并发布（如 v2.3.5 ~ v2.3.7）
