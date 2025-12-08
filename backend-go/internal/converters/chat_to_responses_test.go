package converters

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertResponsesToOpenAIChatRequest(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		model    string
		stream   bool
		validate func(t *testing.T, result []byte)
	}{
		{
			name: "基本文本输入",
			input: `{
				"model": "gpt-4",
				"input": "Hello, world!",
				"instructions": "You are a helpful assistant."
			}`,
			model:  "gpt-4o",
			stream: false,
			validate: func(t *testing.T, result []byte) {
				root := gjson.ParseBytes(result)
				if root.Get("model").String() != "gpt-4o" {
					t.Errorf("model should be gpt-4o, got %s", root.Get("model").String())
				}
				if root.Get("stream").Bool() != false {
					t.Error("stream should be false")
				}
				messages := root.Get("messages").Array()
				if len(messages) != 2 {
					t.Errorf("should have 2 messages (system + user), got %d", len(messages))
				}
				if messages[0].Get("role").String() != "system" {
					t.Error("first message should be system")
				}
				if messages[1].Get("role").String() != "user" {
					t.Error("second message should be user")
				}
			},
		},
		{
			name: "带 tools 的请求",
			input: `{
				"model": "gpt-4",
				"input": [{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "What's the weather?"}]}],
				"tools": [
					{
						"name": "get_weather",
						"description": "Get weather info",
						"parameters": {"type": "object", "properties": {"location": {"type": "string"}}}
					}
				]
			}`,
			model:  "gpt-4o",
			stream: true,
			validate: func(t *testing.T, result []byte) {
				root := gjson.ParseBytes(result)
				if root.Get("stream").Bool() != true {
					t.Error("stream should be true")
				}
				tools := root.Get("tools").Array()
				if len(tools) != 1 {
					t.Errorf("should have 1 tool, got %d", len(tools))
				}
				if tools[0].Get("function.name").String() != "get_weather" {
					t.Error("tool name should be get_weather")
				}
			},
		},
		{
			name: "function_call 和 function_call_output",
			input: `{
				"model": "gpt-4",
				"input": [
					{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "What's the weather in NYC?"}]},
					{"type": "function_call", "call_id": "call_123", "name": "get_weather", "arguments": "{\"location\": \"NYC\"}"},
					{"type": "function_call_output", "call_id": "call_123", "output": "Sunny, 72°F"}
				]
			}`,
			model:  "gpt-4o",
			stream: false,
			validate: func(t *testing.T, result []byte) {
				root := gjson.ParseBytes(result)
				messages := root.Get("messages").Array()
				if len(messages) != 3 {
					t.Errorf("should have 3 messages, got %d", len(messages))
				}
				// 第二条消息应该是 assistant with tool_calls
				if messages[1].Get("role").String() != "assistant" {
					t.Error("second message should be assistant")
				}
				if !messages[1].Get("tool_calls").Exists() {
					t.Error("assistant message should have tool_calls")
				}
				// 第三条消息应该是 tool
				if messages[2].Get("role").String() != "tool" {
					t.Error("third message should be tool")
				}
			},
		},
		{
			name: "reasoning effort 转换",
			input: `{
				"model": "o1-mini",
				"input": "Think about this",
				"reasoning": {"effort": "high"}
			}`,
			model:  "o1-mini",
			stream: false,
			validate: func(t *testing.T, result []byte) {
				root := gjson.ParseBytes(result)
				if root.Get("reasoning_effort").String() != "high" {
					t.Errorf("reasoning_effort should be high, got %s", root.Get("reasoning_effort").String())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertResponsesToOpenAIChatRequest(tt.model, []byte(tt.input), tt.stream)
			tt.validate(t, result)
		})
	}
}

func TestConvertOpenAIChatToResponses_Stream(t *testing.T) {
	ctx := context.Background()

	// 模拟 OpenAI Chat Completions SSE 流
	sseLines := []string{
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world!"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		`data: [DONE]`,
	}

	originalReq := []byte(`{"model":"gpt-4o","input":"Hi"}`)

	var state any
	var allEvents []string

	for _, line := range sseLines {
		events := ConvertOpenAIChatToResponses(ctx, "gpt-4o", originalReq, nil, []byte(line), &state)
		allEvents = append(allEvents, events...)
	}

	// 验证事件序列
	if len(allEvents) == 0 {
		t.Fatal("should produce events")
	}

	// 检查是否有 response.created 事件
	hasCreated := false
	hasInProgress := false
	hasCompleted := false
	hasTextDelta := false

	for _, ev := range allEvents {
		if strings.Contains(ev, "response.created") {
			hasCreated = true
		}
		if strings.Contains(ev, "response.in_progress") {
			hasInProgress = true
		}
		if strings.Contains(ev, "response.completed") {
			hasCompleted = true
		}
		if strings.Contains(ev, "response.output_text.delta") {
			hasTextDelta = true
		}
	}

	if !hasCreated {
		t.Error("should have response.created event")
	}
	if !hasInProgress {
		t.Error("should have response.in_progress event")
	}
	if !hasCompleted {
		t.Error("should have response.completed event")
	}
	if !hasTextDelta {
		t.Error("should have response.output_text.delta event")
	}
}

