# 版本历史

> **注意**: v2.0.0 开始为 Go 语言重写版本，v1.x 为 TypeScript 版本

---

## [v2.4.17] - 2025-12-30

### 🐛 修复

- **修复 ModelMapping 导致请求字段丢失** - 解决使用模型重定向时 Claude API 返回 403 的问题：
  - 原因：`ClaudeRequest` 结构体缺少 `metadata` 字段，JSON 反序列化时该字段被丢弃
  - 表现：配置 `modelMapping` 后请求被上游拒绝（如 `opus` → `claude-opus-4-5-20251101`）
  - 修复：在 `ClaudeRequest` 中添加 `Metadata map[string]interface{}` 字段
  - 涉及文件：`backend-go/internal/types/types.go`

---

## [v2.4.16] - 2025-12-30

### 🐛 修复

- **修复 Gemini 渠道预期请求 URL 预览** - 创建渠道时预览显示正确的 `/v1beta` 路径：
  - 原问题：Gemini 渠道预览错误显示 `/v1` 而后端实际使用 `/v1beta`
  - 修复：当 serviceType 为 gemini 时使用 `/v1beta` 作为版本前缀
  - 涉及文件：`frontend/src/components/AddChannelModal.vue`

---

## [v2.4.15] - 2025-12-30

### 🐛 修复

- **修复 Gemini API 路由注册失败** - 解决 Gin 框架路由 panic 问题：
  - 原因：Gin 不支持 `:param\:literal` 格式，即使转义冒号也会被解析为两个通配符
  - 方案：使用 `*modelAction` 通配符捕获 `model:action` 整体，在 handler 内解析
  - 涉及文件：`main.go`、`internal/handlers/gemini/handler.go`

### ✨ 新功能

- **Gemini 历史指标 API 完整实现** - 补全 Gemini 模块的历史数据端点：
  - `GET /api/gemini/channels/metrics/history` - 渠道级别指标历史
  - `GET /api/gemini/channels/:id/keys/metrics/history` - Key 级别指标历史
  - `GET /api/gemini/global/stats/history` - 全局统计历史
  - 涉及文件：`internal/handlers/channel_metrics_handler.go`、`main.go`

- **Gemini 前端管理界面完整实现** - 与 Messages/Responses 功能完全对齐：
  - 新增 Gemini Tab 切换，支持完整渠道 CRUD、Key 管理、状态/促销设置
  - KeyTrendChart 和 GlobalStatsChart 组件支持 Gemini 数据展示（移除降级显示）
  - 涉及文件：`frontend/src/App.vue`、`frontend/src/components/`、`frontend/src/services/api.ts`

---

## [v2.4.14] - 2025-12-29

### ✨ 新功能

- **新增 Gemini API 模块** - 与 `/v1/messages`、`/v1/responses` 同级的完整 Gemini 代理支持：
  - **代理端点**：`POST /v1/models/{model}:generateContent`（非流式）、`:streamGenerateContent`（流式）
  - **协议转换**：支持 Gemini 请求转发到 Claude/OpenAI/Gemini 上游，双向转换器自动处理格式差异
  - **渠道管理 API**：完整 CRUD、API Key 管理、状态/促销设置、指标监控（`/api/gemini/channels/*`）
  - **多渠道调度**：集成 ChannelScheduler，支持优先级、熔断、Trace 亲和性
  - **认证方式**：兼容 Gemini 原生格式（`x-goog-api-key` 头、`?key=` 参数）
  - 涉及文件：`internal/handlers/gemini/`、`internal/converters/gemini_converter.go`、`internal/types/gemini.go`

### 🔧 重构

- **config 包模块化拆分** - 将 1973 行的单文件拆分为 6 个职责清晰的模块：
  - `config.go`（297 行）：核心类型定义 + 共享方法
  - `config_loader.go`（384 行）：配置加载、迁移、验证、文件监听
  - `config_messages.go`（429 行）：Messages 渠道 CRUD
  - `config_responses.go`（380 行）：Responses 渠道 CRUD
  - `config_gemini.go`（361 行）：Gemini 渠道 CRUD
  - `config_utils.go`（183 行）：工具函数（去重、模型重定向、状态辅助）
  - 遵循单一职责原则，提升代码可维护性

