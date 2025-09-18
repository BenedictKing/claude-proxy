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

    const headers = new Headers()
    // 复制必要的请求头
    request.headers.forEach((value, key) => {
      const lowerKey = key.toLowerCase()
      if (lowerKey !== 'authorization' && lowerKey !== 'x-api-key' && lowerKey !== 'host') {
        headers.set(key, value)
      }
    })

    // 确保 User-Agent 的兼容性
    const userAgent = headers.get('user-agent')
    if (!/^claude-cli/i.test(userAgent || '')) {
      headers.set('User-Agent', 'claude-cli/1.0.58 (external, cli)')
    }

    // 根据 API 密钥格式和上游配置，决定认证方式
    // Anthropic 官方 API (sk-ant-...) 使用 x-api-key
    // 许多第三方 Claude 兼容服务使用 Bearer Token
    if (apiKey.startsWith('sk-ant-')) {
      headers.set('x-api-key', apiKey)
    } else {
      headers.set('Authorization', `Bearer ${apiKey}`)
    }

    headers.set('anthropic-version', '2023-06-01')
    headers.set('Content-Type', 'application/json')

    return new Request(finalUrl, {
      method: 'POST',
      headers,
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