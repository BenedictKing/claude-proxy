package config

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// UpstreamConfig 上游配置
type UpstreamConfig struct {
	BaseURL            string            `json:"baseUrl"`
	APIKeys            []string          `json:"apiKeys"`
	ServiceType        string            `json:"serviceType"` // gemini, openai, openaiold, claude
	Name               string            `json:"name,omitempty"`
	Description        string            `json:"description,omitempty"`
	Website            string            `json:"website,omitempty"`
	InsecureSkipVerify bool              `json:"insecureSkipVerify,omitempty"`
	ModelMapping       map[string]string `json:"modelMapping,omitempty"`
	// 多渠道调度相关字段
	Priority       int        `json:"priority"`                 // 渠道优先级（数字越小优先级越高，默认按索引）
	Status         string     `json:"status"`                   // 渠道状态：active（正常）, suspended（暂停）, disabled（备用池）
	PromotionUntil *time.Time `json:"promotionUntil,omitempty"` // 促销期截止时间，在此期间内优先使用此渠道（忽略trace亲和）
}

// UpstreamUpdate 用于部分更新 UpstreamConfig
type UpstreamUpdate struct {
	Name               *string           `json:"name"`
	ServiceType        *string           `json:"serviceType"`
	BaseURL            *string           `json:"baseUrl"`
	APIKeys            []string          `json:"apiKeys"`
	Description        *string           `json:"description"`
	Website            *string           `json:"website"`
	InsecureSkipVerify *bool             `json:"insecureSkipVerify"`
	ModelMapping       map[string]string `json:"modelMapping"`
	// 多渠道调度相关字段
	Priority       *int       `json:"priority"`
	Status         *string    `json:"status"`
	PromotionUntil *time.Time `json:"promotionUntil"`
}

// Config 配置结构
type Config struct {
	Upstream        []UpstreamConfig `json:"upstream"`
	CurrentUpstream int              `json:"currentUpstream,omitempty"` // 已废弃：旧格式兼容用
	LoadBalance     string           `json:"loadBalance"`               // round-robin, random, failover

	// Responses 接口专用配置（独立于 /v1/messages）
	ResponsesUpstream        []UpstreamConfig `json:"responsesUpstream"`
	CurrentResponsesUpstream int              `json:"currentResponsesUpstream,omitempty"` // 已废弃：旧格式兼容用
	ResponsesLoadBalance     string           `json:"responsesLoadBalance"`
}

// FailedKey 失败密钥记录
type FailedKey struct {
	Timestamp    time.Time
	FailureCount int
}

// ConfigManager 配置管理器
type ConfigManager struct {
	mu                    sync.RWMutex
	config                Config
	configFile            string
	requestCount          int
	responsesRequestCount int
	watcher               *fsnotify.Watcher
	failedKeysCache       map[string]*FailedKey
	keyRecoveryTime       time.Duration
	maxFailureCount       int
}

const (
	maxBackups      = 10
	keyRecoveryTime = 5 * time.Minute
	maxFailureCount = 3
)

// NewConfigManager 创建配置管理器
func NewConfigManager(configFile string) (*ConfigManager, error) {
	cm := &ConfigManager{
		configFile:      configFile,
		failedKeysCache: make(map[string]*FailedKey),
		keyRecoveryTime: keyRecoveryTime,
		maxFailureCount: maxFailureCount,
	}

	// 加载配置
	if err := cm.loadConfig(); err != nil {
		return nil, err
	}

	// 启动文件监听
	if err := cm.startWatcher(); err != nil {
		log.Printf("启动配置文件监听失败: %v", err)
	}

	// 启动定期清理
	go cm.cleanupExpiredFailures()

	return cm, nil
}

