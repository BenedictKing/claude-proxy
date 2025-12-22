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
	LastSuccessAt       *time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt       *time.Time `json:"lastFailureAt,omitempty"`
	CircuitBrokenAt     *time.Time `json:"circuitBrokenAt,omitempty"` // 熔断开始时间
	// 滑动窗口记录（最近 N 次请求的结果）
	recentResults []bool // true=success, false=failure
	// 带时间戳的请求记录（用于分时段统计，保留24小时）
	requestHistory []RequestRecord
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
}

// MetricsManager 指标管理器
type MetricsManager struct {
	mu                  sync.RWMutex
	keyMetrics          map[string]*KeyMetrics // key: hash(baseURL + apiKey)
	windowSize          int                    // 滑动窗口大小
	failureThreshold    float64                // 失败率阈值
	circuitRecoveryTime time.Duration          // 熔断恢复时间
	stopCh              chan struct{}          // 用于停止清理 goroutine

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
			log.Printf("⚠️ [%s] 加载历史指标数据失败: %v", apiType, err)
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
		log.Printf("📊 [%s] 无历史指标数据需要加载", m.apiType)
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

	// 重建滑动窗口（取最近 windowSize 条记录）
	for _, metrics := range m.keyMetrics {
		n := len(metrics.requestHistory)
		start := 0
		if n > m.windowSize {
			start = n - m.windowSize
		}
		metrics.recentResults = make([]bool, 0, m.windowSize)
		for i := start; i < n; i++ {
			metrics.recentResults = append(metrics.recentResults, metrics.requestHistory[i].Success)
		}
	}

	log.Printf("✅ [%s] 已从持久化存储加载 %d 条历史记录，重建 %d 个 Key 指标",
		m.apiType, len(records), len(m.keyMetrics))
	return nil
}

// getOrCreateKeyLocked 获取或创建 Key 指标（用于加载时，已知 metricsKey 和 keyMask）
func (m *MetricsManager) getOrCreateKeyLocked(baseURL, metricsKey, keyMask string) *KeyMetrics {
	if metrics, exists := m.keyMetrics[metricsKey]; exists {
		return metrics
	}
	metrics := &KeyMetrics{
		MetricsKey:    metricsKey,
		BaseURL:       baseURL,
		KeyMask:       keyMask,
		recentResults: make([]bool, 0, m.windowSize),
	}
	m.keyMetrics[metricsKey] = metrics
	return metrics
}

// generateMetricsKey 生成指标键 hash(baseURL + apiKey)
func generateMetricsKey(baseURL, apiKey string) string {
	h := sha256.New()
	h.Write([]byte(baseURL + "|" + apiKey))
	return hex.EncodeToString(h.Sum(nil))[:16] // 取前16位作为键
}

// getOrCreateKey 获取或创建 Key 指标
func (m *MetricsManager) getOrCreateKey(baseURL, apiKey string) *KeyMetrics {
	metricsKey := generateMetricsKey(baseURL, apiKey)
	if metrics, exists := m.keyMetrics[metricsKey]; exists {
		return metrics
	}
	metrics := &KeyMetrics{
		MetricsKey:    metricsKey,
		BaseURL:       baseURL,
		KeyMask:       utils.MaskAPIKey(apiKey),
		recentResults: make([]bool, 0, m.windowSize),
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

	metrics := m.getOrCreateKey(baseURL, apiKey)
	metrics.RequestCount++
	metrics.SuccessCount++
	metrics.ConsecutiveFailures = 0

	now := time.Now()
	metrics.LastSuccessAt = &now

	// 成功后清除熔断标记
	if metrics.CircuitBrokenAt != nil {
		metrics.CircuitBrokenAt = nil
		log.Printf("✅ Key [%s] (%s) 因请求成功退出熔断状态", metrics.KeyMask, metrics.BaseURL)
	}

	// 更新滑动窗口
	m.appendToWindowKey(metrics, true)

	// 提取 Token 数据（如果有）
	var inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int64
	if usage != nil {
		inputTokens = int64(usage.InputTokens)
		outputTokens = int64(usage.OutputTokens)
		cacheCreationTokens = int64(usage.CacheCreationInputTokens)
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

	metrics := m.getOrCreateKey(baseURL, apiKey)
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
		log.Printf("⚡ Key [%s] (%s) 进入熔断状态（失败率: %.1f%%）", metrics.KeyMask, metrics.BaseURL, m.calculateKeyFailureRateInternal(metrics)*100)
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

	// 清理超过24小时的记录
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
	} else if newStart == -1 && len(metrics.requestHistory) > 0 {
		// 所有记录都过期，清空切片
		metrics.requestHistory = metrics.requestHistory[:0]
	}
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
		metrics.LastSuccessAt = nil
		metrics.LastFailureAt = nil
		metrics.CircuitBrokenAt = nil
		metrics.recentResults = make([]bool, 0, m.windowSize)
		metrics.requestHistory = nil
		log.Printf("🔄 Key [%s] (%s) 指标已完全重置", metrics.KeyMask, metrics.BaseURL)
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
				log.Printf("✅ Key [%s] (%s) 熔断自动恢复（已超过 %v）", metrics.KeyMask, metrics.BaseURL, m.circuitRecoveryTime)
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
		log.Printf("🧹 清理了 %d 个过期 Key 指标: %v", len(removed), removed)
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
					}
				}
			}
		}

		successRate := float64(100)
		if requestCount > 0 {
			successRate = float64(successCount) / float64(requestCount) * 100
		}

		result[label] = TimeWindowStats{
			RequestCount: requestCount,
			SuccessCount: successCount,
			FailureCount: failureCount,
			SuccessRate:  successRate,
		}
	}

	return result
}

// ============ 废弃的旧方法（保留签名以便编译，但标记为废弃）============

// Deprecated: 使用 IsChannelHealthyWithKeys 代替
// IsChannelHealthy 判断渠道是否健康（旧方法，不再使用 channelIndex）
// 此方法保留是为了兼容，但始终返回 true，调用方应迁移到新方法
func (m *MetricsManager) IsChannelHealthy(channelIndex int) bool {
	log.Printf("⚠️ 警告: 调用了废弃的 IsChannelHealthy(channelIndex=%d)，请迁移到 IsChannelHealthyWithKeys", channelIndex)
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
	log.Printf("⚠️ 警告: 调用了废弃的 Reset(channelIndex=%d)，请迁移到 ResetKey", channelIndex)
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
				// 使用 Before(endTime) 排除恰好落在 endTime 的记录，避免 offset 越界
				if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
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
			Timestamp:    startTime.Add(time.Duration(i+1) * interval),
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

// GetAllKeysHistoricalStats 获取所有 Key 的历史统计（不区分渠道）
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
			Timestamp:    startTime.Add(time.Duration(i+1) * interval),
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
