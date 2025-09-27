import express from 'express'
import { Readable } from 'stream'
import path from 'path'
import * as provider from './providers/provider'
import * as gemini from './providers/gemini'
import * as openaiold from './providers/openaiold'
import * as openai from './providers/openai'
import * as claude from './providers/claude'
import { configManager, UpstreamConfig } from './config/config'
import { envConfigManager } from './config/env'
import { maskApiKey } from './utils/index'
import webRoutes from './api/web-routes'
import chokidar from 'chokidar'
import { Agent, fetch as undiciFetch } from 'undici'

// 敏感头统一掩码配置与函数
const SENSITIVE_HEADER_KEYS = new Set(['authorization', 'x-api-key', 'x-goog-api-key'])
function maskHeaderValue(key: string, value: string): string {
  const lowerKey = key.toLowerCase()
  if (lowerKey === 'authorization') {
    return (
      value.replace(/^(Bearer\s+)(.+)$/i, (_, prefix, token) => `${prefix}${maskApiKey(token)}`) || maskApiKey(value)
    )
  }
  return SENSITIVE_HEADER_KEYS.has(lowerKey) ? maskApiKey(value) : value
}

const app = express()
app.use(express.json({ limit: '50mb' }))

// CORS 配置 - 允许开发环境跨域访问
app.use((req, res, next) => {
  const origin = req.headers.origin

  // 开发环境允许所有localhost源，生产环境可以更严格
  if (process.env.NODE_ENV === 'development') {
    if (origin && origin.includes('localhost')) {
      res.setHeader('Access-Control-Allow-Origin', origin)
    }
  } else {
    // 生产环境可以设置具体的允许域名
    res.setHeader('Access-Control-Allow-Origin', '*')
  }

  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS')
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, x-api-key')
  res.setHeader('Access-Control-Allow-Credentials', 'true')

  // 处理预检请求
  if (req.method === 'OPTIONS') {
    res.status(200).end()
    return
  }

  next()
})

// Web管理界面访问控制中间件
const webAuthMiddleware = (req: any, res: any, next: any) => {
  // 对于健康检查、开发信息等公开端点，直接放行
  if (
    req.path === envConfigManager.getConfig().healthCheckPath ||
    req.path === '/admin/config/reload' ||
    (isDevelopment && req.path === '/admin/dev/info')
  ) {
    return next()
  }

  // 对于前端静态资源文件（CSS、JS、图片等），直接放行
  if (
    req.path.startsWith('/assets/') ||
    req.path.endsWith('.css') ||
    req.path.endsWith('.js') ||
    req.path.endsWith('.ico') ||
    req.path.endsWith('.png') ||
    req.path.endsWith('.jpg') ||
    req.path.endsWith('.gif') ||
    req.path.endsWith('.svg') ||
    req.path.endsWith('.woff') ||
    req.path.endsWith('.woff2') ||
    req.path.endsWith('.ttf') ||
    req.path.endsWith('.eot')
  ) {
    return next()
  }

  // 对于API代理端点，已在后续处理
  if (req.path.startsWith('/v1/')) {
    return next()
  }

  // 如果禁用了Web UI，对所有其他路径返回404
  if (!envConfigManager.getConfig().enableWebUI) {
    return res.status(404).json({
      error: 'Web界面已禁用',
      message: '此服务器运行在纯API模式下，请通过API端点访问服务'
    })
  }

  // 对于Web管理界面，检查访问密钥
  let providedApiKey = req.headers['x-api-key'] || req.headers['authorization'] || req.query.key

  // 移除 Bearer 前缀（如果有）
  if (providedApiKey && typeof providedApiKey === 'string') {
    providedApiKey = providedApiKey.replace(/^bearer\s+/i, '')
  }

  const expectedApiKey = envConfigManager.getConfig().proxyAccessKey

  if (!providedApiKey || providedApiKey !== expectedApiKey) {
    console.warn(`[${new Date().toISOString()}] 🔒 Web界面访问被拒绝 - IP: ${req.ip}, Path: ${req.path}`)

    // 返回简单的认证页面
    return res.status(401).send(`
      <!DOCTYPE html>
      <html>
      <head>
        <title>Claude Proxy - 访问验证</title>
        <meta charset="utf-8">
        <meta name="viewport" content="width=device-width, initial-scale=1">
        <style>
          body { font-family: system-ui, -apple-system, sans-serif; background: #f5f5f5; margin: 0; padding: 40px; }
          .container { max-width: 400px; margin: 100px auto; background: white; padding: 40px; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
          h1 { color: #333; margin-bottom: 20px; }
          input { width: 100%; padding: 12px; border: 1px solid #ddd; border-radius: 4px; margin-bottom: 20px; box-sizing: border-box; }
          button { width: 100%; padding: 12px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer; }
          button:hover { background: #0056b3; }
          .error { color: #dc3545; margin-bottom: 20px; font-size: 14px; }
        </style>
      </head>
      <body>
        <div class="container">
          <h1>🔐 Claude Proxy 管理界面</h1>
          <div class="error">请输入访问密钥以继续</div>
          <form onsubmit="handleAuth(event)">
            <input type="password" id="apiKey" placeholder="访问密钥 (PROXY_ACCESS_KEY)" required>
            <button type="submit">访问管理界面</button>
          </form>
        </div>
        <script>
          function handleAuth(e) {
            e.preventDefault();
            const key = document.getElementById('apiKey').value;
            const url = new URL(window.location);
            url.searchParams.set('key', key);
            window.location.href = url.toString();
          }
        </script>
      </body>
      </html>
    `)
  }

  next()
}

