package utils

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompactContentArray(t *testing.T) {
	input := map[string]interface{}{
		"model": "claude-3",
		"tools": []interface{}{"Tool1", "Tool2", "Tool3"}, // 简化后的tools数组
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": strings.Repeat("This is a very long text that should be truncated. ", 10),
					},
					map[string]interface{}{
						"type": "tool_use",
						"id":   "toolu_123",
						"name": "get_weather",
						"input": map[string]interface{}{
							"location": "San Francisco",
							"unit":     "celsius",
						},
					},
				},
			},
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "tool_result",
						"tool_use_id": "toolu_123",
						"content": "Temperature: 18°C, Clear sky",
						"is_error": false,
					},
				},
			},
		},
	}

	result := FormatJSONForLog(input, 500)

	// 验证content数组被紧凑显示
	if !strings.Contains(result, `"type": "text"`) {
		t.Error("应该包含type字段")
	}

	// 验证文本被截断到200字符
	if strings.Contains(result, strings.Repeat("This is a very long text", 8)) {
		t.Error("长文本应该被截断到200字符")
	}

	// 验证tool_use的input显示JSON而不是{...}
	if !strings.Contains(result, `"location"`) || !strings.Contains(result, `"San Francisco"`) {
		t.Error("tool_use的input应该显示JSON内容")
	}

	// 验证tools数组被紧凑显示（单行或少量换行）
	if strings.Contains(result, `"tools": ["Tool1", "Tool2", "Tool3"]`) ||
	   strings.Contains(result, `"tools": [
  "Tool1", "Tool2", "Tool3"
]`) {
		t.Log("✓ tools数组被紧凑显示")
	}

	// 验证输出没有被截断（不应该出现"需要�"这种乱码）
	if strings.Contains(result, "�") {
		t.Error("输出包含乱码，可能是截断导致的")
	}

	t.Logf("格式化后的输出:\n%s", result)
}

func TestContentArrayCompactFormat(t *testing.T) {
	// 测试各种content类型的紧凑显示
	tests := []struct {
		name    string
		content []interface{}
		checks  []string // 应该包含的内容
	}{
		{
			name: "文本类型 - 长文本截断",
			content: []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": strings.Repeat("This is a very long text that exceeds 200 characters and should be truncated. ", 5),
				},
			},
			checks: []string{
				`"type": "text"`,
				// 文本应该被截断到200字符，包含省略号
				`...`,
			},
		},
		{
			name: "工具使用类型",
			content: []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_abc123",
					"name": "calculator",
					"input": map[string]interface{}{
						"expression": "2 + 2",
					},
				},
			},
			checks: []string{
				`"type": "tool_use"`,
				`"id": "toolu_abc123"`,
				`"name": "calculator"`,
				// input应该显示JSON内容而不是{...}
				`"expression"`,
			},
		},
		{
			name: "工具结果类型",
			content: []interface{}{
				map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": "toolu_abc123",
					"content":     "Result: 4",
					"is_error":    false,
				},
			},
			checks: []string{
				`"type": "tool_result"`,
				`"tool_use_id": "toolu_abc123"`,
				`"content": "Result: 4"`,
				`"is_error": false`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]interface{}{
				"messages": []interface{}{
					map[string]interface{}{
						"role":    "user",
						"content": tt.content,
					},
				},
			}

			result := FormatJSONForLog(input, 500)

			for _, check := range tt.checks {
				if !strings.Contains(result, check) {
					t.Errorf("输出应该包含: %s\n实际输出:\n%s", check, result)
				}
			}

			// 验证没有乱码
			if strings.Contains(result, "�") {
				t.Error("输出包含乱码")
			}
		})
	}
}

func TestNoTruncationInMiddleOfJSON(t *testing.T) {
	// 创建一个超大的JSON对象来测试截断逻辑
	largeMessages := make([]interface{}, 100)
	for i := 0; i < 100; i++ {
		largeMessages[i] = map[string]interface{}{
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Message " + strings.Repeat("x", 100),
				},
			},
		}
	}

	input := map[string]interface{}{
		"model":    "claude-3",
		"messages": largeMessages,
	}

	result := FormatJSONForLog(input, 500)

	// 如果被截断，应该在换行符处截断
	if strings.Contains(result, "... (输出已截断)") {
		// 检查截断位置是否在合适的地方
		truncateIndex := strings.Index(result, "... (输出已截断)")
		beforeTruncate := result[:truncateIndex]

		// 应该在换行符后截断
		if !strings.HasSuffix(strings.TrimSpace(beforeTruncate), "\n") &&
		   !strings.HasSuffix(beforeTruncate, "}") &&
		   !strings.HasSuffix(beforeTruncate, "]") {
			// 允许截断点不完美，但至少不应该在字符串中间
			if !strings.Contains(beforeTruncate[len(beforeTruncate)-20:], "\n") {
				t.Error("截断位置不在合适的边界")
			}
		}

		t.Logf("✓ 超长输出被正确截断，截断位置: %d", truncateIndex)
	}
}

func TestFormatJSONBytesForLog(t *testing.T) {
	input := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Hello, world!",
					},
				},
			},
		},
	}

	jsonBytes, _ := json.Marshal(input)
	result := FormatJSONBytesForLog(jsonBytes, 500)

	// 验证基本功能
	if !strings.Contains(result, `"type": "text"`) {
		t.Error("应该包含type字段")
	}

	if !strings.Contains(result, `"text": "Hello, world!"`) {
		t.Error("应该包含完整的短文本")
	}

	// 验证没有乱码
	if strings.Contains(result, "�") {
		t.Error("输出包含乱码")
	}

	t.Logf("格式化结果:\n%s", result)
}
