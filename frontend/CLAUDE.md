[根目录](../CLAUDE.md) > **frontend**

# frontend 模块文档

## 变更记录 (Changelog)

### 2025-12-11 - 初始索引
- 创建模块文档
- 记录 Vue 组件结构
- 整理 API 服务和组合式函数

---

## 模块职责

Vue 3 + Vuetify 3 实现的 Web 管理界面，提供：
- 渠道配置和管理
- 实时指标监控
- 多渠道编排（拖拽排序）
- 暗黑/亮色主题切换
- API 密钥管理
- 渠道健康状态展示

## 入口与启动

**主入口**: `src/main.ts`

初始化流程：
1. 创建 Vue 应用实例
2. 注册 Vuetify 插件 (`plugins/vuetify.ts`)
3. 挂载到 `#app` DOM 节点

**启动命令**:
```bash
# 开发模式
bun run dev

# 生产构建
bun run build

# 预览构建结果
bun run preview
```

## 对外接口

### 路由
- **单页应用**: 所有功能在根路由 `/` 下
- **认证流程**: URL 参数 `?key=xxx` 或本地存储的 API 密钥

### 与后端交互
通过 `src/services/api.ts` 封装的 API 客户端：

| 方法 | 端点 | 功能 |
|------|------|------|
| `fetchChannels()` | `GET /api/channels` | 获取渠道列表 |
| `addChannel()` | `POST /api/channels` | 添加渠道 |
| `updateChannel()` | `PUT /api/channels/:id` | 更新渠道 |
| `deleteChannel()` | `DELETE /api/channels/:id` | 删除渠道 |
| `pingChannel()` | `GET /api/ping/:id` | 测试渠道连通性 |
| `pingAllChannels()` | `GET /api/ping` | 测试所有渠道 |
| `reorderChannels()` | `POST /api/channels/reorder` | 调整渠道顺序 |
| `setChannelStatus()` | `PATCH /api/channels/:id/status` | 更新渠道状态 |

## 关键依赖与配置

### 依赖版本
```json
{
  "vue": "^3.5.24",
  "vuetify": "^3.10.11",
  "@mdi/font": "^7.4.47",
  "vuedraggable": "^4.1.0",
  "vite": "^7.2.2",
  "typescript": "^5.9.3"
}
```

### 构建配置
- **构建工具**: Vite 7
- **输出目录**: `dist/`
- **CSS 预处理**: Sass
- **图标库**: Material Design Icons (MDI)

### 环境变量
```bash
# Vite 环境变量（开发模式）
VITE_API_BASE_URL=http://localhost:3000
```

## 数据模型

### Channel 接口
```typescript
interface Channel {
  baseUrl: string
  apiKeys: string[]
  serviceType: 'openai' | 'openaiold' | 'gemini' | 'claude'
  name?: string
  description?: string
  website?: string
  insecureSkipVerify?: boolean
  modelMapping?: Record<string, string>
  priority: number
  status: 'active' | 'suspended' | 'disabled'
  promotionUntil?: string
}
```

### 主题配置
```typescript
interface Theme {
  isDark: boolean
  primaryColor: string
}
```

## 核心组件

### App.vue
**职责**: 根组件，处理认证和布局

核心功能：
- API 密钥验证（URL 参数或本地存储）
- 自动认证加载提示
- 认证对话框展示
- 主题状态管理

### ChannelOrchestration.vue
**职责**: 渠道编排主界面

核心功能：
- 渠道列表展示
- 拖拽排序（vuedraggable）
- 添加/编辑/删除渠道
- 批量 Ping 测试
- 实时指标刷新
- 负载均衡策略切换

### ChannelCard.vue
**职责**: 单个渠道卡片展示

核心功能：
- 渠道基本信息展示
- API 密钥管理（添加/删除/置顶/置底）
- 渠道状态切换（active/suspended/disabled）
- 单个渠道 Ping 测试
- 促销期设置
- 指标数据展示（成功率、请求数）

### ChannelStatusBadge.vue
**职责**: 渠道状态徽章

