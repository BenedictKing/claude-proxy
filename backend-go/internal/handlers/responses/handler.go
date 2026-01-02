// Package responses 提供 Responses API 的处理器
package responses

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/converters"
	"github.com/BenedictKing/claude-proxy/internal/handlers/common"
	"github.com/BenedictKing/claude-proxy/internal/middleware"
	"github.com/BenedictKing/claude-proxy/internal/providers"
	"github.com/BenedictKing/claude-proxy/internal/scheduler"
	"github.com/BenedictKing/claude-proxy/internal/session"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
	"github.com/gin-gonic/gin"
)

// Handler Responses API 代理处理器
// 支持多渠道调度：当配置多个渠道时自动启用
func Handler(
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	sessionManager *session.SessionManager,
	channelScheduler *scheduler.ChannelScheduler,
) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// 先进行认证
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		startTime := time.Now()

		// 读取原始请求体
		maxBodySize := envCfg.MaxRequestBodySize
		bodyBytes, err := common.ReadRequestBody(c, maxBodySize)
		if err != nil {
			return
		}

		// 解析 Responses 请求
		var responsesReq types.ResponsesRequest
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &responsesReq)
		}

		// 提取对话标识用于 Trace 亲和性
		userID := common.ExtractConversationID(c, bodyBytes)

		// 记录原始请求信息（仅在入口处记录一次）
		common.LogOriginalRequest(c, bodyBytes, envCfg, "Responses")

		// 检查是否为多渠道模式
		isMultiChannel := channelScheduler.IsMultiChannelMode(true) // true = isResponses

		if isMultiChannel {
			handleMultiChannel(c, envCfg, cfgManager, channelScheduler, sessionManager, bodyBytes, responsesReq, userID, startTime)
		} else {
			handleSingleChannel(c, envCfg, cfgManager, channelScheduler, sessionManager, bodyBytes, responsesReq, startTime)
		}
	})
}

// handleMultiChannel 处理多渠道 Responses 请求
func handleMultiChannel(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	sessionManager *session.SessionManager,
	bodyBytes []byte,
	responsesReq types.ResponsesRequest,
	userID string,
	startTime time.Time,
) {
	failedChannels := make(map[int]bool)
	var lastError error
	var lastFailoverError *common.FailoverError

	maxChannelAttempts := channelScheduler.GetActiveChannelCount(true) // true = isResponses

	for channelAttempt := 0; channelAttempt < maxChannelAttempts; channelAttempt++ {
		selection, err := channelScheduler.SelectChannel(c.Request.Context(), userID, failedChannels, true)
		if err != nil {
			lastError = err
			break
		}

		upstream := selection.Upstream
		channelIndex := selection.ChannelIndex

		if envCfg.ShouldLog("info") {
			log.Printf("[Responses-Select] 选择渠道: [%d] %s (原因: %s, 尝试 %d/%d)",
				channelIndex, upstream.Name, selection.Reason, channelAttempt+1, maxChannelAttempts)
		}

		success, successKey, successBaseURLIdx, failoverErr, usage := tryChannelWithAllKeys(c, envCfg, cfgManager, channelScheduler, sessionManager, upstream, channelIndex, bodyBytes, responsesReq, startTime)

		if success {
			if successKey != "" {
				channelScheduler.RecordSuccessWithUsage(upstream.GetAllBaseURLs()[successBaseURLIdx], successKey, usage, true)
			}
			channelScheduler.SetTraceAffinity(userID, channelIndex)
			return
		}

		failedChannels[channelIndex] = true

		if failoverErr != nil {
			lastFailoverError = failoverErr
			lastError = fmt.Errorf("渠道 [%d] %s 失败", channelIndex, upstream.Name)
		}

		log.Printf("[Responses-Failover] 警告: 渠道 [%d] %s 所有密钥都失败，尝试下一个渠道", channelIndex, upstream.Name)
	}

	log.Printf("[Responses-Error] 所有渠道都失败了")
	common.HandleAllChannelsFailed(c, cfgManager.GetFuzzyModeEnabled(), lastFailoverError, lastError, "Responses")
}

