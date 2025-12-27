package scheduler

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/metrics"
	"github.com/BenedictKing/claude-proxy/internal/session"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/warmup"
)

// ChannelScheduler 多渠道调度器
type ChannelScheduler struct {
	mu                      sync.RWMutex
	configManager           *config.ConfigManager
	messagesMetricsManager  *metrics.MetricsManager // Messages 渠道指标
	responsesMetricsManager *metrics.MetricsManager // Responses 渠道指标
	traceAffinity           *session.TraceAffinityManager
	urlManager              *warmup.URLManager // URL 管理器（非阻塞，动态排序）
}

// NewChannelScheduler 创建多渠道调度器
func NewChannelScheduler(
	cfgManager *config.ConfigManager,
	messagesMetrics *metrics.MetricsManager,
	responsesMetrics *metrics.MetricsManager,
	traceAffinity *session.TraceAffinityManager,
	urlMgr *warmup.URLManager,
) *ChannelScheduler {
	return &ChannelScheduler{
		configManager:           cfgManager,
		messagesMetricsManager:  messagesMetrics,
		responsesMetricsManager: responsesMetrics,
		traceAffinity:           traceAffinity,
		urlManager:              urlMgr,
	}
}

// getMetricsManager 根据类型获取对应的指标管理器
func (s *ChannelScheduler) getMetricsManager(isResponses bool) *metrics.MetricsManager {
	if isResponses {
		return s.responsesMetricsManager
	}
	return s.messagesMetricsManager
}

// SelectionResult 渠道选择结果
type SelectionResult struct {
	Upstream     *config.UpstreamConfig
	ChannelIndex int
	Reason       string // 选择原因（用于日志）
}

