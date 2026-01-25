package metrics

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
)

// RequestRecord 带时间戳的请求记录（扩展版，支持 Token 和 Cache 数据）
type RequestRecord struct {
	Timestamp                time.Time
	Success                  bool
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
}

// KeyMetrics 单个 Key 的指标（绑定到 BaseURL + Key 组合）
type KeyMetrics struct {
	MetricsKey          string     `json:"metricsKey"`          // hash(baseURL + apiKey)
	BaseURL             string     `json:"baseUrl"`             // 用于显示
	KeyMask             string     `json:"keyMask"`             // 脱敏的 key（用于显示）
	RequestCount        int64      `json:"requestCount"`        // 总请求数
	SuccessCount        int64      `json:"successCount"`        // 成功数
	FailureCount        int64      `json:"failureCount"`        // 失败数
	ConsecutiveFailures int64      `json:"consecutiveFailures"` // 连续失败数
	ActiveRequests      int64      `json:"activeRequests"`      // 进行中的请求数
	LastSuccessAt       *time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt       *time.Time `json:"lastFailureAt,omitempty"`
	CircuitBrokenAt     *time.Time `json:"circuitBrokenAt,omitempty"` // 熔断开始时间
	// 滑动窗口记录（最近 N 次请求的结果）
	recentResults []bool // true=success, false=failure
	// 带时间戳的请求记录（用于分时段统计，保留24小时）
	requestHistory []RequestRecord
	// 进行中请求在 requestHistory 中的索引（用于“连接即计数”，结束后回写成功/失败与 token）
	pendingHistoryIdx map[uint64]int
}

// ChannelMetrics 渠道聚合指标（用于 API 返回，兼容旧结构）
type ChannelMetrics struct {
	ChannelIndex        int        `json:"channelIndex"`
	RequestCount        int64      `json:"requestCount"`
	SuccessCount        int64      `json:"successCount"`
	FailureCount        int64      `json:"failureCount"`
	ConsecutiveFailures int64      `json:"consecutiveFailures"`
	LastSuccessAt       *time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt       *time.Time `json:"lastFailureAt,omitempty"`
	CircuitBrokenAt     *time.Time `json:"circuitBrokenAt,omitempty"`
	// 滑动窗口记录（兼容旧代码）
	recentResults []bool
	// 带时间戳的请求记录
	requestHistory []RequestRecord
}

// TimeWindowStats 分时段统计
type TimeWindowStats struct {
	RequestCount int64   `json:"requestCount"`
	SuccessCount int64   `json:"successCount"`
	FailureCount int64   `json:"failureCount"`
	SuccessRate  float64 `json:"successRate"`
	// Token 统计（按时间窗口聚合）
	InputTokens         int64 `json:"inputTokens,omitempty"`
	OutputTokens        int64 `json:"outputTokens,omitempty"`
	CacheCreationTokens int64 `json:"cacheCreationTokens,omitempty"`
	CacheReadTokens     int64 `json:"cacheReadTokens,omitempty"`
	// CacheHitRate 缓存命中率（Token口径），范围 0-100
	// 定义：cacheReadTokens / (cacheReadTokens + inputTokens) * 100
	CacheHitRate float64 `json:"cacheHitRate,omitempty"`
}

// MetricsManager 指标管理器
type MetricsManager struct {
	mu                  sync.RWMutex
	keyMetrics          map[string]*KeyMetrics // key: hash(baseURL + apiKey)
	windowSize          int                    // 滑动窗口大小
	failureThreshold    float64                // 失败率阈值
	circuitRecoveryTime time.Duration          // 熔断恢复时间
	stopCh              chan struct{}          // 用于停止清理 goroutine
	nextRequestID       uint64                 // 单进程递增请求ID（用于 pendingHistoryIdx）

	// 持久化存储（可选）
	store   PersistenceStore
	apiType string // "messages" 或 "responses"
}

// NewMetricsManager 创建指标管理器
func NewMetricsManager() *MetricsManager {
	m := &MetricsManager{
		keyMetrics:          make(map[string]*KeyMetrics),
		windowSize:          10,               // 默认基于最近 10 次请求计算失败率
		failureThreshold:    0.5,              // 默认 50% 失败率阈值
		circuitRecoveryTime: 15 * time.Minute, // 默认 15 分钟自动恢复
		stopCh:              make(chan struct{}),
	}
	// 启动后台熔断恢复任务
	go m.cleanupCircuitBreakers()
	return m
}

// NewMetricsManagerWithConfig 创建带配置的指标管理器
func NewMetricsManagerWithConfig(windowSize int, failureThreshold float64) *MetricsManager {
	if windowSize < 3 {
		windowSize = 3 // 最小 3
	}
	if failureThreshold <= 0 || failureThreshold > 1 {
		failureThreshold = 0.5
	}
	m := &MetricsManager{
		keyMetrics:          make(map[string]*KeyMetrics),
		windowSize:          windowSize,
		failureThreshold:    failureThreshold,
		circuitRecoveryTime: 15 * time.Minute,
		stopCh:              make(chan struct{}),
	}
	// 启动后台熔断恢复任务
	go m.cleanupCircuitBreakers()
	return m
}

// NewMetricsManagerWithPersistence 创建带持久化的指标管理器
func NewMetricsManagerWithPersistence(windowSize int, failureThreshold float64, store PersistenceStore, apiType string) *MetricsManager {
	if windowSize < 3 {
		windowSize = 3
	}
	if failureThreshold <= 0 || failureThreshold > 1 {
		failureThreshold = 0.5
	}
	m := &MetricsManager{
		keyMetrics:          make(map[string]*KeyMetrics),
		windowSize:          windowSize,
		failureThreshold:    failureThreshold,
		circuitRecoveryTime: 15 * time.Minute,
		stopCh:              make(chan struct{}),
		store:               store,
		apiType:             apiType,
	}

	// 从持久化存储加载历史数据
	if store != nil {
		if err := m.loadFromStore(); err != nil {
			log.Printf("[Metrics-Load] 警告: [%s] 加载历史指标数据失败: %v", apiType, err)
		}
	}

	// 启动后台熔断恢复任务
	go m.cleanupCircuitBreakers()
	return m
}

// loadFromStore 从持久化存储加载数据
func (m *MetricsManager) loadFromStore() error {
	if m.store == nil {
		return nil
	}

	// 加载最近 24 小时的数据
	since := time.Now().Add(-24 * time.Hour)
	records, err := m.store.LoadRecords(since, m.apiType)
	if err != nil {
		return err
	}

	if len(records) == 0 {
		log.Printf("[Metrics-Load] [%s] 无历史指标数据需要加载", m.apiType)
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 重建内存中的 KeyMetrics
	for _, r := range records {
		metrics := m.getOrCreateKeyLocked(r.BaseURL, r.MetricsKey, r.KeyMask)

		// 重建请求历史
		metrics.requestHistory = append(metrics.requestHistory, RequestRecord{
			Timestamp:                r.Timestamp,
			Success:                  r.Success,
			InputTokens:              r.InputTokens,
			OutputTokens:             r.OutputTokens,
			CacheCreationInputTokens: r.CacheCreationTokens,
			CacheReadInputTokens:     r.CacheReadTokens,
		})

		// 更新聚合计数
		metrics.RequestCount++
		if r.Success {
			metrics.SuccessCount++
			if metrics.LastSuccessAt == nil || r.Timestamp.After(*metrics.LastSuccessAt) {
				t := r.Timestamp
				metrics.LastSuccessAt = &t
			}
		} else {
			metrics.FailureCount++
			if metrics.LastFailureAt == nil || r.Timestamp.After(*metrics.LastFailureAt) {
				t := r.Timestamp
				metrics.LastFailureAt = &t
			}
		}
	}

	// 重建滑动窗口（只从最近 15 分钟的记录中取最近 windowSize 条）
	// 避免历史失败记录导致渠道长期处于不健康状态
	windowCutoff := time.Now().Add(-15 * time.Minute)
	for _, metrics := range m.keyMetrics {
		metrics.recentResults = make([]bool, 0, m.windowSize)
		// 从历史记录中筛选最近 15 分钟内的记录
		var recentRecords []bool
		for _, record := range metrics.requestHistory {
			if record.Timestamp.After(windowCutoff) {
				recentRecords = append(recentRecords, record.Success)
			}
		}
		// 取最近 windowSize 条
		n := len(recentRecords)
		start := 0
		if n > m.windowSize {
			start = n - m.windowSize
		}
		for i := start; i < n; i++ {
			metrics.recentResults = append(metrics.recentResults, recentRecords[i])
		}
	}

	log.Printf("[Metrics-Load] [%s] 已从持久化存储加载 %d 条历史记录，重建 %d 个 Key 指标",
		m.apiType, len(records), len(m.keyMetrics))
	return nil
}

// getOrCreateKeyLocked 获取或创建 Key 指标（用于加载时，已知 metricsKey 和 keyMask）
func (m *MetricsManager) getOrCreateKeyLocked(baseURL, metricsKey, keyMask string) *KeyMetrics {
	if metrics, exists := m.keyMetrics[metricsKey]; exists {
		return metrics
	}
	metrics := &KeyMetrics{
		MetricsKey:        metricsKey,
		BaseURL:           baseURL,
		KeyMask:           keyMask,
		recentResults:     make([]bool, 0, m.windowSize),
		pendingHistoryIdx: make(map[uint64]int),
	}
	m.keyMetrics[metricsKey] = metrics
	return metrics
}

// generateMetricsKey 生成指标键 hash(baseURL + apiKey)（内部使用）
func generateMetricsKey(baseURL, apiKey string) string {
	h := sha256.New()
	h.Write([]byte(baseURL + "|" + apiKey))
	return hex.EncodeToString(h.Sum(nil))[:16] // 取前16位作为键
}

// GenerateMetricsKey 生成指标键 hash(baseURL + apiKey)（导出供外部使用）
func GenerateMetricsKey(baseURL, apiKey string) string {
	return generateMetricsKey(baseURL, apiKey)
}

// getOrCreateKey 获取或创建 Key 指标
func (m *MetricsManager) getOrCreateKey(baseURL, apiKey string) *KeyMetrics {
	metricsKey := generateMetricsKey(baseURL, apiKey)
	if metrics, exists := m.keyMetrics[metricsKey]; exists {
		return metrics
	}
	metrics := &KeyMetrics{
		MetricsKey:        metricsKey,
		BaseURL:           baseURL,
		KeyMask:           utils.MaskAPIKey(apiKey),
		recentResults:     make([]bool, 0, m.windowSize),
		pendingHistoryIdx: make(map[uint64]int),
	}
	m.keyMetrics[metricsKey] = metrics
	return metrics
}

// RecordSuccess 记录成功请求（新方法，使用 baseURL + apiKey）
func (m *MetricsManager) RecordSuccess(baseURL, apiKey string) {
	m.RecordSuccessWithUsage(baseURL, apiKey, nil)
}

// RecordSuccessWithUsage 记录成功请求（带 Usage 数据）
func (m *MetricsManager) RecordSuccessWithUsage(baseURL, apiKey string, usage *types.Usage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordSuccessWithUsageLocked(baseURL, apiKey, usage, time.Now())
}

func (m *MetricsManager) recordSuccessWithUsageLocked(baseURL, apiKey string, usage *types.Usage, now time.Time) {
	metrics := m.getOrCreateKey(baseURL, apiKey)
	metrics.RequestCount++
	metrics.SuccessCount++
	metrics.ConsecutiveFailures = 0

	metrics.LastSuccessAt = &now

	// 成功后清除熔断标记
	if metrics.CircuitBrokenAt != nil {
		metrics.CircuitBrokenAt = nil
		log.Printf("[Metrics-Circuit] Key [%s] (%s) 因请求成功退出熔断状态", metrics.KeyMask, metrics.BaseURL)
	}

	// 更新滑动窗口
	m.appendToWindowKey(metrics, true)

	// 提取 Token 数据（如果有）
	var inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int64
	if usage != nil {
		inputTokens = int64(usage.InputTokens)
		outputTokens = int64(usage.OutputTokens)
		// cache_creation_input_tokens 有时不会返回（只返回 5m/1h 细分字段），这里做兜底汇总。
		cacheCreationTokens = int64(usage.CacheCreationInputTokens)
		if cacheCreationTokens <= 0 {
			cacheCreationTokens = int64(usage.CacheCreation5mInputTokens + usage.CacheCreation1hInputTokens)
		}
		cacheReadTokens = int64(usage.CacheReadInputTokens)
	}

	// 记录带时间戳的请求
	m.appendToHistoryKeyWithUsage(metrics, now, true, inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens)

	// 写入持久化存储（异步，不阻塞）
	if m.store != nil {
		m.store.AddRecord(PersistentRecord{
			MetricsKey:          metrics.MetricsKey,
			BaseURL:             baseURL,
			KeyMask:             metrics.KeyMask,
			Timestamp:           now,
			Success:             true,
			InputTokens:         inputTokens,
			OutputTokens:        outputTokens,
			CacheCreationTokens: cacheCreationTokens,
			CacheReadTokens:     cacheReadTokens,
			APIType:             m.apiType,
		})
	}
}

// RecordFailure 记录失败请求（新方法，使用 baseURL + apiKey）
func (m *MetricsManager) RecordFailure(baseURL, apiKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordFailureLocked(baseURL, apiKey, time.Now())
}

func (m *MetricsManager) recordFailureLocked(baseURL, apiKey string, now time.Time) {
	metrics := m.getOrCreateKey(baseURL, apiKey)
	metrics.RequestCount++
	metrics.FailureCount++
	metrics.ConsecutiveFailures++

	metrics.LastFailureAt = &now

	// 更新滑动窗口
	m.appendToWindowKey(metrics, false)

	// 检查是否刚进入熔断状态
	if metrics.CircuitBrokenAt == nil && m.isKeyCircuitBroken(metrics) {
		metrics.CircuitBrokenAt = &now
		log.Printf("[Metrics-Circuit] Key [%s] (%s) 进入熔断状态（失败率: %.1f%%）", metrics.KeyMask, metrics.BaseURL, m.calculateKeyFailureRateInternal(metrics)*100)
	}

	// 记录带时间戳的请求
	m.appendToHistoryKey(metrics, now, false)

	// 写入持久化存储（异步，不阻塞）
	if m.store != nil {
		m.store.AddRecord(PersistentRecord{
			MetricsKey:          metrics.MetricsKey,
			BaseURL:             baseURL,
			KeyMask:             metrics.KeyMask,
			Timestamp:           now,
			Success:             false,
			InputTokens:         0,
			OutputTokens:        0,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
			APIType:             m.apiType,
		})
	}
}

// RecordRequestConnected 记录“开始发起上游请求（TCP 建连阶段）”的请求（用于更实时的活跃度统计）。
// 返回 requestID，用于后续在请求结束时回写成功/失败与 token。
func (m *MetricsManager) RecordRequestConnected(baseURL, apiKey string) uint64 {
	return m.RecordRequestConnectedAt(baseURL, apiKey, time.Now())
}

// RecordRequestConnectedAt 与 RecordRequestConnected 相同，但允许注入时间戳（用于测试）。
func (m *MetricsManager) RecordRequestConnectedAt(baseURL, apiKey string, timestamp time.Time) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := m.getOrCreateKey(baseURL, apiKey)
	// RequestCount 改为在 finalize 阶段统一增加，避免 fallback 路径二次计数

	m.nextRequestID++
	requestID := m.nextRequestID

	if metrics.pendingHistoryIdx == nil {
		metrics.pendingHistoryIdx = make(map[uint64]int)
	}

	metrics.requestHistory = append(metrics.requestHistory, RequestRecord{
		Timestamp: timestamp,
		Success:   true, // 先按成功计数；结束时会回写真实结果
	})
	metrics.pendingHistoryIdx[requestID] = len(metrics.requestHistory) - 1

	// 清理历史并同步修正索引
	m.cleanupHistoryLocked(metrics)

	return requestID
}

