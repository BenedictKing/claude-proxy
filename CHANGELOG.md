# 版本历史

> **注意**: v2.0.0 开始为 Go 语言重写版本，v1.x 为 TypeScript 版本

---

## [Unreleased]

### ✨ 新功能

- **新增 Key 级别使用趋势图表**
  - 支持三种视图模式：流量（Traffic）、Token I/O、缓存 R/W
  - Token I/O 和缓存模式使用双向面积图显示（上方 Input/Read，下方 Output/Creation）
  - **Key 筛选逻辑**：当 key 超过 10 个时，先取最近使用的 5 个，再从其他 key 中按访问量补全到 10 个
  - Key 名称显示前 8 个字符以保持简洁
  - 多 Key 曲线使用不同颜色（蓝、橙、绿、紫、粉）
  - 底部快照卡片实时显示各 Key 的 Input tokens、Output tokens
  - 后端扩展 `RequestRecord` 结构记录 Token 和 Cache 数据
  - 新增 `/api/channels/:id/keys/metrics/history` 和 `/api/responses/channels/:id/keys/metrics/history` 端点
  - 涉及文件：`backend-go/internal/metrics/channel_metrics.go`, `backend-go/internal/handlers/channel_metrics_handler.go`, `backend-go/main.go`, `frontend/src/components/KeyTrendChart.vue`, `frontend/src/services/api.ts`

### 🔧 改进

- **Key 趋势图表自动刷新及线条样式优化**
  - 图表展开时启动每分钟自动刷新，关闭时自动停止
  - 自动刷新跳过正在加载中的请求，避免并发竞争
  - Token I/O 和缓存 R/W 模式下区分线条样式：0轴上方（Input/Read）实线，0轴下方（Output/Write）虚线
  - 修复双向模式下颜色映射错误，同一 Key 的 Input/Output 现在使用相同颜色
  - 涉及文件：`frontend/src/components/KeyTrendChart.vue`
- 优化请求成功记录逻辑，在响应完成后记录 Usage 数据
- 调度器新增 `RecordSuccessWithUsage` 方法支持传递 Token 统计
- **Key 趋势图表 UI 优化**
  - 移除快照卡片中冗余的 RPM 显示，合并 In/Out 到同一行
  - Tooltip 格式简化：`sk-xxx Input: 92.7K` 代替 `sk-xxx (In): 92.7K Input`
  - Y 轴独立缩放：Input 和 Output 使用各自的最大值范围，解决数量级差异大时显示不清晰问题
- **Key 趋势数据粒度优化**
  - 统一使用 1 分钟聚合粒度，提供更精确的时间序列数据
  - 之前：1h=5分钟, 6h=15分钟, 24h=1小时；现在：统一 1 分钟
  - 数据点数量：1h=60点, 6h=360点, 24h=1440点

### 🐛 修复

- **修复渠道编辑时无法清除官网 URL 的问题**
  - 原因：空字符串被转为 `undefined` 导致字段不传递，后端不更新
  - 修复：空字符串正常传递，后端可正确清除已有值

---

## [v2.1.33] - 2025-12-20

### ✨ 新功能

- **新增 Fuzzy Mode（模糊模式）错误处理开关**
  - 默认启用，可通过前端实时切换
  - 启用时：所有非 2xx 错误自动触发 failover，尝试下一个渠道/密钥
  - 启用时：所有渠道失败后返回通用 503 错误，不透传上游错误详情
  - 关闭时：保持原有精确错误分类和透传行为
  - 后端新增 `/api/settings/fuzzy-mode` GET/PUT 端点
  - 前端新增 Fuzzy 模式切换按钮（位于负载均衡策略按钮左侧）
  - 涉及文件：`backend-go/internal/config/config.go`, `backend-go/internal/handlers/config.go`, `backend-go/internal/handlers/proxy.go`, `backend-go/internal/handlers/responses.go`, `frontend/src/App.vue`, `frontend/src/services/api.ts`

- **新增渠道指标历史数据 API 和时间序列图表**
  - 后端新增 `/api/channels/metrics/history` 和 `/api/responses/channels/metrics/history` 端点
  - 支持 `duration` (1h, 6h, 24h) 和 `interval` (5m, 15m, 1h) 参数查询历史指标
  - 新增 `HistoryDataPoint` 结构体，包含请求数、成功数、失败数、成功率
  - 新增 `GetHistoricalStats` 和 `GetAllKeysHistoricalStats` 方法实现数据聚合
  - 涉及文件：`backend-go/internal/handlers/channel_metrics_handler.go`, `backend-go/internal/metrics/channel_metrics.go`, `backend-go/main.go`

- **前端新增渠道指标时间序列图表组件**
  - 引入 ApexCharts 库（apexcharts, vue3-apexcharts）
  - 新增 `ChannelMetricsChart.vue` 组件，支持展开/收起交互
  - 渠道列表中新增图表展开按钮，点击查看使用趋势
  - 新增 `getChannelMetricsHistory` 和 `getResponsesChannelMetricsHistory` API 方法
  - 涉及文件：`frontend/src/components/ChannelMetricsChart.vue`, `frontend/src/components/ChannelOrchestration.vue`, `frontend/src/services/api.ts`

### 🐛 Bug 修复

- **修复历史数据查询的 divide-by-zero 风险**
  - 在 `GetHistoricalStats` 和 `GetAllKeysHistoricalStats` 函数开头添加参数验证
  - 防止 `interval <= 0` 或 `duration <= 0` 时导致的 panic

- **修复历史数据聚合的性能问题**
  - 将双重循环 O(records × buckets) 改为单次遍历 O(records)
  - 使用 map[int64]*bucketData 结构按时间分桶
  - 涉及文件：`backend-go/internal/metrics/channel_metrics.go`

- **修复时间边界对齐问题**
  - 使用 `Truncate(interval)` 对齐时间边界，使桶分布均匀
  - 涉及文件：`backend-go/internal/metrics/channel_metrics.go`

- **移除未使用的代码**
  - 删除 `GetAggregatedMetricsHistory` handler 函数（死代码）
  - 涉及文件：`backend-go/internal/handlers/channel_metrics_handler.go`

### 🔧 改进

- **改进前端图表加载体验**
  - 加载数据时禁用时间范围切换按钮
  - 添加错误 snackbar 通知用户
  - 涉及文件：`frontend/src/components/ChannelMetricsChart.vue`

- **修复边界数据丢失问题**
  - endTime 延伸一个 interval，确保当前时间段的请求也被包含
  - 防止落在 interval 边界的请求丢失
  - 涉及文件：`backend-go/internal/metrics/channel_metrics.go`

- **修复 channelType 切换不刷新问题**
  - 添加对 channelType 的 watch，切换时自动刷新数据
  - 防止跨渠道数据残留
  - 涉及文件：`frontend/src/components/ChannelMetricsChart.vue`

- **修复事件冒泡问题**
  - 官网链接按钮添加 `@click.stop` 修饰符
  - 防止点击链接时触发图表展开
  - 涉及文件：`frontend/src/components/ChannelOrchestration.vue`

- **修复当前时间段数据丢失问题**
  - numPoints 增加 1，确保当前时间段的数据被正确包含
  - 防止图表不显示最近的流量数据
  - 涉及文件：`backend-go/internal/metrics/channel_metrics.go`

- **修复 ChannelOrchestration 切换类型不刷新问题**
  - 添加对 channelType 的 watch，切换时刷新指标并收起图表
  - 防止 Messages/Responses 切换时显示旧数据
  - 涉及文件：`frontend/src/components/ChannelOrchestration.vue`

- **添加 interval 参数最小值限制**
  - 限制 interval 最小值为 1 分钟，防止生成过多 bucket
  - 防止恶意请求导致服务器资源耗尽
  - 涉及文件：`backend-go/internal/handlers/channel_metrics_handler.go`

- **修复 endTime 边界数据丢失问题**
  - 使用 `Before(endTime)` 替代 `!After(endTime)`，排除恰好落在边界的记录
  - 防止 offset 越界导致数据丢失
  - 涉及文件：`backend-go/internal/metrics/channel_metrics.go`