// loadConfig 加载配置
func (cm *ConfigManager) loadConfig() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 如果配置文件不存在，创建默认配置
	if _, err := os.Stat(cm.configFile); os.IsNotExist(err) {
		defaultConfig := Config{
			Upstream: []UpstreamConfig{
				{
					Name:        "Gemini",
					BaseURL:     "https://generativelanguage.googleapis.com/v1beta",
					APIKeys:     []string{},
					ServiceType: "gemini",
					Status:      "active",
				},
			},
			CurrentUpstream:          0,
			LoadBalance:              "failover",
			ResponsesUpstream:        []UpstreamConfig{},
			CurrentResponsesUpstream: 0,
			ResponsesLoadBalance:     "failover",
		}

		// 确保目录存在
		if err := os.MkdirAll(filepath.Dir(cm.configFile), 0755); err != nil {
			return err
		}

		return cm.saveConfigLocked(defaultConfig)
	}

	// 读取配置文件
	data, err := os.ReadFile(cm.configFile)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &cm.config); err != nil {
		return err
	}

	// 兼容旧配置：如果 ResponsesLoadBalance 为空则回退到主配置
	if cm.config.LoadBalance == "" {
		cm.config.LoadBalance = "failover"
	}
	if cm.config.ResponsesLoadBalance == "" {
		cm.config.ResponsesLoadBalance = cm.config.LoadBalance
	}

	// 兼容旧格式：检测是否需要迁移（旧格式无 status 字段或 status 为空）
	needMigrationUpstream := false
	needMigrationResponses := false

	// 检查 Messages 渠道是否需要迁移
	if len(cm.config.Upstream) > 0 {
		hasStatusField := false
		for _, up := range cm.config.Upstream {
			if up.Status != "" {
				hasStatusField = true
				break
			}
		}
		if !hasStatusField {
			needMigrationUpstream = true
		}
	}

	// 检查 Responses 渠道是否需要迁移
	if len(cm.config.ResponsesUpstream) > 0 {
		hasStatusField := false
		for _, up := range cm.config.ResponsesUpstream {
			if up.Status != "" {
				hasStatusField = true
				break
			}
		}
		if !hasStatusField {
			needMigrationResponses = true
		}
	}

	if needMigrationUpstream || needMigrationResponses {
		log.Printf("检测到旧格式配置，正在迁移到新格式...")

		// Messages 渠道迁移
		if needMigrationUpstream && len(cm.config.Upstream) > 0 {
			currentIdx := cm.config.CurrentUpstream
			if currentIdx < 0 || currentIdx >= len(cm.config.Upstream) {
				currentIdx = 0
			}
			// 设置所有渠道的 status 状态
			for i := range cm.config.Upstream {
				if i == currentIdx {
					cm.config.Upstream[i].Status = "active"
				} else {
					cm.config.Upstream[i].Status = "disabled"
				}
			}
			log.Printf("Messages 渠道 [%d] %s 已设置为 active，其他 %d 个渠道已设为 disabled",
				currentIdx, cm.config.Upstream[currentIdx].Name, len(cm.config.Upstream)-1)
		}

		// Responses 渠道迁移
		if needMigrationResponses && len(cm.config.ResponsesUpstream) > 0 {
			currentIdx := cm.config.CurrentResponsesUpstream
			if currentIdx < 0 || currentIdx >= len(cm.config.ResponsesUpstream) {
				currentIdx = 0
			}
			// 设置所有渠道的 status 状态
			for i := range cm.config.ResponsesUpstream {
				if i == currentIdx {
					cm.config.ResponsesUpstream[i].Status = "active"
				} else {
					cm.config.ResponsesUpstream[i].Status = "disabled"
				}
			}
			log.Printf("Responses 渠道 [%d] %s 已设置为 active，其他 %d 个渠道已设为 disabled",
				currentIdx, cm.config.ResponsesUpstream[currentIdx].Name, len(cm.config.ResponsesUpstream)-1)
		}

		// 保存迁移后的配置（saveConfigLocked 会自动清理废弃字段）
		if err := cm.saveConfigLocked(cm.config); err != nil {
			log.Printf("保存迁移后的配置失败: %v", err)
			return err
		}
		log.Printf("配置迁移完成")
	}

	// 自检：没有配置 key 的渠道自动暂停
	needSave := cm.validateChannelKeys()
	if needSave {
		if err := cm.saveConfigLocked(cm.config); err != nil {
			log.Printf("保存自检后的配置失败: %v", err)
			return err
		}
	}

	return nil
}

// validateChannelKeys 自检渠道密钥配置
// 没有配置 API key 的渠道，即使状态为 active 也应暂停
// 返回 true 表示有配置被修改，需要保存
func (cm *ConfigManager) validateChannelKeys() bool {
	modified := false

	// 检查 Messages 渠道
	for i := range cm.config.Upstream {
		upstream := &cm.config.Upstream[i]
		status := upstream.Status
		if status == "" {
			status = "active"
		}

		// 如果是 active 状态但没有配置 key，自动设为 suspended
		if status == "active" && len(upstream.APIKeys) == 0 {
			upstream.Status = "suspended"
			modified = true
			log.Printf("⚠️ [自检] Messages 渠道 [%d] %s 没有配置 API key，已自动暂停", i, upstream.Name)
		}
	}

	// 检查 Responses 渠道
	for i := range cm.config.ResponsesUpstream {
		upstream := &cm.config.ResponsesUpstream[i]
		status := upstream.Status
		if status == "" {
			status = "active"
		}

		// 如果是 active 状态但没有配置 key，自动设为 suspended
		if status == "active" && len(upstream.APIKeys) == 0 {
			upstream.Status = "suspended"
			modified = true
			log.Printf("⚠️ [自检] Responses 渠道 [%d] %s 没有配置 API key，已自动暂停", i, upstream.Name)
		}
	}

	return modified
}

