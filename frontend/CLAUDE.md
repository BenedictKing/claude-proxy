# frontend 模块文档

[← 根目录](../CLAUDE.md)

## 模块职责

Vue 3 + Vuetify 3 Web 管理界面：渠道配置、实时监控、拖拽排序、主题切换。

## 启动命令

```bash
bun run dev       # 开发服务器
bun run build     # 生产构建
bun run preview   # 预览构建
```

## 核心组件

| 组件 | 职责 |
|------|------|
| `App.vue` | 根组件，认证和布局 |
| `ChannelOrchestration.vue` | 渠道编排主界面 |
| `ChannelCard.vue` | 渠道卡片（状态、密钥、指标） |
| `AddChannelModal.vue` | 添加/编辑渠道对话框 |

## API 服务

`src/services/api.ts` 封装后端交互：

- `fetchChannels()` / `addChannel()` / `updateChannel()` / `deleteChannel()`
- `pingChannel()` / `pingAllChannels()`
- `reorderChannels()` / `setChannelStatus()`

## 主题配置

编辑 `src/plugins/vuetify.ts` 中的 `lightTheme` 和 `darkTheme`。

## 构建产物

生产构建输出到 `dist/`，会被嵌入到 Go 后端二进制文件中（`embed.FS`）。
