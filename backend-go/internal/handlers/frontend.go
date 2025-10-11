package handlers

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// ServeFrontend 提供前端静态文件服务
func ServeFrontend(r *gin.Engine, frontendFS embed.FS) {
	// 从嵌入的文件系统中提取 frontend/dist 子目录
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		// 如果提取失败，返回错误页面
		r.GET("/", func(c *gin.Context) {
			c.HTML(503, "", getErrorPage())
		})
		return
	}

	// 使用 Gin 的静态文件服务
	r.StaticFS("/assets", http.FS(distFS))

	// 处理所有其他路由（SPA 支持）
	r.NoRoute(func(c *gin.Context) {
		// 尝试读取 index.html
		indexContent, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			c.HTML(503, "", getErrorPage())
			return
		}

		c.Data(200, "text/html; charset=utf-8", indexContent)
	})

	// 根路径
	r.GET("/", func(c *gin.Context) {
		indexContent, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			c.HTML(503, "", getErrorPage())
			return
		}

		c.Data(200, "text/html; charset=utf-8", indexContent)
	})
}

// getErrorPage 获取错误页面
func getErrorPage() string {
	return `<!DOCTYPE html>
<html>
<head>
  <title>Claude Proxy - 配置错误</title>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    body { font-family: system-ui; padding: 40px; background: #f5f5f5; }
    .error { max-width: 600px; margin: 0 auto; background: white; padding: 40px; border-radius: 8px; }
    h1 { color: #dc3545; }
    code { background: #f8f9fa; padding: 2px 6px; border-radius: 3px; }
    pre { background: #f8f9fa; padding: 16px; border-radius: 4px; overflow-x: auto; }
  </style>
</head>
<body>
  <div class="error">
    <h1>❌ 前端资源未找到</h1>
    <p>无法找到前端构建文件。请执行以下步骤之一：</p>
    <h3>方案1: 重新构建(推荐)</h3>
    <pre>./build.sh</pre>
    <h3>方案2: 禁用Web界面</h3>
    <p>在 <code>.env</code> 文件中设置: <code>ENABLE_WEB_UI=false</code></p>
    <p>然后只使用API端点: <code>/v1/messages</code></p>
  </div>
</body>
</html>`
}
