package handlers

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/claude-proxy/internal/config"
	"github.com/yourusername/claude-proxy/internal/middleware"
	"github.com/yourusername/claude-proxy/internal/providers"
	"github.com/yourusername/claude-proxy/internal/types"
)

// ProxyHandler 代理处理器
func ProxyHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// 先进行认证
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		startTime := time.Now()

		// 解析请求体
		var claudeReq types.ClaudeRequest
		if err := c.ShouldBindJSON(&claudeReq); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request body"})
			return
		}

		if envCfg.EnableRequestLogs {
			log.Printf("📥 收到请求: %s %s", c.Request.Method, c.Request.URL.Path)
		}

		// 获取当前上游配置
		upstream, err := cfgManager.GetCurrentUpstream()
		if err != nil {
			c.JSON(503, gin.H{
				"error": "未配置任何渠道，请先在管理界面添加渠道",
				"code":  "NO_UPSTREAM",
			})
			return
		}

		if len(upstream.APIKeys) == 0 {
			c.JSON(503, gin.H{
				"error": fmt.Sprintf("当前渠道 \"%s\" 未配置API密钥", upstream.Name),
				"code":  "NO_API_KEYS",
			})
			return
		}

		// 获取提供商
		provider := providers.GetProvider(upstream.ServiceType)
		if provider == nil {
			c.JSON(400, gin.H{"error": "Unsupported service type"})
			return
		}

		// 实现 failover 重试逻辑
		maxRetries := len(upstream.APIKeys)
		failedKeys := make(map[string]bool)
		var lastError error

		for attempt := 0; attempt < maxRetries; attempt++ {
			apiKey, err := cfgManager.GetNextAPIKey(upstream, failedKeys)
			if err != nil {
				lastError = err
				break
			}

			if envCfg.ShouldLog("info") {
				log.Printf("🎯 使用上游: %s - %s (尝试 %d/%d)", upstream.Name, upstream.BaseURL, attempt+1, maxRetries)
				log.Printf("🔑 使用API密钥: %s", maskAPIKey(apiKey))
			}

			// 转换请求
			providerReq, err := provider.ConvertToProviderRequest(&claudeReq, upstream, apiKey)
			if err != nil {
				lastError = err
				failedKeys[apiKey] = true
				continue
			}

			// 发送请求
			resp, err := sendRequest(providerReq, upstream, envCfg)
			if err != nil {
				lastError = err
				failedKeys[apiKey] = true
				cfgManager.MarkKeyAsFailed(apiKey)
				log.Printf("⚠️ API密钥失败: %v", err)
				continue
			}

			// 检查响应状态
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				// 检查是否需要 failover
				shouldFailover := shouldRetryWithNextKey(resp.StatusCode, bodyBytes)
				if shouldFailover {
					lastError = fmt.Errorf("上游错误: %d", resp.StatusCode)
					failedKeys[apiKey] = true
					cfgManager.MarkKeyAsFailed(apiKey)
					log.Printf("⚠️ API密钥失败，原因: %s", string(bodyBytes))
					continue
				}

				// 非 failover 错误，直接返回
				c.Data(resp.StatusCode, "application/json", bodyBytes)
				return
			}

			// 处理成功响应
			if claudeReq.Stream {
				handleStreamResponse(c, resp, provider, envCfg, startTime)
			} else {
				handleNormalResponse(c, resp, provider, envCfg, startTime)
			}
			return
		}

		// 所有密钥都失败了
		log.Printf("💥 所有API密钥都失败了")
		c.JSON(500, gin.H{
			"error":   "所有上游API密钥都不可用",
			"details": lastError.Error(),
		})
	})
}

// sendRequest 发送HTTP请求
func sendRequest(providerReq *types.ProviderRequest, upstream *config.UpstreamConfig, envCfg *config.EnvConfig) (*http.Response, error) {
	client := &http.Client{
		Timeout: time.Duration(envCfg.RequestTimeout) * time.Millisecond,
	}

	// 如果需要跳过 TLS 验证
	if upstream.InsecureSkipVerify {
		log.Printf("⚠️ 正在跳过对 %s 的TLS证书验证", providerReq.URL)
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	bodyBytes, err := json.Marshal(providerReq.Body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(providerReq.Method, providerReq.URL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	// 设置请求头
	for key, value := range providerReq.Headers {
		req.Header.Set(key, value)
	}

	if envCfg.EnableRequestLogs {
		log.Printf("🌐 实际请求URL: %s", providerReq.URL)
		log.Printf("📤 请求方法: %s", providerReq.Method)
	}

	return client.Do(req)
}

// handleNormalResponse 处理非流式响应
func handleNormalResponse(c *gin.Context, resp *http.Response, provider providers.Provider, envCfg *config.EnvConfig, startTime time.Time) {
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to read response"})
		return
	}

	providerResp := &types.ProviderResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       bodyBytes,
		Stream:     false,
	}

	claudeResp, err := provider.ConvertToClaudeResponse(providerResp)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to convert response"})
		return
	}

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("⏱️ 响应完成: %dms, 状态: %d", responseTime, resp.StatusCode)
	}

	c.JSON(200, claudeResp)
}

// handleStreamResponse 处理流式响应
func handleStreamResponse(c *gin.Context, resp *http.Response, provider providers.Provider, envCfg *config.EnvConfig, startTime time.Time) {
	defer resp.Body.Close()

	eventChan, errChan, err := provider.HandleStreamResponse(resp.Body)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to handle stream response"})
		return
	}

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 流式传输
	c.Stream(func(w io.Writer) bool {
		select {
		case event, ok := <-eventChan:
			if !ok {
				if envCfg.EnableResponseLogs {
					responseTime := time.Since(startTime).Milliseconds()
					log.Printf("⏱️ 流式响应完成: %dms", responseTime)
				}
				return false
			}
			c.SSEvent("", event)
			return true

		case err := <-errChan:
			log.Printf("💥 流式传输错误: %v", err)
			return false
		}
	})
}

// shouldRetryWithNextKey 判断是否应该使用下一个密钥重试
func shouldRetryWithNextKey(statusCode int, bodyBytes []byte) bool {
	// 401/403 通常是认证问题
	if statusCode == 401 || statusCode == 403 {
		return true
	}

	// 检查错误消息
	var errResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &errResp); err == nil {
		if errObj, ok := errResp["error"].(map[string]interface{}); ok {
			if msg, ok := errObj["message"].(string); ok {
				msgLower := strings.ToLower(msg)
				if strings.Contains(msgLower, "insufficient") ||
					strings.Contains(msgLower, "invalid") ||
					strings.Contains(msgLower, "unauthorized") ||
					strings.Contains(msgLower, "quota") ||
					strings.Contains(msgLower, "rate limit") ||
					strings.Contains(msgLower, "credit") ||
					strings.Contains(msgLower, "balance") {
					return true
				}
			}

			if errType, ok := errObj["type"].(string); ok {
				errTypeLower := strings.ToLower(errType)
				if strings.Contains(errTypeLower, "permission") ||
					strings.Contains(errTypeLower, "insufficient") ||
					strings.Contains(errTypeLower, "over_quota") ||
					strings.Contains(errTypeLower, "billing") {
					return true
				}
			}
		}
	}

	// 500+ 错误也可以尝试 failover
	if statusCode >= 500 {
		return true
	}

	return false
}

// maskAPIKey 掩码API密钥
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}