// RecordRequestFinalizeSuccess 回写成功结果与 token（requestID 来自 RecordRequestConnected）。
func (m *MetricsManager) RecordRequestFinalizeSuccess(baseURL, apiKey string, requestID uint64, usage *types.Usage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	metrics, exists := m.keyMetrics[metricsKey]
	if !exists {
		m.recordSuccessWithUsageLocked(baseURL, apiKey, usage, time.Now())
		return
	}

	idx, ok := metrics.pendingHistoryIdx[requestID]
	if !ok || idx < 0 || idx >= len(metrics.requestHistory) {
		m.recordSuccessWithUsageLocked(baseURL, apiKey, usage, time.Now())
		return
	}
	delete(metrics.pendingHistoryIdx, requestID)

	// 正常路径：在此统一增加 RequestCount
	metrics.RequestCount++
	metrics.SuccessCount++
	metrics.ConsecutiveFailures = 0

	now := time.Now()
	metrics.LastSuccessAt = &now

	// 成功后清除熔断标记
	if metrics.CircuitBrokenAt != nil {
		metrics.CircuitBrokenAt = nil
		log.Printf("[Metrics-Circuit] Key [%s] (%s) 因请求成功退出熔断状态", metrics.KeyMask, metrics.BaseURL)
	}

	// 更新滑动窗口
	m.appendToWindowKey(metrics, true)

	// 提取 Token 数据（如果有）
	var inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int64
	if usage != nil {
		inputTokens = int64(usage.InputTokens)
		outputTokens = int64(usage.OutputTokens)
		// cache_creation_input_tokens 有时不会返回（只返回 5m/1h 细分字段），这里做兜底汇总。
		cacheCreationTokens = int64(usage.CacheCreationInputTokens)
		if cacheCreationTokens <= 0 {
			cacheCreationTokens = int64(usage.CacheCreation5mInputTokens + usage.CacheCreation1hInputTokens)
		}
		cacheReadTokens = int64(usage.CacheReadInputTokens)
	}

	// 回写历史记录（时间戳保持为“请求开始（TCP 建连阶段）”时刻）
	record := &metrics.requestHistory[idx]
	record.Success = true
	record.InputTokens = inputTokens
	record.OutputTokens = outputTokens
	record.CacheCreationInputTokens = cacheCreationTokens
	record.CacheReadInputTokens = cacheReadTokens

	// 写入持久化存储（异步，不阻塞）
	if m.store != nil {
		m.store.AddRecord(PersistentRecord{
			MetricsKey:          metrics.MetricsKey,
			BaseURL:             baseURL,
			KeyMask:             metrics.KeyMask,
			Timestamp:           record.Timestamp,
			Success:             true,
			InputTokens:         inputTokens,
			OutputTokens:        outputTokens,
			CacheCreationTokens: cacheCreationTokens,
			CacheReadTokens:     cacheReadTokens,
			APIType:             m.apiType,
		})
	}
}

// RecordRequestFinalizeFailure 回写失败结果（requestID 来自 RecordRequestConnected）。
func (m *MetricsManager) RecordRequestFinalizeFailure(baseURL, apiKey string, requestID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	metrics, exists := m.keyMetrics[metricsKey]
	if !exists {
		m.recordFailureLocked(baseURL, apiKey, time.Now())
		return
	}

	idx, ok := metrics.pendingHistoryIdx[requestID]
	if !ok || idx < 0 || idx >= len(metrics.requestHistory) {
		m.recordFailureLocked(baseURL, apiKey, time.Now())
		return
	}
	delete(metrics.pendingHistoryIdx, requestID)

	// 正常路径：在此统一增加 RequestCount
	metrics.RequestCount++
	metrics.FailureCount++
	metrics.ConsecutiveFailures++

	now := time.Now()
	metrics.LastFailureAt = &now

	// 更新滑动窗口
	m.appendToWindowKey(metrics, false)

	// 检查是否刚进入熔断状态
	if metrics.CircuitBrokenAt == nil && m.isKeyCircuitBroken(metrics) {
		metrics.CircuitBrokenAt = &now
		log.Printf("[Metrics-Circuit] Key [%s] (%s) 进入熔断状态（失败率: %.1f%%）", metrics.KeyMask, metrics.BaseURL, m.calculateKeyFailureRateInternal(metrics)*100)
	}

	// 回写历史记录（时间戳保持为“请求开始（TCP 建连阶段）”时刻）
	record := &metrics.requestHistory[idx]
	record.Success = false
	record.InputTokens = 0
	record.OutputTokens = 0
	record.CacheCreationInputTokens = 0
	record.CacheReadInputTokens = 0

	// 写入持久化存储（异步，不阻塞）
	if m.store != nil {
		m.store.AddRecord(PersistentRecord{
			MetricsKey:          metrics.MetricsKey,
			BaseURL:             baseURL,
			KeyMask:             metrics.KeyMask,
			Timestamp:           record.Timestamp,
			Success:             false,
			InputTokens:         0,
			OutputTokens:        0,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
			APIType:             m.apiType,
		})
	}
}

// RecordRequestFinalizeClientCancel 记录客户端取消的请求（计入总请求数但不计入失败）
func (m *MetricsManager) RecordRequestFinalizeClientCancel(baseURL, apiKey string, requestID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	metrics, exists := m.keyMetrics[metricsKey]
	if !exists {
		return
	}

	idx, ok := metrics.pendingHistoryIdx[requestID]
	if !ok || idx < 0 || idx >= len(metrics.requestHistory) {
		return
	}
	delete(metrics.pendingHistoryIdx, requestID)

	// 仅计入总请求数，不计入失败数
	metrics.RequestCount++
	// 注意：不重置 ConsecutiveFailures，客户端取消不应影响连续失败计数

	// 不更新滑动窗口（不影响失败率计算）
	// 不检查熔断状态（客户端取消不应触发熔断）

	// 从历史记录中移除（客户端取消不记录）
	metrics.requestHistory = append(metrics.requestHistory[:idx], metrics.requestHistory[idx+1:]...)
	// 更新后续索引
	for rid, ridx := range metrics.pendingHistoryIdx {
		if ridx > idx {
			metrics.pendingHistoryIdx[rid] = ridx - 1
		}
	}
}

// RecordRequestStart 记录请求开始（增加进行中计数）
func (m *MetricsManager) RecordRequestStart(baseURL, apiKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := m.getOrCreateKey(baseURL, apiKey)
	metrics.ActiveRequests++
}

// RecordRequestEnd 记录请求结束（减少进行中计数）
func (m *MetricsManager) RecordRequestEnd(baseURL, apiKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	if metrics, exists := m.keyMetrics[metricsKey]; exists {
		if metrics.ActiveRequests > 0 {
			metrics.ActiveRequests--
		}
	}
}

