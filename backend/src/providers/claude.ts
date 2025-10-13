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
    // 对于Claude provider，尽可能保持透传，只修改必要的部分

    // 1. 构建目标URL：baseUrl已包含完整路径，只需添加端点
    const originalUrl = new URL(request.url)

    // 提取端点路径（移除 /v1 前缀）
    const endpoint = originalUrl.pathname.replace(/^\/v1/, '')

    // 构建完整目标URL - 如果baseUrl不包含/v1，则需要添加
    let targetUrl = baseUrl
    if (!targetUrl.endsWith('/')) {
      targetUrl += '/'
    }
    // 检测baseUrl是否已包含/v1路径
    if (!targetUrl.includes('/v1/') && !targetUrl.endsWith('/v1')) {
      targetUrl += 'v1/'
    }
    targetUrl += endpoint.replace(/^\//, '') + originalUrl.search
    
    // 从baseUrl解析出正确的主机信息用于Host头
    const baseUrlObj = new URL(baseUrl)

    // 2. 克隆headers，最小化修改
    const headers = new Headers(request.headers)
    
    // 设置正确的Host头
    headers.set('Host', baseUrlObj.hostname)
    
    // 设置认证头
    if (apiKey.startsWith('sk-ant-')) {
      headers.set('x-api-key', apiKey)
    } else {
      headers.set('Authorization', `Bearer ${apiKey}`)
    }
    
    // 移除代理级别的认证头（如果存在）
    headers.delete('x-proxy-key')
    
    // 确保兼容的User-Agent
    const userAgent = headers.get('user-agent')
    if (!userAgent || !/^claude-cli/i.test(userAgent)) {
      headers.set('User-Agent', 'claude-cli/1.0.58 (external, cli)')
    }

    // 3. 处理请求体：对于纯透传，保持原始body
    let requestBody: ReadableStream<Uint8Array> | null = request.body
    
    // 只有在需要模型重定向时才解析和重构请求体
    if (upstream && upstream.modelMapping && Object.keys(upstream.modelMapping).length > 0) {
      const claudeRequest = (await request.json()) as types.ClaudeRequest
      claudeRequest.model = redirectModel(claudeRequest.model, upstream)
      const bodyString = JSON.stringify(claudeRequest)
      // 将字符串转换为ReadableStream
      requestBody = new ReadableStream({
        start(controller) {
          controller.enqueue(new TextEncoder().encode(bodyString))
          controller.close()
        }
      })
    }

    return new Request(targetUrl, {
      method: request.method,
      headers: headers,
      body: requestBody
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
