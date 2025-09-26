import { Router } from 'express'
import { Agent, fetch as undiciFetch } from 'undici'
import { configManager } from '../config/config'
import { maskApiKey } from '../utils/index'
import { envConfigManager } from '../config/env'

const router = Router()

// Web管理界面访问控制中间件
const adminAuthMiddleware = (req: any, res: any, next: any) => {
  // 获取访问密钥
  let providedApiKey = req.headers['x-api-key'] || req.headers['authorization']

  // 移除 Bearer 前缀（如果有）
  if (providedApiKey && typeof providedApiKey === 'string') {
    providedApiKey = providedApiKey.replace(/^bearer\s+/i, '')
  }

  const expectedApiKey = envConfigManager.getConfig().proxyAccessKey

  if (!providedApiKey || providedApiKey !== expectedApiKey) {
    console.warn(`[${new Date().toISOString()}] 🔒 Web管理界面访问密钥验证失败 - IP: ${req.ip}`)
    return res.status(401).json({ 
      error: 'Unauthorized', 
      message: '管理界面访问需要有效的API密钥'
    })
  }

  next()
}

// 应用访问控制中间件到所有管理API
router.use('/api', adminAuthMiddleware)

// 获取所有渠道
router.get('/api/channels', (req, res) => {
  try {
    const config = configManager.getConfig()
    res.json({
      channels: config.upstream.map((u, index) => ({
        ...u,
        apiKeys: u.apiKeys, // 返回原始密钥给前端
        index,
        latency: null,
        status: 'unknown'
      })),
      current: config.currentUpstream,
      loadBalance: config.loadBalance
    })
  } catch (error) {
    res.status(500).json({ error: error instanceof Error ? error.message : '未知错误' })
  }
})

// 添加渠道
router.post('/api/channels', (req, res) => {
  try {
    const { name, serviceType, baseUrl, apiKeys, description, website, insecureSkipVerify, modelMapping } = req.body
    
    if (!name || !serviceType || !baseUrl) {
      return res.status(400).json({ error: '缺少必填字段' })
    }

    // 可选官网校验
    if (website) {
      try { new URL(website) } catch { return res.status(400).json({ error: '官网URL无效' }) }
    }

    configManager.addUpstream({
      name,
      serviceType,
      baseUrl,
      apiKeys: apiKeys || [],
      description,
      website,
      insecureSkipVerify,
      modelMapping
    })
    
    res.json({ success: true })
  } catch (error) {
    res.status(400).json({ error: error instanceof Error ? error.message : '未知错误' })
  }
})

// 更新渠道
router.put('/api/channels/:id', (req, res) => {
  try {
    const id = parseInt(req.params.id)
    const config = configManager.getConfig()
    
    if (id < 0 || id >= config.upstream.length) {
      return res.status(404).json({ error: '渠道未找到' })
    }

    // 校验官网地址（可选）
    if (req.body && typeof req.body.website === 'string') {
      if (req.body.website.trim() !== '') {
        try { new URL(req.body.website) } catch { return res.status(400).json({ error: '官网URL无效' }) }
      }
    }

    // 准备更新数据，直接使用前端传来的原始密钥
    const updateData = {
      ...req.body,
      insecureSkipVerify: !!req.body.insecureSkipVerify,
      apiKeys: req.body.apiKeys || []
    };

    // 使用准备好的数据更新配置
    configManager.updateUpstream(id, updateData);
    
    res.json({ success: true })
  } catch (error) {
    res.status(400).json({ error: error instanceof Error ? error.message : '未知错误' })
  }
})

// 删除渠道
router.delete('/api/channels/:id', (req, res) => {
  try {
    const id = parseInt(req.params.id)
    configManager.removeUpstream(id)
    res.json({ success: true })
  } catch (error) {
    res.status(400).json({ error: error instanceof Error ? error.message : '未知错误' })
  }
})

// 设为当前渠道
router.post('/api/channels/:id/current', (req, res) => {
  try {
    const id = parseInt(req.params.id)
    configManager.setUpstream(id)
    res.json({ success: true })
  } catch (error) {
    res.status(400).json({ error: error instanceof Error ? error.message : '未知错误' })
  }
})

// 添加API密钥
router.post('/api/channels/:id/keys', (req, res) => {
  try {
    const id = parseInt(req.params.id)
    const { apiKey } = req.body
    
    if (!apiKey) {
      return res.status(400).json({ error: 'API密钥不能为空' })
    }

    configManager.addApiKey(id, apiKey)
    res.json({ success: true })
  } catch (error) {
    res.status(400).json({ error: error instanceof Error ? error.message : '未知错误' })
  }
})

