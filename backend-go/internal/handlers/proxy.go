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

		// 读取原始请求体（限制最大大小，通过环境变量配置）
		maxBodySize := envCfg.MaxRequestBodySize
		limitedReader := io.LimitReader(c.Request.Body, maxBodySize+1)
		bodyBytes, err := io.ReadAll(limitedReader)
		if err != nil {
			c.JSON(400, gin.H{"error": "Failed to read request body"})
			return
		}
		if int64(len(bodyBytes)) > maxBodySize {
			// 排空剩余请求体，避免 keep-alive 连接污染
			io.Copy(io.Discard, c.Request.Body)
			c.JSON(413, gin.H{"error": fmt.Sprintf("Request body too large, maximum size is %d MB", maxBodySize/1024/1024)})
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
			log.Printf("⚡ 跳过熔断中的 Key: %s", utils.MaskAPIKey(apiKey))
			continue
		}

		if envCfg.ShouldLog("info") {
			log.Printf("🔑 使用API密钥: %s (尝试 %d/%d)", utils.MaskAPIKey(apiKey), attempt+1, maxRetries)
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
			log.Printf("⚡ 跳过熔断中的 Key: %s", utils.MaskAPIKey(apiKey))
			continue
		}

		if envCfg.ShouldLog("info") {
			log.Printf("🎯 使用上游: %s - %s (尝试 %d/%d)", upstream.Name, upstream.BaseURL, attempt+1, maxRetries)
			log.Printf("🔑 使用API密钥: %s", utils.MaskAPIKey(apiKey))
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
	// 如果 input_tokens 为 0 或 1（虚假值），也需要补全
	// 但如果有 cache_creation_input_tokens 或 cache_read_input_tokens，则 input_tokens 为 0/1 是正常的
	if claudeResp.Usage == nil {
		estimatedInput := utils.EstimateRequestTokens(requestBody)
		estimatedOutput := utils.EstimateResponseTokens(claudeResp.Content)
		claudeResp.Usage = &types.Usage{
			InputTokens:  estimatedInput,
			OutputTokens: estimatedOutput,
		}
		if envCfg.EnableResponseLogs {
			log.Printf("🔢 [Token补全] 上游无Usage, 本地估算: input=%d, output=%d", estimatedInput, estimatedOutput)
		}
	} else {
		originalInput := claudeResp.Usage.InputTokens
		originalOutput := claudeResp.Usage.OutputTokens
		patched := false

		// 检查是否有缓存 token（如果有，input_tokens 为 0/1 是正常的）
		hasCacheTokens := claudeResp.Usage.CacheCreationInputTokens > 0 || claudeResp.Usage.CacheReadInputTokens > 0

		// 只有在没有缓存 token 的情况下才补全 input_tokens
		if claudeResp.Usage.InputTokens <= 1 && !hasCacheTokens {
			claudeResp.Usage.InputTokens = utils.EstimateRequestTokens(requestBody)
			patched = true
		}
		if claudeResp.Usage.OutputTokens <= 1 {
			claudeResp.Usage.OutputTokens = utils.EstimateResponseTokens(claudeResp.Content)
			patched = true
		}
		if envCfg.EnableResponseLogs {
			if patched {
				log.Printf("🔢 [Token补全] 虚假值: InputTokens=%d→%d, OutputTokens=%d→%d",
					originalInput, claudeResp.Usage.InputTokens, originalOutput, claudeResp.Usage.OutputTokens)
			}
			// 记录完整的 token 信息
			log.Printf("🔢 [Token统计] InputTokens=%d, OutputTokens=%d, CacheCreationInputTokens=%d, CacheReadInputTokens=%d, PromptTokens=%d, CompletionTokens=%d",
				claudeResp.Usage.InputTokens, claudeResp.Usage.OutputTokens,
				claudeResp.Usage.CacheCreationInputTokens, claudeResp.Usage.CacheReadInputTokens,
				claudeResp.Usage.PromptTokens, claudeResp.Usage.CompletionTokens)
		}
	}

	// 监听客户端断开连接
	ctx := c.Request.Context()
	go func() {
		<-ctx.Done()
		// 检查响应是否已完成
		if !c.Writer.Written() {
			if envCfg.EnableResponseLogs {
				responseTime := time.Since(startTime).Milliseconds()
				log.Printf("⏱️ 响应中断: %dms, 状态: %d", responseTime, resp.StatusCode)
			}
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

	// 设置响应头
	setupStreamHeaders(c, resp)

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("⚠️ ResponseWriter不支持Flush接口")
		return
	}
	flusher.Flush()

	// 初始化流处理上下文
	ctx := newStreamContext(envCfg, upstream)

	// 事件循环
	processStreamEvents(c, w, flusher, eventChan, errChan, ctx, envCfg, startTime, requestBody)
}

// setupStreamHeaders 设置流式响应头
func setupStreamHeaders(c *gin.Context, resp *http.Response) {
	utils.ForwardResponseHeaders(resp.Header, c.Writer)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(200)
}

// streamContext 流处理上下文
type streamContext struct {
	logBuffer        bytes.Buffer
	outputTextBuffer bytes.Buffer
	synthesizer      *utils.StreamSynthesizer
	loggingEnabled   bool
	clientGone       bool
	hasUsage         bool
	needTokenPatch   bool
	// 累积的 token 统计（从流事件中收集，借鉴 new-api 的设计）
	// message_start: 获取 input_tokens 和 cache tokens
	// message_delta: 获取最终的 output_tokens，如果 input_tokens > 0 则更新
	collectedUsage collectedUsageData
}

func newStreamContext(envCfg *config.EnvConfig, upstream *config.UpstreamConfig) *streamContext {
	ctx := &streamContext{
		loggingEnabled: envCfg.IsDevelopment() && envCfg.EnableResponseLogs,
	}
	if ctx.loggingEnabled {
		// 所有 Provider 的 HandleStreamResponse 都会将响应转换为 Claude SSE 格式
		// 因此日志合成器应该使用 "claude" 类型来解析转换后的事件
		ctx.synthesizer = utils.NewStreamSynthesizer("claude")
	}
	return ctx
}

// processStreamEvents 处理流事件循环
func processStreamEvents(c *gin.Context, w gin.ResponseWriter, flusher http.Flusher, eventChan <-chan string, errChan <-chan error, ctx *streamContext, envCfg *config.EnvConfig, startTime time.Time, requestBody []byte) {
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				logStreamCompletion(ctx, envCfg, startTime)
				return
			}
			processStreamEvent(c, w, flusher, event, ctx, envCfg, requestBody)

		case err, ok := <-errChan:
			if !ok {
				continue
			}
			if err != nil {
				log.Printf("💥 流式传输错误: %v", err)
				logPartialResponse(ctx, envCfg)
				return
			}
		}
	}
}