---

## [v2.4.12] - 2025-12-29

### 🐛 修复

- **修复 Responses API 错误消息提取失败的问题** - 解决 upstream_error 字段无法被正确解析：
  - 扩展 `classifyByErrorMessage` 函数：支持多个消息字段（`message`, `upstream_error`, `detail`）
  - 支持嵌套对象格式：当 `upstream_error` 为对象时，提取其中的 `message` 字段
  - 之前仅检查 `error.message` 字段，导致 `{type, upstream_error}` 格式的错误无法被识别
  - 新增 4 个测试用例覆盖 upstream_error 字符串、嵌套对象、detail 字段等场景
  - 涉及文件：`internal/handlers/common/failover.go`, `internal/handlers/common/failover_test.go`

---

## [v2.4.11] - 2025-12-29

### 🐛 修复

- **修复 Fuzzy 模式下 403 + 预扣费消息未触发 Key 降级的问题** - 补充 v2.4.10 修复的遗漏场景：
  - 修改 `shouldRetryWithNextKeyFuzzy` 函数：新增 `bodyBytes` 参数，对非 402/429 状态码检查消息体中的配额关键词
  - 之前 Fuzzy 模式仅检查状态码（402/429 = quota），不解析消息体，导致 403 + "预扣费额度失败" 返回 `isQuotaRelated=false`
  - 新增 `TestShouldRetryWithNextKey_FuzzyMode_403WithQuotaMessage` 测试用例
  - 涉及文件：`internal/handlers/common/failover.go`, `internal/handlers/common/failover_test.go`

### 🔧 调试

- **添加 Key 降级调试日志** - 用于追踪 `isQuotaRelated` 值和密钥降级流程：
  - 在 `ShouldRetryWithNextKey` 调用后记录返回值（statusCode, shouldFailover, isQuotaRelated）
  - 在密钥标记为配额相关失败时记录日志
  - 涉及文件：`internal/handlers/messages/handler.go`
- **改进 .env.example 文档** - 添加日志配置默认值说明（默认启用，需显式设置 false 禁用）

---

## [v2.4.10] - 2025-12-29

### 🐛 修复

- **修复 403 预扣费额度不足的 Key 未被自动降级的问题** - 解决配额不足的密钥始终被优先尝试：
  - 修改 `shouldRetryWithNextKeyNormal` 逻辑：即使 HTTP 状态码已触发 failover，仍检查消息体确定是否为配额相关错误
  - 之前 403 状态码直接返回 `isQuotaRelated=false`，跳过消息体解析，导致 `DeprioritizeAPIKey` 未被调用
  - 新增 "预扣费" 关键词到 `quotaKeywords` 列表，确保匹配中文预扣费错误消息
  - 涉及文件：`internal/handlers/common/failover.go`

---

## [v2.4.9] - 2025-12-27

### 🔧 改进

- **重构 URL 预热机制为非阻塞动态排序** - 解决首次请求延迟 500ms+ 的问题：
  - 移除阻塞式 ping 预热（`URLWarmupManager`），改用非阻塞的 `URLManager`
  - 新排序策略：基于实际请求结果动态调整 URL 顺序
    - 请求成功：重置失败计数，URL 保持/提升位置
    - 请求失败：增加失败计数，URL 移到末尾
    - 冷却期机制：失败的 URL 在 30 秒后自动恢复可用
  - 排序规则：无失败记录优先 > 冷却期已过 > 仍在冷却期
  - 涉及文件：`warmup/url_manager.go`（新建）、`warmup/url_warmup.go`（删除）、`scheduler/channel_scheduler.go`、`messages/handler.go`、`responses/handler.go`、`main.go`

---

## [v2.4.8] - 2025-12-27

### 🐛 修复

