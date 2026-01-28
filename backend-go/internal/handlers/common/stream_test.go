package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/BenedictKing/claude-proxy/internal/utils"
)

func TestPatchUsageFieldsWithLog_NilInputTokens(t *testing.T) {
	tests := []struct {
		name           string
		usage          map[string]interface{}
		estimatedInput int
		hasCacheTokens bool
		wantPatched    bool
		wantValue      int
	}{
		{
			name:           "nil input_tokens without cache - should patch",
			usage:          map[string]interface{}{"input_tokens": nil, "output_tokens": float64(100)},
			estimatedInput: 10920,
			hasCacheTokens: false,
			wantPatched:    true,
			wantValue:      10920,
		},
		{
			name:           "nil input_tokens with cache - should also patch",
			usage:          map[string]interface{}{"input_tokens": nil, "output_tokens": float64(100)},
			estimatedInput: 10920,
			hasCacheTokens: true,
			wantPatched:    true,
			wantValue:      10920,
		},
		{
			name:           "valid input_tokens - should not patch",
			usage:          map[string]interface{}{"input_tokens": float64(5000), "output_tokens": float64(100)},
			estimatedInput: 10920,
			hasCacheTokens: true,
			wantPatched:    false,
			wantValue:      5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patchUsageFieldsWithLog(tt.usage, tt.estimatedInput, 100, tt.hasCacheTokens, false, "test", false)

			if tt.wantPatched {
				if v, ok := tt.usage["input_tokens"].(int); !ok || v != tt.wantValue {
					t.Errorf("expected input_tokens=%d, got %v", tt.wantValue, tt.usage["input_tokens"])
				}
			} else if tt.usage["input_tokens"] == nil {
				// nil case - expected to remain nil
			} else if v, ok := tt.usage["input_tokens"].(float64); ok && int(v) != tt.wantValue {
				t.Errorf("expected input_tokens=%d, got %v", tt.wantValue, tt.usage["input_tokens"])
			}
		})
	}
}

func TestPatchMessageStartInputTokensIfNeeded(t *testing.T) {
	requestBody := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello world hello world hello world"}]}]}`)
	estimated := utils.EstimateRequestTokens(requestBody)
	if estimated <= 0 {
		t.Fatalf("expected estimated input tokens > 0, got %d", estimated)
	}

	extractInputTokens := func(t *testing.T, event string) float64 {
		t.Helper()
		for _, line := range strings.Split(event, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data); err != nil {
				t.Fatalf("failed to unmarshal data: %v", err)
			}
			msg, ok := data["message"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing message field")
			}
			usage, ok := msg["usage"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing message.usage field")
			}
			v, ok := usage["input_tokens"].(float64)
			if !ok {
				t.Fatalf("missing input_tokens field")
			}
			return v
		}
		t.Fatalf("no data line found")
		return 0
	}

	t.Run("input_tokens=0 should patch in message_start", func(t *testing.T) {
		event := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n"
		hasUsage, needInputPatch, _, usageData := CheckEventUsageStatus(event, false)
		if !hasUsage {
			t.Fatalf("expected hasUsage=true")
		}
		if !needInputPatch {
			t.Fatalf("expected needInputPatch=true")
		}

		patched := PatchMessageStartInputTokensIfNeeded(event, requestBody, needInputPatch, usageData, false, false)
		got := extractInputTokens(t, patched)
		if got != float64(estimated) {
			t.Fatalf("expected input_tokens=%d, got %v", estimated, got)
		}
	})

	t.Run("input_tokens<10 should patch in message_start", func(t *testing.T) {
		event := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n"
		hasUsage, needInputPatch, _, usageData := CheckEventUsageStatus(event, false)
		if !hasUsage {
			t.Fatalf("expected hasUsage=true")
		}
		if needInputPatch {
			t.Fatalf("expected needInputPatch=false")
		}

		patched := PatchMessageStartInputTokensIfNeeded(event, requestBody, needInputPatch, usageData, false, false)
		got := extractInputTokens(t, patched)
		if got != float64(estimated) {
			t.Fatalf("expected input_tokens=%d, got %v", estimated, got)
		}
	})

	t.Run("cache hit should not patch input_tokens", func(t *testing.T) {
		event := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":0,\"cache_read_input_tokens\":100}}}\n\n"
		hasUsage, needInputPatch, _, usageData := CheckEventUsageStatus(event, false)
		if !hasUsage {
			t.Fatalf("expected hasUsage=true")
		}
		if needInputPatch {
			t.Fatalf("expected needInputPatch=false")
		}

		patched := PatchMessageStartInputTokensIfNeeded(event, requestBody, needInputPatch, usageData, false, false)
		got := extractInputTokens(t, patched)
		if got != 0 {
			t.Fatalf("expected input_tokens=0, got %v", got)
		}
	})

	t.Run("valid input_tokens should not patch", func(t *testing.T) {
		event := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":50,\"output_tokens\":0}}}\n\n"
		hasUsage, needInputPatch, _, usageData := CheckEventUsageStatus(event, false)
		if !hasUsage {
			t.Fatalf("expected hasUsage=true")
		}
		if needInputPatch {
			t.Fatalf("expected needInputPatch=false")
		}

		patched := PatchMessageStartInputTokensIfNeeded(event, requestBody, needInputPatch, usageData, false, false)
		got := extractInputTokens(t, patched)
		if got != 50 {
			t.Fatalf("expected input_tokens=50, got %v", got)
		}
	})
}
