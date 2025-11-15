package converters

import (
	"github.com/BenedictKing/claude-proxy/internal/session"
	"github.com/BenedictKing/claude-proxy/internal/types"
)

// ResponsesConverter 定义 Responses API 转换器接口
// 用于将 Responses 格式转换为不同上游服务的格式
type ResponsesConverter interface {
	// ToProviderRequest 将 Responses 请求转换为上游服务的请求格式
	// 返回：请求体（map 或其他类型）、错误
	ToProviderRequest(sess *session.Session, req *types.ResponsesRequest) (interface{}, error)

	// FromProviderResponse 将上游服务的响应转换为 Responses 格式
	// 返回：Responses 响应、错误
	FromProviderResponse(resp map[string]interface{}, sessionID string) (*types.ResponsesResponse, error)

	// GetProviderName 获取上游服务名称（用于日志和调试）
	GetProviderName() string
}
