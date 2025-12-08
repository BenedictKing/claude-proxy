package converters

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertResponsesToOpenAIChatRequest 将 OpenAI Responses 请求格式转换为 OpenAI Chat Completions 格式
// 转换内容包括:
// 1. model 和 stream 配置
// 2. instructions → system message
// 3. input 数组 → messages 数组
// 4. tools 定义转换
// 5. function_call 和 function_call_output 处理
// 6. 生成参数映射 (max_tokens, reasoning 等)
//
// 参数:
//   - modelName: 要使用的模型名称
//   - inputRawJSON: Responses 格式的原始 JSON 请求
//   - stream: 是否为流式请求
//
// 返回:
//   - []byte: Chat Completions 格式的请求 JSON
func ConvertResponsesToOpenAIChatRequest(modelName string, inputRawJSON []byte, stream bool) []byte {
	// 基础 Chat Completions 模板
	out := `{"model":"","messages":[],"stream":false}`

	root := gjson.ParseBytes(inputRawJSON)

	// 设置 model
	out, _ = sjson.Set(out, "model", modelName)

	// 设置 stream
	out, _ = sjson.Set(out, "stream", stream)

	// 如果是流式请求，添加 stream_options 以获取 usage 信息
	if stream {
		out, _ = sjson.Set(out, "stream_options.include_usage", true)
	}

	// 映射生成参数
	if maxTokens := root.Get("max_output_tokens"); maxTokens.Exists() {
		out, _ = sjson.Set(out, "max_tokens", maxTokens.Int())
	}

	if parallelToolCalls := root.Get("parallel_tool_calls"); parallelToolCalls.Exists() {
		out, _ = sjson.Set(out, "parallel_tool_calls", parallelToolCalls.Bool())
	}

	if temperature := root.Get("temperature"); temperature.Exists() {
		out, _ = sjson.Set(out, "temperature", temperature.Float())
	}

	if topP := root.Get("top_p"); topP.Exists() {
		out, _ = sjson.Set(out, "top_p", topP.Float())
	}

	if user := root.Get("user"); user.Exists() {
		out, _ = sjson.Set(out, "user", user.String())
	}

	// 转换 instructions → system message
	if instructions := root.Get("instructions"); instructions.Exists() && instructions.String() != "" {
		systemMessage := `{"role":"system","content":""}`
		systemMessage, _ = sjson.Set(systemMessage, "content", instructions.String())
		out, _ = sjson.SetRaw(out, "messages.-1", systemMessage)
	}

	// 转换 input 数组 → messages
	if input := root.Get("input"); input.Exists() {
		if input.IsArray() {
			out = convertInputArrayToMessages(input, out)
		} else if input.Type == gjson.String {
			// 简单字符串输入
			msg := `{"role":"user","content":""}`
			msg, _ = sjson.Set(msg, "content", input.String())
			out, _ = sjson.SetRaw(out, "messages.-1", msg)
		}
	}

	// 转换 tools
	if tools := root.Get("tools"); tools.Exists() && tools.IsArray() {
		out = convertToolsToOpenAIFormat(tools, out)
	}

	// 转换 reasoning.effort → reasoning_effort
	if reasoningEffort := root.Get("reasoning.effort"); reasoningEffort.Exists() {
		effort := reasoningEffort.String()
		switch effort {
		case "none":
			out, _ = sjson.Set(out, "reasoning_effort", "none")
		case "auto":
			out, _ = sjson.Set(out, "reasoning_effort", "auto")
		case "minimal":
			out, _ = sjson.Set(out, "reasoning_effort", "low")
		case "low":
			out, _ = sjson.Set(out, "reasoning_effort", "low")
		case "medium":
			out, _ = sjson.Set(out, "reasoning_effort", "medium")
		case "high":
			out, _ = sjson.Set(out, "reasoning_effort", "high")
		case "xhigh":
			out, _ = sjson.Set(out, "reasoning_effort", "xhigh")
		default:
			out, _ = sjson.Set(out, "reasoning_effort", "auto")
		}
	}

	// 转换 tool_choice
	if toolChoice := root.Get("tool_choice"); toolChoice.Exists() {
		out, _ = sjson.Set(out, "tool_choice", toolChoice.String())
	}

	return []byte(out)
}

