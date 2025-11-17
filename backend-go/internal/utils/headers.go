package utils

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// PrepareUpstreamHeaders 准备上游请求头（统一头部处理逻辑）
// 保留原始请求头，移除代理相关头部，设置认证头
// 注意：此函数适用于Claude类型渠道，对于其他类型请使用 PrepareMinimalHeaders
func PrepareUpstreamHeaders(c *gin.Context, targetHost string) http.Header {
	headers := c.Request.Header.Clone()

	// 设置正确的Host头部
	headers.Set("Host", targetHost)

	// 移除代理相关头部
	headers.Del("x-proxy-key")
	headers.Del("X-Forwarded-Host")
	headers.Del("X-Forwarded-Proto")

	// 移除 Accept-Encoding，让 Go 的 http.Client 自动处理 gzip 压缩/解压缩
	// 这样可以避免在原始请求包含 Accept-Encoding 时 Go 不自动解压缩的问题
	headers.Del("Accept-Encoding")

	return headers
}

// PrepareMinimalHeaders 准备最小化请求头（适用于非Claude渠道如OpenAI、Gemini等）
// 只保留必要的头部：Content-Type和Host，不包含任何Anthropic特定头部
// 注意：不设置Accept-Encoding，让Go的http.Client自动处理gzip压缩
func PrepareMinimalHeaders(targetHost string) http.Header {
	headers := http.Header{}

	// 只设置最基本的头部
	headers.Set("Host", targetHost)
	headers.Set("Content-Type", "application/json")
	// 不显式设置Accept-Encoding，让Go的http.Client自动添加并处理gzip解压

	return headers
}

// SetAuthenticationHeader 设置认证头部（统一使用Bearer格式）
func SetAuthenticationHeader(headers http.Header, apiKey string) {
	// 移除旧的认证头
	headers.Del("authorization")
	headers.Del("x-api-key")
	headers.Del("x-goog-api-key")

	// 统一使用 Authorization: Bearer 格式
	// 所有上游服务（claude, responses, openai 等）都使用此格式
	headers.Set("Authorization", "Bearer "+apiKey)
}

// SetGeminiAuthenticationHeader 设置Gemini认证头部
func SetGeminiAuthenticationHeader(headers http.Header, apiKey string) {
	headers.Del("authorization")
	headers.Del("x-api-key")
	headers.Set("x-goog-api-key", apiKey)
}

// EnsureCompatibleUserAgent 确保兼容的User-Agent（仅在必要时设置）
func EnsureCompatibleUserAgent(headers http.Header, serviceType string) {
	userAgent := headers.Get("User-Agent")

	// 仅在Claude服务类型且用户未设置或设置不正确时才修改
	if serviceType == "claude" {
		if userAgent == "" || !strings.HasPrefix(strings.ToLower(userAgent), "claude-cli") {
			headers.Set("User-Agent", "claude-cli/1.0.58 (external, cli)")
		}
	}
}

// ForwardResponseHeaders 转发上游响应头到客户端
// 作为透明代理，应该转发所有响应头，只过滤框架自动处理的头部
func ForwardResponseHeaders(upstreamHeaders http.Header, clientWriter http.ResponseWriter) {
	// 不应转发的头部列表（由框架或代理层自动处理）
	skipHeaders := map[string]bool{
		"transfer-encoding": true, // 由框架自动处理
		"content-length":    true, // 由框架自动处理
		"connection":        true, // 代理层控制
		"content-encoding":  true, // 如果已解压则不应转发
	}

	// 复制所有上游响应头到客户端
	for key, values := range upstreamHeaders {
		lowerKey := strings.ToLower(key)

		// 跳过不应转发的头部
		if skipHeaders[lowerKey] {
			continue
		}

		// 转发头部（可能有多个值）
		for _, value := range values {
			clientWriter.Header().Add(key, value)
		}
	}
}