- **修复多端点渠道密钥轮换时的并发竞争问题** - 解决高并发下 BaseURL 被错误修改导致密钥跨渠道混用：
  - 新增 `UpstreamConfig.Clone()` 深拷贝方法，避免并发修改共享对象
  - Messages/Responses Handler 改用深拷贝替代临时修改模式
  - 新增 `MarkWarmupURLFailed()` 方法，请求失败时触发预热缓存失效
  - HTTP 5xx 和网络超时均会触发预热缓存失效，确保失败端点被重新排序
  - 涉及文件：`config/config.go`、`messages/handler.go`、`responses/handler.go`、`scheduler/channel_scheduler.go`、`warmup/url_warmup.go`

---

## [v2.4.6] - 2025-12-27

### ✨ 新功能

- **多端点预热排序** - 渠道首次访问前自动 ping 所有端点，按延迟排序：
  - 新增 `internal/warmup/url_warmup.go` 预热管理器模块
  - 渠道首次访问时自动并发 ping 所有 BaseURL
  - 排序策略：成功的端点优先，同类型按延迟从低到高排序
  - ping 结果缓存 5 分钟，避免频繁测试
  - 支持并发安全的预热请求去重（多个请求同时触发时只执行一次预热）
  - Messages 和 Responses API 均支持预热排序

---

## [v2.4.5] - 2025-12-27

### 🔧 改进

- **统一日志前缀规范** - Messages 和 Responses 接口日志标签标准化：
  - Messages 流式处理日志统一使用 `[Messages-Stream]`、`[Messages-Stream-Token]` 前缀
  - Responses 流式处理日志保持 `[Responses-Stream]`、`[Responses-Stream-Token]` 前缀
  - 修复 3 处遗漏前缀的错误日志（`messages/handler.go`、`responses/handler.go`）
  - 更新 `backend-go/CLAUDE.md` 日志规范文档

---

## [v2.4.4] - 2025-12-27

### ✨ 新功能

- **全局流量和 Token 统计图表** - 新增全局统计可视化功能：
  - 后端新增 `/api/messages/global/stats/history` 和 `/api/responses/global/stats/history` API
  - 支持请求数量（成功/失败/总量）和 Token 总量（输入/输出）统计
  - 前端新增 `GlobalStatsChart.vue` 组件，支持流量/Token 双视图切换
  - 时间范围支持 1h / 6h / 24h / 今日 多档位切换
  - 用户偏好（时间范围、视图模式）按 Messages/Responses 分别保存到 localStorage
  - 以顶部可折叠卡片形式展示，随当前 Tab 自动切换对应 API 类型的统计

- **渠道 Key 趋势图表支持"今日"** - KeyTrendChart 新增今日时间范围选项：
  - 后端 `GetChannelKeyMetricsHistory` 支持 `duration=today` 参数
  - 前端添加"今日"按钮，动态计算从今日 0 点到当前的时长

---

## [v2.4.3] - 2025-12-27

### 🐛 修复

- **Responses API Token 统计修复** - 解决上游无 usage 时本地统计无数据的问题：
  - 修复 SSE 事件解析格式兼容性：支持 `data:` 和 `data: ` 两种格式（某些上游不带空格）
  - 修复 `handleSuccess` / `handleStreamSuccess` 不返回 usage 数据的问题
  - 修复调用点使用 `RecordSuccess` 而非 `RecordSuccessWithUsage` 导致 token 统计未入库
  - 涉及函数：`checkResponsesEventUsage`、`injectResponsesUsageToCompletedEvent`、`patchResponsesCompletedEventUsage`、`tryChannelWithAllKeys`

---

## [v2.4.2] - 2025-12-26

### 🐛 修复

- **原始请求日志修复** - 修复多渠道模式下原始请求头/请求体日志不显示的问题：
  - 将 `LogOriginalRequest` 调用移至 Handler 入口处，确保无论单/多渠道模式都只记录一次
  - 移除单渠道处理函数中重复的日志调用和未使用变量
  - 同时修复 Messages 和 Responses 两个处理器

### 🧹 清理

- **移除废弃环境变量 `LOAD_BALANCE_STRATEGY`** - 负载均衡策略已迁移至 config.json 热重载配置：
  - 删除 `env.go` 中 `LoadBalanceStrategy` 字段
  - 更新 `.env.example`、`docker-compose.yml`、`README.md` 移除相关配置
  - 更新 `CLAUDE.md` 添加配置方式说明

