import * as types from './types'
import * as provider from './provider'
import * as utils from './utils'
import { redirectModel } from './config'

export class impl implements provider.Provider {
    async convertToProviderRequest(request: Request, baseUrl: string, apiKey: string, upstream?: import('./config').UpstreamConfig): Promise<Request> {
        const claudeRequest = (await request.json()) as types.ClaudeRequest
        const openaiRequest = this.convertToOpenAIRequestBody(claudeRequest, upstream)

        const finalUrl = utils.buildUrl(baseUrl, 'chat/completions')

        const headers = new Headers()
        // 只复制必要的头，排除授权相关的头
        // request.headers.forEach((value, key) => {
        //     const lowerKey = key.toLowerCase()
        //     if (lowerKey !== 'authorization' && lowerKey !== 'x-api-key' && lowerKey !== 'host') {
        //         headers.set(key, value)
        //     }
        // })
        headers.set('Authorization', `Bearer ${apiKey}`)
        headers.set('Content-Type', 'application/json')

        return new Request(finalUrl, {
            method: 'POST',
            headers,
            body: JSON.stringify(openaiRequest)
        })
    }

    async convertToClaudeResponse(openaiResponse: Response): Promise<Response> {
        if (!openaiResponse.ok) {
            return openaiResponse
        }

        const contentType = openaiResponse.headers.get('content-type') || ''
        const isStream = contentType.includes('text/event-stream')

        if (isStream) {
            return this.convertStreamResponse(openaiResponse)
        } else {
            return this.convertNormalResponse(openaiResponse)
        }
    }

    // 兼容各种来源的 role 值（如 'tools'、'system' 等）
    private normalizeClaudeRole(role: any): 'system' | 'user' | 'assistant' | 'tool' {
        const r = String(role ?? '').toLowerCase()
        if (r === 'assistant') return 'assistant'
        if (r === 'system') return 'system'
        if (r === 'tool' || r === 'tools') return 'tool'
        return 'user'
    }

    // 支持 Claude 顶层 system 为 string 或 content[]（含 {type:'text'} 块）
    private extractSystemText(systemField: any): string | undefined {
        if (!systemField) return undefined
        if (typeof systemField === 'string') return systemField
        // 可能是单个对象或数组
        const arr = Array.isArray(systemField) ? systemField : [systemField]
        const parts: string[] = []
        for (const item of arr) {
            if (item && typeof item === 'object' && item.type === 'text' && typeof item.text === 'string') {
                parts.push(item.text)
            }
        }
        if (parts.length === 0) return undefined
        return parts.join('\n')
    }

    private convertToOpenAIRequestBody(claudeRequest: types.ClaudeRequest, upstream?: import('./config').UpstreamConfig): types.OpenAIRequest {
        const convertedMessages = this.convertMessages(claudeRequest.messages)
        const systemText = this.extractSystemText((claudeRequest as any).system)
        const messages: types.OpenAIMessage[] = systemText
            ? [{ role: 'system', content: systemText }, ...convertedMessages]
            : convertedMessages

        // 应用模型重定向
        const finalModel = upstream ? redirectModel(claudeRequest.model, upstream) : claudeRequest.model
        
        const openaiRequest: types.OpenAIRequest = {
            model: finalModel,
            messages,
            stream: claudeRequest.stream
        }

        if (claudeRequest.tools && claudeRequest.tools.length > 0) {
            openaiRequest.tools = claudeRequest.tools.map(tool => ({
                type: 'function',
                function: {
                    name: tool.name,
                    description: tool.description,
                    parameters: utils.cleanJsonSchema(tool.input_schema)
                }
            }))
            openaiRequest.tool_choice = 'auto'
        }

        if (claudeRequest.temperature !== undefined) {
            openaiRequest.temperature = claudeRequest.temperature
        }

        if (claudeRequest.max_tokens !== undefined) {
            // 使用新版字段以兼容 o4 系列及新接口行为
            openaiRequest.max_completion_tokens = claudeRequest.max_tokens
        }

        return openaiRequest
    }

    private convertMessages(claudeMessages: types.ClaudeMessage[]): types.OpenAIMessage[] {
        const openaiMessages: types.OpenAIMessage[] = []
        const toolCallMap = new Map<string, string>()

        for (const message of claudeMessages) {
            const normalizedRole = this.normalizeClaudeRole((message as any).role)
            if (typeof message.content === 'string') {
                // 纯文本消息：支持 system/user/assistant；tool 角色的纯文本忽略
                if (normalizedRole !== 'tool') {
                    openaiMessages.push({
                        role: normalizedRole,
                        content: message.content
                    })
                }
                continue
            }

            const textContents: string[] = []
            const toolCalls: types.OpenAIToolCall[] = []
            const toolResults: Array<{ tool_call_id: string; content: string }> = []

            for (const content of message.content) {
                switch (content.type) {
                    case 'text':
                        textContents.push(content.text)
                        break
                    case 'tool_use':
                        toolCallMap.set(content.id, content.id)
                        toolCalls.push({
                            id: content.id,
                            type: 'function',
                            function: {
                                name: content.name,
                                arguments: JSON.stringify(content.input)
                            }
                        })
                        break
                    case 'tool_result':
                        toolResults.push({
                            tool_call_id: content.tool_use_id,
                            content:
                                typeof content.content === 'string' ? content.content : JSON.stringify(content.content)
                        })
                        break
                }
            }

            // 优先推送 tool_result，确保紧跟在上一次 assistant 的 tool_calls 之后
            for (const toolResult of toolResults) {
                openaiMessages.push({
                    role: 'tool',
                    tool_call_id: toolResult.tool_call_id,
                    content: toolResult.content
                })
            }

            if ((textContents.length > 0 || toolCalls.length > 0) && normalizedRole !== 'tool') {
                const openaiMessage: types.OpenAIMessage = {
                    role: normalizedRole === 'assistant' ? 'assistant' : normalizedRole === 'system' ? 'system' : 'user',
                    content: textContents.length > 0 ? textContents.join('\n') : null
                }

                if (toolCalls.length > 0) {
                    openaiMessage.tool_calls = toolCalls
                }

                openaiMessages.push(openaiMessage)
            }
        }

        return openaiMessages
    }