func TestConvertOpenAIChatToResponses_ToolCall(t *testing.T) {
	ctx := context.Background()

	// 模拟带 tool_call 的 SSE 流
	sseLines := []string{
		`data: {"id":"chatcmpl-456","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-456","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"loc"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-456","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ation\": \"NYC\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-456","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}

	originalReq := []byte(`{"model":"gpt-4o","input":"What's the weather?","tools":[{"name":"get_weather"}]}`)

	var state any
	var allEvents []string

	for _, line := range sseLines {
		events := ConvertOpenAIChatToResponses(ctx, "gpt-4o", originalReq, nil, []byte(line), &state)
		allEvents = append(allEvents, events...)
	}

	// 验证是否有 function_call 相关事件
	hasFuncAdded := false
	hasFuncDelta := false
	hasFuncDone := false

	for _, ev := range allEvents {
		if strings.Contains(ev, "response.output_item.added") && strings.Contains(ev, "function_call") {
			hasFuncAdded = true
		}
		if strings.Contains(ev, "response.function_call_arguments.delta") {
			hasFuncDelta = true
		}
		if strings.Contains(ev, "response.function_call_arguments.done") {
			hasFuncDone = true
		}
	}

	if !hasFuncAdded {
		t.Error("should have function_call output_item.added event")
	}
	if !hasFuncDelta {
		t.Error("should have function_call_arguments.delta event")
	}
	if !hasFuncDone {
		t.Error("should have function_call_arguments.done event")
	}
}

func TestConvertOpenAIChatToResponsesNonStream(t *testing.T) {
	ctx := context.Background()

	// 模拟 OpenAI Chat Completions 非流式响应
	chatResponse := `{
		"id": "chatcmpl-789",
		"object": "chat.completion",
		"created": 1234567890,
		"model": "gpt-4o",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Hello! How can I help you today?"
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 8,
			"total_tokens": 18
		}
	}`

	originalReq := []byte(`{"model":"gpt-4o","input":"Hi","instructions":"Be helpful"}`)

	result := ConvertOpenAIChatToResponsesNonStream(ctx, "gpt-4o", originalReq, nil, []byte(chatResponse), nil)

	// 解析结果
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// 验证基本字段
	if resp["object"] != "response" {
		t.Errorf("object should be response, got %v", resp["object"])
	}
	if resp["status"] != "completed" {
		t.Errorf("status should be completed, got %v", resp["status"])
	}

	// 验证 output
	output, ok := resp["output"].([]interface{})
	if !ok || len(output) == 0 {
		t.Fatal("output should have items")
	}

	msgItem := output[0].(map[string]interface{})
	if msgItem["type"] != "message" {
		t.Errorf("first output item should be message, got %v", msgItem["type"])
	}

	// 验证 usage
	usage, ok := resp["usage"].(map[string]interface{})
	if !ok {
		t.Fatal("usage should exist")
	}
	if usage["input_tokens"].(float64) != 10 {
		t.Errorf("input_tokens should be 10, got %v", usage["input_tokens"])
	}
	if usage["output_tokens"].(float64) != 8 {
		t.Errorf("output_tokens should be 8, got %v", usage["output_tokens"])
	}
}

func TestConvertOpenAIChatToResponsesNonStream_ToolCalls(t *testing.T) {
	ctx := context.Background()

	// 模拟带 tool_calls 的响应
	chatResponse := `{
		"id": "chatcmpl-tool",
		"object": "chat.completion",
		"created": 1234567890,
		"model": "gpt-4o",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_xyz",
						"type": "function",
						"function": {
							"name": "search",
							"arguments": "{\"query\": \"test\"}"
						}
					}
				]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 5, "completion_tokens": 10, "total_tokens": 15}
	}`

	originalReq := []byte(`{"model":"gpt-4o","input":"Search for test"}`)

	result := ConvertOpenAIChatToResponsesNonStream(ctx, "gpt-4o", originalReq, nil, []byte(chatResponse), nil)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	output, ok := resp["output"].([]interface{})
	if !ok || len(output) == 0 {
		t.Fatal("output should have items")
	}

	// 查找 function_call item
	var funcItem map[string]interface{}
	for _, item := range output {
		itemMap := item.(map[string]interface{})
		if itemMap["type"] == "function_call" {
			funcItem = itemMap
			break
		}
	}

	if funcItem == nil {
		t.Fatal("should have function_call item")
	}

	if funcItem["name"] != "search" {
		t.Errorf("function name should be search, got %v", funcItem["name"])
	}
	if funcItem["call_id"] != "call_xyz" {
		t.Errorf("call_id should be call_xyz, got %v", funcItem["call_id"])
	}
}
