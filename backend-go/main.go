package main

import (
	"embed"
	"fmt"
	"log"

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

	// 设置版本信息到 handlers 包
	handlers.SetVersionInfo(Version, BuildTime, GitCommit)

	// 初始化配置管理器
	envCfg := config.NewEnvConfig()
	cfgManager, err := config.NewConfigManager(".config/config.json")
	if err != nil {
		log.Fatalf("初始化配置管理器失败: %v", err)
	}

	// 设置 Gin 模式
	if envCfg.IsProduction() {
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
	if envCfg.IsDevelopment() {
		r.GET("/admin/dev/info", handlers.DevInfo(envCfg, cfgManager))
	}

	// Web 管理界面 API 路由
	apiGroup := r.Group("/api")
	{
		// 渠道管理 (兼容前端 /api/channels 路由)
		apiGroup.GET("/channels", handlers.GetUpstreams(cfgManager))
		apiGroup.POST("/channels", handlers.AddUpstream(cfgManager))
		apiGroup.PUT("/channels/:id", handlers.UpdateUpstream(cfgManager))
		apiGroup.DELETE("/channels/:id", handlers.DeleteUpstream(cfgManager))
		apiGroup.POST("/channels/:id/keys", handlers.AddApiKey(cfgManager))
		apiGroup.DELETE("/channels/:id/keys/:apiKey", handlers.DeleteApiKey(cfgManager))
		apiGroup.POST("/channels/:id/current", handlers.SetCurrentUpstream(cfgManager))


		// 负载均衡
		apiGroup.PUT("/loadbalance", handlers.UpdateLoadBalance(cfgManager))

		// Ping测试
		apiGroup.GET("/ping/:id", handlers.PingChannel(cfgManager))
		apiGroup.GET("/ping", handlers.PingAllChannels(cfgManager))
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
	fmt.Printf("📌 版本: %s\n", Version)
	if BuildTime != "unknown" {
		fmt.Printf("🕐 构建时间: %s\n", BuildTime)
	}
	if GitCommit != "unknown" {
		fmt.Printf("🔖 Git提交: %s\n", GitCommit)
	}
	fmt.Printf("📍 本地地址: http://localhost:%d\n", envCfg.Port)
	fmt.Printf("🌐 管理界面: http://localhost:%d\n", envCfg.Port)
	fmt.Printf("📋 统一入口: POST /v1/messages\n")
	fmt.Printf("💚 健康检查: GET %s\n", envCfg.HealthCheckPath)
	fmt.Printf("📊 环境: %s\n\n", envCfg.Env)

	if err := r.Run(addr); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