// convertInputArrayToMessages 将 input 数组转换为 messages 数组
func convertInputArrayToMessages(input gjson.Result, out string) string {
	input.ForEach(func(_, item gjson.Result) bool {
		itemType := item.Get("type").String()

		// 如果没有 type 但有 role，则视为 message
		if itemType == "" && item.Get("role").String() != "" {
			itemType = "message"
		}

		switch itemType {
		case "message":
			out = convertMessageItem(item, out)

		case "function_call":
			out = convertFunctionCallItem(item, out)

		case "function_call_output":
			out = convertFunctionCallOutputItem(item, out)
		}

		return true
	})

	return out
}

// convertMessageItem 转换 message 类型的 item
func convertMessageItem(item gjson.Result, out string) string {
	role := item.Get("role").String()
	if role == "" {
		role = "user"
	}

	message := `{"role":"","content":""}`
	message, _ = sjson.Set(message, "role", role)

	content := item.Get("content")
	if content.Exists() {
		if content.IsArray() {
			// content 是数组，需要提取文本
			var messageContent string
			var toolCalls []interface{}

			content.ForEach(func(_, contentItem gjson.Result) bool {
				contentType := contentItem.Get("type").String()
				if contentType == "" {
					contentType = "input_text"
				}

				switch contentType {
				case "input_text", "output_text", "text":
					text := contentItem.Get("text").String()
					if messageContent != "" {
						messageContent += "\n" + text
					} else {
						messageContent = text
					}
				}
				return true
			})

			if messageContent != "" {
				message, _ = sjson.Set(message, "content", messageContent)
			}

			if len(toolCalls) > 0 {
				message, _ = sjson.Set(message, "tool_calls", toolCalls)
			}
		} else if content.Type == gjson.String {
			// content 是字符串
			message, _ = sjson.Set(message, "content", content.String())
		}
	}

	out, _ = sjson.SetRaw(out, "messages.-1", message)
	return out
}

// convertFunctionCallItem 转换 function_call 类型的 item
func convertFunctionCallItem(item gjson.Result, out string) string {
	// function_call → assistant message with tool_calls
	assistantMessage := `{"role":"assistant","tool_calls":[]}`

	toolCall := `{"id":"","type":"function","function":{"name":"","arguments":""}}`

	if callID := item.Get("call_id"); callID.Exists() {
		toolCall, _ = sjson.Set(toolCall, "id", callID.String())
	}

	if name := item.Get("name"); name.Exists() {
		toolCall, _ = sjson.Set(toolCall, "function.name", name.String())
	}

	if arguments := item.Get("arguments"); arguments.Exists() {
		toolCall, _ = sjson.Set(toolCall, "function.arguments", arguments.String())
	}

	assistantMessage, _ = sjson.SetRaw(assistantMessage, "tool_calls.0", toolCall)
	out, _ = sjson.SetRaw(out, "messages.-1", assistantMessage)

	return out
}

// convertFunctionCallOutputItem 转换 function_call_output 类型的 item
func convertFunctionCallOutputItem(item gjson.Result, out string) string {
	// function_call_output → tool message
	toolMessage := `{"role":"tool","tool_call_id":"","content":""}`

	if callID := item.Get("call_id"); callID.Exists() {
		toolMessage, _ = sjson.Set(toolMessage, "tool_call_id", callID.String())
	}

	if output := item.Get("output"); output.Exists() {
		toolMessage, _ = sjson.Set(toolMessage, "content", output.String())
	}

	out, _ = sjson.SetRaw(out, "messages.-1", toolMessage)
	return out
}

// convertToolsToOpenAIFormat 将 Responses tools 转换为 OpenAI Chat Completions tools 格式
func convertToolsToOpenAIFormat(tools gjson.Result, out string) string {
	var chatCompletionsTools []interface{}

	tools.ForEach(func(_, tool gjson.Result) bool {
		chatTool := `{"type":"function","function":{}}`

		function := `{"name":"","description":"","parameters":{}}`

		if name := tool.Get("name"); name.Exists() {
			function, _ = sjson.Set(function, "name", name.String())
		}

		if description := tool.Get("description"); description.Exists() {
			function, _ = sjson.Set(function, "description", description.String())
		}

		if parameters := tool.Get("parameters"); parameters.Exists() {
			function, _ = sjson.SetRaw(function, "parameters", parameters.Raw)
		}

		chatTool, _ = sjson.SetRaw(chatTool, "function", function)
		chatCompletionsTools = append(chatCompletionsTools, gjson.Parse(chatTool).Value())

		return true
	})

	if len(chatCompletionsTools) > 0 {
		out, _ = sjson.Set(out, "tools", chatCompletionsTools)
	}

	return out
}
