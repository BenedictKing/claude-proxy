package providers

import (
	"io"

	"github.com/yourusername/claude-proxy/internal/config"
	"github.com/yourusername/claude-proxy/internal/types"
)

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/claude-proxy/internal/config"
	"github.com/yourusername/claude-proxy/internal/types"
)

// Provider 提供商接口
type Provider interface {
	// ConvertToProviderRequest 将 gin context 中的请求转换为目标上游的 http.Request，并返回用于日志的原始请求体
	ConvertToProviderRequest(c *gin.Context, upstream *config.UpstreamConfig, apiKey string) (*http.Request, []byte, error)

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