    private async convertNormalResponse(openaiResponse: Response): Promise<Response> {
        const openaiData = (await openaiResponse.json()) as types.OpenAIResponse

        const claudeResponse: types.ClaudeResponse = {
            id: utils.generateId(),
            type: 'message',
            role: 'assistant',
            content: []
        }

        if (openaiData.choices && openaiData.choices.length > 0) {
            const choice = openaiData.choices[0]
            const message = choice.message

            if (message.content) {
                claudeResponse.content.push({
                    type: 'text',
                    text: message.content
                })
            }

            if (message.tool_calls) {
                for (const toolCall of message.tool_calls) {
                    claudeResponse.content.push({
                        type: 'tool_use',
                        id: toolCall.id,
                        name: toolCall.function.name,
                        input: JSON.parse(toolCall.function.arguments)
                    })
                }
                claudeResponse.stop_reason = 'tool_use'
            } else if (choice.finish_reason === 'length') {
                claudeResponse.stop_reason = 'max_tokens'
            } else {
                claudeResponse.stop_reason = 'end_turn'
            }
        }

        if (openaiData.usage) {
            claudeResponse.usage = {
                input_tokens: openaiData.usage.prompt_tokens,
                output_tokens: openaiData.usage.completion_tokens
            }
        }

        return new Response(JSON.stringify(claudeResponse), {
            status: openaiResponse.status,
            headers: {
                'Content-Type': 'application/json'
            }
        })
    }

    private async convertStreamResponse(openaiResponse: Response): Promise<Response> {
        // 用于累积工具调用数据
        const toolCallAccumulator = new Map<number, { id?: string; name?: string; arguments?: string }>()
        // 仅在确认 OpenAI 本轮以 tool_calls 结束时，向下游发送一次 stop_reason=tool_use
        let toolUseStopEmitted = false
        
        return utils.processProviderStream(openaiResponse, (jsonStr, textBlockIndex, toolUseBlockIndex) => {
            const openaiData = JSON.parse(jsonStr) as types.OpenAIStreamResponse
            if (!openaiData.choices || openaiData.choices.length === 0) {
                return null
            }

            const choice = openaiData.choices[0]
            const delta = choice.delta
            const events: string[] = []
            let currentTextIndex = textBlockIndex
            let currentToolIndex = toolUseBlockIndex

            if (delta.content) {
                events.push(...utils.processTextPart(delta.content, currentTextIndex))
                currentTextIndex++
            }

            if (delta.tool_calls) {
                for (const toolCall of delta.tool_calls) {
                    const toolIndex = toolCall.index ?? 0

                    // 获取或创建工具调用累积器
                    if (!toolCallAccumulator.has(toolIndex)) {
                        toolCallAccumulator.set(toolIndex, {})
                    }
                    const accumulated = toolCallAccumulator.get(toolIndex)!

                    // 累积数据
                    if (toolCall.id) {
                        accumulated.id = toolCall.id
                    }
                    if (toolCall.function?.name) {
                        accumulated.name = toolCall.function.name
                    }
                    if (toolCall.function?.arguments) {
                        accumulated.arguments = (accumulated.arguments || '') + toolCall.function.arguments
                    }

                    // 检查是否收集完整（包含 id/name/args），并且arguments是有效JSON
                    if (accumulated.id && accumulated.name && accumulated.arguments) {
                        try {
                            const args = JSON.parse(accumulated.arguments)
                            events.push(
                                ...utils.processToolUsePart(
                                    {
                                        id: accumulated.id,
                                        name: accumulated.name,
                                        args: args
                                    },
                                    currentToolIndex
                                )
                            )
                            currentToolIndex++
                            // 清除已处理的工具调用
                            toolCallAccumulator.delete(toolIndex)
                        } catch (e) {
                            // JSON还不完整，继续累积
                        }
                    }
                }
            }

            // 仅当 OpenAI 明确以 tool_calls 结束时，再发送一次 stop_reason=tool_use，避免并行多工具时提前结束
            if (!toolUseStopEmitted && (choice.finish_reason === 'tool_calls' || choice.finish_reason === 'function_call')) {
                events.push(
                    `event: message_delta\n` +
                        `data: ${JSON.stringify({
                            type: 'message_delta',
                            delta: { stop_reason: 'tool_use' }
                        })}\n\n`
                )
                toolUseStopEmitted = true
            }

            return {
                events,
                textBlockIndex: currentTextIndex,
                toolUseBlockIndex: currentToolIndex
            }
        })
    }
}