// 应用Web界面访问控制
app.use(webAuthMiddleware)

// Web管理界面API路由
app.use(webRoutes)

// 静态文件服务（前端构建产物）- 仅在启用Web UI时
if (envConfigManager.getConfig().enableWebUI) {
  // 使用 process.cwd() 获取运行时工作目录，而不是编译时的 __dirname
  // 这样避免了Bun编译时将 __dirname 硬编码为构建时路径的问题
  const frontendDistPath = path.join(process.cwd(), 'frontend/dist')

  app.use(express.static(frontendDistPath))
  // SPA 路由支持
  app.get('/', (req, res) => {
    res.sendFile(path.join(frontendDistPath, 'index.html'))
  })
} else {
  // 纯API模式：根路径返回API信息
  app.get('/', (req, res) => {
    res.json({
      name: 'Claude API Proxy',
      mode: 'API Only',
      version: '1.0.0',
      endpoints: {
        health: envConfigManager.getConfig().healthCheckPath,
        proxy: '/v1/messages',
        config: '/admin/config/reload'
      },
      message: 'Web界面已禁用，此服务器运行在纯API模式下'
    })
  })
}

// 开发模式检测
const isDevelopment = process.env.NODE_ENV === 'development'
const isManagedByRunner = process.env.RUNNER === 'dev-runner'

// 开发模式中间件
if (isDevelopment) {
  app.use((req, res, next) => {
    res.setHeader('X-Development-Mode', 'true')
    next()
  })
}

// 健康检查端点
app.get(envConfigManager.getConfig().healthCheckPath, (req, res) => {
  const healthData = {
    status: 'healthy',
    timestamp: new Date().toISOString(),
    uptime: process.uptime(),
    mode: isDevelopment ? 'development' : 'production',
    config: {
      upstreamCount: configManager.getConfig().upstream.length,
      currentUpstream: configManager.getConfig().currentUpstream,
      loadBalance: configManager.getConfig().loadBalance
    }
  }

  res.json(healthData)
})

// 配置重载端点
app.post('/admin/config/reload', (req, res) => {
  try {
    configManager.reloadConfig()
    res.json({
      status: 'success',
      message: '配置已重载',
      timestamp: new Date().toISOString(),
      config: {
        upstreamCount: configManager.getConfig().upstream.length,
        currentUpstream: configManager.getConfig().currentUpstream,
        loadBalance: configManager.getConfig().loadBalance
      }
    })
  } catch (error) {
    res.status(500).json({
      status: 'error',
      message: '配置重载失败',
      error: error instanceof Error ? error.message : '未知错误'
    })
  }
})

// 开发信息端点（仅在开发模式）
if (isDevelopment) {
  app.get('/admin/dev/info', (req, res) => {
    res.json({
      status: 'development',
      timestamp: new Date().toISOString(),
      config: configManager.getConfig(),
      environment: envConfigManager.getConfig()
    })
  })
}

