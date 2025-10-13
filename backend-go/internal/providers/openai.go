package providers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/types"
)

// OpenAIProvider OpenAI 提供商
type OpenAIProvider struct{}

// ConvertToProviderRequest 转换为 OpenAI 请求
func (p *OpenAIProvider) ConvertToProviderRequest(c *gin.Context, upstream *config.UpstreamConfig, apiKey string) (*http.Request, []byte, error) {
	// 读取和解析原始请求体
	originalBodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("读取请求体失败: %w", err)
	}
	// 恢复请求体，以便gin context可以被其他地方再次读取（尽管这里我们已经完全处理了）
	c.Request.Body = io.NopCloser(bytes.NewReader(originalBodyBytes))

	var claudeReq types.ClaudeRequest
	if err := json.Unmarshal(originalBodyBytes, &claudeReq); err != nil {
		return nil, originalBodyBytes, fmt.Errorf("解析Claude请求体失败: %w", err)
	}

	// --- 复用旧的转换逻辑 ---
	openaiReq := &types.OpenAIRequest{
		Model:       config.RedirectModel(claudeReq.Model, upstream),
		Messages:    p.convertMessages(&claudeReq),
		Stream:      claudeReq.Stream,
		Temperature: claudeReq.Temperature,
	}

	if claudeReq.MaxTokens > 0 {
		openaiReq.MaxCompletionTokens = claudeReq.MaxTokens
	} else {
		openaiReq.MaxCompletionTokens = 65535
	}

	// 转换工具
	if len(claudeReq.Tools) > 0 {
		openaiReq.Tools = p.convertTools(claudeReq.Tools)
		openaiReq.ToolChoice = "auto"
	}
	// --- 转换逻辑结束 ---

	reqBodyBytes, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, originalBodyBytes, fmt.Errorf("序列化OpenAI请求体失败: %w", err)
	}

	url := strings.TrimSuffix(upstream.BaseURL, "/") + "/chat/completions"

	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBodyBytes))
	if err != nil {
		return nil, originalBodyBytes, fmt.Errorf("创建OpenAI请求失败: %w", err)
	}

	// 复制原始Header，并覆盖认证和内容类型
	req.Header = c.Request.Header.Clone()
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Host = req.URL.Host // 设置正确的Host头部
	req.Header.Del("x-proxy-key")
	req.Header.Del("X-Forwarded-Host")
	req.Header.Del("X-Forwarded-Proto")

	return req, originalBodyBytes, nil
}

// convertMessages 转换消息
func (p *OpenAIProvider) convertMessages(claudeReq *types.ClaudeRequest) []types.OpenAIMessage {
	messages := []types.OpenAIMessage{}

	// 添加系统消息
	if claudeReq.System != nil {
		systemText := extractSystemText(claudeReq.System)
		if systemText != "" {
			messages = append(messages, types.OpenAIMessage{
				Role:    "system",
				Content: systemText,
			})
		}
	}

	// 转换普通消息
	for _, msg := range claudeReq.Messages {
		openaiMsg := p.convertMessage(msg)
		messages = append(messages, openaiMsg...)
	}

	return messages
}