// processStreamEvent 处理单个流事件
func processStreamEvent(c *gin.Context, w gin.ResponseWriter, flusher http.Flusher, event string, ctx *streamContext, envCfg *config.EnvConfig, requestBody []byte) {
	// 提取文本用于估算 token（必须在检测 usage 之前，确保累积内容）
	extractTextFromEvent(event, &ctx.outputTextBuffer)

	// 检测并收集 usage（借鉴 new-api 的设计，持续从流事件中收集 token 统计）
	// message_start: 获取 input_tokens 和 cache tokens
	// message_delta: 获取最终的 output_tokens，如果 input_tokens > 0 则更新
	hasUsage, needPatch, usageData := checkEventUsageStatus(event, envCfg.EnableResponseLogs && envCfg.ShouldLog("debug"))
	if hasUsage {
		// 首次检测到 usage
		if !ctx.hasUsage {
			ctx.hasUsage = true
			ctx.needTokenPatch = needPatch
			if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") && needPatch && !isMessageDeltaEvent(event) {
				log.Printf("🔢 [Stream-Token] 检测到虚假值, 延迟到流结束修补")
			}
		}
		// 累积收集 usage 数据
		// InputTokens: 取最大值（避免中间更新的真实值被最终事件的旧值覆盖）
		// OutputTokens: 取最大值（最终事件的 output_tokens 通常是最准确的）
		if usageData.InputTokens > ctx.collectedUsage.InputTokens {
			ctx.collectedUsage.InputTokens = usageData.InputTokens
		}
		if usageData.OutputTokens > ctx.collectedUsage.OutputTokens {
			ctx.collectedUsage.OutputTokens = usageData.OutputTokens
		}
		if usageData.CacheCreationInputTokens > 0 {
			ctx.collectedUsage.CacheCreationInputTokens = usageData.CacheCreationInputTokens
		}
		if usageData.CacheReadInputTokens > 0 {
			ctx.collectedUsage.CacheReadInputTokens = usageData.CacheReadInputTokens
		}
	}

	// 日志缓存
	if ctx.loggingEnabled {
		ctx.logBuffer.WriteString(event)
		if ctx.synthesizer != nil {
			for _, line := range strings.Split(event, "\n") {
				ctx.synthesizer.ProcessLine(line)
			}
		}
	}

	// 在 message_stop 前注入 usage（上游完全没有 usage 的情况）
	if !ctx.hasUsage && !ctx.clientGone && isMessageStopEvent(event) {
		usageEvent := buildUsageEvent(requestBody, ctx.outputTextBuffer.String())
		if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
			log.Printf("🔢 [Stream-Token注入] 上游无usage, 注入本地估算事件")
		}
		w.Write([]byte(usageEvent))
		flusher.Flush()
		ctx.hasUsage = true
	}

	// 修补 token（在 message_delta 或 message_stop 时修补，确保内容已完整累积）
	eventToSend := event
	if ctx.needTokenPatch && hasEventWithUsage(event) {
		// 只在流结束事件（message_delta 或 message_stop）时修补
		if isMessageDeltaEvent(event) || isMessageStopEvent(event) {
			// 优先使用收集到的真实 token 值，否则使用估算值（借鉴 new-api 的容错设计）
			inputTokens := ctx.collectedUsage.InputTokens
			if inputTokens == 0 {
				inputTokens = utils.EstimateRequestTokens(requestBody)
			}
			outputTokens := ctx.collectedUsage.OutputTokens
			if outputTokens == 0 {
				outputTokens = utils.EstimateTokens(ctx.outputTextBuffer.String())
			}
			// 传递已收集的缓存 token 信息，避免从最终事件中读取（最终事件通常不含缓存字段）
			hasCacheTokens := ctx.collectedUsage.CacheCreationInputTokens > 0 || ctx.collectedUsage.CacheReadInputTokens > 0
			eventToSend = patchTokensInEvent(event, inputTokens, outputTokens, hasCacheTokens, envCfg.EnableResponseLogs && envCfg.ShouldLog("debug"))
			ctx.needTokenPatch = false
		}
	}

	// 转发给客户端
	if !ctx.clientGone {
		if _, err := w.Write([]byte(eventToSend)); err != nil {
			ctx.clientGone = true
			if !isClientDisconnectError(err) {
				log.Printf("⚠️ 流式传输写入错误: %v", err)
			} else if envCfg.ShouldLog("info") {
				log.Printf("ℹ️ 客户端中断连接 (正常行为)，继续接收上游数据...")
			}
		} else {
			flusher.Flush()
		}
	}
}

