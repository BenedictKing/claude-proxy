package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/claude-proxy/internal/config"
)

// HealthCheck 健康检查处理器
func HealthCheck(envCfg *config.EnvConfig, cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		config := cfgManager.GetConfig()

		healthData := gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
			"uptime":    time.Since(startTime).Seconds(),
			"mode":      envCfg.NodeEnv,
			"config": gin.H{
				"upstreamCount":   len(config.Upstream),
				"currentUpstream": config.CurrentUpstream,
				"loadBalance":     config.LoadBalance,
			},
		}

		c.JSON(200, healthData)
	}
}

// ReloadConfig 配置重载处理器
func ReloadConfig(cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := cfgManager.SaveConfig(); err != nil {
			c.JSON(500, gin.H{
				"status":    "error",
				"message":   "配置重载失败",
				"error":     err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}

		config := cfgManager.GetConfig()
		c.JSON(200, gin.H{
			"status":    "success",
			"message":   "配置已重载",
			"timestamp": time.Now().Format(time.RFC3339),
			"config": gin.H{
				"upstreamCount":   len(config.Upstream),
				"currentUpstream": config.CurrentUpstream,
				"loadBalance":     config.LoadBalance,
			},
		})
	}
}

// DevInfo 开发信息处理器
func DevInfo(envCfg *config.EnvConfig, cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":      "development",
			"timestamp":   time.Now().Format(time.RFC3339),
			"config":      cfgManager.GetConfig(),
			"environment": envCfg,
		})
	}
}

var startTime = time.Now()