// convertMessage 转换单个消息
func (p *OpenAIProvider) convertMessage(msg types.ClaudeMessage) []types.OpenAIMessage {
	messages := []types.OpenAIMessage{}

	// 如果是字符串内容
	if str, ok := msg.Content.(string); ok {
		if msg.Role != "tool" {
			messages = append(messages, types.OpenAIMessage{
				Role:    normalizeRole(msg.Role),
				Content: str,
			})
		}
		return messages
	}

	// 如果是内容数组
	contents, ok := msg.Content.([]interface{})
	if !ok {
		return messages
	}

	textContents := []string{}
	toolCalls := []types.OpenAIToolCall{}
	toolResults := []types.OpenAIMessage{}

	for _, c := range contents {
		content, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		contentType, _ := content["type"].(string)

		switch contentType {
		case "text":
			if text, ok := content["text"].(string); ok {
				textContents = append(textContents, text)
			}

		case "tool_use":
			id, _ := content["id"].(string)
			name, _ := content["name"].(string)
			input := content["input"]

			inputJSON, _ := json.Marshal(input)
			toolCalls = append(toolCalls, types.OpenAIToolCall{
				ID:   id,
				Type: "function",
				Function: types.OpenAIToolCallFunction{
					Name:      name,
					Arguments: string(inputJSON),
				},
			})

		case "tool_result":
			toolUseID, _ := content["tool_use_id"].(string)
			resultContent := content["content"]

			var contentStr string
			if str, ok := resultContent.(string); ok {
				contentStr = str
			} else {
				contentJSON, _ := json.Marshal(resultContent)
				contentStr = string(contentJSON)
			}

			toolResults = append(toolResults, types.OpenAIMessage{
				Role:       "tool",
				ToolCallID: toolUseID,
				Content:    contentStr,
			})
		}
	}

	// 添加工具结果
	messages = append(messages, toolResults...)

	// 添加文本和工具调用
	if len(textContents) > 0 || len(toolCalls) > 0 {
		role := normalizeRole(msg.Role)
		if role != "tool" {
			openaiMsg := types.OpenAIMessage{
				Role: role,
			}

			if len(textContents) > 0 {
				openaiMsg.Content = strings.Join(textContents, "\n")
			} else {
				openaiMsg.Content = nil
			}

			if len(toolCalls) > 0 {
				openaiMsg.ToolCalls = toolCalls
			}

			messages = append(messages, openaiMsg)
		}
	}

	return messages
}

// convertTools 转换工具
func (p *OpenAIProvider) convertTools(claudeTools []types.ClaudeTool) []types.OpenAITool {
	tools := []types.OpenAITool{}

	for _, tool := range claudeTools {
		tools = append(tools, types.OpenAITool{
			Type: "function",
			Function: types.OpenAIToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  cleanJsonSchema(tool.InputSchema),
			},
		})
	}

	return tools
}

// cleanJsonSchema 清理 JSON Schema，移除某些上游不支持的字段
func cleanJsonSchema(schema interface{}) interface{} {
	if schema == nil {
		return schema
	}

	// 如果是 map，递归清理
	if schemaMap, ok := schema.(map[string]interface{}); ok {
		cleaned := make(map[string]interface{})

		for key, value := range schemaMap {
			// 移除不需要的字段
			if key == "$schema" || key == "title" || key == "examples" || key == "additionalProperties" {
				continue
			}
			// 移除 format 字段（当类型为 string 时）
			if key == "format" {
				if schemaType, hasType := schemaMap["type"]; hasType && schemaType == "string" {
					continue
				}
			}
			// 递归处理嵌套对象
			if key == "properties" || key == "items" {
				cleaned[key] = cleanJsonSchema(value)
			} else if valueMap, isMap := value.(map[string]interface{}); isMap {
				cleaned[key] = cleanJsonSchema(valueMap)
			} else if valueSlice, isSlice := value.([]interface{}); isSlice {
				cleanedSlice := make([]interface{}, len(valueSlice))
				for i, item := range valueSlice {
					cleanedSlice[i] = cleanJsonSchema(item)
				}
				cleaned[key] = cleanedSlice
			} else {
				cleaned[key] = value
			}
		}

		return cleaned
	}

	// 如果是数组，递归清理每个元素
	if schemaSlice, ok := schema.([]interface{}); ok {
		cleaned := make([]interface{}, len(schemaSlice))
		for i, item := range schemaSlice {
			cleaned[i] = cleanJsonSchema(item)
		}
		return cleaned
	}

	// 其他类型直接返回
	return schema
}

