// Package common 提供 handlers 模块的公共功能
package common

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
)

// FailoverError 封装故障转移错误信息
type FailoverError struct {
	Status int
	Body   []byte
}

// ShouldRetryWithNextKey 判断是否应该使用下一个密钥重试
// 返回: (shouldFailover bool, isQuotaRelated bool)
//
// apiType: 接口类型（Messages/Responses/Gemini），用于日志标签前缀
// fuzzyMode: 启用时，所有非 2xx 错误都触发 failover（模糊处理错误类型）
//
// HTTP 状态码分类策略（非 fuzzy 模式）：
//   - 4xx 客户端错误：部分应触发 failover（密钥/配额问题）
//   - 5xx 服务端错误：应触发 failover（上游临时故障）
//   - 2xx/3xx：不应触发 failover（成功或重定向）
//
// isQuotaRelated 标记用于调度器优先级调整：
//   - true: 额度/配额相关，降低密钥优先级
//   - false: 临时错误，不影响优先级
func ShouldRetryWithNextKey(statusCode int, bodyBytes []byte, fuzzyMode bool, apiType string) (bool, bool) {
	log.Printf("[%s-Failover-Entry] ShouldRetryWithNextKey 入口: statusCode=%d, bodyLen=%d, fuzzyMode=%v",
		apiType, statusCode, len(bodyBytes), fuzzyMode)
	if fuzzyMode {
		return shouldRetryWithNextKeyFuzzy(statusCode, bodyBytes, apiType)
	}
	return shouldRetryWithNextKeyNormal(statusCode, bodyBytes, apiType)
}

// shouldRetryWithNextKeyFuzzy Fuzzy 模式：所有非 2xx 错误都尝试 failover
// 同时检查消息体中的配额相关关键词，确保 403 + "预扣费额度" 等情况能正确识别
// 但对于内容审核等不可重试错误，即使在 Fuzzy 模式下也不应重试
func shouldRetryWithNextKeyFuzzy(statusCode int, bodyBytes []byte, apiType string) (bool, bool) {
	log.Printf("[%s-Failover-Fuzzy] 进入 Fuzzy 模式处理: statusCode=%d, bodyLen=%d", apiType, statusCode, len(bodyBytes))
	if statusCode >= 200 && statusCode < 300 {
		return false, false
	}

	// 检查是否为不可重试错误（内容审核等）
	if len(bodyBytes) > 0 {
		if isNonRetryableError(bodyBytes) {
			log.Printf("[%s-Failover-Fuzzy] 检测到不可重试错误，不进行 failover", apiType)
			return false, false
		}
	}

	// 状态码直接标记为配额相关
	if statusCode == 402 || statusCode == 429 {
		log.Printf("[%s-Failover-Fuzzy] 状态码 %d 直接标记为配额相关", apiType, statusCode)
		return true, true
	}

	// 对于其他状态码，检查消息体是否包含配额相关关键词
	// 这样 403 + "预扣费额度" 消息 → isQuotaRelated=true
	if len(bodyBytes) > 0 {
		_, msgQuota := classifyByErrorMessage(bodyBytes, apiType)
		if msgQuota {
			log.Printf("[%s-Failover-Fuzzy] 消息体包含配额相关关键词，标记为配额相关", apiType)
			return true, true
		}
	}

	log.Printf("[%s-Failover-Fuzzy] Fuzzy 模式结果: shouldFailover=true, isQuotaRelated=false", apiType)
	return true, false
}

// shouldRetryWithNextKeyNormal 原有的精确错误分类逻辑
func shouldRetryWithNextKeyNormal(statusCode int, bodyBytes []byte, apiType string) (bool, bool) {
	// 先检查是否为不可重试错误（内容审核等），这类错误无论状态码如何都不应重试
	if len(bodyBytes) > 0 && isNonRetryableError(bodyBytes) {
		log.Printf("[%s-Failover-Debug] 检测到不可重试错误，不进行 failover", apiType)
		return false, false
	}

	shouldFailover, isQuotaRelated := classifyByStatusCode(statusCode)

	log.Printf("[%s-Failover-Debug] shouldRetryWithNextKeyNormal: statusCode=%d, bodyLen=%d, shouldFailover=%v, isQuotaRelated=%v",
		apiType, statusCode, len(bodyBytes), shouldFailover, isQuotaRelated)

	if shouldFailover {
		// 如果状态码已标记为 quota 相关，直接返回
		if isQuotaRelated {
			return true, true
		}
		// 否则，仍检查消息体是否包含 quota 相关关键词
		// 这样 403 + "预扣费额度" 消息 → isQuotaRelated=true
		log.Printf("[%s-Failover-Debug] 调用 classifyByErrorMessage, body=%s", apiType, string(bodyBytes))
		_, msgQuota := classifyByErrorMessage(bodyBytes, apiType)
		log.Printf("[%s-Failover-Debug] classifyByErrorMessage 返回: msgQuota=%v", apiType, msgQuota)
		if msgQuota {
			return true, true
		}
		return true, false
	}

	// statusCode 不触发 failover 时，完全依赖消息体判断
	return classifyByErrorMessage(bodyBytes, apiType)
}

