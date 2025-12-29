package common

import (
	"encoding/json"
	"testing"
)

// TestClassifyByStatusCode 测试基于状态码的分类
func TestClassifyByStatusCode(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		wantFailover bool
		wantQuota    bool
	}{
		// 认证/授权错误
		{"401 Unauthorized", 401, true, false},
		{"403 Forbidden", 403, true, false},

		// 配额/计费错误
		{"402 Payment Required", 402, true, true},
		{"429 Too Many Requests", 429, true, true},

		// 超时错误
		{"408 Request Timeout", 408, true, false},

		// 服务端错误
		{"500 Internal Server Error", 500, true, false},
		{"502 Bad Gateway", 502, true, false},
		{"503 Service Unavailable", 503, true, false},
		{"504 Gateway Timeout", 504, true, false},

		// 不应 failover 的客户端错误
		{"400 Bad Request", 400, false, false},
		{"404 Not Found", 404, false, false},
		{"405 Method Not Allowed", 405, false, false},
		{"413 Payload Too Large", 413, false, false},
		{"422 Unprocessable Entity", 422, false, false},

		// 成功状态码
		{"200 OK", 200, false, false},
		{"201 Created", 201, false, false},
		{"204 No Content", 204, false, false},

		// 重定向
		{"301 Moved Permanently", 301, false, false},
		{"302 Found", 302, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFailover, gotQuota := classifyByStatusCode(tt.statusCode)
			if gotFailover != tt.wantFailover {
				t.Errorf("classifyByStatusCode(%d) failover = %v, want %v", tt.statusCode, gotFailover, tt.wantFailover)
			}
			if gotQuota != tt.wantQuota {
				t.Errorf("classifyByStatusCode(%d) quota = %v, want %v", tt.statusCode, gotQuota, tt.wantQuota)
			}
		})
	}
}

// TestClassifyMessage 测试基于错误消息的分类
func TestClassifyMessage(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		wantFailover bool
		wantQuota    bool
	}{
		// 配额相关
		{"insufficient credits", "You have insufficient credits", true, true},
		{"quota exceeded", "API quota exceeded for this month", true, true},
		{"rate limit", "Rate limit exceeded, please retry later", true, true},
		{"balance", "Account balance is zero", true, true},
		{"billing", "Billing issue detected", true, true},
		{"中文-积分不足", "您的积分不足，请充值", true, true},
		{"中文-余额不足", "账户余额不足", true, true},
		{"中文-请求数限制", "已达到请求数限制", true, true},

		// 认证相关
		{"invalid api key", "Invalid API key provided", true, false},
		{"unauthorized", "Unauthorized access", true, false},
		{"token expired", "Your token has expired", true, false},
		{"permission denied", "Permission denied for this resource", true, false},
		{"中文-密钥无效", "密钥无效，请检查", true, false},

		// 临时错误
		{"timeout", "Request timeout, please retry", true, false},
		{"server overloaded", "Server is overloaded", true, false},
		{"temporarily unavailable", "Service temporarily unavailable", true, false},
		{"中文-超时", "请求超时", true, false},

		// 不应 failover
		{"normal error", "Something went wrong", false, false},
		{"validation error", "Field 'name' is required", false, false},
		{"empty message", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFailover, gotQuota := classifyMessage(tt.message)
			if gotFailover != tt.wantFailover {
				t.Errorf("classifyMessage(%q) failover = %v, want %v", tt.message, gotFailover, tt.wantFailover)
			}
			if gotQuota != tt.wantQuota {
				t.Errorf("classifyMessage(%q) quota = %v, want %v", tt.message, gotQuota, tt.wantQuota)
			}
		})
	}
}