// saveConfigLocked 保存配置（已加锁）
func (cm *ConfigManager) saveConfigLocked(config Config) error {
	// 备份当前配置
	cm.backupConfig()

	// 清理已废弃字段，确保不会被序列化到 JSON
	config.CurrentUpstream = 0
	config.CurrentResponsesUpstream = 0

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	cm.config = config
	return os.WriteFile(cm.configFile, data, 0644)
}

// SaveConfig 保存配置
func (cm *ConfigManager) SaveConfig() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.saveConfigLocked(cm.config)
}

// backupConfig 备份配置
func (cm *ConfigManager) backupConfig() {
	if _, err := os.Stat(cm.configFile); os.IsNotExist(err) {
		return
	}

	backupDir := filepath.Join(filepath.Dir(cm.configFile), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Printf("创建备份目录失败: %v", err)
		return
	}

	// 读取当前配置
	data, err := os.ReadFile(cm.configFile)
	if err != nil {
		log.Printf("读取配置文件失败: %v", err)
		return
	}

	// 创建备份文件
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("config-%s.json", timestamp))
	if err := os.WriteFile(backupFile, data, 0644); err != nil {
		log.Printf("写入备份文件失败: %v", err)
		return
	}

	// 清理旧备份
	cm.cleanupOldBackups(backupDir)
}

// cleanupOldBackups 清理旧备份
func (cm *ConfigManager) cleanupOldBackups(backupDir string) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	if len(entries) <= maxBackups {
		return
	}

	// 删除最旧的备份
	for i := 0; i < len(entries)-maxBackups; i++ {
		os.Remove(filepath.Join(backupDir, entries[i].Name()))
	}
}

// startWatcher 启动文件监听
func (cm *ConfigManager) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	cm.watcher = watcher

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Printf("检测到配置文件变化，重载配置...")
					if err := cm.loadConfig(); err != nil {
						log.Printf("配置重载失败: %v", err)
					} else {
						log.Printf("配置已重载")
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("文件监听错误: %v", err)
			}
		}
	}()

	return watcher.Add(cm.configFile)
}

// GetConfig 获取配置
func (cm *ConfigManager) GetConfig() Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

// GetCurrentUpstream 获取当前上游配置
func (cm *ConfigManager) GetCurrentUpstream() (*UpstreamConfig, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.config.Upstream) == 0 {
		return nil, fmt.Errorf("未配置任何上游渠道")
	}

	if cm.config.CurrentUpstream >= len(cm.config.Upstream) {
		return nil, fmt.Errorf("当前渠道索引 %d 无效", cm.config.CurrentUpstream)
	}

	upstream := cm.config.Upstream[cm.config.CurrentUpstream]
	return &upstream, nil
}

// GetNextAPIKey 获取下一个 API 密钥（Messages 负载均衡）
func (cm *ConfigManager) GetNextAPIKey(upstream *UpstreamConfig, failedKeys map[string]bool) (string, error) {
	return cm.getNextAPIKeyWithStrategy(upstream, failedKeys, cm.config.LoadBalance, &cm.requestCount)
}

// GetNextResponsesAPIKey 获取下一个 API 密钥（Responses 负载均衡）
func (cm *ConfigManager) GetNextResponsesAPIKey(upstream *UpstreamConfig, failedKeys map[string]bool) (string, error) {
	return cm.getNextAPIKeyWithStrategy(upstream, failedKeys, cm.config.ResponsesLoadBalance, &cm.responsesRequestCount)
}

func (cm *ConfigManager) getNextAPIKeyWithStrategy(upstream *UpstreamConfig, failedKeys map[string]bool, strategy string, requestCounter *int) (string, error) {
	if len(upstream.APIKeys) == 0 {
		return "", fmt.Errorf("上游 %s 没有可用的API密钥", upstream.Name)
	}

	// 综合考虑临时失败密钥和内存中的失败密钥
	availableKeys := []string{}
	for _, key := range upstream.APIKeys {
		if !failedKeys[key] && !cm.isKeyFailed(key) {
			availableKeys = append(availableKeys, key)
		}
	}

	if len(availableKeys) == 0 {
		// 如果所有密钥都失效,检查是否有可以恢复的密钥
		allFailedKeys := []string{}
		for _, key := range upstream.APIKeys {
			if failedKeys[key] || cm.isKeyFailed(key) {
				allFailedKeys = append(allFailedKeys, key)
			}
		}

		if len(allFailedKeys) == len(upstream.APIKeys) {
			// 如果所有密钥都在内存失败缓存中,尝试选择失败时间最早的密钥
			var oldestFailedKey string
			oldestTime := time.Now()

			cm.mu.RLock()
			for _, key := range upstream.APIKeys {
				if !failedKeys[key] { // 排除本次请求已经尝试过的密钥
					if failure, exists := cm.failedKeysCache[key]; exists {
						if failure.Timestamp.Before(oldestTime) {
							oldestTime = failure.Timestamp
							oldestFailedKey = key
						}
					}
				}
			}
			cm.mu.RUnlock()

			if oldestFailedKey != "" {
				log.Printf("⚠️ 所有密钥都失效,尝试最早失败的密钥: %s", maskAPIKey(oldestFailedKey))
				return oldestFailedKey, nil
			}
		}

		return "", fmt.Errorf("上游 %s 的所有API密钥都暂时不可用", upstream.Name)
	}

	// 根据负载均衡策略选择密钥
	switch strategy {
	case "round-robin":
		cm.mu.Lock()
		*requestCounter++
		index := (*requestCounter - 1) % len(availableKeys)
		selectedKey := availableKeys[index]
		cm.mu.Unlock()
		log.Printf("轮询选择密钥 %s (%d/%d)", maskAPIKey(selectedKey), index+1, len(availableKeys))
		return selectedKey, nil

	case "random":
		index := rand.Intn(len(availableKeys))
		selectedKey := availableKeys[index]
		log.Printf("随机选择密钥 %s (%d/%d)", maskAPIKey(selectedKey), index+1, len(availableKeys))
		return selectedKey, nil

	case "failover":
		fallthrough
	default:
		selectedKey := availableKeys[0]
		// 获取该密钥在原始列表中的索引
		keyIndex := 0
		for i, key := range upstream.APIKeys {
			if key == selectedKey {
				keyIndex = i + 1
				break
			}
		}
		log.Printf("故障转移选择密钥 %s (%d/%d)", maskAPIKey(selectedKey), keyIndex, len(upstream.APIKeys))
		return selectedKey, nil
	}
}

