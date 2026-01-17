package metrics

import (
	"math"
	"testing"
	"time"
)

// floatEquals 使用容差比较浮点数
func floatEquals(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestGetRecentActivityMultiURL_EmptyInputs(t *testing.T) {
	m := NewMetricsManager()
	defer m.Stop()

	// 空 baseURLs
	result := m.GetRecentActivityMultiURL(0, []string{}, []string{"key1"})
	if result.ChannelIndex != 0 {
		t.Errorf("expected channelIndex 0, got %d", result.ChannelIndex)
	}
	if len(result.Segments) != 150 {
		t.Errorf("expected 150 segments, got %d", len(result.Segments))
	}
	if result.RPM != 0 || result.TPM != 0 {
		t.Errorf("expected RPM=0, TPM=0 for empty input")
	}

	// 空 activeKeys
	result = m.GetRecentActivityMultiURL(0, []string{"http://example.com"}, []string{})
	if len(result.Segments) != 150 {
		t.Errorf("expected 150 segments, got %d", len(result.Segments))
	}
}

func TestGetRecentActivityMultiURL_SegmentBoundaries(t *testing.T) {
	m := NewMetricsManager()
	defer m.Stop()

	baseURL := "http://test.com"
	apiKey := "test-key"

	// 模拟在不同时间点的请求
	now := time.Now()
	m.mu.Lock()
	metrics := m.getOrCreateKey(baseURL, apiKey)

	// 添加当前 6 秒段的请求（应该在最后一个 segment）
	metrics.requestHistory = append(metrics.requestHistory, RequestRecord{
		Timestamp:    now,
		Success:      true,
		InputTokens:  100,
		OutputTokens: 50,
	})

	// 添加 5 分钟前的请求（5*60/6 = 50 段前）
	metrics.requestHistory = append(metrics.requestHistory, RequestRecord{
		Timestamp:    now.Add(-5 * time.Minute),
		Success:      true,
		InputTokens:  200,
		OutputTokens: 100,
	})

	// 添加 14 分钟前的请求（14*60/6 = 140 段前）
	metrics.requestHistory = append(metrics.requestHistory, RequestRecord{
		Timestamp:    now.Add(-14 * time.Minute),
		Success:      false,
		InputTokens:  50,
		OutputTokens: 25,
	})

	// 添加 16 分钟前的请求（应该被排除，超出 15 分钟窗口）
	metrics.requestHistory = append(metrics.requestHistory, RequestRecord{
		Timestamp:    now.Add(-16 * time.Minute),
		Success:      true,
		InputTokens:  1000,
		OutputTokens: 500,
	})
	m.mu.Unlock()

	result := m.GetRecentActivityMultiURL(1, []string{baseURL}, []string{apiKey})

	// 验证 channelIndex
	if result.ChannelIndex != 1 {
		t.Errorf("expected channelIndex 1, got %d", result.ChannelIndex)
	}

	// 验证 segment 数量（150 段，每段 6 秒）
	if len(result.Segments) != 150 {
		t.Errorf("expected 150 segments, got %d", len(result.Segments))
	}

	// 验证总请求数（应该是 3，排除 16 分钟前的）
	var totalRequests int64
	for _, seg := range result.Segments {
		totalRequests += seg.RequestCount
	}
	if totalRequests != 3 {
		t.Errorf("expected 3 total requests, got %d", totalRequests)
	}

	// 验证 RPM 计算（15 分钟平均）
	expectedRPM := 3.0 / 15.0
	if !floatEquals(result.RPM, expectedRPM, 0.0001) {
		t.Errorf("expected RPM %.4f, got %.4f", expectedRPM, result.RPM)
	}

	// 验证 TPM 只计算 OutputTokens（50 + 100 + 25 = 175）
	expectedTPM := 175.0 / 15.0
	if !floatEquals(result.TPM, expectedTPM, 0.0001) {
		t.Errorf("expected TPM %.4f, got %.4f", expectedTPM, result.TPM)
	}
}

func TestGetRecentActivityMultiURL_FailureCount(t *testing.T) {
	m := NewMetricsManager()
	defer m.Stop()

	baseURL := "http://test.com"
	apiKey := "test-key"

	now := time.Now()
	m.mu.Lock()
	metrics := m.getOrCreateKey(baseURL, apiKey)

	// 添加 2 个成功和 1 个失败
	metrics.requestHistory = append(metrics.requestHistory,
		RequestRecord{Timestamp: now, Success: true},
		RequestRecord{Timestamp: now, Success: true},
		RequestRecord{Timestamp: now, Success: false},
	)
	m.mu.Unlock()

	result := m.GetRecentActivityMultiURL(0, []string{baseURL}, []string{apiKey})

	// 找到有数据的 segment
	var foundSeg *ActivitySegment
	for i := range result.Segments {
		if result.Segments[i].RequestCount > 0 {
			foundSeg = &result.Segments[i]
			break
		}
	}

	if foundSeg == nil {
		t.Fatal("expected to find a segment with data")
	}

	if foundSeg.RequestCount != 3 {
		t.Errorf("expected 3 requests, got %d", foundSeg.RequestCount)
	}
	if foundSeg.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", foundSeg.SuccessCount)
	}
	if foundSeg.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", foundSeg.FailureCount)
	}
}

