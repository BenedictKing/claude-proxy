package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/claude-proxy/internal/config"
	"github.com/yourusername/claude-proxy/internal/httpclient"
	"github.com/yourusername/claude-proxy/internal/middleware"
	"github.com/yourusername/claude-proxy/internal/providers"
	"github.com/yourusername/claude-proxy/internal/types"
)

// simplifyTools 递归地简化一个值，主要是处理'tools'字段
func simplifyTools(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		newMap := make(map[string]interface{}, len(v))
		for key, val := range v {
			if key == "tools" {
				if tools, ok := val.([]interface{}); ok {
					var simplifiedTools []interface{}
					for _, tool := range tools {
						var simplifiedTool interface{} = tool // 默认是原始 tool 对象
						if toolMap, ok := tool.(map[string]interface{}); ok {
							// 检查 Claude 格式: tool.name
							if name, ok := toolMap["name"].(string); ok {
								simplifiedTool = name
							} else if function, ok := toolMap["function"].(map[string]interface{}); ok {
								// 检查 OpenAI 格式: tool.function.name
								if name, ok := function["name"].(string); ok {
									simplifiedTool = name
								}
							}
						}
						simplifiedTools = append(simplifiedTools, simplifiedTool)
					}
					newMap[key] = simplifiedTools
					continue
				}
			}
			newMap[key] = simplifyTools(val)
		}
		return newMap
	case []interface{}:
		newSlice := make([]interface{}, len(v))
		for i, item := range v {
			newSlice[i] = simplifyTools(item)
		}
		return newSlice
	default:
		return v
	}
}

// simplifyToolsInJSON 接收 JSON 字节数组，简化其中的 'tools' 字段以供日志记录
func simplifyToolsInJSON(jsonData []byte) []byte {
	var data interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return jsonData // 如果不是有效的JSON，返回原始数据
	}

	simplifiedData := simplifyTools(data)

	simplifiedBytes, err := json.Marshal(simplifiedData)
	if err != nil {
		return jsonData // 如果重新序列化失败，返回原始数据
	}

	return simplifiedBytes
}

