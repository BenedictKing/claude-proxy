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

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/httpclient"
	"github.com/BenedictKing/claude-proxy/internal/middleware"
	"github.com/BenedictKing/claude-proxy/internal/providers"
	"github.com/BenedictKing/claude-proxy/internal/scheduler"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
	"github.com/gin-gonic/gin"
)

// ProxyHandler 代理处理器
// 支持多渠道调度：当配置多个渠道时自动启用
func ProxyHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager, channelScheduler *scheduler.ChannelScheduler) gin.HandlerFunc {
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

		// claudeReq 变量用于判断是否流式请求和提取 user_id
		var claudeReq types.ClaudeRequest
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &claudeReq)
		}

		// 提取 user_id 用于 Trace 亲和性
		userID := extractUserID(bodyBytes)

		// 检查是否为多渠道模式
		isMultiChannel := channelScheduler.IsMultiChannelMode(false)

		if isMultiChannel {
			// 多渠道模式：使用调度器
			handleMultiChannelProxy(c, envCfg, cfgManager, channelScheduler, bodyBytes, claudeReq, userID, startTime)
		} else {
			// 单渠道模式：使用现有逻辑（也记录指标）
			handleSingleChannelProxy(c, envCfg, cfgManager, channelScheduler, bodyBytes, claudeReq, startTime)
		}
	})
}