---

## [v2.4.0] - 2025-12-26

### ✨ 改进

- **渠道编辑表单优化** - 改进 AddChannelModal 用户体验：
  - 预期请求支持显示所有 BaseURL 端点，而非仅显示首个
  - 修复 Gemini 类型渠道预期请求显示错误端点的问题（应为 `/generateContent`）
  - 修复从快速模式切换到详细模式时 BaseURL 输入框为空的问题
  - 表单字段重排：TLS 验证开关和描述字段移至表单末尾
  - BaseURL 输入框不再自动修改用户输入，仅在提交时进行去重处理
  - 调整预期请求区域下方间距，改善视觉效果

- **API Key/BaseURL 策略简化** - 移除过度设计，采用纯 failover 模式：
  - 删除 `ResourceAffinityManager` 及相关代码（资源亲和性）
  - 移除 API Key 策略选择（round-robin/random/failover），始终使用优先级顺序
  - 移除 BaseURL 策略选择，始终使用优先级顺序并在失败时切换
  - 前端删除策略选择器，简化渠道配置界面
  - 保留渠道级 Trace 亲和性（TraceAffinityManager）用于会话一致性
  - 清理遗留无用代码：`requestCount`/`responsesRequestCount` 字段、`EnableStreamEventDedup` 环境变量

### 🐛 修复

- **多 BaseURL failover 失效** - 修复当所有 API Key 在首个 BaseURL 失败后不会切换到下一个 BaseURL 的问题：
  - 重构 `tryChannelWithAllKeys` 函数，采用嵌套循环遍历所有 BaseURL
  - 重构 `handleSingleChannel` 函数，单渠道模式也支持多 BaseURL failover
  - 每个 BaseURL 尝试所有 Key 后，若全部失败则自动切换下一个
  - 每次切换 BaseURL 时重置失败 Key 列表
  - 同时修复 Messages 和 Responses 两个处理器
  - 修复 `GetEffectiveBaseURL()` 优先级：临时设置的 `BaseURL` 字段优先于 `BaseURLs` 数组
  - 移除废弃代码：`MarkBaseURLFailed()`、`baseURLIndex` 字段

- **SSE 流式事件完整性** - 修复 Claude Provider 流式响应可能在事件边界处截断的问题：
  - 改用事件缓冲机制，按空行分隔完整 SSE 事件后再转发
  - 确保 `event:`/`data:`/`id:`/`retry:` 等字段作为整体发送
  - 处理上游未以空行结尾的边界情况

- **前端延迟测试结果被覆盖** - 修复 ping 延迟值显示几秒后消失的问题：
  - 新增 `mergeChannelsWithLocalData()` 函数保留本地延迟测试结果
  - 应用于自动刷新、Tab 切换、手动刷新三处数据更新点
  - 添加 5 分钟有效期检查，确保过期数据自动清除

---

## [v2.3.11] - 2025-12-26

### 🐛 修复

- **Responses API usage 字段缺失** - 修复当上游服务（OpenAI/Gemini）不返回 usage 信息时，`response.completed` 事件完全不包含 `usage` 字段的问题：
  - 转换器现在始终生成基础 `usage` 字段（`input_tokens`、`output_tokens`、`total_tokens`），即使值为 0
  - Handler 检测到 usage 存在后，会用本地 token 估算值替换 0 值
  - 确保下游客户端始终能获得合理的 token 使用估算

### ✨ 新功能

- **API Key/Base URL 去重** - 前后端全链路自动去重：
  - 前端详细表单模式输入时自动过滤重复 URL（忽略末尾 `/` 和 `#` 差异）
  - 后端 AddUpstream/UpdateUpstream 接口添加去重逻辑
  - 同时覆盖 Messages 和 Responses 渠道

### 🔧 改进

- **API Key 策略推荐调整** - 将默认推荐策略从"轮询"改为"故障转移"，更符合实际使用场景
- **延迟测试结果持久显示** - 优化渠道延迟测试体验：
  - 测试结果直接显示在故障转移序列列表中，不再使用短暂 Toast 通知
  - 延迟结果保持显示 5 分钟后自动清除
  - 支持单个渠道测试和批量测试统一行为