// classifyByStatusCode 基于 HTTP 状态码分类
func classifyByStatusCode(statusCode int) (bool, bool) {
	switch {
	// 认证/授权错误 (应 failover，非配额相关)
	case statusCode == 401:
		return true, false
	case statusCode == 403:
		return true, false

	// 配额/计费错误 (应 failover，配额相关)
	case statusCode == 402:
		return true, true
	case statusCode == 429:
		return true, true

	// 超时错误 (应 failover，非配额相关)
	case statusCode == 408:
		return true, false

	// 需要检查消息体的状态码 (交给第二层判断)
	case statusCode == 400:
		return false, false

	// 请求错误 (不应 failover，客户端问题)
	case statusCode == 404, statusCode == 405, statusCode == 406,
		statusCode == 409, statusCode == 410, statusCode == 411,
		statusCode == 412, statusCode == 413, statusCode == 414,
		statusCode == 415, statusCode == 416, statusCode == 417,
		statusCode == 422, statusCode == 423, statusCode == 424,
		statusCode == 426, statusCode == 428, statusCode == 431,
		statusCode == 451:
		return false, false

	// 服务端错误 (应 failover，非配额相关)
	case statusCode >= 500:
		return true, false

	// 其他 4xx (保守处理，不 failover)
	case statusCode >= 400 && statusCode < 500:
		return false, false

	// 成功/重定向 (不应 failover)
	default:
		return false, false
	}
}

// classifyByErrorMessage 基于错误消息内容分类
func classifyByErrorMessage(bodyBytes []byte, apiType string) (bool, bool) {
	var errResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &errResp); err != nil {
		log.Printf("[%s-Failover-Debug] JSON解析失败: %v, body长度=%d", apiType, err, len(bodyBytes))
		return false, false
	}

	errObj, ok := errResp["error"].(map[string]interface{})
	if !ok {
		log.Printf("[%s-Failover-Debug] 未找到error对象, keys=%v", apiType, getMapKeys(errResp))
		return false, false
	}

	// 检查 error.code 字段，某些错误码不应重试（内容审核、无效请求等）
	if errCode, ok := errObj["code"].(string); ok {
		if isNonRetryableErrorCode(errCode) {
			log.Printf("[%s-Failover-Debug] 检测到不可重试错误码: %s", apiType, errCode)
			return false, false
		}
	}

	// 尝试多个可能的消息字段: message, upstream_error, detail
	messageFields := []string{"message", "upstream_error", "detail"}
	for _, field := range messageFields {
		if msg, ok := errObj[field].(string); ok {
			log.Printf("[%s-Failover-Debug] 提取到消息 (字段: %s): %s", apiType, field, msg)
			if failover, quota := classifyMessage(msg); failover {
				log.Printf("[%s-Failover-Debug] 消息分类结果: failover=%v, quota=%v", apiType, failover, quota)
				return true, quota
			}
		}
	}

	// 如果 upstream_error 是嵌套对象，尝试提取其中的消息
	if upstreamErr, ok := errObj["upstream_error"].(map[string]interface{}); ok {
		if msg, ok := upstreamErr["message"].(string); ok {
			log.Printf("[%s-Failover-Debug] 提取到嵌套 upstream_error.message: %s", apiType, msg)
			if failover, quota := classifyMessage(msg); failover {
				log.Printf("[%s-Failover-Debug] 消息分类结果: failover=%v, quota=%v", apiType, failover, quota)
				return true, quota
			}
		}
	}

	// 检查 type 字段
	if errType, ok := errObj["type"].(string); ok {
		if failover, quota := classifyErrorType(errType); failover {
			return true, quota
		}
	}

	log.Printf("[%s-Failover-Debug] 未匹配任何关键词, errObj keys=%v", apiType, getMapKeys(errObj))
	return false, false
}

// classifyMessage 基于错误消息内容分类
func classifyMessage(msg string) (bool, bool) {
	msgLower := strings.ToLower(msg)

	// 配额/余额相关关键词 (failover + quota)
	quotaKeywords := []string{
		"insufficient", "quota", "credit", "balance",
		"rate limit", "limit exceeded", "exceeded",
		"billing", "payment", "subscription",
		"积分不足", "余额不足", "请求数限制", "额度", "预扣费",
	}
	for _, keyword := range quotaKeywords {
		if strings.Contains(msgLower, keyword) {
			return true, true
		}
	}

	// 认证/授权相关关键词 (failover + 非 quota)
	authKeywords := []string{
		"invalid", "unauthorized", "authentication",
		"api key", "apikey", "token", "expired",
		"permission", "forbidden", "denied",
		"密钥无效", "认证失败", "权限不足",
	}
	for _, keyword := range authKeywords {
		if strings.Contains(msgLower, keyword) {
			return true, false
		}
	}

	// 临时错误关键词 (failover + 非 quota)
	transientKeywords := []string{
		"timeout", "timed out", "temporarily",
		"overloaded", "unavailable", "retry",
		"server error", "internal error",
		"超时", "暂时", "重试",
	}
	for _, keyword := range transientKeywords {
		if strings.Contains(msgLower, keyword) {
			return true, false
		}
	}

	return false, false
}

