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
	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/httpclient"
	"github.com/BenedictKing/claude-proxy/internal/middleware"
	"github.com/BenedictKing/claude-proxy/internal/providers"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
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

		// 读取原始请求体
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(400, gin.H{"error": "Failed to read request body"})
			return
		}
		// 恢复请求体供后续使用
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// claudeReq 变量用于判断是否流式请求
		var claudeReq types.ClaudeRequest
		// 尝试解析，失败也无妨
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &claudeReq)
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
		failedKeys := make(map[string]bool) // 记录本次请求中已经失败过的 key
		var lastError error
		var lastOriginalBodyBytes []byte // 用于记录最后一次尝试的原始请求体，以便日志记录
		// 记录最后一次需要failover的上游错误，用于所有密钥都失败时回传原始错误
		var lastFailoverError *struct {
			Status int
			Body   []byte
		}
		// 候选降级密钥（仅当后续有密钥成功调用时，才将这些密钥移到列表末尾）
		deprioritizeCandidates := make(map[string]bool)

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
			providerReq, originalBodyBytes, err := provider.ConvertToProviderRequest(c, upstream, apiKey)
			if err != nil {
				lastError = err
				failedKeys[apiKey] = true
				if originalBodyBytes != nil { // 记录下用于日志的原始 body
					lastOriginalBodyBytes = originalBodyBytes
				}
				continue
			}
			lastOriginalBodyBytes = originalBodyBytes // 记录下用于日志的原始 body

			// --- 请求日志记录 ---
			if envCfg.EnableRequestLogs {
				log.Printf("📥 收到请求: %s %s", c.Request.Method, c.Request.URL.Path)
				if envCfg.IsDevelopment() {
					logBody := lastOriginalBodyBytes
					// 对于流式透传，如果 bodyBytes 为空，需要从原始请求体中读取
					if len(logBody) == 0 && c.Request.Body != nil {
						bodyFromContext, _ := io.ReadAll(c.Request.Body)
						c.Request.Body = io.NopCloser(bytes.NewReader(bodyFromContext)) // 恢复
						logBody = bodyFromContext
					}

					// 使用智能截断和简化函数（与TS版本对齐）
					formattedBody := utils.FormatJSONBytesForLog(logBody, 500)
					log.Printf("📄 原始请求体:\n%s", formattedBody)

					// 对请求头做敏感信息脱敏
					sanitizedHeaders := make(map[string]string)
					for key, values := range c.Request.Header {
						if len(values) > 0 {
							sanitizedHeaders[key] = values[0]
						}
					}
					maskedHeaders := utils.MaskSensitiveHeaders(sanitizedHeaders)
					headersJSON, _ := json.MarshalIndent(maskedHeaders, "", "  ")
					log.Printf("📥 原始请求头:\n%s", string(headersJSON))
				}
			}
			// --- 请求日志记录结束 ---

			// 发送请求
			// claudeReq.Stream 用于判断是否是流式请求
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

				// 兜底处理：如果响应体是 gzip 压缩的，尝试解压缩
				// 这确保错误信息始终可读，用于日志和重试逻辑
				bodyBytes = utils.DecompressGzipIfNeeded(resp, bodyBytes)

				// 检查是否需要 failover
				shouldFailover, isQuotaRelated := shouldRetryWithNextKey(resp.StatusCode, bodyBytes)
				if shouldFailover {
					lastError = fmt.Errorf("上游错误: %d", resp.StatusCode)
					failedKeys[apiKey] = true
					cfgManager.MarkKeyAsFailed(apiKey)

					// 增强的日志输出
					log.Printf("⚠️ API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)
					if envCfg.EnableResponseLogs && envCfg.IsDevelopment() {
						formattedBody := utils.FormatJSONBytesForLog(bodyBytes, 500)
						log.Printf("📦 失败原因:\n%s", formattedBody)
					} else if envCfg.EnableResponseLogs {
						// 生产环境打印简短信息
						log.Printf("失败原因: %s", string(bodyBytes))
					}

					// 记录最后一次failover错误（用于所有密钥失败时返回）
					lastFailoverError = &struct {
						Status int
						Body   []byte
					}{
						Status: resp.StatusCode,
						Body:   bodyBytes,
					}

					// 仅记录候选降级密钥，待后续任一密钥成功时再移动到末尾
					if isQuotaRelated {
						deprioritizeCandidates[apiKey] = true
					}

					continue
				}

				// 非 failover 错误，记录日志后返回
				if envCfg.EnableResponseLogs {
					log.Printf("⚠️ 上游返回错误: %d", resp.StatusCode)
					if envCfg.IsDevelopment() {
						// 格式化错误响应体
						formattedBody := utils.FormatJSONBytesForLog(bodyBytes, 500)
						log.Printf("📦 错误响应体:\n%s", formattedBody)

						// 打印错误响应头
						respHeaders := make(map[string]string)
						for key, values := range resp.Header {
							if len(values) > 0 {
								respHeaders[key] = values[0]
							}
						}
						respHeadersJSON, _ := json.MarshalIndent(respHeaders, "", "  ")
						log.Printf("📋 错误响应头:\n%s", string(respHeadersJSON))
					}
				}
				c.Data(resp.StatusCode, "application/json", bodyBytes)
				return
			}

			// 处理成功响应
			// 如果本次请求最终成功，执行降级移动（仅对额度/余额相关失败的密钥）
			if len(deprioritizeCandidates) > 0 {
				for key := range deprioritizeCandidates {
					if err := cfgManager.DeprioritizeAPIKey(key); err != nil {
						log.Printf("⚠️ 密钥降级失败: %v", err)
					}
				}
			}

			if claudeReq.Stream {
				handleStreamResponse(c, resp, provider, envCfg, startTime, upstream)
			} else {
				handleNormalResponse(c, resp, provider, envCfg, startTime)
			}
			return
		}

		// 所有密钥都失败了
		log.Printf("💥 所有API密钥都失败了")

		// 若有记录的最后一次上游错误，按原状态码和内容返回
		if lastFailoverError != nil {
			status := lastFailoverError.Status
			if status == 0 {
				status = 500
			}

			// 尝试解析为JSON返回
			var errBody map[string]interface{}
			if err := json.Unmarshal(lastFailoverError.Body, &errBody); err == nil {
				c.JSON(status, errBody)
			} else {
				// 如果不是JSON，返回通用错误
				c.JSON(status, gin.H{
					"error": string(lastFailoverError.Body),
				})
			}
		} else {
			// 没有上游错误记录，返回通用错误
			c.JSON(500, gin.H{
				"error":   "所有上游API密钥都不可用",
				"details": lastError.Error(),
			})
		}
	})
}