// SelectChannel 选择最佳渠道
// 优先级: 促销期渠道 > Trace亲和（促销渠道失败时回退） > 渠道优先级顺序
func (s *ChannelScheduler) SelectChannel(
	ctx context.Context,
	userID string,
	failedChannels map[int]bool,
	isResponses bool,
) (*SelectionResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 获取活跃渠道列表
	activeChannels := s.getActiveChannels(isResponses)
	if len(activeChannels) == 0 {
		return nil, fmt.Errorf("没有可用的活跃渠道")
	}

	// 获取对应类型的指标管理器
	metricsManager := s.getMetricsManager(isResponses)

	// 0. 检查促销期渠道（最高优先级，绕过健康检查）
	promotedChannel := s.findPromotedChannel(activeChannels, isResponses)
	if promotedChannel != nil && !failedChannels[promotedChannel.Index] {
		// 促销渠道存在且未失败，直接使用（不检查健康状态，让用户设置的促销渠道有机会尝试）
		upstream := s.getUpstreamByIndex(promotedChannel.Index, isResponses)
		if upstream != nil && len(upstream.APIKeys) > 0 {
			failureRate := metricsManager.CalculateChannelFailureRate(upstream.BaseURL, upstream.APIKeys)
			log.Printf("[Scheduler-Promotion] 促销期优先选择渠道: [%d] %s (失败率: %.1f%%, 绕过健康检查)", promotedChannel.Index, upstream.Name, failureRate*100)
			return &SelectionResult{
				Upstream:     upstream,
				ChannelIndex: promotedChannel.Index,
				Reason:       "promotion_priority",
			}, nil
		} else if upstream != nil {
			log.Printf("[Scheduler-Promotion] 警告: 促销渠道 [%d] %s 无可用密钥，跳过", promotedChannel.Index, upstream.Name)
		}
	} else if promotedChannel != nil {
		log.Printf("[Scheduler-Promotion] 警告: 促销渠道 [%d] %s 已在本次请求中失败，跳过", promotedChannel.Index, promotedChannel.Name)
	}

	// 1. 检查 Trace 亲和性（促销渠道失败时或无促销渠道时）
	if userID != "" {
		if preferredIdx, ok := s.traceAffinity.GetPreferredChannel(userID); ok {
			for _, ch := range activeChannels {
				if ch.Index == preferredIdx && !failedChannels[preferredIdx] {
					// 检查渠道状态：只有 active 状态才使用亲和性
					if ch.Status != "active" {
						log.Printf("[Scheduler-Affinity] 跳过亲和渠道 [%d] %s: 状态为 %s (user: %s)", preferredIdx, ch.Name, ch.Status, maskUserID(userID))
						continue
					}
					// 检查渠道是否健康
					upstream := s.getUpstreamByIndex(preferredIdx, isResponses)
					if upstream != nil && metricsManager.IsChannelHealthyWithKeys(upstream.BaseURL, upstream.APIKeys) {
						log.Printf("[Scheduler-Affinity] Trace亲和选择渠道: [%d] %s (user: %s)", preferredIdx, upstream.Name, maskUserID(userID))
						return &SelectionResult{
							Upstream:     upstream,
							ChannelIndex: preferredIdx,
							Reason:       "trace_affinity",
						}, nil
					}
				}
			}
		}
	}

	// 2. 按优先级遍历活跃渠道
	for _, ch := range activeChannels {
		// 跳过本次请求已经失败的渠道
		if failedChannels[ch.Index] {
			continue
		}

		// 跳过非 active 状态的渠道（suspended 等）
		if ch.Status != "active" {
			log.Printf("[Scheduler-Channel] 跳过非活跃渠道: [%d] %s (状态: %s)", ch.Index, ch.Name, ch.Status)
			continue
		}

		upstream := s.getUpstreamByIndex(ch.Index, isResponses)
		if upstream == nil || len(upstream.APIKeys) == 0 {
			continue
		}

		// 跳过失败率过高的渠道（已熔断或即将熔断）
		if !metricsManager.IsChannelHealthyWithKeys(upstream.BaseURL, upstream.APIKeys) {
			failureRate := metricsManager.CalculateChannelFailureRate(upstream.BaseURL, upstream.APIKeys)
			log.Printf("[Scheduler-Channel] 警告: 跳过不健康渠道: [%d] %s (失败率: %.1f%%)", ch.Index, ch.Name, failureRate*100)
			continue
		}

		log.Printf("[Scheduler-Channel] 选择渠道: [%d] %s (优先级: %d)", ch.Index, upstream.Name, ch.Priority)
		return &SelectionResult{
			Upstream:     upstream,
			ChannelIndex: ch.Index,
			Reason:       "priority_order",
		}, nil
	}

	// 3. 所有健康渠道都失败，选择失败率最低的作为降级
	return s.selectFallbackChannel(activeChannels, failedChannels, isResponses)
}

// findPromotedChannel 查找处于促销期的渠道
func (s *ChannelScheduler) findPromotedChannel(activeChannels []ChannelInfo, isResponses bool) *ChannelInfo {
	for i := range activeChannels {
		ch := &activeChannels[i]
		if ch.Status != "active" {
			continue
		}
		upstream := s.getUpstreamByIndex(ch.Index, isResponses)
		if upstream != nil {
			if config.IsChannelInPromotion(upstream) {
				log.Printf("[Scheduler-Promotion] 找到促销渠道: [%d] %s (promotionUntil: %v)", ch.Index, upstream.Name, upstream.PromotionUntil)
				return ch
			}
		}
	}
	return nil
}

// selectFallbackChannel 选择降级渠道（失败率最低的）
func (s *ChannelScheduler) selectFallbackChannel(
	activeChannels []ChannelInfo,
	failedChannels map[int]bool,
	isResponses bool,
) (*SelectionResult, error) {
	metricsManager := s.getMetricsManager(isResponses)
	var bestChannel *ChannelInfo
	var bestUpstream *config.UpstreamConfig
	bestFailureRate := float64(2) // 初始化为不可能的值

	for i := range activeChannels {
		ch := &activeChannels[i]
		if failedChannels[ch.Index] {
			continue
		}
		// 跳过非 active 状态的渠道
		if ch.Status != "active" {
			continue
		}

		upstream := s.getUpstreamByIndex(ch.Index, isResponses)
		if upstream == nil || len(upstream.APIKeys) == 0 {
			continue
		}

		failureRate := metricsManager.CalculateChannelFailureRate(upstream.BaseURL, upstream.APIKeys)
		if failureRate < bestFailureRate {
			bestFailureRate = failureRate
			bestChannel = ch
			bestUpstream = upstream
		}
	}

	if bestChannel != nil && bestUpstream != nil {
		log.Printf("[Scheduler-Fallback] 警告: 降级选择渠道: [%d] %s (失败率: %.1f%%)",
			bestChannel.Index, bestUpstream.Name, bestFailureRate*100)
		return &SelectionResult{
			Upstream:     bestUpstream,
			ChannelIndex: bestChannel.Index,
			Reason:       "fallback",
		}, nil
	}

	return nil, fmt.Errorf("所有渠道都不可用")
}

