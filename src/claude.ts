import * as provider from './provider'
import * as utils from './utils'
import * as types from './types'

export class impl implements provider.Provider {
    async convertToProviderRequest(request: Request, baseUrl: string, apiKey: string, upstream?: import('./config').UpstreamConfig): Promise<Request> {
        const claudeRequest = (await request.json()) as types.ClaudeRequest

        // Claude API 的端点通常是 /v1/messages
        const finalUrl = utils.buildUrl(baseUrl, 'messages')

        const headers = new Headers()
        request.headers.forEach((value, key) => {
            const lowerKey = key.toLowerCase()
            if (lowerKey !== 'authorization' && lowerKey !== 'x-api-key' && lowerKey !== 'host') {
                if (lowerKey == 'user-agent')
                    if (value && value.startsWith('claude-cli')) headers.set(key, value)
                    else headers.set('User-Agent', 'claude-cli/1.0.58 (external, cli)')
                headers.set(key, value)
            }
        })

        // Anthropic 官方 API 使用 x-api-key 和 anthropic-version
        // headers.set('x-api-key', apiKey)
        headers.set('Authorization', `Bearer ${apiKey}`)
        headers.set('anthropic-version', '2023-06-01')
        headers.set('Content-Type', 'application/json')

        return new Request(finalUrl, {
            method: 'POST',
            headers,
            body: JSON.stringify(claudeRequest)
        })
    }

    async convertToClaudeResponse(providerResponse: Response): Promise<Response> {
        // 需要重新构建响应，避免传输编码问题
        const body = await providerResponse.text()

        // 创建新的 Headers，排除可能导致问题的头部
        const headers = new Headers()
        providerResponse.headers.forEach((value, key) => {
            const lowerKey = key.toLowerCase()
            // 移除可能导致编码问题的头部
            if (lowerKey !== 'content-encoding' && lowerKey !== 'transfer-encoding' && lowerKey !== 'content-length') {
                headers.set(key, value)
            }
        })

        // 设置正确的内容类型
        headers.set('content-type', 'application/json')

        return new Response(body, {
            status: providerResponse.status,
            statusText: providerResponse.statusText,
            headers
        })
    }
}
