package utils

import (
	"encoding/json"
	"strings"
)

// TruncateJSONIntelligently 智能截断JSON中的长文本内容,保持结构完整
// 只截断字符串值,不影响JSON结构
func TruncateJSONIntelligently(data interface{}, maxTextLength int) interface{} {
	if data == nil {
		return nil
	}

	switch v := data.(type) {
	case string:
		if len(v) > maxTextLength {
			return v[:maxTextLength] + "..."
		}
		return v

	case float64, int, int64, bool:
		return v

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = TruncateJSONIntelligently(item, maxTextLength)
		}
		return result

	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, value := range v {
			result[key] = TruncateJSONIntelligently(value, maxTextLength)
		}
		return result

	default:
		return v
	}
}

// SimplifyToolsArray 简化tools数组为名称列表,减少日志输出
// 将完整的工具定义简化为只显示工具名称
func SimplifyToolsArray(data interface{}) interface{} {
	if data == nil {
		return nil
	}

	switch v := data.(type) {
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = SimplifyToolsArray(item)
		}
		return result

	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, value := range v {
			// 如果是tools字段且是数组,提取工具名称
			if key == "tools" {
				if toolsArray, ok := value.([]interface{}); ok {
					simplifiedTools := make([]interface{}, len(toolsArray))
					for i, tool := range toolsArray {
						simplifiedTools[i] = extractToolName(tool)
					}
					result[key] = simplifiedTools
					continue
				}
			}
			// 如果是content字段且是数组,标记为需要紧凑显示
			if key == "content" {
				if contentArray, ok := value.([]interface{}); ok {
					result[key] = compactContentArray(contentArray)
					continue
				}
			}
			result[key] = SimplifyToolsArray(value)
		}
		return result

	default:
		return v
	}
}

// compactContentArray 紧凑显示content数组
// 只保留type和text/id/name等关键字段的简短摘要
func compactContentArray(contents []interface{}) []interface{} {
	result := make([]interface{}, len(contents))
	for i, item := range contents {
		if contentMap, ok := item.(map[string]interface{}); ok {
			compact := make(map[string]interface{})

			// 保留type字段
			if contentType, ok := contentMap["type"].(string); ok {
				compact["type"] = contentType

				// 根据类型保留关键信息
				switch contentType {
				case "text":
					if text, ok := contentMap["text"].(string); ok {
						// 文本内容截断到前200个字符
						if len(text) > 200 {
							compact["text"] = text[:200] + "..."
						} else {
							compact["text"] = text
						}
					}
				case "tool_use":
					if id, ok := contentMap["id"].(string); ok {
						compact["id"] = id
					}
					if name, ok := contentMap["name"].(string); ok {
						compact["name"] = name
					}
					// input字段紧凑显示 - 保留结构但截断长字符串值
					if input, ok := contentMap["input"]; ok {
						compactInput := truncateInputValues(input, 200)
						compact["input"] = compactInput
					}
				case "tool_result":
					if toolUseID, ok := contentMap["tool_use_id"].(string); ok {
						compact["tool_use_id"] = toolUseID
					}
					// content字段显示前200字符
					if content, ok := contentMap["content"].(string); ok {
						if len(content) > 200 {
							compact["content"] = content[:200] + "..."
						} else {
							compact["content"] = content
						}
					}
					if isError, ok := contentMap["is_error"].(bool); ok {
						compact["is_error"] = isError
					}
				case "image":
					if source, ok := contentMap["source"].(map[string]interface{}); ok {
						compact["source"] = map[string]interface{}{
							"type": source["type"],
						}
					}
				}
			}
			result[i] = compact
		} else {
			result[i] = item
		}
	}
	return result
}

// truncateInputValues 递归截断input对象中的长字符串值
// 保留JSON结构,只截断字符串值到指定长度
func truncateInputValues(data interface{}, maxLength int) interface{} {
	switch v := data.(type) {
	case string:
		if len(v) > maxLength {
			return v[:maxLength] + "..."
		}
		return v

	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, value := range v {
			result[key] = truncateInputValues(value, maxLength)
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = truncateInputValues(item, maxLength)
		}
		return result

	default:
		return v
	}
}

// extractToolName 从工具定义中提取名称
// 支持Claude格式(tool.name)和OpenAI格式(tool.function.name)
func extractToolName(tool interface{}) interface{} {
	toolMap, ok := tool.(map[string]interface{})
	if !ok {
		return tool
	}

	// 检查Claude格式: tool.name
	if name, ok := toolMap["name"].(string); ok {
		return name
	}

	// 检查OpenAI格式: tool.function.name
	if function, ok := toolMap["function"].(map[string]interface{}); ok {
		if name, ok := function["name"].(string); ok {
			return name
		}
	}

	return tool
}