- **修复空桶成功率误导问题**
  - 空桶成功率默认值从 100% 改为 0%
  - 避免无请求时显示 100% 成功率造成误导
  - 涉及文件：`backend-go/internal/metrics/channel_metrics.go`

## [v2.1.32] - 2025-12-19

### 🐛 Bug 修复

- **修复编辑渠道弹窗中基础 URL 布局和验证问题**
  - 使用 `hide-details="auto"` 保留 URL 验证错误显示
  - 使用独立 `<div class="base-url-hint">` 显示"预期请求"提示，避免输入时布局跳动
  - 新增 `baseUrlHasError` 计算属性，有错误时隐藏预期请求提示
  - 修复编辑模式下 `formBaseUrlPreview` 未立即同步导致提示不显示的问题
  - 涉及文件：`frontend/src/components/AddChannelModal.vue`

### ✨ 新功能

- **改进快速添加渠道的 API Key 识别算法**
  - 支持通用 `xx-xxx` 和 `xx_xxx` 前缀格式（如 `ut_xxx`、`sk-xxx`、`api-xxx`）
  - 支持 JWT 格式识别（`eyJ` 开头）
  - 保留 Google API Key 格式（`AIza` 开头）和长字符串（≥32字符）识别
  - 涉及文件：`frontend/src/components/AddChannelModal.vue`

- **新增 API Key 和 URL 识别单元测试**
  - 38 个测试用例覆盖各种 API Key 格式和 URL 验证场景
  - 新增文件：`frontend/src/components/__tests__/quickInputParser.test.ts`

### 🔧 重构

- **提取快速输入解析工具到独立模块**
  - 将 `isValidApiKey`、`isValidUrl`、`parseQuickInput` 从组件提取到 `utils/quickInputParser.ts`
  - 组件和测试共用同一套解析逻辑，避免代码重复和维护性问题
  - 修复 `isValidUrl` 正则不支持无路径 `#` 结尾 URL 的问题（如 `https://api.example.com#`）
  - 新增文件：`frontend/src/utils/quickInputParser.ts`

---

## [v2.1.31] - 2025-12-19

### ✨ 新功能

- **前端显示版本号和更新检查**
  - 在顶部应用栏显示当前运行版本号
  - 自动检查 GitHub 最新版本，有更新时显示橙色提示
  - 点击版本号可跳转 GitHub Release 页面
  - 30 分钟 localStorage 缓存，错误状态 5 分钟短期缓存
  - 新增文件：`frontend/src/services/version.ts`
  - 涉及文件：`frontend/src/App.vue`, `frontend/src/services/api.ts`, `frontend/vite.config.ts`

---

## [v2.1.30] - 2025-12-19

### ✨ 新功能

- **强制探测模式 (Force Probe Mode)**
  - 问题：网络故障导致所有 Key 进入熔断状态后，即使网络恢复，系统仍跳过所有熔断 Key，无法自动恢复服务
  - 解决：当检测到渠道所有 Key 都被熔断时，自动启用强制探测模式，忽略熔断状态强制尝试请求
  - 日志标识：`🔍 [强制探测] 渠道 xxx 所有 Key 都被熔断，启用强制探测模式`
  - 涉及文件：`handlers/proxy.go`, `handlers/responses.go`

### 🐛 Bug 修复

- **修复 Responses 渠道缺少熔断检查的问题**
  - `tryResponsesChannelWithAllKeys` 和 `tryCompactChannelWithAllKeys` 现在也会检查 Key 熔断状态
  - 与 Messages 渠道行为保持一致

### 🗑️ 移除

- **移除基础 URL 输入框下方的固定提示文本**
  - 移除 "通常为: https://api.openai.com/v1" 等冗余提示
  - 原因：输入框已有 placeholder 提示，且下方已显示预期请求 URL 预览
  - 删除未使用的 `getUrlHint()` 函数
  - 涉及文件：`frontend/src/components/AddChannelModal.vue`

---

## [v2.1.28] - 2025-12-19

### ✨ 新功能

- **BaseURL 支持 `#` 结尾跳过自动添加 `/v1`**
  - 问题：部分 API 服务端点不带 `/v1` 前缀，但系统会自动添加
  - 解决：在 BaseURL 末尾添加 `#` 标记，系统将跳过自动添加 `/v1`
  - 示例：`https://api.example.com#` → 请求 `https://api.example.com/chat/completions`
  - 示例：`https://api.example.com` → 请求 `https://api.example.com/v1/chat/completions`
  - 涉及文件：`providers/openai.go`, `providers/claude.go`, `providers/responses.go`

- **前端快速添加显示预期请求 URL**
  - 在快速添加模式下显示实际请求的完整 URL
  - 支持识别 `#` 结尾的 BaseURL
  - 涉及文件：`frontend/src/components/AddChannelModal.vue`

---

## [v2.1.27] - 2025-12-19

### ♻️ 重构

- **移除 Claude Provider 畸形 tool_call 修复逻辑**
  - 删除 `toolCallFixer` 相关代码，简化流式响应处理
  - 移除未使用的 `fmt`、`log`、`time` 导入
  - 涉及文件：`internal/providers/claude.go`

---

## [v2.1.26] - 2025-12-19

### ✨ 新功能

- **Responses 渠道新增模型选项**
  - 添加 `gpt-5.2-codex` 到模型重定向下拉列表

---

## [v2.1.24] - 2025-12-17

### ✨ 新功能

- **Responses 渠道新增模型选项**
  - 添加 `gpt-5.2`、`gpt-5` 到源模型下拉列表

### ♻️ 重构

- **移除 openaiold 服务类型支持**
  - 删除不再使用的 OpenAI 旧版 API (completions) 支持
  - 简化前后端代码和文档，共删除 30 行冗余代码

---

## [v2.1.23] - 2025-12-13

### 🐛 Bug 修复

- **修复 402 状态码未触发 failover 的问题**
  - 问题：促销渠道返回 402 (Payment Required) 错误后，不会尝试其他密钥或渠道
  - 原因：`shouldRetryWithNextKey` 函数未处理 402 状态码
  - 修复：将 402 纳入 failover 重试逻辑，标记为额度相关错误并触发密钥/渠道切换

### 🧪 测试

- **新增 failover 分类逻辑单元测试**
  - `TestClassifyByStatusCode`: 状态码分类测试（20 个用例）
  - `TestClassifyMessage`: 错误消息分类测试（20 个用例）
  - `TestClassifyErrorType`: 错误类型分类测试（14 个用例）
  - `TestClassifyByErrorMessage`: 响应体解析测试（7 个用例）
  - `TestShouldRetryWithNextKey`: 完整重试逻辑测试（9 个用例）
  - 新增文件：`backend-go/internal/handlers/failover_test.go`

### ♻️ 重构

- **重构 HTTP 状态码 failover 判断逻辑**
  - 将原有的"打补丁式" if-else 改为系统化的两层分类策略
  - **第一层 - 状态码分类** `classifyByStatusCode()`:
    - 401/403: failover + 非配额（认证问题）
    - 402/429: failover + 配额相关（余额/限流）
    - 408: failover + 非配额（上游超时）
    - 5xx: failover + 非配额（服务端故障）
    - 400: 交给第二层判断
    - 其他 4xx: 不 failover（客户端请求问题）
  - **第二层 - 消息体分类** `classifyByErrorMessage()`:
    - 返回 `(shouldFailover, isQuotaRelated)` 双值
    - `classifyMessage()`: 基于错误消息关键词分类
    - `classifyErrorType()`: 基于错误类型分类
  - **关键词分类**:
    - 配额类: insufficient, quota, credit, balance, rate limit 等
    - 认证类: invalid, unauthorized, api key, token, expired 等
    - 临时类: timeout, overloaded, unavailable, retry 等
  - 影响文件：`backend-go/internal/handlers/proxy.go`

---

## [v2.1.20] - 2025-12-12

### ✨ 新功能