// tryChannelWithAllKeys 尝试使用 Responses 渠道的所有密钥（纯 failover 模式）
// 返回: success, successKey, successBaseURLIdx, failoverError, usage
func tryChannelWithAllKeys(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	sessionManager *session.SessionManager,
	upstream *config.UpstreamConfig,
	channelIndex int,
	bodyBytes []byte,
	responsesReq types.ResponsesRequest,
	startTime time.Time,
) (bool, string, int, *common.FailoverError, *types.Usage) {
	if len(upstream.APIKeys) == 0 {
		return false, "", 0, nil, nil
	}

	provider := &providers.ResponsesProvider{SessionManager: sessionManager}
	metricsManager := channelScheduler.GetResponsesMetricsManager()
	baseURLs := upstream.GetAllBaseURLs()

	// 获取动态排序后的 URL 列表（非阻塞，立即返回）
	sortedURLResults := channelScheduler.GetSortedURLsForChannel(channelIndex, baseURLs)

	var lastFailoverError *common.FailoverError
	deprioritizeCandidates := make(map[string]bool)

	// 强制探测模式
	forceProbeMode := common.AreAllKeysSuspended(metricsManager, upstream.BaseURL, upstream.APIKeys)
	if forceProbeMode {
		log.Printf("[Responses-ForceProbe] 渠道 %s 所有 Key 都被熔断，启用强制探测模式", upstream.Name)
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
			apiKey, err := cfgManager.GetNextResponsesAPIKey(upstream, failedKeys)
			if err != nil {
				break // 当前 BaseURL 没有可用 Key，尝试下一个 BaseURL
			}

			// 检查熔断状态
			if !forceProbeMode && metricsManager.ShouldSuspendKey(currentBaseURL, apiKey) {
				failedKeys[apiKey] = true
				log.Printf("[Responses-Circuit] 跳过熔断中的 Key: %s", utils.MaskAPIKey(apiKey))
				continue
			}

			if envCfg.ShouldLog("info") {
				log.Printf("[Responses-Key] 使用API密钥: %s (BaseURL %d/%d, 尝试 %d/%d)", utils.MaskAPIKey(apiKey), sortedIdx+1, len(sortedURLResults), attempt+1, maxRetries)
			}

			// 使用深拷贝避免并发修改问题
			upstreamCopy := upstream.Clone()
			upstreamCopy.BaseURL = currentBaseURL

			providerReq, _, err := provider.ConvertToProviderRequest(c, upstreamCopy, apiKey)

			if err != nil {
				failedKeys[apiKey] = true
				channelScheduler.RecordFailure(currentBaseURL, apiKey, true)
				continue
			}

			resp, err := common.SendRequest(providerReq, upstream, envCfg, responsesReq.Stream)
			if err != nil {
				failedKeys[apiKey] = true
				cfgManager.MarkKeyAsFailed(apiKey)
				channelScheduler.RecordFailure(currentBaseURL, apiKey, true)
				// 网络错误（超时等）触发 URL 动态降级
				channelScheduler.MarkURLFailure(channelIndex, currentBaseURL)
				log.Printf("[Responses-Key] 警告: API密钥失败: %v", err)
				continue
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				respBodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				respBodyBytes = utils.DecompressGzipIfNeeded(resp, respBodyBytes)

				shouldFailover, isQuotaRelated := common.ShouldRetryWithNextKey(resp.StatusCode, respBodyBytes, cfgManager.GetFuzzyModeEnabled())
				if shouldFailover {
					failedKeys[apiKey] = true
					cfgManager.MarkKeyAsFailed(apiKey)
					channelScheduler.RecordFailure(currentBaseURL, apiKey, true)
					// HTTP 5xx 等错误也触发 URL 动态降级
					channelScheduler.MarkURLFailure(channelIndex, currentBaseURL)
					log.Printf("[Responses-Key] 警告: API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)

					lastFailoverError = &common.FailoverError{
						Status: resp.StatusCode,
						Body:   respBodyBytes,
					}

					if isQuotaRelated {
						deprioritizeCandidates[apiKey] = true
					}
					continue
				}

				// 非 failover 错误，记录失败指标后返回
				channelScheduler.RecordFailure(currentBaseURL, apiKey, true)
				c.Data(resp.StatusCode, "application/json", respBodyBytes)
				return true, "", 0, nil, nil
			}

			if len(deprioritizeCandidates) > 0 {
				for key := range deprioritizeCandidates {
					_ = cfgManager.DeprioritizeAPIKey(key)
				}
			}

			// 标记 URL 成功，触发动态排序优化
			channelScheduler.MarkURLSuccess(channelIndex, currentBaseURL)

			usage := handleSuccess(c, resp, provider, upstream.ServiceType, envCfg, sessionManager, startTime, &responsesReq, bodyBytes)
			return true, apiKey, originalIdx, nil, usage
		}
		// 当前 BaseURL 的所有 Key 都失败，记录并尝试下一个 BaseURL
		if sortedIdx < len(sortedURLResults)-1 {
			log.Printf("[Responses-BaseURL] BaseURL %d/%d 所有 Key 失败，切换到下一个 BaseURL", sortedIdx+1, len(sortedURLResults))
		}
	}

	return false, "", 0, lastFailoverError, nil
}

