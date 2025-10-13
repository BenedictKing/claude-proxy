package handlers

import (
	"fmt" // 新增
	"net/http" // 新增
	"strconv"
	"strings"
	"sync" // 新增
	"time" // 新增

	"github.com/gin-gonic/gin"
	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/httpclient" // 新增
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
				"latency":            nil,
				"status":             "unknown",
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

		if err := cfgManager.AddUpstream(upstream); err != nil {
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

		var updates config.UpstreamUpdate
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request body"})
			return
		}

		if err := cfgManager.UpdateUpstream(id, updates); err != nil {
			if strings.Contains(err.Error(), "无效的上游索引") {
				c.JSON(404, gin.H{"error": "Upstream not found"})
			} else {
				c.JSON(500, gin.H{"error": "Failed to save config"})
			}
			return
		}

		cfg := cfgManager.GetConfig()
		c.JSON(200, gin.H{
			"message":  "上游已更新",
			"upstream": cfg.Upstream[id],
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

		removed, err := cfgManager.RemoveUpstream(id)
		if err != nil {
			if strings.Contains(err.Error(), "无效的上游索引") {
				c.JSON(404, gin.H{"error": "Upstream not found"})
			} else {
				c.JSON(500, gin.H{"error": "Failed to save config"})
			}
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

		if err := cfgManager.AddAPIKey(id, req.APIKey); err != nil {
			if strings.Contains(err.Error(), "无效的上游索引") {
				c.JSON(404, gin.H{"error": "Upstream not found"})
			} else if strings.Contains(err.Error(), "API密钥已存在") {
				c.JSON(400, gin.H{"error": "API密钥已存在"})
			} else {
				c.JSON(500, gin.H{"error": "Failed to save config"})
			}
			return
		}

		c.JSON(200, gin.H{
			"message": "API密钥已添加",
			"success": true,
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

		if err := cfgManager.RemoveAPIKey(id, apiKey); err != nil {
			if strings.Contains(err.Error(), "无效的上游索引") {
				c.JSON(404, gin.H{"error": "Upstream not found"})
			} else if strings.Contains(err.Error(), "API密钥不存在") {
				c.JSON(404, gin.H{"error": "API key not found"})
			} else {
				c.JSON(500, gin.H{"error": "Failed to save config"})
			}
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

		if err := cfgManager.SetCurrentUpstream(id); err != nil {
			if strings.Contains(err.Error(), "无效的上游索引") {
				c.JSON(404, gin.H{"error": "Upstream not found"})
			} else {
				c.JSON(500, gin.H{"error": "Failed to save config"})
			}
			return
		}

		c.JSON(200, gin.H{
			"message": "当前上游已切换",
			"current": id,
			"success": true,
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

		if err := cfgManager.SetLoadBalance(req.Strategy); err != nil {
			if strings.Contains(err.Error(), "无效的负载均衡策略") {
				c.JSON(400, gin.H{"error": err.Error()})
			} else {
				c.JSON(500, gin.H{"error": "Failed to save config"})
			}
			return
		}

		c.JSON(200, gin.H{
			"message":  "负载均衡策略已更新",
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
			return
		}

		config := cfgManager.GetConfig()
		if id < 0 || id >= len(config.Upstream) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Channel not found"})
			return
		}

		channel := config.Upstream[id]
		startTime := time.Now()

		testURL := strings.TrimSuffix(channel.BaseURL, "/")
		switch channel.ServiceType {
		case "openai", "openaiold", "gemini":
			testURL += "/models"
		}

		client := httpclient.GetManager().GetStandardClient(5*time.Second, channel.InsecureSkipVerify)
		req, err := http.NewRequest("HEAD", testURL, nil)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "status": "error", "error": "Failed to create request"})
			return
		}

		resp, err := client.Do(req)
		latency := time.Since(startTime).Milliseconds()

		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"latency": latency,
				"status":  "error",
				"error":   err.Error(),
			})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"latency": latency,
				"status":  "healthy",
			})
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"latency": latency,
				"status":  "unhealthy",
				"error":   fmt.Sprintf("Status code: %d", resp.StatusCode),
			})
		}
	}
}

// PingAllChannels Ping所有渠道
func PingAllChannels(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := cfgManager.GetConfig()
		results := make(chan gin.H)
		var wg sync.WaitGroup

		for i, channel := range cfg.Upstream {
			wg.Add(1)
			go func(id int, ch config.UpstreamConfig) {
				defer wg.Done()

				startTime := time.Now()
				testURL := strings.TrimSuffix(ch.BaseURL, "/")
				switch ch.ServiceType {
				case "openai", "openaiold", "gemini":
					testURL += "/models"
				}

				client := httpclient.GetManager().GetStandardClient(5*time.Second, ch.InsecureSkipVerify)
				req, err := http.NewRequest("HEAD", testURL, nil)
				if err != nil {
					results <- gin.H{"id": id, "name": ch.Name, "latency": 0, "status": "error", "error": "req_creation_failed"}
					return
				}

				resp, err := client.Do(req)
				latency := time.Since(startTime).Milliseconds()

				if err != nil {
					results <- gin.H{"id": id, "name": ch.Name, "latency": latency, "status": "error", "error": err.Error()}
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode >= 200 && resp.StatusCode < 400 {
					results <- gin.H{"id": id, "name": ch.Name, "latency": latency, "status": "healthy"}
				} else {
					results <- gin.H{"id": id, "name": ch.Name, "latency": latency, "status": "unhealthy"}
				}
			}(i, channel)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		var finalResults []gin.H
		for res := range results {
			finalResults = append(finalResults, res)
		}

		c.JSON(http.StatusOK, finalResults)
	}
}
