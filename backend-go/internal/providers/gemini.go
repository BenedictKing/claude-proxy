package providers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yourusername/claude-proxy/internal/config"
	"github.com/yourusername/claude-proxy/internal/types"
)

// GeminiProvider Gemini 提供商
type GeminiProvider struct{}

// ConvertToProviderRequest 转换为 Gemini 请求
func (p *GeminiProvider) ConvertToProviderRequest(claudeReq *types.ClaudeRequest, upstream *config.UpstreamConfig, apiKey string) (*types.ProviderRequest, error) {
	geminiReq := p.convertToGeminiRequest(claudeReq, upstream)

	body, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, err
	}

	// 构建 URL
	model := config.RedirectModel(claudeReq.Model, upstream)
	action := "generateContent"
	if claudeReq.Stream {
		action = "streamGenerateContent"
	}

	url := fmt.Sprintf("%s/models/%s:%s?key=%s",
		strings.TrimSuffix(upstream.BaseURL, "/"),
		model,
		action,
		apiKey)

	return &types.ProviderRequest{
		URL:    url,
		Method: "POST",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: body,
	}, nil
}

// convertToGeminiRequest 转换为 Gemini 请求体
func (p *GeminiProvider) convertToGeminiRequest(claudeReq *types.ClaudeRequest, upstream *config.UpstreamConfig) map[string]interface{} {
	req := map[string]interface{}{
		"contents": p.convertMessages(claudeReq.Messages),
	}

	// 添加系统指令
	if claudeReq.System != nil {
		systemText := extractSystemText(claudeReq.System)
		if systemText != "" {
			req["systemInstruction"] = map[string]interface{}{
				"parts": []map[string]string{
					{"text": systemText},
				},
			}
		}
	}

	// 生成配置
	genConfig := map[string]interface{}{}

	if claudeReq.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = claudeReq.MaxTokens
	}

	if claudeReq.Temperature > 0 {
		genConfig["temperature"] = claudeReq.Temperature
	}

	if len(genConfig) > 0 {
		req["generationConfig"] = genConfig
	}

	// 工具
	if len(claudeReq.Tools) > 0 {
		req["tools"] = []map[string]interface{}{
			{
				"functionDeclarations": p.convertTools(claudeReq.Tools),
			},
		}
	}

	return req
}

// convertMessages 转换消息
func (p *GeminiProvider) convertMessages(claudeMessages []types.ClaudeMessage) []map[string]interface{} {
	messages := []map[string]interface{}{}

	for _, msg := range claudeMessages {
		geminiMsg := p.convertMessage(msg)
		if geminiMsg != nil {
			messages = append(messages, geminiMsg)
		}
	}

	return messages
}