// ConvertToClaudeResponse 转换为 Claude 响应
func (p *OpenAIProvider) ConvertToClaudeResponse(providerResp *types.ProviderResponse) (*types.ClaudeResponse, error) {
	var openaiResp types.OpenAIResponse
	if err := json.Unmarshal(providerResp.Body, &openaiResp); err != nil {
		return nil, err
	}

	claudeResp := &types.ClaudeResponse{
		ID:      generateID(),
		Type:    "message",
		Role:    "assistant",
		Content: []types.ClaudeContent{},
	}

	if len(openaiResp.Choices) > 0 {
		choice := openaiResp.Choices[0]
		msg := choice.Message

		// 添加文本内容
		if str, ok := msg.Content.(string); ok && str != "" {
			claudeResp.Content = append(claudeResp.Content, types.ClaudeContent{
				Type: "text",
				Text: str,
			})
		}

		// 添加工具调用
		for _, toolCall := range msg.ToolCalls {
			var input interface{}
			json.Unmarshal([]byte(toolCall.Function.Arguments), &input)

			claudeResp.Content = append(claudeResp.Content, types.ClaudeContent{
				Type:  "tool_use",
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: input,
			})
		}

		// 设置停止原因
		if len(msg.ToolCalls) > 0 {
			claudeResp.StopReason = "tool_use"
		} else if choice.FinishReason == "length" {
			claudeResp.StopReason = "max_tokens"
		} else {
			claudeResp.StopReason = "end_turn"
		}
	}

	// 添加使用统计
	if openaiResp.Usage != nil {
		claudeResp.Usage = &types.Usage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		}
	}

	return claudeResp, nil
}

