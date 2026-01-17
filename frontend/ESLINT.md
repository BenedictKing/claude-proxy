# ESLint 配置说明

## 已安装的包

- `eslint` - ESLint 核心
- `@eslint/js` - ESLint JavaScript 推荐规则
- `eslint-plugin-vue` - Vue 3 专用规则
- `vue-eslint-parser` - Vue 文件解析器
- `@typescript-eslint/parser` - TypeScript 解析器
- `@typescript-eslint/eslint-plugin` - TypeScript 规则
- `eslint-config-prettier` - 禁用与 Prettier 冲突的规则
- `eslint-plugin-prettier` - 将 Prettier 作为 ESLint 规则运行

## 可用命令

```bash
# 检查代码
npm run lint

# 自动修复可修复的问题
npm run lint:fix

# 格式化代码（Prettier）
npm run format
```

## 配置特点

### 1. ESLint 9+ Flat Config 格式
使用最新的 Flat Config 格式（`eslint.config.js`），不再使用旧的 `.eslintrc` 格式。

### 2. Vue 3 支持
- 使用 `eslint-plugin-vue` 的推荐规则
- 支持 Vue 3 Composition API
- 自动检测 Vue 组件问题

### 3. TypeScript 支持
- 完整的 TypeScript 类型检查
- 自动检测未使用的变量（以 `_` 开头的变量会被忽略）
- 警告使用 `any` 类型

### 4. Prettier 集成
- 自动禁用与 Prettier 冲突的规则
- 保持代码风格一致性
- 可以通过 `npm run format` 格式化代码

### 5. 浏览器环境支持
配置了常用的浏览器全局变量：
- `window`, `document`, `navigator`
- `localStorage`, `sessionStorage`
- `setTimeout`, `setInterval`
- `fetch`, `URL`, `AbortController`
- 等等

## 主要规则

### Vue 规则
- ✅ 允许单词组件名（`vue/multi-word-component-names: off`）
- ⚠️ 警告使用 `v-html`
- ⚠️ 建议显式声明 `emits`
- ❌ 强制自闭合标签规范

### TypeScript 规则
- ⚠️ 警告使用 `any` 类型
- ⚠️ 警告未使用的变量（以 `_` 开头除外）

### 通用规则
- ⚠️ 生产环境警告 `console.log`
- ❌ 生产环境禁止 `debugger`
- ⚠️ 建议使用 `const` 而非 `let`
- ❌ 禁止使用 `var`

## 忽略的文件

以下文件/目录会被自动忽略：
- `dist/**` - 构建产物
- `node_modules/**` - 依赖包
- `*.config.js` / `*.config.ts` - 配置文件
- `coverage/**` - 测试覆盖率报告
- `.vite/**` - Vite 缓存

## 与 Prettier 的关系

ESLint 负责代码质量检查（逻辑错误、最佳实践等），Prettier 负责代码格式化（缩进、引号、分号等）。两者通过 `eslint-config-prettier` 完美集成，不会产生冲突。

## 建议的工作流

1. **开发时**：编辑器实时显示 ESLint 警告/错误
2. **提交前**：运行 `npm run lint:fix` 自动修复问题
3. **CI/CD**：在持续集成中运行 `npm run lint` 确保代码质量

## IDE 集成

### VS Code
安装 ESLint 扩展：
```
ext install dbaeumer.vscode-eslint
```

在 `.vscode/settings.json` 中添加：
```json
{
  "editor.codeActionsOnSave": {
    "source.fixAll.eslint": true
  },
  "eslint.validate": [
    "javascript",
    "typescript",
    "vue"
  ]
}
```

### WebStorm / IntelliJ IDEA
ESLint 支持已内置，在设置中启用即可：
`Settings → Languages & Frameworks → JavaScript → Code Quality Tools → ESLint`

## 自定义规则

如需修改规则，编辑 `eslint.config.js` 文件。例如：

```javascript
{
  rules: {
    // 关闭某个规则
    'vue/multi-word-component-names': 'off',

    // 修改规则级别（off / warn / error）
    'no-console': 'warn',

    // 带选项的规则
    '@typescript-eslint/no-unused-vars': [
      'warn',
      { argsIgnorePattern: '^_' }
    ]
  }
}
```

## 常见问题

### Q: 为什么有些规则显示警告而不是错误？
A: 警告不会阻止代码运行，但会提醒您注意潜在问题。错误则必须修复。

### Q: 如何临时禁用某个规则？
A: 使用 ESLint 注释：
```javascript
// eslint-disable-next-line no-console
console.log('debug info')

/* eslint-disable vue/multi-word-component-names */
// 多行代码
/* eslint-enable vue/multi-word-component-names */
```

### Q: ESLint 和 Prettier 冲突怎么办？
A: 已通过 `eslint-config-prettier` 解决冲突，不应该出现此问题。如果遇到，请检查配置顺序。

## 参考资源

- [ESLint 官方文档](https://eslint.org/)
- [eslint-plugin-vue 文档](https://eslint.vuejs.org/)
- [TypeScript ESLint 文档](https://typescript-eslint.io/)
- [Prettier 官方文档](https://prettier.io/)
