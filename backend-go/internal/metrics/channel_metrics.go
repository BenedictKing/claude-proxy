package metrics

import (
	"log"
	"sync"
	"time"
)

// RequestRecord 带时间戳的请求记录
type RequestRecord struct {
	Timestamp time.Time
	Success   bool
}

// ChannelMetrics 渠道指标
type ChannelMetrics struct {
	ChannelIndex        int        `json:"channelIndex"`
	RequestCount        int64      `json:"requestCount"`
	SuccessCount        int64      `json:"successCount"`
	FailureCount        int64      `json:"failureCount"`
	ConsecutiveFailures int64      `json:"consecutiveFailures"`
	LastSuccessAt       *time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt       *time.Time `json:"lastFailureAt,omitempty"`
	CircuitBrokenAt     *time.Time `json:"circuitBrokenAt,omitempty"` // 熔断开始时间
	// 滑动窗口记录（最近 N 次请求的结果）
	recentResults []bool // true=success, false=failure
	// 带时间戳的请求记录（用于分时段统计，保留24小时）
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
	metrics             map[int]*ChannelMetrics // key: channelIndex
	windowSize          int                     // 滑动窗口大小
	failureThreshold    float64                 // 失败率阈值
	circuitRecoveryTime time.Duration           // 熔断恢复时间
	stopCh              chan struct{}           // 用于停止清理 goroutine
}

