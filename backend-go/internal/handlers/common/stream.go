// Package common 提供 handlers 模块的公共功能
package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/providers"
	"github.com/BenedictKing/claude-proxy/internal/scheduler"
	"github.com/BenedictKing/claude-proxy/internal/types"
	"github.com/BenedictKing/claude-proxy/internal/utils"
	"github.com/gin-gonic/gin"
)

// StreamContext 流处理上下文
type StreamContext struct {
	LogBuffer        bytes.Buffer
	OutputTextBuffer bytes.Buffer
	Synthesizer      *utils.StreamSynthesizer
	LoggingEnabled   bool
	ClientGone       bool
	HasUsage         bool
	NeedTokenPatch   bool
	// 累积的 token 统计
	CollectedUsage CollectedUsageData
}

// CollectedUsageData 从流事件中收集的 usage 数据
type CollectedUsageData struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	// 缓存 TTL 细分
	CacheCreation5mInputTokens int
	CacheCreation1hInputTokens int
	CacheTTL                   string // "5m" | "1h" | "mixed"
}

// NewStreamContext 创建流处理上下文
func NewStreamContext(envCfg *config.EnvConfig) *StreamContext {
	ctx := &StreamContext{
		LoggingEnabled: envCfg.IsDevelopment() && envCfg.EnableResponseLogs,
	}
	if ctx.LoggingEnabled {
		ctx.Synthesizer = utils.NewStreamSynthesizer("claude")
	}
	return ctx
}

// SetupStreamHeaders 设置流式响应头
func SetupStreamHeaders(c *gin.Context, resp *http.Response) {
	utils.ForwardResponseHeaders(resp.Header, c.Writer)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(200)
}

// ProcessStreamEvents 处理流事件循环
// 返回值: error 表示流处理过程中是否发生错误（用于调用方决定是否记录失败指标）
func ProcessStreamEvents(
	c *gin.Context,
	w gin.ResponseWriter,
	flusher http.Flusher,
	eventChan <-chan string,
	errChan <-chan error,
	ctx *StreamContext,
	envCfg *config.EnvConfig,
	startTime time.Time,
	requestBody []byte,
	channelScheduler *scheduler.ChannelScheduler,
	upstream *config.UpstreamConfig,
	apiKey string,
) error {
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				logStreamCompletion(ctx, envCfg, startTime, channelScheduler, upstream, apiKey)
				return nil
			}
			ProcessStreamEvent(c, w, flusher, event, ctx, envCfg, requestBody)

		case err, ok := <-errChan:
			if !ok {
				continue
			}
			if err != nil {
				log.Printf("💥 流式传输错误: %v", err)
				logPartialResponse(ctx, envCfg)

				// 记录失败指标
				channelScheduler.RecordFailure(upstream.BaseURL, apiKey, false)

				// 向客户端发送错误事件（如果连接仍然有效）
				if !ctx.ClientGone {
					errorEvent := BuildStreamErrorEvent(err)
					w.Write([]byte(errorEvent))
					flusher.Flush()
				}

				return err
			}
		}
	}
}

