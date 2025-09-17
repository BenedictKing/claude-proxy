# 发布指南

本文档为项目维护者提供了一套标准的版本发布流程，以确保版本迭代的一致性和清晰度。

## 版本规范

项目遵循**语义化版本 2.0.0 (Semantic Versioning)** 规范。版本格式为 `主版本号.次版本号.修订号` (MAJOR.MINOR.PATCH)，版本号递增规则如下：

-   **主版本号 (MAJOR)**: 当你做了不兼容的 API 修改。
-   **次版本号 (MINOR)**: 当你做了向下兼容的功能性新增。
-   **修订号 (PATCH)**: 当你做了向下兼容的问题修正。

## 发布流程

### 步骤 1: 准备工作

1.  确保本地的 `main` 分支是最新且稳定的。
    ```bash
    git checkout main
    git pull origin main
    ```

2.  确认所有计划内的功能和修复都已合并到 `main` 分支。

3.  验证代码质量和构建状态。
    ```bash
    # TypeScript 类型检查
    bun run type-check
    
    # 构建验证
    bun run build
    ```

### 步骤 2: 更新版本日志 (`CHANGELOG.md`)

1.  打开 `CHANGELOG.md` 文件。
2.  在文件顶部新增一个版本标题，格式为 `## vX.Y.Z - YYYY-MM-DD`。
3.  在标题下，根据本次版本的变更内容，添加相应的分类，如：
    -   `### ✨ 新功能`
    -   `### 🐛 Bug 修复`
    -   `### ♻️ 重构`
    -   `### 📚 文档`
    -   `### ⚙️ 其他`

4.  运行以下命令，查看自上一个版本以来的所有提交记录，以帮助你整理更新日志。
    ```bash
    # 将 v1.0.0 替换为上一个版本的标签
    git log v1.0.0...HEAD --oneline
    ```

### 步骤 3: 更新 `package.json` 中的版本号

1.  打开 `package.json` 文件。
2.  将 `version` 字段的值更新为新的版本号。

    例如，从 `1.0.0` 更新到 `1.0.1`:
    ```diff
    - "version": "1.0.0",
    + "version": "1.0.1",
    ```

### 步骤 4: 提交版本更新

1.  将 `CHANGELOG.md` 和 `package.json` 的修改提交到暂存区。
    ```bash
    git add CHANGELOG.md package.json
    ```

2.  使用标准化的提交信息进行提交。
    ```bash
    # 将 vX.Y.Z 替换为新版本号
    git commit -m "chore(release): prepare for vX.Y.Z"
    ```

3.  将提交推送到远程 `main` 分支。
    ```bash
    git push origin main
    ```

### 步骤 5: 创建并推送 Git 标签

1.  为此次提交创建一个附注标签（annotated tag）。
    ```bash
    # 将 vX.Y.Z 替换为新版本号
    git tag -a vX.Y.Z -m "Release vX.Y.Z"
    ```

2.  将新创建的标签推送到远程仓库。
    ```bash
    # 将 vX.Y.Z 替换为新版本号
    git push origin vX.Y.Z
    ```

### 步骤 6: 在 GitHub 上创建 Release (可选但推荐)

1.  访问项目的 GitHub 页面，进入 "Releases" 部分。
2.  点击 "Draft a new release"。
3.  从 "Choose a tag" 下拉菜单中选择你刚刚推送的标签（如 `vX.Y.Z`）。
4.  将 `CHANGELOG.md` 中对应版本的更新内容复制到发布说明中。
5.  点击 "Publish release"。

至此，新版本的发布流程已全部完成。