// isKeyCircuitBroken 判断 Key 是否达到熔断条件（内部方法，调用前需持有锁）
func (m *MetricsManager) isKeyCircuitBroken(metrics *KeyMetrics) bool {
	// 最小请求数保护：至少 max(3, windowSize/2) 次请求才判断熔断
	minRequests := max(3, m.windowSize/2)
	if len(metrics.recentResults) < minRequests {
		return false
	}
	return m.calculateKeyFailureRateInternal(metrics) >= m.failureThreshold
}

// calculateKeyFailureRateInternal 计算 Key 失败率（内部方法，调用前需持有锁）
func (m *MetricsManager) calculateKeyFailureRateInternal(metrics *KeyMetrics) float64 {
	if len(metrics.recentResults) == 0 {
		return 0
	}
	failures := 0
	for _, success := range metrics.recentResults {
		if !success {
			failures++
		}
	}
	return float64(failures) / float64(len(metrics.recentResults))
}

// appendToWindowKey 向 Key 滑动窗口添加记录
func (m *MetricsManager) appendToWindowKey(metrics *KeyMetrics, success bool) {
	metrics.recentResults = append(metrics.recentResults, success)
	// 保持窗口大小
	if len(metrics.recentResults) > m.windowSize {
		metrics.recentResults = metrics.recentResults[1:]
	}
}

// appendToHistoryKey 向 Key 历史记录添加请求（保留24小时）
func (m *MetricsManager) appendToHistoryKey(metrics *KeyMetrics, timestamp time.Time, success bool) {
	m.appendToHistoryKeyWithUsage(metrics, timestamp, success, 0, 0, 0, 0)
}

// cleanupHistoryLocked 清理超过 24 小时的历史记录，并同步修正 pendingHistoryIdx 索引。
// 注意：调用方需要持有写锁。
func (m *MetricsManager) cleanupHistoryLocked(metrics *KeyMetrics) {
	if metrics == nil || len(metrics.requestHistory) == 0 {
		return
	}

	cutoff := time.Now().Add(-24 * time.Hour)

	newStart := -1
	for i, record := range metrics.requestHistory {
		if record.Timestamp.After(cutoff) {
			newStart = i
			break
		}
	}

	if newStart > 0 {
		metrics.requestHistory = metrics.requestHistory[newStart:]
		// 索引平移：老数据被切走后，pending 索引需要整体减去 newStart
		if metrics.pendingHistoryIdx != nil && len(metrics.pendingHistoryIdx) > 0 {
			for id, idx := range metrics.pendingHistoryIdx {
				if idx < newStart {
					delete(metrics.pendingHistoryIdx, id)
					continue
				}
				metrics.pendingHistoryIdx[id] = idx - newStart
			}
		}
		return
	}

	if newStart == -1 {
		// 所有记录都过期，清空切片
		metrics.requestHistory = metrics.requestHistory[:0]
		if metrics.pendingHistoryIdx != nil {
			for id := range metrics.pendingHistoryIdx {
				delete(metrics.pendingHistoryIdx, id)
			}
		}
	}
}

// appendToHistoryKeyWithUsage 向 Key 历史记录添加请求（带 Usage 数据）
func (m *MetricsManager) appendToHistoryKeyWithUsage(metrics *KeyMetrics, timestamp time.Time, success bool, inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int64) {
	metrics.requestHistory = append(metrics.requestHistory, RequestRecord{
		Timestamp:                timestamp,
		Success:                  success,
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheCreationInputTokens: cacheCreationTokens,
		CacheReadInputTokens:     cacheReadTokens,
	})

	// 清理超过 24 小时的记录
	m.cleanupHistoryLocked(metrics)
}

// IsKeyHealthy 判断单个 Key 是否健康
func (m *MetricsManager) IsKeyHealthy(baseURL, apiKey string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	metrics, exists := m.keyMetrics[metricsKey]
	if !exists || len(metrics.recentResults) == 0 {
		return true // 没有记录，默认健康
	}

	return m.calculateKeyFailureRateInternal(metrics) < m.failureThreshold
}

// IsChannelHealthy 判断渠道是否健康（基于当前活跃 Keys 聚合计算）
// activeKeys: 当前渠道配置的所有活跃 API Keys
func (m *MetricsManager) IsChannelHealthyWithKeys(baseURL string, activeKeys []string) bool {
	if len(activeKeys) == 0 {
		return false // 没有 Key，不健康
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// 聚合所有活跃 Key 的指标
	var totalResults []bool
	for _, apiKey := range activeKeys {
		metricsKey := generateMetricsKey(baseURL, apiKey)
		if metrics, exists := m.keyMetrics[metricsKey]; exists {
			totalResults = append(totalResults, metrics.recentResults...)
		}
	}

	// 没有任何记录，默认健康
	if len(totalResults) == 0 {
		return true
	}

	// 最小请求数保护：至少 max(3, windowSize/2) 次请求才判断健康状态
	minRequests := max(3, m.windowSize/2)
	if len(totalResults) < minRequests {
		return true // 请求数不足，默认健康
	}

	// 计算聚合失败率
	failures := 0
	for _, success := range totalResults {
		if !success {
			failures++
		}
	}
	failureRate := float64(failures) / float64(len(totalResults))

	return failureRate < m.failureThreshold
}

// CalculateKeyFailureRate 计算单个 Key 的失败率
func (m *MetricsManager) CalculateKeyFailureRate(baseURL, apiKey string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	metrics, exists := m.keyMetrics[metricsKey]
	if !exists || len(metrics.recentResults) == 0 {
		return 0
	}

	return m.calculateKeyFailureRateInternal(metrics)
}

// CalculateChannelFailureRate 计算渠道聚合失败率
func (m *MetricsManager) CalculateChannelFailureRate(baseURL string, activeKeys []string) float64 {
	if len(activeKeys) == 0 {
		return 0
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalResults []bool
	for _, apiKey := range activeKeys {
		metricsKey := generateMetricsKey(baseURL, apiKey)
		if metrics, exists := m.keyMetrics[metricsKey]; exists {
			totalResults = append(totalResults, metrics.recentResults...)
		}
	}

	if len(totalResults) == 0 {
		return 0
	}

	failures := 0
	for _, success := range totalResults {
		if !success {
			failures++
		}
	}

	return float64(failures) / float64(len(totalResults))
}

// GetKeyMetrics 获取单个 Key 的指标
func (m *MetricsManager) GetKeyMetrics(baseURL, apiKey string) *KeyMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	if metrics, exists := m.keyMetrics[metricsKey]; exists {
		// 返回副本
		return &KeyMetrics{
			MetricsKey:          metrics.MetricsKey,
			BaseURL:             metrics.BaseURL,
			KeyMask:             metrics.KeyMask,
			RequestCount:        metrics.RequestCount,
			SuccessCount:        metrics.SuccessCount,
			FailureCount:        metrics.FailureCount,
			ConsecutiveFailures: metrics.ConsecutiveFailures,
			LastSuccessAt:       metrics.LastSuccessAt,
			LastFailureAt:       metrics.LastFailureAt,
			CircuitBrokenAt:     metrics.CircuitBrokenAt,
		}
	}
	return nil
}

// GetChannelAggregatedMetrics 获取渠道聚合指标（基于活跃 Keys）
func (m *MetricsManager) GetChannelAggregatedMetrics(channelIndex int, baseURL string, activeKeys []string) *ChannelMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	aggregated := &ChannelMetrics{
		ChannelIndex: channelIndex,
	}

	var latestSuccess, latestFailure, latestCircuitBroken *time.Time
	var maxConsecutiveFailures int64

	for _, apiKey := range activeKeys {
		metricsKey := generateMetricsKey(baseURL, apiKey)
		if metrics, exists := m.keyMetrics[metricsKey]; exists {
			aggregated.RequestCount += metrics.RequestCount
			aggregated.SuccessCount += metrics.SuccessCount
			aggregated.FailureCount += metrics.FailureCount
			if metrics.ConsecutiveFailures > maxConsecutiveFailures {
				maxConsecutiveFailures = metrics.ConsecutiveFailures
			}
			aggregated.recentResults = append(aggregated.recentResults, metrics.recentResults...)
			aggregated.requestHistory = append(aggregated.requestHistory, metrics.requestHistory...)

			// 取最新的时间戳
			if metrics.LastSuccessAt != nil && (latestSuccess == nil || metrics.LastSuccessAt.After(*latestSuccess)) {
				latestSuccess = metrics.LastSuccessAt
			}
			if metrics.LastFailureAt != nil && (latestFailure == nil || metrics.LastFailureAt.After(*latestFailure)) {
				latestFailure = metrics.LastFailureAt
			}
			if metrics.CircuitBrokenAt != nil && (latestCircuitBroken == nil || metrics.CircuitBrokenAt.After(*latestCircuitBroken)) {
				latestCircuitBroken = metrics.CircuitBrokenAt
			}
		}
	}

	aggregated.LastSuccessAt = latestSuccess
	aggregated.LastFailureAt = latestFailure
	aggregated.CircuitBrokenAt = latestCircuitBroken
	aggregated.ConsecutiveFailures = maxConsecutiveFailures

	return aggregated
}

// KeyUsageInfo Key 使用信息（用于排序筛选）
type KeyUsageInfo struct {
	APIKey       string
	KeyMask      string
	RequestCount int64
	LastUsedAt   *time.Time
}

// GetChannelKeyUsageInfo 获取渠道下所有 Key 的使用信息（用于排序筛选）
// 返回的 keys 已按最近使用时间排序
func (m *MetricsManager) GetChannelKeyUsageInfo(baseURL string, apiKeys []string) []KeyUsageInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]KeyUsageInfo, 0, len(apiKeys))

	for _, apiKey := range apiKeys {
		metricsKey := generateMetricsKey(baseURL, apiKey)
		metrics, exists := m.keyMetrics[metricsKey]

		var keyMask string
		var requestCount int64
		var lastUsedAt *time.Time

		if exists {
			keyMask = metrics.KeyMask
			requestCount = metrics.RequestCount
			lastUsedAt = metrics.LastSuccessAt
			if lastUsedAt == nil {
				lastUsedAt = metrics.LastFailureAt
			}
		} else {
			// Key 还没有指标记录，使用默认脱敏
			keyMask = utils.MaskAPIKey(apiKey)
			requestCount = 0
		}

		infos = append(infos, KeyUsageInfo{
			APIKey:       apiKey,
			KeyMask:      keyMask,
			RequestCount: requestCount,
			LastUsedAt:   lastUsedAt,
		})
	}

	// 按最近使用时间排序（最近的在前面）
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].LastUsedAt == nil && infos[j].LastUsedAt == nil {
			return infos[i].RequestCount > infos[j].RequestCount // 都未使用时，按访问量排序
		}
		if infos[i].LastUsedAt == nil {
			return false // i 未使用，排后面
		}
		if infos[j].LastUsedAt == nil {
			return true // j 未使用，i 排前面
		}
		return infos[i].LastUsedAt.After(*infos[j].LastUsedAt)
	})

	return infos
}