// ProcessStreamEvent 处理单个流事件
func ProcessStreamEvent(
	c *gin.Context,
	w gin.ResponseWriter,
	flusher http.Flusher,
	event string,
	ctx *StreamContext,
	envCfg *config.EnvConfig,
	requestBody []byte,
) {
	// 提取文本用于估算 token
	ExtractTextFromEvent(event, &ctx.OutputTextBuffer)

	// 检测并收集 usage
	hasUsage, needPatch, usageData := CheckEventUsageStatus(event, envCfg.EnableResponseLogs && envCfg.ShouldLog("debug"))
	if hasUsage {
		if !ctx.HasUsage {
			ctx.HasUsage = true
			ctx.NeedTokenPatch = needPatch
			if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") && needPatch && !IsMessageDeltaEvent(event) {
				log.Printf("🔢 [Stream-Token] 检测到虚假值, 延迟到流结束修补")
			}
		}
		// 累积收集 usage 数据
		updateCollectedUsage(&ctx.CollectedUsage, usageData)
	}

	// 日志缓存
	if ctx.LoggingEnabled {
		ctx.LogBuffer.WriteString(event)
		if ctx.Synthesizer != nil {
			for _, line := range strings.Split(event, "\n") {
				ctx.Synthesizer.ProcessLine(line)
			}
		}
	}

	// 在 message_stop 前注入 usage（上游完全没有 usage 的情况）
	if !ctx.HasUsage && !ctx.ClientGone && IsMessageStopEvent(event) {
		usageEvent := BuildUsageEvent(requestBody, ctx.OutputTextBuffer.String())
		if envCfg.EnableResponseLogs && envCfg.ShouldLog("debug") {
			log.Printf("🔢 [Stream-Token注入] 上游无usage, 注入本地估算事件")
		}
		w.Write([]byte(usageEvent))
		flusher.Flush()
		ctx.HasUsage = true
	}

	// 修补 token
	eventToSend := event
	if ctx.NeedTokenPatch && HasEventWithUsage(event) {
		if IsMessageDeltaEvent(event) || IsMessageStopEvent(event) {
			inputTokens := ctx.CollectedUsage.InputTokens
			if inputTokens == 0 {
				inputTokens = utils.EstimateRequestTokens(requestBody)
			}
			outputTokens := ctx.CollectedUsage.OutputTokens
			if outputTokens == 0 {
				outputTokens = utils.EstimateTokens(ctx.OutputTextBuffer.String())
			}
			hasCacheTokens := ctx.CollectedUsage.CacheCreationInputTokens > 0 || ctx.CollectedUsage.CacheReadInputTokens > 0
			eventToSend = PatchTokensInEvent(event, inputTokens, outputTokens, hasCacheTokens, envCfg.EnableResponseLogs && envCfg.ShouldLog("debug"))
			ctx.NeedTokenPatch = false
		}
	}

	// 转发给客户端
	if !ctx.ClientGone {
		if _, err := w.Write([]byte(eventToSend)); err != nil {
			ctx.ClientGone = true
			if !IsClientDisconnectError(err) {
				log.Printf("⚠️ 流式传输写入错误: %v", err)
			} else if envCfg.ShouldLog("info") {
				log.Printf("ℹ️ 客户端中断连接 (正常行为)，继续接收上游数据...")
			}
		} else {
			flusher.Flush()
		}
	}
}

// updateCollectedUsage 更新收集的 usage 数据
func updateCollectedUsage(collected *CollectedUsageData, usageData CollectedUsageData) {
	if usageData.InputTokens > collected.InputTokens {
		collected.InputTokens = usageData.InputTokens
	}
	if usageData.OutputTokens > collected.OutputTokens {
		collected.OutputTokens = usageData.OutputTokens
	}
	if usageData.CacheCreationInputTokens > 0 {
		collected.CacheCreationInputTokens = usageData.CacheCreationInputTokens
	}
	if usageData.CacheReadInputTokens > 0 {
		collected.CacheReadInputTokens = usageData.CacheReadInputTokens
	}
	if usageData.CacheCreation5mInputTokens > 0 {
		collected.CacheCreation5mInputTokens = usageData.CacheCreation5mInputTokens
	}
	if usageData.CacheCreation1hInputTokens > 0 {
		collected.CacheCreation1hInputTokens = usageData.CacheCreation1hInputTokens
	}
	if usageData.CacheTTL != "" {
		collected.CacheTTL = usageData.CacheTTL
	}
}

// logStreamCompletion 记录流完成日志
func logStreamCompletion(ctx *StreamContext, envCfg *config.EnvConfig, startTime time.Time, channelScheduler *scheduler.ChannelScheduler, upstream *config.UpstreamConfig, apiKey string) {
	if envCfg.EnableResponseLogs {
		log.Printf("⏱️ 流式响应完成: %dms", time.Since(startTime).Milliseconds())
	}

	if envCfg.IsDevelopment() {
		logSynthesizedContent(ctx)
	}

	// 将累积的 usage 数据转换为 *types.Usage
	var usage *types.Usage
	hasUsageData := ctx.CollectedUsage.InputTokens > 0 ||
		ctx.CollectedUsage.OutputTokens > 0 ||
		ctx.CollectedUsage.CacheCreationInputTokens > 0 ||
		ctx.CollectedUsage.CacheReadInputTokens > 0 ||
		ctx.CollectedUsage.CacheCreation5mInputTokens > 0 ||
		ctx.CollectedUsage.CacheCreation1hInputTokens > 0
	if hasUsageData {
		usage = &types.Usage{
			InputTokens:                ctx.CollectedUsage.InputTokens,
			OutputTokens:               ctx.CollectedUsage.OutputTokens,
			CacheCreationInputTokens:   ctx.CollectedUsage.CacheCreationInputTokens,
			CacheReadInputTokens:       ctx.CollectedUsage.CacheReadInputTokens,
			CacheCreation5mInputTokens: ctx.CollectedUsage.CacheCreation5mInputTokens,
			CacheCreation1hInputTokens: ctx.CollectedUsage.CacheCreation1hInputTokens,
			CacheTTL:                   ctx.CollectedUsage.CacheTTL,
		}
	}

	// 记录成功指标
	channelScheduler.RecordSuccessWithUsage(upstream.BaseURL, apiKey, usage, false)
}