// isClientDisconnectError 判断是否为客户端断开连接错误
func isClientDisconnectError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset")
}

// logStreamCompletion 记录流完成日志
func logStreamCompletion(ctx *streamContext, envCfg *config.EnvConfig, startTime time.Time) {
	if !envCfg.EnableResponseLogs {
		return
	}
	log.Printf("⏱️ 流式响应完成: %dms", time.Since(startTime).Milliseconds())

	if envCfg.IsDevelopment() {
		logSynthesizedContent(ctx)
	}
}

// logPartialResponse 记录部分响应日志
func logPartialResponse(ctx *streamContext, envCfg *config.EnvConfig) {
	if envCfg.EnableResponseLogs && envCfg.IsDevelopment() {
		logSynthesizedContent(ctx)
	}
}

// logSynthesizedContent 记录合成内容
func logSynthesizedContent(ctx *streamContext) {
	if ctx.synthesizer != nil {
		content := ctx.synthesizer.GetSynthesizedContent()
		if content != "" && !ctx.synthesizer.IsParseFailed() {
			log.Printf("🛰️  上游流式响应合成内容:\n%s", strings.TrimSpace(content))
			return
		}
	}
	if ctx.logBuffer.Len() > 0 {
		log.Printf("🛰️  上游流式响应原始内容:\n%s", ctx.logBuffer.String())
	}
}