// sendRequest 发送HTTP请求
func sendRequest(req *http.Request, upstream *config.UpstreamConfig, envCfg *config.EnvConfig, isStream bool) (*http.Response, error) {
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
		log.Printf("⚠️ 正在跳过对 %s 的TLS证书验证", req.URL.String())
	}

	if envCfg.EnableRequestLogs {
		log.Printf("🌐 实际请求URL: %s", req.URL.String())
		log.Printf("📤 请求方法: %s", req.Method)
		if envCfg.IsDevelopment() {
			// 对请求头做敏感信息脱敏
			reqHeaders := make(map[string]string)
			for key, values := range req.Header {
				if len(values) > 0 {
					reqHeaders[key] = values[0]
				}
			}
			maskedReqHeaders := utils.MaskSensitiveHeaders(reqHeaders)
			reqHeadersJSON, _ := json.MarshalIndent(maskedReqHeaders, "", "  ")
			log.Printf("📋 实际请求头:\n%s", string(reqHeadersJSON))

			if req.Body != nil {
				// 读取请求体用于日志
				bodyBytes, err := io.ReadAll(req.Body)
				if err == nil {
					// 恢复请求体
					req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

					// 使用智能截断和简化函数（与TS版本对齐）
					formattedBody := utils.FormatJSONBytesForLog(bodyBytes, 500)
					log.Printf("📦 实际请求体:\n%s", formattedBody)
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
			// 响应头(不需要脱敏)
			respHeaders := make(map[string]string)
			for key, values := range resp.Header {
				if len(values) > 0 {
					respHeaders[key] = values[0]
				}
			}
			respHeadersJSON, _ := json.MarshalIndent(respHeaders, "", "  ")
			log.Printf("📋 响应头:\n%s", string(respHeadersJSON))

			// 使用智能截断（与TS版本对齐）
			formattedBody := utils.FormatJSONBytesForLog(bodyBytes, 500)
			log.Printf("📦 响应体:\n%s", formattedBody)
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

	// 监听响应关闭事件(客户端断开连接)
	closeNotify := c.Writer.CloseNotify()
	go func() {
		select {
		case <-closeNotify:
			// 检查响应是否已完成
			if !c.Writer.Written() {
				if envCfg.EnableResponseLogs {
					responseTime := time.Since(startTime).Milliseconds()
					log.Printf("⏱️ 响应中断: %dms, 状态: %d", responseTime, resp.StatusCode)
				}
			}
		case <-time.After(10 * time.Second):
			// 超时退出goroutine,避免泄漏
			return
		}
	}()

	// 转发上游响应头到客户端（透明代理）
	utils.ForwardResponseHeaders(resp.Header, c.Writer)

	c.JSON(200, claudeResp)

	// 响应完成后记录
	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("⏱️ 响应发送完成: %dms, 状态: %d", responseTime, resp.StatusCode)
	}
}

// handleStreamResponse 处理流式响应
func handleStreamResponse(c *gin.Context, resp *http.Response, provider providers.Provider, envCfg *config.EnvConfig, startTime time.Time, upstream *config.UpstreamConfig) {
	defer resp.Body.Close()

	eventChan, errChan, err := provider.HandleStreamResponse(resp.Body)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to handle stream response"})
		return
	}

	// 先转发上游响应头（透明代理）
	utils.ForwardResponseHeaders(resp.Header, c.Writer)

	// 设置 SSE 响应头（可能覆盖上游的 Content-Type）
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Status(200)

	var logBuffer bytes.Buffer
	var synthesizer *utils.StreamSynthesizer
	streamLoggingEnabled := envCfg.IsDevelopment() && envCfg.EnableResponseLogs
	if streamLoggingEnabled {
		synthesizer = utils.NewStreamSynthesizer(upstream.ServiceType)
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("⚠️ ResponseWriter不支持Flush接口")
		return
	}
	flusher.Flush()

	clientGone := false
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				// 通道关闭，流式传输结束
				if envCfg.EnableResponseLogs {
					responseTime := time.Since(startTime).Milliseconds()
					log.Printf("⏱️ 流式响应完成: %dms", responseTime)

					// 打印完整的响应内容
					if envCfg.IsDevelopment() {
						if synthesizer != nil {
							synthesizedContent := synthesizer.GetSynthesizedContent()
							parseFailed := synthesizer.IsParseFailed()
							if synthesizedContent != "" && !parseFailed {
								log.Printf("🛰️  上游流式响应合成内容:\n%s", strings.TrimSpace(synthesizedContent))
							} else if logBuffer.Len() > 0 {
								log.Printf("🛰️  上游流式响应原始内容:\n%s", logBuffer.String())
							}
						} else if logBuffer.Len() > 0 {
							// synthesizer为nil时，直接打印原始内容
							log.Printf("🛰️  上游流式响应原始内容:\n%s", logBuffer.String())
						}
					}
				}
				return
			}

			// 缓存事件用于最后的日志输出
			if streamLoggingEnabled {
				logBuffer.WriteString(event)
				if synthesizer != nil {
					lines := strings.Split(event, "\n")
					for _, line := range lines {
						synthesizer.ProcessLine(line)
					}
				}
			}

			// 实时转发给客户端（流式传输）
			if !clientGone {
				_, err := w.Write([]byte(event))
				if err != nil {
					clientGone = true // 标记客户端已断开，停止后续写入
					errMsg := err.Error()
					if strings.Contains(errMsg, "broken pipe") || strings.Contains(errMsg, "connection reset") {
						if envCfg.ShouldLog("info") {
							log.Printf("ℹ️ 客户端中断连接 (正常行为)，继续接收上游数据...")
						}
					} else {
						log.Printf("⚠️ 流式传输写入错误: %v", err)
					}
					// 注意：这里不再return，而是继续循环以耗尽eventChan
				} else {
					flusher.Flush()
				}
			}

		case err, ok := <-errChan:
			if !ok {
				// errChan关闭，但这不一定意味着流结束，继续等待eventChan
				continue
			}
			if err != nil {
				// 真的有错误发生
				log.Printf("💥 流式传输错误: %v", err)

				// 打印已接收到的部分响应
				if envCfg.EnableResponseLogs && envCfg.IsDevelopment() {
					if synthesizer != nil {
						synthesizedContent := synthesizer.GetSynthesizedContent()
						if synthesizedContent != "" && !synthesizer.IsParseFailed() {
							log.Printf("🛰️  上游流式响应合成内容 (部分):\n%s", strings.TrimSpace(synthesizedContent))
						} else if logBuffer.Len() > 0 {
							log.Printf("🛰️  上游流式响应原始内容 (部分):\n%s", logBuffer.String())
						}
					}
				}
				return
			}
		}
	}
}