// TestClassifyErrorType 测试基于错误类型的分类
func TestClassifyErrorType(t *testing.T) {
	tests := []struct {
		name         string
		errType      string
		wantFailover bool
		wantQuota    bool
	}{
		// 配额相关
		{"over_quota", "over_quota", true, true},
		{"quota_exceeded", "quota_exceeded", true, true},
		{"rate_limit_exceeded", "rate_limit_exceeded", true, true},
		{"billing_error", "billing_error", true, true},
		{"insufficient_funds", "insufficient_funds", true, true},

		// 认证相关
		{"authentication_error", "authentication_error", true, false},
		{"invalid_api_key", "invalid_api_key", true, false},
		{"permission_denied", "permission_denied", true, false},

		// 服务端错误
		{"server_error", "server_error", true, false},
		{"internal_error", "internal_error", true, false},
		{"service_unavailable", "service_unavailable", true, false},

		// 不应 failover
		{"invalid_request", "invalid_request", false, false},
		{"validation_error", "validation_error", false, false},
		{"unknown_error", "unknown_error", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFailover, gotQuota := classifyErrorType(tt.errType)
			if gotFailover != tt.wantFailover {
				t.Errorf("classifyErrorType(%q) failover = %v, want %v", tt.errType, gotFailover, tt.wantFailover)
			}
			if gotQuota != tt.wantQuota {
				t.Errorf("classifyErrorType(%q) quota = %v, want %v", tt.errType, gotQuota, tt.wantQuota)
			}
		})
	}
}

// TestClassifyByErrorMessage 测试基于响应体的分类
func TestClassifyByErrorMessage(t *testing.T) {
	tests := []struct {
		name         string
		body         map[string]interface{}
		wantFailover bool
		wantQuota    bool
	}{
		{
			name: "quota error in message",
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "You have exceeded your quota",
					"type":    "error",
				},
			},
			wantFailover: true,
			wantQuota:    true,
		},
		{
			name: "auth error in message",
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Invalid API key",
					"type":    "error",
				},
			},
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name: "quota error in type",
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Error occurred",
					"type":    "over_quota",
				},
			},
			wantFailover: true,
			wantQuota:    true,
		},
		{
			name: "server error in type",
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Error occurred",
					"type":    "server_error",
				},
			},
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name: "no failover keywords",
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Bad request format",
					"type":    "invalid_request",
				},
			},
			wantFailover: false,
			wantQuota:    false,
		},
		{
			name:         "empty body",
			body:         map[string]interface{}{},
			wantFailover: false,
			wantQuota:    false,
		},
		{
			name: "no error field",
			body: map[string]interface{}{
				"status": "error",
			},
			wantFailover: false,
			wantQuota:    false,
		},
		// upstream_error 字段支持（Responses API 错误格式）
		{
			name: "upstream_error string field - auth error",
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"type":           "upstream_error",
					"upstream_error": "Invalid API key provided",
				},
			},
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name: "upstream_error string field - quota error",
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"type":           "upstream_error",
					"upstream_error": "Rate limit exceeded, please retry later",
				},
			},
			wantFailover: true,
			wantQuota:    true,
		},
		{
			name: "upstream_error nested object with message",
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"type": "upstream_error",
					"upstream_error": map[string]interface{}{
						"message": "Insufficient credits",
					},
				},
			},
			wantFailover: true,
			wantQuota:    true,
		},
		{
			name: "detail field - auth error",
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"type":   "error",
					"detail": "Token expired, please refresh",
				},
			},
			wantFailover: true,
			wantQuota:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			gotFailover, gotQuota := classifyByErrorMessage(bodyBytes)
			if gotFailover != tt.wantFailover {
				t.Errorf("classifyByErrorMessage() failover = %v, want %v", gotFailover, tt.wantFailover)
			}
			if gotQuota != tt.wantQuota {
				t.Errorf("classifyByErrorMessage() quota = %v, want %v", gotQuota, tt.wantQuota)
			}
		})
	}
}

// TestClassifyByErrorMessage_InvalidJSON 测试无效 JSON 的处理
func TestClassifyByErrorMessage_InvalidJSON(t *testing.T) {
	invalidBodies := [][]byte{
		[]byte("not json"),
		[]byte("{invalid}"),
		[]byte(""),
		nil,
	}

	for _, body := range invalidBodies {
		gotFailover, gotQuota := classifyByErrorMessage(body)
		if gotFailover || gotQuota {
			t.Errorf("classifyByErrorMessage(%q) should return (false, false) for invalid JSON", string(body))
		}
	}
}

