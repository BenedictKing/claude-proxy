export function generateId(): string {
  return Math.random().toString(36).substring(2)
}

// 标准化 Claude 角色名称
export function normalizeClaudeRole(role: any): 'system' | 'user' | 'assistant' | 'tool' {
  const r = String(role ?? '').toLowerCase()
  if (r === 'assistant' || r === 'model') return 'assistant'
  if (r === 'user' || r === 'human') return 'user'
  if (r === 'system') return 'system'
  if (r === 'tool') return 'tool'
  return 'user' // 默认为用户角色
}

// API密钥掩码函数 - 保留前后5个字符，中间用***代替
export function maskApiKey(apiKey: string): string {
  if (!apiKey) return ''
  const len = apiKey.length
  if (len <= 10) return `${apiKey.slice(0, 3)}***${apiKey.slice(-2)}`
  return `${apiKey.slice(0, 8)}***${apiKey.slice(-5)}`
}

export function sendMessageStart(controller: ReadableStreamDefaultController): void {
  const event = `event: message_start\ndata: ${JSON.stringify({
    type: 'message_start',
    message: {
      id: generateId(),
      type: 'message',
      role: 'assistant',
      content: []
    }
  })}\n\n`
  controller.enqueue(new TextEncoder().encode(event))
}

export function sendMessageStop(controller: ReadableStreamDefaultController): void {
  const event = `event: message_stop\ndata: ${JSON.stringify({
    type: 'message_stop'
  })}\n\n`
  controller.enqueue(new TextEncoder().encode(event))
}

export function processTextPart(text: string, index: number): string[] {
  const events: string[] = []

  events.push(
    `event: content_block_start\ndata: ${JSON.stringify({
      type: 'content_block_start',
      index,
      content_block: {
        type: 'text',
        text: ''
      }
    })}\n\n`
  )

  events.push(
    `event: content_block_delta\ndata: ${JSON.stringify({
      type: 'content_block_delta',
      index,
      delta: {
        type: 'text_delta',
        text
      }
    })}\n\n`
  )

  events.push(
    `event: content_block_stop\ndata: ${JSON.stringify({
      type: 'content_block_stop',
      index
    })}\n\n`
  )

  return events
}

export function processToolUsePart(functionCall: { name: string; args: any; id?: string }, index: number): string[] {
  const events: string[] = []
  // If upstream provided a stable id (e.g., OpenAI tool_call.id), use it;
  // otherwise generate one for providers that don't expose ids (e.g., Gemini).
  const toolUseId = functionCall.id || generateId()

  events.push(
    `event: content_block_start\ndata: ${JSON.stringify({
      type: 'content_block_start',
      index,
      content_block: {
        type: 'tool_use',
        id: toolUseId,
        name: functionCall.name,
        input: {}
      }
    })}\n\n`
  )

  events.push(
    `event: content_block_delta\ndata: ${JSON.stringify({
      type: 'content_block_delta',
      index,
      delta: {
        type: 'input_json_delta',
        partial_json: JSON.stringify(functionCall.args)
      }
    })}\n\n`
  )

  events.push(
    `event: content_block_stop\ndata: ${JSON.stringify({
      type: 'content_block_stop',
      index
    })}\n\n`
  )

  return events
}

export function buildUrl(baseUrl: string, endpoint: string): string {
  let finalUrl = baseUrl
  if (!finalUrl.endsWith('/')) {
    finalUrl += '/'
  }
  return finalUrl + endpoint
}

export async function processProviderStream(
  providerResponse: Response,
  processLine: (
    jsonStr: string,
    textIndex: number,
    toolIndex: number
  ) => { events: string[]; textBlockIndex: number; toolUseBlockIndex: number } | null
): Promise<Response> {
  const stream = new ReadableStream({
    async start(controller) {
      const reader = providerResponse.body?.getReader()
      if (!reader) {
        controller.close()
        return
      }

      const decoder = new TextDecoder()
      let buffer = ''
      let textBlockIndex = 0
      let toolUseBlockIndex = 0

      sendMessageStart(controller)

      try {
        // 主处理循环
        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          const chunk = buffer + decoder.decode(value, { stream: true })
          const lines = chunk.split('\n')

          buffer = lines.pop() || ''

          for (const line of lines) {
            const trimmedLine = line.trim()
            if (!trimmedLine) continue

            // 使用正则匹配 SSE data 字段，支持各种格式
            const dataMatch = trimmedLine.match(/^data:\s*(.*)$/)
            const jsonStr = dataMatch ? dataMatch[1].trim() : trimmedLine

            if (jsonStr === '[DONE]') continue
            if (!jsonStr) continue

            const result = processLine(jsonStr, textBlockIndex, toolUseBlockIndex)
            if (result) {
              textBlockIndex = result.textBlockIndex
              toolUseBlockIndex = result.toolUseBlockIndex
              for (const event of result.events) {
                controller.enqueue(new TextEncoder().encode(event))
              }
            }
          }
        }

        // 处理缓冲区中剩余的数据
        if (buffer.trim()) {
          const dataMatch = buffer.trim().match(/^data:\s*(.*)$/)
          let jsonStr = dataMatch ? dataMatch[1].trim() : buffer.trim()

          if (jsonStr && jsonStr !== '[DONE]') {
            const result = processLine(jsonStr, textBlockIndex, toolUseBlockIndex)
            if (result) {
              for (const event of result.events) {
                controller.enqueue(new TextEncoder().encode(event))
              }
            }
          }
        }

        // 正常结束流
        sendMessageStop(controller)
        controller.close()
      } catch (error) {
        // 发生错误时，向流的消费者发出错误信号
        console.error(`[${new Date().toISOString()}] 💥 Stream processing error:`, error)
        controller.error(error)
      } finally {
        // 无论成功或失败，都释放 reader lock
        reader.releaseLock()
      }
    }
  })

  return new Response(stream, {
    status: providerResponse.status,
    headers: {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive'
    }
  })
}

export function cleanJsonSchema(schema: any): any {
  if (!schema || typeof schema !== 'object') {
    return schema
  }

  const cleaned = { ...schema }

  for (const key in cleaned) {
    if (key === '$schema' || key === 'title' || key === 'examples' || key === 'additionalProperties') {
      delete cleaned[key]
    } else if (key === 'enum' && Array.isArray(cleaned[key])) {
      cleaned[key] = cleaned[key]
    } else if (key === 'format' && cleaned.type === 'string') {
      delete cleaned[key]
    } else if (key === 'properties' && typeof cleaned[key] === 'object') {
      cleaned[key] = cleanJsonSchema(cleaned[key])
    } else if (key === 'items' && typeof cleaned[key] === 'object') {
      cleaned[key] = cleanJsonSchema(cleaned[key])
    } else if (typeof cleaned[key] === 'object' && !Array.isArray(cleaned[key])) {
      cleaned[key] = cleanJsonSchema(cleaned[key])
    }
  }

  return cleaned
}