// MarkKeyAsFailed 标记密钥失败
func (cm *ConfigManager) MarkKeyAsFailed(apiKey string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if failure, exists := cm.failedKeysCache[apiKey]; exists {
		failure.FailureCount++
		failure.Timestamp = time.Now()
	} else {
		cm.failedKeysCache[apiKey] = &FailedKey{
			Timestamp:    time.Now(),
			FailureCount: 1,
		}
	}

	failure := cm.failedKeysCache[apiKey]
	recoveryTime := cm.keyRecoveryTime
	if failure.FailureCount > cm.maxFailureCount {
		recoveryTime = cm.keyRecoveryTime * 2
	}

	log.Printf("标记API密钥失败: %s (失败次数: %d, 恢复时间: %v)",
		maskAPIKey(apiKey), failure.FailureCount, recoveryTime)
}

// isKeyFailed 检查密钥是否失败
func (cm *ConfigManager) isKeyFailed(apiKey string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	failure, exists := cm.failedKeysCache[apiKey]
	if !exists {
		return false
	}

	recoveryTime := cm.keyRecoveryTime
	if failure.FailureCount > cm.maxFailureCount {
		recoveryTime = cm.keyRecoveryTime * 2
	}

	return time.Since(failure.Timestamp) < recoveryTime
}

// cleanupExpiredFailures 清理过期的失败记录
func (cm *ConfigManager) cleanupExpiredFailures() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cm.mu.Lock()
		now := time.Now()
		for key, failure := range cm.failedKeysCache {
			recoveryTime := cm.keyRecoveryTime
			if failure.FailureCount > cm.maxFailureCount {
				recoveryTime = cm.keyRecoveryTime * 2
			}

			if now.Sub(failure.Timestamp) > recoveryTime {
				delete(cm.failedKeysCache, key)
				log.Printf("API密钥 %s 已从失败列表中恢复", maskAPIKey(key))
			}
		}
		cm.mu.Unlock()
	}
}

// clearFailedKeysForUpstream 清理指定渠道的所有失败 key 记录
// 当渠道被删除时调用，避免内存泄漏和冷却状态残留
func (cm *ConfigManager) clearFailedKeysForUpstream(upstream *UpstreamConfig) {
	for _, key := range upstream.APIKeys {
		if _, exists := cm.failedKeysCache[key]; exists {
			delete(cm.failedKeysCache, key)
			log.Printf("已清理被删除渠道 %s 的失败密钥记录: %s", upstream.Name, maskAPIKey(key))
		}
	}
}

// SetCurrentUpstream 设置当前上游
func (cm *ConfigManager) SetCurrentUpstream(index int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.Upstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	cm.config.CurrentUpstream = index

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已切换到上游: [%d] %s", index, cm.config.Upstream[index].Name)
	return nil
}

// AddUpstream 添加上游
func (cm *ConfigManager) AddUpstream(upstream UpstreamConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.config.Upstream = append(cm.config.Upstream, upstream)

	// 如果这是第一个渠道,自动设为当前
	if len(cm.config.Upstream) == 1 {
		cm.config.CurrentUpstream = 0
		log.Printf("首个Messages渠道已自动设为当前: %s", upstream.Name)
	}

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已添加上游: %s", upstream.Name)
	return nil
}

