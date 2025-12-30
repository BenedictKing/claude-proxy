// Package gemini 提供 Gemini API 的处理器
package gemini

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
	"github.com/BenedictKing/claude-proxy/internal/converters"
	"github.com/BenedictKing/claude-proxy/internal/handlers/common"
	"github.com/BenedictKing/claude-proxy/internal/middleware"
	"github.com/BenedictKing/claude-proxy/internal/scheduler"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
	"github.com/gin-gonic/gin"
)

// Handler Gemini API 代理处理器
// 支持多渠道调度：当配置多个渠道时自动启用
func Handler(
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// 支持两种认证方式：x-goog-api-key（Gemini 原生）和 x-api-key（通用）
		apiKey := extractGeminiAPIKey(c)
		if apiKey == "" {
			// 使用标准认证中间件
			middleware.ProxyAuthMiddleware(envCfg)(c)
			if c.IsAborted() {
				return
			}
		}

		startTime := time.Now()

		// 读取原始请求体
		maxBodySize := envCfg.MaxRequestBodySize
		bodyBytes, err := common.ReadRequestBody(c, maxBodySize)
		if err != nil {
			return
		}

		// 解析 Gemini 请求
		var geminiReq types.GeminiRequest
		if len(bodyBytes) > 0 {
			if err := json.Unmarshal(bodyBytes, &geminiReq); err != nil {
				c.JSON(400, types.GeminiError{
					Error: types.GeminiErrorDetail{
						Code:    400,
						Message: fmt.Sprintf("Invalid request body: %v", err),
						Status:  "INVALID_ARGUMENT",
					},
				})
				return
			}
		}

		// 从 URL 路径提取模型名称
		// 格式: /v1/models/{model}:generateContent 或 /v1/models/{model}:streamGenerateContent
		// 使用 *modelAction 通配符捕获整个后缀，如 /gemini-pro:generateContent
		modelAction := c.Param("modelAction")
		// 移除前导斜杠（Gin 的 * 通配符会保留前导斜杠）
		modelAction = strings.TrimPrefix(modelAction, "/")
		model := extractModelName(modelAction)
		if model == "" {
			c.JSON(400, types.GeminiError{
				Error: types.GeminiErrorDetail{
					Code:    400,
					Message: "Model name is required in URL path",
					Status:  "INVALID_ARGUMENT",
				},
			})
			return
		}

		// 判断是否流式
		isStream := strings.Contains(c.Request.URL.Path, "streamGenerateContent")

		// 提取对话标识用于 Trace 亲和性
		userID := common.ExtractConversationID(c, bodyBytes)

		// 记录原始请求信息
		common.LogOriginalRequest(c, bodyBytes, envCfg, "Gemini")

		// 检查是否为多渠道模式
		isMultiChannel := channelScheduler.IsMultiChannelModeGemini()

		if isMultiChannel {
			handleMultiChannel(c, envCfg, cfgManager, channelScheduler, bodyBytes, &geminiReq, model, isStream, userID, startTime)
		} else {
			handleSingleChannel(c, envCfg, cfgManager, channelScheduler, bodyBytes, &geminiReq, model, isStream, startTime)
		}
	})
}

// extractGeminiAPIKey 从请求中提取 Gemini 风格的 API Key
func extractGeminiAPIKey(c *gin.Context) string {
	// 1. x-goog-api-key header（Gemini 原生）
	if key := c.GetHeader("x-goog-api-key"); key != "" {
		return key
	}
	// 2. ?key= query parameter
	if key := c.Query("key"); key != "" {
		return key
	}
	return ""
}

// extractModelName 从 URL 参数提取模型名称
// 输入: "gemini-2.0-flash:generateContent" 或 "gemini-2.0-flash"
// 输出: "gemini-2.0-flash"
func extractModelName(param string) string {
	if param == "" {
		return ""
	}
	// 移除 :generateContent 或 :streamGenerateContent 后缀
	if idx := strings.Index(param, ":"); idx > 0 {
		return param[:idx]
	}
	return param
}