// extractUserID 从请求体中提取 user_id
func extractUserID(bodyBytes []byte) string {
	var req struct {
		Metadata struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err == nil {
		return req.Metadata.UserID
	}
	return ""
}

// handleMultiChannelProxy 处理多渠道代理请求
func handleMultiChannelProxy(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	bodyBytes []byte,
	claudeReq types.ClaudeRequest,
	userID string,
	startTime time.Time,
) {
	failedChannels := make(map[int]bool)
	var lastError error
	var lastFailoverError *struct {
		Status int
		Body   []byte
	}

	// 获取活跃渠道数量作为最大重试次数
	maxChannelAttempts := channelScheduler.GetActiveChannelCount(false)

	for channelAttempt := 0; channelAttempt < maxChannelAttempts; channelAttempt++ {
		// 使用调度器选择渠道
		selection, err := channelScheduler.SelectChannel(c.Request.Context(), userID, failedChannels, false)
		if err != nil {
			lastError = err
			break
		}

		upstream := selection.Upstream
		channelIndex := selection.ChannelIndex

		if envCfg.ShouldLog("info") {
			log.Printf("🎯 [多渠道] 选择渠道: [%d] %s (原因: %s, 尝试 %d/%d)",
				channelIndex, upstream.Name, selection.Reason, channelAttempt+1, maxChannelAttempts)
		}

		// 尝试使用该渠道的所有 key，返回成功使用的 key
		success, successKey, failoverErr := tryChannelWithAllKeys(c, envCfg, cfgManager, channelScheduler, upstream, bodyBytes, claudeReq, startTime, false)

		if success {
			// 记录成功的 key，更新 Trace 亲和
			if successKey != "" {
				channelScheduler.RecordSuccess(upstream.BaseURL, successKey, false)
			}
			channelScheduler.SetTraceAffinity(userID, channelIndex)
			return
		}

		// 渠道所有 key 都失败，标记渠道失败
		failedChannels[channelIndex] = true

		if failoverErr != nil {
			lastFailoverError = failoverErr
			lastError = fmt.Errorf("渠道 [%d] %s 失败", channelIndex, upstream.Name)
		}

		log.Printf("⚠️ [多渠道] 渠道 [%d] %s 所有密钥都失败，尝试下一个渠道", channelIndex, upstream.Name)
	}

	// 所有渠道都失败
	log.Printf("💥 [多渠道] 所有渠道都失败了")

	if lastFailoverError != nil {
		status := lastFailoverError.Status
		if status == 0 {
			status = 503
		}
		var errBody map[string]interface{}
		if err := json.Unmarshal(lastFailoverError.Body, &errBody); err == nil {
			c.JSON(status, errBody)
		} else {
			c.JSON(status, gin.H{"error": string(lastFailoverError.Body)})
		}
	} else {
		errMsg := "所有渠道都不可用"
		if lastError != nil {
			errMsg = lastError.Error()
		}
		c.JSON(503, gin.H{
			"error":   "所有渠道都不可用",
			"details": errMsg,
		})
	}
}

// tryChannelWithAllKeys 尝试使用渠道的所有密钥
// 返回 (success bool, successKey string, lastFailoverError *struct{Status int; Body []byte})
func tryChannelWithAllKeys(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	upstream *config.UpstreamConfig,
	bodyBytes []byte,
	claudeReq types.ClaudeRequest,
	startTime time.Time,
	isResponses bool,
) (bool, string, *struct {
	Status int
	Body   []byte
}) {
	if len(upstream.APIKeys) == 0 {
		return false, "", nil
	}

	provider := providers.GetProvider(upstream.ServiceType)
	if provider == nil {
		return false, "", nil
	}

	// 获取指标管理器用于检查熔断状态
	metricsManager := channelScheduler.GetMessagesMetricsManager()
	if isResponses {
		metricsManager = channelScheduler.GetResponsesMetricsManager()
	}

	maxRetries := len(upstream.APIKeys)
	failedKeys := make(map[string]bool)
	var lastFailoverError *struct {
		Status int
		Body   []byte
	}
	deprioritizeCandidates := make(map[string]bool)

	for attempt := 0; attempt < maxRetries; attempt++ {
		// 恢复请求体
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		apiKey, err := cfgManager.GetNextAPIKey(upstream, failedKeys)
		if err != nil {
			break
		}

		// 检查该 Key 是否处于熔断状态，跳过熔断的 Key
		if metricsManager.ShouldSuspendKey(upstream.BaseURL, apiKey) {
			failedKeys[apiKey] = true
			log.Printf("⚡ 跳过熔断中的 Key: %s", maskAPIKey(apiKey))
			continue
		}

		if envCfg.ShouldLog("info") {
			log.Printf("🔑 使用API密钥: %s (尝试 %d/%d)", maskAPIKey(apiKey), attempt+1, maxRetries)
		}

		// 转换请求
		providerReq, _, err := provider.ConvertToProviderRequest(c, upstream, apiKey)
		if err != nil {
			failedKeys[apiKey] = true
			// 记录该 key 失败
			channelScheduler.RecordFailure(upstream.BaseURL, apiKey, isResponses)
			continue
		}

		// 发送请求
		resp, err := sendRequest(providerReq, upstream, envCfg, claudeReq.Stream)
		if err != nil {
			failedKeys[apiKey] = true
			cfgManager.MarkKeyAsFailed(apiKey)
			// 记录该 key 失败
			channelScheduler.RecordFailure(upstream.BaseURL, apiKey, isResponses)
			log.Printf("⚠️ API密钥失败: %v", err)
			continue
		}

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			respBodyBytes = utils.DecompressGzipIfNeeded(resp, respBodyBytes)

			shouldFailover, isQuotaRelated := shouldRetryWithNextKey(resp.StatusCode, respBodyBytes)
			if shouldFailover {
				failedKeys[apiKey] = true
				cfgManager.MarkKeyAsFailed(apiKey)
				// 记录该 key 失败
				channelScheduler.RecordFailure(upstream.BaseURL, apiKey, isResponses)
				log.Printf("⚠️ API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)

				lastFailoverError = &struct {
					Status int
					Body   []byte
				}{
					Status: resp.StatusCode,
					Body:   respBodyBytes,
				}

				if isQuotaRelated {
					deprioritizeCandidates[apiKey] = true
				}
				continue
			}

			// 非 failover 错误，直接返回（请求已处理但不算成功）
			c.Data(resp.StatusCode, "application/json", respBodyBytes)
			return true, "", nil // 返回 true 表示请求已处理，但 successKey 为空表示不记录成功
		}

		// 处理成功响应
		if len(deprioritizeCandidates) > 0 {
			for key := range deprioritizeCandidates {
				_ = cfgManager.DeprioritizeAPIKey(key)
			}
		}

		if claudeReq.Stream {
			handleStreamResponse(c, resp, provider, envCfg, startTime, upstream, bodyBytes)
		} else {
			handleNormalResponse(c, resp, provider, envCfg, startTime, bodyBytes)
		}
		return true, apiKey, nil
	}

	return false, "", lastFailoverError
}

// handleSingleChannelProxy 处理单渠道代理请求（现有逻辑）
func handleSingleChannelProxy(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	bodyBytes []byte,
	claudeReq types.ClaudeRequest,
	startTime time.Time,
) {
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
	var lastOriginalBodyBytes []byte
	var lastFailoverError *struct {
		Status int
		Body   []byte
	}
	deprioritizeCandidates := make(map[string]bool)

	// 获取指标管理器用于检查熔断状态
	metricsManager := channelScheduler.GetMessagesMetricsManager()

	for attempt := 0; attempt < maxRetries; attempt++ {
		// 恢复请求体
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		apiKey, err := cfgManager.GetNextAPIKey(upstream, failedKeys)
		if err != nil {
			lastError = err
			break
		}

		// 检查该 Key 是否处于熔断状态，跳过熔断的 Key
		if metricsManager.ShouldSuspendKey(upstream.BaseURL, apiKey) {
			failedKeys[apiKey] = true
			log.Printf("⚡ 跳过熔断中的 Key: %s", maskAPIKey(apiKey))
			continue
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
			channelScheduler.RecordFailure(upstream.BaseURL, apiKey, false)
			if originalBodyBytes != nil {
				lastOriginalBodyBytes = originalBodyBytes
			}
			continue
		}
		lastOriginalBodyBytes = originalBodyBytes

		// 请求日志记录
		if envCfg.EnableRequestLogs {
			log.Printf("📥 收到请求: %s %s", c.Request.Method, c.Request.URL.Path)
			if envCfg.IsDevelopment() {
				logBody := lastOriginalBodyBytes
				if len(logBody) == 0 && c.Request.Body != nil {
					bodyFromContext, _ := io.ReadAll(c.Request.Body)
					c.Request.Body = io.NopCloser(bytes.NewReader(bodyFromContext))
					logBody = bodyFromContext
				}
				formattedBody := utils.FormatJSONBytesForLog(logBody, 500)
				log.Printf("📄 原始请求体:\n%s", formattedBody)

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

		// 发送请求
		resp, err := sendRequest(providerReq, upstream, envCfg, claudeReq.Stream)
		if err != nil {
			lastError = err
			failedKeys[apiKey] = true
			cfgManager.MarkKeyAsFailed(apiKey)
			channelScheduler.RecordFailure(upstream.BaseURL, apiKey, false)
			log.Printf("⚠️ API密钥失败: %v", err)
			continue
		}

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			respBodyBytes = utils.DecompressGzipIfNeeded(resp, respBodyBytes)

			shouldFailover, isQuotaRelated := shouldRetryWithNextKey(resp.StatusCode, respBodyBytes)
			if shouldFailover {
				lastError = fmt.Errorf("上游错误: %d", resp.StatusCode)
				failedKeys[apiKey] = true
				cfgManager.MarkKeyAsFailed(apiKey)
				channelScheduler.RecordFailure(upstream.BaseURL, apiKey, false)

				log.Printf("⚠️ API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)
				if envCfg.EnableResponseLogs && envCfg.IsDevelopment() {
					formattedBody := utils.FormatJSONBytesForLog(respBodyBytes, 500)
					log.Printf("📦 失败原因:\n%s", formattedBody)
				} else if envCfg.EnableResponseLogs {
					log.Printf("失败原因: %s", string(respBodyBytes))
				}

				lastFailoverError = &struct {
					Status int
					Body   []byte
				}{
					Status: resp.StatusCode,
					Body:   respBodyBytes,
				}

				if isQuotaRelated {
					deprioritizeCandidates[apiKey] = true
				}
				continue
			}

			// 非 failover 错误
			if envCfg.EnableResponseLogs {
				log.Printf("⚠️ 上游返回错误: %d", resp.StatusCode)
				if envCfg.IsDevelopment() {
					formattedBody := utils.FormatJSONBytesForLog(respBodyBytes, 500)
					log.Printf("📦 错误响应体:\n%s", formattedBody)

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
			c.Data(resp.StatusCode, "application/json", respBodyBytes)
			return
		}

		// 处理成功响应
		channelScheduler.RecordSuccess(upstream.BaseURL, apiKey, false)

		if len(deprioritizeCandidates) > 0 {
			for key := range deprioritizeCandidates {
				if err := cfgManager.DeprioritizeAPIKey(key); err != nil {
					log.Printf("⚠️ 密钥降级失败: %v", err)
				}
			}
		}

		if claudeReq.Stream {
			handleStreamResponse(c, resp, provider, envCfg, startTime, upstream, bodyBytes)
		} else {
			handleNormalResponse(c, resp, provider, envCfg, startTime, bodyBytes)
		}
		return
	}

	// 所有密钥都失败了
	log.Printf("💥 所有API密钥都失败了")

	if lastFailoverError != nil {
		status := lastFailoverError.Status
		if status == 0 {
			status = 500
		}
		var errBody map[string]interface{}
		if err := json.Unmarshal(lastFailoverError.Body, &errBody); err == nil {
			c.JSON(status, errBody)
		} else {
			c.JSON(status, gin.H{"error": string(lastFailoverError.Body)})
		}
	} else {
		errMsg := "未知错误"
		if lastError != nil {
			errMsg = lastError.Error()
		}
		c.JSON(500, gin.H{
			"error":   "所有上游API密钥都不可用",
			"details": errMsg,
		})
	}
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
func handleNormalResponse(c *gin.Context, resp *http.Response, provider providers.Provider, envCfg *config.EnvConfig, startTime time.Time, requestBody []byte) {
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

	// 如果上游没有返回 Usage，本地估算
	if claudeResp.Usage == nil {
		claudeResp.Usage = &types.Usage{
			InputTokens:  utils.EstimateRequestTokens(requestBody),
			OutputTokens: utils.EstimateResponseTokens(claudeResp.Content),
		}
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
func handleStreamResponse(c *gin.Context, resp *http.Response, provider providers.Provider, envCfg *config.EnvConfig, startTime time.Time, upstream *config.UpstreamConfig, requestBody []byte) {
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
	hasUsage := false                 // 跟踪是否已收到 usage
	var outputTextBuffer bytes.Buffer // 累积输出文本用于估算 token
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

			// 使用 JSON 解析精确检测 usage 字段
			if !hasUsage {
				hasUsage = checkEventHasUsage(event)
			}

			// 提取文本内容用于估算输出 token
			extractTextFromEvent(event, &outputTextBuffer)

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

			// 检测 message_stop 事件，在其之前注入 usage（如果需要）
			if !hasUsage && !clientGone && isMessageStopEvent(event) {
				usageEvent := buildUsageEvent(requestBody, outputTextBuffer.String())
				w.Write([]byte(usageEvent))
				flusher.Flush()
				hasUsage = true // 防止重复注入
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

// buildUsageEvent 构建带 usage 的 message_delta SSE 事件
func buildUsageEvent(requestBody []byte, outputText string) string {
	inputTokens := utils.EstimateRequestTokens(requestBody)
	outputTokens := utils.EstimateTokens(outputText)

	event := map[string]interface{}{
		"type": "message_delta",
		"usage": map[string]int{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}
	eventJSON, _ := json.Marshal(event)
	return fmt.Sprintf("event: message_delta\ndata: %s\n\n", eventJSON)
}

// checkEventHasUsage 使用 JSON 解析精确检测事件是否包含 usage 字段
func checkEventHasUsage(event string) bool {
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		// 检查顶层 usage 字段
		if hasUsageFields(data["usage"]) {
			return true
		}

		// 检查 message.usage（Claude 流式响应格式）
		if msg, ok := data["message"].(map[string]interface{}); ok {
			if hasUsageFields(msg["usage"]) {
				return true
			}
		}
	}
	return false
}

// hasUsageFields 检查 usage 对象是否包含 token 字段
func hasUsageFields(usage interface{}) bool {
	if u, ok := usage.(map[string]interface{}); ok {
		_, hasInput := u["input_tokens"]
		_, hasOutput := u["output_tokens"]
		return hasInput || hasOutput
	}
	return false
}

// isMessageStopEvent 使用 JSON 解析检测是否为 message_stop 事件
func isMessageStopEvent(event string) bool {
	// 先检查 event: 行
	if strings.Contains(event, "event: message_stop") {
		return true
	}

	// 再检查 data 中的 type 字段
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		if data["type"] == "message_stop" {
			return true
		}
	}
	return false
}

// extractTextFromEvent 从 SSE 事件中提取文本内容
func extractTextFromEvent(event string, buf *bytes.Buffer) {
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		// Claude SSE: delta.text (text_delta 类型)
		if delta, ok := data["delta"].(map[string]interface{}); ok {
			if text, ok := delta["text"].(string); ok {
				buf.WriteString(text)
			}
			// tool_use: delta.partial_json
			if partialJSON, ok := delta["partial_json"].(string); ok {
				buf.WriteString(partialJSON)
			}
		}

		// content_block_start 中的初始文本
		if cb, ok := data["content_block"].(map[string]interface{}); ok {
			if text, ok := cb["text"].(string); ok {
				buf.WriteString(text)
			}
		}
	}
}
