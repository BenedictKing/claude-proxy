import fs from 'fs'
import path from 'path'
import { maskApiKey } from '../utils'

export interface UpstreamConfig {
  baseUrl: string
  apiKeys: string[]
  serviceType: 'gemini' | 'openai' | 'openaiold' | 'claude'
  name?: string
  description?: string // 备注字段，用于记录渠道详细信息
  insecureSkipVerify?: boolean // 新增：是否跳过TLS证书验证
  modelMapping?: {
    opus?: string
    sonnet?: string
    haiku?: string
  }
}

export interface Config {
  upstream: UpstreamConfig[]
  currentUpstream: number // 当前使用的上游索引
  loadBalance: 'round-robin' | 'random' | 'failover'
}

// 模型重定向工具函数
export function redirectModel(model: string, upstream: UpstreamConfig): string {
  if (!upstream.modelMapping) {
    return model
  }

  let modelType: 'opus' | 'sonnet' | 'haiku' | null = null

  if (model.includes('opus')) {
    modelType = 'opus'
  } else if (model.includes('sonnet')) {
    modelType = 'sonnet'
  } else if (model.includes('haiku')) {
    modelType = 'haiku'
  }

  if (modelType && upstream.modelMapping[modelType]) {
    return upstream.modelMapping[modelType]!
  }

  return model
}

const CONFIG_FILE = path.join(process.cwd(), 'config.json')

const DEFAULT_CONFIG: Config = {
  upstream: [
    {
      name: 'Gemini',
      baseUrl: 'https://generativelanguage.googleapis.com/v1beta',
      apiKeys: [],
      serviceType: 'gemini'
    }
  ],
  currentUpstream: 0,
  loadBalance: 'round-robin'
}

class ConfigManager {
  private config: Config
  private requestCount: number = 0
  private watcher: fs.FSWatcher | null = null
  // 失败密钥的内存缓存：记录密钥失败的时间戳
  private failedKeysCache: Map<string, { timestamp: number; failureCount: number }> = new Map()
  // 密钥恢复时间（毫秒）- 5分钟后重新尝试失败的密钥
  private readonly KEY_RECOVERY_TIME = 5 * 60 * 1000
  // 最大失败次数 - 超过此次数的密钥将被延长恢复时间
  private readonly MAX_FAILURE_COUNT = 3

  constructor(enableWatcher: boolean = true) {
    this.config = this.loadConfig()
    if (enableWatcher) {
      this.startConfigWatcher()
    }
    
    // 启动定期清理过期失败记录的定时器
    setInterval(() => {
      this.cleanupExpiredFailures()
    }, 60000) // 每分钟清理一次
  }

  public findUpstreamIndex(indexOrName: number | string): number {
    // 如果输入是数字字符串，先解析为数字
    if (typeof indexOrName === 'string') {
      const parsedIndex = parseInt(indexOrName, 10)
      if (!isNaN(parsedIndex)) {
        indexOrName = parsedIndex
      }
    }

    if (typeof indexOrName === 'string') {
      // 按名称查找
      const foundIndex = this.config.upstream.findIndex(
        upstream => upstream.name?.toLowerCase() === indexOrName.toLowerCase()
      )
      if (foundIndex === -1) {
        throw new Error(`未找到名称为 "${indexOrName}" 的上游`)
      }
      return foundIndex
    } else {
      // 按索引查找
      if (indexOrName < 0 || indexOrName >= this.config.upstream.length) {
        throw new Error(`无效的上游索引: ${indexOrName}`)
      }
      return indexOrName
    }
  }

  private startConfigWatcher(): void {
    try {
      this.watcher = fs.watch(CONFIG_FILE, eventType => {
        if (eventType === 'change') {
          console.log(`[${new Date().toISOString()}] 🔧 检测到配置文件变化，重载配置...`)
          this.reloadConfig()
        }
      })
      console.log(`[${new Date().toISOString()}] 🔧 配置文件监听已启动 (配置变更不会重启服务器)`)
    } catch (error) {
      console.warn(`[${new Date().toISOString()}] 配置文件监听启动失败:`, error)
    }
  }

