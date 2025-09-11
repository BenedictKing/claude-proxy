export interface JsonSchema {
    type?: 'object' | 'string' | 'number' | 'boolean' | 'array'
    properties?: { [key: string]: JsonSchema }
    required?: string[]
    description?: string
    items?: JsonSchema
}

export interface ClaudeTool {
    name: string
    description: string
    input_schema: JsonSchema
}

export type ClaudeContent =
    | string
    | Array<
          | { type: 'text'; text: string }
          | { type: 'tool_use'; id: string; name: string; input: any }
          | { type: 'tool_result'; tool_use_id: string; content: string; is_error?: boolean }
      >

export interface ClaudeMessage {
    role: 'user' | 'assistant'
    content: ClaudeContent
}

export interface ClaudeRequest {
    model: string
    // Claude v1/messages supports a top-level system prompt. Some clients (e.g., Claude Code)
    // rely on this to instruct the model to use tools. We surface it so providers can forward it.
    system?: string | Array<{ type: 'text'; text: string; cache_control?: { type: 'ephemeral' } }>
    messages: ClaudeMessage[]
    max_tokens?: number
    temperature?: number
    stream?: boolean
    tools?: ClaudeTool[]
}

export interface ClaudeResponse {
    id: string
    type: 'message'
    role: 'assistant'
    content: Array<{ type: 'text'; text: string } | { type: 'tool_use'; id: string; name: string; input: any }>
    stop_reason?: 'end_turn' | 'tool_use' | 'max_tokens'
    usage?: {
        input_tokens: number
        output_tokens: number
    }
}

export interface GeminiFunctionDeclaration {
    name: string
    description: string
    parameters: JsonSchema
}

export interface GeminiTool {
    functionDeclarations: GeminiFunctionDeclaration[]
}

export type GeminiPart =
    | { text: string }
    | { functionCall: { name: string; args: any } }
    | { functionResponse: { name: string; response: any } }

export interface GeminiContent {
    parts: GeminiPart[]
    role?: 'user' | 'model' | 'tool'
}

export interface GeminiRequest {
    model?: string
    contents: GeminiContent[]
    tools?: GeminiTool[]
    generationConfig?: {
        temperature?: number
        maxOutputTokens?: number
    }
}

export interface GeminiCandidate {
    content: {
        parts: GeminiPart[]
        role: 'model'
    }
    finishReason?: string
}

export interface GeminiResponse {
    candidates: GeminiCandidate[]
    usageMetadata?: {
        promptTokenCount: number
        candidatesTokenCount: number
        totalTokenCount: number
    }
}

export interface ClaudeStreamEvent {
    type:
        | 'message_start'
        | 'content_block_start'
        | 'content_block_delta'
        | 'content_block_stop'
        | 'message_delta'
        | 'message_stop'
    message?: Partial<ClaudeResponse>
    content_block?: {
        type: 'text' | 'tool_use'
        text?: string
        id?: string
        name?: string
        input?: any
    }
    delta?: {
        type: 'text_delta' | 'input_json_delta'
        text?: string
        partial_json?: string
    }
    index?: number
    usage?: {
        input_tokens: number
        output_tokens: number
    }
}

export interface OpenAIMessage {
    role: 'system' | 'user' | 'assistant' | 'tool'
    content?: string | null
    tool_calls?: OpenAIToolCall[]
    tool_call_id?: string
}

export interface OpenAIToolCall {
    id: string
    type: 'function'
    function: {
        name: string
        arguments: string
    }
}

export interface OpenAITool {
    type: 'function'
    function: {
        name: string
        description?: string
        parameters?: any
        strict?: boolean
    }
}

export interface OpenAIRequest {
    model: string
    messages: OpenAIMessage[]
    tools?: OpenAITool[]
    tool_choice?: string | { type: string; function?: { name: string } }
    temperature?: number
    max_tokens?: number
    max_completion_tokens?: number // 新的参数支持
    stream?: boolean
}

export interface OpenAIChoice {
    index: number
    message: OpenAIMessage
    finish_reason: string | null
}

export interface OpenAIResponse {
    id: string
    object: string
    created: number
    model: string
    choices: OpenAIChoice[]
    usage?: {
        prompt_tokens: number
        completion_tokens: number
        total_tokens: number
    }
}

export interface OpenAIStreamChoice {
    index: number
    delta: {
        role?: string
        content?: string
        tool_calls?: Array<{
            index: number
            id?: string
            type?: 'function'
            function?: {
                name?: string
                arguments?: string
            }
        }>
    }
    finish_reason?: string | null
}

export interface OpenAIStreamResponse {
    id: string
    object: string
    created: number
    model: string
    choices: OpenAIStreamChoice[]
}

// Responses API 类型定义
export interface OpenAIResponsesRequest {
    model: string
    input: OpenAIMessage[]
    tools?: OpenAITool[]
    tool_choice?: string | { type: string; function?: { name: string } }
    temperature?: number
    max_tokens?: number // 保留向后兼容
    max_completion_tokens?: number // 新的参数名
    stream?: boolean
    reasoning_effort?: string
    stream_options?: {
        include_usage?: boolean
    }
}

export interface OpenAIResponsesItem {
    type: 'text' | 'function_call' | 'reasoning'
    id?: string
    call_id?: string
    name?: string
    arguments?: string
    content?: string
    text?: string
}

export interface OpenAIResponsesOutput {
    output: OpenAIResponsesItem[]
    usage?: {
        prompt_tokens: number
        completion_tokens: number
        total_tokens: number
    }
}

export interface OpenAIResponsesStreamChoice {
    index: number
    delta: {
        role?: string
        content?: string
        tool_calls?: Array<{
            index: number
            id?: string
            type?: 'function'
            function?: {
                name?: string
                arguments?: string
            }
        }>
    }
    finish_reason?: string | null
}

export interface OpenAIResponsesStreamResponse {
    id: string
    object: string
    created: number
    model: string
    choices: OpenAIResponsesStreamChoice[]
}