// 统一入口：处理所有POST请求到 /v1/messages
app.post('/v1/messages', async (req, res) => {
  const startTime = Date.now()

  try {
    if (envConfigManager.getConfig().enableRequestLogs) {
      console.log(`[${new Date().toISOString()}] ${isDevelopment ? '📥' : ''} 收到请求: ${req.method} ${req.path}`)
      if (isDevelopment) {
        const originalBodyStr = JSON.stringify(req.body, null, 2)
        console.debug(
          `[${new Date().toISOString()}] 📋 原始请求体:`,
          originalBodyStr.length > 1500 ? originalBodyStr.substring(0, 1500) + '...' : originalBodyStr
        )
        // 对请求头做敏感信息脱敏
        const sanitizedReqHeaders: { [key: string]: string } = {}
        Object.entries(req.headers).forEach(([k, v]) => {
          if (typeof v === 'string') {
            sanitizedReqHeaders[k] = maskHeaderValue(k, v)
          } else if (Array.isArray(v)) {
            sanitizedReqHeaders[k] = v.map(val => maskHeaderValue(k, val)).join(', ')
          }
        })
        console.debug(`[${new Date().toISOString()}] 📥 原始请求头:`, JSON.stringify(sanitizedReqHeaders, null, 2))
      }
    }

    // 验证代理访问密钥
    let providedApiKey = req.headers['x-api-key'] || req.headers['authorization']

    // 移除 Bearer 前缀（如果有）
    if (providedApiKey && typeof providedApiKey === 'string') {
      providedApiKey = providedApiKey.replace(/^bearer\s+/i, '')
    }

    const expectedApiKey = envConfigManager.getConfig().proxyAccessKey

    if (!providedApiKey || providedApiKey !== expectedApiKey) {
      if (envConfigManager.shouldLog('warn')) {
        console.warn(`[${new Date().toISOString()}] ${isDevelopment ? '🔒' : ''} 代理访问密钥验证失败`)
      }
      res.status(401).json({ error: 'Invalid proxy access key' })
      return
    }

    // 获取当前选中的上游配置
    let upstream: UpstreamConfig
    try {
      upstream = configManager.getCurrentUpstream()
      if (!upstream.apiKeys.length) {
        throw new Error(`当前渠道 "${upstream.name || upstream.serviceType}" 没有配置API密钥`)
      }
    } catch (error) {
      console.error('获取当前渠道配置失败:', error)
      res.status(500).json({ error: '当前渠道配置错误或没有可用的API密钥' })
      return
    }

    // 确定提供商实现
    let providerImpl: provider.Provider
    switch (upstream.serviceType) {
      case 'gemini':
        providerImpl = new gemini.impl()
        break
      case 'openai':
        providerImpl = new openai.impl()
        break
      case 'openaiold':
        providerImpl = new openaiold.impl()
        break
      case 'claude':
        providerImpl = new claude.impl()
        break
      default:
        res.status(400).json({ error: 'Unsupported type' })
        return
    }

    // 实现 failover 重试逻辑 - 只在当前渠道的API密钥之间重试
    const maxRetries = upstream.apiKeys.length
    const failedKeys = new Set<string>()
    let providerResponse: Response | null = null
    let lastError: Error | null = null
    // 记录最后一次需要failover的上游错误，用于所有密钥都失败时回传原始错误
    let lastFailoverError: { status: number; body?: any; text?: string } | null = null
    // 候选降级密钥（仅当后续有密钥成功调用时，才将这些密钥移到列表末尾）
    const deprioritizeCandidates = new Set<string>()

    for (let attempt = 0; attempt < maxRetries; attempt++) {
      let apiKey: string | undefined
      try {
        // 获取API密钥（排除已失败的密钥）
        apiKey = configManager.getNextApiKey(upstream, failedKeys)

        if (envConfigManager.shouldLog('info')) {
          console.log(
            `[${new Date().toISOString()}] ${isDevelopment ? '🎯' : ''} 使用上游: ${upstream.name || upstream.serviceType} - ${upstream.baseUrl} (尝试 ${attempt + 1}/${maxRetries})`
          )
          console.log(`[${new Date().toISOString()}] ${isDevelopment ? '🔑' : ''} 使用API密钥: ${maskApiKey(apiKey)}`)
        }

        // 构造提供商所需的 Request 对象
        // 使用 req.rawHeaders 来最大限度地保留原始头部顺序
        const headers = new Headers()
        for (let i = 0; i < req.rawHeaders.length; i += 2) {
          const key = req.rawHeaders[i]
          const value = req.rawHeaders[i + 1]
          const lowerKey = key.toLowerCase()

          if (lowerKey !== 'x-api-key' && lowerKey !== 'authorization') {
            headers.append(key, value)
          }
        }

        // 获取原始请求体字符串，避免JSON重新序列化
        let originalBodyString: string
        try {
          // 尝试获取原始请求体的JSON字符串表示
          originalBodyString = JSON.stringify(req.body)
        } catch (error) {
          // 如果序列化失败，回退到空对象
          originalBodyString = '{}'
        }

        // 构建完整的URL，避免相对路径导致Request构造失败
        const protocol = req.protocol || 'http'
        const host = req.get('host') || 'localhost:3000'
        const fullUrl = `${protocol}://${host}${req.url || '/v1/messages'}`

        const incomingRequest = new Request(fullUrl, {
          method: req.method,
          headers: headers,
          body: originalBodyString
        })

        // 协议转换：Claude -> Provider
        const providerRequest = await providerImpl.convertToProviderRequest(
          incomingRequest,
          upstream.baseUrl,
          apiKey,
          upstream
        )

        // 记录实际发出的请求
        if (isDevelopment || envConfigManager.getConfig().enableRequestLogs) {
          console.log(`[${new Date().toISOString()}] 🌐 实际请求URL: ${providerRequest.url}`)
          console.log(`[${new Date().toISOString()}] 📤 请求方法: ${providerRequest.method}`)
          const reqHeaders: { [key: string]: string } = {}
          providerRequest.headers.forEach((value, key) => {
            reqHeaders[key] = maskHeaderValue(key, value)
          })
          console.debug(`[${new Date().toISOString()}] 📋 实际请求头:`, JSON.stringify(reqHeaders, null, 2))
          try {
            const body = JSON.stringify(await providerRequest.clone().json(), null, 2)
            if (body.length > 0) {
              console.debug(
                `[${new Date().toISOString()}] 📦 实际请求体:`,
                body.length > 1500 ? body.substring(0, 1500) + '...' : body
              )
            }
          } catch (error) {
            console.log(
              `[${new Date().toISOString()}] 📦 请求体: [无法读取 - ${error instanceof Error ? error.message : '未知错误'}]`
            )
          }
        }

        // 调用上游（Bun 与 Node 使用不同的选项）
        const isBun = typeof (globalThis as any).Bun !== 'undefined'
        if (isBun) {
          const bunOpts: any = {}
          if (upstream.insecureSkipVerify) {
            console.log(`[${new Date().toISOString()}] ⚠️ 正在跳过对 ${providerRequest.url} 的TLS证书验证`)
            bunOpts.tls = { rejectUnauthorized: false }
          }
          providerResponse = await fetch(providerRequest as any, bunOpts)
        } else {
          const fetchOptions: any = {}
          if (upstream.insecureSkipVerify) {
            console.log(`[${new Date().toISOString()}] ⚠️ 正在跳过对 ${providerRequest.url} 的TLS证书验证`)
            const insecureConnect: any = {
              rejectUnauthorized: false,
              checkServerIdentity: () => undefined
            }
            fetchOptions.dispatcher = new Agent({ connect: insecureConnect })
          }
          providerResponse = (await undiciFetch(providerRequest as any, fetchOptions)) as any
        }

        // 检查响应是否成功或是否需要failover
        if (providerResponse && providerResponse.ok) {
          // 2xx 状态码认为是成功的
          break
        } else if (providerResponse) {
          // 检查是否是需要failover的错误
          let shouldFailover = false
          let isQuotaRelated = false
          let errorMessage = `上游错误: ${providerResponse.status} ${providerResponse.statusText}`

          // 尝试解析错误响应体来判断是否需要failover
          try {
            const cloneForParse = providerResponse.clone()
            const errorBody = await cloneForParse.json()

            // 检查特定的错误类型：积分不足、密钥无效、余额不足等
            if (errorBody.error) {
              const errorMsg = errorBody.error.message || errorBody.error || ''
              const errorType = (errorBody.error.type || '').toString().toLowerCase()
              if (typeof errorMsg === 'string') {
                const lowerErrorMsg = errorMsg.toLowerCase()
                // 这些错误应该触发failover到下一个密钥
                if (
                  lowerErrorMsg.includes('积分不足') ||
                  lowerErrorMsg.includes('insufficient') ||
                  lowerErrorMsg.includes('invalid') ||
                  lowerErrorMsg.includes('unauthorized') ||
                  lowerErrorMsg.includes('quota') ||
                  lowerErrorMsg.includes('rate limit') ||
                  lowerErrorMsg.includes('credit') ||
                  lowerErrorMsg.includes('balance') ||
                  errorType.includes('permission') ||
                  errorType.includes('insufficient') ||
                  errorType.includes('over_quota') ||
                  errorType.includes('billing')
                ) {
                  shouldFailover = true
                  errorMessage = `API密钥错误: ${errorMsg}`
                  // 标记是否为额度/余额相关问题（供成功后降级使用）
                  if (
                    lowerErrorMsg.includes('积分不足') ||
                    lowerErrorMsg.includes('insufficient') ||
                    lowerErrorMsg.includes('credit') ||
                    lowerErrorMsg.includes('balance') ||
                    lowerErrorMsg.includes('quota') ||
                    errorType.includes('over_quota') ||
                    errorType.includes('billing')
                  ) {
                    isQuotaRelated = true
                  }
                }
              }
            }

            // 401/403 状态码通常是认证/授权问题，应该failover
            if (providerResponse.status === 401 || providerResponse.status === 403) {
              shouldFailover = true
            }

            // 400 Bad Request 中的特定错误也可能需要failover
            if (providerResponse.status === 400 && shouldFailover) {
              // 已经在上面的错误消息检查中设置了shouldFailover
            }

            // 如果确定需要failover，记录原始错误体
            if (shouldFailover) {
              lastFailoverError = { status: providerResponse.status, body: errorBody }
            }
          } catch (parseError) {
            // 无法解析响应体，使用状态码判断
            if (providerResponse.status === 401 || providerResponse.status === 403 || providerResponse.status >= 500) {
              shouldFailover = true
              try {
                const text = await providerResponse.clone().text()
                lastFailoverError = { status: providerResponse.status, text }
              } catch {}
            }
          }

          if (shouldFailover) {
            // 仅记录候选降级密钥，待后续任一密钥成功时再移动到末尾
            if (isQuotaRelated && apiKey) {
              deprioritizeCandidates.add(apiKey)
            }
            throw new Error(errorMessage)
          } else {
            // 其他错误（如模型不存在、请求格式错误等）不需要failover，直接返回给客户端
            break
          }
        }
      } catch (error) {
        lastError = error instanceof Error ? error : new Error(String(error))
        console.warn(`[${new Date().toISOString()}] ⚠️ API密钥失败，原因: ${lastError.message}`)

        // 如果这是最后一次尝试，直接抛出错误
        if (attempt === maxRetries - 1) {
          break
        }

        // 标记当前密钥为失败，继续尝试下一个
        if (apiKey) {
          failedKeys.add(apiKey)
          // 同时在内存中标记密钥失败
          configManager.markKeyAsFailed(apiKey)
          console.log(`[${new Date().toISOString()}] 🔄 将尝试当前渠道的下一个API密钥`)
        } else {
          // 如果无法获取密钥（例如，所有密钥都已尝试过），则没有可重试的密钥了
          break
        }
      }
    }

    // 如果所有重试都失败了
    if (!providerResponse) {
      console.error(`[${new Date().toISOString()}] 💥 所有API密钥都失败了`)
      // 若有记录的最后一次上游错误，按原状态码和内容返回（满足“若无可用密钥才返回原错误”）
      if (lastFailoverError) {
        const status = lastFailoverError.status || 500
        if (lastFailoverError.body && typeof lastFailoverError.body === 'object') {
          res.status(status).json(lastFailoverError.body)
        } else {
          res.status(status).json({ error: lastError?.message || lastFailoverError.text || 'Upstream error' })
        }
      } else {
        res.status(500).json({
          error: '所有上游API密钥都不可用',
          details: lastError?.message
        })
      }
      return
    }

    // 如果本次请求最终成功，执行降级移动（仅对额度/余额相关失败的密钥）
    if (providerResponse.ok && deprioritizeCandidates.size > 0) {
      for (const key of deprioritizeCandidates) {
        try {
          configManager.deprioritizeApiKeyForCurrentUpstream(key)
        } catch {}
      }
    }

    // 记录响应信息
    if (isDevelopment || envConfigManager.getConfig().enableResponseLogs) {
      console.log(
        `[${new Date().toISOString()}] 📥 响应状态: ${providerResponse.status} ${providerResponse.statusText}`
      )
      const responseHeaders: { [key: string]: string } = {}
      providerResponse.headers.forEach((value, key) => {
        responseHeaders[key] = value
      })
      console.log(`[${new Date().toISOString()}] 📋 响应头:`, JSON.stringify(responseHeaders, null, 2))

      // 在 debug 级别下记录响应体
      if (envConfigManager.shouldLog('debug')) {
        const contentType = providerResponse.headers.get('content-type') || ''
        const isStream = contentType.includes('text/event-stream')

        if (isStream) {
          if (providerResponse.body) {
            const [logStream, processStream] = providerResponse.body.tee()

            // 在后台异步记录流式响应的合成内容
            ;(async () => {
              try {
                const fullBody = await new Response(logStream).text()
                if (fullBody.trim().length > 0) {
                  let synthesizedContent = ''
                  const toolCallAccumulator = new Map<number, { id?: string; name?: string; arguments?: string }>()
                  const lines = fullBody.trim().split('\n')
                  let parseFailed = false

                  for (const line of lines) {
                    const trimmedLine = line.trim()
                    // 使用正则匹配 SSE data 字段，支持 'data:' 和 'data: ' 格式
                    const dataMatch = trimmedLine.match(/^data:\s*(.*)$/)
                    if (!dataMatch) continue

                    const jsonStr = dataMatch[1].trim()
                    if (jsonStr === '[DONE]') continue

                    try {
                      const data = JSON.parse(jsonStr)

                      if (upstream.serviceType === 'gemini') {
                        if (data.candidates && data.candidates[0]?.content?.parts) {
                          for (const part of data.candidates[0].content.parts) {
                            if (part.text) {
                              synthesizedContent += part.text
                            }
                            if (part.functionCall) {
                              const fc = part.functionCall
                              synthesizedContent += `\nTool Call: ${fc.name}(${JSON.stringify(fc.args)})`
                            }
                          }
                        }
                      } else if (upstream.serviceType === 'openai' || upstream.serviceType === 'openaiold') {
                        if (data.choices && data.choices[0]?.delta?.content) {
                          synthesizedContent += data.choices[0].delta.content
                        }
                        if (data.choices && data.choices[0]?.delta?.tool_calls) {
                          for (const toolCall of data.choices[0].delta.tool_calls) {
                            const index = toolCall.index ?? 0
                            if (!toolCallAccumulator.has(index)) {
                              toolCallAccumulator.set(index, {})
                            }
                            const accumulated = toolCallAccumulator.get(index)!
                            if (toolCall.id) accumulated.id = toolCall.id
                            if (toolCall.function?.name) accumulated.name = toolCall.function.name
                            if (toolCall.function?.arguments) {
                              accumulated.arguments = (accumulated.arguments || '') + toolCall.function.arguments
                            }
                          }
                        }
                      } else if (upstream.serviceType === 'claude') {
                        if (data.type === 'content_block_delta') {
                          if (data.delta?.type === 'text_delta' && data.delta.text) {
                            synthesizedContent += data.delta.text
                          } else if (data.delta?.type === 'input_json_delta' && data.delta.partial_json) {
                            // 累积工具调用的JSON片段
                            const blockIndex = data.index ?? 0
                            if (!toolCallAccumulator.has(blockIndex)) {
                              toolCallAccumulator.set(blockIndex, { arguments: '' })
                            }
                            const accumulated = toolCallAccumulator.get(blockIndex)!
                            accumulated.arguments = (accumulated.arguments || '') + data.delta.partial_json
                          }
                        } else if (data.type === 'content_block_start' && data.content_block?.type === 'tool_use') {
                          // 记录工具调用的基本信息
                          const blockIndex = data.index ?? 0
                          if (!toolCallAccumulator.has(blockIndex)) {
                            toolCallAccumulator.set(blockIndex, {})
                          }
                          const accumulated = toolCallAccumulator.get(blockIndex)!
                          accumulated.id = data.content_block.id
                          accumulated.name = data.content_block.name
                        }
                      }
                    } catch (e) {
                      // 如果任何一个块解析失败，就放弃合成，回退到打印原始日志
                      parseFailed = true
                      break
                    }
                  }

                  if (toolCallAccumulator.size > 0) {
                    let toolCallsString = ''
                    for (const [index, tool] of toolCallAccumulator.entries()) {
                      const args = tool.arguments || '{}'
                      const name = tool.name || 'unknown_function'
                      const id = tool.id || `tool_${index}`
                      try {
                        const parsedArgs = JSON.parse(args)
                        toolCallsString += `\nTool Call: ${name}(${JSON.stringify(parsedArgs)}) [ID: ${id}]`
                      } catch (e) {
                        toolCallsString += `\nTool Call: ${name}(${args}) [ID: ${id}]`
                      }
                    }
                    synthesizedContent += toolCallsString
                  }

                  if (synthesizedContent.trim() && !parseFailed) {
                    console.log(
                      `[${new Date().toISOString()}] 🛰️  上游流式响应合成内容:\n---\n${synthesizedContent.trim()}\n---`
                    )
                  } else {
                    // 如果合成失败或内容为空，则打印原始响应体
                    console.log(
                      `[${new Date().toISOString()}] 🛰️  上游流式响应体 (完整):\n---\n${fullBody.trim()}\n---`
                    )
                  }
                }
              } catch (e) {
                console.error(`[${new Date().toISOString()}] 💥 日志流读取错误:`, e)
              }
            })()

            // 创建一个新的 Response 对象，用于后续处理
            providerResponse = new Response(processStream, {
              status: providerResponse.status,
              statusText: providerResponse.statusText,
              headers: providerResponse.headers
            })
          }
        } else {
          // 对于非流式响应，克隆并记录
          try {
            const responseClone = providerResponse.clone()
            const body = await responseClone.text()
            if (body.length > 0) {
              console.log(
                `[${new Date().toISOString()}] 📦 响应体:`,
                body.length > 1000 ? body.substring(0, 1000) + '...' : body
              )
            }
          } catch (error) {
            console.log(`[${new Date().toISOString()}] 📦 响应体: [无法读取 - ${(error as Error).message}]`)
          }
        }
      }
    }

    // 协议转换：Provider -> Claude
    const claudeResponse = await providerImpl.convertToClaudeResponse(providerResponse)

    res.status(claudeResponse.status)
    claudeResponse.headers.forEach((value, key) => {
      res.setHeader(key, value)
    })

    // 监听响应完成事件以记录时间
    res.on('finish', () => {
      if (envConfigManager.getConfig().enableResponseLogs) {
        const responseTime = Date.now() - startTime
        console.log(
          `[${new Date().toISOString()}] ${isDevelopment ? '⏱️' : ''} 响应完成: ${responseTime}ms, 状态: ${claudeResponse.status}`
        )
      }
    })

    // 监听响应关闭事件（例如客户端断开连接或流错误）
    res.on('close', () => {
      if (!res.writableFinished) {
        if (envConfigManager.getConfig().enableResponseLogs) {
          const responseTime = Date.now() - startTime
          console.log(
            `[${new Date().toISOString()}] ${isDevelopment ? '⏱️' : ''} 响应中断: ${responseTime}ms, 状态: ${claudeResponse.status}`
          )
        }
      }
    })

    if (claudeResponse.body) {
      const nodeStream = Readable.fromWeb(claudeResponse.body as any)
      nodeStream.on('error', error => {
        // 这个错误来自上游流（例如，通过 controller.error() 抛出）
        // 我们在这里记录它，因为主 catch 块无法捕获异步流错误
        console.error(`[${new Date().toISOString()}] 💥 流式传输期间发生错误:`, error.message)
        // pipe 会自动处理销毁 res，所以我们不需要手动操作
      })
      nodeStream.pipe(res)
    } else {
      res.end()
    }
  } catch (error) {
    console.error('服务器错误:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// 开发模式文件监听
function setupDevelopmentWatchers() {
  if (!isDevelopment || isManagedByRunner) return

  // 源码文件监听
  const sourceWatcher = chokidar.watch(['src/**/*.ts'], {
    ignored: [/node_modules/, 'config.json'],
    persistent: true,
    ignoreInitial: true
  })

  sourceWatcher.on('change', filePath => {
    console.log(`\n[${new Date().toISOString()}] 📝 检测到源码文件变化: ${filePath}`)
    console.log(`[${new Date().toISOString()}] 🔄 请手动重启服务器以应用更改`)
  })

  sourceWatcher.on('add', filePath => {
    console.log(`\n[${new Date().toISOString()}] ➕ 检测到新源码文件: ${filePath}`)
    console.log(`[${new Date().toISOString()}] 🔄 请手动重启服务器以应用更改`)
  })

  sourceWatcher.on('unlink', filePath => {
    console.log(`\n[${new Date().toISOString()}] 🗑️ 检测到源码文件删除: ${filePath}`)
    console.log(`[${new Date().toISOString()}] 🔄 请手动重启服务器以应用更改`)
  })

  // 环境变量文件监听
  const envWatcher = chokidar.watch(['../.env', '../.env.example'], {
    persistent: true,
    ignoreInitial: true
  })

  envWatcher.on('change', filePath => {
    console.log(`\n[${new Date().toISOString()}] 🌍 检测到环境变量文件变化: ${filePath}`)
    console.log(`[${new Date().toISOString()}] 🔄 环境变量变化需要重启服务器`)
  })

  console.log(`[${new Date().toISOString()}] 🔍 开发模式文件监听已启动`)
}

// 启动服务器
const envConfig = envConfigManager.getConfig()

// 优雅关闭处理
process.on('SIGINT', () => {
  console.log('\n正在关闭服务器...')
  process.exit(0)
})

process.on('SIGTERM', () => {
  console.log('\n正在关闭服务器...')
  process.exit(0)
})

// 设置开发模式监听
setupDevelopmentWatchers()

app.listen(envConfig.port, () => {
  console.log(`\n🚀 Claude API代理服务器已启动`)
  console.log(`📍 本地地址: http://localhost:${envConfig.port}`)
  console.log(`🌐 管理界面: http://localhost:${envConfig.port}`)
  console.log(`📋 统一入口: POST /v1/messages`)
  console.log(`💚 健康检查: GET ${envConfig.healthCheckPath}`)

  if (isDevelopment) {
    console.log(`🔧 开发信息: GET /admin/dev/info`)
    console.log(
      `⚙️  当前配置: ${configManager.getCurrentUpstream().name || configManager.getCurrentUpstream().serviceType} - ${configManager.getCurrentUpstream().baseUrl}`
    )
    console.log(`🔧 配置管理: bun run config --help`)
    console.log(`📊 环境: ${envConfig.nodeEnv}`)
    console.log(`🔍 开发模式 - 详细日志已启用`)

    console.log(`\n📁 文件监听状态:`)
    if (isManagedByRunner) {
      console.log(`   - 源码/环境变量: 监听中 (由 dev-runner 自动重启)`)
      console.log(`   - 配置文件: 监听中 (自动热重载)`)
    } else {
      console.log(`   - 源码/环境变量: 监听中 (变化需手动重启)`)
      console.log(`   - 配置文件: 监听中 (自动热重载)`)
    }

    console.log(`\n💡 提示:`)
    if (isManagedByRunner) {
      console.log(`   - 源码和环境变量文件变化将自动重启服务器。`)
    } else {
      console.log(`   - 推荐使用 'bun run dev' 以获得源码修改后自动重启功能。`)
      console.log(`   - 源码或环境变量文件变化需要手动重启服务器。`)
    }
    console.log(`   - 配置文件(config.json)变化会自动重载，无需重启。`)
    console.log(`   - 使用 Ctrl+C 停止服务器。\n`)
  } else {
    console.log(`📊 环境: ${envConfig.nodeEnv}`)
    console.log(`\n💡 提示: 使用 Ctrl+C 停止服务器\n`)
  }
})

export default app