---

## [v2.3.10] - 2025-12-25

### ✨ 新功能

- **快速添加支持等号分割** - 输入 `KEY=value` 格式时自动按等号分割，识别 `value` 为 API Key
- **快速添加支持多 Base URL** - 自动识别输入中所有 HTTP 链接作为 Base URL（最多 10 个）
- **多 URL 预期请求展示** - 快速添加模式下逐一展示每个 URL 的预期请求地址

---

## [v2.3.9] - 2025-12-25

### ✨ 新功能

- **渠道级 API Key 策略** - 每个渠道可独立配置 API Key 分配策略：
  - `round-robin`（默认）：轮询分发请求到不同 Key
  - `random`：随机选择 Key
  - `failover`：故障转移，优先使用第一个 Key
  - 单 Key 时自动强制使用 `failover`，UI 显示禁用状态
- **多 BaseURL 支持** - 单个渠道可配置多个 BaseURL，支持三种策略：
  - `round-robin`（默认）：轮询分发请求，自动分散负载
  - `random`：随机选择 URL
  - `failover`：手动故障转移（需配合外部监控切换）
- **促销期状态展示** - 渠道列表显示正在"抢优先级"的渠道，带火箭图标和剩余时间
- **延迟测试优化** - 批量测试时直接在列表显示每个渠道的延迟值，颜色根据延迟等级变化（绿/黄/红）
- **多 URL 延迟测试** - 当渠道配置多个 BaseURL 时，并发测试所有 URL 并显示最快的延迟
- **资源亲和性** - 记录用户成功使用的 BaseURL 和 API Key 索引，后续请求优先使用相同资源组合，减少不必要的资源切换

---

## [v2.3.8] - 2025-12-24

### 🔨 重构

- **日志输出规范化** - 移除所有 emoji 符号，统一使用 `[Component-Action]` 标签格式，确保跨平台兼容性

---

## [v2.3.7] - 2025-12-24

### 🐛 修复

- **滑动窗口重建逻辑优化** - 服务重启时只从最近 15 分钟的历史记录重建滑动窗口，避免历史失败记录导致渠道长期处于不健康状态

---

## [v2.3.6] - 2025-12-24

### ✨ 新功能

- **快速添加渠道 - API Key 识别增强** - 大幅改进 `quickInputParser` 的密钥识别能力
  - 新增各平台特定格式支持：OpenAI (sk-/sk-proj-)、Anthropic (sk-ant-api03-)、Google Gemini (AIza)、OpenRouter (sk-or-v1-)、Hugging Face (hf_)、Groq (gsk_)、Perplexity (pplx-)、Replicate (r8_)、智谱 AI (id.secret)、火山引擎 (UUID/AK)
  - 新增宽松兜底规则：常见前缀 (sk/api/key/ut/hf/gsk/cr/ms/r8/pplx) + 任意后缀，支持识别短密钥如 `sk-111`
  - 新增配置键名排除：全大写下划线分隔格式 (如 `API_TIMEOUT_MS`) 不再被误识别为密钥

### 🐛 修复

- **Claude Code settings.json 解析修复** - 粘贴 Claude Code 配置时，不再将键名 (`ANTHROPIC_AUTH_TOKEN` 等) 误识别为 API 密钥

---

## [v2.3.5] - 2025-12-24

### ✨ 新功能

- **Responses API Token 统计补全** - 为 Responses 接口添加完整的输入输出 Token 统计功能
  - 非流式响应：自动检测上游是否返回 usage，无 usage 时本地估算，修补虚假值（`input_tokens/output_tokens <= 1`）
  - 流式响应：累积收集流事件中的文本内容，在 `response.completed` 事件中检测并修补 Token 统计
  - 新增 `EstimateResponsesRequestTokens`、`EstimateResponsesOutputTokens` 专用估算函数
  - 支持缓存 Token 细分统计（5m/1h TTL）
  - 与 Messages API 保持一致的处理逻辑

### 🐛 修复

