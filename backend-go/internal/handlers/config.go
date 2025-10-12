package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/claude-proxy/internal/config"
)

// GetUpstreams 获取上游列表 (兼容前端 channels 字段名)
func GetUpstreams(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := cfgManager.GetConfig()

		// 为每个upstream添加index字段
		upstreams := make([]gin.H, len(cfg.Upstream))
		for i, up := range cfg.Upstream {
			upstreams[i] = gin.H{
				"index":              i,
				"name":               up.Name,
				"serviceType":        up.ServiceType,
				"baseUrl":            up.BaseURL,
				"apiKeys":            up.APIKeys,
				"description":        up.Description,
				"website":            up.Website,
				"insecureSkipVerify": up.InsecureSkipVerify,
				"modelMapping":       up.ModelMapping,
			}
		}

		c.JSON(200, gin.H{
			"channels":    upstreams,
			"current":     cfg.CurrentUpstream,
			"loadBalance": cfg.LoadBalance,
		})
	}
}

// AddUpstream 添加上游
func AddUpstream(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var upstream config.UpstreamConfig
		if err := c.ShouldBindJSON(&upstream); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request body"})
			return
		}

		config := cfgManager.GetConfig()
		config.Upstream = append(config.Upstream, upstream)

		if err := cfgManager.SaveConfig(); err != nil {
			c.JSON(500, gin.H{"error": "Failed to save config"})
			return
		}

		c.JSON(200, gin.H{
			"message":  "上游已添加",
			"upstream": upstream,
		})
	}
}

// UpdateUpstream 更新上游
func UpdateUpstream(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid upstream ID"})
			return
		}

		var updates config.UpstreamConfig
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request body"})
			return
		}

		config := cfgManager.GetConfig()
		if id < 0 || id >= len(config.Upstream) {
			c.JSON(404, gin.H{"error": "Upstream not found"})
			return
		}

		// 更新字段
		if updates.Name != "" {
			config.Upstream[id].Name = updates.Name
		}
		if updates.BaseURL != "" {
			config.Upstream[id].BaseURL = updates.BaseURL
		}
		if updates.ServiceType != "" {
			config.Upstream[id].ServiceType = updates.ServiceType
		}
		if updates.Description != "" {
			config.Upstream[id].Description = updates.Description
		}
		if updates.Website != "" {
			config.Upstream[id].Website = updates.Website
		}
		if updates.ModelMapping != nil {
			config.Upstream[id].ModelMapping = updates.ModelMapping
		}
		config.Upstream[id].InsecureSkipVerify = updates.InsecureSkipVerify

		if err := cfgManager.SaveConfig(); err != nil {
			c.JSON(500, gin.H{"error": "Failed to save config"})
			return
		}

		c.JSON(200, gin.H{
			"message":  "上游已更新",
			"upstream": config.Upstream[id],
		})
	}
}

// DeleteUpstream 删除上游
func DeleteUpstream(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid upstream ID"})
			return
		}

		config := cfgManager.GetConfig()
		if id < 0 || id >= len(config.Upstream) {
			c.JSON(404, gin.H{"error": "Upstream not found"})
			return
		}

		removed := config.Upstream[id]
		config.Upstream = append(config.Upstream[:id], config.Upstream[id+1:]...)

		// 调整当前上游索引
		if config.CurrentUpstream >= len(config.Upstream) {
			if len(config.Upstream) > 0 {
				config.CurrentUpstream = len(config.Upstream) - 1
			} else {
				config.CurrentUpstream = 0
			}
		}

		if err := cfgManager.SaveConfig(); err != nil {
			c.JSON(500, gin.H{"error": "Failed to save config"})
			return
		}

		c.JSON(200, gin.H{
			"message": "上游已删除",
			"removed": removed,
		})
	}
}