// GetChannelKeyUsageInfoMultiURL 获取渠道 Key 使用信息（支持多 URL 聚合）
func (m *MetricsManager) GetChannelKeyUsageInfoMultiURL(baseURLs []string, apiKeys []string) []KeyUsageInfo {
	if len(baseURLs) == 0 {
		return []KeyUsageInfo{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]KeyUsageInfo, 0, len(apiKeys))

	for _, apiKey := range apiKeys {
		var keyMask string
		var requestCount int64
		var lastUsedAt *time.Time
		hasMetrics := false

		// 遍历所有 BaseURL 聚合同一 Key 的指标
		for _, baseURL := range baseURLs {
			metricsKey := generateMetricsKey(baseURL, apiKey)
			if metrics, exists := m.keyMetrics[metricsKey]; exists {
				hasMetrics = true
				if keyMask == "" {
					keyMask = metrics.KeyMask
				}
				requestCount += metrics.RequestCount

				// 取最近的使用时间
				var usedAt *time.Time
				if metrics.LastSuccessAt != nil {
					usedAt = metrics.LastSuccessAt
				}
				if usedAt == nil {
					usedAt = metrics.LastFailureAt
				}
				if usedAt != nil && (lastUsedAt == nil || usedAt.After(*lastUsedAt)) {
					lastUsedAt = usedAt
				}
			}
		}

		if !hasMetrics {
			// Key 还没有指标记录，使用默认脱敏
			keyMask = utils.MaskAPIKey(apiKey)
			requestCount = 0
		}

		infos = append(infos, KeyUsageInfo{
			APIKey:       apiKey,
			KeyMask:      keyMask,
			RequestCount: requestCount,
			LastUsedAt:   lastUsedAt,
		})
	}

	// 按最近使用时间排序（最近的在前面）
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].LastUsedAt == nil && infos[j].LastUsedAt == nil {
			return infos[i].RequestCount > infos[j].RequestCount // 都未使用时，按访问量排序
		}
		if infos[i].LastUsedAt == nil {
			return false // i 未使用，排后面
		}
		if infos[j].LastUsedAt == nil {
			return true // j 未使用，i 排前面
		}
		return infos[i].LastUsedAt.After(*infos[j].LastUsedAt)
	})

	return infos
}

// SelectTopKeys 筛选展示的 Key
// 策略：先取最近使用的 5 个，再从其他 Key 中按访问量补全到 10 个
func SelectTopKeys(infos []KeyUsageInfo, maxDisplay int) []KeyUsageInfo {
	if len(infos) <= maxDisplay {
		return infos
	}

	// 分离：最近使用的和未使用的
	var recentKeys []KeyUsageInfo
	var otherKeys []KeyUsageInfo

	for i, info := range infos {
		if i < 5 {
			recentKeys = append(recentKeys, info)
		} else {
			otherKeys = append(otherKeys, info)
		}
	}

	// 其他 Key 按访问量排序（降序）
	sort.Slice(otherKeys, func(i, j int) bool {
		return otherKeys[i].RequestCount > otherKeys[j].RequestCount
	})

	// 补全到 maxDisplay 个
	result := make([]KeyUsageInfo, 0, maxDisplay)
	result = append(result, recentKeys...)

	needCount := maxDisplay - len(recentKeys)
	if needCount > 0 && len(otherKeys) > 0 {
		if len(otherKeys) > needCount {
			otherKeys = otherKeys[:needCount]
		}
		result = append(result, otherKeys...)
	}

	return result
}

// GetAllKeyMetrics 获取所有 Key 的指标
func (m *MetricsManager) GetAllKeyMetrics() []*KeyMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*KeyMetrics, 0, len(m.keyMetrics))
	for _, metrics := range m.keyMetrics {
		result = append(result, &KeyMetrics{
			MetricsKey:          metrics.MetricsKey,
			BaseURL:             metrics.BaseURL,
			KeyMask:             metrics.KeyMask,
			RequestCount:        metrics.RequestCount,
			SuccessCount:        metrics.SuccessCount,
			FailureCount:        metrics.FailureCount,
			ConsecutiveFailures: metrics.ConsecutiveFailures,
			LastSuccessAt:       metrics.LastSuccessAt,
			LastFailureAt:       metrics.LastFailureAt,
			CircuitBrokenAt:     metrics.CircuitBrokenAt,
		})
	}
	return result
}

// GetTimeWindowStatsForKey 获取指定 Key 在时间窗口内的统计
func (m *MetricsManager) GetTimeWindowStatsForKey(baseURL, apiKey string, duration time.Duration) TimeWindowStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	metrics, exists := m.keyMetrics[metricsKey]
	if !exists {
		return TimeWindowStats{SuccessRate: 100}
	}

	cutoff := time.Now().Add(-duration)
	var requestCount, successCount, failureCount int64

	for _, record := range metrics.requestHistory {
		if record.Timestamp.After(cutoff) {
			requestCount++
			if record.Success {
				successCount++
			} else {
				failureCount++
			}
		}
	}

	successRate := float64(100)
	if requestCount > 0 {
		successRate = float64(successCount) / float64(requestCount) * 100
	}

	return TimeWindowStats{
		RequestCount: requestCount,
		SuccessCount: successCount,
		FailureCount: failureCount,
		SuccessRate:  successRate,
	}
}

// GetAllTimeWindowStatsForKey 获取单个 Key 所有时间窗口的统计
func (m *MetricsManager) GetAllTimeWindowStatsForKey(baseURL, apiKey string) map[string]TimeWindowStats {
	return map[string]TimeWindowStats{
		"15m": m.GetTimeWindowStatsForKey(baseURL, apiKey, 15*time.Minute),
		"1h":  m.GetTimeWindowStatsForKey(baseURL, apiKey, 1*time.Hour),
		"6h":  m.GetTimeWindowStatsForKey(baseURL, apiKey, 6*time.Hour),
		"24h": m.GetTimeWindowStatsForKey(baseURL, apiKey, 24*time.Hour),
	}
}

// ResetKeyFailureState 重置单个 Key 的熔断/失败状态（保留历史统计与总量计数）。
// 用于“恢复熔断”场景：清零连续失败、清空滑动窗口、解除熔断标记。
func (m *MetricsManager) ResetKeyFailureState(baseURL, apiKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	if metrics, exists := m.keyMetrics[metricsKey]; exists {
		metrics.ConsecutiveFailures = 0
		metrics.recentResults = make([]bool, 0, m.windowSize)
		metrics.CircuitBrokenAt = nil
		log.Printf("[Metrics-Reset] Key [%s] (%s) 熔断状态已重置（保留历史统计）", metrics.KeyMask, metrics.BaseURL)
	}
}

// ResetKey 重置单个 Key 的指标
func (m *MetricsManager) ResetKey(baseURL, apiKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	if metrics, exists := m.keyMetrics[metricsKey]; exists {
		// 完全重置所有字段
		metrics.RequestCount = 0
		metrics.SuccessCount = 0
		metrics.FailureCount = 0
		metrics.ConsecutiveFailures = 0
		metrics.ActiveRequests = 0
		metrics.LastSuccessAt = nil
		metrics.LastFailureAt = nil
		metrics.CircuitBrokenAt = nil
		metrics.recentResults = make([]bool, 0, m.windowSize)
		metrics.requestHistory = nil
		if metrics.pendingHistoryIdx != nil {
			for id := range metrics.pendingHistoryIdx {
				delete(metrics.pendingHistoryIdx, id)
			}
		}
		log.Printf("[Metrics-Reset] Key [%s] (%s) 指标已完全重置", metrics.KeyMask, metrics.BaseURL)
	}
}

// ResetAll 重置所有指标
func (m *MetricsManager) ResetAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.keyMetrics = make(map[string]*KeyMetrics)
}

// Stop 停止后台清理任务
func (m *MetricsManager) Stop() {
	close(m.stopCh)
}

// DeleteKeysForChannel 删除指定渠道的所有内存指标
// baseURLs: 渠道的所有 BaseURL（支持多端点 failover）
// apiKeys: 渠道的所有 API Key
// 返回所有可能的 metricsKey 列表（无论内存中是否存在，用于后续清理持久化数据）
func (m *MetricsManager) DeleteKeysForChannel(baseURLs, apiKeys []string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var allKeys []string
	var deletedFromMemory int

	for _, baseURL := range baseURLs {
		for _, apiKey := range apiKeys {
			metricsKey := generateMetricsKey(baseURL, apiKey)
			allKeys = append(allKeys, metricsKey)
			if _, exists := m.keyMetrics[metricsKey]; exists {
				delete(m.keyMetrics, metricsKey)
				deletedFromMemory++
			}
		}
	}

	if deletedFromMemory > 0 {
		log.Printf("[Metrics-Delete] 已删除 %d 个内存指标记录", deletedFromMemory)
	}

	return allKeys
}

// DeleteChannelMetrics 删除渠道的所有指标数据（内存 + 持久化）
// baseURLs: 渠道的所有 BaseURL（支持多端点 failover）
// apiKeys: 渠道的所有 API Key
// apiType: 接口类型（messages/responses/gemini），用于持久化数据过滤
// 返回被删除的持久化记录数
func (m *MetricsManager) DeleteChannelMetrics(baseURLs, apiKeys []string, apiType string) int64 {
	// 1. 删除内存指标，获取 metricsKey 列表
	deletedKeys := m.DeleteKeysForChannel(baseURLs, apiKeys)

	// 2. 删除持久化数据
	if m.store != nil && len(deletedKeys) > 0 {
		deleted, err := m.store.DeleteRecordsByMetricsKeys(deletedKeys, apiType)
		if err != nil {
			log.Printf("[Metrics-Delete] 警告: 删除持久化指标记录失败: %v", err)
			return 0
		}
		if deleted > 0 {
			log.Printf("[Metrics-Delete] 已删除 %d 条 %s 持久化指标记录", deleted, apiType)
		}
		return deleted
	}

	return 0
}

// cleanupCircuitBreakers 后台任务：定期检查并恢复超时的熔断 Key，清理过期指标
func (m *MetricsManager) cleanupCircuitBreakers() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// 每小时清理一次过期 Key
	cleanupTicker := time.NewTicker(1 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ticker.C:
			m.recoverExpiredCircuitBreakers()
		case <-cleanupTicker.C:
			m.cleanupStaleKeys()
		case <-m.stopCh:
			return
		}
	}
}

// recoverExpiredCircuitBreakers 恢复超时的熔断 Key
func (m *MetricsManager) recoverExpiredCircuitBreakers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, metrics := range m.keyMetrics {
		if metrics.CircuitBrokenAt != nil {
			elapsed := now.Sub(*metrics.CircuitBrokenAt)
			if elapsed > m.circuitRecoveryTime {
				// 重置熔断状态
				metrics.ConsecutiveFailures = 0
				metrics.recentResults = make([]bool, 0, m.windowSize)
				metrics.CircuitBrokenAt = nil
				log.Printf("[Metrics-Circuit] Key [%s] (%s) 熔断自动恢复（已超过 %v）", metrics.KeyMask, metrics.BaseURL, m.circuitRecoveryTime)
			}
		}
	}
}

