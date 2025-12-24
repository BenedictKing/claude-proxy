package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/converters"
	"github.com/BenedictKing/claude-proxy/internal/session"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
	"github.com/gin-gonic/gin"
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
	// 1. 读取原始请求体
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("读取请求体失败: %w", err)
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var providerReq interface{}

	// 2. 使用转换器工厂创建转换器
	converter := converters.NewConverter(upstream.ServiceType)

	// 3. 判断是否为透传模式
	if _, ok := converter.(*converters.ResponsesPassthroughConverter); ok {
		// [Mode-Passthrough] 透传模式：使用 map 保留所有字段
		var reqMap map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &reqMap); err != nil {
			return nil, bodyBytes, fmt.Errorf("透传模式下解析请求失败: %w", err)
		}

		// 只做模型重定向
		if model, ok := reqMap["model"].(string); ok {
			reqMap["model"] = config.RedirectModel(model, upstream)
		}

		providerReq = reqMap
	} else {
		// [Mode-Convert] 非透传模式：保持原有逻辑
		var responsesReq types.ResponsesRequest
		if err := json.Unmarshal(bodyBytes, &responsesReq); err != nil {
			return nil, bodyBytes, fmt.Errorf("解析 Responses 请求失败: %w", err)
		}

		// 获取或创建会话
		sess, err := p.SessionManager.GetOrCreateSession(responsesReq.PreviousResponseID)
		if err != nil {
			return nil, bodyBytes, fmt.Errorf("获取会话失败: %w", err)
		}

		// 模型重定向
		responsesReq.Model = config.RedirectModel(responsesReq.Model, upstream)

		// 转换请求
		convertedReq, err := converter.ToProviderRequest(sess, &responsesReq)
		if err != nil {
			return nil, bodyBytes, fmt.Errorf("转换请求失败: %w", err)
		}
		providerReq = convertedReq
	}

	// 4. 序列化请求体
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

	// 8. 设置请求头（透明代理）
	// 使用统一的头部处理逻辑，保留客户端的大部分 headers
	req.Header = utils.PrepareUpstreamHeaders(c, req.URL.Host)

	// 删除客户端的所有认证头，避免冲突
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")

	// 根据 ServiceType 设置对应的认证头
	switch upstream.ServiceType {
	case "gemini":
		// 只有 Gemini 使用特殊的认证头
		utils.SetGeminiAuthenticationHeader(req.Header, apiKey)
	default:
		// claude, responses, openai 等都使用 Authorization: Bearer
		utils.SetAuthenticationHeader(req.Header, apiKey)
	}

	// 确保 Content-Type 正确
	req.Header.Set("Content-Type", "application/json")

	return req, bodyBytes, nil
}

// buildTargetURL 根据上游类型构建目标 URL
// 智能拼接逻辑：
// 1. 如果 baseURL 以 # 结尾，跳过自动添加 /v1
// 2. 如果 baseURL 已包含版本号后缀（如 /v1, /v2, /v8, /v1beta），直接拼接端点路径
// 3. 如果 baseURL 不包含版本号后缀，自动添加 /v1 再拼接端点路径
func (p *ResponsesProvider) buildTargetURL(upstream *config.UpstreamConfig) string {
	baseURL := upstream.BaseURL
	skipVersionPrefix := strings.HasSuffix(baseURL, "#")
	if skipVersionPrefix {
		baseURL = strings.TrimSuffix(baseURL, "#")
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 使用正则表达式检测 baseURL 是否以版本号结尾（/v1, /v2, /v1beta, /v2alpha等）
	versionPattern := regexp.MustCompile(`/v\d+[a-z]*$`)
	hasVersionSuffix := versionPattern.MatchString(baseURL)

	// 根据 ServiceType 确定端点路径
	var endpoint string
	switch upstream.ServiceType {
	case "responses":
		endpoint = "/responses"
	case "claude":
		endpoint = "/messages"
	default:
		endpoint = "/chat/completions"
	}

	// 如果 baseURL 已包含版本号或以#结尾，直接拼接端点
	// 否则添加 /v1 再拼接端点
	if hasVersionSuffix || skipVersionPrefix {
		return baseURL + endpoint
	}
	return baseURL + "/v1" + endpoint
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