// ChannelInfo 渠道信息（用于排序）
type ChannelInfo struct {
	Index    int
	Name     string
	Priority int
	Status   string
}

// getActiveChannels 获取活跃渠道列表（按优先级排序）
func (s *ChannelScheduler) getActiveChannels(isResponses bool) []ChannelInfo {
	cfg := s.configManager.GetConfig()

	var upstreams []config.UpstreamConfig
	if isResponses {
		upstreams = cfg.ResponsesUpstream
	} else {
		upstreams = cfg.Upstream
	}

	// 筛选活跃渠道
	var activeChannels []ChannelInfo
	for i, upstream := range upstreams {
		status := upstream.Status
		if status == "" {
			status = "active" // 默认为活跃
		}

		// 只选择 active 状态的渠道（suspended 也算在活跃序列中，但会被健康检查过滤）
		if status != "disabled" {
			priority := upstream.Priority
			if priority == 0 {
				priority = i // 默认优先级为索引
			}

			activeChannels = append(activeChannels, ChannelInfo{
				Index:    i,
				Name:     upstream.Name,
				Priority: priority,
				Status:   status,
			})
		}
	}

	// 按优先级排序（数字越小优先级越高）
	sort.Slice(activeChannels, func(i, j int) bool {
		return activeChannels[i].Priority < activeChannels[j].Priority
	})

	return activeChannels
}

// getUpstreamByIndex 根据索引获取上游配置
// 注意：返回的是副本，避免指向 slice 元素的指针在 slice 重分配后失效
func (s *ChannelScheduler) getUpstreamByIndex(index int, isResponses bool) *config.UpstreamConfig {
	cfg := s.configManager.GetConfig()

	var upstreams []config.UpstreamConfig
	if isResponses {
		upstreams = cfg.ResponsesUpstream
	} else {
		upstreams = cfg.Upstream
	}

	if index >= 0 && index < len(upstreams) {
		// 返回副本，避免返回指向 slice 元素的指针
		upstream := upstreams[index]
		return &upstream
	}
	return nil
}

// RecordSuccess 记录渠道成功（使用 baseURL + apiKey）
func (s *ChannelScheduler) RecordSuccess(baseURL, apiKey string, isResponses bool) {
	s.getMetricsManager(isResponses).RecordSuccess(baseURL, apiKey)
}

// RecordSuccessWithUsage 记录渠道成功（带 Usage 数据）
func (s *ChannelScheduler) RecordSuccessWithUsage(baseURL, apiKey string, usage *types.Usage, isResponses bool) {
	s.getMetricsManager(isResponses).RecordSuccessWithUsage(baseURL, apiKey, usage)
}

// RecordFailure 记录渠道失败（使用 baseURL + apiKey）
func (s *ChannelScheduler) RecordFailure(baseURL, apiKey string, isResponses bool) {
	s.getMetricsManager(isResponses).RecordFailure(baseURL, apiKey)
}

// SetTraceAffinity 设置 Trace 亲和
func (s *ChannelScheduler) SetTraceAffinity(userID string, channelIndex int) {
	if userID != "" {
		s.traceAffinity.SetPreferredChannel(userID, channelIndex)
	}
}