// cleanupStaleKeys 清理过期的 Key 指标（超过 48 小时无活动）
func (m *MetricsManager) cleanupStaleKeys() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	staleThreshold := 48 * time.Hour
	var removed []string

	for key, metrics := range m.keyMetrics {
		// 判断最后活动时间
		var lastActivity time.Time
		if metrics.LastSuccessAt != nil {
			lastActivity = *metrics.LastSuccessAt
		}
		if metrics.LastFailureAt != nil && metrics.LastFailureAt.After(lastActivity) {
			lastActivity = *metrics.LastFailureAt
		}

		// 如果从未有活动或超过阈值，删除
		if lastActivity.IsZero() || now.Sub(lastActivity) > staleThreshold {
			delete(m.keyMetrics, key)
			removed = append(removed, metrics.KeyMask)
		}
	}

	if len(removed) > 0 {
		log.Printf("[Metrics-Cleanup] 清理了 %d 个过期 Key 指标: %v", len(removed), removed)
	}
}

// GetCircuitRecoveryTime 获取熔断恢复时间
func (m *MetricsManager) GetCircuitRecoveryTime() time.Duration {
	return m.circuitRecoveryTime
}

// GetFailureThreshold 获取失败率阈值
func (m *MetricsManager) GetFailureThreshold() float64 {
	return m.failureThreshold
}

// GetWindowSize 获取滑动窗口大小
func (m *MetricsManager) GetWindowSize() int {
	return m.windowSize
}

// ============ 兼容旧 API 的方法（基于 channelIndex，需要调用方提供 baseURL 和 keys）============

// MetricsResponse API 响应结构
type MetricsResponse struct {
	ChannelIndex        int                        `json:"channelIndex"`
	RequestCount        int64                      `json:"requestCount"`
	SuccessCount        int64                      `json:"successCount"`
	FailureCount        int64                      `json:"failureCount"`
	SuccessRate         float64                    `json:"successRate"`
	ErrorRate           float64                    `json:"errorRate"`
	ConsecutiveFailures int64                      `json:"consecutiveFailures"`
	ActiveRequests      int64                      `json:"activeRequests"` // 进行中请求数
	Latency             int64                      `json:"latency"`
	LastSuccessAt       *string                    `json:"lastSuccessAt,omitempty"`
	LastFailureAt       *string                    `json:"lastFailureAt,omitempty"`
	CircuitBrokenAt     *string                    `json:"circuitBrokenAt,omitempty"`
	TimeWindows         map[string]TimeWindowStats `json:"timeWindows,omitempty"`
	KeyMetrics          []*KeyMetricsResponse      `json:"keyMetrics,omitempty"` // 各 Key 的详细指标
}

// KeyMetricsResponse 单个 Key 的 API 响应
type KeyMetricsResponse struct {
	KeyMask             string  `json:"keyMask"`
	RequestCount        int64   `json:"requestCount"`
	SuccessCount        int64   `json:"successCount"`
	FailureCount        int64   `json:"failureCount"`
	SuccessRate         float64 `json:"successRate"`
	ConsecutiveFailures int64   `json:"consecutiveFailures"`
	CircuitBroken       bool    `json:"circuitBroken"`
}