- **渠道名称支持点击打开编辑弹窗**
  - 故障转移序列和备用资源池中的渠道名称现在可点击，与密钥数量点击行为一致
  - 新增 hover 视觉反馈：鼠标指针变手型、文字变主题色并显示下划线
  - 支持键盘可访问性：`tabindex`、`role="button"`、Enter/Space 键触发、`:focus-visible` 轮廓样式
  - 影响文件：`frontend/src/components/ChannelOrchestration.vue`

---

## [v2.1.19] - 2025-12-12

### 🐛 Bug 修复

- **修复添加渠道弹窗密钥重复错误状态残留问题**
  - 切换渠道弹窗时，之前的"该密钥已存在"错误提示不会被清理
  - 修复：在弹窗打开时统一清理 `apiKeyError` 和 `duplicateKeyIndex` 状态
  - 影响文件：`frontend/src/components/AddChannelModal.vue`

- **修复备用池渠道卡片在移动端的显示问题**
  - 问题：描述文本过长时导致卡片溢出，窄屏下布局混乱
  - 修复：备用池改为所有宽度下自动填充布局（`auto-fill`）
  - 影响文件：`frontend/src/components/ChannelOrchestration.vue`

- **优化移动端页面布局间距**
  - 减少主容器左右内边距（16px → 8px）
  - 减少统计卡片之间的间距
  - 影响文件：`frontend/src/App.vue`

- **优化渠道编排响应式布局**
  - 960px 以下：恢复显示健康度指标和密钥数量
  - 600px 以下：隐藏健康度指标和密钥数量，保持精简
  - 影响文件：`frontend/src/components/ChannelOrchestration.vue`

### ✨ 新功能

- **新增 `/v1/responses/compact` 端点**
  - 支持 OpenAI Responses API 的上下文压缩功能，用于长期代理工作流
  - 完整的 Key 轮转：每个渠道尝试所有可用 API Key，401/429/配额错误自动切换
  - 多渠道故障转移：复用 `shouldRetryWithNextKey` 逻辑判断是否切换渠道
  - 会话亲和性：提取 `userID` 用于 Trace 亲和，记录成功/失败到调度器
  - 请求体大小限制：复用 `MaxRequestBodySize` 配置，防止内存耗尽
  - 影响文件：`backend-go/internal/handlers/responses.go`, `backend-go/main.go`

---

## [v2.1.14] - 2025-12-12

### 🐛 Bug 修复

- **修复流式响应 Token 计数中间更新被覆盖的问题**
  - 问题：当流中有多个 usage 事件时，中间事件的更准确 Token 值会被最终事件的旧值覆盖
  - 场景：`message.usage(875)` → `message.usage(1003)` → `顶层usage(875)`，最终错误使用 875
  - 修复：
    1. 收集逻辑改为取最大值，避免后续事件覆盖已收集的更大值
    2. 修补逻辑增强：不仅在值 <= 1 时修补，还在收集值 > 当前值时也进行修补
    3. 保持缓存 token 守卫：缓存请求合法报告 `input_tokens` 为 0/1，不应被覆盖
  - 影响文件：`backend-go/internal/handlers/proxy.go`

---

## [v2.1.13] - 2025-12-11

### 🔧 改进

- **Token 补全日志改为 debug 级别**
  - 将 `[Stream-Token检测]`、`[Stream-Token补全]`、`[Stream-Token统计]` 等日志从 info 改为 debug
  - 条件：`EnableResponseLogs=true` **且** `LOG_LEVEL=debug` 时输出
  - 保持与 `EnableResponseLogs` 开关的兼容性，不会绕过原有的日志控制

---

## [v2.1.12] - 2025-12-11

### ✨ 新功能

- **支持 Claude 缓存 Token 计数**
  - `Usage` 类型新增 `cache_creation_input_tokens` 和 `cache_read_input_tokens` 字段
  - 完整支持 Claude API 的 Prompt Caching 功能统计
  - Token 补全逻辑智能识别缓存场景：当有缓存 token 时，`input_tokens` 为 0/1 是正常的（缓存命中），不再触发补全

### 🐛 Bug 修复

- **修复流式响应缓存 Token 判断逻辑**
  - 问题：`patchUsageFieldsWithLog` 从当前事件读取缓存 token 来判断是否跳过补全
  - 但 Claude 的 `message_delta`/`message_stop` 事件通常**不包含**缓存字段（仅出现在 `message_start`）
  - 导致缓存请求的 `input_tokens` 被错误地估算覆盖
  - 修复：从 `ctx.collectedUsage`（流全程收集的缓存信息）传入 `hasCacheTokens` 参数

### 🔧 改进

- **Token 日志输出完善**
  - 非流式响应：输出完整的 token 统计（`input`, `output`, `cache_creation`, `cache_read`）
  - 流式响应：同样输出完整的 token 统计
  - 日志格式统一，便于监控和分析

### 📝 技术细节

- 修改文件：
  - `internal/types/types.go` - 扩展 Usage 结构体
  - `internal/handlers/proxy.go` - 更新 token 补全逻辑和日志输出

---

## [v2.1.10] - 2025-12-11

### 🐛 Bug 修复

- **修复流式响应 Token 计数补全逻辑**
  - 问题：上游返回 `output_tokens=1`（虚假值）时未触发补全，因为之前只检测 `== 0`
  - 问题：`message_start` 事件检测到虚假值后立即修补，但此时 `outputTextBuffer` 为空导致估算为 0
  - 修复：将 `output_tokens` 补全条件从 `== 0` 改为 `<= 1`（与 `input_tokens` 一致）
  - 修复：延迟到 `message_delta` 或 `message_stop` 事件时修补，确保内容已完整累积
  - 新增 `isMessageDeltaEvent()` 函数检测流结束事件

### 🔧 改进

- **Token 补全日志改进**
  - 新增详细的 Token 检测和补全日志（仅在 `EnableResponseLogs=true` 时输出）
  - 日志标签：`[Token补全]`、`[Stream-Token检测]`、`[Stream-Token修补]`
  - 所有新增日志使用 `EnableResponseLogs` 开关，避免生产环境日志过多

---

## [v2.1.9] - 2025-12-11

### 🧹 代码重构

- **修复代码质量问题**
  - `handlers/proxy.go:280` - 添加 `DeprioritizeAPIKey` 错误日志，避免静默忽略
  - `handlers/proxy.go:629-643` - 使用 `c.Request.Context()` 替代已废弃的 `CloseNotify()`，修复潜在 goroutine 泄漏
  - **DRY 重构**: 删除 3 处重复的 `maskAPIKey` 函数，统一使用 `utils.MaskAPIKey`
    - 删除 `config/config.go:989-1005`
    - 删除 `handlers/proxy.go:897-913`
    - 删除 `metrics/channel_metrics.go:111-116`

---

## [v2.1.8] - 2025-12-11

### 🧹 代码重构

- **重构过长方法，提升代码可读性和可维护性**
  - `config/config.go:loadConfig` (130行 → 5个函数)：
    - 提取 `createDefaultConfig()` - 创建默认配置
    - 提取 `applyConfigDefaults()` - 应用配置默认值
    - 提取 `migrateOldFormat()` - 旧格式迁移检测
    - 提取 `migrateUpstreams()` - 单渠道列表迁移（消除重复代码）
  - `handlers/proxy.go:handleStreamResponse` (145行 → 10个函数)：
    - 引入 `streamContext` 结构体封装流状态
    - 提取 `setupStreamHeaders()` - 设置响应头
    - 提取 `processStreamEvents()` - 事件循环
    - 提取 `processStreamEvent()` - 单事件处理
    - 提取日志辅助函数：`logStreamCompletion()`, `logPartialResponse()`, `logSynthesizedContent()`
    - 提取 `isClientDisconnectError()` - 断连错误判断
  - 遵循 SOLID/KISS/DRY 原则

---

## [v2.1.7] - 2025-12-11

### 🐛 Bug 修复

