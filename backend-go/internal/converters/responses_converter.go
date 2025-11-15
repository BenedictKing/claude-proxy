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
// instructions 参数会被转换为 Claude API 的 system 参数（不在 messages 中）
func ResponsesToClaudeMessages(sess *session.Session, newInput interface{}, instructions string) ([]types.ClaudeMessage, string, error) {
	messages := []types.ClaudeMessage{}

	// 1. 处理历史消息
	for _, item := range sess.Messages {
		msg, err := responsesItemToClaudeMessage(item)
		if err != nil {
			return nil, "", fmt.Errorf("转换历史消息失败: %w", err)
		}
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	// 2. 处理新输入
	newItems, err := parseResponsesInput(newInput)
	if err != nil {
		return nil, "", err
	}

	for _, item := range newItems {
		msg, err := responsesItemToClaudeMessage(item)
		if err != nil {
			return nil, "", fmt.Errorf("转换新消息失败: %w", err)
		}
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	return messages, instructions, nil
}

// responsesItemToClaudeMessage 单个 ResponsesItem 转换为 Claude Message
func responsesItemToClaudeMessage(item types.ResponsesItem) (*types.ClaudeMessage, error) {
	switch item.Type {
	case "message":
		// 新格式：嵌套结构（type=message, role=user/assistant, content=[]ContentBlock）
		role := item.Role
		if role == "" {
			role = "user" // 默认为 user
		}

		contentText := extractTextFromContent(item.Content)
		if contentText == "" {
			return nil, nil // 空内容，跳过
		}

		return &types.ClaudeMessage{
			Role: role,
			Content: []types.ClaudeContent{
				{
					Type: "text",
					Text: contentText,
				},
			},
		}, nil

	case "text":
		// 旧格式：简单 string（向后兼容）
		contentStr := extractTextFromContent(item.Content)
		if contentStr == "" {
			return nil, fmt.Errorf("text 类型的 content 不能为空")
		}

		// 使用 item.Role（如果存在），否则默认为 user
		role := "user"
		if item.Role != "" {
			role = item.Role
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
func ResponsesToOpenAIChatMessages(sess *session.Session, newInput interface{}, instructions string) ([]map[string]interface{}, error) {
	messages := []map[string]interface{}{}

	// 1. 处理 instructions（如果存在）
	if instructions != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": instructions,
		})
	}

	// 2. 处理历史消息
	for _, item := range sess.Messages {
		msg := responsesItemToOpenAIMessage(item)
		if msg != nil {
			messages = append(messages, msg)
		}
	}

	// 3. 处理新输入
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
	switch item.Type {
	case "message":
		// 新格式：嵌套结构
		role := item.Role
		if role == "" {
			role = "user"
		}

		contentText := extractTextFromContent(item.Content)
		if contentText == "" {
			return nil
		}

		return map[string]interface{}{
			"role":    role,
			"content": contentText,
		}

	case "text":
		// 旧格式：简单 string
		contentStr := extractTextFromContent(item.Content)
		if contentStr == "" {
			return nil
		}

		role := "user"
		if item.Role != "" {
			role = item.Role
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

// extractTextFromContent 从 content 中提取文本内容
// 支持三种格式：
// 1. string - 直接返回
// 2. []ContentBlock - 提取 input_text/output_text 类型的 text 字段
// 3. []interface{} - 动态解析为 ContentBlock
func extractTextFromContent(content interface{}) string {
	// 1. 如果是 string，直接返回
	if str, ok := content.(string); ok {
		return str
	}

	// 2. 如果是 []ContentBlock（已解析类型）
	if blocks, ok := content.([]types.ContentBlock); ok {
		texts := []string{}
		for _, block := range blocks {
			if block.Type == "input_text" || block.Type == "output_text" {
				texts = append(texts, block.Text)
			}
		}
		return strings.Join(texts, "\n")
	}

	// 3. 如果是 []interface{}（未解析类型）
	if arr, ok := content.([]interface{}); ok {
		texts := []string{}
		for _, c := range arr {
			if block, ok := c.(map[string]interface{}); ok {
				blockType, _ := block["type"].(string)
				if blockType == "input_text" || blockType == "output_text" {
					if text, ok := block["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "\n")
	}

	return ""
}

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