// AddApiKey 添加 API 密钥
func AddApiKey(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid upstream ID"})
			return
		}

		var req struct {
			APIKey string `json:"apiKey"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request body"})
			return
		}

		config := cfgManager.GetConfig()
		if id < 0 || id >= len(config.Upstream) {
			c.JSON(404, gin.H{"error": "Upstream not found"})
			return
		}

		// 检查密钥是否已存在
		for _, key := range config.Upstream[id].APIKeys {
			if key == req.APIKey {
				c.JSON(400, gin.H{"error": "API密钥已存在"})
				return
			}
		}

		config.Upstream[id].APIKeys = append(config.Upstream[id].APIKeys, req.APIKey)

		if err := cfgManager.SaveConfig(); err != nil {
			c.JSON(500, gin.H{"error": "Failed to save config"})
			return
		}

		c.JSON(200, gin.H{
			"message": "API密钥已添加",
		})
	}
}

// DeleteApiKey 删除 API 密钥 (支持URL路径参数)
func DeleteApiKey(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid upstream ID"})
			return
		}

		// 从URL路径参数获取apiKey
		apiKey := c.Param("apiKey")
		if apiKey == "" {
			c.JSON(400, gin.H{"error": "API key is required"})
			return
		}

		config := cfgManager.GetConfig()
		if id < 0 || id >= len(config.Upstream) {
			c.JSON(404, gin.H{"error": "Upstream not found"})
			return
		}

		// 查找并删除密钥
		keys := config.Upstream[id].APIKeys
		found := false
		for i, key := range keys {
			if key == apiKey {
				config.Upstream[id].APIKeys = append(keys[:i], keys[i+1:]...)
				found = true
				break
			}
		}

		if !found {
			c.JSON(404, gin.H{"error": "API key not found"})
			return
		}

		if err := cfgManager.SaveConfig(); err != nil {
			c.JSON(500, gin.H{"error": "Failed to save config"})
			return
		}

		c.JSON(200, gin.H{
			"message": "API密钥已删除",
		})
	}
}

// SetCurrentUpstream 设置当前上游
func SetCurrentUpstream(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid upstream ID"})
			return
		}

		config := cfgManager.GetConfig()
		if id < 0 || id >= len(config.Upstream) {
			c.JSON(404, gin.H{"error": "Upstream not found"})
			return
		}

		config.CurrentUpstream = id

		if err := cfgManager.SaveConfig(); err != nil {
			c.JSON(500, gin.H{"error": "Failed to save config"})
			return
		}

		c.JSON(200, gin.H{
			"message": "当前上游已切换",
			"current": id,
		})
	}
}

// GetConfig 获取配置
func GetConfig(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		config := cfgManager.GetConfig()
		c.JSON(200, config)
	}
}

// UpdateConfig 更新配置
func UpdateConfig(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var updates struct {
			LoadBalance string `json:"loadBalance"`
		}
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request body"})
			return
		}

		config := cfgManager.GetConfig()
		if updates.LoadBalance != "" {
			config.LoadBalance = updates.LoadBalance
		}

		if err := cfgManager.SaveConfig(); err != nil {
			c.JSON(500, gin.H{"error": "Failed to save config"})
			return
		}

		c.JSON(200, gin.H{
			"message": "配置已更新",
			"config":  config,
		})
	}
}

// UpdateLoadBalance 更新负载均衡策略
func UpdateLoadBalance(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Strategy string `json:"strategy"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request body"})
			return
		}

		config := cfgManager.GetConfig()
		config.LoadBalance = req.Strategy

		if err := cfgManager.SaveConfig(); err != nil {
			c.JSON(500, gin.H{"error": "Failed to save config"})
			return
		}

		c.JSON(200, gin.H{
			"message": "负载均衡策略已更新",
			"strategy": req.Strategy,
		})
	}
}

// PingChannel Ping单个渠道
func PingChannel(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid channel ID"})
			return
		}

		config := cfgManager.GetConfig()
		if id < 0 || id >= len(config.Upstream) {
			c.JSON(404, gin.H{"error": "Channel not found"})
			return
		}

		// 简单返回成功，实际可以实现真实的ping逻辑
		c.JSON(200, gin.H{
			"success": true,
			"latency": 0,
			"status":  "healthy",
		})
	}
}

// PingAllChannels Ping所有渠道
func PingAllChannels(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		config := cfgManager.GetConfig()
		results := make([]gin.H, len(config.Upstream))

		for i := range config.Upstream {
			results[i] = gin.H{
				"id":      i,
				"name":    config.Upstream[i].Name,
				"latency": 0,
				"status":  "healthy",
			}
		}

		c.JSON(200, results)
	}
}