// handleMultiChannel 处理多渠道 Gemini 请求
func handleMultiChannel(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	bodyBytes []byte,
	geminiReq *types.GeminiRequest,
	model string,
	isStream bool,
	userID string,
	startTime time.Time,
) {
	failedChannels := make(map[int]bool)
	var lastError error
	var lastFailoverError *common.FailoverError

	maxChannelAttempts := channelScheduler.GetActiveGeminiChannelCount()

	for channelAttempt := 0; channelAttempt < maxChannelAttempts; channelAttempt++ {
		selection, err := channelScheduler.SelectGeminiChannel(c.Request.Context(), userID, failedChannels)
		if err != nil {
			lastError = err
			break
		}

		upstream := selection.Upstream
		channelIndex := selection.ChannelIndex

		if envCfg.ShouldLog("info") {
			log.Printf("[Gemini-Select] 选择渠道: [%d] %s (原因: %s, 尝试 %d/%d)",
				channelIndex, upstream.Name, selection.Reason, channelAttempt+1, maxChannelAttempts)
		}

		success, successKey, successBaseURLIdx, failoverErr, usage := tryChannelWithAllKeys(
			c, envCfg, cfgManager, channelScheduler, upstream, channelIndex,
			bodyBytes, geminiReq, model, isStream, startTime,
		)

		if success {
			if successKey != "" {
				channelScheduler.RecordGeminiSuccessWithUsage(upstream.GetAllBaseURLs()[successBaseURLIdx], successKey, usage)
			}
			channelScheduler.SetTraceAffinity(userID, channelIndex)
			return
		}

		failedChannels[channelIndex] = true

		if failoverErr != nil {
			lastFailoverError = failoverErr
			lastError = fmt.Errorf("渠道 [%d] %s 失败", channelIndex, upstream.Name)
		}

		log.Printf("[Gemini-Failover] 警告: 渠道 [%d] %s 所有密钥都失败，尝试下一个渠道", channelIndex, upstream.Name)
	}

	log.Printf("[Gemini-Error] 所有渠道都失败了")
	handleAllChannelsFailed(c, lastFailoverError, lastError)
}

// tryChannelWithAllKeys 尝试使用 Gemini 渠道的所有密钥
func tryChannelWithAllKeys(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	upstream *config.UpstreamConfig,
	channelIndex int,
	bodyBytes []byte,
	geminiReq *types.GeminiRequest,
	model string,
	isStream bool,
	startTime time.Time,
) (bool, string, int, *common.FailoverError, *types.Usage) {
	if len(upstream.APIKeys) == 0 {
		return false, "", 0, nil, nil
	}

	metricsManager := channelScheduler.GetGeminiMetricsManager()
	baseURLs := upstream.GetAllBaseURLs()

	// 获取动态排序后的 URL 列表
	sortedURLResults := channelScheduler.GetSortedURLsForChannel(channelIndex, baseURLs)

	var lastFailoverError *common.FailoverError
	deprioritizeCandidates := make(map[string]bool)

	// 强制探测模式
	forceProbeMode := common.AreAllKeysSuspended(metricsManager, upstream.BaseURL, upstream.APIKeys)
	if forceProbeMode {
		log.Printf("[Gemini-ForceProbe] 渠道 %s 所有 Key 都被熔断，启用强制探测模式", upstream.Name)
	}

	for sortedIdx, urlResult := range sortedURLResults {
		currentBaseURL := urlResult.URL
		originalIdx := urlResult.OriginalIdx
		failedKeys := make(map[string]bool)
		maxRetries := len(upstream.APIKeys)

		for attempt := 0; attempt < maxRetries; attempt++ {
			common.RestoreRequestBody(c, bodyBytes)

			apiKey, err := cfgManager.GetNextGeminiAPIKey(upstream, failedKeys)
			if err != nil {
				break
			}

			// 检查熔断状态
			if !forceProbeMode && metricsManager.ShouldSuspendKey(currentBaseURL, apiKey) {
				failedKeys[apiKey] = true
				log.Printf("[Gemini-Circuit] 跳过熔断中的 Key: %s", utils.MaskAPIKey(apiKey))
				continue
			}

			if envCfg.ShouldLog("info") {
				log.Printf("[Gemini-Key] 使用API密钥: %s (BaseURL %d/%d, 尝试 %d/%d)",
					utils.MaskAPIKey(apiKey), sortedIdx+1, len(sortedURLResults), attempt+1, maxRetries)
			}

			// 构建请求
			providerReq, err := buildProviderRequest(c, upstream, currentBaseURL, apiKey, geminiReq, model, isStream)
			if err != nil {
				failedKeys[apiKey] = true
				channelScheduler.RecordGeminiFailure(currentBaseURL, apiKey)
				continue
			}

			resp, err := common.SendRequest(providerReq, upstream, envCfg, isStream)
			if err != nil {
				failedKeys[apiKey] = true
				cfgManager.MarkKeyAsFailed(apiKey)
				channelScheduler.RecordGeminiFailure(currentBaseURL, apiKey)
				channelScheduler.MarkURLFailure(channelIndex, currentBaseURL)
				log.Printf("[Gemini-Key] 警告: API密钥失败: %v", err)
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
					channelScheduler.RecordGeminiFailure(currentBaseURL, apiKey)
					channelScheduler.MarkURLFailure(channelIndex, currentBaseURL)
					log.Printf("[Gemini-Key] 警告: API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)

					lastFailoverError = &common.FailoverError{
						Status: resp.StatusCode,
						Body:   respBodyBytes,
					}

					if isQuotaRelated {
						deprioritizeCandidates[apiKey] = true
					}
					continue
				}

				// 非 failover 错误
				channelScheduler.RecordGeminiFailure(currentBaseURL, apiKey)
				c.Data(resp.StatusCode, "application/json", respBodyBytes)
				return true, "", 0, nil, nil
			}

			if len(deprioritizeCandidates) > 0 {
				for key := range deprioritizeCandidates {
					_ = cfgManager.DeprioritizeAPIKey(key)
				}
			}

			channelScheduler.MarkURLSuccess(channelIndex, currentBaseURL)

			usage := handleSuccess(c, resp, upstream.ServiceType, envCfg, startTime, geminiReq, model, isStream)
			return true, apiKey, originalIdx, nil, usage
		}

		if sortedIdx < len(sortedURLResults)-1 {
			log.Printf("[Gemini-BaseURL] BaseURL %d/%d 所有 Key 失败，切换到下一个 BaseURL", sortedIdx+1, len(sortedURLResults))
		}
	}

	return false, "", 0, lastFailoverError, nil
}