func TestGetRecentActivityMultiURL_MultipleURLs(t *testing.T) {
	m := NewMetricsManager()
	defer m.Stop()

	baseURL1 := "http://test1.com"
	baseURL2 := "http://test2.com"
	apiKey := "test-key"

	now := time.Now()
	m.mu.Lock()
	metrics1 := m.getOrCreateKey(baseURL1, apiKey)
	metrics1.requestHistory = append(metrics1.requestHistory, RequestRecord{
		Timestamp:    now,
		Success:      true,
		OutputTokens: 100,
	})

	metrics2 := m.getOrCreateKey(baseURL2, apiKey)
	metrics2.requestHistory = append(metrics2.requestHistory, RequestRecord{
		Timestamp:    now,
		Success:      true,
		OutputTokens: 200,
	})
	m.mu.Unlock()

	result := m.GetRecentActivityMultiURL(0, []string{baseURL1, baseURL2}, []string{apiKey})

	// 验证聚合了两个 URL 的数据
	var totalRequests int64
	for _, seg := range result.Segments {
		totalRequests += seg.RequestCount
	}
	if totalRequests != 2 {
		t.Errorf("expected 2 total requests from 2 URLs, got %d", totalRequests)
	}

	// TPM 应该是 (100 + 200) / 15
	expectedTPM := 300.0 / 15.0
	if !floatEquals(result.TPM, expectedTPM, 0.0001) {
		t.Errorf("expected TPM %.4f, got %.4f", expectedTPM, result.TPM)
	}
}

func TestGetRecentActivityMultiURL_MultipleKeys(t *testing.T) {
	m := NewMetricsManager()
	defer m.Stop()

	baseURL := "http://test.com"
	apiKey1 := "test-key-1"
	apiKey2 := "test-key-2"

	now := time.Now()
	m.mu.Lock()
	metrics1 := m.getOrCreateKey(baseURL, apiKey1)
	metrics1.requestHistory = append(metrics1.requestHistory, RequestRecord{
		Timestamp:    now,
		Success:      true,
		OutputTokens: 150,
	})

	metrics2 := m.getOrCreateKey(baseURL, apiKey2)
	metrics2.requestHistory = append(metrics2.requestHistory, RequestRecord{
		Timestamp:    now,
		Success:      true,
		OutputTokens: 250,
	})
	m.mu.Unlock()

	result := m.GetRecentActivityMultiURL(0, []string{baseURL}, []string{apiKey1, apiKey2})

	// 验证聚合了两个 Key 的数据
	var totalRequests int64
	for _, seg := range result.Segments {
		totalRequests += seg.RequestCount
	}
	if totalRequests != 2 {
		t.Errorf("expected 2 total requests from 2 Keys, got %d", totalRequests)
	}

	// TPM 应该是 (150 + 250) / 15
	expectedTPM := 400.0 / 15.0
	if !floatEquals(result.TPM, expectedTPM, 0.0001) {
		t.Errorf("expected TPM %.4f, got %.4f", expectedTPM, result.TPM)
	}
}

func TestGetRecentActivityMultiURL_MultipleURLsAndKeys(t *testing.T) {
	m := NewMetricsManager()
	defer m.Stop()

	baseURL1 := "http://test1.com"
	baseURL2 := "http://test2.com"
	apiKey1 := "test-key-1"
	apiKey2 := "test-key-2"

	now := time.Now()
	m.mu.Lock()
	// URL1 + Key1
	metrics11 := m.getOrCreateKey(baseURL1, apiKey1)
	metrics11.requestHistory = append(metrics11.requestHistory, RequestRecord{
		Timestamp:    now,
		Success:      true,
		OutputTokens: 100,
	})

	// URL1 + Key2
	metrics12 := m.getOrCreateKey(baseURL1, apiKey2)
	metrics12.requestHistory = append(metrics12.requestHistory, RequestRecord{
		Timestamp:    now,
		Success:      true,
		OutputTokens: 200,
	})

	// URL2 + Key1
	metrics21 := m.getOrCreateKey(baseURL2, apiKey1)
	metrics21.requestHistory = append(metrics21.requestHistory, RequestRecord{
		Timestamp:    now,
		Success:      false,
		OutputTokens: 150,
	})

	// URL2 + Key2
	metrics22 := m.getOrCreateKey(baseURL2, apiKey2)
	metrics22.requestHistory = append(metrics22.requestHistory, RequestRecord{
		Timestamp:    now,
		Success:      true,
		OutputTokens: 250,
	})
	m.mu.Unlock()

	result := m.GetRecentActivityMultiURL(0, []string{baseURL1, baseURL2}, []string{apiKey1, apiKey2})

	// 验证聚合了所有 URL × Key 组合的数据（2×2=4 个请求）
	var totalRequests int64
	var totalFailures int64
	for _, seg := range result.Segments {
		totalRequests += seg.RequestCount
		totalFailures += seg.FailureCount
	}
	if totalRequests != 4 {
		t.Errorf("expected 4 total requests from 2 URLs × 2 Keys, got %d", totalRequests)
	}
	if totalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", totalFailures)
	}

	// TPM 应该是 (100 + 200 + 150 + 250) / 15
	expectedTPM := 700.0 / 15.0
	if !floatEquals(result.TPM, expectedTPM, 0.0001) {
		t.Errorf("expected TPM %.4f, got %.4f", expectedTPM, result.TPM)
	}

	// RPM 应该是 4 / 15
	expectedRPM := 4.0 / 15.0
	if !floatEquals(result.RPM, expectedRPM, 0.0001) {
		t.Errorf("expected RPM %.4f, got %.4f", expectedRPM, result.RPM)
	}
}
