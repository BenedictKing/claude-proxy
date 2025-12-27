// Package warmup 提供多端点渠道的预热和延迟排序功能
package warmup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/httpclient"
)

// URLLatencyResult 单个 URL 的延迟测试结果
type URLLatencyResult struct {
	URL         string
	OriginalIdx int           // 原始索引（用于指标记录）
	Latency     time.Duration // 延迟时间
	Success     bool          // 是否成功
	Error       string        // 错误信息（如果失败）
}

// ChannelWarmupCache 渠道端点预热缓存
type ChannelWarmupCache struct {
	ChannelKey string             // 缓存键
	SortedURLs []URLLatencyResult // 按延迟排序的结果
	CachedAt   time.Time          // 缓存时间
	ExpiresAt  time.Time          // 过期时间
}

// URLWarmupManager 端点预热管理器
type URLWarmupManager struct {
	mu             sync.RWMutex
	cache          map[string]*ChannelWarmupCache // key: channelKey
	cacheTTL       time.Duration                  // 缓存有效期（默认5分钟）
	pingTimeout    time.Duration                  // 单个 ping 超时（默认5秒）
	pendingWarmups map[string]chan struct{}       // 正在进行的预热任务
}

// NewURLWarmupManager 创建预热管理器
func NewURLWarmupManager(cacheTTL, pingTimeout time.Duration) *URLWarmupManager {
	return &URLWarmupManager{
		cache:          make(map[string]*ChannelWarmupCache),
		cacheTTL:       cacheTTL,
		pingTimeout:    pingTimeout,
		pendingWarmups: make(map[string]chan struct{}),
	}
}

// GetSortedURLs 获取排序后的 URL 列表（核心方法）
// 如果缓存有效，直接返回缓存结果
// 如果缓存过期或不存在，触发预热并等待完成
// 返回值：排序后的 URL 列表（包含原始索引）
func (m *URLWarmupManager) GetSortedURLs(ctx context.Context, channelIndex int, urls []string, insecureSkipVerify bool) []URLLatencyResult {
	// 单个或无 URL，无需预热
	if len(urls) <= 1 {
		if len(urls) == 1 {
			return []URLLatencyResult{{URL: urls[0], OriginalIdx: 0, Success: true}}
		}
		return nil
	}

	channelKey := m.generateChannelKey(channelIndex, urls)

	// 1. 快速路径：检查缓存
	m.mu.RLock()
	if cache, ok := m.cache[channelKey]; ok && time.Now().Before(cache.ExpiresAt) {
		m.mu.RUnlock()
		log.Printf("[Warmup-Cache] 缓存命中: 渠道 [%d]", channelIndex)
		return cache.SortedURLs
	}
	m.mu.RUnlock()

	// 2. 慢路径：需要预热
	m.mu.Lock()

	// 2.1 双重检查
	if cache, ok := m.cache[channelKey]; ok && time.Now().Before(cache.ExpiresAt) {
		m.mu.Unlock()
		log.Printf("[Warmup-Cache] 缓存命中(二次检查): 渠道 [%d]", channelIndex)
		return cache.SortedURLs
	}

	// 2.2 检查是否有正在进行的预热
	if waitCh, ok := m.pendingWarmups[channelKey]; ok {
		m.mu.Unlock()
		// 等待已有预热完成
		select {
		case <-waitCh:
			// 预热完成，读取缓存
			m.mu.RLock()
			cache := m.cache[channelKey]
			m.mu.RUnlock()
			if cache != nil {
				return cache.SortedURLs
			}
			// 缓存为空，返回原始顺序
			return m.urlsToResults(urls)
		case <-ctx.Done():
			// 上下文取消，返回原始顺序
			return m.urlsToResults(urls)
		}
	}

	// 2.3 创建等待通道，执行预热
	waitCh := make(chan struct{})
	m.pendingWarmups[channelKey] = waitCh
	m.mu.Unlock()

	// 执行预热（不持锁）
	cache := m.warmupChannel(channelIndex, urls, insecureSkipVerify)

	// 更新缓存
	m.mu.Lock()
	m.cache[channelKey] = cache
	delete(m.pendingWarmups, channelKey)
	close(waitCh) // 通知所有等待者
	m.mu.Unlock()

	return cache.SortedURLs
}

// InvalidateCache 使指定渠道的缓存失效
func (m *URLWarmupManager) InvalidateCache(channelIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 遍历所有缓存，删除匹配的渠道
	prefix := fmt.Sprintf("%d|", channelIndex)
	for key := range m.cache {
		if strings.HasPrefix(key, prefix) {
			delete(m.cache, key)
			log.Printf("[Warmup-Cache] 缓存失效: 渠道 [%d]", channelIndex)
		}
	}
}

