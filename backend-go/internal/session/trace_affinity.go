package session

import (
	"sync"
	"time"
)

// TraceAffinity 记录 trace 与渠道的亲和关系
type TraceAffinity struct {
	ChannelIndex int
	LastUsedAt   time.Time
}

// TraceAffinityManager 管理 trace 与渠道的亲和性
type TraceAffinityManager struct {
	mu       sync.RWMutex
	affinity map[string]*TraceAffinity // key: user_id
	ttl      time.Duration
	stopCh   chan struct{} // 用于停止清理 goroutine
}

// NewTraceAffinityManager 创建 Trace 亲和性管理器
func NewTraceAffinityManager() *TraceAffinityManager {
	mgr := &TraceAffinityManager{
		affinity: make(map[string]*TraceAffinity),
		ttl:      30 * time.Minute, // 默认 30 分钟无活动后过期
		stopCh:   make(chan struct{}),
	}

	// 启动定期清理
	go mgr.cleanupLoop()

	return mgr
}

// NewTraceAffinityManagerWithTTL 创建带自定义 TTL 的管理器
func NewTraceAffinityManagerWithTTL(ttl time.Duration) *TraceAffinityManager {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}

	mgr := &TraceAffinityManager{
		affinity: make(map[string]*TraceAffinity),
		ttl:      ttl,
		stopCh:   make(chan struct{}),
	}

	go mgr.cleanupLoop()

	return mgr
}

// GetPreferredChannel 获取 user_id 偏好的渠道
// 返回渠道索引和是否存在
func (m *TraceAffinityManager) GetPreferredChannel(userID string) (int, bool) {
	if userID == "" {
		return -1, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	affinity, exists := m.affinity[userID]
	if !exists {
		return -1, false
	}

	// 检查是否过期
	if time.Since(affinity.LastUsedAt) > m.ttl {
		return -1, false
	}

	return affinity.ChannelIndex, true
}

// SetPreferredChannel 设置 user_id 偏好的渠道
func (m *TraceAffinityManager) SetPreferredChannel(userID string, channelIndex int) {
	if userID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.affinity[userID] = &TraceAffinity{
		ChannelIndex: channelIndex,
		LastUsedAt:   time.Now(),
	}
}

// UpdateLastUsed 更新最后使用时间（续期）
func (m *TraceAffinityManager) UpdateLastUsed(userID string) {
	if userID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if affinity, exists := m.affinity[userID]; exists {
		affinity.LastUsedAt = time.Now()
	}
}

// Remove 移除 user_id 的亲和记录
func (m *TraceAffinityManager) Remove(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.affinity, userID)
}

// RemoveByChannel 移除指定渠道的所有亲和记录
// 用于渠道被禁用或删除时
func (m *TraceAffinityManager) RemoveByChannel(channelIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for userID, affinity := range m.affinity {
		if affinity.ChannelIndex == channelIndex {
			delete(m.affinity, userID)
		}
	}
}

// Cleanup 清理过期的亲和记录
func (m *TraceAffinityManager) Cleanup() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	cleaned := 0

	for userID, affinity := range m.affinity {
		if now.Sub(affinity.LastUsedAt) > m.ttl {
			delete(m.affinity, userID)
			cleaned++
		}
	}

	return cleaned
}

// cleanupLoop 定期清理过期记录
func (m *TraceAffinityManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute) // 每 5 分钟清理一次
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.Cleanup()
		case <-m.stopCh:
			return
		}
	}
}

// Stop 停止清理 goroutine，释放资源
func (m *TraceAffinityManager) Stop() {
	close(m.stopCh)
}

// Size 返回当前亲和记录数量
func (m *TraceAffinityManager) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.affinity)
}

// GetTTL 获取 TTL 设置
func (m *TraceAffinityManager) GetTTL() time.Duration {
	return m.ttl
}

// GetAll 获取所有亲和记录（用于调试）
func (m *TraceAffinityManager) GetAll() map[string]TraceAffinity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]TraceAffinity, len(m.affinity))
	for userID, affinity := range m.affinity {
		result[userID] = *affinity
	}
	return result
}