// handleSingleChannel 处理单渠道 Responses 请求
func handleSingleChannel(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	sessionManager *session.SessionManager,
	bodyBytes []byte,
	responsesReq types.ResponsesRequest,
	startTime time.Time,
) {
	upstream, err := cfgManager.GetCurrentResponsesUpstream()
	if err != nil {
		c.JSON(503, gin.H{
			"error": "未配置任何 Responses 渠道，请先在管理界面添加渠道",
			"code":  "NO_RESPONSES_UPSTREAM",
		})
		return
	}

	if len(upstream.APIKeys) == 0 {
		c.JSON(503, gin.H{
			"error": fmt.Sprintf("当前 Responses 渠道 \"%s\" 未配置API密钥", upstream.Name),
			"code":  "NO_API_KEYS",
		})
		return
	}

	provider := &providers.ResponsesProvider{SessionManager: sessionManager}

	metricsManager := channelScheduler.GetResponsesMetricsManager()
	baseURLs := upstream.GetAllBaseURLs()

	var lastError error
	var lastFailoverError *common.FailoverError
	deprioritizeCandidates := make(map[string]bool)

	// 强制探测模式：检查首个 BaseURL 的所有 Key 是否都被熔断
	forceProbeMode := common.AreAllKeysSuspended(metricsManager, baseURLs[0], upstream.APIKeys)
	if forceProbeMode {
		log.Printf("[Responses-ForceProbe] 渠道 %s 所有 Key 都被熔断，启用强制探测模式", upstream.Name)
	}

	// 纯 failover：遍历所有 BaseURL，每个 BaseURL 尝试所有 Key
	for baseURLIdx, currentBaseURL := range baseURLs {
		failedKeys := make(map[string]bool) // 每个 BaseURL 重置失败 Key 列表
		maxRetries := len(upstream.APIKeys)

		for attempt := 0; attempt < maxRetries; attempt++ {
			common.RestoreRequestBody(c, bodyBytes)

			apiKey, err := cfgManager.GetNextResponsesAPIKey(upstream, failedKeys)
			if err != nil {
				lastError = err
				break // 当前 BaseURL 没有可用 Key，尝试下一个 BaseURL
			}

			// 检查熔断状态
			if !forceProbeMode && metricsManager.ShouldSuspendKey(currentBaseURL, apiKey) {
				failedKeys[apiKey] = true
				log.Printf("[Responses-Circuit] 跳过熔断中的 Key: %s", utils.MaskAPIKey(apiKey))
				continue
			}

			if envCfg.ShouldLog("info") {
				log.Printf("[Responses-Upstream] 使用 Responses 上游: %s - %s (BaseURL %d/%d, 尝试 %d/%d)", upstream.Name, currentBaseURL, baseURLIdx+1, len(baseURLs), attempt+1, maxRetries)
				log.Printf("[Responses-Key] 使用API密钥: %s", utils.MaskAPIKey(apiKey))
			}

			// 使用深拷贝避免并发修改问题
			upstreamCopy := upstream.Clone()
			upstreamCopy.BaseURL = currentBaseURL

			providerReq, _, err := provider.ConvertToProviderRequest(c, upstreamCopy, apiKey)

			if err != nil {
				lastError = err
				failedKeys[apiKey] = true
				channelScheduler.RecordFailure(currentBaseURL, apiKey, true)
				continue
			}

			resp, err := common.SendRequest(providerReq, upstream, envCfg, responsesReq.Stream)
			if err != nil {
				lastError = err
				failedKeys[apiKey] = true
				cfgManager.MarkKeyAsFailed(apiKey)
				channelScheduler.RecordFailure(currentBaseURL, apiKey, true)
				log.Printf("[Responses-Key] 警告: API密钥失败: %v", err)
				continue
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				respBodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				respBodyBytes = utils.DecompressGzipIfNeeded(resp, respBodyBytes)

				shouldFailover, isQuotaRelated := common.ShouldRetryWithNextKey(resp.StatusCode, respBodyBytes, cfgManager.GetFuzzyModeEnabled())
				if shouldFailover {
					lastError = fmt.Errorf("上游错误: %d", resp.StatusCode)
					failedKeys[apiKey] = true
					cfgManager.MarkKeyAsFailed(apiKey)
					channelScheduler.RecordFailure(currentBaseURL, apiKey, true)

					log.Printf("[Responses-Key] 警告: Responses API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)
					if envCfg.EnableResponseLogs && envCfg.IsDevelopment() {
						var formattedBody string
						if envCfg.RawLogOutput {
							formattedBody = utils.FormatJSONBytesRaw(respBodyBytes)
						} else {
							formattedBody = utils.FormatJSONBytesForLog(respBodyBytes, 500)
						}
						log.Printf("[Responses-Error] 失败原因:\n%s", formattedBody)
					} else if envCfg.EnableResponseLogs {
						log.Printf("[Responses-Error] 失败原因: %s", string(respBodyBytes))
					}

					lastFailoverError = &common.FailoverError{
						Status: resp.StatusCode,
						Body:   respBodyBytes,
					}

					if isQuotaRelated {
						deprioritizeCandidates[apiKey] = true
					}
					continue
				}

				// 非 failover 错误，记录失败指标后返回
				if envCfg.EnableResponseLogs {
					log.Printf("[Responses-Response] 警告: Responses 上游返回错误: %d", resp.StatusCode)
					if envCfg.IsDevelopment() {
						var formattedBody string
						if envCfg.RawLogOutput {
							formattedBody = utils.FormatJSONBytesRaw(respBodyBytes)
						} else {
							formattedBody = utils.FormatJSONBytesForLog(respBodyBytes, 500)
						}
						log.Printf("[Responses-Response] 错误响应体:\n%s", formattedBody)

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
						log.Printf("[Responses-Response] 错误响应头:\n%s", string(respHeadersJSON))
					}
				}
				channelScheduler.RecordFailure(currentBaseURL, apiKey, true)
				c.Data(resp.StatusCode, "application/json", respBodyBytes)
				return
			}

			if len(deprioritizeCandidates) > 0 {
				for key := range deprioritizeCandidates {
					if err := cfgManager.DeprioritizeAPIKey(key); err != nil {
						log.Printf("[Responses-Key] 警告: 密钥降级失败: %v", err)
					}
				}
			}

			usage := handleSuccess(c, resp, provider, upstream.ServiceType, envCfg, sessionManager, startTime, &responsesReq, bodyBytes)
			channelScheduler.RecordSuccessWithUsage(currentBaseURL, apiKey, usage, true)
			return
		}
	}

	log.Printf("[Responses-Error] 所有 Responses API密钥都失败了")
	common.HandleAllKeysFailed(c, cfgManager.GetFuzzyModeEnabled(), lastFailoverError, lastError, "Responses")
}

// handleSuccess 处理成功的 Responses 响应
func handleSuccess(
	c *gin.Context,
	resp *http.Response,
	provider *providers.ResponsesProvider,
	upstreamType string,
	envCfg *config.EnvConfig,
	sessionManager *session.SessionManager,
	startTime time.Time,
	originalReq *types.ResponsesRequest,
	originalRequestJSON []byte,
) *types.Usage {
	defer resp.Body.Close()

	isStream := originalReq != nil && originalReq.Stream

	if isStream {
		return handleStreamSuccess(c, resp, upstreamType, envCfg, startTime, originalReq, originalRequestJSON)
	}

	// 非流式响应处理
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to read response"})
		return nil
	}

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("[Responses-Timing] Responses 响应完成: %dms, 状态: %d", responseTime, resp.StatusCode)
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
			log.Printf("[Responses-Response] 响应头:\n%s", string(respHeadersJSON))

			var formattedBody string
			if envCfg.RawLogOutput {
				formattedBody = utils.FormatJSONBytesRaw(bodyBytes)
			} else {
				formattedBody = utils.FormatJSONBytesForLog(bodyBytes, 500)
			}
			log.Printf("[Responses-Response] 响应体:\n%s", formattedBody)
		}
	}

	providerResp := &types.ProviderResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       bodyBytes,
		Stream:     false,
	}

	responsesResp, err := provider.ConvertToResponsesResponse(providerResp, upstreamType, "")
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to convert response"})
		return nil
	}

	// Token 补全逻辑
	patchResponsesUsage(responsesResp, originalRequestJSON, envCfg)

	// 更新会话
	if originalReq.Store == nil || *originalReq.Store {
		sess, err := sessionManager.GetOrCreateSession(originalReq.PreviousResponseID)
		if err == nil {
			inputItems, _ := parseInputToItems(originalReq.Input)
			for _, item := range inputItems {
				sessionManager.AppendMessage(sess.ID, item, 0)
			}

			for _, item := range responsesResp.Output {
				sessionManager.AppendMessage(sess.ID, item, responsesResp.Usage.TotalTokens)
			}

			sessionManager.UpdateLastResponseID(sess.ID, responsesResp.ID)
			sessionManager.RecordResponseMapping(responsesResp.ID, sess.ID)

			if sess.LastResponseID != "" {
				responsesResp.PreviousID = sess.LastResponseID
			}
		}
	}

	utils.ForwardResponseHeaders(resp.Header, c.Writer)
	c.JSON(200, responsesResp)

	// 返回 usage 数据用于指标记录
	return &types.Usage{
		InputTokens:              responsesResp.Usage.InputTokens,
		OutputTokens:             responsesResp.Usage.OutputTokens,
		CacheCreationInputTokens: responsesResp.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     responsesResp.Usage.CacheReadInputTokens,
	}
}