// logPartialResponse 记录部分响应日志
func logPartialResponse(ctx *StreamContext, envCfg *config.EnvConfig) {
	if envCfg.EnableResponseLogs && envCfg.IsDevelopment() {
		logSynthesizedContent(ctx)
	}
}

// logSynthesizedContent 记录合成内容
func logSynthesizedContent(ctx *StreamContext) {
	if ctx.Synthesizer != nil {
		content := ctx.Synthesizer.GetSynthesizedContent()
		if content != "" && !ctx.Synthesizer.IsParseFailed() {
			log.Printf("🛰️  上游流式响应合成内容:\n%s", strings.TrimSpace(content))
			return
		}
	}
	if ctx.LogBuffer.Len() > 0 {
		log.Printf("🛰️  上游流式响应原始内容:\n%s", ctx.LogBuffer.String())
	}
}

// IsClientDisconnectError 判断是否为客户端断开连接错误
func IsClientDisconnectError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset")
}

// HandleStreamResponse 处理流式响应（Messages API）
func HandleStreamResponse(
	c *gin.Context,
	resp *http.Response,
	provider providers.Provider,
	envCfg *config.EnvConfig,
	startTime time.Time,
	upstream *config.UpstreamConfig,
	requestBody []byte,
	channelScheduler *scheduler.ChannelScheduler,
	apiKey string,
) {
	defer resp.Body.Close()

	eventChan, errChan, err := provider.HandleStreamResponse(resp.Body)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to handle stream response"})
		return
	}

	SetupStreamHeaders(c, resp)

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("⚠️ ResponseWriter不支持Flush接口")
		return
	}
	flusher.Flush()

	ctx := NewStreamContext(envCfg)
	ProcessStreamEvents(c, w, flusher, eventChan, errChan, ctx, envCfg, startTime, requestBody, channelScheduler, upstream, apiKey)
}

// ========== Token 检测和修补相关函数 ==========

// CheckEventUsageStatus 检测事件是否包含 usage 字段
func CheckEventUsageStatus(event string, enableLog bool) (bool, bool, CollectedUsageData) {
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		// 检查顶层 usage 字段
		if hasUsage, needInputPatch, needOutputPatch := checkUsageFieldsWithPatch(data["usage"]); hasUsage {
			needPatch := needInputPatch || needOutputPatch
			var usageData CollectedUsageData
			if usage, ok := data["usage"].(map[string]interface{}); ok {
				if enableLog {
					logUsageDetection("顶层usage", usage, needPatch)
				}
				usageData = extractUsageFromMap(usage)
			}
			return true, needPatch, usageData
		}

		// 检查 message.usage
		if msg, ok := data["message"].(map[string]interface{}); ok {
			if hasUsage, needInputPatch, needOutputPatch := checkUsageFieldsWithPatch(msg["usage"]); hasUsage {
				needPatch := needInputPatch || needOutputPatch
				var usageData CollectedUsageData
				if usage, ok := msg["usage"].(map[string]interface{}); ok {
					if enableLog {
						logUsageDetection("message.usage", usage, needPatch)
					}
					usageData = extractUsageFromMap(usage)
				}
				return true, needPatch, usageData
			}
		}
	}
	return false, false, CollectedUsageData{}
}