// UpdateUpstream 更新上游
func (cm *ConfigManager) UpdateUpstream(index int, updates UpstreamUpdate) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.Upstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	upstream := &cm.config.Upstream[index]

	if updates.Name != nil {
		upstream.Name = *updates.Name
	}
	if updates.BaseURL != nil {
		upstream.BaseURL = *updates.BaseURL
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
		upstream.APIKeys = updates.APIKeys
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
		return err
	}

	log.Printf("已更新上游: [%d] %s", index, cm.config.Upstream[index].Name)
	return nil
}

// RemoveUpstream 删除上游
func (cm *ConfigManager) RemoveUpstream(index int) (*UpstreamConfig, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.Upstream) {
		return nil, fmt.Errorf("无效的上游索引: %d", index)
	}

	removed := cm.config.Upstream[index]
	cm.config.Upstream = append(cm.config.Upstream[:index], cm.config.Upstream[index+1:]...)

	// 清理被删除渠道的失败 key 冷却记录
	cm.clearFailedKeysForUpstream(&removed)

	// 调整当前上游索引
	if cm.config.CurrentUpstream >= len(cm.config.Upstream) {
		if len(cm.config.Upstream) > 0 {
			cm.config.CurrentUpstream = len(cm.config.Upstream) - 1
		} else {
			cm.config.CurrentUpstream = 0
		}
	}

	// 如果删除后只剩一个渠道,自动设为当前
	if len(cm.config.Upstream) == 1 {
		cm.config.CurrentUpstream = 0
		log.Printf("唯一Messages渠道已自动设为当前: %s", cm.config.Upstream[0].Name)
	}

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return nil, err
	}

	log.Printf("已删除上游: %s", removed.Name)
	return &removed, nil
}

// AddAPIKey 添加API密钥
func (cm *ConfigManager) AddAPIKey(index int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.Upstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	// 检查密钥是否已存在
	for _, key := range cm.config.Upstream[index].APIKeys {
		if key == apiKey {
			return fmt.Errorf("API密钥已存在")
		}
	}

	cm.config.Upstream[index].APIKeys = append(cm.config.Upstream[index].APIKeys, apiKey)

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已添加API密钥到上游 [%d] %s", index, cm.config.Upstream[index].Name)
	return nil
}

