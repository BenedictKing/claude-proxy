package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/converters"
	"github.com/BenedictKing/claude-proxy/internal/session"
	"github.com/BenedictKing/claude-proxy/internal/types"
)

// ResponsesProvider Responses API 提供商
type ResponsesProvider struct {
	SessionManager *session.SessionManager
}

// ConvertToProviderRequest 将 Responses 请求转换为上游格式
func (p *ResponsesProvider) ConvertToProviderRequest(
	c *gin.Context,
	upstream *config.UpstreamConfig,
	apiKey string,
) (*http.Request, []byte, error) {
	// 1. 解析 Responses 请求
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("读取请求体失败: %w", err)
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var responsesReq types.ResponsesRequest
	if err := json.Unmarshal(bodyBytes, &responsesReq); err != nil {
		return nil, bodyBytes, fmt.Errorf("解析 Responses 请求失败: %w", err)
	}

	// 2. 获取或创建会话
	sess, err := p.SessionManager.GetOrCreateSession(responsesReq.PreviousResponseID)
	if err != nil {
		return nil, bodyBytes, fmt.Errorf("获取会话失败: %w", err)
	}

	// 3. 模型重定向
	model := config.RedirectModel(responsesReq.Model, upstream)

	// 4. 根据上游类型选择转换策略
	switch upstream.ServiceType {
	case "responses":
		// 透传（原生 Responses 上游）
		return p.passthroughRequest(c, upstream, apiKey, responsesReq, model, bodyBytes)

	case "claude":
		// 转换为 Claude Messages 格式
		return p.convertToClaudeRequest(c, upstream, apiKey, sess, responsesReq, model, bodyBytes)

	case "openai":
		// 转换为 OpenAI Chat 格式
		return p.convertToOpenAIChatRequest(c, upstream, apiKey, sess, responsesReq, model, bodyBytes)

	case "openaiold":
		// 转换为 OpenAI Completions 格式
		return p.convertToOpenAICompletionsRequest(c, upstream, apiKey, sess, responsesReq, model, bodyBytes)

	default:
		return nil, bodyBytes, fmt.Errorf("不支持的上游类型: %s", upstream.ServiceType)
	}
}

