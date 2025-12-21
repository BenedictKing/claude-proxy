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

		// 检查是否为多渠道模式
		isMultiChannel := channelScheduler.IsMultiChannelMode(true) // true = isResponses

		if isMultiChannel {
			handleMultiChannel(c, envCfg, cfgManager, channelScheduler, sessionManager, bodyBytes, responsesReq, userID, startTime)
		} else {
			handleSingleChannel(c, envCfg, cfgManager, sessionManager, bodyBytes, responsesReq, startTime)
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
			log.Printf("🎯 [多渠道/Responses] 选择渠道: [%d] %s (原因: %s, 尝试 %d/%d)",
				channelIndex, upstream.Name, selection.Reason, channelAttempt+1, maxChannelAttempts)
		}

		success, successKey, failoverErr := tryChannelWithAllKeys(c, envCfg, cfgManager, channelScheduler, sessionManager, upstream, bodyBytes, responsesReq, startTime)

		if success {
			if successKey != "" {
				channelScheduler.RecordSuccess(upstream.BaseURL, successKey, true)
			}
			channelScheduler.SetTraceAffinity(userID, channelIndex)
			return
		}

		failedChannels[channelIndex] = true

		if failoverErr != nil {
			lastFailoverError = failoverErr
			lastError = fmt.Errorf("渠道 [%d] %s 失败", channelIndex, upstream.Name)
		}

		log.Printf("⚠️ [多渠道/Responses] 渠道 [%d] %s 所有密钥都失败，尝试下一个渠道", channelIndex, upstream.Name)
	}

	log.Printf("💥 [多渠道/Responses] 所有渠道都失败了")
	common.HandleAllChannelsFailed(c, cfgManager.GetFuzzyModeEnabled(), lastFailoverError, lastError, "Responses")
}

// tryChannelWithAllKeys 尝试使用 Responses 渠道的所有密钥
func tryChannelWithAllKeys(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	channelScheduler *scheduler.ChannelScheduler,
	sessionManager *session.SessionManager,
	upstream *config.UpstreamConfig,
	bodyBytes []byte,
	responsesReq types.ResponsesRequest,
	startTime time.Time,
) (bool, string, *common.FailoverError) {
	if len(upstream.APIKeys) == 0 {
		return false, "", nil
	}

	provider := &providers.ResponsesProvider{SessionManager: sessionManager}
	metricsManager := channelScheduler.GetResponsesMetricsManager()

	maxRetries := len(upstream.APIKeys)
	failedKeys := make(map[string]bool)
	var lastFailoverError *common.FailoverError
	deprioritizeCandidates := make(map[string]bool)

	// 强制探测模式
	forceProbeMode := common.AreAllKeysSuspended(metricsManager, upstream.BaseURL, upstream.APIKeys)
	if forceProbeMode {
		log.Printf("🔍 [强制探测/Responses] 渠道 %s 所有 Key 都被熔断，启用强制探测模式", upstream.Name)
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		common.RestoreRequestBody(c, bodyBytes)

		apiKey, err := cfgManager.GetNextResponsesAPIKey(upstream, failedKeys)
		if err != nil {
			break
		}

		// 检查熔断状态
		if !forceProbeMode && metricsManager.ShouldSuspendKey(upstream.BaseURL, apiKey) {
			failedKeys[apiKey] = true
			log.Printf("⚡ [Responses] 跳过熔断中的 Key: %s", utils.MaskAPIKey(apiKey))
			continue
		}

		if envCfg.ShouldLog("info") {
			log.Printf("🔑 [Responses] 使用API密钥: %s (尝试 %d/%d)", utils.MaskAPIKey(apiKey), attempt+1, maxRetries)
		}

		providerReq, _, err := provider.ConvertToProviderRequest(c, upstream, apiKey)
		if err != nil {
			failedKeys[apiKey] = true
			channelScheduler.RecordFailure(upstream.BaseURL, apiKey, true)
			continue
		}

		resp, err := common.SendRequest(providerReq, upstream, envCfg, responsesReq.Stream)
		if err != nil {
			failedKeys[apiKey] = true
			cfgManager.MarkKeyAsFailed(apiKey)
			channelScheduler.RecordFailure(upstream.BaseURL, apiKey, true)
			log.Printf("⚠️ [Responses] API密钥失败: %v", err)
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
				channelScheduler.RecordFailure(upstream.BaseURL, apiKey, true)
				log.Printf("⚠️ [Responses] API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)

				lastFailoverError = &common.FailoverError{
					Status: resp.StatusCode,
					Body:   respBodyBytes,
				}

				if isQuotaRelated {
					deprioritizeCandidates[apiKey] = true
				}
				continue
			}

			c.Data(resp.StatusCode, "application/json", respBodyBytes)
			return true, "", nil
		}

		if len(deprioritizeCandidates) > 0 {
			for key := range deprioritizeCandidates {
				_ = cfgManager.DeprioritizeAPIKey(key)
			}
		}

		handleSuccess(c, resp, provider, upstream.ServiceType, envCfg, sessionManager, startTime, &responsesReq, bodyBytes)
		return true, apiKey, nil
	}

	return false, "", lastFailoverError
}