  stopConfigWatcher(): void {
    if (this.watcher) {
      this.watcher.close()
      this.watcher = null
      console.log(`[${new Date().toISOString()}] 配置文件监听已停止`)
    }
  }

  reloadConfig(): void {
    this.config = this.loadConfig()
    const currentUpstream = this.getCurrentUpstream()
    console.log(`[${new Date().toISOString()}] 配置已重载`)
    console.log(`⚙️  当前配置: ${currentUpstream.name || currentUpstream.serviceType} - ${currentUpstream.baseUrl}`)
  }

  private loadConfig(): Config {
    try {
      if (fs.existsSync(CONFIG_FILE)) {
        const data = fs.readFileSync(CONFIG_FILE, 'utf-8')
        return JSON.parse(data)
      } else {
        this.saveConfig(DEFAULT_CONFIG)
        return DEFAULT_CONFIG
      }
    } catch (error) {
      console.error('加载配置文件失败，使用默认配置:', error)
      return DEFAULT_CONFIG
    }
  }

  private saveConfig(config: Config): void {
    try {
      fs.writeFileSync(CONFIG_FILE, JSON.stringify(config, null, 2))
    } catch (error) {
      console.error('保存配置文件失败:', error)
      throw error
    }
  }

  getConfig(): Config {
    return this.config
  }

  getCurrentUpstream(): UpstreamConfig {
    return this.config.upstream[this.config.currentUpstream]
  }

  addUpstream(upstream: UpstreamConfig): void {
    this.config.upstream.push(upstream)
    this.saveConfig(this.config)
    console.log(`已添加上游: ${upstream.name || upstream.serviceType} - ${upstream.baseUrl}`)
  }

  removeUpstream(indexOrName: number | string): void {
    const index = this.findUpstreamIndex(indexOrName)
    const removed = this.config.upstream.splice(index, 1)[0]
    if (this.config.currentUpstream >= this.config.upstream.length) {
      this.config.currentUpstream = Math.max(0, this.config.upstream.length - 1)
    }
    this.saveConfig(this.config)
    console.log(`已删除上游: ${removed.name || removed.serviceType}`)
  }

  updateUpstream(indexOrName: number | string, upstream: Partial<UpstreamConfig>): void {
    const index = this.findUpstreamIndex(indexOrName)
    this.config.upstream[index] = { ...this.config.upstream[index], ...upstream }
    this.saveConfig(this.config)
    console.log(`已更新上游: ${this.config.upstream[index].name || this.config.upstream[index].serviceType}`)
  }

  addApiKey(indexOrName: number | string, apiKey: string): void {
    const index = this.findUpstreamIndex(indexOrName)
    if (!this.config.upstream[index].apiKeys.includes(apiKey)) {
      this.config.upstream[index].apiKeys.push(apiKey)
      this.saveConfig(this.config)
      console.log(`已添加API密钥到上游 [${index}] ${this.config.upstream[index].name}`)
    } else {
      console.log('API密钥已存在')
    }
  }

  removeApiKey(indexOrName: number | string, apiKey: string): void {
    const index = this.findUpstreamIndex(indexOrName)
    const keyIndex = this.config.upstream[index].apiKeys.indexOf(apiKey)
    if (keyIndex > -1) {
      this.config.upstream[index].apiKeys.splice(keyIndex, 1)
      this.saveConfig(this.config)
      console.log(`已删除API密钥从上游 [${index}] ${this.config.upstream[index].name}`)
    } else {
      console.log('API密钥不存在')
    }
  }

  setUpstream(indexOrName: number | string): void {
    const targetIndex = this.findUpstreamIndex(indexOrName)
    this.config.currentUpstream = targetIndex
    this.saveConfig(this.config)
    console.log(
      `已切换到上游: ${this.config.upstream[targetIndex].name || this.config.upstream[targetIndex].serviceType}`
    )
  }