- **修复前端 MDI 图标无法显示的问题**：`vite-plugin-vuetify` 的 `autoImport` 会覆盖手动配置的 `mdi-svg` 图标设置
  - 问题原因：`mdi-xxx` 字符串被错误地当作 SVG path 数据解析，导致控制台报错 `Expected number, "mdi-xxx"`
  - 解决方案：创建自定义 `IconSet` 组件手动处理图标名称到 SVG path 的映射，同时保留 `vite-plugin-vuetify`（`autoImport: false`）以加载 SCSS 样式配置
  - 修改文件：
    - `frontend/vite.config.ts` - 设置 `autoImport: false`，保留 SCSS 配置加载
    - `frontend/src/plugins/vuetify.ts` - 实现自定义 SVG iconset
    - `frontend/src/App.vue` - 修复 `currentChannelIndex` prop 类型警告
  - 优点：按需加载图标，避免打包整个字体文件，保持小体积

---

## [v2.1.4] - 2025-12-11

### 🐛 Bug 修复

- **修复前端渠道健康度统计不显示数据的问题**：后端 `GetChannelMetricsWithConfig` API 遗漏了 `timeWindows` 字段
  - 问题原因：`metricsManager.ToResponse()` 已正确计算分时段统计数据，但 handler 构建 JSON 响应时未包含该字段
  - 修复文件：`backend-go/internal/handlers/channel_metrics_handler.go:41`
  - 影响：前端 `ChannelOrchestration.vue` 中的 15m/1h/6h/24h 成功率和请求数现在可正常显示

---

## [v2.1.1] - 2025-12-11

### ✨ 新功能

- **新增 `QUIET_POLLING_LOGS` 环境变量**：设为 `true` 时静默前端轮询端点的认证成功日志，避免调试时日志刷屏
  - 受影响端点：`/api/channels`、`/api/channels/metrics`、`/api/channels/scheduler/stats`
  - 默认值：`false`（保持原有行为）

---

## [v2.0.20-go] - 2025-12-08

### 🐛 Bug 修复

- **修复单渠道模式渠道选择逻辑**：`disabled` 状态的渠道不再被错误选中，现在优先选择第一个 `active` 渠道

### 🧹 代码清理

- 移除废弃的 `currentUpstream` 相关代码和 API 接口

---

## [v2.0.11-go] - 2025-12-06

### 🚀 重大功能

#### 多渠道智能调度器

新增完整的多渠道调度系统，支持智能故障转移和负载均衡：

**核心模块**：

- **ChannelScheduler** (`internal/scheduler/channel_scheduler.go`)
  - 基于优先级的渠道选择
  - Trace 亲和性支持（同一用户会话绑定到同一渠道）
  - 失败率检测和自动熔断
  - 降级选择（选择失败率最低的渠道）

- **MetricsManager** (`internal/metrics/channel_metrics.go`)
  - 滑动窗口算法计算实时成功率
  - 可配置窗口大小（默认 10 次请求）
  - 可配置失败率阈值（默认 50%）
  - 自动熔断和恢复机制
  - 熔断自动恢复（默认 15 分钟后自动尝试恢复）
  - 熔断时间戳记录（`circuitBrokenAt` 字段）

- **TraceAffinityManager** (`internal/session/trace_affinity.go`)
  - 用户会话与渠道绑定
  - TTL 自动过期（默认 30 分钟）
  - 定期清理过期记录

**调度优先级**：
1. Trace 亲和性（优先使用用户之前成功的渠道）
2. 健康检查（跳过失败率过高的渠道）
3. 优先级顺序（数字越小优先级越高）
4. 降级选择（所有渠道都不健康时选择最佳的）

#### 渠道状态管理

新增渠道状态字段，支持三种状态：

| 状态 | 说明 |
|------|------|
| `active` | 正常运行，参与调度 |
| `suspended` | 暂停状态，保留在故障转移序列但跳过 |
| `disabled` | 备用池，不参与调度 |

> ⚠️ **注意**：`suspended` 是配置层面的状态，需手动恢复；运行时熔断会在 15 分钟后自动恢复。

**配置字段扩展**：
- `priority` - 渠道优先级（数字越小优先级越高）
- `status` - 渠道状态（active/suspended/disabled）

**向后兼容**：
- 旧配置文件自动迁移到新格式
- `currentUpstream` 字段自动转换为 status 状态

#### 渠道密钥自检

- 启动时自动检测无 API Key 的渠道
- 无 Key 渠道自动设置为 `suspended` 状态
- 防止因配置错误导致请求失败

### 🎨 前端 UI

#### 渠道编排面板

新增 `ChannelOrchestration.vue` 组件：

- **拖拽排序**: 通过拖拽调整渠道优先级，自动保存
- **实时指标**: 显示成功率、请求数、延迟等指标
- **状态切换**: 一键切换 active/suspended/disabled 状态
- **备用池管理**: 独立管理备用渠道
- **多渠道/单渠道模式**: 自动检测并显示当前模式

#### 渠道状态徽章

新增 `ChannelStatusBadge.vue` 组件：

- 实时显示渠道健康状态
- 颜色编码：绿色（健康）、黄色（警告）、红色（熔断）
- 悬停显示详细指标

#### 响应式 UI 优化

- 移动端适配优化
- 复古像素主题增强
- 暗色模式操作栏背景色适配

### 🔧 技术改进

#### API 端点

- `GET /api/channels/metrics` - 获取 Messages 渠道指标
- `GET /api/responses/channels/metrics` - 获取 Responses 渠道指标
- `POST /api/channels/:id/resume` - 恢复熔断渠道
- `POST /api/responses/channels/:id/resume` - 恢复 Responses 熔断渠道
- `GET /api/scheduler/stats` - 获取调度器统计信息（含熔断恢复时间）
- `PATCH /api/channels/:id` - 更新渠道配置（支持 priority/status）
- `PATCH /api/channels/order` - 批量更新渠道优先级顺序

#### CORS 增强

- 支持 PATCH 方法
- OPTIONS 预检请求返回 204

#### 代理目标配置

- 新增 `VITE_PROXY_TARGET` 环境变量
- 前端开发时可配置后端代理目标

### 📝 技术细节

**新增模块**：

| 模块 | 路径 | 职责 |
|------|------|------|
| **调度器** | `internal/scheduler/` | 多渠道调度逻辑 |
| **指标** | `internal/metrics/` | 渠道健康度指标 |
| **亲和性** | `internal/session/trace_affinity.go` | 用户会话亲和 |

**架构图**：

```
请求 → 调度器选择渠道 → 执行请求 → 记录指标
           ↓                          ↓
     Trace亲和检查              成功/失败统计
           ↓                          ↓
     健康度检查                 滑动窗口更新
           ↓                          ↓
     优先级排序                 熔断判断
```

---

## [v2.0.10-go] - 2025-12-06

### 🎨 UI 重构

#### 复古像素 (Retro Pixel) 主题

采用 **Neo-Brutalism** 设计语言，完全重构前端样式：

- **无圆角**: 全局 `border-radius: 0`
- **等宽字体**: `Courier New`, `Consolas`, `Liberation Mono`
- **粗实体边框**: `2px solid` 黑色/白色边框
- **硬阴影**: `box-shadow: Npx Npx 0 0` 偏移阴影（无模糊）
- **按压交互**: hover 上浮 + active 按压效果
- **高对比度状态标签**: 实心背景 + 实体边框
- **复古纸张背景**: 亮色模式使用 `#fffbeb`

### 🔧 技术变更

- 移除 DaisyUI 依赖
- 移除玻璃拟态 (Glassmorphism) 效果
- 简化主题配置 (`useTheme.ts`)

---

## [v2.0.9-go] - 2025-12-04

### ✨ 新功能

- 新增 API 密钥排序功能：支持将最后一个密钥置顶、第一个密钥置底
- 前端 API 密钥列表显示置顶/置底按钮（仅当密钥数量 > 1 时）

---

## [v2.0.8-go] - 2025-12-04

### 🐛 Bug 修复

- 修复 429 速率限制错误不触发密钥切换的问题
- 新增中文错误消息 "请求数限制" 的识别支持

---

## [v2.0.7-go] - 2025-11-22

### ✨ 改进

- Codex Responses 负载均衡独立配置：新增 `responsesLoadBalance` 字段和 `/api/responses/loadbalance` 路由，前端在 Codex 标签页单独设置策略，不再影响 Claude 渠道。
- 置顶状态分离：Codex 管理页置顶改用 `codex-proxy-pinned-channels`，不再与 Claude 共享 localStorage。