- **缓存 Token 5m/1h 字段检测完善** - 修复缓存 Token 检测逻辑，同时检测 `cache_creation_5m_input_tokens` 和 `cache_creation_1h_input_tokens` 字段
- **类型化 ResponsesItem 处理** - `EstimateResponsesOutputTokens` 现支持直接处理 `[]types.ResponsesItem` 类型
- **total_tokens 零值补全** - 修复当上游返回有效 `input_tokens/output_tokens` 但 `total_tokens` 为 0 时未自动补全的问题（非流式和流式均已修复）
- **特殊类型 Token 估算回退** - 当 `ResponsesItem` 的 `Type` 为 `function_call`、`reasoning` 等特殊类型时，自动序列化整个结构进行估算
- **流式 delta 类型扩展** - `extractResponsesTextFromEvent` 现支持更多 delta 事件类型：`output_json.delta`、`content_part.delta`、`audio.delta`、`audio_transcript.delta`
- **流式缓冲区内存保护** - `outputTextBuffer` 添加 1MB 大小上限，防止长流式响应导致内存溢出
- **Claude/OpenAI 缓存格式区分** - 新增 `HasClaudeCache` 标志，正确区分 Claude 原生缓存字段（`cache_creation/read_input_tokens`）和 OpenAI 格式（`input_tokens_details.cached_tokens`），避免 OpenAI 格式错误阻止 `input_tokens` 补全
- **流式缓存标志传播** - 修复 `updateResponsesStreamUsage` 未传播 `HasClaudeCache` 标志的问题，确保流式响应正确识别 Claude 缓存

---

## [v2.3.4] - 2025-12-23

### ✨ 新功能

- **Models API 增强** - `/v1/models` 端点重大改进
  - 使用调度器按故障转移顺序选择渠道（与 Messages/Responses API 一致）
  - 同时从 Messages 和 Responses 两种渠道获取模型列表并合并去重
  - 添加详细日志：渠道名称、脱敏 Key、选择原因
  - 移除对 Claude 原生渠道的跳过限制（第三方 Claude 代理通常支持 /models）
  - 移除不常用的 `DELETE /v1/models/:model` 端点

---

## [v2.3.3] - 2025-12-23

### ✨ 新功能

- **Models API 端点支持** - 新增 `/v1/models` 系列端点，转发到上游 OpenAI 兼容服务
  - `GET /v1/models` - 获取模型列表
  - `GET /v1/models/:model` - 获取单个模型详情
  - `DELETE /v1/models/:model` - 删除微调模型
  - 自动跳过不支持的 Claude 原生渠道，遍历所有上游直到成功或返回 404

---

## [v2.3.2] - 2025-12-23

### ✨ 新功能

- **快速添加渠道自动检测协议类型** - 根据 URL 路径自动选择正确的服务类型
  - `/messages` → Claude 协议
  - `/chat/completions` → OpenAI 协议
  - `/responses` → Responses 协议
  - `/generateContent` → Gemini 协议
- **快速添加支持 `%20` 分隔符** - 解析输入时自动将 URL 编码的空格转换为实际空格

---

## [v2.3.1] - 2025-12-22

### ✨ 新功能

- **HTTP 响应头超时可配置** - 新增 `RESPONSE_HEADER_TIMEOUT` 环境变量（默认 60 秒，范围 30-120 秒），解决上游响应慢导致的 `http2: timeout awaiting response headers` 错误

---

## [v2.3.0] - 2025-12-22

### ✨ 新功能

- **快速添加渠道支持引号内容提取** - 支持从双引号/单引号中提取 URL 和 API Key，可直接粘贴 Claude Code 环境变量 JSON 配置格式
- **SQLite 指标持久化存储** - 服务重启后不再丢失历史指标数据，启动时自动加载最近 24 小时数据
  - 新增 `METRICS_PERSISTENCE_ENABLED`（默认 true）和 `METRICS_RETENTION_DAYS`（默认 7）配置
  - 异步批量写入（100 条/批或每 30 秒），WAL 模式高并发，自动清理过期数据
- **完整的 Responses API Token Usage 统计** - 支持多格式自动检测（Claude/Gemini/OpenAI）、缓存 TTL 细分统计（5m/1h）
- **Messages API 缓存 TTL 细分统计** - 区分 5 分钟和 1 小时 TTL 的缓存创建统计