// shouldRetryWithNextKey 判断是否应该使用下一个密钥重试
// 返回: (shouldFailover bool, isQuotaRelated bool)
func shouldRetryWithNextKey(statusCode int, bodyBytes []byte) (bool, bool) {
	// 401/403 通常是认证问题
	if statusCode == 401 || statusCode == 403 {
		return true, false
	}

	// 429 速率限制，切换下一个密钥
	if statusCode == 429 {
		return true, true
	}

	isQuotaRelated := false

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
					strings.Contains(msg, "请求数限制") ||
					strings.Contains(msgLower, "credit") ||
					strings.Contains(msgLower, "balance") {

					// 判断是否为额度/余额相关
					if strings.Contains(msgLower, "积分不足") ||
						strings.Contains(msgLower, "insufficient") ||
						strings.Contains(msgLower, "credit") ||
						strings.Contains(msgLower, "balance") ||
						strings.Contains(msgLower, "quota") ||
						strings.Contains(msg, "请求数限制") {
						isQuotaRelated = true
					}
					return true, isQuotaRelated
				}
			}

			if errType, ok := errObj["type"].(string); ok {
				errTypeLower := strings.ToLower(errType)
				if strings.Contains(errTypeLower, "permission") ||
					strings.Contains(errTypeLower, "insufficient") ||
					strings.Contains(errTypeLower, "over_quota") ||
					strings.Contains(errTypeLower, "billing") {

					// 判断是否为额度/余额相关
					if strings.Contains(errTypeLower, "over_quota") ||
						strings.Contains(errTypeLower, "billing") ||
						strings.Contains(errTypeLower, "insufficient") {
						isQuotaRelated = true
					}
					return true, isQuotaRelated
				}
			}
		}
	}

	// 500+ 错误也可以尝试 failover
	if statusCode >= 500 {
		return true, false
	}

	return false, false
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