// InvalidateAllCache 使所有缓存失效
func (m *URLWarmupManager) InvalidateAllCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = make(map[string]*ChannelWarmupCache)
	log.Printf("[Warmup-Cache] 所有缓存已清空")
}

// GetCacheStats 获取缓存统计（用于调试/监控）
func (m *URLWarmupManager) GetCacheStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	validCount := 0
	expiredCount := 0
	now := time.Now()

	for _, cache := range m.cache {
		if now.Before(cache.ExpiresAt) {
			validCount++
		} else {
			expiredCount++
		}
	}

	return map[string]interface{}{
		"total_entries":   len(m.cache),
		"valid_entries":   validCount,
		"expired_entries": expiredCount,
		"pending_warmups": len(m.pendingWarmups),
		"cache_ttl":       m.cacheTTL.String(),
	}
}

// warmupChannel 执行渠道预热
func (m *URLWarmupManager) warmupChannel(channelIndex int, urls []string, insecureSkipVerify bool) *ChannelWarmupCache {
	log.Printf("[Warmup-Channel] 渠道 [%d] 开始预热: %d 个 URL", channelIndex, len(urls))

	// 并发 ping 所有 URL
	results := make([]URLLatencyResult, len(urls))
	var wg sync.WaitGroup

	for i, url := range urls {
		wg.Add(1)
		go func(idx int, testURL string) {
			defer wg.Done()
			results[idx] = m.pingURL(testURL, idx, insecureSkipVerify)
		}(i, url)
	}
	wg.Wait()

	// 按延迟排序（成功的端点优先，同类型按延迟升序）
	sortedResults := make([]URLLatencyResult, len(results))
	copy(sortedResults, results)

	sort.Slice(sortedResults, func(i, j int) bool {
		// 成功的端点优先于失败的端点
		if sortedResults[i].Success != sortedResults[j].Success {
			return sortedResults[i].Success
		}
		// 同类型按延迟升序
		return sortedResults[i].Latency < sortedResults[j].Latency
	})

	// 记录日志
	if len(sortedResults) > 0 {
		fastest := sortedResults[0]
		status := "成功"
		if !fastest.Success {
			status = "失败"
		}
		log.Printf("[Warmup-Channel] 渠道 [%d] 预热完成: %d 个 URL, 最快: %s (%dms, %s)",
			channelIndex, len(urls), fastest.URL, fastest.Latency.Milliseconds(), status)
	}

	now := time.Now()
	return &ChannelWarmupCache{
		ChannelKey: m.generateChannelKey(channelIndex, urls),
		SortedURLs: sortedResults,
		CachedAt:   now,
		ExpiresAt:  now.Add(m.cacheTTL),
	}
}

// pingURL 测试单个 URL
func (m *URLWarmupManager) pingURL(testURL string, originalIdx int, insecureSkipVerify bool) URLLatencyResult {
	startTime := time.Now()
	testURL = strings.TrimSuffix(testURL, "/")

	client := httpclient.GetManager().GetStandardClient(m.pingTimeout, insecureSkipVerify)

	req, err := http.NewRequest("HEAD", testURL, nil)
	if err != nil {
		return URLLatencyResult{
			URL:         testURL,
			OriginalIdx: originalIdx,
			Latency:     time.Since(startTime),
			Success:     false,
			Error:       "req_creation_failed",
		}
	}

	resp, err := client.Do(req)
	latency := time.Since(startTime)

	if err != nil {
		return URLLatencyResult{
			URL:         testURL,
			OriginalIdx: originalIdx,
			Latency:     latency,
			Success:     false,
			Error:       err.Error(),
		}
	}
	resp.Body.Close()

	return URLLatencyResult{
		URL:         testURL,
		OriginalIdx: originalIdx,
		Latency:     latency,
		Success:     true,
	}
}

// generateChannelKey 生成渠道缓存键
func (m *URLWarmupManager) generateChannelKey(channelIndex int, urls []string) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d|", channelIndex)))
	for _, url := range urls {
		h.Write([]byte(url + "|"))
	}
	return fmt.Sprintf("%d|%s", channelIndex, hex.EncodeToString(h.Sum(nil))[:16])
}

// urlsToResults 将 URL 列表转换为默认结果（保持原始顺序）
func (m *URLWarmupManager) urlsToResults(urls []string) []URLLatencyResult {
	results := make([]URLLatencyResult, len(urls))
	for i, url := range urls {
		results[i] = URLLatencyResult{
			URL:         url,
			OriginalIdx: i,
			Success:     true,
		}
	}
	return results
}
