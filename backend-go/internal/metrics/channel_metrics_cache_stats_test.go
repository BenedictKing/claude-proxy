package metrics

import (
	"math"
	"testing"

	"github.com/BenedictKing/claude-proxy/internal/types"
)

func TestToResponse_TimeWindowsIncludesCacheStats(t *testing.T) {
	m := NewMetricsManagerWithConfig(10, 0.5)

	baseURL := "https://example.com"
	key1 := "k1"
	key2 := "k2"

	m.RecordSuccessWithUsage(baseURL, key1, &types.Usage{
		InputTokens:              100,
		OutputTokens:             10,
		CacheCreationInputTokens: 20,
		CacheReadInputTokens:     50,
	})
	m.RecordSuccessWithUsage(baseURL, key2, &types.Usage{
		InputTokens:  200,
		OutputTokens: 20,
	})

	resp := m.ToResponse(0, baseURL, []string{key1, key2}, 0)
	stats, ok := resp.TimeWindows["15m"]
	if !ok {
		t.Fatalf("expected timeWindows[15m] to exist")
	}

	if stats.InputTokens != 300 {
		t.Fatalf("expected inputTokens=300, got %d", stats.InputTokens)
	}
	if stats.OutputTokens != 30 {
		t.Fatalf("expected outputTokens=30, got %d", stats.OutputTokens)
	}
	if stats.CacheCreationTokens != 20 {
		t.Fatalf("expected cacheCreationTokens=20, got %d", stats.CacheCreationTokens)
	}
	if stats.CacheReadTokens != 50 {
		t.Fatalf("expected cacheReadTokens=50, got %d", stats.CacheReadTokens)
	}

	wantHitRate := float64(50) / float64(50+300) * 100
	if math.Abs(stats.CacheHitRate-wantHitRate) > 0.01 {
		t.Fatalf("expected cacheHitRate=%.4f, got %.4f", wantHitRate, stats.CacheHitRate)
	}
}
