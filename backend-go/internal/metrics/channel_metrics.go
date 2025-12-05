package metrics

import (
	"sync"
	"time"
)

// ChannelMetrics 渠道指标
type ChannelMetrics struct {
	ChannelIndex        int        `json:"channelIndex"`
	RequestCount        int64      `json:"requestCount"`
	SuccessCount        int64      `json:"successCount"`
	FailureCount        int64      `json:"failureCount"`
	ConsecutiveFailures int64      `json:"consecutiveFailures"`
	LastSuccessAt       *time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt       *time.Time `json:"lastFailureAt,omitempty"`
	// 滑动窗口记录（最近 N 次请求的结果）
	recentResults []bool // true=success, false=failure
}

// MetricsManager 指标管理器
type MetricsManager struct {
	mu           sync.RWMutex
	metrics      map[int]*ChannelMetrics // key: channelIndex
	windowSize   int                     // 滑动窗口大小
	failureThreshold float64             // 失败率阈值
}

// NewMetricsManager 创建指标管理器
func NewMetricsManager() *MetricsManager {
	return &MetricsManager{
		metrics:          make(map[int]*ChannelMetrics),
		windowSize:       10,  // 默认基于最近 10 次请求计算失败率
		failureThreshold: 0.5, // 默认 50% 失败率阈值
	}
}

// NewMetricsManagerWithConfig 创建带配置的指标管理器
func NewMetricsManagerWithConfig(windowSize int, failureThreshold float64) *MetricsManager {
	if windowSize <= 0 {
		windowSize = 10
	}
	if failureThreshold <= 0 || failureThreshold > 1 {
		failureThreshold = 0.5
	}
	return &MetricsManager{
		metrics:          make(map[int]*ChannelMetrics),
		windowSize:       windowSize,
		failureThreshold: failureThreshold,
	}
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

	// 更新滑动窗口
	m.appendToWindow(metrics, true)
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
}

// appendToWindow 向滑动窗口添加记录
func (m *MetricsManager) appendToWindow(metrics *ChannelMetrics, success bool) {
	metrics.recentResults = append(metrics.recentResults, success)
	// 保持窗口大小
	if len(metrics.recentResults) > m.windowSize {
		metrics.recentResults = metrics.recentResults[1:]
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
		// 保留历史统计，但清除滑动窗口
	}
}

// ResetAll 重置所有渠道指标
func (m *MetricsManager) ResetAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = make(map[int]*ChannelMetrics)
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

	return resp
}