// handleSingleChannel 处理单渠道 Gemini 请求
func handleSingleChannel(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	bodyBytes []byte,
	geminiReq *types.GeminiRequest,
	model string,
	isStream bool,
	startTime time.Time,
) {
	upstream, err := cfgManager.GetCurrentGeminiUpstream()
	if err != nil {
		c.JSON(503, types.GeminiError{
			Error: types.GeminiErrorDetail{
				Code:    503,
				Message: "No Gemini upstream configured",
				Status:  "UNAVAILABLE",
			},
		})
		return
	}

	if len(upstream.APIKeys) == 0 {
		c.JSON(503, types.GeminiError{
			Error: types.GeminiErrorDetail{
				Code:    503,
				Message: fmt.Sprintf("No API keys configured for upstream \"%s\"", upstream.Name),
				Status:  "UNAVAILABLE",
			},
		})
		return
	}

	metricsManager := channelScheduler.GetGeminiMetricsManager()
	baseURLs := upstream.GetAllBaseURLs()

	var lastError error
	var lastFailoverError *common.FailoverError
	deprioritizeCandidates := make(map[string]bool)

	forceProbeMode := common.AreAllKeysSuspended(metricsManager, baseURLs[0], upstream.APIKeys)
	if forceProbeMode {
		log.Printf("[Gemini-ForceProbe] 渠道 %s 所有 Key 都被熔断，启用强制探测模式", upstream.Name)
	}

	for baseURLIdx, currentBaseURL := range baseURLs {
		failedKeys := make(map[string]bool)
		maxRetries := len(upstream.APIKeys)

		for attempt := 0; attempt < maxRetries; attempt++ {
			common.RestoreRequestBody(c, bodyBytes)

			apiKey, err := cfgManager.GetNextGeminiAPIKey(upstream, failedKeys)
			if err != nil {
				lastError = err
				break
			}

			if !forceProbeMode && metricsManager.ShouldSuspendKey(currentBaseURL, apiKey) {
				failedKeys[apiKey] = true
				log.Printf("[Gemini-Circuit] 跳过熔断中的 Key: %s", utils.MaskAPIKey(apiKey))
				continue
			}

			if envCfg.ShouldLog("info") {
				log.Printf("[Gemini-Upstream] 使用 Gemini 上游: %s - %s (BaseURL %d/%d, 尝试 %d/%d)",
					upstream.Name, currentBaseURL, baseURLIdx+1, len(baseURLs), attempt+1, maxRetries)
				log.Printf("[Gemini-Key] 使用API密钥: %s", utils.MaskAPIKey(apiKey))
			}

			providerReq, err := buildProviderRequest(c, upstream, currentBaseURL, apiKey, geminiReq, model, isStream)
			if err != nil {
				lastError = err
				failedKeys[apiKey] = true
				channelScheduler.RecordGeminiFailure(currentBaseURL, apiKey)
				continue
			}

			resp, err := common.SendRequest(providerReq, upstream, envCfg, isStream)
			if err != nil {
				lastError = err
				failedKeys[apiKey] = true
				cfgManager.MarkKeyAsFailed(apiKey)
				channelScheduler.RecordGeminiFailure(currentBaseURL, apiKey)
				log.Printf("[Gemini-Key] 警告: API密钥失败: %v", err)
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
					channelScheduler.RecordGeminiFailure(currentBaseURL, apiKey)
					log.Printf("[Gemini-Key] 警告: API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)

					lastFailoverError = &common.FailoverError{
						Status: resp.StatusCode,
						Body:   respBodyBytes,
					}

					if isQuotaRelated {
						deprioritizeCandidates[apiKey] = true
					}
					continue
				}

				channelScheduler.RecordGeminiFailure(currentBaseURL, apiKey)
				c.Data(resp.StatusCode, "application/json", respBodyBytes)
				return
			}

			if len(deprioritizeCandidates) > 0 {
				for key := range deprioritizeCandidates {
					_ = cfgManager.DeprioritizeAPIKey(key)
				}
			}

			usage := handleSuccess(c, resp, upstream.ServiceType, envCfg, startTime, geminiReq, model, isStream)
			channelScheduler.RecordGeminiSuccessWithUsage(currentBaseURL, apiKey, usage)
			return
		}
	}

	log.Printf("[Gemini-Error] 所有 API密钥都失败了")
	handleAllKeysFailed(c, lastFailoverError, lastError)
}