// ToResponseMultiURL 转换为 API 响应格式（支持多 BaseURL 聚合）
// baseURLs: 渠道配置的所有 BaseURL（用于多端点 failover 场景）
// historicalKeys: 历史 API Key（用于统计聚合，只计入总数不显示在 KeyMetrics 中）
func (m *MetricsManager) ToResponseMultiURL(channelIndex int, baseURLs []string, activeKeys []string, latency int64, historicalKeys ...[]string) *MetricsResponse {
	// 如果没有配置 BaseURL，返回空响应
	if len(baseURLs) == 0 {
		return &MetricsResponse{
			ChannelIndex: channelIndex,
			Latency:      latency,
			SuccessRate:  100,
			ErrorRate:    0,
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	resp := &MetricsResponse{
		ChannelIndex: channelIndex,
		Latency:      latency,
	}

	if len(activeKeys) == 0 {
		resp.SuccessRate = 100
		resp.ErrorRate = 0
		return resp
	}

	// 用于按 API Key 聚合的临时结构
	type keyAggregation struct {
		keyMask             string
		requestCount        int64
		successCount        int64
		failureCount        int64
		consecutiveFailures int64
		circuitBroken       bool
	}
	keyAggMap := make(map[string]*keyAggregation) // key: apiKey

	var latestSuccess, latestFailure, latestCircuitBroken *time.Time
	var totalResults []bool
	var maxConsecutiveFailures int64

	// 遍历所有 BaseURL 和 Key 的组合
	for _, baseURL := range baseURLs {
		for _, apiKey := range activeKeys {
			metricsKey := generateMetricsKey(baseURL, apiKey)
			if metrics, exists := m.keyMetrics[metricsKey]; exists {
				resp.RequestCount += metrics.RequestCount
				resp.SuccessCount += metrics.SuccessCount
				resp.FailureCount += metrics.FailureCount
				resp.ActiveRequests += metrics.ActiveRequests
				if metrics.ConsecutiveFailures > maxConsecutiveFailures {
					maxConsecutiveFailures = metrics.ConsecutiveFailures
				}
				totalResults = append(totalResults, metrics.recentResults...)

				// 取最新的时间戳
				if metrics.LastSuccessAt != nil && (latestSuccess == nil || metrics.LastSuccessAt.After(*latestSuccess)) {
					latestSuccess = metrics.LastSuccessAt
				}
				if metrics.LastFailureAt != nil && (latestFailure == nil || metrics.LastFailureAt.After(*latestFailure)) {
					latestFailure = metrics.LastFailureAt
				}
				if metrics.CircuitBrokenAt != nil && (latestCircuitBroken == nil || metrics.CircuitBrokenAt.After(*latestCircuitBroken)) {
					latestCircuitBroken = metrics.CircuitBrokenAt
				}

				// 按 API Key 聚合（同一 Key 在不同 URL 的指标合并）
				if agg, ok := keyAggMap[apiKey]; ok {
					agg.requestCount += metrics.RequestCount
					agg.successCount += metrics.SuccessCount
					agg.failureCount += metrics.FailureCount
					if metrics.ConsecutiveFailures > agg.consecutiveFailures {
						agg.consecutiveFailures = metrics.ConsecutiveFailures
					}
					if metrics.CircuitBrokenAt != nil {
						agg.circuitBroken = true
					}
				} else {
					keyAggMap[apiKey] = &keyAggregation{
						keyMask:             metrics.KeyMask,
						requestCount:        metrics.RequestCount,
						successCount:        metrics.SuccessCount,
						failureCount:        metrics.FailureCount,
						consecutiveFailures: metrics.ConsecutiveFailures,
						circuitBroken:       metrics.CircuitBrokenAt != nil,
					}
				}
			}
		}
	}

	// 聚合历史 Key 的指标（只计入总数，不显示在 KeyMetrics 中）
	if len(historicalKeys) > 0 && len(historicalKeys[0]) > 0 {
		for _, baseURL := range baseURLs {
			for _, apiKey := range historicalKeys[0] {
				metricsKey := generateMetricsKey(baseURL, apiKey)
				if metrics, exists := m.keyMetrics[metricsKey]; exists {
					resp.RequestCount += metrics.RequestCount
					resp.SuccessCount += metrics.SuccessCount
					resp.FailureCount += metrics.FailureCount
					// 历史 Key 不计入 totalResults（不影响实时失败率计算）
					// 历史 Key 不计入 maxConsecutiveFailures（不影响熔断判断）
				}
			}
		}
	}

	// 构建按 Key 聚合后的响应（保持 activeKeys 顺序）
	var keyResponses []*KeyMetricsResponse
	for _, apiKey := range activeKeys {
		if agg, ok := keyAggMap[apiKey]; ok {
			keySuccessRate := float64(100)
			if agg.requestCount > 0 {
				keySuccessRate = float64(agg.successCount) / float64(agg.requestCount) * 100
			}
			keyResponses = append(keyResponses, &KeyMetricsResponse{
				KeyMask:             agg.keyMask,
				RequestCount:        agg.requestCount,
				SuccessCount:        agg.successCount,
				FailureCount:        agg.failureCount,
				SuccessRate:         keySuccessRate,
				ConsecutiveFailures: agg.consecutiveFailures,
				CircuitBroken:       agg.circuitBroken,
			})
		}
	}

	// 计算聚合失败率
	resp.ConsecutiveFailures = maxConsecutiveFailures

	if len(totalResults) > 0 {
		failures := 0
		for _, success := range totalResults {
			if !success {
				failures++
			}
		}
		failureRate := float64(failures) / float64(len(totalResults))
		resp.SuccessRate = (1 - failureRate) * 100
		resp.ErrorRate = failureRate * 100
	} else {
		resp.SuccessRate = 100
		resp.ErrorRate = 0
	}

	if latestSuccess != nil {
		t := latestSuccess.Format(time.RFC3339)
		resp.LastSuccessAt = &t
	}
	if latestFailure != nil {
		t := latestFailure.Format(time.RFC3339)
		resp.LastFailureAt = &t
	}
	if latestCircuitBroken != nil {
		t := latestCircuitBroken.Format(time.RFC3339)
		resp.CircuitBrokenAt = &t
	}

	resp.KeyMetrics = keyResponses

	// 计算聚合的时间窗口统计（多 URL 版本）
	resp.TimeWindows = m.calculateAggregatedTimeWindowsMultiURL(baseURLs, activeKeys)

	return resp
}

// ToResponse 转换为 API 响应格式（需要提供 baseURL 和 activeKeys）
func (m *MetricsManager) ToResponse(channelIndex int, baseURL string, activeKeys []string, latency int64) *MetricsResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp := &MetricsResponse{
		ChannelIndex: channelIndex,
		Latency:      latency,
	}

	if len(activeKeys) == 0 {
		resp.SuccessRate = 100
		resp.ErrorRate = 0
		return resp
	}

	var keyResponses []*KeyMetricsResponse
	var latestSuccess, latestFailure, latestCircuitBroken *time.Time
	var totalResults []bool
	var maxConsecutiveFailures int64

	for _, apiKey := range activeKeys {
		metricsKey := generateMetricsKey(baseURL, apiKey)
		if metrics, exists := m.keyMetrics[metricsKey]; exists {
			resp.RequestCount += metrics.RequestCount
			resp.SuccessCount += metrics.SuccessCount
			resp.FailureCount += metrics.FailureCount
			resp.ActiveRequests += metrics.ActiveRequests
			if metrics.ConsecutiveFailures > maxConsecutiveFailures {
				maxConsecutiveFailures = metrics.ConsecutiveFailures
			}
			totalResults = append(totalResults, metrics.recentResults...)

			// 取最新的时间戳
			if metrics.LastSuccessAt != nil && (latestSuccess == nil || metrics.LastSuccessAt.After(*latestSuccess)) {
				latestSuccess = metrics.LastSuccessAt
			}
			if metrics.LastFailureAt != nil && (latestFailure == nil || metrics.LastFailureAt.After(*latestFailure)) {
				latestFailure = metrics.LastFailureAt
			}
			if metrics.CircuitBrokenAt != nil && (latestCircuitBroken == nil || metrics.CircuitBrokenAt.After(*latestCircuitBroken)) {
				latestCircuitBroken = metrics.CircuitBrokenAt
			}

			// 单个 Key 的指标
			keySuccessRate := float64(100)
			if metrics.RequestCount > 0 {
				keySuccessRate = float64(metrics.SuccessCount) / float64(metrics.RequestCount) * 100
			}
			keyResponses = append(keyResponses, &KeyMetricsResponse{
				KeyMask:             metrics.KeyMask,
				RequestCount:        metrics.RequestCount,
				SuccessCount:        metrics.SuccessCount,
				FailureCount:        metrics.FailureCount,
				SuccessRate:         keySuccessRate,
				ConsecutiveFailures: metrics.ConsecutiveFailures,
				CircuitBroken:       metrics.CircuitBrokenAt != nil,
			})
		}
	}

	// 计算聚合失败率
	resp.ConsecutiveFailures = maxConsecutiveFailures

	if len(totalResults) > 0 {
		failures := 0
		for _, success := range totalResults {
			if !success {
				failures++
			}
		}
		failureRate := float64(failures) / float64(len(totalResults))
		resp.SuccessRate = (1 - failureRate) * 100
		resp.ErrorRate = failureRate * 100
	} else {
		resp.SuccessRate = 100
		resp.ErrorRate = 0
	}

	if latestSuccess != nil {
		t := latestSuccess.Format(time.RFC3339)
		resp.LastSuccessAt = &t
	}
	if latestFailure != nil {
		t := latestFailure.Format(time.RFC3339)
		resp.LastFailureAt = &t
	}
	if latestCircuitBroken != nil {
		t := latestCircuitBroken.Format(time.RFC3339)
		resp.CircuitBrokenAt = &t
	}

	resp.KeyMetrics = keyResponses

	// 计算聚合的时间窗口统计
	resp.TimeWindows = m.calculateAggregatedTimeWindowsInternal(baseURL, activeKeys)

	return resp
}

// calculateAggregatedTimeWindowsInternal 计算聚合的时间窗口统计（内部方法，调用前需持有锁）
func (m *MetricsManager) calculateAggregatedTimeWindowsInternal(baseURL string, activeKeys []string) map[string]TimeWindowStats {
	windows := map[string]time.Duration{
		"15m": 15 * time.Minute,
		"1h":  1 * time.Hour,
		"6h":  6 * time.Hour,
		"24h": 24 * time.Hour,
	}

	result := make(map[string]TimeWindowStats)
	now := time.Now()

	for label, duration := range windows {
		cutoff := now.Add(-duration)
		var requestCount, successCount, failureCount int64
		var inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int64

		for _, apiKey := range activeKeys {
			metricsKey := generateMetricsKey(baseURL, apiKey)
			if metrics, exists := m.keyMetrics[metricsKey]; exists {
				for _, record := range metrics.requestHistory {
					if record.Timestamp.After(cutoff) {
						requestCount++
						if record.Success {
							successCount++
						} else {
							failureCount++
						}
						inputTokens += record.InputTokens
						outputTokens += record.OutputTokens
						cacheCreationTokens += record.CacheCreationInputTokens
						cacheReadTokens += record.CacheReadInputTokens
					}
				}
			}
		}

		successRate := float64(100)
		if requestCount > 0 {
			successRate = float64(successCount) / float64(requestCount) * 100
		}

		cacheHitRate := float64(0)
		denom := cacheReadTokens + inputTokens
		if denom > 0 {
			cacheHitRate = float64(cacheReadTokens) / float64(denom) * 100
		}

		result[label] = TimeWindowStats{
			RequestCount:        requestCount,
			SuccessCount:        successCount,
			FailureCount:        failureCount,
			SuccessRate:         successRate,
			InputTokens:         inputTokens,
			OutputTokens:        outputTokens,
			CacheCreationTokens: cacheCreationTokens,
			CacheReadTokens:     cacheReadTokens,
			CacheHitRate:        cacheHitRate,
		}
	}

	return result
}

// calculateAggregatedTimeWindowsMultiURL 计算聚合的时间窗口统计（多 URL 版本，内部方法，调用前需持有锁）
func (m *MetricsManager) calculateAggregatedTimeWindowsMultiURL(baseURLs []string, activeKeys []string) map[string]TimeWindowStats {
	windows := map[string]time.Duration{
		"15m": 15 * time.Minute,
		"1h":  1 * time.Hour,
		"6h":  6 * time.Hour,
		"24h": 24 * time.Hour,
	}

	result := make(map[string]TimeWindowStats)
	now := time.Now()

	for label, duration := range windows {
		cutoff := now.Add(-duration)
		var requestCount, successCount, failureCount int64
		var inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int64

		// 遍历所有 BaseURL 和 Key 的组合
		for _, baseURL := range baseURLs {
			for _, apiKey := range activeKeys {
				metricsKey := generateMetricsKey(baseURL, apiKey)
				if metrics, exists := m.keyMetrics[metricsKey]; exists {
					for _, record := range metrics.requestHistory {
						if record.Timestamp.After(cutoff) {
							requestCount++
							if record.Success {
								successCount++
							} else {
								failureCount++
							}
							inputTokens += record.InputTokens
							outputTokens += record.OutputTokens
							cacheCreationTokens += record.CacheCreationInputTokens
							cacheReadTokens += record.CacheReadInputTokens
						}
					}
				}
			}
		}

		successRate := float64(100)
		if requestCount > 0 {
			successRate = float64(successCount) / float64(requestCount) * 100
		}

		cacheHitRate := float64(0)
		denom := cacheReadTokens + inputTokens
		if denom > 0 {
			cacheHitRate = float64(cacheReadTokens) / float64(denom) * 100
		}

		result[label] = TimeWindowStats{
			RequestCount:        requestCount,
			SuccessCount:        successCount,
			FailureCount:        failureCount,
			SuccessRate:         successRate,
			InputTokens:         inputTokens,
			OutputTokens:        outputTokens,
			CacheCreationTokens: cacheCreationTokens,
			CacheReadTokens:     cacheReadTokens,
			CacheHitRate:        cacheHitRate,
		}
	}

	return result
}

// ============ 废弃的旧方法（保留签名以便编译，但标记为废弃）============

// Deprecated: 使用 IsChannelHealthyWithKeys 代替
// IsChannelHealthy 判断渠道是否健康（旧方法，不再使用 channelIndex）
// 此方法保留是为了兼容，但始终返回 true，调用方应迁移到新方法
func (m *MetricsManager) IsChannelHealthy(channelIndex int) bool {
	log.Printf("[Metrics-Deprecated] 警告: 调用了废弃的 IsChannelHealthy(channelIndex=%d)，请迁移到 IsChannelHealthyWithKeys", channelIndex)
	return true // 默认健康，避免影响现有逻辑
}

// Deprecated: 使用 CalculateChannelFailureRate 代替
func (m *MetricsManager) CalculateFailureRate(channelIndex int) float64 {
	return 0
}

// Deprecated: 使用 CalculateChannelFailureRate 代替
func (m *MetricsManager) CalculateSuccessRate(channelIndex int) float64 {
	return 1
}

// Deprecated: 使用 ResetKey 代替
func (m *MetricsManager) Reset(channelIndex int) {
	log.Printf("[Metrics-Deprecated] 警告: 调用了废弃的 Reset(channelIndex=%d)，请迁移到 ResetKey", channelIndex)
}

// Deprecated: 使用 GetChannelAggregatedMetrics 代替
func (m *MetricsManager) GetMetrics(channelIndex int) *ChannelMetrics {
	return nil
}

// Deprecated: 使用 GetAllKeyMetrics 代替
func (m *MetricsManager) GetAllMetrics() []*ChannelMetrics {
	return nil
}

// Deprecated: 使用 GetTimeWindowStatsForKey 代替
func (m *MetricsManager) GetTimeWindowStats(channelIndex int, duration time.Duration) TimeWindowStats {
	return TimeWindowStats{SuccessRate: 100}
}

// Deprecated: 使用 GetAllTimeWindowStatsForKey 代替
func (m *MetricsManager) GetAllTimeWindowStats(channelIndex int) map[string]TimeWindowStats {
	return map[string]TimeWindowStats{
		"15m": {SuccessRate: 100},
		"1h":  {SuccessRate: 100},
		"6h":  {SuccessRate: 100},
		"24h": {SuccessRate: 100},
	}
}

// Deprecated: 使用新的 ShouldSuspendKey 代替
func (m *MetricsManager) ShouldSuspend(channelIndex int) bool {
	return false
}

// ShouldSuspendKey 判断单个 Key 是否应该熔断
func (m *MetricsManager) ShouldSuspendKey(baseURL, apiKey string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metricsKey := generateMetricsKey(baseURL, apiKey)
	metrics, exists := m.keyMetrics[metricsKey]
	if !exists {
		return false
	}

	// 最小请求数保护：至少 max(3, windowSize/2) 次请求才判断
	minRequests := max(3, m.windowSize/2)
	if len(metrics.recentResults) < minRequests {
		return false
	}

	return m.calculateKeyFailureRateInternal(metrics) >= m.failureThreshold
}

// ============ 历史数据查询方法（用于图表可视化）============

// HistoryDataPoint 历史数据点（用于时间序列图表）
type HistoryDataPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	RequestCount int64     `json:"requestCount"`
	SuccessCount int64     `json:"successCount"`
	FailureCount int64     `json:"failureCount"`
	SuccessRate  float64   `json:"successRate"`
}

// KeyHistoryDataPoint Key 级别历史数据点（包含 Token 和 Cache 数据）
type KeyHistoryDataPoint struct {
	Timestamp                time.Time `json:"timestamp"`
	RequestCount             int64     `json:"requestCount"`
	SuccessCount             int64     `json:"successCount"`
	FailureCount             int64     `json:"failureCount"`
	SuccessRate              float64   `json:"successRate"`
	InputTokens              int64     `json:"inputTokens"`
	OutputTokens             int64     `json:"outputTokens"`
	CacheCreationInputTokens int64     `json:"cacheCreationTokens"`
	CacheReadInputTokens     int64     `json:"cacheReadTokens"`
}

