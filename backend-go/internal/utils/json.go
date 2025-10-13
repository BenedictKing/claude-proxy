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
			result[key] = SimplifyToolsArray(value)
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
	// 先简化tools数组
	simplified := SimplifyToolsArray(data)
	// 再截断长文本
	truncated := TruncateJSONIntelligently(simplified, maxTextLength)

	// 美化输出
	formatted, err := json.MarshalIndent(truncated, "", "  ")
	if err != nil {
		// 如果格式化失败,尝试普通序列化
		if plain, err := json.Marshal(truncated); err == nil {
			str := string(plain)
			if len(str) > 500 {
				return str[:500] + "..."
			}
			return str
		}
		return "[无法格式化JSON]"
	}

	result := string(formatted)
	// 如果格式化后仍然太长,截断
	if len(result) > 5000 {
		return result[:5000] + "\n... (输出已截断)"
	}
	return result
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
