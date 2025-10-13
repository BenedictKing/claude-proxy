package providers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/types"
)

// ClaudeProvider Claude 提供商（直接透传）
type ClaudeProvider struct{}

// ConvertToProviderRequest 转换为 Claude 请求（实现真正的透传）
func (p *ClaudeProvider) ConvertToProviderRequest(c *gin.Context, upstream *config.UpstreamConfig, apiKey string) (*http.Request, []byte, error) {
	var bodyBytes []byte
	var err error

	// 仅在需要模型重定向时才解析和重构请求体
	if upstream.ModelMapping != nil && len(upstream.ModelMapping) > 0 {
		bodyBytes, err = io.ReadAll(c.Request.Body)
		if err != nil {
			return nil, nil, err
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes)) // 恢复body

		var claudeReq types.ClaudeRequest
		if err := json.Unmarshal(bodyBytes, &claudeReq); err != nil {
			return nil, bodyBytes, err
		}
		claudeReq.Model = config.RedirectModel(claudeReq.Model, upstream)

		bodyBytes, err = json.Marshal(claudeReq)
		if err != nil {
			return nil, nil, err
		}
	} else {
		// 如果不需要模型重定向，则直接从原始请求中读取body用于日志和请求转发
		bodyBytes, err = io.ReadAll(c.Request.Body)
		if err != nil {
			return nil, nil, err
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes)) // 恢复body
	}

	// 构建目标URL
	endpoint := strings.TrimPrefix(c.Request.URL.Path, "/v1")
	targetURL := strings.TrimSuffix(upstream.BaseURL, "/") + endpoint
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	// 创建请求
	var req *http.Request
	if len(bodyBytes) > 0 {
		req, err = http.NewRequest(c.Request.Method, targetURL, bytes.NewReader(bodyBytes))
	} else {
		// 如果 bodyBytes 为空（例如 GET 请求或原始请求体为空），则直接使用 nil Body
		req, err = http.NewRequest(c.Request.Method, targetURL, nil)
	}
	if err != nil {
		return nil, nil, err
	}

	// 复制并修改Header
	req.Header = c.Request.Header.Clone()
	req.Host = req.URL.Host // 设置正确的Host头部

	// 正确设置认证头
	if strings.HasPrefix(apiKey, "sk-ant-") {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Del("Authorization")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Del("x-api-key")
	}
	req.Header.Del("x-proxy-key") // 移除代理访问密钥

	// 确保兼容的User-Agent（如果用户未设置或设置不正确）
	userAgent := req.Header.Get("User-Agent")
	if userAgent == "" || !strings.HasPrefix(strings.ToLower(userAgent), "claude-cli") {
		req.Header.Set("User-Agent", "claude-cli/1.0.58 (external, cli)")
	}

	// 移除可能存在的代理在请求链路中添加的 Host 头，确保使用正确的目标 Host
	req.Header.Del("X-Forwarded-Host")
	req.Header.Del("X-Forwarded-Proto")

	return req, bodyBytes, nil
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

			// 直接转发 SSE 事件（包括空行）
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