// shouldRetryWithNextKey 判断是否应该使用下一个密钥重试
// 返回: (shouldFailover bool, isQuotaRelated bool)
//
// HTTP 状态码分类策略：
//   - 4xx 客户端错误：部分应触发 failover（密钥/配额问题）
//   - 5xx 服务端错误：应触发 failover（上游临时故障）
//   - 2xx/3xx：不应触发 failover（成功或重定向）
//
// isQuotaRelated 标记用于调度器优先级调整：
//   - true: 额度/配额相关，降低密钥优先级
//   - false: 临时错误，不影响优先级
func shouldRetryWithNextKey(statusCode int, bodyBytes []byte) (bool, bool) {
	// 第一层：基于状态码的快速分类
	shouldFailover, isQuotaRelated := classifyByStatusCode(statusCode)
	if shouldFailover {
		return true, isQuotaRelated
	}

	// 第二层：解析响应体，检查错误消息
	// 用于 400/408 等需要进一步判断的状态码
	msgFailover, msgQuota := classifyByErrorMessage(bodyBytes)
	if msgFailover {
		return true, msgQuota
	}

	return false, false
}

// classifyByStatusCode 基于 HTTP 状态码分类
// 返回: (shouldFailover bool, isQuotaRelated bool)
func classifyByStatusCode(statusCode int) (bool, bool) {
	switch {
	// === 认证/授权错误 (应 failover，非配额相关) ===
	case statusCode == 401: // Unauthorized - 密钥无效
		return true, false
	case statusCode == 403: // Forbidden - 权限不足
		return true, false

	// === 配额/计费错误 (应 failover，配额相关) ===
	case statusCode == 402: // Payment Required - 余额不足、订阅过期
		return true, true
	case statusCode == 429: // Too Many Requests - 速率限制、配额耗尽
		return true, true

	// === 超时错误 (应 failover，非配额相关) ===
	case statusCode == 408: // Request Timeout - 上游超时，应尝试其他密钥/渠道
		return true, false

	// === 需要检查消息体的状态码 (交给第二层判断) ===
	case statusCode == 400: // Bad Request - 可能是密钥无效、配额问题等，需检查消息体
		return false, false

	// === 请求错误 (不应 failover，客户端问题) ===
	case statusCode == 404: // Not Found - 端点不存在，换密钥无意义
		return false, false
	case statusCode == 405: // Method Not Allowed
		return false, false
	case statusCode == 406: // Not Acceptable
		return false, false
	case statusCode == 409: // Conflict
		return false, false
	case statusCode == 410: // Gone
		return false, false
	case statusCode == 411: // Length Required
		return false, false
	case statusCode == 412: // Precondition Failed
		return false, false
	case statusCode == 413: // Payload Too Large
		return false, false
	case statusCode == 414: // URI Too Long
		return false, false
	case statusCode == 415: // Unsupported Media Type
		return false, false
	case statusCode == 416: // Range Not Satisfiable
		return false, false
	case statusCode == 417: // Expectation Failed
		return false, false
	case statusCode == 422: // Unprocessable Entity - 请求格式正确但语义错误
		return false, false
	case statusCode == 423: // Locked
		return false, false
	case statusCode == 424: // Failed Dependency
		return false, false
	case statusCode == 426: // Upgrade Required
		return false, false
	case statusCode == 428: // Precondition Required
		return false, false
	case statusCode == 431: // Request Header Fields Too Large
		return false, false
	case statusCode == 451: // Unavailable For Legal Reasons
		return false, false

	// === 服务端错误 (应 failover，非配额相关) ===
	case statusCode >= 500: // 5xx 服务端错误
		return true, false

	// === 其他 4xx (保守处理，不 failover) ===
	case statusCode >= 400 && statusCode < 500:
		return false, false

	// === 成功/重定向 (不应 failover) ===
	default:
		return false, false
	}
}

// classifyByErrorMessage 基于错误消息内容分类
// 用于处理状态码无法明确判断的情况（如 400/408 错误）
// 返回: (shouldFailover bool, isQuotaRelated bool)
func classifyByErrorMessage(bodyBytes []byte) (bool, bool) {
	var errResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &errResp); err != nil {
		return false, false
	}

	errObj, ok := errResp["error"].(map[string]interface{})
	if !ok {
		return false, false
	}

	// 检查 error.message 字段
	if msg, ok := errObj["message"].(string); ok {
		if failover, quota := classifyMessage(msg); failover {
			return true, quota
		}
	}

	// 检查 error.type 字段
	if errType, ok := errObj["type"].(string); ok {
		if failover, quota := classifyErrorType(errType); failover {
			return true, quota
		}
	}

	return false, false
}

