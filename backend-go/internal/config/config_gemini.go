package config

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// ============== Gemini 渠道方法 ==============

// GetCurrentGeminiUpstream 获取当前 Gemini 上游配置
// 优先选择第一个 active 状态的渠道，若无则回退到第一个渠道
func (cm *ConfigManager) GetCurrentGeminiUpstream() (*UpstreamConfig, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.config.GeminiUpstream) == 0 {
		return nil, fmt.Errorf("未配置任何 Gemini 渠道")
	}

	// 优先选择第一个 active 状态的渠道
	for i := range cm.config.GeminiUpstream {
		status := cm.config.GeminiUpstream[i].Status
		if status == "" || status == "active" {
			return &cm.config.GeminiUpstream[i], nil
		}
	}

	// 没有 active 渠道，回退到第一个渠道
	return &cm.config.GeminiUpstream[0], nil
}

// AddGeminiUpstream 添加 Gemini 上游
func (cm *ConfigManager) AddGeminiUpstream(upstream UpstreamConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 新建渠道默认设为 active
	if upstream.Status == "" {
		upstream.Status = "active"
	}

	// 去重 API Keys 和 Base URLs
	upstream.APIKeys = deduplicateStrings(upstream.APIKeys)
	upstream.BaseURLs = deduplicateBaseURLs(upstream.BaseURLs)

	cm.config.GeminiUpstream = append(cm.config.GeminiUpstream, upstream)

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("[Config-Upstream] 已添加 Gemini 上游: %s", upstream.Name)
	return nil
}

// UpdateGeminiUpstream 更新 Gemini 上游
// 返回值：shouldResetMetrics 表示是否需要重置渠道指标（熔断状态）
func (cm *ConfigManager) UpdateGeminiUpstream(index int, updates UpstreamUpdate) (shouldResetMetrics bool, err error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.GeminiUpstream) {
		return false, fmt.Errorf("无效的 Gemini 上游索引: %d", index)
	}

	upstream := &cm.config.GeminiUpstream[index]

	if updates.Name != nil {
		upstream.Name = *updates.Name
	}
	if updates.BaseURL != nil {
		upstream.BaseURL = *updates.BaseURL
	}
	if updates.BaseURLs != nil {
		upstream.BaseURLs = deduplicateBaseURLs(updates.BaseURLs)
	}
	if updates.ServiceType != nil {
		upstream.ServiceType = *updates.ServiceType
	}
	if updates.Description != nil {
		upstream.Description = *updates.Description
	}
	if updates.Website != nil {
		upstream.Website = *updates.Website
	}
	if updates.APIKeys != nil {
		// 只有单 key 场景且 key 被更换时，才自动激活并重置熔断
		if len(upstream.APIKeys) == 1 && len(updates.APIKeys) == 1 &&
			upstream.APIKeys[0] != updates.APIKeys[0] {
			shouldResetMetrics = true
			if upstream.Status == "suspended" {
				upstream.Status = "active"
				log.Printf("[Config-Upstream] Gemini 渠道 [%d] %s 已从暂停状态自动激活（单 key 更换）", index, upstream.Name)
			}
		}
		upstream.APIKeys = deduplicateStrings(updates.APIKeys)
	}
	if updates.ModelMapping != nil {
		upstream.ModelMapping = updates.ModelMapping
	}
	if updates.InsecureSkipVerify != nil {
		upstream.InsecureSkipVerify = *updates.InsecureSkipVerify
	}
	if updates.Priority != nil {
		upstream.Priority = *updates.Priority
	}
	if updates.Status != nil {
		upstream.Status = *updates.Status
	}
	if updates.PromotionUntil != nil {
		upstream.PromotionUntil = updates.PromotionUntil
	}

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return false, err
	}

	log.Printf("[Config-Upstream] 已更新 Gemini 上游: [%d] %s", index, cm.config.GeminiUpstream[index].Name)
	return shouldResetMetrics, nil
}

// RemoveGeminiUpstream 删除 Gemini 上游
func (cm *ConfigManager) RemoveGeminiUpstream(index int) (*UpstreamConfig, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.GeminiUpstream) {
		return nil, fmt.Errorf("无效的 Gemini 上游索引: %d", index)
	}

	removed := cm.config.GeminiUpstream[index]
	cm.config.GeminiUpstream = append(cm.config.GeminiUpstream[:index], cm.config.GeminiUpstream[index+1:]...)

	// 清理被删除渠道的失败 key 冷却记录
	cm.clearFailedKeysForUpstream(&removed)

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return nil, err
	}

	log.Printf("[Config-Upstream] 已删除 Gemini 上游: %s", removed.Name)
	return &removed, nil
}

// AddGeminiAPIKey 添加 Gemini 上游的 API 密钥
func (cm *ConfigManager) AddGeminiAPIKey(index int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.GeminiUpstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	// 检查密钥是否已存在
	for _, key := range cm.config.GeminiUpstream[index].APIKeys {
		if key == apiKey {
			return fmt.Errorf("API密钥已存在")
		}
	}

	cm.config.GeminiUpstream[index].APIKeys = append(cm.config.GeminiUpstream[index].APIKeys, apiKey)

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("[Config-Key] 已添加API密钥到 Gemini 上游 [%d] %s", index, cm.config.GeminiUpstream[index].Name)
	return nil
}