核心功能：
- 状态颜色映射（active=成功，suspended=警告，disabled=默认）
- 状态文本翻译（启用/暂停/禁用）

### AddChannelModal.vue
**职责**: 添加/编辑渠道对话框

核心功能：
- 表单验证
- 服务类型选择
- API 密钥输入（支持多个）
- 模型映射配置
- SSL 验证选项

## 组合式函数

### useTheme.ts
**职责**: 主题管理

核心功能：
- 暗黑/亮色主题切换
- 主题状态持久化（localStorage）
- Vuetify 主题同步

```typescript
const { isDark, toggleTheme } = useTheme()
```

## 样式和主题

### 主题配置 (plugins/vuetify.ts)
```typescript
const lightTheme = {
  dark: false,
  colors: {
    primary: '#1976D2',
    secondary: '#424242',
    // ...
  }
}

const darkTheme = {
  dark: true,
  colors: {
    primary: '#2196F3',
    secondary: '#616161',
    // ...
  }
}
```

### 全局样式 (assets/style.css)
- Tailwind CSS 基础
- DaisyUI 组件（备用）
- 自定义滚动条样式

### Vuetify 样式 (styles/settings.scss)
- 覆盖 Vuetify 默认样式
- 自定义组件样式

## 测试与质量

### 类型检查
```bash
bun run type-check
```

### 代码规范
- TypeScript 严格模式
- 避免 `any` 类型
- Vue 3 Composition API 风格

### 待改进
- [ ] 添加单元测试（Vitest）
- [ ] 添加 E2E 测试（Playwright）
- [ ] 添加组件文档（Storybook）

## 常见问题 (FAQ)

### Q1: 如何添加新的组件？
1. 在 `src/components/` 创建 `.vue` 文件
2. 使用 `<script setup lang="ts">` 语法
3. 在需要的地方直接 `import` 使用（自动注册）

### Q2: 如何调试 API 请求？
- 打开浏览器开发者工具 → Network 标签页
- 查看 API 请求的 Headers 和 Response
- 确认 `x-api-key` 是否正确传递

### Q3: 为什么主题切换不生效？
- 检查 localStorage 中的 `theme` 键值
- 清除浏览器缓存后重试
- 确认 Vuetify 主题配置已正确同步

### Q4: 如何自定义主题颜色？
编辑 `src/plugins/vuetify.ts` 中的 `lightTheme` 和 `darkTheme` 配置。

### Q5: 拖拽排序不生效？
确认 `vuedraggable` 版本兼容 Vue 3，检查 `v-model` 绑定是否正确。

## 相关文件清单

```
frontend/
├── src/
│   ├── main.ts                       # 主入口
│   ├── env.d.ts                      # TypeScript 环境声明
│   ├── App.vue                       # 根组件
│   ├── components/                   # Vue 组件
│   │   ├── ChannelOrchestration.vue  # 渠道编排主界面
│   │   ├── ChannelCard.vue           # 渠道卡片
│   │   ├── ChannelStatusBadge.vue    # 状态徽章
│   │   └── AddChannelModal.vue       # 添加/编辑对话框
│   ├── services/                     # API 服务
│   │   └── api.ts                    # API 客户端
│   ├── composables/                  # 组合式函数
│   │   └── useTheme.ts               # 主题管理
│   ├── plugins/                      # 插件
│   │   └── vuetify.ts                # Vuetify 配置
│   ├── styles/                       # 样式
│   │   └── settings.scss             # Vuetify 自定义样式
│   └── assets/                       # 静态资源
│       └── style.css                 # 全局样式
├── package.json                      # 依赖配置
├── vite.config.ts                    # Vite 配置
├── tsconfig.json                     # TypeScript 配置
└── index.html                        # HTML 模板
```

## 构建产物

### 开发模式
- 热模块替换（HMR）
- Source Map 支持
- 运行在 `http://localhost:5173`

### 生产构建
- 输出目录: `dist/`
- 代码分割和懒加载
- CSS 压缩和合并
- 资源 Hash 命名

构建产物会被嵌入到 Go 后端的二进制文件中（`embed.FS`）。