// HandleStreamResponse 处理流式响应
func (p *OpenAIProvider) HandleStreamResponse(body io.ReadCloser) (<-chan string, <-chan error, error) {
	eventChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(eventChan)
		// defer close(errChan) // 移除此行，避免竞态条件
		defer body.Close()

		scanner := bufio.NewScanner(body)
		textBlockIndex := 0
		toolUseBlockIndex := 0
		toolCallAccumulator := make(map[int]*ToolCallAccumulator)
		toolUseStopEmitted := false

		for scanner.Scan() {
			line := scanner.Text()
			line = strings.TrimSpace(line)

			if line == "" || line == "data: [DONE]" {
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			jsonStr := strings.TrimPrefix(line, "data: ")

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
				continue
			}

			// 检查是否有错误
			if errObj, ok := chunk["error"]; ok {
				errChan <- fmt.Errorf("upstream error: %v", errObj)
				return
			}

			choices, ok := chunk["choices"].([]interface{})
			if !ok || len(choices) == 0 {
				continue
			}

			choice, ok := choices[0].(map[string]interface{})
			if !ok {
				continue
			}

			delta, ok := choice["delta"].(map[string]interface{})
			if !ok {
				continue
			}

			// 处理文本内容
			if content, ok := delta["content"].(string); ok && content != "" {
				events := processTextPart(content, textBlockIndex)
				for _, event := range events {
					eventChan <- event
				}
				textBlockIndex++
			}

			// 处理工具调用
			if toolCalls, ok := delta["tool_calls"].([]interface{});  ok {
				for _, tc := range toolCalls {
					toolCall, ok := tc.(map[string]interface{})
					if !ok {
						continue
					}

					index := 0
					if idx, ok := toolCall["index"].(float64); ok {
						index = int(idx)
					}

					// 获取或创建累加器
					if _, exists := toolCallAccumulator[index]; !exists {
						toolCallAccumulator[index] = &ToolCallAccumulator{}
					}
					acc := toolCallAccumulator[index]

					// 累积数据
					if id, ok := toolCall["id"].(string); ok {
						acc.ID = id
					}

					if function, ok := toolCall["function"].(map[string]interface{}); ok {
						if name, ok := function["name"].(string); ok {
							acc.Name = name
						}
						if args, ok := function["arguments"].(string); ok {
							acc.Arguments += args
						}
					}

					// 检查是否完整
					if acc.ID != "" && acc.Name != "" && acc.Arguments != "" {
						var args interface{}
						if err := json.Unmarshal([]byte(acc.Arguments), &args); err == nil {
							events := processToolUsePart(acc.ID, acc.Name, args, toolUseBlockIndex)
							for _, event := range events {
								eventChan <- event
							}
							toolUseBlockIndex++
							delete(toolCallAccumulator, index)
						}
					}
				}
			}

			// 处理结束原因
			if finishReason, ok := choice["finish_reason"].(string); ok {
				if !toolUseStopEmitted && (finishReason == "tool_calls" || finishReason == "function_call") {
					event := map[string]interface{}{
						"type": "message_delta",
						"delta": map[string]string{
							"stop_reason": "tool_use",
						},
					}
					eventJSON, _ := json.Marshal(event)
					eventChan <- fmt.Sprintf("event: message_delta\ndata: %s\n\n", eventJSON)
					toolUseStopEmitted = true
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- err
		}
	}()

	return eventChan, errChan, nil
}

// ToolCallAccumulator 工具调用累加器
type ToolCallAccumulator struct {
	ID        string
	Name      string
	Arguments string
}

// processTextPart 处理文本部分
func processTextPart(text string, index int) []string {
	events := []string{}

	// content_block_start
	startEvent := map[string]interface{}{
		"type": "content_block_start",
		"index": index,
		"content_block": map[string]string{
			"type": "text",
			"text": "",
		},
	}
	startJSON, _ := json.Marshal(startEvent)
	events = append(events, fmt.Sprintf("event: content_block_start\ndata: %s\n\n", startJSON))

	// content_block_delta
	deltaEvent := map[string]interface{}{
		"type": "content_block_delta",
		"index": index,
		"delta": map[string]string{
			"type": "text_delta",
			"text": text,
		},
	}
	deltaJSON, _ := json.Marshal(deltaEvent)
	events = append(events, fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", deltaJSON))

	// content_block_stop
	stopEvent := map[string]interface{}{
		"type": "content_block_stop",
		"index": index,
	}
	stopJSON, _ := json.Marshal(stopEvent)
	events = append(events, fmt.Sprintf("event: content_block_stop\ndata: %s\n\n", stopJSON))

	return events
}

// processToolUsePart 处理工具使用部分
func processToolUsePart(id, name string, input interface{}, index int) []string {
	events := []string{}

	// content_block_start
	startEvent := map[string]interface{}{
		"type": "content_block_start",
		"index": index,
		"content_block": map[string]interface{}{
			"type": "tool_use",
			"id": id,
			"name": name,
		},
	}
	startJSON, _ := json.Marshal(startEvent)
	events = append(events, fmt.Sprintf("event: content_block_start\ndata: %s\n\n", startJSON))

	// content_block_delta
	inputJSON, _ := json.Marshal(input)
	deltaEvent := map[string]interface{}{
		"type": "content_block_delta",
		"index": index,
		"delta": map[string]string{
			"type": "input_json_delta",
			"partial_json": string(inputJSON),
		},
	}
	deltaJSON, _ := json.Marshal(deltaEvent)
	events = append(events, fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", deltaJSON))

	// content_block_stop
	stopEvent := map[string]interface{}{
		"type": "content_block_stop",
		"index": index,
	}
	stopJSON, _ := json.Marshal(stopEvent)
	events = append(events, fmt.Sprintf("event: content_block_stop\ndata: %s\n\n", stopJSON))

	return events
}

// 辅助函数

func extractSystemText(system interface{}) string {
	if str, ok := system.(string); ok {
		return str
	}

	// 可能是数组
	arr, ok := system.([]interface{})
	if !ok {
		return ""
	}

	parts := []string{}
	for _, item := range arr {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		if obj["type"] == "text" {
			if text, ok := obj["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}

	return strings.Join(parts, "\n")
}

func normalizeRole(role string) string {
	role = strings.ToLower(role)
	switch role {
	case "user", "assistant", "system", "tool":
		return role
	default:
		return "user"
	}
}

func generateID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