// TestShouldRetryWithNextKey_403WithPredeductQuotaError 测试 403 + 预扣费额度失败的场景
// 这是生产环境实际发生的错误格式
func TestShouldRetryWithNextKey_403WithPredeductQuotaError(t *testing.T) {
	// 使用生产环境的精确 JSON 格式
	body := []byte(`{"error":{"type":"new_api_error","message":"预扣费额度失败, 用户剩余额度: ¥0.053950, 需要预扣费额度: ¥0.191160, 下次重置时间: 2025-01-01 00:00:00"},"type":"error"}`)

	gotFailover, gotQuota := ShouldRetryWithNextKey(403, body, false)

	if !gotFailover {
		t.Errorf("ShouldRetryWithNextKey(403, prededuct_error, false) failover = %v, want true", gotFailover)
	}
	if !gotQuota {
		t.Errorf("ShouldRetryWithNextKey(403, prededuct_error, false) quota = %v, want true", gotQuota)
	}
}

// TestClassifyMessage_ChineseQuotaKeywords 测试中文额度关键词
func TestClassifyMessage_ChineseQuotaKeywords(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		wantFailover bool
		wantQuota    bool
	}{
		{"预扣费额度失败", "预扣费额度失败, 用户剩余额度: ¥0.053950", true, true},
		{"额度不足", "账户额度不足", true, true},
		{"预扣费失败", "预扣费失败，请充值", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFailover, gotQuota := classifyMessage(tt.message)
			if gotFailover != tt.wantFailover {
				t.Errorf("classifyMessage(%q) failover = %v, want %v", tt.message, gotFailover, tt.wantFailover)
			}
			if gotQuota != tt.wantQuota {
				t.Errorf("classifyMessage(%q) quota = %v, want %v", tt.message, gotQuota, tt.wantQuota)
			}
		})
	}
}

// TestShouldRetryWithNextKey 测试完整的重试判断逻辑
func TestShouldRetryWithNextKey(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		body         map[string]interface{}
		wantFailover bool
		wantQuota    bool
	}{
		// 403 + 中文配额相关消息
		{
			name:       "403 with chinese quota message",
			statusCode: 403,
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"type":    "new_api_error",
					"message": "预扣费额度失败, 用户剩余额度: ¥0.053950",
				},
				"type": "error",
			},
			wantFailover: true,
			wantQuota:    true,
		},
		// 状态码优先
		{
			name:         "401 always failover",
			statusCode:   401,
			body:         map[string]interface{}{},
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "402 always failover with quota",
			statusCode:   402,
			body:         map[string]interface{}{},
			wantFailover: true,
			wantQuota:    true,
		},
		{
			name:         "408 always failover",
			statusCode:   408,
			body:         map[string]interface{}{},
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "500 always failover",
			statusCode:   500,
			body:         map[string]interface{}{},
			wantFailover: true,
			wantQuota:    false,
		},
		// 400 需要检查消息体
		{
			name:       "400 with quota message",
			statusCode: 400,
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Quota exceeded",
				},
			},
			wantFailover: true,
			wantQuota:    true,
		},
		{
			name:       "400 with auth message",
			statusCode: 400,
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Invalid API key",
				},
			},
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:       "400 without failover keywords",
			statusCode: 400,
			body: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Bad request",
				},
			},
			wantFailover: false,
			wantQuota:    false,
		},
		// 404 不应 failover
		{
			name:         "404 never failover",
			statusCode:   404,
			body:         map[string]interface{}{},
			wantFailover: false,
			wantQuota:    false,
		},
		// 200 不应 failover
		{
			name:         "200 never failover",
			statusCode:   200,
			body:         map[string]interface{}{},
			wantFailover: false,
			wantQuota:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			// 测试非 Fuzzy 模式（精确错误分类）
			gotFailover, gotQuota := ShouldRetryWithNextKey(tt.statusCode, bodyBytes, false)
			if gotFailover != tt.wantFailover {
				t.Errorf("shouldRetryWithNextKey(%d, ..., false) failover = %v, want %v", tt.statusCode, gotFailover, tt.wantFailover)
			}
			if gotQuota != tt.wantQuota {
				t.Errorf("shouldRetryWithNextKey(%d, ..., false) quota = %v, want %v", tt.statusCode, gotQuota, tt.wantQuota)
			}
		})
	}
}