// classifyMessage 基于错误消息内容分类
// 返回: (shouldFailover bool, isQuotaRelated bool)
func classifyMessage(msg string) (bool, bool) {
	msgLower := strings.ToLower(msg)

	// 配额/余额相关关键词 (failover + quota)
	quotaKeywords := []string{
		"insufficient", "quota", "credit", "balance",
		"rate limit", "limit exceeded", "exceeded",
		"billing", "payment", "subscription",
		"积分不足", "余额不足", "请求数限制", "额度",
	}
	for _, keyword := range quotaKeywords {
		if strings.Contains(msgLower, keyword) {
			return true, true
		}
	}

	// 认证/授权相关关键词 (failover + 非 quota)
	authKeywords := []string{
		"invalid", "unauthorized", "authentication",
		"api key", "apikey", "token", "expired",
		"permission", "forbidden", "denied",
		"密钥无效", "认证失败", "权限不足",
	}
	for _, keyword := range authKeywords {
		if strings.Contains(msgLower, keyword) {
			return true, false
		}
	}

	// 临时错误关键词 (failover + 非 quota)
	transientKeywords := []string{
		"timeout", "timed out", "temporarily",
		"overloaded", "unavailable", "retry",
		"server error", "internal error",
		"超时", "暂时", "重试",
	}
	for _, keyword := range transientKeywords {
		if strings.Contains(msgLower, keyword) {
			return true, false
		}
	}

	return false, false
}

// classifyErrorType 基于错误类型分类
// 返回: (shouldFailover bool, isQuotaRelated bool)
func classifyErrorType(errType string) (bool, bool) {
	typeLower := strings.ToLower(errType)

	// 配额相关的错误类型 (failover + quota)
	quotaTypes := []string{
		"over_quota", "quota_exceeded", "rate_limit",
		"billing", "insufficient", "payment",
	}
	for _, t := range quotaTypes {
		if strings.Contains(typeLower, t) {
			return true, true
		}
	}

	// 认证相关的错误类型 (failover + 非 quota)
	authTypes := []string{
		"authentication", "authorization", "permission",
		"invalid_api_key", "invalid_token", "expired",
	}
	for _, t := range authTypes {
		if strings.Contains(typeLower, t) {
			return true, false
		}
	}

	// 服务端错误类型 (failover + 非 quota)
	serverTypes := []string{
		"server_error", "internal_error", "service_unavailable",
		"timeout", "overloaded",
	}
	for _, t := range serverTypes {
		if strings.Contains(typeLower, t) {
			return true, false
		}
	}

	return false, false
}

// logUsageDetection 统一格式输出 usage 检测日志
func logUsageDetection(location string, usage map[string]interface{}, needPatch bool) {
	inputTokens := usage["input_tokens"]
	outputTokens := usage["output_tokens"]
	cacheCreation, _ := usage["cache_creation_input_tokens"].(float64)
	cacheRead, _ := usage["cache_read_input_tokens"].(float64)

	log.Printf("🔢 [Stream-Token检测] %s: InputTokens=%v, OutputTokens=%v, CacheCreation=%.0f, CacheRead=%.0f, 需补全=%v",
		location, inputTokens, outputTokens, cacheCreation, cacheRead, needPatch)
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

// collectedUsageData 从流事件中收集的 usage 数据
type collectedUsageData struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// extractUsageFromMap 从 usage map 中提取 token 数据
func extractUsageFromMap(usage map[string]interface{}) collectedUsageData {
	var data collectedUsageData
	if v, ok := usage["input_tokens"].(float64); ok {
		data.InputTokens = int(v)
	}
	if v, ok := usage["output_tokens"].(float64); ok {
		data.OutputTokens = int(v)
	}
	if v, ok := usage["cache_creation_input_tokens"].(float64); ok {
		data.CacheCreationInputTokens = int(v)
	}
	if v, ok := usage["cache_read_input_tokens"].(float64); ok {
		data.CacheReadInputTokens = int(v)
	}
	return data
}

// checkEventUsageStatus 检测事件是否包含 usage 字段，并判断是否需要修补 input_tokens/output_tokens
// 返回: (hasUsage bool, needPatch bool, usageData collectedUsageData)
func checkEventUsageStatus(event string, enableLog bool) (bool, bool, collectedUsageData) {
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		// 检查顶层 usage 字段（通常在 message_delta 事件）
		if hasUsage, needInputPatch, needOutputPatch := checkUsageFieldsWithPatch(data["usage"]); hasUsage {
			needPatch := needInputPatch || needOutputPatch
			var usageData collectedUsageData
			if usage, ok := data["usage"].(map[string]interface{}); ok {
				if enableLog {
					logUsageDetection("顶层usage", usage, needPatch)
				}
				usageData = extractUsageFromMap(usage)
			}
			return true, needPatch, usageData
		}

		// 检查 message.usage（Claude message_start 事件格式）
		if msg, ok := data["message"].(map[string]interface{}); ok {
			if hasUsage, needInputPatch, needOutputPatch := checkUsageFieldsWithPatch(msg["usage"]); hasUsage {
				needPatch := needInputPatch || needOutputPatch
				var usageData collectedUsageData
				if usage, ok := msg["usage"].(map[string]interface{}); ok {
					if enableLog {
						logUsageDetection("message.usage", usage, needPatch)
					}
					usageData = extractUsageFromMap(usage)
				}
				return true, needPatch, usageData
			}
		}
	}
	return false, false, collectedUsageData{}
}