// patchResponsesUsage 补全 Responses 响应的 Token 统计
func patchResponsesUsage(resp *types.ResponsesResponse, requestBody []byte, envCfg *config.EnvConfig) {
	// 检查是否有 Claude 原生缓存 token（有时才跳过 input_tokens 修补）
	// 仅检测 Claude 原生字段：cache_creation_input_tokens, cache_read_input_tokens,
	// cache_creation_5m_input_tokens, cache_creation_1h_input_tokens
	// 注意：不检测 input_tokens_details.cached_tokens（OpenAI 格式），避免错误跳过
	hasClaudeCache := resp.Usage.CacheCreationInputTokens > 0 ||
		resp.Usage.CacheReadInputTokens > 0 ||
		resp.Usage.CacheCreation5mInputTokens > 0 ||
		resp.Usage.CacheCreation1hInputTokens > 0

	// 检查是否需要补全
	needInputPatch := resp.Usage.InputTokens <= 1 && !hasClaudeCache
	needOutputPatch := resp.Usage.OutputTokens <= 1

	// 如果 usage 完全为空，进行完整估算
	if resp.Usage.InputTokens == 0 && resp.Usage.OutputTokens == 0 && resp.Usage.TotalTokens == 0 {
		estimatedInput := utils.EstimateResponsesRequestTokens(requestBody)
		estimatedOutput := estimateResponsesOutputFromItems(resp.Output)
		resp.Usage.InputTokens = estimatedInput
		resp.Usage.OutputTokens = estimatedOutput
		resp.Usage.TotalTokens = estimatedInput + estimatedOutput
		if envCfg.EnableResponseLogs {
			log.Printf("[Responses-Token] 上游无Usage, 本地估算: input=%d, output=%d", estimatedInput, estimatedOutput)
		}
		return
	}

	// 修补虚假值
	originalInput := resp.Usage.InputTokens
	originalOutput := resp.Usage.OutputTokens
	patched := false

	if needInputPatch {
		resp.Usage.InputTokens = utils.EstimateResponsesRequestTokens(requestBody)
		patched = true
	}
	if needOutputPatch {
		resp.Usage.OutputTokens = estimateResponsesOutputFromItems(resp.Output)
		patched = true
	}

	// 重新计算 TotalTokens（修补时或 total_tokens 为 0 但 input/output 有效时）
	if patched || (resp.Usage.TotalTokens == 0 && (resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0)) {
		resp.Usage.TotalTokens = resp.Usage.InputTokens + resp.Usage.OutputTokens
	}

	if envCfg.EnableResponseLogs {
		if patched {
			log.Printf("[Responses-Token] 虚假值修补: InputTokens=%d->%d, OutputTokens=%d->%d",
				originalInput, resp.Usage.InputTokens, originalOutput, resp.Usage.OutputTokens)
		}
		log.Printf("[Responses-Token] InputTokens=%d, OutputTokens=%d, TotalTokens=%d, CacheCreation=%d, CacheRead=%d, CacheCreation5m=%d, CacheCreation1h=%d, CacheTTL=%s",
			resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens,
			resp.Usage.CacheCreationInputTokens, resp.Usage.CacheReadInputTokens,
			resp.Usage.CacheCreation5mInputTokens, resp.Usage.CacheCreation1hInputTokens,
			resp.Usage.CacheTTL)
	}
}

// estimateResponsesOutputFromItems 从 ResponsesItem 数组估算输出 token
func estimateResponsesOutputFromItems(output []types.ResponsesItem) int {
	if len(output) == 0 {
		return 0
	}

	total := 0
	for _, item := range output {
		// 处理 content
		if item.Content != nil {
			switch v := item.Content.(type) {
			case string:
				total += utils.EstimateTokens(v)
			case []interface{}:
				for _, block := range v {
					if b, ok := block.(map[string]interface{}); ok {
						if text, ok := b["text"].(string); ok {
							total += utils.EstimateTokens(text)
						}
					}
				}
			case []types.ContentBlock:
				// 处理结构化 ContentBlock 数组
				for _, block := range v {
					if block.Text != "" {
						total += utils.EstimateTokens(block.Text)
					}
				}
			default:
				// 回退：序列化后估算
				data, _ := json.Marshal(v)
				total += utils.EstimateTokens(string(data))
			}
		}

		// 处理 tool_use
		if item.ToolUse != nil {
			if item.ToolUse.Name != "" {
				total += utils.EstimateTokens(item.ToolUse.Name) + 2
			}
			if item.ToolUse.Input != nil {
				data, _ := json.Marshal(item.ToolUse.Input)
				total += utils.EstimateTokens(string(data))
			}
		}

		// 处理 function_call 类型（item.Type == "function_call"）
		if item.Type == "function_call" {
			// 在转换后的响应中，function_call 的参数可能在 Content 中
			if contentStr, ok := item.Content.(string); ok {
				total += utils.EstimateTokens(contentStr)
			}
		}
	}

	return total
}

