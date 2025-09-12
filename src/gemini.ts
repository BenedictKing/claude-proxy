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
    const geminiRequest = this.convertToGeminiRequestBody(claudeRequest, upstream)

    // 应用模型重定向
    const finalModel = upstream ? redirectModel(claudeRequest.model, upstream) : claudeRequest.model
    const endpoint = `models/${finalModel}:${claudeRequest.stream ? 'streamGenerateContent?alt=sse' : 'generateContent'}`
    const finalUrl = utils.buildUrl(baseUrl, endpoint)

    const headers = new Headers(request.headers)
    headers.set('x-goog-api-key', apiKey)
    headers.set('Content-Type', 'application/json')

    return new Request(finalUrl, {
      method: 'POST',
      headers,
      body: JSON.stringify(geminiRequest)
    })
  }

  async convertToClaudeResponse(geminiResponse: Response): Promise<Response> {
    if (!geminiResponse.ok) {
      return geminiResponse
    }

    const contentType = geminiResponse.headers.get('content-type') || ''
    const isStream = contentType.includes('text/event-stream')

    if (isStream) {
      return this.convertStreamResponse(geminiResponse)
    } else {
      return this.convertNormalResponse(geminiResponse)
    }
  }

  private convertToGeminiRequestBody(
    claudeRequest: types.ClaudeRequest,
    upstream?: import('./config').UpstreamConfig
  ): types.GeminiRequest {
    const toolUseMap = this.buildToolUseMap(claudeRequest.messages)
    const contents = this.convertMessages(claudeRequest.messages, toolUseMap)

    const geminiRequest: types.GeminiRequest = {
      model: claudeRequest.model,
      contents
    }

    if (claudeRequest.tools && claudeRequest.tools.length > 0) {
      geminiRequest.tools = [
        {
          functionDeclarations: claudeRequest.tools.map(tool => ({
            name: tool.name,
            description: tool.description,
            parameters: utils.cleanJsonSchema(tool.input_schema)
          }))
        }
      ]
    }

    if (claudeRequest.temperature !== undefined || claudeRequest.max_tokens !== undefined) {
      geminiRequest.generationConfig = {}
      if (claudeRequest.temperature !== undefined) {
        geminiRequest.generationConfig.temperature = claudeRequest.temperature
      }
      if (claudeRequest.max_tokens !== undefined) {
        geminiRequest.generationConfig.maxOutputTokens = claudeRequest.max_tokens
      }
    }

    return geminiRequest
  }

  private buildToolUseMap(messages: types.ClaudeMessage[]): Map<string, string> {
    const toolUseMap = new Map<string, string>()

    for (const message of messages) {
      if (typeof message.content === 'string') continue

      for (const content of message.content) {
        if (content.type === 'tool_use') {
          toolUseMap.set(content.id, content.name)
        }
      }
    }

    return toolUseMap
  }

  private convertMessages(messages: types.ClaudeMessage[], toolUseMap: Map<string, string>): types.GeminiContent[] {
    const contents: types.GeminiContent[] = []

    for (const message of messages) {
      if (typeof message.content === 'string') {
        contents.push({
          parts: [{ text: message.content }],
          role: message.role === 'assistant' ? 'model' : 'user'
        })
        continue
      }

      const textParts: types.GeminiPart[] = []
      const toolUseParts: types.GeminiPart[] = []
      const toolResultParts: types.GeminiPart[] = []

      for (const content of message.content) {
        switch (content.type) {
          case 'text':
            textParts.push({ text: content.text })
            break
          case 'tool_use':
            toolUseParts.push({
              functionCall: {
                name: content.name,
                args: content.input
              }
            })
            break
          case 'tool_result':
            const functionName = toolUseMap.get(content.tool_use_id)
            if (functionName) {
              toolResultParts.push({
                functionResponse: {
                  name: functionName,
                  response: { content: content.content }
                }
              })
            }
            break
        }
      }

      if (textParts.length > 0 || toolUseParts.length > 0) {
        contents.push({
          parts: [...textParts, ...toolUseParts],
          role: message.role === 'assistant' ? 'model' : 'user'
        })
      }

      if (toolResultParts.length > 0) {
        contents.push({
          parts: toolResultParts,
          role: 'tool'
        })
      }
    }

    return contents
  }

  private async convertNormalResponse(geminiResponse: Response): Promise<Response> {
    const geminiData = (await geminiResponse.json()) as types.GeminiResponse

    const claudeResponse: types.ClaudeResponse = {
      id: utils.generateId(),
      type: 'message',
      role: 'assistant',
      content: []
    }

    if (geminiData.candidates && geminiData.candidates.length > 0) {
      const candidate = geminiData.candidates[0]
      let hasToolUse = false

      for (const part of candidate.content.parts) {
        if ('text' in part) {
          claudeResponse.content.push({
            type: 'text',
            text: part.text
          })
        } else if ('functionCall' in part) {
          hasToolUse = true
          claudeResponse.content.push({
            type: 'tool_use',
            id: utils.generateId(),
            name: part.functionCall.name,
            input: part.functionCall.args
          })
        }
      }

      if (hasToolUse) {
        claudeResponse.stop_reason = 'tool_use'
      } else if (candidate.finishReason === 'MAX_TOKENS') {
        claudeResponse.stop_reason = 'max_tokens'
      } else {
        claudeResponse.stop_reason = 'end_turn'
      }
    }

    if (geminiData.usageMetadata) {
      claudeResponse.usage = {
        input_tokens: geminiData.usageMetadata.promptTokenCount,
        output_tokens: geminiData.usageMetadata.candidatesTokenCount
      }
    }

    return new Response(JSON.stringify(claudeResponse), {
      status: geminiResponse.status,
      headers: {
        'Content-Type': 'application/json'
      }
    })
  }

  private async convertStreamResponse(geminiResponse: Response): Promise<Response> {
    return utils.processProviderStream(geminiResponse, (jsonStr, textBlockIndex, toolUseBlockIndex) => {
      let geminiData: any
      try {
        geminiData = JSON.parse(jsonStr)
      } catch (e) {
        console.warn(`[${new Date().toISOString()}] 🟡 Gemini stream JSON parse error, skipping a chunk.`)
        return null
      }

      // 关键修复：检查上游流中是否直接返回了错误对象
      if (geminiData.error) {
        console.error(`[${new Date().toISOString()}] 🚨 Upstream error in stream:`, JSON.stringify(geminiData.error))
        throw new Error(`Upstream stream error: ${geminiData.error.message || JSON.stringify(geminiData.error)}`)
      }

      if (!geminiData || !geminiData.candidates || !geminiData.candidates.length) {
        return null
      }

      const candidate = geminiData.candidates[0]
      const events: string[] = []
      let currentTextIndex = textBlockIndex
      let currentToolIndex = toolUseBlockIndex

      if (candidate.content) {
        for (const part of candidate.content.parts) {
          if ('text' in part && part.text) {
            events.push(...utils.processTextPart(part.text, currentTextIndex))
            currentTextIndex++
          } else if ('functionCall' in part) {
            events.push(...utils.processToolUsePart(part.functionCall, currentToolIndex))
            currentToolIndex++
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
