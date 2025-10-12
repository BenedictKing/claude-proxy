package providers

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/yourusername/claude-proxy/internal/config"
	"github.com/yourusername/claude-proxy/internal/types"
)

// ClaudeProvider Claude 提供商（直接透传）
type ClaudeProvider struct{}

// ConvertToProviderRequest 转换为 Claude 请求（直接透传）
func (p *ClaudeProvider) ConvertToProviderRequest(claudeReq *types.ClaudeRequest, upstream *config.UpstreamConfig, apiKey string) (*types.ProviderRequest, error) {
	// 应用模型重定向
	claudeReq.Model = config.RedirectModel(claudeReq.Model, upstream)

	body, err := json.Marshal(claudeReq)
	if err != nil {
		return nil, err
	}

	// 智能构建URL：避免 /v1 重复
	baseURL := strings.TrimSuffix(upstream.BaseURL, "/")
	var url string
	if strings.HasSuffix(baseURL, "/v1") {
		// BaseURL 已包含 /v1，只添加 /messages
		url = baseURL + "/messages"
	} else {
		// BaseURL 不包含 /v1，添加完整路径
		url = baseURL + "/v1/messages"
	}

	return &types.ProviderRequest{
		URL:    url,
		Method: "POST",
		Headers: map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": "2023-06-01",
			"Content-Type":      "application/json",
		},
		Body: body,
	}, nil
}

// ConvertToClaudeResponse 转换为 Claude 响应（直接透传）
func (p *ClaudeProvider) ConvertToClaudeResponse(providerResp *types.ProviderResponse) (*types.ClaudeResponse, error) {
	var claudeResp types.ClaudeResponse
	if err := json.Unmarshal(providerResp.Body, &claudeResp); err != nil {
		return nil, err
	}
	return &claudeResp, nil
}

// HandleStreamResponse 处理流式响应（直接透传）
func (p *ClaudeProvider) HandleStreamResponse(body io.ReadCloser) (<-chan string, <-chan error, error) {
	eventChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(eventChan)
		defer close(errChan)
		defer body.Close()

		scanner := bufio.NewScanner(body)

		for scanner.Scan() {
			line := scanner.Text()

			// 直接转发 SSE 事件
			if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") || line == "" {
				eventChan <- line + "\n"
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- err
		}
	}()

	return eventChan, errChan, nil
}

// OpenAIOldProvider 旧版 OpenAI 提供商
type OpenAIOldProvider struct {
	OpenAIProvider
}
