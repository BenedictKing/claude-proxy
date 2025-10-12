package providers

import (
	"io"

	"github.com/yourusername/claude-proxy/internal/config"
	"github.com/yourusername/claude-proxy/internal/types"
)

// Provider 提供商接口
type Provider interface {
	// ConvertToProviderRequest 将 Claude 请求转换为提供商请求
	ConvertToProviderRequest(claudeReq *types.ClaudeRequest, upstream *config.UpstreamConfig, apiKey string) (*types.ProviderRequest, error)

	// ConvertToClaudeResponse 将提供商响应转换为 Claude 响应
	ConvertToClaudeResponse(providerResp *types.ProviderResponse) (*types.ClaudeResponse, error)

	// HandleStreamResponse 处理流式响应
	HandleStreamResponse(body io.ReadCloser) (<-chan string, <-chan error, error)
}

// GetProvider 根据服务类型获取提供商
func GetProvider(serviceType string) Provider {
	switch serviceType {
	case "openai":
		return &OpenAIProvider{}
	case "openaiold":
		return &OpenAIOldProvider{}
	case "gemini":
		return &GeminiProvider{}
	case "claude":
		return &ClaudeProvider{}
	default:
		return nil
	}
}
