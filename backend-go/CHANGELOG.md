# Changelog

## [v2.1.15] - 2025-12-12

### Fixed - 安全加固与资源管理优化

**本次修改解决的问题**：
1. 请求体无大小限制，可被 DoS 攻击利用
2. ConfigManager 存在 goroutine 泄漏和资源未释放问题
3. 负载均衡计数器存在数据竞争风险
4. 服务器缺少优雅关闭机制

**具体改动**：

1. **请求体大小限制** (`handlers/proxy.go`, `handlers/responses.go`)
   - 新增 `MAX_REQUEST_BODY_SIZE_MB` 环境变量（默认 50MB）
   - `/v1/messages` 和 `/v1/responses` 端点均应用限制
   - 超限返回 413 状态码

2. **Goroutine 泄漏修复** (`config/config.go`)
   - 添加 `stopChan` 用于通知后台 goroutine 退出
   - `startWatcher()` 和 `cleanupExpiredFailures()` 监听停止信号
   - 添加 `Close()` 方法释放 watcher 资源

3. **数据竞争修复** (`config/config.go`)
   - `requestCount` 和 `responsesRequestCount` 改为 `int64` 类型
   - 使用 `sync/atomic.AddInt64()` 进行原子操作

4. **优雅关闭** (`main.go`)
   - 监听 SIGINT/SIGTERM 信号
   - 10 秒超时优雅关闭 HTTP 服务器
   - `defer cfgManager.Close()` 确保资源释放
   - 根据关闭结果输出准确日志

5. **Close() 幂等性** (`config/config.go`)
   - 使用 `sync.Once` 确保多次调用不会 panic

## [v2.1.7] - 2025-12-11

### Fixed - Token 计数补全：处理虚假值场景

**问题背景**：
- 某些上游服务返回 usage 但 `input_tokens` 为 0 或 1（虚假值）
- 实际 token 计数在 `cache_creation_input_tokens` 等字段中
- 示例：`{"input_tokens":1,"cache_creation_input_tokens":89623,...}`

**解决方案**：
- 当 `input_tokens <= 1` 时，用本地估算值覆盖
- 当 `output_tokens == 0` 时，用本地估算值覆盖
- 保留上游返回的其他字段（cache_creation_input_tokens 等）

**具体改动**：
1. `internal/handlers/proxy.go`
   - `handleNormalResponse()` - 增加 input_tokens/output_tokens 虚假值检测和补全
   - `handleStreamResponse()` - 流式响应中检测并修补虚假 token 值
   - `checkEventUsageStatus()` - 替代 checkEventHasUsage，返回是否需要修补
   - `patchTokensInEvent()` - 修补 SSE 事件中的 token 字段
   - `patchUsageFields()` - 修补 usage 对象中的 token 字段

## [v2.1.6] - 2025-12-11

### Added - Messages API Token 计数补全

**问题背景**：
- 某些 OpenAI 兼容的上游服务不返回 token 计数（usage 字段为空）
- 客户端依赖 usage 信息进行成本统计和限流

**解决方案**：
- 当上游响应没有返回 usage 时，本地估算 token 数量并附加到响应中
- 使用字符估算法：CJK 字符 ~1.5 字符/token，英文 ~3.5 字符/token

**具体改动**：
1. `internal/utils/token_counter.go` - 新增 token 估算工具
   - `EstimateTokens()` - 基于字符估算 token 数量
   - `EstimateRequestTokens()` - 估算请求的输入 token
   - `EstimateResponseTokens()` - 估算响应的输出 token
2. `internal/handlers/proxy.go`
   - `handleNormalResponse()` - 非流式响应 Usage 补全
   - `handleStreamResponse()` - 流式响应 Usage 补全（在 message_stop 之前注入）
   - `buildUsageEvent()` - 构建带 usage 的 SSE 事件
   - `extractTextFromEvent()` - 从流式事件中提取文本（支持 text_delta 和 partial_json）
   - `checkEventHasUsage()` - 使用 JSON 解析精确检测 usage 字段，避免误判

## [v2.1.2] - 2025-12-11

### Changed - Gin Logger 过滤：减少 /api/channels 轮询日志噪音

