# 贡献指南

本文档为项目贡献者提供了一套标准化的指导，以确保代码库的一致性和高质量。

## 如何贡献

欢迎通过提交 Issue 和 Pull Request 为本项目贡献力量！

1.  Fork 本项目。
2.  创建特性分支 (`git checkout -b feature/AmazingFeature`)。
3.  提交改动 (`git commit -m 'feat: Add some AmazingFeature'`)。
4.  推送到分支 (`git push origin feature/AmazingFeature`)。
5.  开启 Pull Request。

## 版本规范

项目遵循 **语义化版本 2.0.0 (Semantic Versioning)** 规范。版本格式为 `主版本号.次版本号.修订号` (MAJOR.MINOR.PATCH)，版本号递增规则如下：

-   **主版本号 (MAJOR)**: 当你做了不兼容的 API 修改。
-   **次版本号 (MINOR)**: 当你做了向下兼容的功能性新增。
-   **修订号 (PATCH)**: 当你做了向下兼容的问题修正。

## 发布流程

本节为项目维护者提供了版本发布流程概述。贡献者通常不需要直接执行这些步骤。

1.  **准备工作**:
    *   确保本地 `main` 分支是最新的。
    *   确认所有计划内的功能和修复已合并。
    *   运行类型检查 (`bun run type-check`) 和构建验证 (`bun run build`)。
2.  **更新日志**: 更新 `CHANGELOG.md`，新增版本标题，并分类记录变更内容。
3.  **更新版本号**: 更新 `package.json` 中的 `version` 字段。
4.  **提交**: 提交 `CHANGELOG.md` 和 `package.json` 的修改，提交信息格式为 `chore(release): prepare for vX.Y.Z`。
5.  **创建标签**: 为此次提交创建附注标签 `git tag -a vX.Y.Z -m "Release vX.Y.Z"` 并推送到远程。
6.  **GitHub Release**: 在 GitHub 上创建 Release，将 `CHANGELOG.md` 中的对应版本内容复制到发布说明中。

## 编码规范

### 设计原则

项目严格遵循以下软件工程原则：

1.  **KISS 原则 (Keep It Simple, Stupid)**: 追求代码和设计的极致简洁，优先选择最直观的解决方案。
2.  **DRY 原则 (Don't Repeat Yourself)**: 消除重复代码，提取共享函数，统一相似功能的实现方式。
3.  **YAGNI 原则 (You Aren't Gonna Need It)**: 仅实现当前明确所需的功能，删除未使用的代码和依赖，避免过度设计。
4.  **函数式编程优先**: 优先使用 `map`、`reduce`、`filter` 等函数式方法和不可变数据操作。

### 代码质量标准

-   使用 TypeScript 严格模式，避免 `any` 类型。
-   所有函数都有明确的类型声明。
-   实现优雅的错误处理和日志记录。
-   遵循 Prettier 格式化（2空格、单引号、无分号、宽度120、LF EOL）。

### 文件命名规范

-   **文件名**: `kebab-case` (例: `config-manager.ts`)
-   **类名**: `PascalCase` (例: `ConfigManager`)
-   **Vue 组件名**: `PascalCase` (例: `ChannelCard.vue`)
-   **类型/接口名**: `PascalCase`
-   **函数名**: `camelCase` (例: `getNextApiKey`)
-   **常量名**: `SCREAMING_SNAKE_CASE` (例: `DEFAULT_CONFIG`)

### TypeScript 规范

-   使用严格的 TypeScript 配置。
-   所有函数和变量都有明确的类型声明。
-   使用接口定义数据结构。
-   避免使用 `any` 类型。

## 测试指南

### 开发测试

在提交代码前，请确保：
-   运行 TypeScript 类型检查：`bun run type-check`
-   运行构建验证：`bun run build`
-   通过健康检查端点 (`GET http://localhost:3000/health`) 进行冒烟测试。
-   对于 UI 变更，在 Pull Request 中包含简短的测试计划和截图/GIF。

### 提交与 Pull Request 指南

-   **Conventional Commits**: 提交信息遵循 `conventional-commits` 规范，例如 `feat:`, `fix:`, `refactor:`, `chore:`。
    -   示例: `feat(frontend): add ESC to close modal`, `fix(backend): redact Authorization header`。
-   **PR 内容**: Pull Request 必须包含：
    -   目的说明
    -   关联的 Issue (如果有)
    -   详细的测试步骤
    -   配置/环境变量变更说明
    -   UI 变更的截图/GIF

## 安全与配置提示

-   **切勿提交敏感信息**: 永远不要将密钥或敏感配置提交到版本控制中。使用 `.env` 文件和 `backend/config.json` 进行管理。
-   **访问密钥**: `PROXY_ACCESS_KEY` 是代理访问的必需密钥。避免在日志中记录完整的 API 密钥。

## Agent-Specific Notes

-   保持代码差异最小化，与现有代码风格保持一致。
-   当行为发生变化时，及时更新相关文档。
-   除非必要，否则避免进行重命名或大规模重构。
