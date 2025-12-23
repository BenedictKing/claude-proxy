# 版本历史

> **注意**: v2.0.0 开始为 Go 语言重写版本，v1.x 为 TypeScript 版本

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