  setLoadBalance(strategy: 'round-robin' | 'random' | 'failover'): void {
    this.config.loadBalance = strategy
    this.saveConfig(this.config)
    console.log(`已设置负载均衡策略: ${strategy}`)
  }

  getNextUpstream(): UpstreamConfig {
    const upstreams = this.config.upstream.filter(u => u.apiKeys.length > 0)
    if (upstreams.length === 0) {
      throw new Error('没有配置任何带有API密钥的上游')
    }

    let selectedUpstream: UpstreamConfig

    switch (this.config.loadBalance) {
      case 'random': {
        const randomIndex = Math.floor(Math.random() * upstreams.length)
        selectedUpstream = upstreams[randomIndex]
        console.log(`[${new Date().toISOString()}] 随机选择上游: ${selectedUpstream.name}`)
        return selectedUpstream
      }
      case 'round-robin': {
        this.requestCount++
        const selectedIndex = (this.requestCount - 1) % upstreams.length
        selectedUpstream = upstreams[selectedIndex]
        console.log(`[${new Date().toISOString()}] 轮询选择上游: ${selectedUpstream.name}`)
        return selectedUpstream
      }
      case 'failover':
      default: {
        const currentUpstream = this.config.upstream[this.config.currentUpstream]
        if (!currentUpstream || currentUpstream.apiKeys.length === 0) {
          // 如果当前选定的上游不可用，则抛出错误，而不是自动切换
          throw new Error(`当前选定的上游 "${currentUpstream?.name || this.config.currentUpstream}" 没有可用的API密钥`)
        }
        return currentUpstream
      }
    }
  }

  // 清理过期的失败记录
  private cleanupExpiredFailures(): void {
    const now = Date.now()
    for (const [apiKey, failure] of this.failedKeysCache.entries()) {
      const recoveryTime = failure.failureCount > this.MAX_FAILURE_COUNT 
        ? this.KEY_RECOVERY_TIME * 2 // 频繁失败的密钥延长恢复时间
        : this.KEY_RECOVERY_TIME
      
      if (now - failure.timestamp > recoveryTime) {
        this.failedKeysCache.delete(apiKey)
        console.log(`[${new Date().toISOString()}] 🔄 API密钥 ${maskApiKey(apiKey)} 已从失败列表中恢复`)
      }
    }
  }

  // 标记API密钥失败
  markKeyAsFailed(apiKey: string): void {
    const existing = this.failedKeysCache.get(apiKey)
    if (existing) {
      existing.failureCount++
      existing.timestamp = Date.now()
    } else {
      this.failedKeysCache.set(apiKey, {
        timestamp: Date.now(),
        failureCount: 1
      })
    }
    
    const failure = this.failedKeysCache.get(apiKey)!
    const recoveryTime = failure.failureCount > this.MAX_FAILURE_COUNT 
      ? this.KEY_RECOVERY_TIME * 2 
      : this.KEY_RECOVERY_TIME
    
    console.log(`[${new Date().toISOString()}] ❌ 标记API密钥失败: ${maskApiKey(apiKey)} (失败次数: ${failure.failureCount}, 恢复时间: ${Math.round(recoveryTime / 60000)}分钟)`)
  }

  // 检查API密钥是否在失败列表中
  isKeyFailed(apiKey: string): boolean {
    const failure = this.failedKeysCache.get(apiKey)
    if (!failure) return false
    
    const now = Date.now()
    const recoveryTime = failure.failureCount > this.MAX_FAILURE_COUNT 
      ? this.KEY_RECOVERY_TIME * 2 
      : this.KEY_RECOVERY_TIME
    
    return (now - failure.timestamp) < recoveryTime
  }

  // 获取可用的API密钥列表（排除失败的密钥）
  getAvailableKeys(upstream: UpstreamConfig): string[] {
    return upstream.apiKeys.filter(key => !this.isKeyFailed(key))
  }

