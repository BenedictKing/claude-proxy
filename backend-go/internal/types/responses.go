package types

// ============== Responses API 类型定义 ==============

// ResponsesRequest Responses API 请求
type ResponsesRequest struct {
	Model              string      `json:"model"`
	Input              interface{} `json:"input"` // string 或 []ResponsesItem
	PreviousResponseID string      `json:"previous_response_id,omitempty"`
	Store              *bool       `json:"store,omitempty"`              // 默认 true
	MaxTokens          int         `json:"max_tokens,omitempty"`         // 最大 tokens
	Temperature        float64     `json:"temperature,omitempty"`        // 温度参数
	TopP               float64     `json:"top_p,omitempty"`              // top_p 参数
	FrequencyPenalty   float64     `json:"frequency_penalty,omitempty"`  // 频率惩罚
	PresencePenalty    float64     `json:"presence_penalty,omitempty"`   // 存在惩罚
	Stream             bool        `json:"stream,omitempty"`             // 是否流式输出
	Stop               interface{} `json:"stop,omitempty"`               // 停止序列 (string 或 []string)
	User               string      `json:"user,omitempty"`               // 用户标识
	StreamOptions      interface{} `json:"stream_options,omitempty"`     // 流式选项
}

// ResponsesItem Responses API 消息项
type ResponsesItem struct {
	Type    string      `json:"type"`    // text, tool_call, tool_result
	Content interface{} `json:"content"` // 根据 type 不同而变化
	ToolUse *ToolUse    `json:"tool_use,omitempty"`
}

// ToolUse 工具使用定义
type ToolUse struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Input interface{} `json:"input"`
}

// ResponsesResponse Responses API 响应
type ResponsesResponse struct {
	ID         string          `json:"id"`
	Model      string          `json:"model"`
	Output     []ResponsesItem `json:"output"`
	Status     string          `json:"status"` // completed, failed
	PreviousID string          `json:"previous_id,omitempty"`
	Usage      ResponsesUsage  `json:"usage"`
	Created    int64           `json:"created,omitempty"`
}

// ResponsesUsage Responses API 使用统计
type ResponsesUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ResponsesStreamEvent Responses API 流式事件
type ResponsesStreamEvent struct {
	ID         string          `json:"id,omitempty"`
	Model      string          `json:"model,omitempty"`
	Output     []ResponsesItem `json:"output,omitempty"`
	Status     string          `json:"status,omitempty"`
	PreviousID string          `json:"previous_id,omitempty"`
	Usage      *ResponsesUsage `json:"usage,omitempty"`
	Type       string          `json:"type,omitempty"` // delta, done
	Delta      *ResponsesDelta `json:"delta,omitempty"`
}

// ResponsesDelta 流式增量数据
type ResponsesDelta struct {
	Type    string      `json:"type,omitempty"`
	Content interface{} `json:"content,omitempty"`
}