// classifyErrorType 基于错误类型分类
func classifyErrorType(errType string) (bool, bool) {
	typeLower := strings.ToLower(errType)

	// 配额相关的错误类型 (failover + quota)
	quotaTypes := []string{
		"over_quota", "quota_exceeded", "rate_limit",
		"billing", "insufficient", "payment",
	}
	for _, t := range quotaTypes {
		if strings.Contains(typeLower, t) {
			return true, true
		}
	}

	// 认证相关的错误类型 (failover + 非 quota)
	authTypes := []string{
		"authentication", "authorization", "permission",
		"invalid_api_key", "invalid_token", "expired",
	}
	for _, t := range authTypes {
		if strings.Contains(typeLower, t) {
			return true, false
		}
	}

	// 服务端错误类型 (failover + 非 quota)
	serverTypes := []string{
		"server_error", "internal_error", "service_unavailable",
		"timeout", "overloaded",
	}
	for _, t := range serverTypes {
		if strings.Contains(typeLower, t) {
			return true, false
		}
	}

	return false, false
}

// HandleAllChannelsFailed 处理所有渠道都失败的情况
// fuzzyMode: 是否启用模糊模式（返回通用错误）
// lastFailoverError: 最后一个故障转移错误
// lastError: 最后一个错误
// apiType: API 类型（用于错误消息）
func HandleAllChannelsFailed(c *gin.Context, fuzzyMode bool, lastFailoverError *FailoverError, lastError error, apiType string) {
	// Fuzzy 模式下返回通用错误，不透传上游详情
	if fuzzyMode {
		c.JSON(503, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "service_unavailable",
				"message": "All upstream channels are currently unavailable",
			},
		})
		return
	}

	// 非 Fuzzy 模式：透传最后一个错误的详情
	if lastFailoverError != nil {
		status := lastFailoverError.Status
		if status == 0 {
			status = 503
		}
		var errBody map[string]interface{}
		if err := json.Unmarshal(lastFailoverError.Body, &errBody); err == nil {
			c.JSON(status, errBody)
		} else {
			c.JSON(status, gin.H{"error": string(lastFailoverError.Body)})
		}
	} else {
		errMsg := "所有渠道都不可用"
		if lastError != nil {
			errMsg = lastError.Error()
		}
		c.JSON(503, gin.H{
			"error":   "所有" + apiType + "渠道都不可用",
			"details": errMsg,
		})
	}
}

// HandleAllKeysFailed 处理所有密钥都失败的情况（单渠道模式）
func HandleAllKeysFailed(c *gin.Context, fuzzyMode bool, lastFailoverError *FailoverError, lastError error, apiType string) {
	// Fuzzy 模式下返回通用错误
	if fuzzyMode {
		c.JSON(503, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "service_unavailable",
				"message": "All upstream channels are currently unavailable",
			},
		})
		return
	}

	// 非 Fuzzy 模式：透传最后一个错误的详情
	if lastFailoverError != nil {
		status := lastFailoverError.Status
		if status == 0 {
			status = 500
		}
		var errBody map[string]interface{}
		if err := json.Unmarshal(lastFailoverError.Body, &errBody); err == nil {
			c.JSON(status, errBody)
		} else {
			c.JSON(status, gin.H{"error": string(lastFailoverError.Body)})
		}
	} else {
		errMsg := "未知错误"
		if lastError != nil {
			errMsg = lastError.Error()
		}
		c.JSON(500, gin.H{
			"error":   "所有上游" + apiType + "API密钥都不可用",
			"details": errMsg,
		})
	}
}

// getMapKeys 获取 map 的所有 key（用于调试日志）
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// isNonRetryableErrorCode 判断错误码是否不应重试
// 这些错误与请求内容相关，换 Key 重试不会改变结果
func isNonRetryableErrorCode(code string) bool {
	nonRetryableCodes := []string{
		// 内容审核相关
		"sensitive_words_detected",
		"content_policy_violation",
		"content_filter",
		"content_blocked",
		"moderation_blocked",
		// 请求内容无效
		"invalid_request",
		"invalid_request_error",
		"bad_request",
	}
	codeLower := strings.ToLower(code)
	for _, c := range nonRetryableCodes {
		if codeLower == c {
			return true
		}
	}
	return false
}

// isNonRetryableError 检查响应体是否包含不可重试的错误码
func isNonRetryableError(bodyBytes []byte) bool {
	var errResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &errResp); err != nil {
		return false
	}
	errObj, ok := errResp["error"].(map[string]interface{})
	if !ok {
		return false
	}
	if errCode, ok := errObj["code"].(string); ok {
		return isNonRetryableErrorCode(errCode)
	}
	return false
}