**问题背景**：
- 前端 Web UI 每隔几秒轮询 `/api/channels`、`/api/channels/metrics`、`/api/channels/scheduler/stats`
- 这些请求产生大量 `[GIN]` 日志，淹没了真正重要的 API 调用日志（如 `/v1/messages`）

**解决方案**：
- 新增 `internal/middleware/logger.go`，使用 Gin 官方 `gin.LoggerWithConfig` + `Skip` 函数
- 通过 `QUIET_POLLING_LOGS` 环境变量控制（默认 true，开启过滤）
- 仅过滤 GET 请求，POST/PUT/DELETE 管理操作始终记录日志以保留审计跟踪

**具体改动**：
1. `internal/middleware/logger.go` - 新增 FilteredLogger 中间件
2. `internal/config/env.go` - `QUIET_POLLING_LOGS` 环境变量（默认 true）
3. `main.go` - 将 `gin.Default()` 改为 `gin.New()` + `FilteredLogger(envCfg)` + `Recovery()`

**使用方式**：
```bash
# 默认已启用轮询日志过滤

# 如需显示所有日志（调试用），在 .env 文件中设置：
QUIET_POLLING_LOGS=false
```

## [v2.1.1] - 2025-12-11

### Changed - 版本号更新

## [v2.1.0] - 2025-12-11

### Changed - 指标系统重构：从渠道索引绑定改为 Key 级别绑定

**问题背景**：
- 新建的促销渠道被标记为"不健康"，尽管是全新的 API Key
- 根因：指标绑定到 channel index，而非 `BaseURL + APIKey` 组合

**解决方案**：
- 指标键改为 `hash(baseURL + "|" + apiKey)` 的前 16 位
- 每个 Key 独立追踪：请求数、成功/失败数、连续失败数、熔断状态
- 渠道健康状态通过聚合其所有活跃 Key 的指标计算

**具体改动**：
1. `internal/metrics/channel_metrics.go`
   - 新增 `KeyMetrics` 结构体
   - `RecordSuccess/RecordFailure` 改为接收 `(baseURL, apiKey)` 参数
   - 新增 `cleanupStaleKeys()` 清理 48 小时无活动的 Key
   - 修复 `appendToHistoryKey` 内存泄漏（所有记录过期时未清空）
   - `ToResponse` 中 `ConsecutiveFailures` 改用 max 而非 sum

2. `internal/handlers/proxy.go` / `responses.go`
   - 按 Key 记录失败，而非按渠道

3. `internal/handlers/channel_metrics_handler.go`
   - 新增 `GetChannelMetricsWithConfig` 聚合处理器

4. `main.go`
   - 路由绑定到新的 handler

### Fixed - Codex Review 发现的问题

1. **熔断器未生效** (`proxy.go:213-218`)
   - 在 `tryChannelWithAllKeys` 中调用 `ShouldSuspendKey()` 跳过熔断的 Key

2. **单渠道路径缺少指标记录** (`proxy.go:361, 400, 416, 462`)
   - `handleSingleChannelProxy` 添加 `channelScheduler` 参数
   - 在转换失败、发送失败、failover、成功时记录指标

3. **非 failover 错误被计为成功** (`proxy.go:274`)
   - 返回 `successKey=""` 表示不记录成功指标

4. **`GetChannelAggregatedMetrics` 中 `ConsecutiveFailures` 用 sum** (`channel_metrics.go:379-401`)
   - 改用 max 聚合，与 `ToResponse` 保持一致

5. **单渠道路径缺少熔断检查** (`proxy.go:341-359`)
   - `handleSingleChannelProxy` 添加 `ShouldSuspendKey()` 检查，跳过熔断的 Key

6. **`responses.go` 非 failover 错误被计为成功** (`responses.go:247`)
   - 与 `proxy.go` 保持一致，返回 `successKey=""` 不记录成功

7. **恢复 `timeWindows` 字段** (`channel_metrics.go:613, 721-773`)
   - `MetricsResponse` 添加 `TimeWindows` 字段
   - `ToResponse` 中计算聚合的分时段统计
   - 前端兼容性保持
