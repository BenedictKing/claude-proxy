package providers

import (
	"testing"

	"github.com/BenedictKing/claude-proxy/internal/config"
)

func TestBuildTargetURL_SkipVersionWithHash(t *testing.T) {
	p := &ResponsesProvider{}

	tests := []struct {
		name        string
		baseURL     string
		serviceType string
		want        string
	}{
		// 正常情况：自动添加 /v1
		{"normal_responses", "https://api.example.com", "responses", "https://api.example.com/v1/responses"},
		{"normal_claude", "https://api.example.com", "claude", "https://api.example.com/v1/messages"},
		{"normal_openai", "https://api.example.com", "openai", "https://api.example.com/v1/chat/completions"},

		// 已有版本号：不添加 /v1
		{"with_version", "https://api.example.com/v1", "responses", "https://api.example.com/v1/responses"},
		{"with_v2", "https://api.example.com/v2", "openai", "https://api.example.com/v2/chat/completions"},

		// # 结尾：跳过 /v1
		{"hash_skip", "https://api.example.com#", "responses", "https://api.example.com/responses"},
		{"hash_skip_claude", "https://api.example.com#", "claude", "https://api.example.com/messages"},
		{"hash_skip_openai", "https://api.example.com#", "openai", "https://api.example.com/chat/completions"},

		// # 结尾 + 末尾斜杠：正确处理
		{"hash_with_slash", "https://api.example.com/#", "responses", "https://api.example.com/responses"},
		{"hash_with_slash_openai", "https://api.example.com/#", "openai", "https://api.example.com/chat/completions"},

		// 末尾斜杠：正确移除
		{"trailing_slash", "https://api.example.com/", "responses", "https://api.example.com/v1/responses"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &config.UpstreamConfig{
				BaseURL:     tt.baseURL,
				ServiceType: tt.serviceType,
			}
			got := p.buildTargetURL(upstream)
			if got != tt.want {
				t.Errorf("buildTargetURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