// RemoveAPIKey 删除API密钥
func (cm *ConfigManager) RemoveAPIKey(index int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.Upstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	// 查找并删除密钥
	keys := cm.config.Upstream[index].APIKeys
	found := false
	for i, key := range keys {
		if key == apiKey {
			cm.config.Upstream[index].APIKeys = append(keys[:i], keys[i+1:]...)
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

	log.Printf("已从上游 [%d] %s 删除API密钥", index, cm.config.Upstream[index].Name)
	return nil
}

// SetLoadBalance 设置 Messages 负载均衡策略
func (cm *ConfigManager) SetLoadBalance(strategy string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if err := validateLoadBalanceStrategy(strategy); err != nil {
		return err
	}

	cm.config.LoadBalance = strategy

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已设置负载均衡策略: %s", strategy)
	return nil
}

// SetResponsesLoadBalance 设置 Responses 负载均衡策略
func (cm *ConfigManager) SetResponsesLoadBalance(strategy string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if err := validateLoadBalanceStrategy(strategy); err != nil {
		return err
	}

	cm.config.ResponsesLoadBalance = strategy

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已设置 Responses 负载均衡策略: %s", strategy)
	return nil
}

func validateLoadBalanceStrategy(strategy string) error {
	if strategy != "round-robin" && strategy != "random" && strategy != "failover" {
		return fmt.Errorf("无效的负载均衡策略: %s", strategy)
	}
	return nil
}

// DeprioritizeAPIKey 降低API密钥优先级
func (cm *ConfigManager) DeprioritizeAPIKey(apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	upstream := &cm.config.Upstream[cm.config.CurrentUpstream]

	// 查找密钥索引
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

	// 移动到末尾
	upstream.APIKeys = append(upstream.APIKeys[:index], upstream.APIKeys[index+1:]...)
	upstream.APIKeys = append(upstream.APIKeys, apiKey)

	log.Printf("已将API密钥移动到末尾以降低优先级: %s", maskAPIKey(apiKey))
	return cm.saveConfigLocked(cm.config)
}

// MoveAPIKeyToTop 将指定渠道的 API 密钥移到最前面
func (cm *ConfigManager) MoveAPIKeyToTop(upstreamIndex int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if upstreamIndex < 0 || upstreamIndex >= len(cm.config.Upstream) {
		return fmt.Errorf("无效的上游索引: %d", upstreamIndex)
	}

	upstream := &cm.config.Upstream[upstreamIndex]
	index := -1
	for i, key := range upstream.APIKeys {
		if key == apiKey {
			index = i
			break
		}
	}

	if index <= 0 {
		return nil // 已经在最前面或未找到
	}

	// 移动到开头
	upstream.APIKeys = append([]string{apiKey}, append(upstream.APIKeys[:index], upstream.APIKeys[index+1:]...)...)
	return cm.saveConfigLocked(cm.config)
}

// MoveAPIKeyToBottom 将指定渠道的 API 密钥移到最后面
func (cm *ConfigManager) MoveAPIKeyToBottom(upstreamIndex int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if upstreamIndex < 0 || upstreamIndex >= len(cm.config.Upstream) {
		return fmt.Errorf("无效的上游索引: %d", upstreamIndex)
	}

	upstream := &cm.config.Upstream[upstreamIndex]
	index := -1
	for i, key := range upstream.APIKeys {
		if key == apiKey {
			index = i
			break
		}
	}

	if index == -1 || index == len(upstream.APIKeys)-1 {
		return nil // 已经在最后面或未找到
	}

	// 移动到末尾
	upstream.APIKeys = append(upstream.APIKeys[:index], upstream.APIKeys[index+1:]...)
	upstream.APIKeys = append(upstream.APIKeys, apiKey)
	return cm.saveConfigLocked(cm.config)
}

// MoveResponsesAPIKeyToTop 将指定 Responses 渠道的 API 密钥移到最前面
func (cm *ConfigManager) MoveResponsesAPIKeyToTop(upstreamIndex int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if upstreamIndex < 0 || upstreamIndex >= len(cm.config.ResponsesUpstream) {
		return fmt.Errorf("无效的上游索引: %d", upstreamIndex)
	}

	upstream := &cm.config.ResponsesUpstream[upstreamIndex]
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

// MoveResponsesAPIKeyToBottom 将指定 Responses 渠道的 API 密钥移到最后面
func (cm *ConfigManager) MoveResponsesAPIKeyToBottom(upstreamIndex int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if upstreamIndex < 0 || upstreamIndex >= len(cm.config.ResponsesUpstream) {
		return fmt.Errorf("无效的上游索引: %d", upstreamIndex)
	}

	upstream := &cm.config.ResponsesUpstream[upstreamIndex]
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

// RedirectModel 模型重定向
func RedirectModel(model string, upstream *UpstreamConfig) string {
	if upstream.ModelMapping == nil || len(upstream.ModelMapping) == 0 {
		return model
	}

	// 直接匹配（精确匹配优先）
	if mapped, ok := upstream.ModelMapping[model]; ok {
		return mapped
	}

	// 模糊匹配：按源模型长度从长到短排序，确保最长匹配优先
	// 例如：同时配置 "codex" 和 "gpt-5.1-codex" 时，"gpt-5.1-codex" 应该先匹配
	type mapping struct {
		source string
		target string
	}
	mappings := make([]mapping, 0, len(upstream.ModelMapping))
	for source, target := range upstream.ModelMapping {
		mappings = append(mappings, mapping{source, target})
	}
	// 按源模型长度降序排序
	sort.Slice(mappings, func(i, j int) bool {
		return len(mappings[i].source) > len(mappings[j].source)
	})

	// 按排序后的顺序进行模糊匹配
	for _, m := range mappings {
		if strings.Contains(model, m.source) || strings.Contains(m.source, model) {
			return m.target
		}
	}

	return model
}

// maskAPIKey 掩码API密钥（与 TS 版本保持一致）
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}

	length := len(key)
	if length <= 10 {
		// 短密钥：保留前3位和后2位
		if length <= 5 {
			return "***"
		}
		return key[:3] + "***" + key[length-2:]
	}

	// 长密钥：保留前8位和后5位
	return key[:8] + "***" + key[length-5:]
}

// Close 关闭配置管理器
func (cm *ConfigManager) Close() error {
	if cm.watcher != nil {
		return cm.watcher.Close()
	}
	return nil
}

// ============== Responses 接口专用方法 ==============

// GetCurrentResponsesUpstream 获取当前 Responses 上游配置
func (cm *ConfigManager) GetCurrentResponsesUpstream() (*UpstreamConfig, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.config.ResponsesUpstream) == 0 {
		return nil, fmt.Errorf("未配置任何 Responses 渠道")
	}

	if cm.config.CurrentResponsesUpstream >= len(cm.config.ResponsesUpstream) {
		return nil, fmt.Errorf("当前 Responses 渠道索引 %d 无效", cm.config.CurrentResponsesUpstream)
	}

	upstream := cm.config.ResponsesUpstream[cm.config.CurrentResponsesUpstream]
	return &upstream, nil
}

// SetCurrentResponsesUpstream 设置当前 Responses 上游
func (cm *ConfigManager) SetCurrentResponsesUpstream(index int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.ResponsesUpstream) {
		return fmt.Errorf("无效的 Responses 上游索引: %d", index)
	}

	cm.config.CurrentResponsesUpstream = index

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已切换到 Responses 上游: [%d] %s", index, cm.config.ResponsesUpstream[index].Name)
	return nil
}

