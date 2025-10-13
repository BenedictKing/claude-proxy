package utils

import (
	"encoding/json"
	"regexp"
	"strings"
)

// StreamSynthesizer 流式响应内容合成器
type StreamSynthesizer struct {
	serviceType       string
	synthesizedContent strings.Builder
	toolCallAccumulator map[int]*ToolCall
	parseFailed       bool
}

// ToolCall 工具调用累积器
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// NewStreamSynthesizer 创建新的流合成器
func NewStreamSynthesizer(serviceType string) *StreamSynthesizer {
	return &StreamSynthesizer{
		serviceType:         serviceType,
		toolCallAccumulator: make(map[int]*ToolCall),
	}
}

// ProcessLine 处理SSE流的一行
func (s *StreamSynthesizer) ProcessLine(line string) {
	trimmedLine := strings.TrimSpace(line)
	if trimmedLine == "" {
		return
	}

	// 使用正则匹配SSE data字段
	dataRegex := regexp.MustCompile(`^data:\s*(.*)$`)
	matches := dataRegex.FindStringSubmatch(trimmedLine)
	if len(matches) < 2 {
		return
	}

	jsonStr := strings.TrimSpace(matches[1])
	if jsonStr == "[DONE]" || jsonStr == "" {
		return
	}

	// 解析JSON - 不再因失败而停止处理
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// 记录解析失败但继续处理后续行，而不是完全停止
		if !s.parseFailed {
			s.parseFailed = true
			s.synthesizedContent.WriteString("\n[解析警告: 部分JSON解析失败，将显示原始文本内容]")
		}
		return
	}

	// 如果之前解析失败，但现在成功了，重置失败标记
	if s.parseFailed {
		s.parseFailed = false
	}

	// 根据服务类型解析
	switch s.serviceType {
	case "gemini":
		s.processGemini(data)
	case "openai", "openaiold":
		s.processOpenAI(data)
	case "claude":
		s.processClaude(data)
	}
}

// processGemini 处理Gemini格式
func (s *StreamSynthesizer) processGemini(data map[string]interface{}) {
	candidates, ok := data["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return
	}

	candidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return
	}

	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return
	}

	parts, ok := content["parts"].([]interface{})
	if !ok {
		return
	}

	for _, part := range parts {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			continue
		}

		// 文本内容
		if text, ok := partMap["text"].(string); ok {
			s.synthesizedContent.WriteString(text)
		}

		// 函数调用
		if functionCall, ok := partMap["functionCall"].(map[string]interface{}); ok {
			name, _ := functionCall["name"].(string)
			args, _ := functionCall["args"]
			argsJSON, _ := json.Marshal(args)
			s.synthesizedContent.WriteString("\nTool Call: ")
			s.synthesizedContent.WriteString(name)
			s.synthesizedContent.WriteString("(")
			s.synthesizedContent.Write(argsJSON)
			s.synthesizedContent.WriteString(")")
		}
	}
}

// processOpenAI 处理OpenAI格式
func (s *StreamSynthesizer) processOpenAI(data map[string]interface{}) {
	choices, ok := data["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return
	}

	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return
	}

	// 文本内容
	if content, ok := delta["content"].(string); ok {
		s.synthesizedContent.WriteString(content)
	}

	// 工具调用
	if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
		for _, tc := range toolCalls {
			toolCallMap, ok := tc.(map[string]interface{})
			if !ok {
				continue
			}

			index := 0
			if idx, ok := toolCallMap["index"].(float64); ok {
				index = int(idx)
			}

			if s.toolCallAccumulator[index] == nil {
				s.toolCallAccumulator[index] = &ToolCall{}
			}

			accumulated := s.toolCallAccumulator[index]

			if id, ok := toolCallMap["id"].(string); ok {
				accumulated.ID = id
			}

			if function, ok := toolCallMap["function"].(map[string]interface{}); ok {
				if name, ok := function["name"].(string); ok {
					accumulated.Name = name
				}
				if args, ok := function["arguments"].(string); ok {
					accumulated.Arguments += args
				}
			}
		}
	}
}

// processClaude 处理Claude格式
func (s *StreamSynthesizer) processClaude(data map[string]interface{}) {
	eventType, _ := data["type"].(string)

	switch eventType {
	case "content_block_delta":
		delta, ok := data["delta"].(map[string]interface{})
		if !ok {
			return
		}

		deltaType, _ := delta["type"].(string)

		if deltaType == "text_delta" {
			if text, ok := delta["text"].(string); ok {
				s.synthesizedContent.WriteString(text)
			}
		} else if deltaType == "input_json_delta" {
			if partialJSON, ok := delta["partial_json"].(string); ok {
				blockIndex := 0
				if idx, ok := data["index"].(float64); ok {
					blockIndex = int(idx)
				}

				if s.toolCallAccumulator[blockIndex] == nil {
					s.toolCallAccumulator[blockIndex] = &ToolCall{}
				}

				accumulated := s.toolCallAccumulator[blockIndex]
				accumulated.Arguments += partialJSON
			}
		}

	case "content_block_start":
		contentBlock, ok := data["content_block"].(map[string]interface{})
		if !ok {
			return
		}

		if contentBlock["type"] == "tool_use" {
			blockIndex := 0
			if idx, ok := data["index"].(float64); ok {
				blockIndex = int(idx)
			}

			if s.toolCallAccumulator[blockIndex] == nil {
				s.toolCallAccumulator[blockIndex] = &ToolCall{}
			}

			accumulated := s.toolCallAccumulator[blockIndex]

			if id, ok := contentBlock["id"].(string); ok {
				accumulated.ID = id
			}
			if name, ok := contentBlock["name"].(string); ok {
				accumulated.Name = name
			}
		}
	}
}

// GetSynthesizedContent 获取合成的内容
func (s *StreamSynthesizer) GetSynthesizedContent() string {
	// 不再完全失败，即使有解析错误也返回部分结果
	result := s.synthesizedContent.String()

	// 添加工具调用信息
	if len(s.toolCallAccumulator) > 0 {
		var toolCallsBuilder strings.Builder
		for index, tool := range s.toolCallAccumulator {
			args := tool.Arguments
			if args == "" {
				args = "{}"
			}

			name := tool.Name
			if name == "" {
				name = "unknown_function"
			}

			id := tool.ID
			if id == "" {
				id = "tool_" + string(rune(index))
			}

			toolCallsBuilder.WriteString("\nTool Call: ")
			toolCallsBuilder.WriteString(name)
			toolCallsBuilder.WriteString("(")

			// 尝试格式化JSON
			var parsedArgs interface{}
			if err := json.Unmarshal([]byte(args), &parsedArgs); err == nil {
				prettyArgs, _ := json.Marshal(parsedArgs)
				toolCallsBuilder.Write(prettyArgs)
			} else {
				toolCallsBuilder.WriteString(args)
			}

			toolCallsBuilder.WriteString(") [ID: ")
			toolCallsBuilder.WriteString(id)
			toolCallsBuilder.WriteString("]")
		}

		result += toolCallsBuilder.String()
	}

	return result
}

// IsParseFailed 检查解析是否失败
func (s *StreamSynthesizer) IsParseFailed() bool {
	return s.parseFailed
}

// HasToolCalls 检查是否有工具调用被处理
func (s *StreamSynthesizer) HasToolCalls() bool {
	return len(s.toolCallAccumulator) > 0
}