  getNextApiKey(upstream: UpstreamConfig, failedKeys: Set<string> = new Set()): string {
    if (upstream.apiKeys.length === 0) {
      throw new Error(`上游 "${upstream.name}" 没有可用的API密钥`)
    }

    // 综合考虑临时失败密钥和内存中的失败密钥
    const availableKeys = upstream.apiKeys.filter(key => 
      !failedKeys.has(key) && !this.isKeyFailed(key)
    )
    
    if (availableKeys.length === 0) {
      // 如果所有密钥都失效，检查是否有可以恢复的密钥
      const allFailedKeys = upstream.apiKeys.filter(key => failedKeys.has(key) || this.isKeyFailed(key))
      if (allFailedKeys.length === upstream.apiKeys.length) {
        // 如果所有密钥都在内存失败缓存中，尝试选择失败时间最早的密钥
        let oldestFailedKey: string | null = null
        let oldestTime = Date.now()
        
        for (const key of upstream.apiKeys) {
          if (!failedKeys.has(key)) { // 排除本次请求已经尝试过的密钥
            const failure = this.failedKeysCache.get(key)
            if (failure && failure.timestamp < oldestTime) {
              oldestTime = failure.timestamp
              oldestFailedKey = key
            }
          }
        }
        
        if (oldestFailedKey) {
          console.log(`[${new Date().toISOString()}] ⚠️ 所有密钥都失效，尝试最早失败的密钥: ${maskApiKey(oldestFailedKey)}`)
          return oldestFailedKey
        }
      }
      
      throw new Error(`上游 "${upstream.name}" 的所有API密钥都暂时不可用`)
    }

    switch (this.config.loadBalance) {
      case 'round-robin': {
        this.requestCount++
        const selectedKey = availableKeys[(this.requestCount - 1) % availableKeys.length]
        console.log(
          `[${new Date().toISOString()}] 轮询选择密钥 ${maskApiKey(selectedKey)} (${((this.requestCount - 1) % availableKeys.length) + 1}/${availableKeys.length})`
        )
        return selectedKey
      }
      case 'random': {
        const randomIndex = Math.floor(Math.random() * availableKeys.length)
        const selectedKey = availableKeys[randomIndex]
        console.log(
          `[${new Date().toISOString()}] 随机选择密钥 ${maskApiKey(selectedKey)} (${randomIndex + 1}/${availableKeys.length})`
        )
        return selectedKey
      }
      case 'failover':
      default: {
        const selectedKey = availableKeys[0]
        const keyIndex = upstream.apiKeys.indexOf(selectedKey) + 1
        console.log(`[${new Date().toISOString()}] 故障转移选择密钥 ${maskApiKey(selectedKey)} (${keyIndex}/${upstream.apiKeys.length})`)
        return selectedKey
      }
    }
  }

  showConfig(): void {
    const config = this.getConfig()
    console.log('当前配置:')
    console.log(`  负载均衡策略: ${config.loadBalance}`)
    console.log(`  当前上游索引: ${config.currentUpstream}`)
    console.log(`  上游列表:`)
    config.upstream.forEach((upstream, index) => {
      const current = index === config.currentUpstream ? ' (当前)' : ''
      const keyCount = upstream.apiKeys.length
      console.log(`    [${index}] ${upstream.name || upstream.serviceType}${current}`)
      console.log(`        类型: ${upstream.serviceType}`)
      console.log(`        地址: ${upstream.baseUrl}`)
      if (upstream.description) {
        console.log(`        备注: ${upstream.description}`)
      }
      if (upstream.insecureSkipVerify) {
        console.log(`        安全: 跳过TLS证书验证`)
      }
      console.log(`        API密钥数量: ${keyCount}`)
      if (keyCount > 0) {
        console.log(`        API密钥: ${upstream.apiKeys.map(key => maskApiKey(key)).join(', ')}`)
      }
    })
  }
}

export const configManager = new ConfigManager(true)
export const configManagerCLI = new ConfigManager(false)