// passthroughRequest 透传请求（原生 Responses 上游）
func (p *ResponsesProvider) passthroughRequest(
	c *gin.Context,
	upstream *config.UpstreamConfig,
	apiKey string,
	responsesReq types.ResponsesRequest,
	model string,
	originalBody []byte,
) (*http.Request, []byte, error) {
	// 仅修改 model 字段（如果有重定向）
	if model != responsesReq.Model {
		responsesReq.Model = model
		modifiedBody, err := json.Marshal(responsesReq)
		if err == nil {
			originalBody = modifiedBody
		}
	}

	targetURL := fmt.Sprintf("%s/v1/responses", strings.TrimSuffix(upstream.BaseURL, "/"))
	req, err := http.NewRequest("POST", targetURL, bytes.NewReader(originalBody))
	if err != nil {
		return nil, originalBody, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	return req, originalBody, nil
}

// convertToClaudeRequest 转换为 Claude 请求
func (p *ResponsesProvider) convertToClaudeRequest(
	c *gin.Context,
	upstream *config.UpstreamConfig,
	apiKey string,
	sess *session.Session,
	responsesReq types.ResponsesRequest,
	model string,
	originalBody []byte,
) (*http.Request, []byte, error) {
	// 将 Responses 转换为 Claude Messages
	messages, err := converters.ResponsesToClaudeMessages(sess, responsesReq.Input)
	if err != nil {
		return nil, originalBody, fmt.Errorf("转换为 Claude Messages 失败: %w", err)
	}

	// 构建 Claude 请求
	claudeReq := map[string]interface{}{
		"model":      model,
		"messages":   messages,
		"max_tokens": 4096,
	}

	if responsesReq.MaxTokens > 0 {
		claudeReq["max_tokens"] = responsesReq.MaxTokens
	}
	if responsesReq.Temperature > 0 {
		claudeReq["temperature"] = responsesReq.Temperature
	}
	if responsesReq.TopP > 0 {
		claudeReq["top_p"] = responsesReq.TopP
	}
	if responsesReq.Stream {
		claudeReq["stream"] = true
	}

	reqBody, err := json.Marshal(claudeReq)
	if err != nil {
		return nil, originalBody, err
	}

	targetURL := fmt.Sprintf("%s/v1/messages", strings.TrimSuffix(upstream.BaseURL, "/"))
	req, err := http.NewRequest("POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, originalBody, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	return req, originalBody, nil
}

// convertToOpenAIChatRequest 转换为 OpenAI Chat 请求
func (p *ResponsesProvider) convertToOpenAIChatRequest(
	c *gin.Context,
	upstream *config.UpstreamConfig,
	apiKey string,
	sess *session.Session,
	responsesReq types.ResponsesRequest,
	model string,
	originalBody []byte,
) (*http.Request, []byte, error) {
	// 将 Responses 转换为 OpenAI Messages
	messages, err := converters.ResponsesToOpenAIChatMessages(sess, responsesReq.Input)
	if err != nil {
		return nil, originalBody, fmt.Errorf("转换为 OpenAI Messages 失败: %w", err)
	}

	// 构建 OpenAI 请求
	openaiReq := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}

	if responsesReq.MaxTokens > 0 {
		openaiReq["max_tokens"] = responsesReq.MaxTokens
	}
	if responsesReq.Temperature > 0 {
		openaiReq["temperature"] = responsesReq.Temperature
	}
	if responsesReq.TopP > 0 {
		openaiReq["top_p"] = responsesReq.TopP
	}
	if responsesReq.FrequencyPenalty > 0 {
		openaiReq["frequency_penalty"] = responsesReq.FrequencyPenalty
	}
	if responsesReq.PresencePenalty > 0 {
		openaiReq["presence_penalty"] = responsesReq.PresencePenalty
	}
	if responsesReq.Stream {
		openaiReq["stream"] = true
	}

	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, originalBody, err
	}

	targetURL := fmt.Sprintf("%s/v1/chat/completions", strings.TrimSuffix(upstream.BaseURL, "/"))
	req, err := http.NewRequest("POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, originalBody, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	return req, originalBody, nil
}

// convertToOpenAICompletionsRequest 转换为 OpenAI Completions 请求
func (p *ResponsesProvider) convertToOpenAICompletionsRequest(
	c *gin.Context,
	upstream *config.UpstreamConfig,
	apiKey string,
	sess *session.Session,
	responsesReq types.ResponsesRequest,
	model string,
	originalBody []byte,
) (*http.Request, []byte, error) {
	// 提取纯文本 prompt
	prompt, err := converters.ExtractTextFromResponses(sess, responsesReq.Input)
	if err != nil {
		return nil, originalBody, fmt.Errorf("提取文本失败: %w", err)
	}

	// 构建 OpenAI Completions 请求
	completionsReq := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
	}

	if responsesReq.MaxTokens > 0 {
		completionsReq["max_tokens"] = responsesReq.MaxTokens
	}
	if responsesReq.Temperature > 0 {
		completionsReq["temperature"] = responsesReq.Temperature
	}
	if responsesReq.Stream {
		completionsReq["stream"] = true
	}

	reqBody, err := json.Marshal(completionsReq)
	if err != nil {
		return nil, originalBody, err
	}

	targetURL := fmt.Sprintf("%s/v1/completions", strings.TrimSuffix(upstream.BaseURL, "/"))
	req, err := http.NewRequest("POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, originalBody, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	return req, originalBody, nil
}

// ConvertToClaudeResponse 将上游响应转换为 Responses 格式（实际上不再需要 Claude 格式）
func (p *ResponsesProvider) ConvertToClaudeResponse(providerResp *types.ProviderResponse) (*types.ClaudeResponse, error) {
	// 这个方法在 ResponsesHandler 中不会被调用，这里提供兼容性实现
	return nil, fmt.Errorf("ResponsesProvider 不支持 ConvertToClaudeResponse")
}

// ConvertToResponsesResponse 将上游响应转换为 Responses 格式
func (p *ResponsesProvider) ConvertToResponsesResponse(
	providerResp *types.ProviderResponse,
	upstreamType string,
	sessionID string,
) (*types.ResponsesResponse, error) {
	// 解析响应体为 map
	respMap, err := converters.JSONToMap(providerResp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	switch upstreamType {
	case "responses":
		// 透传（已经是 Responses 格式）
		var responsesResp types.ResponsesResponse
		if err := json.Unmarshal(providerResp.Body, &responsesResp); err != nil {
			return nil, err
		}
		return &responsesResp, nil

	case "claude":
		return converters.ClaudeResponseToResponses(respMap, sessionID)

	case "openai":
		return converters.OpenAIChatResponseToResponses(respMap, sessionID)

	case "openaiold":
		return converters.OpenAICompletionsResponseToResponses(respMap, sessionID)

	default:
		return nil, fmt.Errorf("不支持的上游类型: %s", upstreamType)
	}
}

// HandleStreamResponse 处理流式响应（暂不实现）
func (p *ResponsesProvider) HandleStreamResponse(body io.ReadCloser) (<-chan string, <-chan error, error) {
	return nil, nil, fmt.Errorf("Responses Provider 暂不支持流式响应")
}