// handleStreamSuccess 处理流式响应
func handleStreamSuccess(
	c *gin.Context,
	resp *http.Response,
	upstreamType string,
	envCfg *config.EnvConfig,
	startTime time.Time,
	originalReq *types.ResponsesRequest,
	originalRequestJSON []byte,
) *types.Usage {
	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("[Responses-Stream] Responses 流式响应开始: %dms, 状态: %d", responseTime, resp.StatusCode)
	}

	utils.ForwardResponseHeaders(resp.Header, c.Writer)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	var synthesizer *utils.StreamSynthesizer
	var logBuffer bytes.Buffer
	streamLoggingEnabled := envCfg.IsDevelopment() && envCfg.EnableResponseLogs

	if streamLoggingEnabled {
		synthesizer = utils.NewStreamSynthesizer(upstreamType)
	}

	needConvert := upstreamType != "responses"
	var converterState any

	c.Status(resp.StatusCode)
	flusher, _ := c.Writer.(http.Flusher)

	scanner := bufio.NewScanner(resp.Body)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	// Token 统计状态
	var outputTextBuffer bytes.Buffer
	const maxOutputBufferSize = 1024 * 1024 // 1MB 上限，防止内存溢出
	var collectedUsage responsesStreamUsage
	hasUsage := false
	needTokenPatch := false
	clientGone := false

	for scanner.Scan() {
		line := scanner.Text()

		if streamLoggingEnabled {
			logBuffer.WriteString(line + "\n")
			if synthesizer != nil {
				synthesizer.ProcessLine(line)
			}
		}

		// 处理转换后的事件
		var eventsToProcess []string

		if needConvert {
			events := converters.ConvertOpenAIChatToResponses(
				c.Request.Context(),
				originalReq.Model,
				originalRequestJSON,
				nil,
				[]byte(line),
				&converterState,
			)
			eventsToProcess = events
		} else {
			eventsToProcess = []string{line + "\n"}
		}

		for _, event := range eventsToProcess {
			// 提取文本内容用于估算（限制缓冲区大小）
			if outputTextBuffer.Len() < maxOutputBufferSize {
				extractResponsesTextFromEvent(event, &outputTextBuffer)
			}

			// 检测并收集 usage
			detected, needPatch, usageData := checkResponsesEventUsage(event, envCfg.EnableResponseLogs && envCfg.ShouldLog("debug"))
			if detected {
				if !hasUsage {
					hasUsage = true
					needTokenPatch = needPatch
					if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") && needPatch {
						log.Printf("[Responses-Stream-Token] 检测到虚假值, 延迟到流结束修补")
					}
				}
				updateResponsesStreamUsage(&collectedUsage, usageData)
			}

			// 在 response.completed 事件前注入/修补 usage
			eventToSend := event
			if isResponsesCompletedEvent(event) {
				if !hasUsage {
					// 上游完全没有 usage，注入本地估算
					var injectedInput, injectedOutput int
					eventToSend, injectedInput, injectedOutput = injectResponsesUsageToCompletedEvent(event, originalRequestJSON, outputTextBuffer.String(), envCfg)
					// 更新 collectedUsage 以便最终日志输出
					collectedUsage.InputTokens = injectedInput
					collectedUsage.OutputTokens = injectedOutput
					collectedUsage.TotalTokens = injectedInput + injectedOutput
					if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
						log.Printf("[Responses-Stream-Token] 上游无usage, 注入本地估算: input=%d, output=%d", injectedInput, injectedOutput)
					}
				} else if needTokenPatch {
					// 需要修补虚假值
					eventToSend = patchResponsesCompletedEventUsage(event, originalRequestJSON, outputTextBuffer.String(), &collectedUsage, envCfg)
				}
			}

			// 转发给客户端
			if !clientGone {
				_, err := c.Writer.Write([]byte(eventToSend))
				if err != nil {
					clientGone = true
					if !isClientDisconnectError(err) {
						log.Printf("[Responses-Stream] 警告: 流式响应传输错误: %v", err)
					} else if envCfg.ShouldLog("info") {
						log.Printf("[Responses-Stream] 客户端中断连接 (正常行为)，继续接收上游数据...")
					}
				} else if flusher != nil {
					flusher.Flush()
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[Responses-Stream] 警告: 流式响应读取错误: %v", err)
	}

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("[Responses-Stream] Responses 流式响应完成: %dms", responseTime)

		// 输出 Token 统计
		if hasUsage || collectedUsage.InputTokens > 0 || collectedUsage.OutputTokens > 0 {
			log.Printf("[Responses-Stream-Token] InputTokens=%d, OutputTokens=%d, CacheCreation=%d, CacheRead=%d, CacheCreation5m=%d, CacheCreation1h=%d, CacheTTL=%s",
				collectedUsage.InputTokens, collectedUsage.OutputTokens,
				collectedUsage.CacheCreationInputTokens, collectedUsage.CacheReadInputTokens,
				collectedUsage.CacheCreation5mInputTokens, collectedUsage.CacheCreation1hInputTokens,
				collectedUsage.CacheTTL)
		}

		if envCfg.IsDevelopment() {
			if synthesizer != nil {
				synthesizedContent := synthesizer.GetSynthesizedContent()
				parseFailed := synthesizer.IsParseFailed()
				if synthesizedContent != "" && !parseFailed {
					log.Printf("[Responses-Stream] 上游流式响应合成内容:\n%s", strings.TrimSpace(synthesizedContent))
				} else if logBuffer.Len() > 0 {
					log.Printf("[Responses-Stream] 上游流式响应原始内容:\n%s", logBuffer.String())
				}
			} else if logBuffer.Len() > 0 {
				log.Printf("[Responses-Stream] 上游流式响应原始内容:\n%s", logBuffer.String())
			}
		}
	}

	// 返回收集到的 usage 数据
	return &types.Usage{
		InputTokens:              collectedUsage.InputTokens,
		OutputTokens:             collectedUsage.OutputTokens,
		CacheCreationInputTokens: collectedUsage.CacheCreationInputTokens,
		CacheReadInputTokens:     collectedUsage.CacheReadInputTokens,
	}
}