// checkUsageFieldsWithPatch 检查 usage 对象是否包含 token 字段，并判断是否需要修补
// 返回: (hasUsage bool, needInputTokenPatch bool, needOutputTokenPatch bool)
// 注意：如果有 cache_creation_input_tokens 或 cache_read_input_tokens，则 input_tokens 为 0/1 是正常的
func checkUsageFieldsWithPatch(usage interface{}) (bool, bool, bool) {
	if u, ok := usage.(map[string]interface{}); ok {
		inputTokens, hasInput := u["input_tokens"]
		outputTokens, hasOutput := u["output_tokens"]
		if hasInput || hasOutput {
			needInputPatch := false
			needOutputPatch := false

			// 检查是否有缓存 token（如果有，input_tokens 为 0/1 是正常的）
			cacheCreation, _ := u["cache_creation_input_tokens"].(float64)
			cacheRead, _ := u["cache_read_input_tokens"].(float64)
			hasCacheTokens := cacheCreation > 0 || cacheRead > 0

			if hasInput {
				// 只有在没有缓存 token 的情况下才标记需要补全 input_tokens
				if v, ok := inputTokens.(float64); ok && v <= 1 && !hasCacheTokens {
					needInputPatch = true
				}
			}
			if hasOutput {
				if v, ok := outputTokens.(float64); ok && v <= 1 {
					needOutputPatch = true
				}
			}
			return true, needInputPatch, needOutputPatch
		}
	}
	return false, false, false
}

// hasEventWithUsage 检查事件是否包含 usage 字段（用于判断是否需要修补）
func hasEventWithUsage(event string) bool {
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
		if _, ok := data["usage"].(map[string]interface{}); ok {
			return true
		}

		// 检查 message.usage
		if msg, ok := data["message"].(map[string]interface{}); ok {
			if _, ok := msg["usage"].(map[string]interface{}); ok {
				return true
			}
		}
	}
	return false
}

// patchTokensInEvent 修补事件中的 input_tokens 和 output_tokens 字段
// hasCacheTokens: 从 ctx.collectedUsage 传入，判断是否为缓存请求（不能从当前事件读取，因为最终事件通常不含缓存字段）
func patchTokensInEvent(event string, estimatedInputTokens, estimatedOutputTokens int, hasCacheTokens bool, enableLog bool) string {
	var result strings.Builder
	lines := strings.Split(event, "\n")

	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// 修补顶层 usage
		if usage, ok := data["usage"].(map[string]interface{}); ok {
			patchUsageFieldsWithLog(usage, estimatedInputTokens, estimatedOutputTokens, hasCacheTokens, enableLog, "顶层usage")
		}

		// 修补 message.usage
		if msg, ok := data["message"].(map[string]interface{}); ok {
			if usage, ok := msg["usage"].(map[string]interface{}); ok {
				patchUsageFieldsWithLog(usage, estimatedInputTokens, estimatedOutputTokens, hasCacheTokens, enableLog, "message.usage")
			}
		}

		// 重新序列化
		patchedJSON, err := json.Marshal(data)
		if err != nil {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		result.WriteString("data: ")
		result.Write(patchedJSON)
		result.WriteString("\n")
	}

	return result.String()
}