// SimplifyToolsInJSON 简化JSON字节数组中的tools字段
// 这是一个便利函数,直接处理JSON字节
func SimplifyToolsInJSON(jsonData []byte) []byte {
	var data interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return jsonData // 如果不是有效JSON,返回原始数据
	}

	simplifiedData := SimplifyToolsArray(data)

	simplifiedBytes, err := json.Marshal(simplifiedData)
	if err != nil {
		return jsonData // 如果序列化失败,返回原始数据
	}

	return simplifiedBytes
}

// FormatJSONForLog 格式化JSON用于日志输出
// 先简化tools,再截断长文本,最后美化格式
func FormatJSONForLog(data interface{}, maxTextLength int) string {
	// 先简化tools和content数组
	simplified := SimplifyToolsArray(data)
	// 再截断长文本
	truncated := TruncateJSONIntelligently(simplified, maxTextLength)

	// 使用自定义格式化来实现content数组的紧凑显示
	result := formatJSONWithCompactArrays(truncated, "", 0)

	return result
}

// formatMapAsOneLine 将map格式化为单行JSON
func formatMapAsOneLine(m map[string]interface{}) string {
	if len(m) == 0 {
		return "{}"
	}

	var pairs []string
	// 按照特定顺序输出字段（type优先，然后其他字段）
	if typeVal, ok := m["type"]; ok {
		typeJSON, _ := json.Marshal(typeVal)
		pairs = append(pairs, `"type": `+string(typeJSON))
	}

	// 其他字段按字母顺序
	for k, v := range m {
		if k == "type" {
			continue // 已经处理过
		}
		keyJSON, _ := json.Marshal(k)

		// 对于input字段，使用紧凑的单行显示
		if k == "input" {
			if inputMap, ok := v.(map[string]interface{}); ok {
				valueStr := formatInputMapCompact(inputMap)
				pairs = append(pairs, string(keyJSON)+": "+valueStr)
				continue
			}
		}

		valueJSON, _ := json.Marshal(v)
		pairs = append(pairs, string(keyJSON)+": "+string(valueJSON))
	}

	return "{" + strings.Join(pairs, ", ") + "}"
}

// formatInputMapCompact 将input map紧凑格式化为单行
func formatInputMapCompact(m map[string]interface{}) string {
	if len(m) == 0 {
		return "{}"
	}

	var pairs []string
	for k, v := range m {
		keyJSON, _ := json.Marshal(k)
		valueJSON, _ := json.Marshal(v)
		pairs = append(pairs, string(keyJSON)+": "+string(valueJSON))
	}

	return "{" + strings.Join(pairs, ", ") + "}"
}

// formatMessageAsOneLine 将message对象（包含role和content）格式化为紧凑的一行
// 格式：{role: "user", content: [...]}
func formatMessageAsOneLine(m map[string]interface{}) string {
	var parts []string

	// 先输出role
	if role, ok := m["role"]; ok {
		roleJSON, _ := json.Marshal(role)
		parts = append(parts, `"role": `+string(roleJSON))
	}

	// 再输出content（紧凑格式）
	if content, ok := m["content"]; ok {
		// 如果content是字符串，直接输出
		if contentStr, isString := content.(string); isString {
			contentJSON, _ := json.Marshal(contentStr)
			parts = append(parts, `"content": `+string(contentJSON))
		} else if contentArray, isArray := content.([]interface{}); isArray {
			// content数组已经是紧凑格式，直接格式化
			contentItems := make([]string, len(contentArray))
			for i, item := range contentArray {
				if itemMap, ok := item.(map[string]interface{}); ok {
					contentItems[i] = formatMapAsOneLine(itemMap)
				} else {
					itemJSON, _ := json.Marshal(item)
					contentItems[i] = string(itemJSON)
				}
			}
			parts = append(parts, `"content": [`+strings.Join(contentItems, ", ")+`]`)
		}
	}

	return "{" + strings.Join(parts, ", ") + "}"
}