// ProxyHandler 代理处理器
func ProxyHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// 先进行认证
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		startTime := time.Now()

		// 预读请求体（避免多次读取 c.Request.Body）
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(400, gin.H{"error": "Failed to read request body"})
			return
		}
		// 恢复请求体，以便后续其他中间件可能需要读取
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// 解析请求
		var claudeReq types.ClaudeRequest
		if err := json.Unmarshal(bodyBytes, &claudeReq); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request body"})
			return
		}

		if envCfg.EnableRequestLogs {
			log.Printf("📥 收到请求: %s %s", c.Request.Method, c.Request.URL.Path)
			// 在开发模式下，打印更详细的、格式化的原始请求体
			if envCfg.IsDevelopment() {
				// 像TS版一样，简化日志中的tools数组
				simplifiedLogBody := simplifyToolsInJSON(bodyBytes)

				var prettyBody bytes.Buffer
				if err := json.Indent(&prettyBody, simplifiedLogBody, "", "  "); err == nil {
					log.Printf("📄 原始请求体:\n%s", prettyBody.String())
				} else {
					// 如果简化或美化失败，则按原样截断打印原始字节
					if len(bodyBytes) > 500 {
						log.Printf("📄 原始请求体: %s...", string(bodyBytes[:500]))
					} else {
						log.Printf("📄 原始请求体: %s", string(bodyBytes))
					}
				}
			}
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
			resp, err := sendRequest(providerReq, upstream, envCfg, claudeReq.Stream)
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
func sendRequest(providerReq *types.ProviderRequest, upstream *config.UpstreamConfig, envCfg *config.EnvConfig, isStream bool) (*http.Response, error) {
	// 使用全局客户端管理器
	clientManager := httpclient.GetManager()

	var client *http.Client
	if isStream {
		// 流式请求：使用无超时的客户端
		client = clientManager.GetStreamClient(upstream.InsecureSkipVerify)
	} else {
		// 普通请求：使用有超时的客户端
		timeout := time.Duration(envCfg.RequestTimeout) * time.Millisecond
		client = clientManager.GetStandardClient(timeout, upstream.InsecureSkipVerify)
	}

	if upstream.InsecureSkipVerify && envCfg.EnableRequestLogs {
		log.Printf("⚠️ 正在跳过对 %s 的TLS证书验证", providerReq.URL)
	}

	// 处理请求体：支持两种类型
	var bodyBytes []byte
	var err error

	switch v := providerReq.Body.(type) {
	case []byte:
		// 已经是字节数组，直接使用
		bodyBytes = v
	case string:
		// 字符串类型，转换为字节数组
		bodyBytes = []byte(v)
	default:
		// 其他类型，需要JSON序列化
		bodyBytes, err = json.Marshal(providerReq.Body)
		if err != nil {
			return nil, err
		}
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
		if envCfg.IsDevelopment() {
			// 像TS版一样，简化日志中的tools数组
			simplifiedLogBody := simplifyToolsInJSON(bodyBytes)

			// 在开发模式下，打印实际发出的请求体
			var prettyBody bytes.Buffer
			if err := json.Indent(&prettyBody, simplifiedLogBody, "", "  "); err == nil {
				log.Printf("📦 实际请求体:\n%s", prettyBody.String())
			} else {
				// 如果不是有效的JSON，则按原样截断打印
				if len(bodyBytes) > 500 {
					log.Printf("📦 实际请求体: %s...", string(bodyBytes[:500]))
				} else {
					log.Printf("📦 实际请求体: %s", string(bodyBytes))
				}
			}
		}
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

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("⏱️ 响应完成: %dms, 状态: %d", responseTime, resp.StatusCode)
		if envCfg.IsDevelopment() {
			var prettyBody bytes.Buffer
			if err := json.Indent(&prettyBody, bodyBytes, "", "  "); err == nil {
				log.Printf("📦 响应体:\n%s", prettyBody.String())
			} else {
				// 如果不是有效的JSON，则按原样截断打印
				if len(bodyBytes) > 500 {
					log.Printf("📦 响应体: %s...", string(bodyBytes[:500]))
				} else {
					log.Printf("📦 响应体: %s", string(bodyBytes))
				}
			}
		}
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

	var logBuffer bytes.Buffer

	// 流式传输
	c.Stream(func(w io.Writer) bool {
		var writer io.Writer = w
		if envCfg.IsDevelopment() {
			writer = io.MultiWriter(w, &logBuffer)
		}

		select {
		case event, ok := <-eventChan:
			if !ok {
				if envCfg.EnableResponseLogs {
					responseTime := time.Since(startTime).Milliseconds()
					log.Printf("⏱️ 流式响应完成: %dms", responseTime)
					if envCfg.IsDevelopment() && logBuffer.Len() > 0 {
						log.Printf("🛰️  上游流式响应体 (完整):\n---\n%s---", logBuffer.String())
					}
				}
				return false
			}
			// 直接写入，因为provider已格式化为SSE事件
			_, err := writer.Write([]byte(event))
			if err != nil {
				// 客户端可能已断开连接
				log.Printf("⚠️ 写入流时出错: %v", err)
				if envCfg.EnableResponseLogs && envCfg.IsDevelopment() && logBuffer.Len() > 0 {
					log.Printf("🛰️  上游流式响应体 (中断):\n---\n%s---", logBuffer.String())
				}
				return false
			}
			return true

		case err, ok := <-errChan:
			if !ok {
				// errChan被关闭，这不是预期的退出路径
				return false
			}
			if err != nil {
				log.Printf("💥 流式传输错误: %v", err)
			}
			if envCfg.EnableResponseLogs && envCfg.IsDevelopment() && logBuffer.Len() > 0 {
				log.Printf("🛰️  上游流式响应体 (错误):\n---\n%s---", logBuffer.String())
			}
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

// maskAPIKey 掩码API密钥（与 TS 版本保持一致）
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}

	length := len(key)
	if length <= 10 {
		// 短密钥：保留前3位和后2位
		if length <= 5 {
			return "***"
		}
		return key[:3] + "***" + key[length-2:]
	}

	// 长密钥：保留前8位和后5位
	return key[:8] + "***" + key[length-5:]
}
