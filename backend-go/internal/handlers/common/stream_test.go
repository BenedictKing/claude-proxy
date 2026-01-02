package common

import (
	"testing"
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
			patchUsageFieldsWithLog(tt.usage, tt.estimatedInput, 100, tt.hasCacheTokens, false, "test")

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