// checkUsageFieldsWithPatch 检查 usage 对象是否包含 token 字段
func checkUsageFieldsWithPatch(usage interface{}) (bool, bool, bool) {
	if u, ok := usage.(map[string]interface{}); ok {
		inputTokens, hasInput := u["input_tokens"]
		outputTokens, hasOutput := u["output_tokens"]
		if hasInput || hasOutput {
			needInputPatch := false
			needOutputPatch := false

			cacheCreation, _ := u["cache_creation_input_tokens"].(float64)
			cacheRead, _ := u["cache_read_input_tokens"].(float64)
			hasCacheTokens := cacheCreation > 0 || cacheRead > 0

			if hasInput {
				if v, ok := inputTokens.(float64); ok && v <= 1 && !hasCacheTokens {
					needInputPatch = true
				}
			}
			if hasOutput {
				if v, ok := outputTokens.(float64); ok && v <= 1 {
					needOutputPatch = true
				}
			}
			return true, needInputPatch, needOutputPatch
		}
	}
	return false, false, false
}

// extractUsageFromMap 从 usage map 中提取 token 数据
func extractUsageFromMap(usage map[string]interface{}) CollectedUsageData {
	var data CollectedUsageData

	if v, ok := usage["input_tokens"].(float64); ok {
		data.InputTokens = int(v)
	}
	if v, ok := usage["output_tokens"].(float64); ok {
		data.OutputTokens = int(v)
	}
	if v, ok := usage["cache_creation_input_tokens"].(float64); ok {
		data.CacheCreationInputTokens = int(v)
	}
	if v, ok := usage["cache_read_input_tokens"].(float64); ok {
		data.CacheReadInputTokens = int(v)
	}

	var has5m, has1h bool
	if v, ok := usage["cache_creation_5m_input_tokens"].(float64); ok {
		data.CacheCreation5mInputTokens = int(v)
		has5m = data.CacheCreation5mInputTokens > 0
	}
	if v, ok := usage["cache_creation_1h_input_tokens"].(float64); ok {
		data.CacheCreation1hInputTokens = int(v)
		has1h = data.CacheCreation1hInputTokens > 0
	}

	if has5m && has1h {
		data.CacheTTL = "mixed"
	} else if has1h {
		data.CacheTTL = "1h"
	} else if has5m {
		data.CacheTTL = "5m"
	}

	return data
}

// logUsageDetection 统一格式输出 usage 检测日志
func logUsageDetection(location string, usage map[string]interface{}, needPatch bool) {
	inputTokens := usage["input_tokens"]
	outputTokens := usage["output_tokens"]
	cacheCreation, _ := usage["cache_creation_input_tokens"].(float64)
	cacheRead, _ := usage["cache_read_input_tokens"].(float64)

	log.Printf("🔢 [Stream-Token检测] %s: InputTokens=%v, OutputTokens=%v, CacheCreation=%.0f, CacheRead=%.0f, 需补全=%v",
		location, inputTokens, outputTokens, cacheCreation, cacheRead, needPatch)
}

// HasEventWithUsage 检查事件是否包含 usage 字段
func HasEventWithUsage(event string) bool {
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		if _, ok := data["usage"].(map[string]interface{}); ok {
			return true
		}

		if msg, ok := data["message"].(map[string]interface{}); ok {
			if _, ok := msg["usage"].(map[string]interface{}); ok {
				return true
			}
		}
	}
	return false
}

// PatchTokensInEvent 修补事件中的 token 字段
func PatchTokensInEvent(event string, estimatedInputTokens, estimatedOutputTokens int, hasCacheTokens bool, enableLog bool) string {
	var result strings.Builder
	lines := strings.Split(event, "\n")

	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// 修补顶层 usage
		if usage, ok := data["usage"].(map[string]interface{}); ok {
			patchUsageFieldsWithLog(usage, estimatedInputTokens, estimatedOutputTokens, hasCacheTokens, enableLog, "顶层usage")
		}

		// 修补 message.usage
		if msg, ok := data["message"].(map[string]interface{}); ok {
			if usage, ok := msg["usage"].(map[string]interface{}); ok {
				patchUsageFieldsWithLog(usage, estimatedInputTokens, estimatedOutputTokens, hasCacheTokens, enableLog, "message.usage")
			}
		}

		patchedJSON, err := json.Marshal(data)
		if err != nil {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		result.WriteString("data: ")
		result.Write(patchedJSON)
		result.WriteString("\n")
	}

	return result.String()
}