// handleSingleChannel 处理单渠道 Responses 请求
func handleSingleChannel(
	c *gin.Context,
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
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

	maxRetries := len(upstream.APIKeys)
	failedKeys := make(map[string]bool)
	var lastError error
	var lastOriginalBodyBytes []byte
	var lastFailoverError *common.FailoverError
	deprioritizeCandidates := make(map[string]bool)

	for attempt := 0; attempt < maxRetries; attempt++ {
		common.RestoreRequestBody(c, bodyBytes)

		apiKey, err := cfgManager.GetNextResponsesAPIKey(upstream, failedKeys)
		if err != nil {
			lastError = err
			break
		}

		if envCfg.ShouldLog("info") {
			log.Printf("🎯 使用 Responses 上游: %s - %s (尝试 %d/%d)", upstream.Name, upstream.BaseURL, attempt+1, maxRetries)
			log.Printf("🔑 使用API密钥: %s", utils.MaskAPIKey(apiKey))
		}

		providerReq, originalBodyBytes, err := provider.ConvertToProviderRequest(c, upstream, apiKey)
		if err != nil {
			lastError = err
			failedKeys[apiKey] = true
			if originalBodyBytes != nil {
				lastOriginalBodyBytes = originalBodyBytes
			}
			continue
		}
		lastOriginalBodyBytes = originalBodyBytes

		common.LogOriginalRequest(c, lastOriginalBodyBytes, envCfg, "Responses")

		resp, err := common.SendRequest(providerReq, upstream, envCfg, responsesReq.Stream)
		if err != nil {
			lastError = err
			failedKeys[apiKey] = true
			cfgManager.MarkKeyAsFailed(apiKey)
			log.Printf("⚠️ API密钥失败: %v", err)
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

				log.Printf("⚠️ Responses API密钥失败 (状态: %d)，尝试下一个密钥", resp.StatusCode)
				if envCfg.EnableResponseLogs && envCfg.IsDevelopment() {
					var formattedBody string
					if envCfg.RawLogOutput {
						formattedBody = utils.FormatJSONBytesRaw(respBodyBytes)
					} else {
						formattedBody = utils.FormatJSONBytesForLog(respBodyBytes, 500)
					}
					log.Printf("📦 失败原因:\n%s", formattedBody)
				} else if envCfg.EnableResponseLogs {
					log.Printf("失败原因: %s", string(respBodyBytes))
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

			if envCfg.EnableResponseLogs {
				log.Printf("⚠️ Responses 上游返回错误: %d", resp.StatusCode)
				if envCfg.IsDevelopment() {
					var formattedBody string
					if envCfg.RawLogOutput {
						formattedBody = utils.FormatJSONBytesRaw(respBodyBytes)
					} else {
						formattedBody = utils.FormatJSONBytesForLog(respBodyBytes, 500)
					}
					log.Printf("📦 错误响应体:\n%s", formattedBody)

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
					log.Printf("📋 错误响应头:\n%s", string(respHeadersJSON))
				}
			}
			c.Data(resp.StatusCode, "application/json", respBodyBytes)
			return
		}

		if len(deprioritizeCandidates) > 0 {
			for key := range deprioritizeCandidates {
				if err := cfgManager.DeprioritizeAPIKey(key); err != nil {
					log.Printf("⚠️ 密钥降级失败: %v", err)
				}
			}
		}

		handleSuccess(c, resp, provider, upstream.ServiceType, envCfg, sessionManager, startTime, &responsesReq, bodyBytes)
		return
	}

	log.Printf("💥 所有 Responses API密钥都失败了")
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
) {
	defer resp.Body.Close()

	isStream := originalReq != nil && originalReq.Stream

	if isStream {
		handleStreamSuccess(c, resp, upstreamType, envCfg, startTime, originalReq, originalRequestJSON)
		return
	}

	// 非流式响应处理
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to read response"})
		return
	}

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("⏱️ Responses 响应完成: %dms, 状态: %d", responseTime, resp.StatusCode)
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
			log.Printf("📋 响应头:\n%s", string(respHeadersJSON))

			var formattedBody string
			if envCfg.RawLogOutput {
				formattedBody = utils.FormatJSONBytesRaw(bodyBytes)
			} else {
				formattedBody = utils.FormatJSONBytesForLog(bodyBytes, 500)
			}
			log.Printf("📦 响应体:\n%s", formattedBody)
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
		return
	}

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
) {
	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("⏱️ Responses 流式响应开始: %dms, 状态: %d", responseTime, resp.StatusCode)
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

	for scanner.Scan() {
		line := scanner.Text()

		if streamLoggingEnabled {
			logBuffer.WriteString(line + "\n")
			if synthesizer != nil {
				synthesizer.ProcessLine(line)
			}
		}

		if needConvert {
			events := converters.ConvertOpenAIChatToResponses(
				c.Request.Context(),
				originalReq.Model,
				originalRequestJSON,
				nil,
				[]byte(line),
				&converterState,
			)
			for _, event := range events {
				_, err := c.Writer.Write([]byte(event))
				if err != nil {
					log.Printf("⚠️ 流式响应传输错误: %v", err)
					break
				}
			}
		} else {
			_, err := c.Writer.Write([]byte(line + "\n"))
			if err != nil {
				log.Printf("⚠️ 流式响应传输错误: %v", err)
				break
			}
		}

		if flusher != nil {
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("⚠️ 流式响应读取错误: %v", err)
	}

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("✅ Responses 流式响应完成: %dms", responseTime)

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
				log.Printf("🛰️  上游流式响应原始内容:\n%s", logBuffer.String())
			}
		}
	}
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
