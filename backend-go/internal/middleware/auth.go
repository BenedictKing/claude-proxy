package middleware

import (
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/claude-proxy/internal/config"
)

// WebAuthMiddleware Web 访问控制中间件
func WebAuthMiddleware(envCfg *config.EnvConfig, cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 公开端点直接放行
		if path == envCfg.HealthCheckPath ||
			path == "/admin/config/reload" ||
			(envCfg.IsDevelopment() && path == "/admin/dev/info") {
			c.Next()
			return
		}

		// 静态资源文件直接放行
		if isStaticResource(path) {
			c.Next()
			return
		}

		// API 代理端点后续处理
		if strings.HasPrefix(path, "/v1/") {
			c.Next()
			return
		}

		// 如果禁用了 Web UI，返回 404
		if !envCfg.EnableWebUI {
			c.JSON(404, gin.H{
				"error":   "Web界面已禁用",
				"message": "此服务器运行在纯API模式下，请通过API端点访问服务",
			})
			c.Abort()
			return
		}

		// 对于根路径和页面请求，直接服务前端应用，让前端处理认证
		// 前端会自动处理认证流程
		if path == "/" || path == "/index.html" || !strings.Contains(path, ".") {
			// 直接让请求通过，由静态文件服务器处理
			c.Next()
			return
		}

		// 检查访问密钥（仅对API请求）
		providedKey := getAPIKey(c)
		expectedKey := envCfg.ProxyAccessKey

		if providedKey == "" || providedKey != expectedKey {
			log.Printf("🔒 访问被拒绝 - IP: %s, Path: %s", c.ClientIP(), path)

			// 对于API请求返回401
			c.JSON(401, gin.H{
				"error": "Unauthorized",
				"message": "Invalid or missing access key",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// isStaticResource 判断是否为静态资源
func isStaticResource(path string) bool {
	staticExtensions := []string{
		"/assets/", ".css", ".js", ".ico", ".png", ".jpg",
		".gif", ".svg", ".woff", ".woff2", ".ttf", ".eot",
	}

	for _, ext := range staticExtensions {
		if strings.HasPrefix(path, ext) || strings.HasSuffix(path, ext) {
			return true
		}
	}

	return false
}

// getAPIKey 获取 API 密钥
func getAPIKey(c *gin.Context) string {
	// 从 header 获取
	if key := c.GetHeader("x-api-key"); key != "" {
		return key
	}

	if auth := c.GetHeader("Authorization"); auth != "" {
		// 移除 Bearer 前缀
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// 从查询参数获取
	if key := c.Query("key"); key != "" {
		return key
	}

	return ""
}


// ProxyAuthMiddleware 代理访问控制中间件
func ProxyAuthMiddleware(envCfg *config.EnvConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		providedKey := getAPIKey(c)
		expectedKey := envCfg.ProxyAccessKey

		if providedKey == "" || providedKey != expectedKey {
			if envCfg.ShouldLog("warn") {
				log.Printf("🔒 代理访问密钥验证失败 - IP: %s", c.ClientIP())
			}

			c.JSON(401, gin.H{
				"error": "Invalid proxy access key",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