### 🔧 兼容性

- 旧配置文件若未包含 `responsesLoadBalance` 将自动回退到现有 `loadBalance`，无需手工迁移。

---

## [v2.0.6-go] - 2025-11-18

### 🐛 Bug 修复

#### Responses API 透传模式修复

- **问题**: 透传模式下字段丢失和零值字段污染
  - ❌ 原始请求中的高级字段丢失（`tools`, `tool_choice`, `reasoning`, `metadata`, `betas`）
  - ❌ 实际请求中添加了不存在的零值字段（`frequency_penalty: 0`, `temperature: 0`, `max_tokens: 0`）
  - ❌ 导致上游 API 返回参数错误

- **根因**: `ResponsesPassthroughConverter` 通过 Go 结构体字段映射，而非真正的 JSON 透传
  - 结构体定义不完整，缺少高级字段定义
  - 所有结构体字段都被序列化，包括零值字段

- **修复方案** (`internal/providers/responses.go`)
  - ✅ 透传模式下使用 `map[string]interface{}` 解析原始请求
  - ✅ 保留所有原始字段，不经过结构体映射
  - ✅ 不添加任何零值字段
  - ✅ 只执行必要的模型重定向
  - ✅ 非透传模式保持原有逻辑（结构体 + 会话管理 + 转换器）

- **影响范围**: 仅影响 `serviceType: "responses"` 的上游配置

#### 日志显示优化

- **问题**: Responses API 的 `input`/`output` 字段内容在日志中被简化
  - ❌ 只显示 `{"type": "input_text"}`，实际文本内容丢失
  - ❌ 无法通过日志调试消息内容

- **根因**: `utils/json.go` 的日志格式化函数遗漏了 Responses API 特有类型
  - `compactContentArray()` 只处理 Messages API 类型（`text`, `tool_use`, `tool_result`, `image`）
  - 没有处理 Responses API 的 `input_text` 和 `output_text` 类型

- **修复方案** (`internal/utils/json.go`)
  - ✅ 在 `compactContentArray()` switch 语句中添加 `input_text`/`output_text` case
  - ✅ 保留 `text` 字段内容（超过 200 字符自动截断）
  - ✅ 在 `formatJSONWithCompactArrays()` 中添加类型识别

- **影响范围**: 所有使用 `FormatJSONBytesForLog()` 的日志输出

### 🎯 修复效果

**透传模式**：
```diff
# 修复前
- "tools": 字段丢失
- "reasoning": 字段丢失
+ "frequency_penalty": 0  # 不应添加
+ "temperature": 0        # 不应添加

# 修复后
+ "tools": [...]          # 完整保留
+ "reasoning": {...}      # 完整保留
- 不添加零值字段
```

**日志显示**：
```diff
# 修复前
"input": [{"type": "input_text"}]

# 修复后
"input": [{"type": "input_text", "text": "完整的消息内容..."}]
```

### 📝 技术细节

- **文件修改**:
  - `backend-go/internal/providers/responses.go` (第 31-85 行)
  - `backend-go/internal/utils/json.go` (第 112-120, 369-370 行)

- **符合原则**:
  - ✅ KISS - 透传使用 map，不过度设计
  - ✅ DRY - 复用现有转换器工厂和类型判断
  - ✅ YAGNI - 最小改动，不影响其他模块

---

## [v2.0.5-go] - 2025-11-15

### 🚀 重大重构

#### Responses API 转换器架构重构

- **新增转换器接口** (`internal/converters/converter.go`)
  - 定义统一的 `ResponsesConverter` 接口
  - 支持双向转换：Responses ↔ 上游格式
  - 清晰的职责分离和扩展性

- **策略模式 + 工厂模式实现**
  - `OpenAIChatConverter` - Responses → OpenAI Chat Completions
  - `OpenAICompletionsConverter` - Responses → OpenAI Completions
  - `ClaudeConverter` - Responses → Claude Messages API
  - `ResponsesPassthroughConverter` - Responses → Responses (透传)
  - `ConverterFactory` - 根据上游类型自动选择转换器

- **完整支持 Responses API 标准格式**
  - ✅ `instructions` 字段 - 映射为 system message
  - ✅ 嵌套 `content` 数组 - 支持 `input_text`/`output_text` 类型
  - ✅ `type: "message"` 格式 - 区分 message 和 text 类型
  - ✅ `role` 字段 - 直接从 item.role 获取角色
  - ❌ 移除 `[ASSISTANT]` 前缀 hack - 使用标准 role 字段

### ✨ 新功能

- **内容提取函数** (`extractTextFromContent`)
  - 支持三种格式：string、[]ContentBlock、[]interface{}
  - 自动提取 input_text 和 output_text 类型
  - 智能拼接多个文本块

- **类型定义增强**
  - `ResponsesRequest.Instructions` - 系统指令字段
  - `ResponsesItem.Role` - 角色字段（user/assistant）
  - `ContentBlock` - 内容块结构体（type + text）

### 🔧 代码改进

- **ResponsesProvider 简化**
  - 使用工厂模式替代 switch-case
  - 统一的请求转换流程
  - 减少代码重复（从 ~260 行减少到 ~130 行）

- **测试覆盖**
  - 10 个单元测试全部通过
  - 覆盖核心转换逻辑
  - 测试 instructions、message type、会话历史等场景

### 📚 架构优势

- **易于扩展** - 新增上游只需实现 ResponsesConverter 接口
- **职责清晰** - 转换逻辑与 Provider 解耦
- **可测试性** - 每个转换器可独立测试
- **代码复用** - 公共逻辑提取到基础函数

### ⚠️ 破坏性变更

- **移除向后兼容** - 不再支持 `[ASSISTANT]` 前缀
- **函数签名变更**
  - `ResponsesToClaudeMessages` 新增 `instructions` 参数
  - `ResponsesToOpenAIChatMessages` 新增 `instructions` 参数

### 📖 参考

