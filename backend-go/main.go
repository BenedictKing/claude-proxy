package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/yourusername/claude-proxy/internal/config"
	"github.com/yourusername/claude-proxy/internal/handlers"
	"github.com/yourusername/claude-proxy/internal/middleware"
)

//go:embed frontend/dist/*
var frontendFS embed.FS

func main() {
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("没有找到 .env 文件，使用环境变量或默认值")
	}

	// 初始化配置管理器
	envCfg := config.NewEnvConfig()
	cfgManager, err := config.NewConfigManager(".config/config.json")
	if err != nil {
		log.Fatalf("初始化配置管理器失败: %v", err)
	}

	// 设置 Gin 模式
	if envCfg.NodeEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建路由器
	r := gin.Default()

	// 配置 CORS
	r.Use(middleware.CORSMiddleware(envCfg))

	// Web UI 访问控制中间件
	r.Use(middleware.WebAuthMiddleware(envCfg, cfgManager))

	// 健康检查端点
	r.GET(envCfg.HealthCheckPath, handlers.HealthCheck(envCfg, cfgManager))

	// 配置重载端点
	r.POST("/admin/config/reload", handlers.ReloadConfig(cfgManager))

	// 开发信息端点
	if envCfg.NodeEnv == "development" {
		r.GET("/admin/dev/info", handlers.DevInfo(envCfg, cfgManager))
	}

	// Web 管理界面 API 路由
	apiGroup := r.Group("/api")
	{
		apiGroup.GET("/upstreams", handlers.GetUpstreams(cfgManager))
		apiGroup.POST("/upstreams", handlers.AddUpstream(cfgManager))
		apiGroup.PUT("/upstreams/:id", handlers.UpdateUpstream(cfgManager))
		apiGroup.DELETE("/upstreams/:id", handlers.DeleteUpstream(cfgManager))
		apiGroup.POST("/upstreams/:id/keys", handlers.AddApiKey(cfgManager))
		apiGroup.DELETE("/upstreams/:id/keys", handlers.DeleteApiKey(cfgManager))
		apiGroup.POST("/upstreams/:id/use", handlers.SetCurrentUpstream(cfgManager))
		apiGroup.GET("/config", handlers.GetConfig(cfgManager))
		apiGroup.PUT("/config", handlers.UpdateConfig(cfgManager))
	}

	// 代理端点 - 统一入口
	r.POST("/v1/messages", handlers.ProxyHandler(envCfg, cfgManager))

	// 静态文件服务 (嵌入的前端)
	if envCfg.EnableWebUI {
		handlers.ServeFrontend(r, frontendFS)
	} else {
		// 纯 API 模式
		r.GET("/", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"name":    "Claude API Proxy",
				"mode":    "API Only",
				"version": "1.0.0",
				"endpoints": gin.H{
					"health": envCfg.HealthCheckPath,
					"proxy":  "/v1/messages",
					"config": "/admin/config/reload",
				},
				"message": "Web界面已禁用，此服务器运行在纯API模式下",
			})
		})
	}

	// 启动服务器
	addr := fmt.Sprintf(":%d", envCfg.Port)
	fmt.Printf("\n🚀 Claude API代理服务器已启动\n")
	fmt.Printf("📍 本地地址: http://localhost:%d\n", envCfg.Port)
	fmt.Printf("🌐 管理界面: http://localhost:%d\n", envCfg.Port)
	fmt.Printf("📋 统一入口: POST /v1/messages\n")
	fmt.Printf("💚 健康检查: GET %s\n", envCfg.HealthCheckPath)
	fmt.Printf("📊 环境: %s\n\n", envCfg.NodeEnv)

	if err := r.Run(addr); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