// AddResponsesUpstream 添加 Responses 上游
func (cm *ConfigManager) AddResponsesUpstream(upstream UpstreamConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.config.ResponsesUpstream = append(cm.config.ResponsesUpstream, upstream)

	// 如果这是第一个渠道,自动设为当前
	if len(cm.config.ResponsesUpstream) == 1 {
		cm.config.CurrentResponsesUpstream = 0
		log.Printf("首个Responses渠道已自动设为当前: %s", upstream.Name)
	}

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已添加 Responses 上游: %s", upstream.Name)
	return nil
}

// UpdateResponsesUpstream 更新 Responses 上游
func (cm *ConfigManager) UpdateResponsesUpstream(index int, updates UpstreamUpdate) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.ResponsesUpstream) {
		return fmt.Errorf("无效的 Responses 上游索引: %d", index)
	}

	upstream := &cm.config.ResponsesUpstream[index]

	if updates.Name != nil {
		upstream.Name = *updates.Name
	}
	if updates.BaseURL != nil {
		upstream.BaseURL = *updates.BaseURL
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
		upstream.APIKeys = updates.APIKeys
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
		return err
	}

	log.Printf("已更新 Responses 上游: [%d] %s", index, cm.config.ResponsesUpstream[index].Name)
	return nil
}

// RemoveResponsesUpstream 删除 Responses 上游
func (cm *ConfigManager) RemoveResponsesUpstream(index int) (*UpstreamConfig, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.ResponsesUpstream) {
		return nil, fmt.Errorf("无效的 Responses 上游索引: %d", index)
	}

	removed := cm.config.ResponsesUpstream[index]
	cm.config.ResponsesUpstream = append(cm.config.ResponsesUpstream[:index], cm.config.ResponsesUpstream[index+1:]...)

	// 清理被删除渠道的失败 key 冷却记录
	cm.clearFailedKeysForUpstream(&removed)

	// 调整当前上游索引
	if cm.config.CurrentResponsesUpstream >= len(cm.config.ResponsesUpstream) {
		if len(cm.config.ResponsesUpstream) > 0 {
			cm.config.CurrentResponsesUpstream = len(cm.config.ResponsesUpstream) - 1
		} else {
			cm.config.CurrentResponsesUpstream = 0
		}
	}

	// 如果删除后只剩一个渠道,自动设为当前
	if len(cm.config.ResponsesUpstream) == 1 {
		cm.config.CurrentResponsesUpstream = 0
		log.Printf("唯一Responses渠道已自动设为当前: %s", cm.config.ResponsesUpstream[0].Name)
	}

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return nil, err
	}

	log.Printf("已删除 Responses 上游: %s", removed.Name)
	return &removed, nil
}

// AddResponsesAPIKey 添加 Responses 上游的 API 密钥
func (cm *ConfigManager) AddResponsesAPIKey(index int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.ResponsesUpstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	// 检查密钥是否已存在
	for _, key := range cm.config.ResponsesUpstream[index].APIKeys {
		if key == apiKey {
			return fmt.Errorf("API密钥已存在")
		}
	}

	cm.config.ResponsesUpstream[index].APIKeys = append(cm.config.ResponsesUpstream[index].APIKeys, apiKey)

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已添加API密钥到 Responses 上游 [%d] %s", index, cm.config.ResponsesUpstream[index].Name)
	return nil
}