// responsesStreamUsage 流式响应 usage 收集结构
type responsesStreamUsage struct {
	InputTokens                int
	OutputTokens               int
	TotalTokens                int // 用于检测 total_tokens 是否需要补全
	CacheCreationInputTokens   int
	CacheReadInputTokens       int
	CacheCreation5mInputTokens int
	CacheCreation1hInputTokens int
	CacheTTL                   string
	HasClaudeCache             bool // 是否检测到 Claude 原生缓存字段（区别于 OpenAI cached_tokens）
}

// extractResponsesTextFromEvent 从 Responses SSE 事件中提取文本内容
func extractResponsesTextFromEvent(event string, buf *bytes.Buffer) {
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		eventType, _ := data["type"].(string)

		// 处理各种 delta 类型
		switch eventType {
		case "response.output_text.delta":
			if delta, ok := data["delta"].(string); ok {
				buf.WriteString(delta)
			}
		case "response.function_call_arguments.delta":
			if delta, ok := data["delta"].(string); ok {
				buf.WriteString(delta)
			}
		case "response.reasoning_summary_text.delta":
			if text, ok := data["text"].(string); ok {
				buf.WriteString(text)
			}
		case "response.output_json.delta":
			// JSON 输出增量
			if delta, ok := data["delta"].(string); ok {
				buf.WriteString(delta)
			}
		case "response.content_part.delta":
			// 内容块增量（通用）
			if delta, ok := data["delta"].(string); ok {
				buf.WriteString(delta)
			} else if text, ok := data["text"].(string); ok {
				buf.WriteString(text)
			}
		case "response.audio.delta", "response.audio_transcript.delta":
			// 音频转录增量
			if delta, ok := data["delta"].(string); ok {
				buf.WriteString(delta)
			}
		}
	}
}