// TestShouldRetryWithNextKeyFuzzyMode 测试 Fuzzy 模式下的错误分类
// Fuzzy 模式：所有非 2xx 错误都触发 failover
func TestShouldRetryWithNextKeyFuzzyMode(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		wantFailover bool
		wantQuota    bool
	}{
		// 2xx 成功响应不 failover
		{
			name:         "200 OK - no failover",
			statusCode:   200,
			wantFailover: false,
			wantQuota:    false,
		},
		{
			name:         "201 Created - no failover",
			statusCode:   201,
			wantFailover: false,
			wantQuota:    false,
		},
		// 3xx 重定向在 Fuzzy 模式下触发 failover
		{
			name:         "301 Redirect - failover in fuzzy mode",
			statusCode:   301,
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "302 Found - failover in fuzzy mode",
			statusCode:   302,
			wantFailover: true,
			wantQuota:    false,
		},
		// 4xx 客户端错误在 Fuzzy 模式下都触发 failover
		{
			name:         "400 Bad Request - failover in fuzzy mode",
			statusCode:   400,
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "401 Unauthorized - failover in fuzzy mode",
			statusCode:   401,
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "402 Payment Required - failover with quota",
			statusCode:   402,
			wantFailover: true,
			wantQuota:    true, // 配额相关
		},
		{
			name:         "403 Forbidden - failover in fuzzy mode",
			statusCode:   403,
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "404 Not Found - failover in fuzzy mode",
			statusCode:   404,
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "422 Unprocessable Entity - failover in fuzzy mode",
			statusCode:   422,
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "429 Too Many Requests - failover with quota",
			statusCode:   429,
			wantFailover: true,
			wantQuota:    true, // 配额相关
		},
		// 5xx 服务端错误在 Fuzzy 模式下触发 failover
		{
			name:         "500 Internal Server Error - failover in fuzzy mode",
			statusCode:   500,
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "502 Bad Gateway - failover in fuzzy mode",
			statusCode:   502,
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "503 Service Unavailable - failover in fuzzy mode",
			statusCode:   503,
			wantFailover: true,
			wantQuota:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 测试 Fuzzy 模式（所有非 2xx 都 failover）
			gotFailover, gotQuota := ShouldRetryWithNextKey(tt.statusCode, nil, true)
			if gotFailover != tt.wantFailover {
				t.Errorf("shouldRetryWithNextKey(%d, nil, true) failover = %v, want %v", tt.statusCode, gotFailover, tt.wantFailover)
			}
			if gotQuota != tt.wantQuota {
				t.Errorf("shouldRetryWithNextKey(%d, nil, true) quota = %v, want %v", tt.statusCode, gotQuota, tt.wantQuota)
			}
		})
	}
}

// TestShouldRetryWithNextKey_FuzzyMode_403WithQuotaMessage 测试 Fuzzy 模式下 403 + 预扣费消息
// 验证修复：Fuzzy 模式下也会检查消息体中的配额相关关键词
func TestShouldRetryWithNextKey_FuzzyMode_403WithQuotaMessage(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		body         []byte
		wantFailover bool
		wantQuota    bool
	}{
		{
			name:         "403 with prededuct quota error in fuzzy mode",
			statusCode:   403,
			body:         []byte(`{"error":{"type":"new_api_error","message":"预扣费额度失败, 用户剩余额度: ¥0.053950, 需要预扣费额度: ¥0.191160"},"type":"error"}`),
			wantFailover: true,
			wantQuota:    true,
		},
		{
			name:         "403 with insufficient balance in fuzzy mode",
			statusCode:   403,
			body:         []byte(`{"error":{"message":"余额不足，请充值"}}`),
			wantFailover: true,
			wantQuota:    true,
		},
		{
			name:         "403 without quota keywords in fuzzy mode",
			statusCode:   403,
			body:         []byte(`{"error":{"message":"Access denied"}}`),
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "403 with empty body in fuzzy mode",
			statusCode:   403,
			body:         nil,
			wantFailover: true,
			wantQuota:    false,
		},
		{
			name:         "500 with quota message in fuzzy mode",
			statusCode:   500,
			body:         []byte(`{"error":{"message":"Quota exceeded"}}`),
			wantFailover: true,
			wantQuota:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFailover, gotQuota := ShouldRetryWithNextKey(tt.statusCode, tt.body, true)
			if gotFailover != tt.wantFailover {
				t.Errorf("ShouldRetryWithNextKey(%d, body, true) failover = %v, want %v", tt.statusCode, gotFailover, tt.wantFailover)
			}
			if gotQuota != tt.wantQuota {
				t.Errorf("ShouldRetryWithNextKey(%d, body, true) quota = %v, want %v", tt.statusCode, gotQuota, tt.wantQuota)
			}
		})
	}
}