// NewMetricsManager 创建指标管理器
func NewMetricsManager() *MetricsManager {
	m := &MetricsManager{
		metrics:             make(map[int]*ChannelMetrics),
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
	if windowSize <= 0 {
		windowSize = 10
	}
	if failureThreshold <= 0 || failureThreshold > 1 {
		failureThreshold = 0.5
	}
	m := &MetricsManager{
		metrics:             make(map[int]*ChannelMetrics),
		windowSize:          windowSize,
		failureThreshold:    failureThreshold,
		circuitRecoveryTime: 15 * time.Minute,
		stopCh:              make(chan struct{}),
	}
	// 启动后台熔断恢复任务
	go m.cleanupCircuitBreakers()
	return m
}

// getOrCreate 获取或创建渠道指标
func (m *MetricsManager) getOrCreate(channelIndex int) *ChannelMetrics {
	if metrics, exists := m.metrics[channelIndex]; exists {
		return metrics
	}
	metrics := &ChannelMetrics{
		ChannelIndex:  channelIndex,
		recentResults: make([]bool, 0, m.windowSize),
	}
	m.metrics[channelIndex] = metrics
	return metrics
}

// RecordSuccess 记录成功请求
func (m *MetricsManager) RecordSuccess(channelIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := m.getOrCreate(channelIndex)
	metrics.RequestCount++
	metrics.SuccessCount++
	metrics.ConsecutiveFailures = 0

	now := time.Now()
	metrics.LastSuccessAt = &now

	// 成功后清除熔断标记
	if metrics.CircuitBrokenAt != nil {
		metrics.CircuitBrokenAt = nil
		log.Printf("✅ 渠道 [%d] 因请求成功退出熔断状态", channelIndex)
	}

	// 更新滑动窗口
	m.appendToWindow(metrics, true)

	// 记录带时间戳的请求
	m.appendToHistory(metrics, now, true)
}

// RecordFailure 记录失败请求
func (m *MetricsManager) RecordFailure(channelIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := m.getOrCreate(channelIndex)
	metrics.RequestCount++
	metrics.FailureCount++
	metrics.ConsecutiveFailures++

	now := time.Now()
	metrics.LastFailureAt = &now

	// 更新滑动窗口
	m.appendToWindow(metrics, false)

	// 检查是否刚进入熔断状态
	if metrics.CircuitBrokenAt == nil && m.isCircuitBroken(metrics) {
		metrics.CircuitBrokenAt = &now
		log.Printf("⚡ 渠道 [%d] 进入熔断状态（失败率: %.1f%%）", channelIndex, m.calculateFailureRateInternal(metrics)*100)
	}

	// 记录带时间戳的请求
	m.appendToHistory(metrics, now, false)
}

// isCircuitBroken 判断是否达到熔断条件（内部方法，调用前需持有锁）
func (m *MetricsManager) isCircuitBroken(metrics *ChannelMetrics) bool {
	minRequests := m.windowSize / 2
	if len(metrics.recentResults) < minRequests {
		return false
	}
	return m.calculateFailureRateInternal(metrics) >= m.failureThreshold
}

// calculateFailureRateInternal 计算失败率（内部方法，调用前需持有锁）
func (m *MetricsManager) calculateFailureRateInternal(metrics *ChannelMetrics) float64 {
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

// appendToWindow 向滑动窗口添加记录
func (m *MetricsManager) appendToWindow(metrics *ChannelMetrics, success bool) {
	metrics.recentResults = append(metrics.recentResults, success)
	// 保持窗口大小
	if len(metrics.recentResults) > m.windowSize {
		metrics.recentResults = metrics.recentResults[1:]
	}
}

// appendToHistory 向历史记录添加请求（保留24小时）
func (m *MetricsManager) appendToHistory(metrics *ChannelMetrics, timestamp time.Time, success bool) {
	metrics.requestHistory = append(metrics.requestHistory, RequestRecord{
		Timestamp: timestamp,
		Success:   success,
	})

	// 清理超过24小时的记录
	cutoff := time.Now().Add(-24 * time.Hour)
	newStart := 0
	for i, record := range metrics.requestHistory {
		if record.Timestamp.After(cutoff) {
			newStart = i
			break
		}
	}
	if newStart > 0 {
		metrics.requestHistory = metrics.requestHistory[newStart:]
	}
}

// GetTimeWindowStats 获取指定时间窗口内的统计
func (m *MetricsManager) GetTimeWindowStats(channelIndex int, duration time.Duration) TimeWindowStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics, exists := m.metrics[channelIndex]
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

// GetAllTimeWindowStats 获取所有时间窗口的统计（15m, 1h, 6h, 24h）
func (m *MetricsManager) GetAllTimeWindowStats(channelIndex int) map[string]TimeWindowStats {
	return map[string]TimeWindowStats{
		"15m": m.GetTimeWindowStats(channelIndex, 15*time.Minute),
		"1h":  m.GetTimeWindowStats(channelIndex, 1*time.Hour),
		"6h":  m.GetTimeWindowStats(channelIndex, 6*time.Hour),
		"24h": m.GetTimeWindowStats(channelIndex, 24*time.Hour),
	}
}

// GetMetrics 获取渠道指标
func (m *MetricsManager) GetMetrics(channelIndex int) *ChannelMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if metrics, exists := m.metrics[channelIndex]; exists {
		// 返回副本
		return &ChannelMetrics{
			ChannelIndex:        metrics.ChannelIndex,
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

// GetAllMetrics 获取所有渠道指标
func (m *MetricsManager) GetAllMetrics() []*ChannelMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ChannelMetrics, 0, len(m.metrics))
	for _, metrics := range m.metrics {
		result = append(result, &ChannelMetrics{
			ChannelIndex:        metrics.ChannelIndex,
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

// CalculateFailureRate 计算渠道失败率（基于滑动窗口）
func (m *MetricsManager) CalculateFailureRate(channelIndex int) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics, exists := m.metrics[channelIndex]
	if !exists || len(metrics.recentResults) == 0 {
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

// CalculateSuccessRate 计算渠道成功率（基于滑动窗口）
func (m *MetricsManager) CalculateSuccessRate(channelIndex int) float64 {
	return 1 - m.CalculateFailureRate(channelIndex)
}

// IsChannelHealthy 判断渠道是否健康（失败率低于阈值）
func (m *MetricsManager) IsChannelHealthy(channelIndex int) bool {
	return m.CalculateFailureRate(channelIndex) < m.failureThreshold
}

// ShouldSuspend 判断是否应该熔断（失败率达到阈值）
func (m *MetricsManager) ShouldSuspend(channelIndex int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics, exists := m.metrics[channelIndex]
	if !exists {
		return false
	}

	// 至少有一定数量的请求才判断
	minRequests := m.windowSize / 2
	if len(metrics.recentResults) < minRequests {
		return false
	}

	return m.CalculateFailureRate(channelIndex) >= m.failureThreshold
}

// Reset 重置渠道指标（用于恢复熔断）
func (m *MetricsManager) Reset(channelIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if metrics, exists := m.metrics[channelIndex]; exists {
		metrics.ConsecutiveFailures = 0
		metrics.recentResults = make([]bool, 0, m.windowSize)
		metrics.CircuitBrokenAt = nil // 清除熔断时间
		// 保留历史统计，但清除滑动窗口
		log.Printf("🔄 渠道 [%d] 指标已手动重置", channelIndex)
	}
}

// ResetAll 重置所有渠道指标
func (m *MetricsManager) ResetAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = make(map[int]*ChannelMetrics)
}

// Stop 停止后台清理任务
func (m *MetricsManager) Stop() {
	close(m.stopCh)
}

// cleanupCircuitBreakers 后台任务：定期检查并恢复超时的熔断渠道
func (m *MetricsManager) cleanupCircuitBreakers() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.recoverExpiredCircuitBreakers()
		case <-m.stopCh:
			return
		}
	}
}

// recoverExpiredCircuitBreakers 恢复超时的熔断渠道
func (m *MetricsManager) recoverExpiredCircuitBreakers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for idx, metrics := range m.metrics {
		if metrics.CircuitBrokenAt != nil {
			elapsed := now.Sub(*metrics.CircuitBrokenAt)
			if elapsed > m.circuitRecoveryTime {
				// 重置熔断状态
				metrics.ConsecutiveFailures = 0
				metrics.recentResults = make([]bool, 0, m.windowSize)
				metrics.CircuitBrokenAt = nil
				log.Printf("✅ 渠道 [%d] 熔断自动恢复（已超过 %v）", idx, m.circuitRecoveryTime)
			}
		}
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

// MetricsResponse API 响应结构
type MetricsResponse struct {
	ChannelIndex        int     `json:"channelIndex"`
	RequestCount        int64   `json:"requestCount"`
	SuccessCount        int64   `json:"successCount"`
	FailureCount        int64   `json:"failureCount"`
	SuccessRate         float64 `json:"successRate"`
	ErrorRate           float64 `json:"errorRate"`
	ConsecutiveFailures int64   `json:"consecutiveFailures"`
	Latency             int64   `json:"latency"` // 需要从其他地方获取
	LastSuccessAt       *string `json:"lastSuccessAt,omitempty"`
	LastFailureAt       *string `json:"lastFailureAt,omitempty"`
	CircuitBrokenAt     *string `json:"circuitBrokenAt,omitempty"` // 熔断开始时间
}

// ToResponse 转换为 API 响应格式
func (m *MetricsManager) ToResponse(channelIndex int, latency int64) *MetricsResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics, exists := m.metrics[channelIndex]
	if !exists {
		return &MetricsResponse{
			ChannelIndex: channelIndex,
			SuccessRate:  100,
			ErrorRate:    0,
			Latency:      latency,
		}
	}

	failureRate := m.CalculateFailureRate(channelIndex)
	successRate := (1 - failureRate) * 100

	resp := &MetricsResponse{
		ChannelIndex:        channelIndex,
		RequestCount:        metrics.RequestCount,
		SuccessCount:        metrics.SuccessCount,
		FailureCount:        metrics.FailureCount,
		SuccessRate:         successRate,
		ErrorRate:           failureRate * 100,
		ConsecutiveFailures: metrics.ConsecutiveFailures,
		Latency:             latency,
	}

	if metrics.LastSuccessAt != nil {
		t := metrics.LastSuccessAt.Format(time.RFC3339)
		resp.LastSuccessAt = &t
	}
	if metrics.LastFailureAt != nil {
		t := metrics.LastFailureAt.Format(time.RFC3339)
		resp.LastFailureAt = &t
	}
	if metrics.CircuitBrokenAt != nil {
		t := metrics.CircuitBrokenAt.Format(time.RFC3339)
		resp.CircuitBrokenAt = &t
	}

	return resp
}