// UpdateTraceAffinity 更新 Trace 亲和时间（续期）
func (s *ChannelScheduler) UpdateTraceAffinity(userID string) {
	if userID != "" {
		s.traceAffinity.UpdateLastUsed(userID)
	}
}

// GetMessagesMetricsManager 获取 Messages 渠道指标管理器
func (s *ChannelScheduler) GetMessagesMetricsManager() *metrics.MetricsManager {
	return s.messagesMetricsManager
}

// GetResponsesMetricsManager 获取 Responses 渠道指标管理器
func (s *ChannelScheduler) GetResponsesMetricsManager() *metrics.MetricsManager {
	return s.responsesMetricsManager
}

// GetTraceAffinityManager 获取 Trace 亲和性管理器
func (s *ChannelScheduler) GetTraceAffinityManager() *session.TraceAffinityManager {
	return s.traceAffinity
}

// ResetChannelMetrics 重置渠道所有 Key 的指标（用于恢复熔断）
func (s *ChannelScheduler) ResetChannelMetrics(channelIndex int, isResponses bool) {
	upstream := s.getUpstreamByIndex(channelIndex, isResponses)
	if upstream == nil {
		return
	}
	metricsManager := s.getMetricsManager(isResponses)
	for _, apiKey := range upstream.APIKeys {
		metricsManager.ResetKey(upstream.BaseURL, apiKey)
	}
	log.Printf("[Scheduler-Reset] 渠道 [%d] %s 的所有 Key 指标已重置", channelIndex, upstream.Name)
}

// ResetKeyMetrics 重置单个 Key 的指标
func (s *ChannelScheduler) ResetKeyMetrics(baseURL, apiKey string, isResponses bool) {
	s.getMetricsManager(isResponses).ResetKey(baseURL, apiKey)
}

// GetActiveChannelCount 获取活跃渠道数量
func (s *ChannelScheduler) GetActiveChannelCount(isResponses bool) int {
	return len(s.getActiveChannels(isResponses))
}

// IsMultiChannelMode 判断是否为多渠道模式
func (s *ChannelScheduler) IsMultiChannelMode(isResponses bool) bool {
	return s.GetActiveChannelCount(isResponses) > 1
}

// maskUserID 掩码 user_id（保护隐私）
func maskUserID(userID string) string {
	if len(userID) <= 16 {
		return "***"
	}
	return userID[:8] + "***" + userID[len(userID)-4:]
}

// GetSortedURLsForChannel 获取渠道排序后的 URL 列表（非阻塞，立即返回）
// 返回按动态排序的 URL 结果列表，包含原始索引用于指标记录
func (s *ChannelScheduler) GetSortedURLsForChannel(
	channelIndex int,
	urls []string,
) []warmup.URLLatencyResult {
	if s.urlManager == nil || len(urls) <= 1 {
		// 无 URL 管理器或单 URL，返回默认结果
		results := make([]warmup.URLLatencyResult, len(urls))
		for i, url := range urls {
			results[i] = warmup.URLLatencyResult{
				URL:         url,
				OriginalIdx: i,
				Success:     true,
			}
		}
		return results
	}
	return s.urlManager.GetSortedURLs(channelIndex, urls)
}

// MarkURLSuccess 标记 URL 成功
func (s *ChannelScheduler) MarkURLSuccess(channelIndex int, url string) {
	if s.urlManager != nil {
		s.urlManager.MarkSuccess(channelIndex, url)
	}
}

// MarkURLFailure 标记 URL 失败，触发动态排序
func (s *ChannelScheduler) MarkURLFailure(channelIndex int, url string) {
	if s.urlManager != nil {
		s.urlManager.MarkFailure(channelIndex, url)
	}
}

// InvalidateURLCache 使渠道 URL 状态失效
func (s *ChannelScheduler) InvalidateURLCache(channelIndex int) {
	if s.urlManager != nil {
		s.urlManager.InvalidateChannel(channelIndex)
	}
}

// GetURLManagerStats 获取 URL 管理器统计
func (s *ChannelScheduler) GetURLManagerStats() map[string]interface{} {
	if s.urlManager != nil {
		return s.urlManager.GetStats()
	}
	return nil
}
