import express from 'express'
import * as provider from './src/provider'
import * as gemini from './src/gemini'
import * as openaiold from './src/openaiold'
import * as openai from './src/openai'
import * as claude from './src/claude'
import { configManager, UpstreamConfig } from './src/config'
import { envConfigManager } from './src/env'
import { maskApiKey } from './src/utils'
import chokidar from 'chokidar'
import { Agent } from 'undici'

// 敏感头统一掩码配置与函数
const SENSITIVE_HEADER_KEYS = new Set(['authorization', 'x-api-key', 'x-goog-api-key'])
function maskHeaderValue(key: string, value: string): string {
  const lowerKey = key.toLowerCase()
  if (lowerKey === 'authorization') {
    const m = value.match(/^\s*Bearer\s+(.+)$/i)
    if (m) return `Bearer ${maskApiKey(m[1])}`
    return maskApiKey(value)
  }
  if (SENSITIVE_HEADER_KEYS.has(lowerKey)) {
    return maskApiKey(value)
  }
  return value
}

const app = express()
app.use(express.json({ limit: '50mb' }))

// 开发模式检测
const isDevelopment = process.env.NODE_ENV === 'development'

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
        console.log(`[${new Date().toISOString()}] 📋 请求体:`, JSON.stringify(req.body, null, 2))
        // 对请求头做敏感信息脱敏
        const sanitizedReqHeaders: { [key: string]: string } = {}
        Object.entries(req.headers).forEach(([k, v]) => {
          if (typeof v === 'string') {
            sanitizedReqHeaders[k] = maskHeaderValue(k, v)
          } else if (Array.isArray(v)) {
            sanitizedReqHeaders[k] = v.map(val => maskHeaderValue(k, val)).join(', ')
          }
        })
        console.log(`[${new Date().toISOString()}] 📥 请求头:`, JSON.stringify(sanitizedReqHeaders, null, 2))
      }
    }

    // 验证代理访问密钥
    let providedApiKey = req.headers['x-api-key'] || req.headers['authorization']

    // 移除 Bearer 前缀（如果有）
    if (providedApiKey && typeof providedApiKey === 'string' && providedApiKey.toLowerCase().startsWith('bearer ')) {
      providedApiKey = providedApiKey.substring(7)
    }

    const expectedApiKey = envConfigManager.getConfig().proxyAccessKey

    if (!providedApiKey || providedApiKey !== expectedApiKey) {
      if (envConfigManager.shouldLog('warn')) {
        console.warn(`[${new Date().toISOString()}] ${isDevelopment ? '🔒' : ''} 代理访问密钥验证失败`)
      }
      res.status(401).json({ error: 'Invalid proxy access key' })
      return
    }

    // 获取下一个上游和API密钥
    let upstream: UpstreamConfig
    let apiKey: string
    try {
      upstream = configManager.getNextUpstream()
      apiKey = configManager.getNextApiKey(upstream)
    } catch (error) {
      console.error('获取上游配置失败:', error)
      res.status(500).json({ error: '没有可用的上游配置或API密钥' })
      return
    }

    if (envConfigManager.shouldLog('info')) {
      console.log(
        `[${new Date().toISOString()}] ${isDevelopment ? '🎯' : ''} 使用上游: ${upstream.name || upstream.serviceType} - ${upstream.baseUrl}`
      )
      console.log(`[${new Date().toISOString()}] ${isDevelopment ? '🔑' : ''} 使用API密钥: ${maskApiKey(apiKey)}`)
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

    // 构造提供商所需的 Request 对象
    const headers = new Headers()
    Object.entries(req.headers).forEach(([key, value]) => {
      if (typeof value === 'string' && key.toLowerCase() !== 'x-api-key' && key.toLowerCase() !== 'authorization') {
        headers.set(key, value)
      } else if (Array.isArray(value)) {
        headers.set(key, value.join(', '))
      }
    })
    const incomingRequest = new Request('http://localhost/v1/messages', {
      method: 'POST',
      headers: headers,
      body: JSON.stringify(req.body)
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
      console.log(`[${new Date().toISOString()}] 📋 请求头:`, JSON.stringify(reqHeaders, null, 2))
      try {
        const body = await providerRequest.clone().text()
        if (body.length > 0) {
          console.log(
            `[${new Date().toISOString()}] 📦 请求体:`,
            body.length > 500 ? body.substring(0, 500) + '...' : body
          )
        }
      } catch (error) {
        console.log(`[${new Date().toISOString()}] 📦 请求体: [无法读取 - ${error.message}]`)
      }
    }

    // 根据配置决定是否跳过TLS验证
    const fetchOptions: any = {}
    if (upstream.insecureSkipVerify) {
      if (isDevelopment) {
        console.log(`[${new Date().toISOString()}] ⚠️ 正在跳过对 ${providerRequest.url} 的TLS证书验证`)
      }
      fetchOptions.dispatcher = new Agent({
        connect: {
          rejectUnauthorized: false
        }
      })
    }

    // 调用上游
    const providerResponse = await fetch(providerRequest, fetchOptions)

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
    }

    // 协议转换：Provider -> Claude
    const response = await providerImpl.convertToClaudeResponse(providerResponse)

    // 设置响应头并发送响应
    response.headers.forEach((value, key) => {
      res.setHeader(key, value)
    })
    const data = await response.text()
    res.status(response.status).send(data)

    if (envConfigManager.getConfig().enableResponseLogs) {
      const responseTime = Date.now() - startTime
      console.log(
        `[${new Date().toISOString()}] ${isDevelopment ? '⏱️' : ''} 响应时间: ${responseTime}ms, 状态: ${response.status}`
      )
    }
  } catch (error) {
    console.error('服务器错误:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// 开发模式文件监听
function setupDevelopmentWatchers() {
  if (!isDevelopment) return

  // 源码文件监听
  const sourceWatcher = chokidar.watch(['src/**/*.ts', 'server.ts'], {
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
  const envWatcher = chokidar.watch(['.env', '.env.example'], {
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
    console.log(`   🔍 源码文件: 监听中 (变化需手动重启)`)
    console.log(`   ⚙️  配置文件: 监听中 (自动重载)`)
    console.log(`   🌍 环境变量: 监听中 (变化需重启)`)
    console.log(`\n💡 提示:`)
    console.log(`   - 源码文件变化需要手动重启服务器`)
    console.log(`   - 配置文件变化会自动重载，无需重启`)
    console.log(`   - 环境变量变化需要重启服务器`)
    console.log(`   - 使用 Ctrl+C 停止服务器\n`)
  } else {
    console.log(`📊 环境: ${envConfig.nodeEnv}`)
    console.log(`\n💡 提示: 使用 Ctrl+C 停止服务器\n`)
  }
})

export default app