// checkResponsesEventUsage 检测 Responses 事件是否包含 usage
func checkResponsesEventUsage(event string, enableLog bool) (bool, bool, responsesStreamUsage) {
	lines := strings.Split(event, "\n")
	for _, line := range lines {
		// 支持 "data:" 和 "data: " 两种格式（有些上游不带空格）
		var jsonStr string
		if strings.HasPrefix(line, "data:") {
			jsonStr = strings.TrimPrefix(line, "data:")
			jsonStr = strings.TrimPrefix(jsonStr, " ") // 移除可能的前导空格
		} else {
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		eventType, _ := data["type"].(string)

		// 检查 response.completed 事件中的 usage
		if eventType == "response.completed" {
			if response, ok := data["response"].(map[string]interface{}); ok {
				if usage, ok := response["usage"].(map[string]interface{}); ok {
					usageData := extractResponsesUsageFromMap(usage)
					needPatch := usageData.InputTokens <= 1 || usageData.OutputTokens <= 1

					// 仅当检测到 Claude 原生缓存字段时，才跳过 input_tokens 补全
					// OpenAI 的 input_tokens_details.cached_tokens 不应阻止补全
					if usageData.HasClaudeCache && usageData.InputTokens <= 1 {
						needPatch = usageData.OutputTokens <= 1 // 有 Claude 缓存时只检查 output
					}

					// 检查 total_tokens 是否需要补全（有效 input/output 但 total=0）
					if !needPatch && usageData.TotalTokens == 0 && (usageData.InputTokens > 0 || usageData.OutputTokens > 0) {
						needPatch = true
					}

					if enableLog {
						log.Printf("[Responses-Stream-Token] response.completed: InputTokens=%d, OutputTokens=%d, TotalTokens=%d, CacheCreation=%d, CacheRead=%d, HasClaudeCache=%v, 需补全=%v",
							usageData.InputTokens, usageData.OutputTokens, usageData.TotalTokens, usageData.CacheCreationInputTokens, usageData.CacheReadInputTokens, usageData.HasClaudeCache, needPatch)
					}
					return true, needPatch, usageData
				} else if enableLog {
					log.Printf("[Responses-Stream-Token] response.completed 事件中无 usage 字段")
				}
			} else if enableLog {
				log.Printf("[Responses-Stream-Token] response.completed 事件中无 response 字段")
			}
		}
	}
	return false, false, responsesStreamUsage{}
}

// extractResponsesUsageFromMap 从 usage map 中提取数据
func extractResponsesUsageFromMap(usage map[string]interface{}) responsesStreamUsage {
	var data responsesStreamUsage

	if v, ok := usage["input_tokens"].(float64); ok {
		data.InputTokens = int(v)
	}
	if v, ok := usage["output_tokens"].(float64); ok {
		data.OutputTokens = int(v)
	}
	if v, ok := usage["total_tokens"].(float64); ok {
		data.TotalTokens = int(v)
	}
	if v, ok := usage["cache_creation_input_tokens"].(float64); ok {
		data.CacheCreationInputTokens = int(v)
		if v > 0 {
			data.HasClaudeCache = true
		}
	}
	if v, ok := usage["cache_read_input_tokens"].(float64); ok {
		data.CacheReadInputTokens = int(v)
		if v > 0 {
			data.HasClaudeCache = true
		}
	}
	if v, ok := usage["cache_creation_5m_input_tokens"].(float64); ok {
		data.CacheCreation5mInputTokens = int(v)
		if v > 0 {
			data.HasClaudeCache = true
		}
	}
	if v, ok := usage["cache_creation_1h_input_tokens"].(float64); ok {
		data.CacheCreation1hInputTokens = int(v)
		if v > 0 {
			data.HasClaudeCache = true
		}
	}

	// 检查 input_tokens_details.cached_tokens (OpenAI 格式，不设置 HasClaudeCache)
	if details, ok := usage["input_tokens_details"].(map[string]interface{}); ok {
		if cached, ok := details["cached_tokens"].(float64); ok && cached > 0 {
			// 仅当 CacheReadInputTokens 未被设置时才使用 OpenAI 的 cached_tokens
			if data.CacheReadInputTokens == 0 {
				data.CacheReadInputTokens = int(cached)
			}
			// 注意：不设置 HasClaudeCache，因为这是 OpenAI 格式
		}
	}

	// 设置 CacheTTL
	var has5m, has1h bool
	if data.CacheCreation5mInputTokens > 0 {
		has5m = true
	}
	if data.CacheCreation1hInputTokens > 0 {
		has1h = true
	}
	if has5m && has1h {
		data.CacheTTL = "mixed"
	} else if has1h {
		data.CacheTTL = "1h"
	} else if has5m {
		data.CacheTTL = "5m"
	}

	return data
}

// updateResponsesStreamUsage 更新收集的 usage 数据
func updateResponsesStreamUsage(collected *responsesStreamUsage, usageData responsesStreamUsage) {
	if usageData.InputTokens > collected.InputTokens {
		collected.InputTokens = usageData.InputTokens
	}
	if usageData.OutputTokens > collected.OutputTokens {
		collected.OutputTokens = usageData.OutputTokens
	}
	if usageData.TotalTokens > collected.TotalTokens {
		collected.TotalTokens = usageData.TotalTokens
	}
	if usageData.CacheCreationInputTokens > 0 {
		collected.CacheCreationInputTokens = usageData.CacheCreationInputTokens
	}
	if usageData.CacheReadInputTokens > 0 {
		collected.CacheReadInputTokens = usageData.CacheReadInputTokens
	}
	if usageData.CacheCreation5mInputTokens > 0 {
		collected.CacheCreation5mInputTokens = usageData.CacheCreation5mInputTokens
	}
	if usageData.CacheCreation1hInputTokens > 0 {
		collected.CacheCreation1hInputTokens = usageData.CacheCreation1hInputTokens
	}
	if usageData.CacheTTL != "" {
		collected.CacheTTL = usageData.CacheTTL
	}
	// 传播 HasClaudeCache 标志
	if usageData.HasClaudeCache {
		collected.HasClaudeCache = true
	}
}

// isResponsesCompletedEvent 检测是否为 response.completed 事件
func isResponsesCompletedEvent(event string) bool {
	return strings.Contains(event, `"type":"response.completed"`) ||
		strings.Contains(event, `"type": "response.completed"`)
}

// isClientDisconnectError 判断是否为客户端断开连接错误
func isClientDisconnectError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset")
}