### 🔨 重构

- **SQLite 驱动切换为纯 Go 实现** - 从 `go-sqlite3`（CGO）切换为 `modernc.org/sqlite`，简化交叉编译

### 🐛 修复

- **Usage 解析数值类型健壮性** - 支持 `float64`/`int`/`int64`/`int32` 四种数值类型
- **CachedTokens 重复计算** - `CachedTokens` 仅包含 `cache_read`，不再包含 `cache_creation`
- **流式响应纯缓存场景 Usage 丢失** - 有任何 usage 字段时都记录

---

## [v2.2.0] - 2025-12-21

### 🔨 重构

- **Handlers 模块重构为同级子包结构** - 将 Messages/Responses API 处理器重构为同级模块，新增 `handlers/common/` 公共包，代码量减少约 180 行

### 🐛 修复

- **Stream 错误处理完善** - 流式传输错误时发送 SSE 错误事件并记录失败指标
- **CountTokens 端点安全加固** - 应用请求体大小限制
- **非 failover 错误指标记录** - 400/401/403 等错误正确记录失败指标

---

## [v2.1.35] - 2025-12-21

- **流量图表失败率可视化** - 失败率超过 10% 显示红色背景，Tooltip 显示详情

---

## [v2.1.34] - 2025-12-20

- **Key 级别使用趋势图表** - 支持流量/Token I/O/缓存三种视图，智能 Key 筛选
- **合并 Dashboard API** - 3 个并行请求优化为 1 个

---

## [v2.1.33] - 2025-12-20

- **Fuzzy Mode 错误处理开关** - 所有非 2xx 错误自动触发 failover
- **渠道指标历史数据 API** - 支持时间序列图表

---

## [v2.1.25] - 2025-12-18

### ✨ 新功能

- **TransformerMetadata 和 CacheControl 支持** - 转换器元数据保留原始格式信息，实现特性透传
- **FinishReason 统一映射函数** - OpenAI/Anthropic/Responses 三种协议间双向映射
- **原始日志输出开关** - `RAW_LOG_OUTPUT` 环境变量，开启后不进行格式化或截断

---

## [v2.1.23] - 2025-12-13

- 修复编辑渠道弹窗中基础 URL 布局和验证问题

---

## [v2.1.31] - 2025-12-19

- **前端显示版本号和更新检查** - 自动检查 GitHub 最新版本

---

## [v2.1.30] - 2025-12-19

- **强制探测模式** - 所有 Key 熔断时自动启用强制探测

---

## [v2.1.28] - 2025-12-19

- **BaseURL 支持 `#` 结尾跳过自动添加 `/v1`**

---

## [v2.1.27] - 2025-12-19

- 移除 Claude Provider 畸形 tool_call 修复逻辑

---

## [v2.1.26] - 2025-12-19

- Responses 渠道新增 `gpt-5.2-codex` 模型选项

---

## [v2.1.24] - 2025-12-17

- Responses 渠道新增 `gpt-5.2`、`gpt-5` 模型选项
- 移除 openaiold 服务类型支持

---

## [v2.1.23] - 2025-12-13

- 修复 402 状态码未触发 failover 的问题
- 重构 HTTP 状态码 failover 判断逻辑（两层分类策略）

---

## [v2.1.22] - 2025-12-13

### 🐛 修复

- **流式日志合成器类型修复** - 所有 Provider 的 HandleStreamResponse 都将响应转换为 Claude SSE 格式，日志合成器使用 "claude" 类型解析
- **insecureSkipVerify 字段提交修复** - 修复前端 insecureSkipVerify 为 false 时不提交的问题

---

## [v2.1.21] - 2025-12-13

### 🐛 修复

- **促销渠道绕过健康检查** - 促销渠道现在绕过健康检查直接尝试使用，只有本次请求实际失败后才跳过

---

## [v2.1.20] - 2025-12-12

- 渠道名称支持点击打开编辑弹窗

---

## [v2.1.19] - 2025-12-12

- 修复添加渠道弹窗密钥重复错误状态残留
- 新增 `/v1/responses/compact` 端点

