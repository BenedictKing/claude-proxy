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

    // 手动重建headers，以精确控制顺序并替换必要的头部
    const newHeaders = new Headers()
    const upstreamHost = new URL(baseUrl).hostname
    let authHeaderReplaced = false

    // 遍历原始请求头以保留顺序
    for (const [key, value] of request.headers) {
      const lowerKey = key.toLowerCase()

      if (lowerKey === 'host') {
        // 替换为上游 Host
        newHeaders.set('Host', upstreamHost)
        continue
      }

      if (lowerKey === 'authorization' || lowerKey === 'x-api-key') {
        if (!authHeaderReplaced) {
          // 在原始认证头的位置插入新的认证头
          if (apiKey.startsWith('sk-ant-')) {
            newHeaders.set('x-api-key', apiKey)
          } else {
            newHeaders.set('Authorization', `Bearer ${apiKey}`)
          }
          authHeaderReplaced = true
        }
        // 跳过旧的认证头
        continue
      }

      newHeaders.append(key, value)
    }

    // 确保 User-Agent 的兼容性 (如果在原始请求头中没有设置，则添加)
    const userAgent = newHeaders.get('user-agent')
    if (!userAgent || !/^claude-cli/i.test(userAgent)) {
      newHeaders.set('User-Agent', 'claude-cli/1.0.58 (external, cli)')
    }
    
    // 如果原始请求中没有认证头，我们需要确保它被添加
    if (!authHeaderReplaced) {
        if (apiKey.startsWith('sk-ant-')) {
            newHeaders.set('x-api-key', apiKey);
        } else {
            newHeaders.set('Authorization', `Bearer ${apiKey}`);
        }
    }

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