// formatJSONWithCompactArrays 自定义JSON格式化,对content数组使用紧凑单行显示
func formatJSONWithCompactArrays(data interface{}, indent string, depth int) string {
	switch v := data.(type) {
	case nil:
		return "null"

	case bool:
		if v {
			return "true"
		}
		return "false"

	case float64:
		bytes, _ := json.Marshal(v)
		return string(bytes)

	case string:
		bytes, _ := json.Marshal(v)
		return string(bytes)

	case []interface{}:
		if len(v) == 0 {
			return "[]"
		}

		// 检查是否是已经紧凑化的content数组
		isCompactContent := false
		isToolsArray := false

		if len(v) > 0 {
			// 检查第一个元素判断数组类型
			if firstItem, ok := v[0].(map[string]interface{}); ok {
				if typeVal, ok := firstItem["type"].(string); ok {
					// 如果第一个元素有type字段,且看起来是content项,使用紧凑格式
					if typeVal == "text" || typeVal == "tool_use" || typeVal == "tool_result" || typeVal == "image" {
						isCompactContent = true
					}
				}
			} else if _, ok := v[0].(string); ok {
				// 如果数组元素都是字符串,可能是tools数组（已简化为工具名）
				isToolsArray = true
				// 验证是否所有元素都是字符串
				for _, item := range v {
					if _, ok := item.(string); !ok {
						isToolsArray = false
						break
					}
				}
			}
		}

		if isCompactContent {
			// 紧凑单行显示 - 每个content项压缩为单行
			items := make([]string, len(v))
			for i, item := range v {
				// 将单个content项格式化为单行JSON
				if itemMap, ok := item.(map[string]interface{}); ok {
					compactItem := formatMapAsOneLine(itemMap)
					items[i] = compactItem
				} else {
					items[i] = formatJSONWithCompactArrays(item, "", depth+1)
				}
			}
			return "[\n" + indent + "  " + strings.Join(items, ",\n"+indent+"  ") + "\n" + indent + "]"
		}

		if isToolsArray {
			// tools数组使用紧凑的单行显示
			items := make([]string, len(v))
			for i, item := range v {
				itemJSON, _ := json.Marshal(item)
				items[i] = string(itemJSON)
			}
			// 始终使用单行显示所有工具
			return "[" + strings.Join(items, ", ") + "]"
		}

		// 普通数组的多行显示
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = indent + "  " + formatJSONWithCompactArrays(item, indent+"  ", depth+1)
		}
		return "[\n" + strings.Join(items, ",\n") + "\n" + indent + "]"

	case map[string]interface{}:
		if len(v) == 0 {
			return "{}"
		}

		// 检查是否是message对象（包含role和content字段）
		if _, hasRole := v["role"]; hasRole {
			if _, hasContent := v["content"]; hasContent {
				// 这是一个message对象，使用紧凑的单行显示
				return formatMessageAsOneLine(v)
			}
		}

		// 对于普通map,使用多行显示
		var keys []string
		for k := range v {
			keys = append(keys, k)
		}

		items := make([]string, len(keys))
		for i, k := range keys {
			value := formatJSONWithCompactArrays(v[k], indent+"  ", depth+1)
			keyJSON, _ := json.Marshal(k)
			items[i] = indent + "  " + string(keyJSON) + ": " + value
		}
		return "{\n" + strings.Join(items, ",\n") + "\n" + indent + "}"

	default:
		bytes, _ := json.Marshal(v)
		return string(bytes)
	}
}

// FormatJSONBytesForLog 格式化JSON字节数组用于日志输出
func FormatJSONBytesForLog(jsonData []byte, maxTextLength int) string {
	var data interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		// 如果不是有效JSON,按字符串处理
		str := string(jsonData)
		if len(str) > 500 {
			return str[:500] + "..."
		}
		return str
	}

	return FormatJSONForLog(data, maxTextLength)
}

// MaskSensitiveHeaders 脱敏敏感请求头
func MaskSensitiveHeaders(headers map[string]string) map[string]string {
	sensitiveKeys := map[string]bool{
		"authorization": true,
		"x-api-key":     true,
		"x-goog-api-key": true,
	}

	masked := make(map[string]string, len(headers))
	for key, value := range headers {
		lowerKey := strings.ToLower(key)
		if sensitiveKeys[lowerKey] {
			if lowerKey == "authorization" && strings.HasPrefix(value, "Bearer ") {
				token := value[7:]
				masked[key] = "Bearer " + MaskAPIKey(token)
			} else {
				masked[key] = MaskAPIKey(value)
			}
		} else {
			masked[key] = value
		}
	}
	return masked
}

// MaskAPIKey 掩码API密钥
func MaskAPIKey(key string) string {
	if key == "" {
		return ""
	}

	length := len(key)
	if length <= 10 {
		if length <= 5 {
			return "***"
		}
		return key[:3] + "***" + key[length-2:]
	}

	return key[:8] + "***" + key[length-5:]
}