本次重构参考了 [AIClient-2-API](https://github.com/example/AIClient-2-API) 项目的转换策略设计，特别是：
- Responses API 格式的完整实现
- 策略模式 + 工厂模式的架构设计
- instructions → system message 的映射逻辑

---

## [v2.0.4-go] - 2025-11-14

### ✨ 新功能

#### Responses API 透明转发支持

- **Codex Responses API 端点** (`/v1/responses`)
  - 完整支持 Codex Responses API 格式
  - 透明转发到上游 Responses API 服务
  - 支持流式和非流式响应
  - 自动协议转换和错误处理
  - 与 Messages API 相同的负载均衡和故障转移机制

- **会话管理系统** (`internal/session/`)
  - 自动会话创建和多轮对话跟踪
  - 基于 `previous_response_id` 的会话关联
  - 消息历史自动管理（默认限制 100 条消息）
  - Token 使用统计（默认限制 100k tokens）
  - 自动过期清理机制（默认 24 小时）
  - 线程安全的并发访问支持

- **Responses Provider** (`internal/providers/responses.go`)
  - 实现 Responses API 协议转换
  - 支持 `input` 字段（字符串或数组格式）
  - 响应包含 `id` 和 `previous_id` 链接
  - 自动处理 `store` 参数控制会话存储
  - 完整的流式响应支持

- **独立渠道管理**
  - Responses 渠道与 Messages 渠道完全独立
  - 独立的渠道配置和 API 密钥管理
  - 支持通过 Web UI 和管理 API 配置
  - 独立的负载均衡策略

#### Messages API 协议转换增强

- **多上游协议支持**
  - Claude API (Anthropic) - 原生支持，直接透传
  - OpenAI API - 自动双向转换 (Claude ↔ OpenAI 格式)
  - OpenAI 兼容 API - 支持所有 OpenAI 格式兼容服务
  - Gemini API (Google) - 自动双向转换 (Claude ↔ Gemini 格式)

- **统一客户端接口**
  - 客户端只需使用 Claude Messages API 格式
  - 代理自动识别上游类型并转换协议
  - 无需修改客户端代码即可切换不同 AI 服务
  - 支持灵活的成本优化和服务切换

#### Web UI 标题栏 API 类型切换

- **集成式 API 类型切换器**
  - 在标题栏中显示 `Claude / Codex API Proxy` 格式
  - 点击 "Claude" 切换到 Messages API 渠道
  - 点击 "Codex" 切换到 Responses API 渠道
  - 移除了独立的 Tab 切换卡片，节省垂直空间

- **视觉高亮设计**
  - 激活选项显示下划线高亮效果
  - 激活选项字体加粗（font-weight: 900）
  - 未激活选项降低透明度（opacity: 0.55）
  - 悬停时透明度提升并轻微上浮动画

- **统一数据管理**
  - 自动同步切换所有统计卡片数据
  - 当前渠道、负载均衡策略、渠道列表随 Tab 切换更新
  - 保持用户操作的连贯性

### 🎨 UI/UX 优化

- **空间利用优化**
  - 移除独立 Tab 卡片，UI 更紧凑
  - 标题栏集成切换功能，减少视觉干扰
  - 提升页面内容展示空间

- **交互体验提升**
  - 平滑过渡动画（0.18s ease）
  - 悬停反馈（透明度 + 位移）
  - 清晰的视觉状态反馈

### 📝 技术改进

- **架构增强**
  - 新增 Session Manager 模块支持有状态会话
  - Responses Handler 实现完整的请求/响应生命周期
  - ResponsesProvider 遵循统一的 Provider 接口规范
  - 所有 Responses 相关功能均支持故障转移和密钥降级

- **代码简化**
  - 移除多余的 Tab 组件代码
  - 简化 CSS 样式，仅保留必要的下划线高亮风格
  - 提升代码可维护性

- **响应式设计**
  - 支持移动端和桌面端自适应
  - 字体大小根据屏幕尺寸调整（text-h6/text-h5）
  - 保持在不同设备上的良好体验

- **API 端点扩展**
  - `/v1/responses` - Responses API 主端点
  - `/api/responses/channels` - Responses 渠道管理
  - `/api/responses/channels/:id/keys` - Responses 密钥管理
  - `/api/responses/channels/:id/current` - 设置当前 Responses 渠道

### 🔧 其他改进

- **版本管理**
  - 统一版本号至 v2.0.4-go
  - 更新 VERSION 文件和 package.json
  - 完善更新日志文档

---

## [v2.0.3-go] - 2025-10-13

### 🐛 Bug 修复

#### 流式响应文本块管理优化

- **修复 OpenAI/Gemini 流式响应文本块状态追踪** (`openai.go`, `gemini.go`)
  - 引入 `textBlockStarted` 状态标志，确保文本块正确开启/关闭
  - 修复连续文本片段导致多个 `content_block_start` 事件的问题
  - 确保在工具调用或流结束前正确关闭文本块
  - 改进 `content_block_stop` 事件的发送时机和条件判断

- **增强 Gemini Provider 的流式事件序列**:
  - 首个文本块才发送 `content_block_start` 事件
  - 后续文本增量统一使用 `content_block_delta` 事件
  - 工具调用前自动关闭未完成的文本块
  - 流结束时确保所有文本块已关闭

- **改进 OpenAI Provider 的事件同步**:
  - 统一文本块和工具调用的事件序列管理
  - 修复工具调用和文本内容交错时的状态混乱
  - 删除冗余的 `processTextPart` 辅助函数（90行代码减少）

#### 请求头处理优化

- **新增 `PrepareMinimalHeaders` 函数** (`headers.go`)
  - 针对非 Claude 类型渠道（OpenAI、Gemini）使用最小化请求头
  - 避免转发 Anthropic 特定头部（如 `anthropic-version`）导致上游拒绝请求
  - 仅保留必要头部：`Host` 和 `Content-Type`
  - 不显式设置 `Accept-Encoding`，由 Go 的 `http.Client` 自动处理 gzip 压缩

- **区分 Claude 和非 Claude 渠道的头部策略**:
  - **Claude 渠道**: 使用 `PrepareUpstreamHeaders`（保留原始请求头）
  - **OpenAI/Gemini 渠道**: 使用 `PrepareMinimalHeaders`（最小化头部）
  - 提升与不同上游 API 的兼容性

#### OpenAI URL 路径智能拼接

- **自动检测 baseURL 版本号后缀** (`openai.go`)
  - 使用正则表达式 `/v\d$` 检测 URL 是否已包含版本号
  - 已包含版本号（如 `/v1`、`/v2`）时直接拼接 `/chat/completions`
  - 未包含版本号时自动添加 `/v1/chat/completions`
  - 支持自定义上游 API 的灵活配置

#### 日志格式优化

- **简化流式响应日志输出** (`proxy.go`)
  - 移除多余的 `---` 分隔符，减少日志噪音
  - 统一日志格式：`🛰️ 上游流式响应合成内容:\n{content}`
  - 减少视觉干扰，提升日志可读性

- **区分客户端断开和真实错误**:
  - 检测 `broken pipe` 和 `connection reset` 错误
  - 客户端中断连接使用 `ℹ️` info 级别日志
  - 其他错误使用 `⚠️` warning 级别日志
  - 仅在 info 日志级别启用时输出客户端断开信息

### 📝 技术改进

- **代码简化**:
  - 删除 OpenAI Provider 中的 `processTextPart` 辅助函数（45行）
  - 状态管理从函数式转为声明式，提升可维护性
  - 减少重复代码，遵循 DRY 原则

- **错误处理增强**:
  - 流式传输错误分级处理（client vs server error）
  - 改进错误日志的上下文信息
  - 在开发模式下提供更详细的调试信息

### ⚡ 性能优化

- **减少不必要的函数调用**:
  - 文本块事件生成从函数调用改为内联代码
  - 减少 JSON 序列化次数
  - 降低 CPU 和内存开销

- **优化请求头处理**:
  - 最小化头部策略减少请求体大小
  - 避免转发无关头部提升网络效率

---

## [v2.0.2-go] - 2025-10-12

### ✨ 新功能

#### API密钥复制功能
- **一键复制密钥**: 在渠道卡片和编辑弹框中为每个API密钥添加复制按钮
  - 视觉反馈：复制成功后显示绿色勾选图标，2秒后自动恢复
  - 工具提示：鼠标悬停显示"复制密钥"，复制后显示"已复制!"
  - 兼容性：支持现代浏览器的 Clipboard API，自动降级到传统方法
  - 位置：
    - 渠道卡片：展开"API密钥管理"面板中每个密钥右侧
    - 编辑弹框：编辑/添加渠道对话框的"API密钥管理"区域

#### 前端认证优化
- **自动登录功能**: 保存的访问密钥自动验证登录
  - 首次访问：输入密钥后自动保存到本地存储
  - 后续访问：页面刷新时自动验证密钥并直接进入系统
  - 密钥失效：自动检测并提示用户重新输入
  - 加载提示：显示"正在验证访问权限"加载遮罩，提升用户体验

- **移除后端内置登录页面**: 统一由前端Vue应用处理认证
  - 删除Go后端的HTML登录页面（`getAuthPage()`函数）
  - 优化认证中间件：页面请求直接提供Vue应用，API请求才检查密钥
  - 解决双重登录对话框问题，提升用户体验

### 🎨 UI/UX 优化

- **统一视觉风格**: 复制和删除按钮在两处位置保持一致的布局和交互
- **智能状态管理**: 复制状态独立管理，不干扰其他功能
- **密钥掩码显示**: 保持密钥的安全性，只在复制时使用完整密钥

### 🐛 Bug 修复

- **修复双重登录框问题**:
  - 后端不再返回简单的HTML登录页面
  - 前端Vue应用完全接管认证流程
  - 页面加载时不会出现登录框闪烁

- **修复初始化时序问题**:
  - 添加 `isInitialized` 标志控制对话框显示时机
  - 优化自动认证的异步处理逻辑

### 📝 技术改进

- **前端状态管理优化**:
  - 添加 `copiedKeyIndex` 响应式状态追踪复制状态
  - 添加 `isAutoAuthenticating` 和 `isInitialized` 标志管理认证流程

- **剪贴板API降级方案**:
  - 优先使用 `navigator.clipboard.writeText()`
  - 自动降级到 `document.execCommand('copy')`
  - 确保所有浏览器环境都能正常工作

---

## [v2.0.1-go] - 2025-10-12

### 🐛 重要修复

#### 前端资源加载问题修复

- **修复 Vite base 路径配置** (`vite.config.ts`)
  - 添加 `base: '/'` 配置，使用绝对路径适配 Go 嵌入式部署
  - 修复前端资源加载失败问题（"Expected a JavaScript module but got text/html"）
  - 优化构建配置，添加代码分割（vue-vendor, mdi-icons）

- **修复 NoRoute 处理器逻辑** (`frontend.go`)
  - 智能文件服务：先尝试读取实际文件，不存在才返回 index.html
  - 添加 `getContentType()` 函数，正确设置各类资源的 MIME 类型
  - 支持 .html, .css, .js, .json, .svg, .ico, .woff, .woff2 等文件类型
  - 修复 `/favicon.ico` 等静态资源返回 HTML 的问题
  - **添加 API 路由优先处理**：新增 `isAPIPath()` 函数检测 `/v1/`, `/api/`, `/admin/` 前缀，对不存在的 API 端点返回 JSON 格式 404 错误而非 HTML

- **添加 favicon 支持**
  - 创建 `frontend/public/` 目录
  - 添加 SVG 格式的 favicon（轻量、矢量、支持主题）
  - 自动复制到构建产物中

#### API 路由兼容性修复

- **统一前后端 API 路由** (`main.go`)
  - 修改 `/api/upstreams` → `/api/channels`（与前端保持一致）
  - 添加缺失的 handler 函数：
    - `UpdateLoadBalance` - 更新负载均衡策略
    - `PingChannel` - 单个渠道延迟测试
    - `PingAllChannels` - 批量延迟测试
  - 修复 `DeleteApiKey` 支持 URL 路径参数
  - 优化 `GetUpstreams` 返回格式（包含 channels, current, loadBalance）

#### 环境变量优化

- **ENV 变量标准化** (`env.go`, `.env.example`)
  - `NODE_ENV` → `ENV`（更通用的命名）
  - 保持向后兼容（优先读取 `ENV`，回退到 `NODE_ENV`）
  - 添加详细的配置影响说明文档

#### 版本注入修复

- **Makefile 版本信息注入** (`Makefile`)
  - 修复 `make run`、`make dev`、`make dev-backend` 缺少 `-ldflags` 参数
  - 确保运行时显示正确的版本号、构建时间和 Git commit

### ⚡ 性能优化

#### 前端构建缓存机制

- **智能缓存系统** (`Makefile`)
  - 添加 `.build-marker` 标记文件追踪构建状态
  - 自动检测 `frontend/src` 目录文件变更
  - 未变更时跳过编译，**启动速度提升 142 倍**（10秒 → 0.07秒）
  - 新增 `ensure-frontend-built` 目标实现智能构建逻辑

- **缓存性能对比**:
  | 场景 | 之前 | 现在 | 提升 |
  |------|------|------|------|
  | 首次构建 | ~10秒 | ~10秒 | 无变化 |
  | **无变更重启** | ~10秒 | **0.07秒** | **142倍** 🚀 |
  | 有变更重新构建 | ~10秒 | ~8.5秒 | 15%提升 |

### 📝 文档更新

- **README.md 更新**
  - 添加智能缓存机制说明
  - 添加 ENV 环境变量影响详解
  - 更新开发流程最佳实践
  - 添加缓存命令使用说明

- **前端构建优化文档**
  - 说明 Makefile 缓存原理
  - 提供典型开发场景示例
  - Bun vs npm 对比说明

### 🔧 技术改进

- **代码分割优化**
  - 分离 vue-vendor (137KB) 和 mdi-icons 模块
  - 移除无法分割的 @mdi/font 依赖
  - 优化首屏加载性能

- **Content-Type 准确性**
  - 所有静态资源返回正确的 MIME 类型
  - 支持字体文件正确加载
  - 修复浏览器控制台 MIME 类型警告

### 📦 构建系统

- **Makefile 增强**
  - 添加 `build-frontend-internal` 内部目标
  - 优化 `clean` 命令清除缓存标记
  - 改进 `dev-backend` 前端构建检查逻辑

---

## [v2.0.0-go] - 2025-01-15

### 🎉 Go 语言重写版本首次发布

这是 Claude Proxy 的完整 Go 语言重写版本，保留所有 TypeScript 版本功能的同时，带来显著的性能提升和部署便利性。

#### ✨ 新特性

- **🚀 高性能重写**
  - 使用 Go 语言完整重写所有后端代码
  - 原生并发支持（Goroutine）
  - 启动速度提升 20 倍（< 100ms vs 2-3s）
  - 内存占用降低 70%（~20MB vs 50-100MB）

- **📦 单文件部署**
  - 前端资源通过 `embed.FS` 嵌入二进制文件
  - 无需 Node.js 运行时
  - 单个可执行文件包含所有功能
  - 跨平台编译支持（Linux/macOS/Windows，amd64/arm64）

- **🎯 完整功能移植**
  - ✅ 所有 4 种上游服务适配器（OpenAI、Gemini、Claude、OpenAI Old）
  - ✅ 完整的协议转换逻辑
  - ✅ 流式响应和工具调用支持
  - ✅ 配置管理和热重载
  - ✅ API 密钥管理和负载均衡
  - ✅ Web 管理界面（完整嵌入）
  - ✅ Failover 故障转移机制

- **⚙️ 改进的版本管理**
  - 集中式版本控制（`VERSION` 文件）
  - 构建时自动注入版本信息
  - Git commit hash 追踪
  - 健康检查 API 包含版本信息

- **🛠️ 增强的构建系统**
  - 统一的 Makefile 构建系统
  - 支持多平台交叉编译
  - 自动化构建脚本
  - 发布包自动打包

#### 📊 性能对比

| 指标 | TypeScript 版本 | Go 版本 | 提升 |
|------|----------------|---------|------|
| 启动时间 | 2-3s | < 100ms | **20x** |
| 内存占用 | 50-100MB | ~20MB | **70%↓** |
| 部署包大小 | 200MB+ | ~15MB | **90%↓** |
| 并发处理 | 事件循环 | 原生 Goroutine | ⭐⭐⭐ |

#### 🎨 技术栈

- **后端**: Go 1.22+, Gin Framework
- **配置**: fsnotify (热重载), godotenv
- **嵌入**: Go embed.FS
- **构建**: Makefile, Shell Scripts

#### 📝 版本管理优化

现在升级版本只需修改一个文件：

```bash
# 只需编辑根目录的 VERSION 文件
echo "v2.1.0" > VERSION

# 重新构建即可
make build
```

所有构建产物（二进制文件、健康检查 API、启动信息）会自动包含新版本！

#### 🔄 迁移指南

从 TypeScript 版本迁移到 Go 版本：

1. 配置文件完全兼容（`.config/config.json`）
2. 环境变量完全兼容（`.env`）
3. API 端点完全兼容（`/v1/messages`、`/health` 等）
4. Web 管理界面功能一致

只需：
```bash
# 1. 构建 Go 版本
make build

# 2. 使用相同的配置文件
cp -r backend/.config backend-go/.config
cp backend/.env backend-go/.env

# 3. 运行
./backend-go/dist/claude-proxy-linux-amd64
```

#### ⚠️ 已知限制

- 暂无 Docker 镜像（计划在 v2.1.0 提供）
- 配置文件加密功能待实现

---

## v1.2.0 - 2025-09-19

### ✨ 新功能

- **Web管理界面全面升级**: 添加了完整的Web管理面板，支持可视化管理API渠道
- **模型映射功能**: 支持将请求中的模型名重定向到目标模型（如 "opus" → "claude-3-5-sonnet"）
- **渠道置顶功能**: 支持将常用渠道置顶显示，提升管理效率
- **API密钥故障转移**: 实现多密钥负载均衡和自动故障转移机制
- **ESC键快捷操作**: 编辑渠道modal支持ESC键快速关闭

### 🎨 UI/UX 优化

- **暗色模式支持**: 全面支持暗色模式，自动适配系统主题设置
- **渠道卡片重设计**: 采用现代化设计语言，提升视觉体验
- **绿色主题边框**: 统一使用绿色主题色，提升界面一致性
- **密钥数量优化**: 将密钥数量显示移至管理标题栏，界面更紧凑
- **模型选择优化**: 源模型名改为下拉选择（opus/sonnet/haiku），避免输入错误

### 🐛 Bug 修复

- **TypeScript类型错误**: 修复变量作用域相关的类型检查错误
- **CSS变量规范**: 根据Vuetify官方文档修复CSS变量使用方式
- **Header配色问题**: 修复编辑渠道modal在暗色模式下的配色问题
- **图标颜色统一**: 统一modal内图标颜色，保持视觉一致性
- **负载均衡策略**: 修复上游负载均衡策略不生效的问题

### ♻️ 重构

- **项目结构**: 重构为monorepo架构，分离前后端代码
- **渠道卡片样式**: 全面重构渠道卡片组件，优化代码结构
- **主题系统**: 基于Vuetify最佳实践重构主题系统

### ⚙️ 其他

- **构建系统**: 添加TypeScript类型检查和构建验证
- **发布流程**: 完善版本发布指南和自动化流程

## v1.1.0 - 2025-09-17

### 🚀 重大优化更新

这个版本专注于代码质量提升，大幅优化了字符串处理、正则表达式使用和代码结构。

#### ✨ 代码优化

- **SSE 数据解析优化**: 
  - 统一使用正则表达式 `/^data:\s*(.*)$/` 处理 Server-Sent Events 数据
  - 支持多种 SSE 格式（`data:`、`data: `、`data:  ` 等）
  - 提升解析健壮性，减少代码复杂度

- **Bearer Token 处理简化**:
  - 使用正则表达式 `/^bearer\s+/i` 替代复杂的字符串判断
  - 代码行数减少 60%，性能提升明显

- **敏感头部处理重构**:
  - 使用函数式的 `replace()` 回调处理 Authorization 头
  - 统一 API Key 掩码逻辑，提升安全性

- **请求头过滤优化**:
  - 缓存 `toLowerCase()` 转换结果，避免重复计算
  - 提升请求处理性能

- **API Key 掩码函数简化**:
  - 使用 `slice()` 替代 `substring()`
  - 条件逻辑简化，代码更清晰

- **参数解析现代化**:
  - 传统 `for` 循环重构为函数式 `reduce()`
  - 使用正则表达式简化命令行参数解析

#### 🧹 代码重构

- **重复代码消除**:
  - 提取 `normalizeClaudeRole` 函数到 `utils.ts` 共享模块
  - 遵循 DRY 原则，便于维护

- **User-Agent 检查优化**:
  - 使用正则表达式 `/^claude-cli/i` 进行大小写不敏感匹配
  - 提升代码可读性

#### 🔧 构建系统改进

- **新增构建脚本**:
  - 添加 `bun run build` 命令用于项目构建验证
  - 添加 `bun run type-check` 命令用于 TypeScript 类型检查

#### 📈 性能提升

- **代码行数减少**: 总计减少约 30% 的代码行数
- **性能改进**: 减少重复的字符串操作和条件判断
- **内存优化**: 更高效的字符串处理逻辑

#### 🛠️ Claude API 流式响应修复

- **修复 Claude API 流式响应解析**:
  - 正确处理 `content_block_delta` 事件中的 `text_delta` 内容
  - 支持 `input_json_delta` 类型的工具调用内容解析
  - 改进工具调用内容的合成显示格式

- **SSE 格式兼容性增强**:
  - 支持标准 `data: ` 格式和紧凑 `data:` 格式
  - 提升与不同上游服务的兼容性

#### 🧪 质量保证

- **类型安全**: 所有修改通过 TypeScript 类型检查
- **构建验证**: 确保所有优化不影响功能完整性
- **向后兼容**: 保持所有现有 API 接口不变

### 🔄 技术债务清理

这次更新严格遵循了软件工程最佳实践：

- **KISS 原则**: 追求代码和设计的极致简洁
- **DRY 原则**: 消除重复代码，统一处理逻辑  
- **YAGNI 原则**: 删除未使用的代码分支
- **函数式编程**: 优先使用函数式方法处理数据转换

---

## v1.0.0 - 2025-09-13

### 🎉 初始版本发布

这是 Claude API 代理服务器的第一个稳定版本。

#### ✨ 主要功能

- **多上游支持**: 内置 `openai`, `openaiold`, `gemini`, 和 `claude` 提供商，实现协议转换。
- **配置管理**:
  - 通过 `config.json` 文件管理上游服务。
  - 提供 `bun run config` 命令行工具，用于动态增、删、改、查上游配置。
  - 支持配置热重载，修改配置无需重启服务。
- **负载均衡**:
  - 支持对单个上游内的多个 API 密钥进行负载均衡。
  - 提供 `round-robin`（轮询）、`random`（随机）和 `failover`（故障转移）三种策略。
- **统一访问入口**: 所有请求通过 `/v1/messages` 代理，简化客户端配置。
- **全面的 API 兼容性**:
  - 支持流式（stream）和非流式响应。
  - 支持工具调用（Tool Use）。
- **环境配置**: 通过 `.env` 文件管理服务器端口、日志级别、访问密钥等。
- **部署与开发**:
  - 提供 `bun run dev` 开发模式，支持源码修改后自动重启。
  - 提供详细的 `README.md` 和 `DEVELOPMENT.md` 文档，包含 PM2 和 Docker 的部署指南。
- **健壮性与监控**:
  - 内置 `/health` 健康检查端点。
  - 详细的请求与响应日志系统。
  - 对上游流式响应中的错误进行捕获和处理。

---

## v2.0.1 升级指南

> 从 v2.0.0-go 升级到 v2.0.1 的完整指南

### 🎯 升级概述

v2.0.1 主要修复了前端资源加载问题和性能优化，强烈建议所有 v2.0.0 用户升级。

#### 主要改进

- ✅ **修复前端无法加载** - 解决 Vite base 路径配置问题
- ✅ **性能提升 142 倍** - 智能缓存机制，开发时启动仅需 0.07 秒
- ✅ **API 路由修复** - 前后端路由完全匹配
- ✅ **ENV 标准化** - 更通用的环境变量命名

#### 升级步骤

1. **备份配置（可选但推荐）**
   ```bash
   cp backend-go/.config/config.json backend-go/.config/config.json.backup
   cp backend-go/.env backend-go/.env.backup
   ```

2. **更新代码**
   ```bash
   git pull origin main
   ```

3. **更新环境变量（推荐）**
   编辑 `backend-go/.env`：
   ```diff
   - NODE_ENV=development
   + # 运行环境: development | production
   + ENV=development
   ```
   **注意**：旧的 `NODE_ENV` 仍然有效（向后兼容），但建议迁移到 `ENV`。

4. **重新构建**
   ```bash
   make clean
   make build-frontend-internal
   make run
   ```

5. **验证升级**
   ```bash
   make info  # 应该显示 Version: v2.0.1
   curl http://localhost:3001/health | jq '.version'
   ```

#### 新功能使用

**智能缓存**：
- 首次构建：~10 秒
- 无变更重启：**0.07 秒**（提升 142 倍）
- 有变更重新构建：~8.5 秒

**ENV 变量详细配置**：
- `ENV=development`：开发模式（详细日志、开发端点、宽松 CORS）
- `ENV=production`：生产模式（高性能、严格安全）

#### 破坏性变更

**无破坏性变更**。所有 v2.0.0 配置和 API 完全兼容。

#### 回滚到 v2.0.0

如果升级遇到问题，可以回滚：
```bash
git checkout v2.0.0-go
cp backend-go/.config/config.json.backup backend-go/.config/config.json
cp backend-go/.env.backup backend-go/.env
make clean && make build-frontend-internal
```

**升级成功！** 🎉

---
