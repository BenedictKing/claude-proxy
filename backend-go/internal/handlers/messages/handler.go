// Package messages 提供 Claude Messages API 的处理器
package messages

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/handlers/common"
	"github.com/BenedictKing/claude-proxy/internal/middleware"
	"github.com/BenedictKing/claude-proxy/internal/providers"
	"github.com/BenedictKing/claude-proxy/internal/scheduler"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
	"github.com/gin-gonic/gin"
)

// Handler Messages API 代理处理器
// 支持多渠道调度：当配置多个渠道时自动启用
func Handler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager, channelScheduler *scheduler.ChannelScheduler) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// 先进行认证
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		startTime := time.Now()

		// 读取请求体
		bodyBytes, err := common.ReadRequestBody(c, envCfg.MaxRequestBodySize)
		if err != nil {
			return
		}

		// 解析请求
		var claudeReq types.ClaudeRequest
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &claudeReq)
		}

		// 提取 user_id 用于 Trace 亲和性
		userID := common.ExtractUserID(bodyBytes)

		// 记录原始请求信息（仅在入口处记录一次）
		common.LogOriginalRequest(c, bodyBytes, envCfg, "Messages")

		// 检查是否为多渠道模式
		isMultiChannel := channelScheduler.IsMultiChannelMode(false)

		if isMultiChannel {
			handleMultiChannel(c, envCfg, cfgManager, channelScheduler, bodyBytes, claudeReq, userID, startTime)
		} else {
			handleSingleChannel(c, envCfg, cfgManager, channelScheduler, bodyBytes, claudeReq, startTime)
		}
	})
}

// handleMultiChannel 处理多渠道代理请求
func handleMultiChannel(
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
	var lastFailoverError *common.FailoverError

	maxChannelAttempts := channelScheduler.GetActiveChannelCount(false)

	for channelAttempt := 0; channelAttempt < maxChannelAttempts; channelAttempt++ {
		selection, err := channelScheduler.SelectChannel(c.Request.Context(), userID, failedChannels, false)
		if err != nil {
			lastError = err
			break
		}

		upstream := selection.Upstream
		channelIndex := selection.ChannelIndex

		if envCfg.ShouldLog("info") {
			log.Printf("[Messages-Select] 选择渠道: [%d] %s (原因: %s, 尝试 %d/%d)",
				channelIndex, upstream.Name, selection.Reason, channelAttempt+1, maxChannelAttempts)
		}

		success, _, _, failoverErr := tryChannelWithAllKeys(c, envCfg, cfgManager, channelScheduler, upstream, channelIndex, bodyBytes, claudeReq, startTime)

		if success {
			channelScheduler.SetTraceAffinity(userID, channelIndex)
			return
		}

		failedChannels[channelIndex] = true

		if failoverErr != nil {
			lastFailoverError = failoverErr
			lastError = fmt.Errorf("渠道 [%d] %s 失败", channelIndex, upstream.Name)
		}

		log.Printf("[Messages-Failover] 警告: 渠道 [%d] %s 所有密钥都失败，尝试下一个渠道", channelIndex, upstream.Name)
	}

	log.Printf("[Messages-Error] 所有渠道都失败了")
	common.HandleAllChannelsFailed(c, cfgManager.GetFuzzyModeEnabled(), lastFailoverError, lastError, "Messages")
}