// injectResponsesUsageToCompletedEvent 向 response.completed 事件注入 usage
// 返回: 修改后的事件字符串, 估算的 inputTokens, 估算的 outputTokens
func injectResponsesUsageToCompletedEvent(event string, requestBody []byte, outputText string, envCfg *config.EnvConfig) (string, int, int) {
	inputTokens := utils.EstimateResponsesRequestTokens(requestBody)
	outputTokens := utils.EstimateTokens(outputText)
	totalTokens := inputTokens + outputTokens

	// 调试日志：记录估算开始
	if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
		log.Printf("[Responses-Stream-Token] injectUsage 开始: inputTokens=%d, outputTokens=%d, event长度=%d",
			inputTokens, outputTokens, len(event))
	}

	var result strings.Builder
	lines := strings.Split(event, "\n")
	injected := false

	for _, line := range lines {
		// 跳过 event: 行，但保留它
		if strings.HasPrefix(line, "event:") {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// 支持 "data:" 和 "data: " 两种格式（有些上游不带空格）
		var jsonStr string
		if strings.HasPrefix(line, "data:") {
			jsonStr = strings.TrimPrefix(line, "data:")
			jsonStr = strings.TrimPrefix(jsonStr, " ") // 移除可能的前导空格
		} else {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			// 调试日志：JSON 解析失败
			if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
				log.Printf("[Responses-Stream-Token] JSON解析失败: %v, 内容前200字符: %.200s", err, jsonStr)
			}
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		eventType, _ := data["type"].(string)

		if eventType == "response.completed" {
			response, ok := data["response"].(map[string]interface{})
			if !ok {
				// response 字段缺失或类型错误，创建一个新的
				if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
					log.Printf("[Responses-Stream-Token] response字段缺失, 创建新的response对象")
				}
				response = make(map[string]interface{})
				data["response"] = response
			}

			response["usage"] = map[string]interface{}{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
				"total_tokens":  totalTokens,
			}
			injected = true

			patchedJSON, err := json.Marshal(data)
			if err != nil {
				if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
					log.Printf("[Responses-Stream-Token] JSON序列化失败: %v", err)
				}
				result.WriteString(line)
				result.WriteString("\n")
				continue
			}

			if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
				log.Printf("[Responses-Stream-Token] 注入本地估算成功: InputTokens=%d, OutputTokens=%d, TotalTokens=%d",
					inputTokens, outputTokens, totalTokens)
			}

			result.WriteString("data: ")
			result.Write(patchedJSON)
			result.WriteString("\n")
		} else {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	// 如果没有成功注入，可能是 SSE 格式不同，尝试直接在整个 event 中查找并替换
	if !injected {
		if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
			log.Printf("[Responses-Stream-Token] 逐行解析未找到, 尝试整体解析 event")
		}

		// 尝试从 event 中提取 JSON 部分（可能是多行格式）
		var jsonStart, jsonEnd int
		for i, line := range lines {
			if strings.HasPrefix(line, "data:") {
				jsonStart = i
				break
			}
		}

		// 合并所有 data: 行（支持 "data:" 和 "data: " 两种格式）
		var jsonBuilder strings.Builder
		for i := jsonStart; i < len(lines); i++ {
			line := lines[i]
			if strings.HasPrefix(line, "data:") {
				jsonData := strings.TrimPrefix(line, "data:")
				jsonData = strings.TrimPrefix(jsonData, " ") // 移除可能的前导空格
				jsonBuilder.WriteString(jsonData)
			} else if line == "" {
				jsonEnd = i
				break
			}
		}

		fullJSON := jsonBuilder.String()
		if fullJSON != "" {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(fullJSON), &data); err == nil {
				eventType, _ := data["type"].(string)
				if eventType == "response.completed" {
					response, ok := data["response"].(map[string]interface{})
					if !ok {
						response = make(map[string]interface{})
						data["response"] = response
					}

					response["usage"] = map[string]interface{}{
						"input_tokens":  inputTokens,
						"output_tokens": outputTokens,
						"total_tokens":  totalTokens,
					}

					patchedJSON, err := json.Marshal(data)
					if err == nil {
						injected = true
						// 重建 event
						result.Reset()
						for i := 0; i < jsonStart; i++ {
							result.WriteString(lines[i])
							result.WriteString("\n")
						}
						result.WriteString("data: ")
						result.Write(patchedJSON)
						result.WriteString("\n")
						for i := jsonEnd; i < len(lines); i++ {
							result.WriteString(lines[i])
							result.WriteString("\n")
						}

						if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
							log.Printf("[Responses-Stream-Token] 整体解析注入成功: InputTokens=%d, OutputTokens=%d",
								inputTokens, outputTokens)
						}
					}
				}
			}
		}
	}

	// 如果仍然没有成功注入，记录警告并打印 event 内容
	if !injected {
		if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
			// 打印 event 的前500个字符帮助调试
			eventPreview := event
			if len(eventPreview) > 500 {
				eventPreview = eventPreview[:500] + "..."
			}
			log.Printf("[Responses-Stream-Token] 警告: 未找到 response.completed 事件进行注入, event内容: %s", eventPreview)
		}
		return event, inputTokens, outputTokens
	}

	return result.String(), inputTokens, outputTokens
}

// patchResponsesCompletedEventUsage 修补 response.completed 事件中的 usage
func patchResponsesCompletedEventUsage(event string, requestBody []byte, outputText string, collected *responsesStreamUsage, envCfg *config.EnvConfig) string {
	var result strings.Builder
	lines := strings.Split(event, "\n")

	for _, line := range lines {
		// 支持 "data:" 和 "data: " 两种格式（有些上游不带空格）
		var jsonStr string
		if strings.HasPrefix(line, "data:") {
			jsonStr = strings.TrimPrefix(line, "data:")
			jsonStr = strings.TrimPrefix(jsonStr, " ") // 移除可能的前导空格
		} else {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		if data["type"] == "response.completed" {
			if response, ok := data["response"].(map[string]interface{}); ok {
				if usage, ok := response["usage"].(map[string]interface{}); ok {
					originalInput := collected.InputTokens
					originalOutput := collected.OutputTokens
					patched := false

					// 修补 input_tokens（仅当没有 Claude 原生缓存时）
					// OpenAI 的 cached_tokens 不应阻止 input_tokens 补全
					if collected.InputTokens <= 1 && !collected.HasClaudeCache {
						estimatedInput := utils.EstimateResponsesRequestTokens(requestBody)
						usage["input_tokens"] = estimatedInput
						collected.InputTokens = estimatedInput
						patched = true
					}

					// 修补 output_tokens
					if collected.OutputTokens <= 1 {
						estimatedOutput := utils.EstimateTokens(outputText)
						usage["output_tokens"] = estimatedOutput
						collected.OutputTokens = estimatedOutput
						patched = true
					}

					// 重新计算 total_tokens（修补时或 total_tokens 为 0 但 input/output 有效时）
					currentTotal := 0
					if t, ok := usage["total_tokens"].(float64); ok {
						currentTotal = int(t)
					}
					if patched || (currentTotal == 0 && (collected.InputTokens > 0 || collected.OutputTokens > 0)) {
						usage["total_tokens"] = collected.InputTokens + collected.OutputTokens
					}

					if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") && patched {
						log.Printf("[Responses-Stream-Token] 虚假值修补: InputTokens=%d->%d, OutputTokens=%d->%d",
							originalInput, collected.InputTokens, originalOutput, collected.OutputTokens)
					}
				}
			}

			patchedJSON, err := json.Marshal(data)
			if err != nil {
				result.WriteString(line)
				result.WriteString("\n")
				continue
			}

			result.WriteString("data: ")
			result.Write(patchedJSON)
			result.WriteString("\n")
		} else {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return result.String()
}

// parseInputToItems 解析 input 为 ResponsesItem 数组
func parseInputToItems(input interface{}) ([]types.ResponsesItem, error) {
	switch v := input.(type) {
	case string:
		return []types.ResponsesItem{{Type: "text", Content: v}}, nil
	case []interface{}:
		items := []types.ResponsesItem{}
		for _, item := range v {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			itemType, _ := itemMap["type"].(string)
			content := itemMap["content"]
			items = append(items, types.ResponsesItem{Type: itemType, Content: content})
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported input type")
	}
}
