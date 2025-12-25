package session

import (
	"fmt"
	"sync"
	"time"
)

// ResourceAffinity 记录成功使用的资源索引
type ResourceAffinity struct {
	BaseURLIndex int
	APIKeyIndex  int
	LastUsedAt   time.Time
}

// ResourceAffinityManager 管理渠道内资源的亲和性
// Key 格式: "channelIndex:userID" → ResourceAffinity
type ResourceAffinityManager struct {
	mu       sync.RWMutex
	affinity map[string]*ResourceAffinity
	ttl      time.Duration
	stopCh   chan struct{}
}

// NewResourceAffinityManager 创建资源亲和性管理器
func NewResourceAffinityManager() *ResourceAffinityManager {
	mgr := &ResourceAffinityManager{
		affinity: make(map[string]*ResourceAffinity),
		ttl:      30 * time.Minute,
		stopCh:   make(chan struct{}),
	}
	go mgr.cleanupLoop()
	return mgr
}

// makeKey 生成 map key
func makeKey(channelIndex int, userID string) string {
	return fmt.Sprintf("%d:%s", channelIndex, userID)
}

// GetPreferred 获取偏好的资源索引（带越界检查）
func (m *ResourceAffinityManager) GetPreferred(channelIndex int, userID string,
	baseURLCount, apiKeyCount int) (baseURLIdx, apiKeyIdx int, ok bool) {

	if userID == "" {
		return -1, -1, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	aff, exists := m.affinity[makeKey(channelIndex, userID)]
	if !exists {
		return -1, -1, false
	}

	// TTL 检查
	if time.Since(aff.LastUsedAt) > m.ttl {
		return -1, -1, false
	}

	// 越界检查：用户可能已删除资源
	if aff.BaseURLIndex >= baseURLCount || aff.APIKeyIndex >= apiKeyCount {
		return -1, -1, false
	}

	return aff.BaseURLIndex, aff.APIKeyIndex, true
}

// Set 记录成功使用的资源索引
func (m *ResourceAffinityManager) Set(channelIndex int, userID string, baseURLIdx, apiKeyIdx int) {
	if userID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.affinity[makeKey(channelIndex, userID)] = &ResourceAffinity{
		BaseURLIndex: baseURLIdx,
		APIKeyIndex:  apiKeyIdx,
		LastUsedAt:   time.Now(),
	}
}

// RemoveByChannel 移除指定渠道的所有亲和记录
func (m *ResourceAffinityManager) RemoveByChannel(channelIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := fmt.Sprintf("%d:", channelIndex)
	for key := range m.affinity {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(m.affinity, key)
		}
	}
}

// Cleanup 清理过期记录
func (m *ResourceAffinityManager) Cleanup() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	cleaned := 0
	for key, aff := range m.affinity {
		if now.Sub(aff.LastUsedAt) > m.ttl {
			delete(m.affinity, key)
			cleaned++
		}
	}
	return cleaned
}

func (m *ResourceAffinityManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
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

// Stop 停止清理 goroutine
func (m *ResourceAffinityManager) Stop() {
	close(m.stopCh)
}

// Size 返回当前记录数量
func (m *ResourceAffinityManager) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.affinity)
}
