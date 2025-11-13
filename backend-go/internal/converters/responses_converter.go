package converters

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BenedictKing/claude-proxy/internal/session"
	"github.com/BenedictKing/claude-proxy/internal/types"
)

// ============== Responses → Claude Messages ==============

// ResponsesToClaudeMessages 将 Responses 格式转换为 Claude Messages 格式
func ResponsesToClaudeMessages(sess *session.Session, newInput interface{}) ([]types.ClaudeMessage, error) {
	messages := []types.ClaudeMessage{}

	// 1. 处理历史消息
	for _, item := range sess.Messages {
		msg, err := responsesItemToClaudeMessage(item)
		if err != nil {
			return nil, fmt.Errorf("转换历史消息失败: %w", err)
		}
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	// 2. 处理新输入
	newItems, err := parseResponsesInput(newInput)
	if err != nil {
		return nil, err
	}

	for _, item := range newItems {
		msg, err := responsesItemToClaudeMessage(item)
		if err != nil {
			return nil, fmt.Errorf("转换新消息失败: %w", err)
		}
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	return messages, nil
}

// responsesItemToClaudeMessage 单个 ResponsesItem 转换为 Claude Message
func responsesItemToClaudeMessage(item types.ResponsesItem) (*types.ClaudeMessage, error) {
	switch item.Type {
	case "text":
		// 文本消息
		contentStr, ok := item.Content.(string)
		if !ok {
			return nil, fmt.Errorf("text 类型的 content 必须是 string")
		}

		// 判断角色：如果是用户输入，角色为 user，否则为 assistant
		role := "user"
		if strings.HasPrefix(contentStr, "[ASSISTANT]") {
			role = "assistant"
			contentStr = strings.TrimPrefix(contentStr, "[ASSISTANT]")
		}

		return &types.ClaudeMessage{
			Role: role,
			Content: []types.ClaudeContent{
				{
					Type: "text",
					Text: contentStr,
				},
			},
		}, nil

	case "tool_call":
		// 工具调用（暂时简化处理）
		return nil, nil

	case "tool_result":
		// 工具结果（暂时简化处理）
		return nil, nil

	default:
		return nil, fmt.Errorf("未知的 item type: %s", item.Type)
	}
}

// ============== Claude Response → Responses ==============

// ClaudeResponseToResponses 将 Claude 响应转换为 Responses 格式
func ClaudeResponseToResponses(claudeResp map[string]interface{}, sessionID string) (*types.ResponsesResponse, error) {
	// 提取字段
	model, _ := claudeResp["model"].(string)
	content, _ := claudeResp["content"].([]interface{})

	// 转换 output
	output := []types.ResponsesItem{}
	for _, c := range content {
		contentBlock, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		blockType, _ := contentBlock["type"].(string)
		if blockType == "text" {
			text, _ := contentBlock["text"].(string)
			output = append(output, types.ResponsesItem{
				Type:    "text",
				Content: text,
			})
		}
	}

	// 提取 usage
	usageMap, _ := claudeResp["usage"].(map[string]interface{})
	usage := types.ResponsesUsage{}
	if usageMap != nil {
		usage.PromptTokens, _ = usageMap["input_tokens"].(int)
		usage.CompletionTokens, _ = usageMap["output_tokens"].(int)
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	// 生成 response ID
	responseID := generateResponseID()

	return &types.ResponsesResponse{
		ID:         responseID,
		Model:      model,
		Output:     output,
		Status:     "completed",
		PreviousID: "", // 将在外部设置
		Usage:      usage,
	}, nil
}

// ============== Responses → OpenAI Chat ==============

// ResponsesToOpenAIChatMessages 将 Responses 格式转换为 OpenAI Chat 格式
func ResponsesToOpenAIChatMessages(sess *session.Session, newInput interface{}) ([]map[string]interface{}, error) {
	messages := []map[string]interface{}{}

	// 1. 处理历史消息
	for _, item := range sess.Messages {
		msg := responsesItemToOpenAIMessage(item)
		if msg != nil {
			messages = append(messages, msg)
		}
	}

	// 2. 处理新输入
	newItems, err := parseResponsesInput(newInput)
	if err != nil {
		return nil, err
	}

	for _, item := range newItems {
		msg := responsesItemToOpenAIMessage(item)
		if msg != nil {
			messages = append(messages, msg)
		}
	}

	return messages, nil
}

// responsesItemToOpenAIMessage 单个 ResponsesItem 转换为 OpenAI Message
func responsesItemToOpenAIMessage(item types.ResponsesItem) map[string]interface{} {
	if item.Type == "text" {
		contentStr, ok := item.Content.(string)
		if !ok {
			return nil
		}

		role := "user"
		if strings.HasPrefix(contentStr, "[ASSISTANT]") {
			role = "assistant"
			contentStr = strings.TrimPrefix(contentStr, "[ASSISTANT]")
		}

		return map[string]interface{}{
			"role":    role,
			"content": contentStr,
		}
	}

	return nil
}

// ============== OpenAI Chat Response → Responses ==============

// OpenAIChatResponseToResponses 将 OpenAI Chat 响应转换为 Responses 格式
func OpenAIChatResponseToResponses(openaiResp map[string]interface{}, sessionID string) (*types.ResponsesResponse, error) {
	// 提取字段
	model, _ := openaiResp["model"].(string)
	choices, _ := openaiResp["choices"].([]interface{})

	// 提取第一个 choice 的 message
	output := []types.ResponsesItem{}
	if len(choices) > 0 {
		choice, ok := choices[0].(map[string]interface{})
		if ok {
			message, _ := choice["message"].(map[string]interface{})
			content, _ := message["content"].(string)
			output = append(output, types.ResponsesItem{
				Type:    "text",
				Content: content,
			})
		}
	}

	// 提取 usage
	usageMap, _ := openaiResp["usage"].(map[string]interface{})
	usage := types.ResponsesUsage{}
	if usageMap != nil {
		promptTokens, _ := usageMap["prompt_tokens"].(float64)
		completionTokens, _ := usageMap["completion_tokens"].(float64)
		totalTokens, _ := usageMap["total_tokens"].(float64)

		usage.PromptTokens = int(promptTokens)
		usage.CompletionTokens = int(completionTokens)
		usage.TotalTokens = int(totalTokens)
	}

	// 生成 response ID
	responseID := generateResponseID()

	return &types.ResponsesResponse{
		ID:         responseID,
		Model:      model,
		Output:     output,
		Status:     "completed",
		PreviousID: "",
		Usage:      usage,
	}, nil
}

// ============== 工具函数 ==============

// parseResponsesInput 解析 input 字段（可能是 string 或 []ResponsesItem）
func parseResponsesInput(input interface{}) ([]types.ResponsesItem, error) {
	switch v := input.(type) {
	case string:
		// 简单文本输入
		return []types.ResponsesItem{
			{
				Type:    "text",
				Content: v,
			},
		}, nil

	case []interface{}:
		// 数组输入
		items := []types.ResponsesItem{}
		for _, item := range v {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			itemType, _ := itemMap["type"].(string)
			content := itemMap["content"]

			items = append(items, types.ResponsesItem{
				Type:    itemType,
				Content: content,
			})
		}
		return items, nil

	case []types.ResponsesItem:
		// 已经是正确类型
		return v, nil

	default:
		return nil, fmt.Errorf("不支持的 input 类型: %T", input)
	}
}

// generateResponseID 生成响应ID
func generateResponseID() string {
	return fmt.Sprintf("resp_%d", getCurrentTimestamp())
}

// getCurrentTimestamp 获取当前时间戳（毫秒）
func getCurrentTimestamp() int64 {
	return 0 // 占位符，实际应使用 time.Now().UnixNano() / 1e6
}

// ExtractTextFromResponses 从 Responses 消息中提取纯文本（用于 OpenAI Completions）
func ExtractTextFromResponses(sess *session.Session, newInput interface{}) (string, error) {
	texts := []string{}

	// 历史消息
	for _, item := range sess.Messages {
		if item.Type == "text" {
			if text, ok := item.Content.(string); ok {
				texts = append(texts, text)
			}
		}
	}

	// 新输入
	newItems, err := parseResponsesInput(newInput)
	if err != nil {
		return "", err
	}

	for _, item := range newItems {
		if item.Type == "text" {
			if text, ok := item.Content.(string); ok {
				texts = append(texts, text)
			}
		}
	}

	return strings.Join(texts, "\n"), nil
}

// OpenAICompletionsResponseToResponses OpenAI Completions 响应转 Responses
func OpenAICompletionsResponseToResponses(completionsResp map[string]interface{}, sessionID string) (*types.ResponsesResponse, error) {
	model, _ := completionsResp["model"].(string)
	choices, _ := completionsResp["choices"].([]interface{})

	output := []types.ResponsesItem{}
	if len(choices) > 0 {
		choice, ok := choices[0].(map[string]interface{})
		if ok {
			text, _ := choice["text"].(string)
			output = append(output, types.ResponsesItem{
				Type:    "text",
				Content: text,
			})
		}
	}

	usage := types.ResponsesUsage{}
	usageMap, _ := completionsResp["usage"].(map[string]interface{})
	if usageMap != nil {
		promptTokens, _ := usageMap["prompt_tokens"].(float64)
		completionTokens, _ := usageMap["completion_tokens"].(float64)
		totalTokens, _ := usageMap["total_tokens"].(float64)

		usage.PromptTokens = int(promptTokens)
		usage.CompletionTokens = int(completionTokens)
		usage.TotalTokens = int(totalTokens)
	}

	responseID := generateResponseID()

	return &types.ResponsesResponse{
		ID:         responseID,
		Model:      model,
		Output:     output,
		Status:     "completed",
		PreviousID: "",
		Usage:      usage,
	}, nil
}

// JSONToMap 将 JSON 字节转为 map
func JSONToMap(data []byte) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := json.Unmarshal(data, &result)
	return result, err
}