// convertMessage 转换单个消息
func (p *GeminiProvider) convertMessage(msg types.ClaudeMessage) map[string]interface{} {
	role := msg.Role
	if role == "assistant" {
		role = "model"
	}

	parts := []interface{}{}

	// 处理字符串内容
	if str, ok := msg.Content.(string); ok {
		parts = append(parts, map[string]string{
			"text": str,
		})
		return map[string]interface{}{
			"role":  role,
			"parts": parts,
		}
	}

	// 处理内容数组
	contents, ok := msg.Content.([]interface{})
	if !ok {
		return nil
	}

	for _, c := range contents {
		content, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		contentType, _ := content["type"].(string)

		switch contentType {
		case "text":
			if text, ok := content["text"].(string); ok {
				parts = append(parts, map[string]string{
					"text": text,
				})
			}

		case "tool_use":
			name, _ := content["name"].(string)
			input := content["input"]

			parts = append(parts, map[string]interface{}{
				"functionCall": map[string]interface{}{
					"name": name,
					"args": input,
				},
			})

		case "tool_result":
			toolUseID, _ := content["tool_use_id"].(string)
			resultContent := content["content"]

			var response interface{}
			if str, ok := resultContent.(string); ok {
				response = map[string]string{"result": str}
			} else {
				response = resultContent
			}

			parts = append(parts, map[string]interface{}{
				"functionResponse": map[string]interface{}{
					"name":     toolUseID,
					"response": response,
				},
			})
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return map[string]interface{}{
		"role":  role,
		"parts": parts,
	}
}

// convertTools 转换工具
func (p *GeminiProvider) convertTools(claudeTools []types.ClaudeTool) []map[string]interface{} {
	tools := []map[string]interface{}{}

	for _, tool := range claudeTools {
		tools = append(tools, map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.InputSchema,
		})
	}

	return tools
}

// ConvertToClaudeResponse 转换为 Claude 响应
func (p *GeminiProvider) ConvertToClaudeResponse(providerResp *types.ProviderResponse) (*types.ClaudeResponse, error) {
	var geminiResp map[string]interface{}
	if err := json.Unmarshal(providerResp.Body, &geminiResp); err != nil {
		return nil, err
	}

	claudeResp := &types.ClaudeResponse{
		ID:      generateID(),
		Type:    "message",
		Role:    "assistant",
		Content: []types.ClaudeContent{},
	}

	candidates, ok := geminiResp["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return claudeResp, nil
	}

	candidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return claudeResp, nil
	}

	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return claudeResp, nil
	}

	parts, ok := content["parts"].([]interface{})
	if !ok {
		return claudeResp, nil
	}

	// 处理各个部分
	for _, p := range parts {
		part, ok := p.(map[string]interface{})
		if !ok {
			continue
		}

		// 文本内容
		if text, ok := part["text"].(string); ok {
			claudeResp.Content = append(claudeResp.Content, types.ClaudeContent{
				Type: "text",
				Text: text,
			})
		}

		// 函数调用
		if fc, ok := part["functionCall"].(map[string]interface{}); ok {
			name, _ := fc["name"].(string)
			args := fc["args"]

			claudeResp.Content = append(claudeResp.Content, types.ClaudeContent{
				Type:  "tool_use",
				ID:    fmt.Sprintf("toolu_%d", len(claudeResp.Content)),
				Name:  name,
				Input: args,
			})
		}
	}

	// 设置停止原因
	finishReason, _ := candidate["finishReason"].(string)
	if strings.Contains(strings.ToLower(finishReason), "stop") {
		// 检查是否有工具调用
		hasToolCall := false
		for _, c := range claudeResp.Content {
			if c.Type == "tool_use" {
				hasToolCall = true
				break
			}
		}

		if hasToolCall {
			claudeResp.StopReason = "tool_use"
		} else {
			claudeResp.StopReason = "end_turn"
		}
	} else if strings.Contains(strings.ToLower(finishReason), "length") {
		claudeResp.StopReason = "max_tokens"
	}

	// 使用统计
	if usageMetadata, ok := geminiResp["usageMetadata"].(map[string]interface{}); ok {
		usage := &types.Usage{}
		if promptTokens, ok := usageMetadata["promptTokenCount"].(float64); ok {
			usage.InputTokens = int(promptTokens)
		}
		if candidatesTokens, ok := usageMetadata["candidatesTokenCount"].(float64); ok {
			usage.OutputTokens = int(candidatesTokens)
		}
		claudeResp.Usage = usage
	}

	return claudeResp, nil
}

// HandleStreamResponse 处理流式响应
func (p *GeminiProvider) HandleStreamResponse(body io.ReadCloser) (<-chan string, <-chan error, error) {
	eventChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(eventChan)
		defer close(errChan)
		defer body.Close()

		scanner := bufio.NewScanner(body)
		textBlockIndex := 0
		toolUseBlockIndex := 0

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

			candidates, ok := chunk["candidates"].([]interface{})
			if !ok || len(candidates) == 0 {
				continue
			}

			candidate, ok := candidates[0].(map[string]interface{})
			if !ok {
				continue
			}

			content, ok := candidate["content"].(map[string]interface{})
			if !ok {
				continue
			}

			parts, ok := content["parts"].([]interface{})
			if !ok {
				continue
			}

			for _, p := range parts {
				part, ok := p.(map[string]interface{})
				if !ok {
					continue
				}

				// 处理文本
				if text, ok := part["text"].(string); ok {
					events := processTextPart(text, textBlockIndex)
					for _, event := range events {
						eventChan <- event
					}
					textBlockIndex++
				}

				// 处理函数调用
				if fc, ok := part["functionCall"].(map[string]interface{}); ok {
					name, _ := fc["name"].(string)
					args := fc["args"]
					id := fmt.Sprintf("toolu_%d", toolUseBlockIndex)

					events := processToolUsePart(id, name, args, toolUseBlockIndex)
					for _, event := range events {
						eventChan <- event
					}
					toolUseBlockIndex++
				}
			}

			// 处理结束原因
			if finishReason, ok := candidate["finishReason"].(string); ok {
				if strings.Contains(strings.ToLower(finishReason), "stop") {
					event := map[string]interface{}{
						"type": "message_delta",
						"delta": map[string]string{
							"stop_reason": "end_turn",
						},
					}
					eventJSON, _ := json.Marshal(event)
					eventChan <- fmt.Sprintf("event: message_delta\ndata: %s\n\n", eventJSON)
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- err
		}
	}()

	return eventChan, errChan, nil
}
