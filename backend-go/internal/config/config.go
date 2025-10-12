package config

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
	"strings"

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
}

// Config 配置结构
type Config struct {
	Upstream        []UpstreamConfig `json:"upstream"`
	CurrentUpstream int              `json:"currentUpstream"`
	LoadBalance     string           `json:"loadBalance"` // round-robin, random, failover
}

// FailedKey 失败密钥记录
type FailedKey struct {
	Timestamp    time.Time
	FailureCount int
}

// ConfigManager 配置管理器
type ConfigManager struct {
	mu                sync.RWMutex
	config            Config
	configFile        string
	requestCount      int
	watcher           *fsnotify.Watcher
	failedKeysCache   map[string]*FailedKey
	keyRecoveryTime   time.Duration
	maxFailureCount   int
}

const (
	maxBackups        = 10
	keyRecoveryTime   = 5 * time.Minute
	maxFailureCount   = 3
)

// NewConfigManager 创建配置管理器
func NewConfigManager(configFile string) (*ConfigManager, error) {
	cm := &ConfigManager{
		configFile:       configFile,
		failedKeysCache:  make(map[string]*FailedKey),
		keyRecoveryTime:  keyRecoveryTime,
		maxFailureCount:  maxFailureCount,
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
				},
			},
			CurrentUpstream: 0,
			LoadBalance:     "round-robin",
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

	return nil
}

// saveConfigLocked 保存配置（已加锁）
func (cm *ConfigManager) saveConfigLocked(config Config) error {
	// 备份当前配置
	cm.backupConfig()

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

// GetNextAPIKey 获取下一个 API 密钥
func (cm *ConfigManager) GetNextAPIKey(upstream *UpstreamConfig, failedKeys map[string]bool) (string, error) {
	if len(upstream.APIKeys) == 0 {
		return "", fmt.Errorf("上游 %s 没有可用的API密钥", upstream.Name)
	}

	// 获取可用密钥列表
	availableKeys := []string{}
	for _, key := range upstream.APIKeys {
		if !failedKeys[key] && !cm.isKeyFailed(key) {
			availableKeys = append(availableKeys, key)
		}
	}

	if len(availableKeys) == 0 {
		return "", fmt.Errorf("上游 %s 的所有API密钥都暂时不可用", upstream.Name)
	}

	// 根据负载均衡策略选择密钥
	switch cm.config.LoadBalance {
	case "round-robin":
		cm.mu.Lock()
		cm.requestCount++
		index := (cm.requestCount - 1) % len(availableKeys)
		cm.mu.Unlock()
		return availableKeys[index], nil

	case "random":
		index := rand.Intn(len(availableKeys))
		return availableKeys[index], nil

	case "failover":
		fallthrough
	default:
		return availableKeys[0], nil
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

	// 调整当前上游索引
	if cm.config.CurrentUpstream >= len(cm.config.Upstream) {
		if len(cm.config.Upstream) > 0 {
			cm.config.CurrentUpstream = len(cm.config.Upstream) - 1
		} else {
			cm.config.CurrentUpstream = 0
		}
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

// SetLoadBalance 设置负载均衡策略
func (cm *ConfigManager) SetLoadBalance(strategy string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 验证策略
	if strategy != "round-robin" && strategy != "random" && strategy != "failover" {
		return fmt.Errorf("无效的负载均衡策略: %s", strategy)
	}

	cm.config.LoadBalance = strategy

	if err := cm.saveConfigLocked(cm.config); err != nil {
		return err
	}

	log.Printf("已设置负载均衡策略: %s", strategy)
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

// RedirectModel 模型重定向
func RedirectModel(model string, upstream *UpstreamConfig) string {
	if upstream.ModelMapping == nil {
		return model
	}

	// 直接匹配
	if mapped, ok := upstream.ModelMapping[model]; ok {
		return mapped
	}

	// 模糊匹配 (与TS版本行为保持一致)
	for sourceModel, targetModel := range upstream.ModelMapping {
		if strings.Contains(model, sourceModel) || strings.Contains(sourceModel, model) {
			return targetModel
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
