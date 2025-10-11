package middleware

import (
	"fmt"
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

		// 检查访问密钥
		providedKey := getAPIKey(c)
		expectedKey := envCfg.ProxyAccessKey

		if providedKey == "" || providedKey != expectedKey {
			log.Printf("🔒 Web界面访问被拒绝 - IP: %s, Path: %s", c.ClientIP(), path)

			// 返回认证页面
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(401, getAuthPage())
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

// getAuthPage 获取认证页面 HTML
func getAuthPage() string {
	return `<!DOCTYPE html>
<html>
<head>
  <title>Claude Proxy - 访问验证</title>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    body { font-family: system-ui, -apple-system, sans-serif; background: #f5f5f5; margin: 0; padding: 40px; }
    .container { max-width: 400px; margin: 100px auto; background: white; padding: 40px; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
    h1 { color: #333; margin-bottom: 20px; }
    input { width: 100%; padding: 12px; border: 1px solid #ddd; border-radius: 4px; margin-bottom: 20px; box-sizing: border-box; }
    button { width: 100%; padding: 12px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer; }
    button:hover { background: #0056b3; }
    .error { color: #dc3545; margin-bottom: 20px; font-size: 14px; }
  </style>
</head>
<body>
  <div class="container">
    <h1>🔐 Claude Proxy 管理界面</h1>
    <div class="error">请输入访问密钥以继续</div>
    <form onsubmit="handleAuth(event)">
      <input type="password" id="apiKey" placeholder="访问密钥 (PROXY_ACCESS_KEY)" required>
      <button type="submit">访问管理界面</button>
    </form>
  </div>
  <script>
    function handleAuth(e) {
      e.preventDefault();
      const key = document.getElementById('apiKey').value;
      const url = new URL(window.location);
      url.searchParams.set('key', key);
      window.location.href = url.toString();
    }
  </script>
</body>
</html>`
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