// tryChannelWithAllKeys 尝试使用渠道的所有密钥（纯 failover 模式）
// 返回: success, successKey, successBaseURLIdx, failoverError
func tryChannelWithAllKeys(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	upstream *config.UpstreamConfig,
	channelIndex int,
	bodyBytes []byte,
	claudeReq types.ClaudeRequest,
	startTime time.Time,
) (bool, string, int, *common.FailoverError) {
	if len(upstream.APIKeys) == 0 {
		return false, "", 0, nil
	}

	provider := providers.GetProvider(upstream.ServiceType)
	if provider == nil {
		return false, "", 0, nil
	}

	metricsManager := channelScheduler.GetMessagesMetricsManager()
	baseURLs := upstream.GetAllBaseURLs()

	// 获取动态排序后的 URL 列表（非阻塞，立即返回）
	sortedURLResults := channelScheduler.GetSortedURLsForChannel(channelIndex, baseURLs)

	var lastFailoverError *common.FailoverError
	deprioritizeCandidates := make(map[string]bool)

	// 强制探测模式
	forceProbeMode := common.AreAllKeysSuspended(metricsManager, upstream.BaseURL, upstream.APIKeys)
	if forceProbeMode {
		log.Printf("[Messages-ForceProbe] 渠道 %s 所有 Key 都被熔断，启用强制探测模式", upstream.Name)
	}

	// 纯 failover：按预热排序遍历所有 BaseURL，每个 BaseURL 尝试所有 Key
	for sortedIdx, urlResult := range sortedURLResults {
		currentBaseURL := urlResult.URL
		originalIdx := urlResult.OriginalIdx // 原始索引用于指标记录
		failedKeys := make(map[string]bool)  // 每个 BaseURL 重置失败 Key 列表
		maxRetries := len(upstream.APIKeys)

		for attempt := 0; attempt < maxRetries; attempt++ {
			common.RestoreRequestBody(c, bodyBytes)

			// 按优先级顺序选择下一个可用 Key
			apiKey, err := cfgManager.GetNextAPIKey(upstream, failedKeys)
			if err != nil {
				break // 当前 BaseURL 没有可用 Key，尝试下一个 BaseURL
			}

			// 检查熔断状态
			if !forceProbeMode && metricsManager.ShouldSuspendKey(currentBaseURL, apiKey) {
				failedKeys[apiKey] = true
				log.Printf("[Messages-Circuit] 跳过熔断中的 Key: %s", utils.MaskAPIKey(apiKey))
				continue
			}

			if envCfg.ShouldLog("info") {
				log.Printf("[Messages-Key] 使用API密钥: %s (BaseURL %d/%d, 尝试 %d/%d)", utils.MaskAPIKey(apiKey), sortedIdx+1, len(sortedURLResults), attempt+1, maxRetries)
			}

			// 使用深拷贝避免并发修改问题
			upstreamCopy := upstream.Clone()
			upstreamCopy.BaseURL = currentBaseURL

			providerReq, _, err := provider.ConvertToProviderRequest(c, upstreamCopy, apiKey)

			if err != nil {
				failedKeys[apiKey] = true
				channelScheduler.RecordFailure(currentBaseURL, apiKey, false)
				continue
			}

			resp, err := common.SendRequest(providerReq, upstream, envCfg, claudeReq.Stream)
			if err != nil {
				failedKeys[apiKey] = true
				cfgManager.MarkKeyAsFailed(apiKey)
				channelScheduler.RecordFailure(currentBaseURL, apiKey, false)
				// 网络错误（超时等）触发 URL 动态降级
				channelScheduler.MarkURLFailure(channelIndex, currentBaseURL)
				log.Printf("[Messages-Key] 警告: API密钥失败: %v", err)
				continue
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				respBodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				respBodyBytes = utils.DecompressGzipIfNeeded(resp, respBodyBytes)

				shouldFailover, isQuotaRelated := common.ShouldRetryWithNextKey(resp.StatusCode, respBodyBytes, cfgManager.GetFuzzyModeEnabled())
				log.Printf("[Messages-Failover] ShouldRetryWithNextKey: statusCode=%d, shouldFailover=%v, isQuotaRelated=%v", resp.StatusCode, shouldFailover, isQuotaRelated)
				if shouldFailover {
					failedKeys[apiKey] = true
					cfgManager.MarkKeyAsFailed(apiKey)
					channelScheduler.RecordFailure(currentBaseURL, apiKey, false)
					// HTTP 5xx 等错误也触发 URL 动态降级
					channelScheduler.MarkURLFailure(channelIndex, currentBaseURL)
					log.Printf("[Messages-Key] 警告: API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)

					if envCfg.EnableResponseLogs && envCfg.IsDevelopment() {
						var formattedBody string
						if envCfg.RawLogOutput {
							formattedBody = utils.FormatJSONBytesRaw(respBodyBytes)
						} else {
							formattedBody = utils.FormatJSONBytesForLog(respBodyBytes, 500)
						}
						log.Printf("[Messages-Error] 失败原因:\n%s", formattedBody)
					} else if envCfg.EnableResponseLogs {
						log.Printf("[Messages-Error] 失败原因: %s", string(respBodyBytes))
					}

					lastFailoverError = &common.FailoverError{
						Status: resp.StatusCode,
						Body:   respBodyBytes,
					}

					if isQuotaRelated {
						deprioritizeCandidates[apiKey] = true
						log.Printf("[Messages-Key] 标记密钥为配额相关失败，待降级: %s", utils.MaskAPIKey(apiKey))
					}
					continue
				}

				// 非 failover 错误，记录失败指标后直接返回
				channelScheduler.RecordFailure(currentBaseURL, apiKey, false)
				c.Data(resp.StatusCode, "application/json", respBodyBytes)
				return true, "", 0, nil
			}

			// 处理成功响应
			if len(deprioritizeCandidates) > 0 {
				for key := range deprioritizeCandidates {
					if err := cfgManager.DeprioritizeAPIKey(key); err != nil {
						log.Printf("[Messages-Key] 警告: 密钥降级失败: %v", err)
					}
				}
			}

			// 标记 URL 成功，触发动态排序优化
			channelScheduler.MarkURLSuccess(channelIndex, currentBaseURL)

			if claudeReq.Stream {
				common.HandleStreamResponse(c, resp, provider, envCfg, startTime, upstreamCopy, bodyBytes, channelScheduler, apiKey, claudeReq.Model)
			} else {
				handleNormalResponse(c, resp, provider, envCfg, startTime, bodyBytes, channelScheduler, upstreamCopy, apiKey)
			}
			return true, apiKey, originalIdx, nil
		}
		// 当前 BaseURL 的所有 Key 都失败，记录并尝试下一个 BaseURL
		if sortedIdx < len(sortedURLResults)-1 {
			log.Printf("[Messages-BaseURL] BaseURL %d/%d 所有 Key 失败，切换到下一个 BaseURL", sortedIdx+1, len(sortedURLResults))
		}
	}

	return false, "", 0, lastFailoverError
}

// handleSingleChannel 处理单渠道代理请求
func handleSingleChannel(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	bodyBytes []byte,
	claudeReq types.ClaudeRequest,
	startTime time.Time,
) {
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

	provider := providers.GetProvider(upstream.ServiceType)
	if provider == nil {
		c.JSON(400, gin.H{"error": "Unsupported service type"})
		return
	}

	metricsManager := channelScheduler.GetMessagesMetricsManager()
	baseURLs := upstream.GetAllBaseURLs()

	var lastError error
	var lastFailoverError *common.FailoverError
	deprioritizeCandidates := make(map[string]bool)

	// 强制探测模式：检查首个 BaseURL 的所有 Key 是否都被熔断
	forceProbeMode := common.AreAllKeysSuspended(metricsManager, baseURLs[0], upstream.APIKeys)
	if forceProbeMode {
		log.Printf("[Messages-ForceProbe] 渠道 %s 所有 Key 都被熔断，启用强制探测模式", upstream.Name)
	}

	// 纯 failover：遍历所有 BaseURL，每个 BaseURL 尝试所有 Key
	for baseURLIdx, currentBaseURL := range baseURLs {
		failedKeys := make(map[string]bool) // 每个 BaseURL 重置失败 Key 列表
		maxRetries := len(upstream.APIKeys)

		for attempt := 0; attempt < maxRetries; attempt++ {
			common.RestoreRequestBody(c, bodyBytes)

			apiKey, err := cfgManager.GetNextAPIKey(upstream, failedKeys)
			if err != nil {
				lastError = err
				break // 当前 BaseURL 没有可用 Key，尝试下一个 BaseURL
			}

			// 检查熔断状态
			if !forceProbeMode && metricsManager.ShouldSuspendKey(currentBaseURL, apiKey) {
				failedKeys[apiKey] = true
				log.Printf("[Messages-Circuit] 跳过熔断中的 Key: %s", utils.MaskAPIKey(apiKey))
				continue
			}

			if envCfg.ShouldLog("info") {
				log.Printf("[Messages-Upstream] 使用上游: %s - %s (BaseURL %d/%d, 尝试 %d/%d)", upstream.Name, currentBaseURL, baseURLIdx+1, len(baseURLs), attempt+1, maxRetries)
				log.Printf("[Messages-Key] 使用API密钥: %s", utils.MaskAPIKey(apiKey))
			}

			// 使用深拷贝避免并发修改问题
			upstreamCopy := upstream.Clone()
			upstreamCopy.BaseURL = currentBaseURL

			providerReq, _, err := provider.ConvertToProviderRequest(c, upstreamCopy, apiKey)

			if err != nil {
				lastError = err
				failedKeys[apiKey] = true
				channelScheduler.RecordFailure(currentBaseURL, apiKey, false)
				continue
			}

			resp, err := common.SendRequest(providerReq, upstream, envCfg, claudeReq.Stream)
			if err != nil {
				lastError = err
				failedKeys[apiKey] = true
				cfgManager.MarkKeyAsFailed(apiKey)
				channelScheduler.RecordFailure(currentBaseURL, apiKey, false)
				log.Printf("[Messages-Key] 警告: API密钥失败: %v", err)
				continue
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				respBodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				respBodyBytes = utils.DecompressGzipIfNeeded(resp, respBodyBytes)

				shouldFailover, isQuotaRelated := common.ShouldRetryWithNextKey(resp.StatusCode, respBodyBytes, cfgManager.GetFuzzyModeEnabled())
				log.Printf("[Messages-Failover] ShouldRetryWithNextKey(SingleChannel): statusCode=%d, shouldFailover=%v, isQuotaRelated=%v", resp.StatusCode, shouldFailover, isQuotaRelated)
				if shouldFailover {
					lastError = fmt.Errorf("上游错误: %d", resp.StatusCode)
					failedKeys[apiKey] = true
					cfgManager.MarkKeyAsFailed(apiKey)
					channelScheduler.RecordFailure(currentBaseURL, apiKey, false)

					log.Printf("[Messages-Key] 警告: API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)
					if envCfg.EnableResponseLogs && envCfg.IsDevelopment() {
						var formattedBody string
						if envCfg.RawLogOutput {
							formattedBody = utils.FormatJSONBytesRaw(respBodyBytes)
						} else {
							formattedBody = utils.FormatJSONBytesForLog(respBodyBytes, 500)
						}
						log.Printf("[Messages-Error] 失败原因:\n%s", formattedBody)
					} else if envCfg.EnableResponseLogs {
						log.Printf("[Messages-Error] 失败原因: %s", string(respBodyBytes))
					}

					lastFailoverError = &common.FailoverError{
						Status: resp.StatusCode,
						Body:   respBodyBytes,
					}

					if isQuotaRelated {
						deprioritizeCandidates[apiKey] = true
						log.Printf("[Messages-Key] 标记密钥为配额相关失败，待降级: %s", utils.MaskAPIKey(apiKey))
					}
					continue
				}

				// 非 failover 错误，记录失败指标后返回
				if envCfg.EnableResponseLogs {
					log.Printf("[Messages-Response] 警告: 上游返回错误: %d", resp.StatusCode)
					if envCfg.IsDevelopment() {
						var formattedBody string
						if envCfg.RawLogOutput {
							formattedBody = utils.FormatJSONBytesRaw(respBodyBytes)
						} else {
							formattedBody = utils.FormatJSONBytesForLog(respBodyBytes, 500)
						}
						log.Printf("[Messages-Response] 错误响应体:\n%s", formattedBody)

						respHeaders := make(map[string]string)
						for key, values := range resp.Header {
							if len(values) > 0 {
								respHeaders[key] = values[0]
							}
						}
						var respHeadersJSON []byte
						if envCfg.RawLogOutput {
							respHeadersJSON, _ = json.Marshal(respHeaders)
						} else {
							respHeadersJSON, _ = json.MarshalIndent(respHeaders, "", "  ")
						}
						log.Printf("[Messages-Response] 错误响应头:\n%s", string(respHeadersJSON))
					}
				}
				channelScheduler.RecordFailure(currentBaseURL, apiKey, false)
				c.Data(resp.StatusCode, "application/json", respBodyBytes)
				return
			}

			// 处理成功响应
			if len(deprioritizeCandidates) > 0 {
				for key := range deprioritizeCandidates {
					if err := cfgManager.DeprioritizeAPIKey(key); err != nil {
						log.Printf("[Messages-Key] 警告: 密钥降级失败: %v", err)
					}
				}
			}

			if claudeReq.Stream {
				common.HandleStreamResponse(c, resp, provider, envCfg, startTime, upstreamCopy, bodyBytes, channelScheduler, apiKey, claudeReq.Model)
			} else {
				handleNormalResponse(c, resp, provider, envCfg, startTime, bodyBytes, channelScheduler, upstreamCopy, apiKey)
			}
			return
		}
	}

	log.Printf("[Messages-Error] 所有API密钥都失败了")
	common.HandleAllKeysFailed(c, cfgManager.GetFuzzyModeEnabled(), lastFailoverError, lastError, "Messages")
}

// handleNormalResponse 处理非流式响应
func handleNormalResponse(
	c *gin.Context,
	resp *http.Response,
	provider providers.Provider,
	envCfg *config.EnvConfig,
	startTime time.Time,
	requestBody []byte,
	channelScheduler *scheduler.ChannelScheduler,
	upstream *config.UpstreamConfig,
	apiKey string,
) {
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to read response"})
		return
	}

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("[Messages-Timing] 响应完成: %dms, 状态: %d", responseTime, resp.StatusCode)
		if envCfg.IsDevelopment() {
			respHeaders := make(map[string]string)
			for key, values := range resp.Header {
				if len(values) > 0 {
					respHeaders[key] = values[0]
				}
			}
			var respHeadersJSON []byte
			if envCfg.RawLogOutput {
				respHeadersJSON, _ = json.Marshal(respHeaders)
			} else {
				respHeadersJSON, _ = json.MarshalIndent(respHeaders, "", "  ")
			}
			log.Printf("[Messages-Response] 响应头:\n%s", string(respHeadersJSON))

			var formattedBody string
			if envCfg.RawLogOutput {
				formattedBody = utils.FormatJSONBytesRaw(bodyBytes)
			} else {
				formattedBody = utils.FormatJSONBytesForLog(bodyBytes, 500)
			}
			log.Printf("[Messages-Response] 响应体:\n%s", formattedBody)
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

	// Token 补全逻辑
	if claudeResp.Usage == nil {
		estimatedInput := utils.EstimateRequestTokens(requestBody)
		estimatedOutput := utils.EstimateResponseTokens(claudeResp.Content)
		claudeResp.Usage = &types.Usage{
			InputTokens:  estimatedInput,
			OutputTokens: estimatedOutput,
		}
		if envCfg.EnableResponseLogs {
			log.Printf("[Messages-Token] 上游无Usage, 本地估算: input=%d, output=%d", estimatedInput, estimatedOutput)
		}
	} else {
		originalInput := claudeResp.Usage.InputTokens
		originalOutput := claudeResp.Usage.OutputTokens
		patched := false

		hasCacheTokens := claudeResp.Usage.CacheCreationInputTokens > 0 || claudeResp.Usage.CacheReadInputTokens > 0

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
				log.Printf("[Messages-Token] 虚假值补全: InputTokens=%d->%d, OutputTokens=%d->%d",
					originalInput, claudeResp.Usage.InputTokens, originalOutput, claudeResp.Usage.OutputTokens)
			}
			log.Printf("[Messages-Token] InputTokens=%d, OutputTokens=%d, CacheCreationInputTokens=%d, CacheReadInputTokens=%d, CacheCreation5m=%d, CacheCreation1h=%d, CacheTTL=%s",
				claudeResp.Usage.InputTokens, claudeResp.Usage.OutputTokens,
				claudeResp.Usage.CacheCreationInputTokens, claudeResp.Usage.CacheReadInputTokens,
				claudeResp.Usage.CacheCreation5mInputTokens, claudeResp.Usage.CacheCreation1hInputTokens,
				claudeResp.Usage.CacheTTL)
		}
	}

	// 监听客户端断开连接
	ctx := c.Request.Context()
	go func() {
		<-ctx.Done()
		if !c.Writer.Written() {
			if envCfg.EnableResponseLogs {
				responseTime := time.Since(startTime).Milliseconds()
				log.Printf("[Messages-Timing] 响应中断: %dms, 状态: %d", responseTime, resp.StatusCode)
			}
		}
	}()

	// 转发上游响应头
	utils.ForwardResponseHeaders(resp.Header, c.Writer)

	c.JSON(200, claudeResp)

	// 记录成功指标
	channelScheduler.RecordSuccessWithUsage(upstream.BaseURL, apiKey, claudeResp.Usage, false)

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("[Messages-Timing] 响应发送完成: %dms, 状态: %d", responseTime, resp.StatusCode)
	}
}

// CountTokensHandler 处理 /v1/messages/count_tokens 请求
func CountTokensHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager, channelScheduler *scheduler.ChannelScheduler) gin.HandlerFunc {
	return func(c *gin.Context) {
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		// 使用统一的请求体读取函数，应用大小限制
		bodyBytes, err := common.ReadRequestBody(c, envCfg.MaxRequestBodySize)
		if err != nil {
			// ReadRequestBody 已经返回了错误响应
			return
		}

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

		inputTokens := utils.EstimateRequestTokens(bodyBytes)

		c.JSON(200, gin.H{
			"input_tokens": inputTokens,
		})

		if envCfg.EnableResponseLogs {
			log.Printf("[Messages-Token] CountTokens本地估算: model=%s, input_tokens=%d", req.Model, inputTokens)
		}
	}
}
