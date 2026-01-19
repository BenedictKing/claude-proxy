package gemini

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/gin-gonic/gin"
)

func TestHandler_RequiresProxyAccessKeyEvenWhenGeminiKeyProvided(t *testing.T) {
	gin.SetMode(gin.TestMode)

	envCfg := &config.EnvConfig{
		ProxyAccessKey:     "secret-key",
		MaxRequestBodySize: 1024 * 1024,
	}

	r := gin.New()
	r.POST("/v1beta/models/*modelAction", Handler(envCfg, nil, nil))

	t.Run("x-goog-api-key does not bypass proxy auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.0-flash:generateContent", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", "any-gemini-key")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("query key does not bypass proxy auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.0-flash:generateContent?key=any-gemini-key", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

// TestStripThoughtSignatures 测试 stripThoughtSignatures 函数
func TestStripThoughtSignatures(t *testing.T) {
	tests := []struct {
		name     string
		input    *types.GeminiRequest
		expected *types.GeminiRequest
	}{
		{
			name: "移除单个 functionCall 的 thought_signature",
			input: &types.GeminiRequest{
				Contents: []types.GeminiContent{
					{
						Parts: []types.GeminiPart{
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "test_function",
									Args:             map[string]interface{}{"arg1": "value1"},
									ThoughtSignature: "test_signature",
								},
							},
						},
					},
				},
			},
			expected: &types.GeminiRequest{
				Contents: []types.GeminiContent{
					{
						Parts: []types.GeminiPart{
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "test_function",
									Args:             map[string]interface{}{"arg1": "value1"},
									ThoughtSignature: "",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "移除多个 functionCall 的 thought_signature",
			input: &types.GeminiRequest{
				Contents: []types.GeminiContent{
					{
						Parts: []types.GeminiPart{
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "func1",
									Args:             map[string]interface{}{},
									ThoughtSignature: "sig1",
								},
							},
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "func2",
									Args:             map[string]interface{}{},
									ThoughtSignature: "sig2",
								},
							},
						},
					},
				},
			},
			expected: &types.GeminiRequest{
				Contents: []types.GeminiContent{
					{
						Parts: []types.GeminiPart{
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "func1",
									Args:             map[string]interface{}{},
									ThoughtSignature: "",
								},
							},
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "func2",
									Args:             map[string]interface{}{},
									ThoughtSignature: "",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "不影响非 functionCall 的 parts",
			input: &types.GeminiRequest{
				Contents: []types.GeminiContent{
					{
						Parts: []types.GeminiPart{
							{
								Text: "some text",
							},
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "func",
									Args:             map[string]interface{}{},
									ThoughtSignature: "sig",
								},
							},
						},
					},
				},
			},
			expected: &types.GeminiRequest{
				Contents: []types.GeminiContent{
					{
						Parts: []types.GeminiPart{
							{
								Text: "some text",
							},
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "func",
									Args:             map[string]interface{}{},
									ThoughtSignature: "",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "处理空 thought_signature",
			input: &types.GeminiRequest{
				Contents: []types.GeminiContent{
					{
						Parts: []types.GeminiPart{
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "func",
									Args:             map[string]interface{}{},
									ThoughtSignature: "",
								},
							},
						},
					},
				},
			},
			expected: &types.GeminiRequest{
				Contents: []types.GeminiContent{
					{
						Parts: []types.GeminiPart{
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "func",
									Args:             map[string]interface{}{},
									ThoughtSignature: "",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripThoughtSignatures(tt.input)

			// 验证结果
			if len(tt.input.Contents) != len(tt.expected.Contents) {
				t.Fatalf("Contents length mismatch: got %d, want %d", len(tt.input.Contents), len(tt.expected.Contents))
			}

			for i := range tt.input.Contents {
				if len(tt.input.Contents[i].Parts) != len(tt.expected.Contents[i].Parts) {
					t.Fatalf("Parts length mismatch at content %d: got %d, want %d", i, len(tt.input.Contents[i].Parts), len(tt.expected.Contents[i].Parts))
				}

				for j := range tt.input.Contents[i].Parts {
					inputPart := &tt.input.Contents[i].Parts[j]
					expectedPart := &tt.expected.Contents[i].Parts[j]

					if inputPart.FunctionCall != nil {
						if expectedPart.FunctionCall == nil {
							t.Fatalf("FunctionCall mismatch at content %d, part %d: got non-nil, want nil", i, j)
						}
						if inputPart.FunctionCall.ThoughtSignature != expectedPart.FunctionCall.ThoughtSignature {
							t.Errorf("ThoughtSignature mismatch at content %d, part %d: got %q, want %q",
								i, j, inputPart.FunctionCall.ThoughtSignature, expectedPart.FunctionCall.ThoughtSignature)
						}
					}
				}
			}
		})
	}
}

// TestBuildProviderRequest_StripThoughtSignature 测试 buildProviderRequest 中的 StripThoughtSignature 配置
func TestBuildProviderRequest_StripThoughtSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name                     string
		stripThoughtSignature    bool
		injectDummyThoughtSig    bool
		inputThoughtSignature    string
		expectedThoughtSignature string
	}{
		{
			name:                     "StripThoughtSignature=true 移除字段",
			stripThoughtSignature:    true,
			injectDummyThoughtSig:    false,
			inputThoughtSignature:    "test_signature",
			expectedThoughtSignature: "",
		},
		{
			name:                     "StripThoughtSignature=false 保留字段",
			stripThoughtSignature:    false,
			injectDummyThoughtSig:    false,
			inputThoughtSignature:    "test_signature",
			expectedThoughtSignature: "test_signature",
		},
		{
			name:                     "StripThoughtSignature=true 优先于 InjectDummyThoughtSignature",
			stripThoughtSignature:    true,
			injectDummyThoughtSig:    true,
			inputThoughtSignature:    "test_signature",
			expectedThoughtSignature: "",
		},
		{
			name:                     "InjectDummyThoughtSignature=true 注入 dummy 值",
			stripThoughtSignature:    false,
			injectDummyThoughtSig:    true,
			inputThoughtSignature:    "",
			expectedThoughtSignature: types.DummyThoughtSignature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &config.UpstreamConfig{
				BaseURL:                     "https://test.example.com",
				ServiceType:                 "gemini",
				StripThoughtSignature:       tt.stripThoughtSignature,
				InjectDummyThoughtSignature: tt.injectDummyThoughtSig,
			}

			geminiReq := &types.GeminiRequest{
				Contents: []types.GeminiContent{
					{
						Parts: []types.GeminiPart{
							{
								FunctionCall: &types.GeminiFunctionCall{
									Name:             "test_function",
									Args:             map[string]interface{}{"arg1": "value1"},
									ThoughtSignature: tt.inputThoughtSignature,
								},
							},
						},
					},
				},
			}

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)

			req, err := buildProviderRequest(c, upstream, upstream.BaseURL, "test-key", geminiReq, "gemini-2.0-flash", false)
			if err != nil {
				t.Fatalf("buildProviderRequest failed: %v", err)
			}

			// 解析请求体
			var resultReq types.GeminiRequest
			if err := json.NewDecoder(req.Body).Decode(&resultReq); err != nil {
				t.Fatalf("Failed to decode request body: %v", err)
			}

			// 验证 thought_signature
			if len(resultReq.Contents) == 0 || len(resultReq.Contents[0].Parts) == 0 {
				t.Fatal("Request body is empty")
			}

			part := resultReq.Contents[0].Parts[0]
			if part.FunctionCall == nil {
				t.Fatal("FunctionCall is nil")
			}

			if part.FunctionCall.ThoughtSignature != tt.expectedThoughtSignature {
				t.Errorf("ThoughtSignature mismatch: got %q, want %q",
					part.FunctionCall.ThoughtSignature, tt.expectedThoughtSignature)
			}

			// 验证原始请求未被修改（深拷贝机制）
			if geminiReq.Contents[0].Parts[0].FunctionCall.ThoughtSignature != tt.inputThoughtSignature {
				t.Errorf("Original request was modified: got %q, want %q",
					geminiReq.Contents[0].Parts[0].FunctionCall.ThoughtSignature, tt.inputThoughtSignature)
			}
		})
	}
}
