// Package common 提供 handlers 模块的公共功能
package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/httpclient"
	"github.com/BenedictKing/claude-proxy/internal/metrics"
	"github.com/BenedictKing/claude-proxy/internal/utils"
	"github.com/gin-gonic/gin"
)

// ReadRequestBody 读取并验证请求体大小
// 返回: (bodyBytes, error)
// 如果请求体过大，会自动返回 413 错误并排空剩余数据
func ReadRequestBody(c *gin.Context, maxBodySize int64) ([]byte, error) {
	limitedReader := io.LimitReader(c.Request.Body, maxBodySize+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		c.JSON(400, gin.H{"error": "Failed to read request body"})
		return nil, err
	}

	if int64(len(bodyBytes)) > maxBodySize {
		// 排空剩余请求体，避免 keep-alive 连接污染
		io.Copy(io.Discard, c.Request.Body)
		c.JSON(413, gin.H{"error": fmt.Sprintf("Request body too large, maximum size is %d MB", maxBodySize/1024/1024)})
		return nil, fmt.Errorf("request body too large")
	}

	// 恢复请求体供后续使用
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

// RestoreRequestBody 恢复请求体供后续使用
func RestoreRequestBody(c *gin.Context, bodyBytes []byte) {
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
}

// SendRequest 发送 HTTP 请求到上游
// isStream: 是否为流式请求（流式请求使用无超时客户端）
func SendRequest(req *http.Request, upstream *config.UpstreamConfig, envCfg *config.EnvConfig, isStream bool) (*http.Response, error) {
	clientManager := httpclient.GetManager()

	var client *http.Client
	if isStream {
		client = clientManager.GetStreamClient(upstream.InsecureSkipVerify)
	} else {
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
			logRequestDetails(req, envCfg)
		}
	}

	return client.Do(req)
}

// logRequestDetails 记录请求详情（仅开发模式）
func logRequestDetails(req *http.Request, envCfg *config.EnvConfig) {
	// 对请求头做敏感信息脱敏
	reqHeaders := make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			reqHeaders[key] = values[0]
		}
	}
	maskedReqHeaders := utils.MaskSensitiveHeaders(reqHeaders)
	var reqHeadersJSON []byte
	if envCfg.RawLogOutput {
		reqHeadersJSON, _ = json.Marshal(maskedReqHeaders)
	} else {
		reqHeadersJSON, _ = json.MarshalIndent(maskedReqHeaders, "", "  ")
	}
	log.Printf("📋 实际请求头:\n%s", string(reqHeadersJSON))

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			var formattedBody string
			if envCfg.RawLogOutput {
				formattedBody = utils.FormatJSONBytesRaw(bodyBytes)
			} else {
				formattedBody = utils.FormatJSONBytesForLog(bodyBytes, 500)
			}
			log.Printf("📦 实际请求体:\n%s", formattedBody)
		}
	}
}

// LogOriginalRequest 记录原始请求信息
func LogOriginalRequest(c *gin.Context, bodyBytes []byte, envCfg *config.EnvConfig, apiType string) {
	if !envCfg.EnableRequestLogs {
		return
	}

	log.Printf("📥 收到%s请求: %s %s", apiType, c.Request.Method, c.Request.URL.Path)

	if envCfg.IsDevelopment() {
		var formattedBody string
		if envCfg.RawLogOutput {
			formattedBody = utils.FormatJSONBytesRaw(bodyBytes)
		} else {
			formattedBody = utils.FormatJSONBytesForLog(bodyBytes, 500)
		}
		log.Printf("📄 原始请求体:\n%s", formattedBody)

		sanitizedHeaders := make(map[string]string)
		for key, values := range c.Request.Header {
			if len(values) > 0 {
				sanitizedHeaders[key] = values[0]
			}
		}
		maskedHeaders := utils.MaskSensitiveHeaders(sanitizedHeaders)
		var headersJSON []byte
		if envCfg.RawLogOutput {
			headersJSON, _ = json.Marshal(maskedHeaders)
		} else {
			headersJSON, _ = json.MarshalIndent(maskedHeaders, "", "  ")
		}
		log.Printf("📥 原始请求头:\n%s", string(headersJSON))
	}
}

// AreAllKeysSuspended 检查渠道的所有 Key 是否都处于熔断状态
// 用于判断是否需要启用强制探测模式
func AreAllKeysSuspended(metricsManager *metrics.MetricsManager, baseURL string, apiKeys []string) bool {
	if len(apiKeys) == 0 {
		return false
	}

	for _, apiKey := range apiKeys {
		if !metricsManager.ShouldSuspendKey(baseURL, apiKey) {
			return false
		}
	}
	return true
}

// ExtractUserID 从请求体中提取 user_id（用于 Messages API）
func ExtractUserID(bodyBytes []byte) string {
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

// ExtractConversationID 从请求中提取对话标识（用于 Responses API）
// 优先级: Conversation_id Header > Session_id Header > prompt_cache_key > metadata.user_id
func ExtractConversationID(c *gin.Context, bodyBytes []byte) string {
	// 1. HTTP Header: Conversation_id
	if convID := c.GetHeader("Conversation_id"); convID != "" {
		return convID
	}

	// 2. HTTP Header: Session_id
	if sessID := c.GetHeader("Session_id"); sessID != "" {
		return sessID
	}

	// 3. Request Body: prompt_cache_key 或 metadata.user_id
	var req struct {
		PromptCacheKey string `json:"prompt_cache_key"`
		Metadata       struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err == nil {
		if req.PromptCacheKey != "" {
			return req.PromptCacheKey
		}
		if req.Metadata.UserID != "" {
			return req.Metadata.UserID
		}
	}

	return ""
}
