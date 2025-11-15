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
	responsesReq.Model = config.RedirectModel(responsesReq.Model, upstream)

	// 4. 使用转换器工厂创建转换器
	converter := converters.NewConverter(upstream.ServiceType)

	// 5. 转换请求
	providerReq, err := converter.ToProviderRequest(sess, &responsesReq)
	if err != nil {
		return nil, bodyBytes, fmt.Errorf("转换请求失败: %w", err)
	}

	// 6. 序列化请求体
	reqBody, err := json.Marshal(providerReq)
	if err != nil {
		return nil, bodyBytes, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 7. 构建 HTTP 请求
	targetURL := p.buildTargetURL(upstream)
	req, err := http.NewRequest("POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, bodyBytes, err
	}

	// 8. 设置请求头
	p.setRequestHeaders(req, upstream, apiKey)

	return req, bodyBytes, nil
}

// buildTargetURL 根据上游类型构建目标 URL
func (p *ResponsesProvider) buildTargetURL(upstream *config.UpstreamConfig) string {
	baseURL := strings.TrimSuffix(upstream.BaseURL, "/")

	switch upstream.ServiceType {
	case "responses":
		return fmt.Sprintf("%s/v1/responses", baseURL)
	case "claude":
		return fmt.Sprintf("%s/v1/messages", baseURL)
	case "openai":
		return fmt.Sprintf("%s/v1/chat/completions", baseURL)
	case "openaiold":
		return fmt.Sprintf("%s/v1/completions", baseURL)
	default:
		return fmt.Sprintf("%s/v1/chat/completions", baseURL)
	}
}

// setRequestHeaders 设置请求头
func (p *ResponsesProvider) setRequestHeaders(req *http.Request, upstream *config.UpstreamConfig, apiKey string) {
	req.Header.Set("Content-Type", "application/json")

	switch upstream.ServiceType {
	case "claude":
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}
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

	// 使用转换器工厂
	converter := converters.NewConverter(upstreamType)
	return converter.FromProviderResponse(respMap, sessionID)
}

// HandleStreamResponse 处理流式响应（暂不实现）
func (p *ResponsesProvider) HandleStreamResponse(body io.ReadCloser) (<-chan string, <-chan error, error) {
	return nil, nil, fmt.Errorf("Responses Provider 暂不支持流式响应")
}
