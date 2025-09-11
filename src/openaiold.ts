import * as types from './types'
import * as provider from './provider'
import * as utils from './utils'
import { redirectModel } from './config'

export class impl implements provider.Provider {
  async convertToProviderRequest(
    request: Request,
    baseUrl: string,
    apiKey: string,
    upstream?: import('./config').UpstreamConfig
  ): Promise<Request> {
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

  private convertToOpenAIRequestBody(
    claudeRequest: types.ClaudeRequest,
    upstream?: import('./config').UpstreamConfig
  ): types.OpenAIRequest {
    const converted = this.convertMessages(claudeRequest.messages)

    // 处理 system 字段，支持字符串和数组格式
    let systemContent: string | undefined
    if (claudeRequest.system) {
      if (typeof claudeRequest.system === 'string') {
        systemContent = claudeRequest.system
      } else if (Array.isArray(claudeRequest.system)) {
        // 从数组中提取文本内容
        const textItem = claudeRequest.system.find(item => item.type === 'text')
        systemContent = textItem?.text
      }
    }

    const messages: types.OpenAIMessage[] = systemContent
      ? [{ role: 'system', content: systemContent }, ...converted]
      : converted

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
          parameters: utils.cleanJsonSchema(tool.input_schema),
          strict: true
        }
      }))
      openaiRequest.tool_choice = 'auto'
    }

    if (claudeRequest.temperature !== undefined) {
      openaiRequest.temperature = claudeRequest.temperature
    }

    if (claudeRequest.max_tokens !== undefined) {
      openaiRequest.max_tokens = claudeRequest.max_tokens
    }

    return openaiRequest
  }

  private convertMessages(claudeMessages: types.ClaudeMessage[]): types.OpenAIMessage[] {
    const openaiMessages: types.OpenAIMessage[] = []
    const toolCallMap = new Map<string, string>()

    for (const message of claudeMessages) {
      const normalizedRole = this.normalizeClaudeRole((message as any).role)
      if (typeof message.content === 'string') {
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
              content: typeof content.content === 'string' ? content.content : JSON.stringify(content.content)
            })
            break
        }
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

      for (const toolResult of toolResults) {
        openaiMessages.push({
          role: 'tool',
          tool_call_id: toolResult.tool_call_id,
          content: toolResult.content
        })
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

    return utils.processProviderStream(openaiResponse, (jsonStr, textBlockIndex, toolUseBlockIndex) => {
      let openaiData: types.OpenAIStreamResponse
      try {
        openaiData = JSON.parse(jsonStr)
      } catch (e) {
        console.error(
          `[${new Date().toISOString()}] 🚨 OpenAI(old) stream JSON parse error, skipping. Raw data:`,
          jsonStr
        )
        return null
      }

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

          // 检查是否收集完整（包含 id/name/args），并且 arguments 是有效 JSON
          // 为了让后续用户的 tool_result 能正确回传给 OpenAI，必须把 OpenAI 返回的 tool_call.id 原样透传给客户端
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
              // 通知客户端该轮以 tool_use 结束，便于立刻触发工具执行
              events.push(
                `event: message_delta\n` +
                  `data: ${JSON.stringify({
                    type: 'message_delta',
                    delta: { stop_reason: 'tool_use' }
                  })}\n\n`
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

      return {
        events,
        textBlockIndex: currentTextIndex,
        toolUseBlockIndex: currentToolIndex
      }
    })
  }
}
