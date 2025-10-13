package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BenedictKing/claude-proxy/internal/config"
)

// CORSMiddleware CORS 中间件
func CORSMiddleware(envCfg *config.EnvConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// 开发环境允许所有 localhost 源
		if envCfg.IsDevelopment() {
			if origin != "" && strings.Contains(origin, "localhost") {
				c.Header("Access-Control-Allow-Origin", origin)
			}
		} else {
			// 生产环境使用配置的源
			c.Header("Access-Control-Allow-Origin", envCfg.CORSOrigin)
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key")
		c.Header("Access-Control-Allow-Credentials", "true")

		// 处理预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(200)
			return
		}

		c.Next()
	}
}