// RemoveGeminiAPIKey 删除 Gemini 上游的 API 密钥
func (cm *ConfigManager) RemoveGeminiAPIKey(index int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.GeminiUpstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	// 查找并删除密钥
	keys := cm.config.GeminiUpstream[index].APIKeys
	found := false
	for i, key := range keys {
		if key == apiKey {
			cm.config.GeminiUpstream[index].APIKeys = append(keys[:i], keys[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("API密钥不存在")
	}

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("[Config-Key] 已从 Gemini 上游 [%d] %s 删除API密钥", index, cm.config.GeminiUpstream[index].Name)
	return nil
}

// GetNextGeminiAPIKey 获取下一个 Gemini API 密钥（纯 failover 模式）
func (cm *ConfigManager) GetNextGeminiAPIKey(upstream *UpstreamConfig, failedKeys map[string]bool) (string, error) {
	return cm.GetNextAPIKey(upstream, failedKeys)
}

// MoveGeminiAPIKeyToTop 将指定 Gemini 渠道的 API 密钥移到最前面
func (cm *ConfigManager) MoveGeminiAPIKeyToTop(upstreamIndex int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if upstreamIndex < 0 || upstreamIndex >= len(cm.config.GeminiUpstream) {
		return fmt.Errorf("无效的上游索引: %d", upstreamIndex)
	}

	upstream := &cm.config.GeminiUpstream[upstreamIndex]
	index := -1
	for i, key := range upstream.APIKeys {
		if key == apiKey {
			index = i
			break
		}
	}

	if index <= 0 {
		return nil
	}

	upstream.APIKeys = append([]string{apiKey}, append(upstream.APIKeys[:index], upstream.APIKeys[index+1:]...)...)
	return cm.saveConfigLocked(cm.config)
}

// MoveGeminiAPIKeyToBottom 将指定 Gemini 渠道的 API 密钥移到最后面
func (cm *ConfigManager) MoveGeminiAPIKeyToBottom(upstreamIndex int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if upstreamIndex < 0 || upstreamIndex >= len(cm.config.GeminiUpstream) {
		return fmt.Errorf("无效的上游索引: %d", upstreamIndex)
	}

	upstream := &cm.config.GeminiUpstream[upstreamIndex]
	index := -1
	for i, key := range upstream.APIKeys {
		if key == apiKey {
			index = i
			break
		}
	}

	if index == -1 || index == len(upstream.APIKeys)-1 {
		return nil
	}

	upstream.APIKeys = append(upstream.APIKeys[:index], upstream.APIKeys[index+1:]...)
	upstream.APIKeys = append(upstream.APIKeys, apiKey)
	return cm.saveConfigLocked(cm.config)
}

// ReorderGeminiUpstreams 重新排序 Gemini 渠道优先级
// order 是渠道索引数组，按新的优先级顺序排列（只更新传入的渠道，支持部分排序）
func (cm *ConfigManager) ReorderGeminiUpstreams(order []int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if len(order) == 0 {
		return fmt.Errorf("排序数组不能为空")
	}

	seen := make(map[int]bool)
	for _, idx := range order {
		if idx < 0 || idx >= len(cm.config.GeminiUpstream) {
			return fmt.Errorf("无效的渠道索引: %d", idx)
		}
		if seen[idx] {
			return fmt.Errorf("重复的渠道索引: %d", idx)
		}
		seen[idx] = true
	}

	// 更新传入渠道的优先级（未传入的渠道保持原优先级不变）
	// 注意：priority 从 1 开始，避免 omitempty 吞掉 0 值
	for i, idx := range order {
		cm.config.GeminiUpstream[idx].Priority = i + 1
	}

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("[Config-Reorder] 已更新 Gemini 渠道优先级顺序 (%d 个渠道)", len(order))
	return nil
}

// SetGeminiChannelStatus 设置 Gemini 渠道状态
func (cm *ConfigManager) SetGeminiChannelStatus(index int, status string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.GeminiUpstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	// 状态值转为小写，支持大小写不敏感
	status = strings.ToLower(status)
	if status != "active" && status != "suspended" && status != "disabled" {
		return fmt.Errorf("无效的状态: %s (允许值: active, suspended, disabled)", status)
	}

	cm.config.GeminiUpstream[index].Status = status

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("[Config-Status] 已设置 Gemini 渠道 [%d] %s 状态为: %s", index, cm.config.GeminiUpstream[index].Name, status)
	return nil
}

// SetGeminiChannelPromotion 设置 Gemini 渠道促销期
func (cm *ConfigManager) SetGeminiChannelPromotion(index int, duration time.Duration) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.GeminiUpstream) {
		return fmt.Errorf("无效的 Gemini 上游索引: %d", index)
	}

	if duration <= 0 {
		cm.config.GeminiUpstream[index].PromotionUntil = nil
		log.Printf("[Config-Promotion] 已清除 Gemini 渠道 [%d] %s 的促销期", index, cm.config.GeminiUpstream[index].Name)
	} else {
		// 清除其他渠道的促销期（同一时间只允许一个促销渠道）
		for i := range cm.config.GeminiUpstream {
			if i != index && cm.config.GeminiUpstream[i].PromotionUntil != nil {
				cm.config.GeminiUpstream[i].PromotionUntil = nil
			}
		}
		promotionEnd := time.Now().Add(duration)
		cm.config.GeminiUpstream[index].PromotionUntil = &promotionEnd
		log.Printf("[Config-Promotion] 已设置 Gemini 渠道 [%d] %s 进入促销期，截止: %s", index, cm.config.GeminiUpstream[index].Name, promotionEnd.Format(time.RFC3339))
	}

	return cm.saveConfigLocked(cm.config)
}

// GetPromotedGeminiChannel 获取当前处于促销期的 Gemini 渠道索引
func (cm *ConfigManager) GetPromotedGeminiChannel() (int, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for i, upstream := range cm.config.GeminiUpstream {
		if IsChannelInPromotion(&upstream) && GetChannelStatus(&upstream) == "active" {
			return i, true
		}
	}
	return -1, false
}