// buildProviderRequest 构建上游请求
func buildProviderRequest(
	c *gin.Context,
	upstream *config.UpstreamConfig,
	baseURL string,
	apiKey string,
	geminiReq *types.GeminiRequest,
	model string,
	isStream bool,
) (*http.Request, error) {
	// 应用模型映射
	mappedModel := config.RedirectModel(model, upstream)

	var requestBody []byte
	var url string
	var err error

	switch upstream.ServiceType {
	case "gemini":
		// Gemini 上游：直接转发
		requestBody, err = json.Marshal(geminiReq)
		if err != nil {
			return nil, err
		}

		action := "generateContent"
		if isStream {
			action = "streamGenerateContent"
		}
		url = fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(baseURL, "/"), mappedModel, action)
		if isStream {
			url += "?alt=sse"
		}

	case "claude":
		// Claude 上游：需要转换
		claudeReq, err := converters.GeminiToClaudeRequest(geminiReq, mappedModel)
		if err != nil {
			return nil, err
		}
		claudeReq["stream"] = isStream
		requestBody, err = json.Marshal(claudeReq)
		if err != nil {
			return nil, err
		}
		url = fmt.Sprintf("%s/v1/messages", strings.TrimRight(baseURL, "/"))

	case "openai":
		// OpenAI 上游：需要转换
		openaiReq, err := converters.GeminiToOpenAIRequest(geminiReq, mappedModel)
		if err != nil {
			return nil, err
		}
		openaiReq["stream"] = isStream
		requestBody, err = json.Marshal(openaiReq)
		if err != nil {
			return nil, err
		}
		url = fmt.Sprintf("%s/v1/chat/completions", strings.TrimRight(baseURL, "/"))

	default:
		// 默认当作 Gemini 处理
		requestBody, err = json.Marshal(geminiReq)
		if err != nil {
			return nil, err
		}
		action := "generateContent"
		if isStream {
			action = "streamGenerateContent"
		}
		url = fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(baseURL, "/"), mappedModel, action)
		if isStream {
			url += "?alt=sse"
		}
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), "POST", url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	switch upstream.ServiceType {
	case "gemini":
		req.Header.Set("x-goog-api-key", apiKey)
	case "claude":
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "openai":
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	default:
		req.Header.Set("x-goog-api-key", apiKey)
	}

	return req, nil
}