// GetHistoricalStats 获取历史统计数据（按时间间隔聚合）
// duration: 查询时间范围 (如 1h, 6h, 24h)
// interval: 聚合间隔 (如 5m, 15m, 1h)
func (m *MetricsManager) GetHistoricalStats(baseURL string, activeKeys []string, duration, interval time.Duration) []HistoryDataPoint {
	// 参数验证
	if interval <= 0 || duration <= 0 {
		return []HistoryDataPoint{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	// 时间对齐到 interval 边界
	startTime := now.Add(-duration).Truncate(interval)
	// endTime 延伸一个 interval，确保当前时间段的请求也被包含
	endTime := now.Truncate(interval).Add(interval)

	// 计算需要多少个数据点（+1 用于包含延伸的当前时间段）
	numPoints := int(duration / interval)
	if numPoints <= 0 {
		numPoints = 1
	}
	numPoints++ // 额外的一个桶用于当前时间段

	// 使用 map 按时间分桶，优化性能：O(records) 而不是 O(records * numPoints)
	buckets := make(map[int64]*bucketData)
	for i := 0; i < numPoints; i++ {
		buckets[int64(i)] = &bucketData{}
	}

	// 收集所有相关 Key 的请求历史并放入对应桶
	for _, apiKey := range activeKeys {
		metricsKey := generateMetricsKey(baseURL, apiKey)
		if metrics, exists := m.keyMetrics[metricsKey]; exists {
			for _, record := range metrics.requestHistory {
				// 使用 [startTime, endTime) 的区间，避免 endTime 处 offset 越界
				if !record.Timestamp.Before(startTime) && record.Timestamp.Before(endTime) {
					// 计算记录应该属于哪个桶
					offset := int64(record.Timestamp.Sub(startTime) / interval)
					if offset >= 0 && offset < int64(numPoints) {
						b := buckets[offset]
						b.requestCount++
						if record.Success {
							b.successCount++
						} else {
							b.failureCount++
						}
					}
				}
			}
		}
	}

	// 构建结果
	result := make([]HistoryDataPoint, numPoints)
	for i := 0; i < numPoints; i++ {
		b := buckets[int64(i)]
		// 空桶成功率默认为 0，避免误导（100% 暗示完美成功）
		successRate := float64(0)
		if b.requestCount > 0 {
			successRate = float64(b.successCount) / float64(b.requestCount) * 100
		}
		result[i] = HistoryDataPoint{
			Timestamp:    startTime.Add(time.Duration(i) * interval),
			RequestCount: b.requestCount,
			SuccessCount: b.successCount,
			FailureCount: b.failureCount,
			SuccessRate:  successRate,
		}
	}

	return result
}

// GetHistoricalStatsMultiURL 获取多 URL 聚合的历史统计数据
func (m *MetricsManager) GetHistoricalStatsMultiURL(baseURLs []string, activeKeys []string, duration, interval time.Duration) []HistoryDataPoint {
	// 参数验证
	if interval <= 0 || duration <= 0 || len(baseURLs) == 0 {
		return []HistoryDataPoint{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	// 时间对齐到 interval 边界
	startTime := now.Add(-duration).Truncate(interval)
	// endTime 延伸一个 interval，确保当前时间段的请求也被包含
	endTime := now.Truncate(interval).Add(interval)

	// 计算需要多少个数据点（+1 用于包含延伸的当前时间段）
	numPoints := int(duration / interval)
	if numPoints <= 0 {
		numPoints = 1
	}
	numPoints++ // 额外的一个桶用于当前时间段

	// 使用 map 按时间分桶，优化性能：O(records) 而不是 O(records * numPoints)
	buckets := make(map[int64]*bucketData)
	for i := 0; i < numPoints; i++ {
		buckets[int64(i)] = &bucketData{}
	}

	// 收集所有 BaseURL 和 Key 组合的请求历史并放入对应桶
	for _, baseURL := range baseURLs {
		for _, apiKey := range activeKeys {
			metricsKey := generateMetricsKey(baseURL, apiKey)
			if metrics, exists := m.keyMetrics[metricsKey]; exists {
				for _, record := range metrics.requestHistory {
					// 使用 [startTime, endTime) 的区间，避免 endTime 处 offset 越界
					if !record.Timestamp.Before(startTime) && record.Timestamp.Before(endTime) {
						// 计算记录应该属于哪个桶
						offset := int64(record.Timestamp.Sub(startTime) / interval)
						if offset >= 0 && offset < int64(numPoints) {
							b := buckets[offset]
							b.requestCount++
							if record.Success {
								b.successCount++
							} else {
								b.failureCount++
							}
						}
					}
				}
			}
		}
	}

	// 构建结果
	result := make([]HistoryDataPoint, numPoints)
	for i := 0; i < numPoints; i++ {
		b := buckets[int64(i)]
		// 空桶成功率默认为 0，避免误导（100% 暗示完美成功）
		successRate := float64(0)
		if b.requestCount > 0 {
			successRate = float64(b.successCount) / float64(b.requestCount) * 100
		}
		result[i] = HistoryDataPoint{
			Timestamp:    startTime.Add(time.Duration(i) * interval),
			RequestCount: b.requestCount,
			SuccessCount: b.successCount,
			FailureCount: b.failureCount,
			SuccessRate:  successRate,
		}
	}

	return result
}

// bucketData 用于时间分桶的辅助结构
type bucketData struct {
	requestCount int64
	successCount int64
	failureCount int64
}

func (m *MetricsManager) GetAllKeysHistoricalStats(duration, interval time.Duration) []HistoryDataPoint {
	// 参数验证
	if interval <= 0 || duration <= 0 {
		return []HistoryDataPoint{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	// 时间对齐到 interval 边界
	startTime := now.Add(-duration).Truncate(interval)
	// endTime 延伸一个 interval，确保当前时间段的请求也被包含
	endTime := now.Truncate(interval).Add(interval)

	numPoints := int(duration / interval)
	if numPoints <= 0 {
		numPoints = 1
	}
	numPoints++ // 额外的一个桶用于当前时间段

	// 使用 map 按时间分桶，优化性能
	buckets := make(map[int64]*bucketData)
	for i := 0; i < numPoints; i++ {
		buckets[int64(i)] = &bucketData{}
	}

	// 收集所有 Key 的请求历史并放入对应桶
	for _, metrics := range m.keyMetrics {
		for _, record := range metrics.requestHistory {
			// 使用 [startTime, endTime) 的区间，避免 endTime 处 offset 越界
			if !record.Timestamp.Before(startTime) && record.Timestamp.Before(endTime) {
				offset := int64(record.Timestamp.Sub(startTime) / interval)
				if offset >= 0 && offset < int64(numPoints) {
					b := buckets[offset]
					b.requestCount++
					if record.Success {
						b.successCount++
					} else {
						b.failureCount++
					}
				}
			}
		}
	}

	// 构建结果
	result := make([]HistoryDataPoint, numPoints)
	for i := 0; i < numPoints; i++ {
		b := buckets[int64(i)]
		// 空桶成功率默认为 0，避免误导（100% 暗示完美成功）
		successRate := float64(0)
		if b.requestCount > 0 {
			successRate = float64(b.successCount) / float64(b.requestCount) * 100
		}
		result[i] = HistoryDataPoint{
			Timestamp:    startTime.Add(time.Duration(i) * interval),
			RequestCount: b.requestCount,
			SuccessCount: b.successCount,
			FailureCount: b.failureCount,
			SuccessRate:  successRate,
		}
	}

	return result
}

// GetKeyHistoricalStats 获取单个 Key 的历史统计数据（包含 Token 和 Cache 数据）
func (m *MetricsManager) GetKeyHistoricalStats(baseURL, apiKey string, duration, interval time.Duration) []KeyHistoryDataPoint {
	// 参数验证
	if interval <= 0 || duration <= 0 {
		return []KeyHistoryDataPoint{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	// 时间对齐到 interval 边界
	startTime := now.Add(-duration).Truncate(interval)
	// endTime 延伸一个 interval，确保当前时间段的请求也被包含
	endTime := now.Truncate(interval).Add(interval)

	numPoints := int(duration / interval)
	if numPoints <= 0 {
		numPoints = 1
	}
	numPoints++ // 额外的一个桶用于当前时间段

	// 使用 map 按时间分桶
	buckets := make(map[int64]*keyBucketData)
	for i := 0; i < numPoints; i++ {
		buckets[int64(i)] = &keyBucketData{}
	}

	// 获取 Key 的指标
	metricsKey := generateMetricsKey(baseURL, apiKey)
	metrics, exists := m.keyMetrics[metricsKey]
	if !exists {
		// Key 不存在，返回空数据点
		result := make([]KeyHistoryDataPoint, numPoints)
		for i := 0; i < numPoints; i++ {
			result[i] = KeyHistoryDataPoint{
				Timestamp: startTime.Add(time.Duration(i+1) * interval),
			}
		}
		return result
	}

	// 收集该 Key 的请求历史并放入对应桶
	for _, record := range metrics.requestHistory {
		// 使用 Before(endTime) 排除恰好落在 endTime 的记录，避免 offset 越界
		if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
			offset := int64(record.Timestamp.Sub(startTime) / interval)
			if offset >= 0 && offset < int64(numPoints) {
				b := buckets[offset]
				b.requestCount++
				if record.Success {
					b.successCount++
				} else {
					b.failureCount++
				}
				// 累加 Token 数据
				b.inputTokens += record.InputTokens
				b.outputTokens += record.OutputTokens
				b.cacheCreationTokens += record.CacheCreationInputTokens
				b.cacheReadTokens += record.CacheReadInputTokens
			}
		}
	}

	// 构建结果
	result := make([]KeyHistoryDataPoint, numPoints)
	for i := 0; i < numPoints; i++ {
		b := buckets[int64(i)]
		// 空桶成功率默认为 0，避免误导（100% 暗示完美成功）
		successRate := float64(0)
		if b.requestCount > 0 {
			successRate = float64(b.successCount) / float64(b.requestCount) * 100
		}
		result[i] = KeyHistoryDataPoint{
			Timestamp:                startTime.Add(time.Duration(i+1) * interval),
			RequestCount:             b.requestCount,
			SuccessCount:             b.successCount,
			FailureCount:             b.failureCount,
			SuccessRate:              successRate,
			InputTokens:              b.inputTokens,
			OutputTokens:             b.outputTokens,
			CacheCreationInputTokens: b.cacheCreationTokens,
			CacheReadInputTokens:     b.cacheReadTokens,
		}
	}

	return result
}

// GetKeyHistoricalStatsMultiURL 获取单个 Key 的多 URL 聚合历史统计
func (m *MetricsManager) GetKeyHistoricalStatsMultiURL(baseURLs []string, apiKey string, duration, interval time.Duration) []KeyHistoryDataPoint {
	// 参数验证
	if interval <= 0 || duration <= 0 || len(baseURLs) == 0 {
		return []KeyHistoryDataPoint{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	// 时间对齐到 interval 边界
	startTime := now.Add(-duration).Truncate(interval)
	// endTime 延伸一个 interval，确保当前时间段的请求也被包含
	endTime := now.Truncate(interval).Add(interval)

	numPoints := int(duration / interval)
	if numPoints <= 0 {
		numPoints = 1
	}
	numPoints++ // 额外的一个桶用于当前时间段

	// 使用 map 按时间分桶
	buckets := make(map[int64]*keyBucketData)
	for i := 0; i < numPoints; i++ {
		buckets[int64(i)] = &keyBucketData{}
	}

	// 遍历所有 BaseURL 聚合同一 Key 的历史数据
	hasData := false
	for _, baseURL := range baseURLs {
		metricsKey := generateMetricsKey(baseURL, apiKey)
		metrics, exists := m.keyMetrics[metricsKey]
		if !exists {
			continue
		}
		hasData = true

		// 收集该 URL+Key 组合的请求历史并放入对应桶
		for _, record := range metrics.requestHistory {
			// 使用 Before(endTime) 排除恰好落在 endTime 的记录，避免 offset 越界
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				offset := int64(record.Timestamp.Sub(startTime) / interval)
				if offset >= 0 && offset < int64(numPoints) {
					b := buckets[offset]
					b.requestCount++
					if record.Success {
						b.successCount++
					} else {
						b.failureCount++
					}
					// 累加 Token 数据
					b.inputTokens += record.InputTokens
					b.outputTokens += record.OutputTokens
					b.cacheCreationTokens += record.CacheCreationInputTokens
					b.cacheReadTokens += record.CacheReadInputTokens
				}
			}
		}
	}

	// 如果没有任何数据，返回空数据点
	if !hasData {
		result := make([]KeyHistoryDataPoint, numPoints)
		for i := 0; i < numPoints; i++ {
			result[i] = KeyHistoryDataPoint{
				Timestamp: startTime.Add(time.Duration(i+1) * interval),
			}
		}
		return result
	}

	// 构建结果
	result := make([]KeyHistoryDataPoint, numPoints)
	for i := 0; i < numPoints; i++ {
		b := buckets[int64(i)]
		// 空桶成功率默认为 0，避免误导（100% 暗示完美成功）
		successRate := float64(0)
		if b.requestCount > 0 {
			successRate = float64(b.successCount) / float64(b.requestCount) * 100
		}
		result[i] = KeyHistoryDataPoint{
			Timestamp:                startTime.Add(time.Duration(i+1) * interval),
			RequestCount:             b.requestCount,
			SuccessCount:             b.successCount,
			FailureCount:             b.failureCount,
			SuccessRate:              successRate,
			InputTokens:              b.inputTokens,
			OutputTokens:             b.outputTokens,
			CacheCreationInputTokens: b.cacheCreationTokens,
			CacheReadInputTokens:     b.cacheReadTokens,
		}
	}

	return result
}

// keyBucketData Key 级别时间分桶的辅助结构（包含 Token 数据）
type keyBucketData struct {
	requestCount        int64
	successCount        int64
	failureCount        int64
	inputTokens         int64
	outputTokens        int64
	cacheCreationTokens int64
	cacheReadTokens     int64
}

// ============ 全局统计数据结构和方法（用于全局流量统计图表）============

// GlobalHistoryDataPoint 全局历史数据点（含 Token 数据）
type GlobalHistoryDataPoint struct {
	Timestamp           time.Time `json:"timestamp"`
	RequestCount        int64     `json:"requestCount"`
	SuccessCount        int64     `json:"successCount"`
	FailureCount        int64     `json:"failureCount"`
	SuccessRate         float64   `json:"successRate"`
	InputTokens         int64     `json:"inputTokens"`
	OutputTokens        int64     `json:"outputTokens"`
	CacheCreationTokens int64     `json:"cacheCreationTokens"`
	CacheReadTokens     int64     `json:"cacheReadTokens"`
}

// GlobalStatsSummary 全局统计汇总
type GlobalStatsSummary struct {
	TotalRequests            int64   `json:"totalRequests"`
	TotalSuccess             int64   `json:"totalSuccess"`
	TotalFailure             int64   `json:"totalFailure"`
	TotalInputTokens         int64   `json:"totalInputTokens"`
	TotalOutputTokens        int64   `json:"totalOutputTokens"`
	TotalCacheCreationTokens int64   `json:"totalCacheCreationTokens"`
	TotalCacheReadTokens     int64   `json:"totalCacheReadTokens"`
	AvgSuccessRate           float64 `json:"avgSuccessRate"`
	Duration                 string  `json:"duration"`
}

// GlobalStatsHistoryResponse 全局统计响应
type GlobalStatsHistoryResponse struct {
	DataPoints []GlobalHistoryDataPoint `json:"dataPoints"`
	Summary    GlobalStatsSummary       `json:"summary"`
}

// GetGlobalHistoricalStatsWithTokens 获取全局历史统计（包含 Token 数据）
// 聚合所有 Key 的数据，按时间间隔分桶
func (m *MetricsManager) GetGlobalHistoricalStatsWithTokens(duration, interval time.Duration) GlobalStatsHistoryResponse {
	// 参数验证
	if interval <= 0 || duration <= 0 {
		return GlobalStatsHistoryResponse{
			DataPoints: []GlobalHistoryDataPoint{},
			Summary:    GlobalStatsSummary{Duration: duration.String()},
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	// 时间对齐到 interval 边界
	startTime := now.Add(-duration).Truncate(interval)
	// endTime 延伸一个 interval，确保当前时间段的请求也被包含
	endTime := now.Truncate(interval).Add(interval)

	numPoints := int(duration / interval)
	if numPoints <= 0 {
		numPoints = 1
	}
	numPoints++ // 额外的一个桶用于当前时间段

	// 使用 map 按时间分桶
	buckets := make(map[int64]*globalBucketData)
	for i := 0; i < numPoints; i++ {
		buckets[int64(i)] = &globalBucketData{}
	}

	// 汇总统计
	var totalRequests, totalSuccess, totalFailure int64
	var totalInputTokens, totalOutputTokens, totalCacheCreation, totalCacheRead int64

	// 遍历所有 Key 的请求历史
	for _, metrics := range m.keyMetrics {
		for _, record := range metrics.requestHistory {
			// 使用 Before(endTime) 排除恰好落在 endTime 的记录，避免 offset 越界
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				offset := int64(record.Timestamp.Sub(startTime) / interval)
				if offset >= 0 && offset < int64(numPoints) {
					b := buckets[offset]
					b.requestCount++
					if record.Success {
						b.successCount++
					} else {
						b.failureCount++
					}
					b.inputTokens += record.InputTokens
					b.outputTokens += record.OutputTokens
					b.cacheCreationTokens += record.CacheCreationInputTokens
					b.cacheReadTokens += record.CacheReadInputTokens

					// 累加汇总
					totalRequests++
					if record.Success {
						totalSuccess++
					} else {
						totalFailure++
					}
					totalInputTokens += record.InputTokens
					totalOutputTokens += record.OutputTokens
					totalCacheCreation += record.CacheCreationInputTokens
					totalCacheRead += record.CacheReadInputTokens
				}
			}
		}
	}

	// 构建数据点结果
	dataPoints := make([]GlobalHistoryDataPoint, numPoints)
	for i := 0; i < numPoints; i++ {
		b := buckets[int64(i)]
		successRate := float64(0)
		if b.requestCount > 0 {
			successRate = float64(b.successCount) / float64(b.requestCount) * 100
		}
		dataPoints[i] = GlobalHistoryDataPoint{
			Timestamp:           startTime.Add(time.Duration(i+1) * interval),
			RequestCount:        b.requestCount,
			SuccessCount:        b.successCount,
			FailureCount:        b.failureCount,
			SuccessRate:         successRate,
			InputTokens:         b.inputTokens,
			OutputTokens:        b.outputTokens,
			CacheCreationTokens: b.cacheCreationTokens,
			CacheReadTokens:     b.cacheReadTokens,
		}
	}

	// 计算平均成功率
	avgSuccessRate := float64(0)
	if totalRequests > 0 {
		avgSuccessRate = float64(totalSuccess) / float64(totalRequests) * 100
	}

	summary := GlobalStatsSummary{
		TotalRequests:            totalRequests,
		TotalSuccess:             totalSuccess,
		TotalFailure:             totalFailure,
		TotalInputTokens:         totalInputTokens,
		TotalOutputTokens:        totalOutputTokens,
		TotalCacheCreationTokens: totalCacheCreation,
		TotalCacheReadTokens:     totalCacheRead,
		AvgSuccessRate:           avgSuccessRate,
		Duration:                 duration.String(),
	}

	return GlobalStatsHistoryResponse{
		DataPoints: dataPoints,
		Summary:    summary,
	}
}

// globalBucketData 全局统计时间分桶的辅助结构
type globalBucketData struct {
	requestCount        int64
	successCount        int64
	failureCount        int64
	inputTokens         int64
	outputTokens        int64
	cacheCreationTokens int64
	cacheReadTokens     int64
}

// CalculateTodayDuration 计算"今日"时间范围（从今天 0 点到现在）
func CalculateTodayDuration() time.Duration {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return now.Sub(startOfDay)
}

// ============ 渠道实时活跃度数据（用于渐变背景显示）============

// ActivitySegment 活跃度分段数据（每 6 秒一段）
type ActivitySegment struct {
	RequestCount int64 `json:"requestCount"`
	SuccessCount int64 `json:"successCount"`
	FailureCount int64 `json:"failureCount"`
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
}

// ChannelRecentActivity 渠道最近活跃度数据
type ChannelRecentActivity struct {
	ChannelIndex int               `json:"channelIndex"`
	Segments     []ActivitySegment `json:"segments"` // 150 段，每段 6 秒，从旧到新（共 15 分钟）
	RPM          float64           `json:"rpm"`      // 15分钟平均 RPM
	TPM          float64           `json:"tpm"`      // 15分钟平均 TPM
}

// GetRecentActivityMultiURL 获取渠道最近活跃度数据（支持多 URL 和多 Key 聚合）
// 参数：
//   - channelIndex: 渠道索引
//   - baseURLs: 渠道的所有故障转移 URL（支持多个）
//   - activeKeys: 渠道的所有活跃 API Key（支持多个）
//
// 返回：
//   - 150 段活跃度数据（每段 6 秒，共 15 分钟）
//   - 自动聚合所有 URL × Key 组合的请求数据
//   - RPM/TPM 为 15 分钟平均值
func (m *MetricsManager) GetRecentActivityMultiURL(channelIndex int, baseURLs []string, activeKeys []string) *ChannelRecentActivity {
	// 150 段，每段 6 秒 = 900 秒 = 15 分钟
	const numSegments = 150
	const segmentDuration = 6 * time.Second

	if len(baseURLs) == 0 || len(activeKeys) == 0 {
		return &ChannelRecentActivity{
			ChannelIndex: channelIndex,
			Segments:     make([]ActivitySegment, numSegments),
			RPM:          0,
			TPM:          0,
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()

	// 时间边界对齐：将 endTime 向上对齐到下一个 segmentDuration 边界
	// 这样每次请求的分段边界都是固定的，不会因为 now 的微小变化而导致数据跳动
	// 例如：segmentDuration=6s，now=12:34:57，则 endTime=12:35:00（包含当前正在进行的段）
	endTimeUnix := now.Unix()
	segmentSeconds := int64(segmentDuration.Seconds())
	alignedEndUnix := ((endTimeUnix / segmentSeconds) + 1) * segmentSeconds
	endTime := time.Unix(alignedEndUnix, 0)
	startTime := endTime.Add(-time.Duration(numSegments) * segmentDuration)

	// 初始化分段数据
	segments := make([]ActivitySegment, numSegments)

	// 汇总统计
	var totalRequests, totalInputTokens, totalOutputTokens int64

	// 遍历所有 BaseURL 和 Key 的组合
	for _, baseURL := range baseURLs {
		for _, apiKey := range activeKeys {
			metricsKey := generateMetricsKey(baseURL, apiKey)
			metrics, exists := m.keyMetrics[metricsKey]
			if !exists {
				continue
			}

			// 遍历该 Key 的请求历史，放入对应分段
			for _, record := range metrics.requestHistory {
				// 检查是否在 [startTime, endTime) 范围内
				if record.Timestamp.Before(startTime) || !record.Timestamp.Before(endTime) {
					continue
				}

				// 计算属于哪个分段
				offset := int(record.Timestamp.Sub(startTime) / segmentDuration)
				if offset < 0 || offset >= numSegments {
					continue
				}

				seg := &segments[offset]
				seg.RequestCount++
				if record.Success {
					seg.SuccessCount++
				} else {
					seg.FailureCount++
				}
				seg.InputTokens += record.InputTokens
				seg.OutputTokens += record.OutputTokens

				// 累加汇总
				totalRequests++
				totalInputTokens += record.InputTokens
				totalOutputTokens += record.OutputTokens
			}
		}
	}

	// 计算 RPM 和 TPM（基于实际窗口时长）
	// TPM 只计算输出 tokens（包含思考），不包含输入 tokens 和缓存 tokens
	windowMinutes := float64(numSegments) * segmentDuration.Minutes()
	rpm := float64(totalRequests) / windowMinutes
	tpm := float64(totalOutputTokens) / windowMinutes

	return &ChannelRecentActivity{
		ChannelIndex: channelIndex,
		Segments:     segments,
		RPM:          rpm,
		TPM:          tpm,
	}
}
