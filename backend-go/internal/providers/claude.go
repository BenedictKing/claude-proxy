package providers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
	"github.com/gin-gonic/gin"
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
	// 智能拼接逻辑：
	// 1. 如果 baseURL 已包含版本号后缀（如 /v1, /v2, /v3），直接拼接端点路径
	// 2. 如果 baseURL 不包含版本号后缀，自动添加 /v1 再拼接端点路径
	endpoint := strings.TrimPrefix(c.Request.URL.Path, "/v1")
	baseURL := strings.TrimSuffix(upstream.BaseURL, "/")

	// 使用正则表达式检测 baseURL 是否以版本号结尾（/v1, /v2, /v1beta, /v2alpha等）
	versionPattern := regexp.MustCompile(`/v\d+[a-z]*$`)

	var targetURL string
	if versionPattern.MatchString(baseURL) {
		// baseURL 已包含版本号，直接拼接
		targetURL = baseURL + endpoint
	} else {
		// baseURL 不包含版本号，添加 /v1
		targetURL = baseURL + "/v1" + endpoint
	}

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

	// 使用统一的头部处理逻辑
	req.Header = utils.PrepareUpstreamHeaders(c, req.URL.Host)
	utils.SetAuthenticationHeader(req.Header, apiKey)
	utils.EnsureCompatibleUserAgent(req.Header, "claude")

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
		// 设置更大的 buffer (1MB) 以处理大 JSON chunk，避免默认 64KB 限制
		const maxScannerBufferSize = 1024 * 1024 // 1MB
		scanner.Buffer(make([]byte, 0, 64*1024), maxScannerBufferSize)

		toolUseStopEmitted := false

		for scanner.Scan() {
			line := scanner.Text()

			// 直接转发 SSE 事件（包括空行）
			if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") || line == "" {
				eventChan <- line + "\n"

				// 检测是否发送了 tool_use 相关的 stop_reason
				if strings.Contains(line, `"stop_reason":"tool_use"`) ||
					strings.Contains(line, `"stop_reason": "tool_use"`) {
					toolUseStopEmitted = true
				}
			}
		}

		if err := scanner.Err(); err != nil {
			// 在 tool_use 场景下，客户端主动断开是正常行为
			// 如果已经发送了 tool_use stop 事件，并且错误是连接断开相关的，则忽略该错误
			errMsg := err.Error()
			if toolUseStopEmitted && (strings.Contains(errMsg, "broken pipe") ||
				strings.Contains(errMsg, "connection reset") ||
				strings.Contains(errMsg, "EOF")) {
				// 这是预期的客户端行为，不报告错误
				return
			}
			errChan <- err
		}
	}()

	return eventChan, errChan, nil
}

// OpenAIOldProvider 旧版 OpenAI 提供商
type OpenAIOldProvider struct {
	OpenAIProvider
}