// RemoveResponsesAPIKey 删除 Responses 上游的 API 密钥
func (cm *ConfigManager) RemoveResponsesAPIKey(index int, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.ResponsesUpstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	// 查找并删除密钥
	keys := cm.config.ResponsesUpstream[index].APIKeys
	found := false
	for i, key := range keys {
		if key == apiKey {
			cm.config.ResponsesUpstream[index].APIKeys = append(keys[:i], keys[i+1:]...)
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

	log.Printf("已从 Responses 上游 [%d] %s 删除API密钥", index, cm.config.ResponsesUpstream[index].Name)
	return nil
}

// ============== 多渠道调度相关方法 ==============

// ReorderUpstreams 重新排序 Messages 渠道优先级
// order 是渠道索引数组，按新的优先级顺序排列（只更新传入的渠道，支持部分排序）
func (cm *ConfigManager) ReorderUpstreams(order []int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if len(order) == 0 {
		return fmt.Errorf("排序数组不能为空")
	}

	// 验证所有索引都有效且不重复
	seen := make(map[int]bool)
	for _, idx := range order {
		if idx < 0 || idx >= len(cm.config.Upstream) {
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
		cm.config.Upstream[idx].Priority = i + 1
	}

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已更新 Messages 渠道优先级顺序 (%d 个渠道)", len(order))
	return nil
}

// ReorderResponsesUpstreams 重新排序 Responses 渠道优先级
// order 是渠道索引数组，按新的优先级顺序排列（只更新传入的渠道，支持部分排序）
func (cm *ConfigManager) ReorderResponsesUpstreams(order []int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if len(order) == 0 {
		return fmt.Errorf("排序数组不能为空")
	}

	seen := make(map[int]bool)
	for _, idx := range order {
		if idx < 0 || idx >= len(cm.config.ResponsesUpstream) {
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
		cm.config.ResponsesUpstream[idx].Priority = i + 1
	}

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已更新 Responses 渠道优先级顺序 (%d 个渠道)", len(order))
	return nil
}

// SetChannelStatus 设置 Messages 渠道状态
func (cm *ConfigManager) SetChannelStatus(index int, status string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.Upstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	// 状态值转为小写，支持大小写不敏感
	status = strings.ToLower(status)
	if status != "active" && status != "suspended" && status != "disabled" {
		return fmt.Errorf("无效的状态: %s (允许值: active, suspended, disabled)", status)
	}

	cm.config.Upstream[index].Status = status

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已设置渠道 [%d] %s 状态为: %s", index, cm.config.Upstream[index].Name, status)
	return nil
}

// SetResponsesChannelStatus 设置 Responses 渠道状态
func (cm *ConfigManager) SetResponsesChannelStatus(index int, status string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.ResponsesUpstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	// 状态值转为小写，支持大小写不敏感
	status = strings.ToLower(status)
	if status != "active" && status != "suspended" && status != "disabled" {
		return fmt.Errorf("无效的状态: %s (允许值: active, suspended, disabled)", status)
	}

	cm.config.ResponsesUpstream[index].Status = status

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已设置 Responses 渠道 [%d] %s 状态为: %s", index, cm.config.ResponsesUpstream[index].Name, status)
	return nil
}

// GetChannelStatus 获取渠道状态（带默认值处理）
func GetChannelStatus(upstream *UpstreamConfig) string {
	if upstream.Status == "" {
		return "active"
	}
	return upstream.Status
}

// GetChannelPriority 获取渠道优先级（带默认值处理）
func GetChannelPriority(upstream *UpstreamConfig, index int) int {
	if upstream.Priority == 0 {
		return index
	}
	return upstream.Priority
}

// SetChannelPromotion 设置渠道促销期
// duration 为促销持续时间，传入 0 表示清除促销期
func (cm *ConfigManager) SetChannelPromotion(index int, duration time.Duration) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.Upstream) {
		return fmt.Errorf("无效的上游索引: %d", index)
	}

	if duration <= 0 {
		cm.config.Upstream[index].PromotionUntil = nil
		log.Printf("已清除渠道 [%d] %s 的促销期", index, cm.config.Upstream[index].Name)
	} else {
		promotionEnd := time.Now().Add(duration)
		cm.config.Upstream[index].PromotionUntil = &promotionEnd
		log.Printf("🎉 已设置渠道 [%d] %s 进入促销期，截止: %s", index, cm.config.Upstream[index].Name, promotionEnd.Format(time.RFC3339))
	}

	return cm.saveConfigLocked(cm.config)
}

// SetResponsesChannelPromotion 设置 Responses 渠道促销期
func (cm *ConfigManager) SetResponsesChannelPromotion(index int, duration time.Duration) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index < 0 || index >= len(cm.config.ResponsesUpstream) {
		return fmt.Errorf("无效的 Responses 上游索引: %d", index)
	}

	if duration <= 0 {
		cm.config.ResponsesUpstream[index].PromotionUntil = nil
		log.Printf("已清除 Responses 渠道 [%d] %s 的促销期", index, cm.config.ResponsesUpstream[index].Name)
	} else {
		promotionEnd := time.Now().Add(duration)
		cm.config.ResponsesUpstream[index].PromotionUntil = &promotionEnd
		log.Printf("🎉 已设置 Responses 渠道 [%d] %s 进入促销期，截止: %s", index, cm.config.ResponsesUpstream[index].Name, promotionEnd.Format(time.RFC3339))
	}

	return cm.saveConfigLocked(cm.config)
}

// IsChannelInPromotion 检查渠道是否处于促销期
func IsChannelInPromotion(upstream *UpstreamConfig) bool {
	if upstream.PromotionUntil == nil {
		return false
	}
	return time.Now().Before(*upstream.PromotionUntil)
}

// GetPromotedChannel 获取当前处于促销期的渠道索引（返回优先级最高的）
func (cm *ConfigManager) GetPromotedChannel() (int, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for i, upstream := range cm.config.Upstream {
		if IsChannelInPromotion(&upstream) && GetChannelStatus(&upstream) == "active" {
			return i, true
		}
	}
	return -1, false
}

// GetPromotedResponsesChannel 获取当前处于促销期的 Responses 渠道索引
func (cm *ConfigManager) GetPromotedResponsesChannel() (int, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for i, upstream := range cm.config.ResponsesUpstream {
		if IsChannelInPromotion(&upstream) && GetChannelStatus(&upstream) == "active" {
			return i, true
		}
	}
	return -1, false
}