---

## [v2.1.15] - 2025-12-12

### 🔒 安全加固

- **请求体大小限制** - 新增 `MAX_REQUEST_BODY_SIZE_MB` 环境变量（默认 50MB），超限返回 413
- **Goroutine 泄漏修复** - ConfigManager 添加 `stopChan` 和 `Close()` 方法释放资源
- **数据竞争修复** - 负载均衡计数器改用 `sync/atomic` 原子操作
- **优雅关闭** - 监听 SIGINT/SIGTERM，10 秒超时优雅关闭

---

## [v2.1.14] - 2025-12-12

- 修复流式响应 Token 计数中间更新被覆盖

---

## [v2.1.12] - 2025-12-11

- 支持 Claude 缓存 Token 计数

---

## [v2.1.10] - 2025-12-11

- 修复流式响应 Token 计数补全逻辑

---

## [v2.1.8] - 2025-12-11

- 重构过长方法，提升代码可读性

---

## [v2.1.7] - 2025-12-11

### 🐛 修复

- 修复前端 MDI 图标无法显示
- **Token 计数补全虚假值处理** - 当 `input_tokens <= 1` 或 `output_tokens == 0` 时用本地估算值覆盖

---

## [v2.1.6] - 2025-12-11

### ✨ 新功能

- **Messages API Token 计数补全** - 当上游不返回 usage 时，本地估算 token 数量并附加到响应中

---

## [v2.1.4] - 2025-12-11

- 修复前端渠道健康度统计不显示数据

---

## [v2.1.1] - 2025-12-11

- 新增 `QUIET_POLLING_LOGS` 环境变量（默认 true），过滤前端轮询日志噪音

---

## [v2.1.0] - 2025-12-11

### 🔨 重构

- **指标系统重构：Key 级别绑定** - 指标键改为 `hash(baseURL + apiKey)`，每个 Key 独立追踪
- **熔断器生效修复** - 在 `tryChannelWithAllKeys` 中调用 `ShouldSuspendKey()` 跳过熔断的 Key
- **单渠道路径指标记录** - 转换失败、发送失败、failover、成功时正确记录指标

---

## [v2.0.20-go] - 2025-12-08

- 修复单渠道模式渠道选择逻辑

---

## [v2.0.11-go] - 2025-12-06

### 🚀 多渠道智能调度器

- **ChannelScheduler** - 基于优先级的渠道选择、Trace 亲和性、失败率检测和自动熔断
- **MetricsManager** - 滑动窗口算法计算实时成功率
- **TraceAffinityManager** - 用户会话与渠道绑定

### 🎨 渠道编排面板

- 拖拽排序、实时指标、状态切换、备用池管理

---

## [v2.0.10-go] - 2025-12-06

### 🎨 复古像素主题

- Neo-Brutalism 设计语言：无圆角、等宽字体、粗实体边框、硬阴影

---

## [v2.0.5-go] - 2025-11-15

### 🚀 Responses API 转换器架构重构

- 策略模式 + 工厂模式实现多上游转换器
- 完整支持 Responses API 标准格式

---

## [v2.0.4-go] - 2025-11-14

### ✨ Responses API 透明转发

- Codex Responses API 端点 (`/v1/responses`)
- 会话管理系统（多轮对话跟踪）
- Messages API 多上游协议支持（Claude/OpenAI/Gemini）

---

## [v2.0.0-go] - 2025-10-15

### 🎉 Go 语言重写版本

- **性能提升**: 启动速度 20x，内存占用 -70%
- **单文件部署**: 前端资源嵌入二进制
- **完整功能移植**: 所有上游适配器、协议转换、流式响应、配置热重载

---

## 历史版本

<details>
<summary>v1.x TypeScript 版本</summary>

### v1.2.0 - 2025-09-19
- Web 管理界面、模型映射、渠道置顶、API 密钥故障转移

### v1.1.0 - 2025-09-17
- SSE 数据解析优化、Bearer Token 处理简化、代码重构

### v1.0.0 - 2025-09-13
- 初始版本：多上游支持、负载均衡、配置管理

</details>
