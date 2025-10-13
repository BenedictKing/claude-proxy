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

// SetAuthenticationHeader 设置认证头部（支持Claude和通用Bearer格式）
func SetAuthenticationHeader(headers http.Header, apiKey string) {
	// 移除旧的认证头
	headers.Del("authorization")
	headers.Del("x-api-key")
	headers.Del("x-goog-api-key")

	// 根据密钥格式设置对应的认证头
	if strings.HasPrefix(apiKey, "sk-ant-") {
		// Claude官方格式
		headers.Set("x-api-key", apiKey)
	} else {
		// 通用Bearer格式（适用于OpenAI等）
		headers.Set("Authorization", "Bearer "+apiKey)
	}
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
