package types

// ClaudeRequest Claude 请求结构
type ClaudeRequest struct {
	Model       string          `json:"model"`
	Messages    []ClaudeMessage `json:"messages"`
	System      interface{}     `json:"system,omitempty"` // string 或 content 数组
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []ClaudeTool    `json:"tools,omitempty"`
}

// ClaudeMessage Claude 消息
type ClaudeMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string 或 content 数组
}

// ClaudeContent Claude 内容块
type ClaudeContent struct {
	Type      string      `json:"type"` // text, tool_use, tool_result
	Text      string      `json:"text,omitempty"`
	ID        string      `json:"id,omitempty"`
	Name      string      `json:"name,omitempty"`
	Input     interface{} `json:"input,omitempty"`
	ToolUseID string      `json:"tool_use_id,omitempty"`
}

// ClaudeTool Claude 工具定义
type ClaudeTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema"`
}

// ClaudeResponse Claude 响应
type ClaudeResponse struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Role       string          `json:"role"`
	Content    []ClaudeContent `json:"content"`
	StopReason string          `json:"stop_reason,omitempty"`
	Usage      *Usage          `json:"usage,omitempty"`
}

// OpenAIRequest OpenAI 请求结构
type OpenAIRequest struct {
	Model               string          `json:"model"`
	Messages            []OpenAIMessage `json:"messages"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         float64         `json:"temperature,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	Tools               []OpenAITool    `json:"tools,omitempty"`
	ToolChoice          string          `json:"tool_choice,omitempty"`
}

// OpenAIMessage OpenAI 消息
type OpenAIMessage struct {
	Role       string          `json:"role"`
	Content    interface{}     `json:"content"` // string 或 null
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// OpenAIToolCall OpenAI 工具调用
type OpenAIToolCall struct {
	ID       string                `json:"id"`
	Type     string                `json:"type"`
	Function OpenAIToolCallFunction `json:"function"`
}

// OpenAIToolCallFunction OpenAI 工具调用函数
type OpenAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAITool OpenAI 工具定义
type OpenAITool struct {
	Type     string                `json:"type"`
	Function OpenAIToolFunction    `json:"function"`
}

// OpenAIToolFunction OpenAI 工具函数
type OpenAIToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters"`
}

// OpenAIResponse OpenAI 响应
type OpenAIResponse struct {
	ID      string          `json:"id"`
	Choices []OpenAIChoice  `json:"choices"`
	Usage   *Usage          `json:"usage,omitempty"`
}

// OpenAIChoice OpenAI 选择
type OpenAIChoice struct {
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason,omitempty"`
}

// Usage 使用情况统计
type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	PromptTokens int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
}

// ProviderRequest 提供商请求（通用）
type ProviderRequest struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    interface{}
}

// ProviderResponse 提供商响应（通用）
type ProviderResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
	Stream     bool
}