// patchUsageFieldsWithLog 修补 usage 对象中的 token 字段
func patchUsageFieldsWithLog(usage map[string]interface{}, estimatedInput, estimatedOutput int, hasCacheTokens bool, enableLog bool, location string) {
	originalInput := usage["input_tokens"]
	originalOutput := usage["output_tokens"]
	inputPatched := false
	outputPatched := false

	cacheCreation, _ := usage["cache_creation_input_tokens"].(float64)
	cacheRead, _ := usage["cache_read_input_tokens"].(float64)
	cacheCreation5m, _ := usage["cache_creation_5m_input_tokens"].(float64)
	cacheCreation1h, _ := usage["cache_creation_1h_input_tokens"].(float64)
	cacheTTL, _ := usage["cache_ttl"].(string)

	if v, ok := usage["input_tokens"].(float64); ok {
		currentInput := int(v)
		if !hasCacheTokens && ((currentInput <= 1) || (estimatedInput > currentInput && estimatedInput > 1)) {
			usage["input_tokens"] = estimatedInput
			inputPatched = true
		}
	}

	if v, ok := usage["output_tokens"].(float64); ok {
		currentOutput := int(v)
		if currentOutput <= 1 || (estimatedOutput > currentOutput && estimatedOutput > 1) {
			usage["output_tokens"] = estimatedOutput
			outputPatched = true
		}
	}

	if enableLog {
		if inputPatched || outputPatched {
			log.Printf("🔢 [Stream-Token补全] %s: InputTokens=%v→%v, OutputTokens=%v→%v",
				location, originalInput, usage["input_tokens"], originalOutput, usage["output_tokens"])
		}
		log.Printf("🔢 [Stream-Token统计] %s: InputTokens=%v, OutputTokens=%v, CacheCreationInputTokens=%.0f, CacheReadInputTokens=%.0f, CacheCreation5m=%.0f, CacheCreation1h=%.0f, CacheTTL=%s",
			location, usage["input_tokens"], usage["output_tokens"], cacheCreation, cacheRead, cacheCreation5m, cacheCreation1h, cacheTTL)
	}
}

// BuildStreamErrorEvent 构建流错误 SSE 事件
func BuildStreamErrorEvent(err error) string {
	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "stream_error",
			"message": fmt.Sprintf("Stream processing error: %v", err),
		},
	}
	eventJSON, _ := json.Marshal(errorEvent)
	return fmt.Sprintf("event: error\ndata: %s\n\n", eventJSON)
}

// BuildUsageEvent 构建带 usage 的 message_delta SSE 事件
func BuildUsageEvent(requestBody []byte, outputText string) string {
	inputTokens := utils.EstimateRequestTokens(requestBody)
	outputTokens := utils.EstimateTokens(outputText)

	event := map[string]interface{}{
		"type": "message_delta",
		"usage": map[string]int{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}
	eventJSON, _ := json.Marshal(event)
	return fmt.Sprintf("event: message_delta\ndata: %s\n\n", eventJSON)
}

// IsMessageStopEvent 检测是否为 message_stop 事件
func IsMessageStopEvent(event string) bool {
	if strings.Contains(event, "event: message_stop") {
		return true
	}

	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		if data["type"] == "message_stop" {
			return true
		}
	}
	return false
}

// IsMessageDeltaEvent 检测是否为 message_delta 事件
func IsMessageDeltaEvent(event string) bool {
	if strings.Contains(event, "event: message_delta") {
		return true
	}
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}
		if data["type"] == "message_delta" {
			return true
		}
	}
	return false
}

// ExtractTextFromEvent 从 SSE 事件中提取文本内容
func ExtractTextFromEvent(event string, buf *bytes.Buffer) {
	for _, line := range strings.Split(event, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		// Claude SSE: delta.text
		if delta, ok := data["delta"].(map[string]interface{}); ok {
			if text, ok := delta["text"].(string); ok {
				buf.WriteString(text)
			}
			if partialJSON, ok := delta["partial_json"].(string); ok {
				buf.WriteString(partialJSON)
			}
		}

		// content_block_start 中的初始文本
		if cb, ok := data["content_block"].(map[string]interface{}); ok {
			if text, ok := cb["text"].(string); ok {
				buf.WriteString(text)
			}
		}
	}
}