// 删除API密钥
router.delete('/api/channels/:id/keys/:key', (req, res) => {
  try {
    const id = parseInt(req.params.id)
    const apiKey = decodeURIComponent(req.params.key) // 前端现在传递原始密钥
    
    // 获取当前渠道配置
    const config = configManager.getConfig()
    if (id < 0 || id >= config.upstream.length) {
      return res.status(404).json({ error: '渠道未找到' })
    }
    
    configManager.removeApiKey(id, apiKey)
    res.json({ success: true })
  } catch (error) {
    res.status(400).json({ error: error instanceof Error ? error.message : '未知错误' })
  }
})

// 测试延迟
router.get('/api/ping/:id', async (req, res) => {
  try {
    const id = parseInt(req.params.id)
    const config = configManager.getConfig()
    const channel = config.upstream[id]
    
    if (!channel) {
      return res.status(404).json({ error: '渠道未找到' })
    }
    
    const startTime = Date.now()
    
    try {
      const controller = new AbortController()
      const timeoutId = setTimeout(() => controller.abort(), 5000)
      
      // 构建测试URL
      let testUrl = channel.baseUrl
      if (!testUrl.endsWith('/')) {
        testUrl += '/'
      }
      
      // 根据服务类型选择合适的健康检查端点
      switch (channel.serviceType) {
        case 'openai':
        case 'openaiold':
          testUrl += 'models'
          break
        case 'gemini':
          testUrl += 'models'
          break
        case 'claude':
          // Claude API 不支持 HEAD 请求，使用基础连通性测试
          break
        default:
          break
      }
      
      const isBun = typeof (globalThis as any).Bun !== 'undefined'
      if (isBun) {
        const bunOpts: any = {}
        if (channel.insecureSkipVerify) {
          bunOpts.tls = { rejectUnauthorized: false }
        }
        await fetch(testUrl, { method: 'HEAD', signal: controller.signal, ...bunOpts } as any)
      } else {
        const dispatcher = channel.insecureSkipVerify
          ? new Agent({ connect: { rejectUnauthorized: false, checkServerIdentity: () => undefined } as any })
          : undefined
        await undiciFetch(testUrl, { method: 'HEAD', signal: controller.signal, dispatcher } as any)
      }
      
      clearTimeout(timeoutId)
      const latency = Date.now() - startTime
      
      res.json({ 
        success: true, 
        latency,
        status: 'healthy'
      })
    } catch (fetchError) {
      const latency = Date.now() - startTime
      res.json({ 
        success: false, 
        latency,
        status: 'error',
        error: fetchError instanceof Error ? fetchError.message : '未知错误'
      })
    }
  } catch (error) {
    res.status(500).json({ error: error instanceof Error ? error.message : '未知错误' })
  }
})

// 测试所有渠道
router.get('/api/ping', async (req, res) => {
  const config = configManager.getConfig()
  const results = []
  
  for (let i = 0; i < config.upstream.length; i++) {
    const channel = config.upstream[i]
    const startTime = Date.now()
    
    try {
      const controller = new AbortController()
      const timeoutId = setTimeout(() => controller.abort(), 3000)
      
      let testUrl = channel.baseUrl
      if (!testUrl.endsWith('/')) {
        testUrl += '/'
      }
      
      await fetch(testUrl, {
        method: 'HEAD',
        signal: controller.signal
      })
      
      clearTimeout(timeoutId)
      
      results.push({
        id: i,
        name: channel.name || channel.serviceType,
        latency: Date.now() - startTime,
        status: 'healthy'
      })
    } catch (error) {
      results.push({
        id: i,
        name: channel.name || channel.serviceType,
        latency: Date.now() - startTime,
        status: 'error'
      })
    }
  }
  
  res.json(results)
})

// 更新负载均衡策略
router.put('/api/loadbalance', (req, res) => {
  try {
    const { strategy } = req.body
    
    if (!['round-robin', 'random', 'failover'].includes(strategy)) {
      return res.status(400).json({ error: '无效的负载均衡策略' })
    }

    configManager.setLoadBalance(strategy)
    res.json({ success: true })
  } catch (error) {
    res.status(400).json({ error: error instanceof Error ? error.message : '未知错误' })
  }
})

export default router