// handleSuccess 处理成功的响应
func handleSuccess(
	c *gin.Context,
	resp *http.Response,
	upstreamType string,
	envCfg *config.EnvConfig,
	startTime time.Time,
	geminiReq *types.GeminiRequest,
	model string,
	isStream bool,
) *types.Usage {
	defer resp.Body.Close()

	if isStream {
		return handleStreamSuccess(c, resp, upstreamType, envCfg, startTime, model)
	}

	// 非流式响应处理
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(500, types.GeminiError{
			Error: types.GeminiErrorDetail{
				Code:    500,
				Message: "Failed to read response",
				Status:  "INTERNAL",
			},
		})
		return nil
	}

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("[Gemini-Timing] 响应完成: %dms, 状态: %d", responseTime, resp.StatusCode)
	}

	// 根据上游类型转换响应
	var geminiResp *types.GeminiResponse

	switch upstreamType {
	case "gemini":
		// 直接解析 Gemini 响应
		if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
			c.Data(resp.StatusCode, "application/json", bodyBytes)
			return nil
		}

	case "claude":
		// 转换 Claude 响应为 Gemini 格式
		var claudeResp map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &claudeResp); err != nil {
			c.Data(resp.StatusCode, "application/json", bodyBytes)
			return nil
		}
		geminiResp, err = converters.ClaudeResponseToGemini(claudeResp)
		if err != nil {
			c.Data(resp.StatusCode, "application/json", bodyBytes)
			return nil
		}

	case "openai":
		// 转换 OpenAI 响应为 Gemini 格式
		var openaiResp map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &openaiResp); err != nil {
			c.Data(resp.StatusCode, "application/json", bodyBytes)
			return nil
		}
		geminiResp, err = converters.OpenAIResponseToGemini(openaiResp)
		if err != nil {
			c.Data(resp.StatusCode, "application/json", bodyBytes)
			return nil
		}

	default:
		// 默认直接返回
		c.Data(resp.StatusCode, "application/json", bodyBytes)
		return nil
	}

	// 返回 Gemini 格式响应
	respBytes, err := json.Marshal(geminiResp)
	if err != nil {
		c.Data(resp.StatusCode, "application/json", bodyBytes)
		return nil
	}

	c.Data(resp.StatusCode, "application/json", respBytes)

	// 提取 usage 统计
	var usage *types.Usage
	if geminiResp.UsageMetadata != nil {
		usage = &types.Usage{
			InputTokens:  geminiResp.UsageMetadata.PromptTokenCount - geminiResp.UsageMetadata.CachedContentTokenCount,
			OutputTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
		}
	}

	return usage
}

// handleAllChannelsFailed 处理所有渠道失败的情况
func handleAllChannelsFailed(c *gin.Context, failoverErr *common.FailoverError, lastError error) {
	if failoverErr != nil {
		c.Data(failoverErr.Status, "application/json", failoverErr.Body)
		return
	}

	errMsg := "All channels failed"
	if lastError != nil {
		errMsg = lastError.Error()
	}

	c.JSON(503, types.GeminiError{
		Error: types.GeminiErrorDetail{
			Code:    503,
			Message: errMsg,
			Status:  "UNAVAILABLE",
		},
	})
}

// handleAllKeysFailed 处理所有 Key 失败的情况
func handleAllKeysFailed(c *gin.Context, failoverErr *common.FailoverError, lastError error) {
	if failoverErr != nil {
		c.Data(failoverErr.Status, "application/json", failoverErr.Body)
		return
	}

	errMsg := "All API keys failed"
	if lastError != nil {
		errMsg = lastError.Error()
	}

	c.JSON(503, types.GeminiError{
		Error: types.GeminiErrorDetail{
			Code:    503,
			Message: errMsg,
			Status:  "UNAVAILABLE",
		},
	})
}
