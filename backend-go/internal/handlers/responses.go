package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/httpclient"
	"github.com/BenedictKing/claude-proxy/internal/middleware"
	"github.com/BenedictKing/claude-proxy/internal/providers"
	"github.com/BenedictKing/claude-proxy/internal/session"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
)

// ResponsesHandler Responses API 代理处理器
func ResponsesHandler(
	envCfg *config.EnvConfig,
	cfgManager *config.ConfigManager,
	sessionManager *session.SessionManager,
) gin.HandlerFunc {
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

		// 解析 Responses 请求
		var responsesReq types.ResponsesRequest
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &responsesReq)
		}

		// 获取当前 Responses 上游配置
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

		// 创建 ResponsesProvider
		provider := &providers.ResponsesProvider{
			SessionManager: sessionManager,
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

		for attempt := 0; attempt < maxRetries; attempt++ {
			apiKey, err := cfgManager.GetNextAPIKey(upstream, failedKeys)
			if err != nil {
				lastError = err
				break
			}

			if envCfg.ShouldLog("info") {
				log.Printf("🎯 使用 Responses 上游: %s - %s (尝试 %d/%d)", upstream.Name, upstream.BaseURL, attempt+1, maxRetries)
				log.Printf("🔑 使用API密钥: %s", maskAPIKey(apiKey))
			}

			// 转换请求
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

			// 请求日志
			if envCfg.EnableRequestLogs {
				log.Printf("📥 收到 Responses 请求: %s %s", c.Request.Method, c.Request.URL.Path)
				if envCfg.IsDevelopment() {
					formattedBody := utils.FormatJSONBytesForLog(lastOriginalBodyBytes, 500)
					log.Printf("📄 原始请求体:\n%s", formattedBody)
				}
			}

			// 发送请求
			resp, err := sendResponsesRequest(providerReq, upstream, envCfg)
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

				// 兜底处理：解压缩
				bodyBytes = utils.DecompressGzipIfNeeded(resp, bodyBytes)

				// 检查是否需要 failover
				shouldFailover, isQuotaRelated := shouldRetryWithNextKey(resp.StatusCode, bodyBytes)
				if shouldFailover {
					lastError = fmt.Errorf("上游错误: %d", resp.StatusCode)
					failedKeys[apiKey] = true
					cfgManager.MarkKeyAsFailed(apiKey)
					log.Printf("⚠️ API密钥失败，原因: %s", string(bodyBytes))

					lastFailoverError = &struct {
						Status int
						Body   []byte
					}{
						Status: resp.StatusCode,
						Body:   bodyBytes,
					}

					if isQuotaRelated {
						deprioritizeCandidates[apiKey] = true
					}

					continue
				}

				// 非 failover 错误，直接返回
				c.Data(resp.StatusCode, "application/json", bodyBytes)
				return
			}

			// 成功响应：降级失败的密钥
			if len(deprioritizeCandidates) > 0 {
				for key := range deprioritizeCandidates {
					if err := cfgManager.DeprioritizeAPIKey(key); err != nil {
						log.Printf("⚠️ 密钥降级失败: %v", err)
					}
				}
			}

			// 处理成功响应
			handleResponsesSuccess(c, resp, provider, upstream.ServiceType, envCfg, sessionManager, startTime, &responsesReq)
			return
		}

		// 所有密钥都失败了
		log.Printf("💥 所有 Responses API密钥都失败了")

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
			c.JSON(500, gin.H{
				"error":   "所有上游 Responses API密钥都不可用",
				"details": lastError.Error(),
			})
		}
	})
}

// sendResponsesRequest 发送 Responses 请求
func sendResponsesRequest(req *http.Request, upstream *config.UpstreamConfig, envCfg *config.EnvConfig) (*http.Response, error) {
	clientManager := httpclient.GetManager()

	// Responses 请求默认使用标准超时
	timeout := time.Duration(envCfg.RequestTimeout) * time.Millisecond
	client := clientManager.GetStandardClient(timeout, upstream.InsecureSkipVerify)

	if upstream.InsecureSkipVerify && envCfg.EnableRequestLogs {
		log.Printf("⚠️ 正在跳过对 %s 的TLS证书验证", req.URL.String())
	}

	if envCfg.EnableRequestLogs {
		log.Printf("🌐 实际请求URL: %s", req.URL.String())
	}

	return client.Do(req)
}

// handleResponsesSuccess 处理成功的 Responses 响应
func handleResponsesSuccess(
	c *gin.Context,
	resp *http.Response,
	provider *providers.ResponsesProvider,
	upstreamType string,
	envCfg *config.EnvConfig,
	sessionManager *session.SessionManager,
	startTime time.Time,
	originalReq *types.ResponsesRequest,
) {
	defer resp.Body.Close()

	// 检查是否为流式响应
	isStream := originalReq != nil && originalReq.Stream

	if isStream {
		// 流式响应处理:直接转发SSE流
		if envCfg.EnableResponseLogs {
			responseTime := time.Since(startTime).Milliseconds()
			log.Printf("⏱️ Responses 流式响应开始: %dms, 状态: %d", responseTime, resp.StatusCode)
		}

		// 设置SSE响应头
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		// 直接转发流式响应
		c.Status(resp.StatusCode)
		_, err := io.Copy(c.Writer, resp.Body)
		if err != nil {
			log.Printf("⚠️ 流式响应传输错误: %v", err)
		}

		if envCfg.EnableResponseLogs {
			responseTime := time.Since(startTime).Milliseconds()
			log.Printf("✅ Responses 流式响应完成: %dms", responseTime)
		}
		return
	}

	// 非流式响应处理(原有逻辑)
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to read response"})
		return
	}

	if envCfg.EnableResponseLogs {
		responseTime := time.Since(startTime).Milliseconds()
		log.Printf("⏱️ Responses 响应完成: %dms, 状态: %d", responseTime, resp.StatusCode)
		if envCfg.IsDevelopment() {
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

	// 转换为 Responses 格式
	responsesResp, err := provider.ConvertToResponsesResponse(providerResp, upstreamType, "")
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to convert response"})
		return
	}

	// 更新会话（如果需要）
	if originalReq.Store == nil || *originalReq.Store {
		// 获取会话
		sess, err := sessionManager.GetOrCreateSession(originalReq.PreviousResponseID)
		if err == nil {
			// 追加用户输入
			inputItems, _ := parseInputToItems(originalReq.Input)
			for _, item := range inputItems {
				sessionManager.AppendMessage(sess.ID, item, 0)
			}

			// 追加助手响应
			for _, item := range responsesResp.Output {
				sessionManager.AppendMessage(sess.ID, item, responsesResp.Usage.TotalTokens)
			}

			// 更新 last response ID
			sessionManager.UpdateLastResponseID(sess.ID, responsesResp.ID)

			// 记录映射
			sessionManager.RecordResponseMapping(responsesResp.ID, sess.ID)

			// 设置 previous_id
			if sess.LastResponseID != "" {
				responsesResp.PreviousID = sess.LastResponseID
			}
		}
	}

	c.JSON(200, responsesResp)
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
