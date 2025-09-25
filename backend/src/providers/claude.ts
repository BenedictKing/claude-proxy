import * as types from '../types'
import * as provider from './provider'
import * as utils from '../utils'
import { redirectModel } from '../config/config'

export class impl implements provider.Provider {
  async convertToProviderRequest(
    request: Request,
    baseUrl: string,
    apiKey: string,
    upstream?: import('../config/config').UpstreamConfig
  ): Promise<Request> {
    const claudeRequest = (await request.json()) as types.ClaudeRequest

    // 应用模型重定向
    if (upstream) {
      claudeRequest.model = redirectModel(claudeRequest.model, upstream)
    }

    // Claude API 的端点通常是 /v1/messages
    const finalUrl = utils.buildUrl(baseUrl, 'messages')

    // 使用数组保证header顺序，然后构建Headers对象
    const headerEntries: [string, string][] = []
    const upstreamHost = new URL(baseUrl).hostname
    let authHeaderReplaced = false
    let userAgentFound = false

    // 遍历原始请求头以保留顺序
    for (const [key, value] of request.headers) {
      const lowerKey = key.toLowerCase()

      if (lowerKey === 'host') {
        // 替换为上游 Host
        headerEntries.push(['Host', upstreamHost])
        continue
      }

      if (lowerKey === 'authorization' || lowerKey === 'x-api-key') {
        if (!authHeaderReplaced) {
          // 在原始认证头的位置插入新的认证头
          if (apiKey.startsWith('sk-ant-')) {
            headerEntries.push(['x-api-key', apiKey])
          } else {
            headerEntries.push(['Authorization', `Bearer ${apiKey}`])
          }
          authHeaderReplaced = true
        }
        // 跳过旧的认证头
        continue
      }

      if (lowerKey === 'user-agent') {
        userAgentFound = true
        // 确保 User-Agent 的兼容性
        if (!/^claude-cli/i.test(value)) {
          headerEntries.push(['User-Agent', 'claude-cli/1.0.58 (external, cli)'])
        } else {
          headerEntries.push([key, value])
        }
        continue
      }

      headerEntries.push([key, value])
    }

    // 如果原始请求中没有认证头，添加到末尾
    if (!authHeaderReplaced) {
      if (apiKey.startsWith('sk-ant-')) {
        headerEntries.push(['x-api-key', apiKey])
      } else {
        headerEntries.push(['Authorization', `Bearer ${apiKey}`])
      }
    }

    // 如果没有User-Agent，添加到末尾
    if (!userAgentFound) {
      headerEntries.push(['User-Agent', 'claude-cli/1.0.58 (external, cli)'])
    }

    // 从有序数组构建Headers对象
    const newHeaders = new Headers(headerEntries)

    return new Request(finalUrl, {
      method: 'POST',
      headers: newHeaders,
      body: JSON.stringify(claudeRequest)
    })
  }

  async convertToClaudeResponse(providerResponse: Response): Promise<Response> {
    // Claude provider 是一个直通层，响应已经是正确的格式。
    // 我们创建一个新的 Response 对象来清理头部，但必须注意不能消耗 body，以支持流式传输。
    const headers = new Headers(providerResponse.headers)
    headers.delete('content-encoding')
    headers.delete('transfer-encoding')

    return new Response(providerResponse.body, {
      status: providerResponse.status,
      statusText: providerResponse.statusText,
      headers
    })
  }
}