// patchUsageFieldsWithLog 修补 usage 对象中的 token 字段，并输出日志
// hasCacheTokens: 从 ctx.collectedUsage 传入（而非从当前事件读取），因为最终事件通常不含缓存字段
// estimatedInput/estimatedOutput: 收集到的最大值（或估算值）
func patchUsageFieldsWithLog(usage map[string]interface{}, estimatedInput, estimatedOutput int, hasCacheTokens bool, enableLog bool, location string) {
	originalInput := usage["input_tokens"]
	originalOutput := usage["output_tokens"]
	inputPatched := false
	outputPatched := false

	// 从当前事件读取缓存 token（仅用于日志输出，不用于判断是否补全）
	cacheCreation, _ := usage["cache_creation_input_tokens"].(float64)
	cacheRead, _ := usage["cache_read_input_tokens"].(float64)
	promptTokens, _ := usage["prompt_tokens"].(float64)
	completionTokens, _ := usage["completion_tokens"].(float64)

	// 补全 input_tokens：
	// 1. 如果当前值 <= 1 且没有缓存 token，使用收集到的值
	// 2. 如果收集到的值 > 当前值且没有缓存 token，也使用收集到的值（中间事件可能有更准确的值）
	// 注意：缓存请求合法地报告 input_tokens 为 0/1，不应被覆盖
	if v, ok := usage["input_tokens"].(float64); ok {
		currentInput := int(v)
		if !hasCacheTokens && ((currentInput <= 1) || (estimatedInput > currentInput && estimatedInput > 1)) {
			usage["input_tokens"] = estimatedInput
			inputPatched = true
		}
	}
	// 补全 output_tokens：
	// 1. 如果当前值 <= 1，使用收集到的值
	// 2. 如果收集到的值 > 当前值，也使用收集到的值
	if v, ok := usage["output_tokens"].(float64); ok {
		currentOutput := int(v)
		if currentOutput <= 1 || (estimatedOutput > currentOutput && estimatedOutput > 1) {
			usage["output_tokens"] = estimatedOutput
			outputPatched = true
		}
	}

	if enableLog {
		if inputPatched || outputPatched {
			log.Printf("🔢 [Stream-Token补全] %s: InputTokens=%v→%v, OutputTokens=%v→%v",
				location, originalInput, usage["input_tokens"], originalOutput, usage["output_tokens"])
		}
		// 记录完整的 token 信息
		log.Printf("🔢 [Stream-Token统计] %s: InputTokens=%v, OutputTokens=%v, CacheCreationInputTokens=%.0f, CacheReadInputTokens=%.0f, PromptTokens=%.0f, CompletionTokens=%.0f",
			location, usage["input_tokens"], usage["output_tokens"], cacheCreation, cacheRead, promptTokens, completionTokens)
	}
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

// isMessageDeltaEvent 检测是否为 message_delta 事件（流结束时包含最终 usage）
func isMessageDeltaEvent(event string) bool {
	if strings.Contains(event, "event: message_delta") {
		return true
	}
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}
		if data["type"] == "message_delta" {
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

// CountTokensHandler 处理 /v1/messages/count_tokens 请求
// 支持两种模式：
// 1. 代理模式：转发到上游获取精确计数（需要上游支持）
// 2. 本地估算模式：使用本地算法快速估算（默认）
func CountTokensHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager, channelScheduler *scheduler.ChannelScheduler) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 认证
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		// 读取请求体
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(400, gin.H{"error": "Failed to read request body"})
			return
		}

		// 解析请求
		var req struct {
			Model    string      `json:"model"`
			System   interface{} `json:"system"`
			Messages interface{} `json:"messages"`
			Tools    interface{} `json:"tools"`
		}
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid JSON"})
			return
		}

		// 本地估算 token 数量
		inputTokens := utils.EstimateRequestTokens(bodyBytes)

		// 返回 Claude API 兼容的响应格式
		c.JSON(200, gin.H{
			"input_tokens": inputTokens,
		})

		if envCfg.EnableResponseLogs {
			log.Printf("🔢 [CountTokens] 本地估算: model=%s, input_tokens=%d", req.Model, inputTokens)
		}
	}
}
