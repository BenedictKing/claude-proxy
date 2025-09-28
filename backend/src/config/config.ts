import fs from 'fs'
import path from 'path'
import { maskApiKey } from '../utils'

export interface UpstreamConfig {
  baseUrl: string
  apiKeys: string[]
  serviceType: 'gemini' | 'openai' | 'openaiold' | 'claude'
  name?: string
  description?: string // 备注字段，用于记录渠道详细信息
  website?: string // 官方网站/控制台入口，供前端直接打开
  insecureSkipVerify?: boolean // 新增：是否跳过TLS证书验证
  modelMapping?: Record<string, string> // 模型重定向映射
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

  // 直接检查映射中是否有匹配的键
  if (upstream.modelMapping[model]) {
    return upstream.modelMapping[model]
  }

  // 如果没有直接匹配，检查是否有包含关系的映射
  for (const [sourceModel, targetModel] of Object.entries(upstream.modelMapping)) {
    if (model.includes(sourceModel) || sourceModel.includes(model)) {
      return targetModel
    }
  }

  return model
}

const CONFIG_DIR = path.join(process.cwd(), '.config')
const CONFIG_FILE = path.join(CONFIG_DIR, 'config.json')
const BACKUP_DIR = path.join(CONFIG_DIR, 'backups')
const MAX_BACKUPS = 10

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
          try {
            this.reloadConfig()
          } catch (error) {
            console.warn(`[${new Date().toISOString()}] 配置重载失败（已忽略以保持服务运行）:`, error)
          }
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
    console.log(`[${new Date().toISOString()}] 配置已重载`)
    try {
      if (!this.config.upstream || this.config.upstream.length === 0) {
        console.warn(`⚠️  当前未配置任何上游渠道`)
        return
      }
      const currentUpstream = this.getCurrentUpstream()
      console.log(`⚙️  当前配置: ${currentUpstream.name || currentUpstream.serviceType} - ${currentUpstream.baseUrl}`)
    } catch (e) {
      console.warn(`⚠️  当前上游配置不可用: ${e instanceof Error ? e.message : String(e)}`)
    }
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
      // 确保配置目录存在
      if (!fs.existsSync(CONFIG_DIR)) {
        fs.mkdirSync(CONFIG_DIR, { recursive: true })
      }
      // 在写入新配置前，为现有配置创建时间戳备份并进行保留策略（最多10个）
      this.backupCurrentConfig()

      fs.writeFileSync(CONFIG_FILE, JSON.stringify(config, null, 2))
    } catch (error) {
      console.error('保存配置文件失败:', error)
      throw error
    }
  }

  // 备份当前配置文件到专用目录，并保留最近的10个备份
  private backupCurrentConfig(): void {
    try {
      if (!fs.existsSync(CONFIG_FILE)) return // 首次生成时无备份

      // 确保备份目录存在
      if (!fs.existsSync(BACKUP_DIR)) {
        fs.mkdirSync(BACKUP_DIR, { recursive: true })
      }

      // 读取当前配置内容并写入到带时间戳的备份文件
      const content = fs.readFileSync(CONFIG_FILE, 'utf-8')
      const ts = new Date().toISOString().replace(/[:.]/g, '-')
      const backupFile = path.join(BACKUP_DIR, `config-${ts}.json`)
      fs.writeFileSync(backupFile, content)

      // 备份轮转：仅保留最近的 MAX_BACKUPS 个
      const entries = fs
        .readdirSync(BACKUP_DIR)
        .filter(f => f.startsWith('config-') && f.endsWith('.json'))
        .map(f => ({
          file: f,
          mtime: fs.statSync(path.join(BACKUP_DIR, f)).mtimeMs
        }))
        .sort((a, b) => b.mtime - a.mtime)

      if (entries.length > MAX_BACKUPS) {
        const toRemove = entries.slice(MAX_BACKUPS)
        for (const e of toRemove) {
          try {
            fs.unlinkSync(path.join(BACKUP_DIR, e.file))
          } catch (err) {
            console.warn('删除旧备份失败:', e.file, err)
          }
        }
      }
    } catch (err) {
      // 备份失败不应阻止配置写入，但要记录警告
      console.warn('备份配置文件失败（将继续写入新配置）:', err)
    }
  }

  getConfig(): Config {
    return this.config
  }

  getCurrentUpstream(): UpstreamConfig {
    if (!this.config.upstream || this.config.upstream.length === 0) {
      throw new Error('未配置任何上游渠道')
    }
    const upstream = this.config.upstream[this.config.currentUpstream]
    if (!upstream) {
      throw new Error(`当前渠道索引 ${this.config.currentUpstream} 无效`)
    }
    return upstream
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

  // 将当前上游中的某个API密钥移动到列表末尾（用于余额不足等情况的降级处理）
  deprioritizeApiKeyForCurrentUpstream(apiKey: string): void {
    const upstream = this.config.upstream[this.config.currentUpstream]
    const idx = upstream.apiKeys.indexOf(apiKey)
    if (idx === -1 || idx === upstream.apiKeys.length - 1) {
      return
    }
    upstream.apiKeys.splice(idx, 1)
    upstream.apiKeys.push(apiKey)
    this.saveConfig(this.config)
    console.log(`[${new Date().toISOString()}] 🔽 已将API密钥移动到末尾以降低优先级: ${maskApiKey(apiKey)}`)
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
    // 始终返回当前选中的渠道，负载均衡只应用于该渠道内的API密钥
    const currentUpstream = this.config.upstream[this.config.currentUpstream]
    if (!currentUpstream) {
      throw new Error(`当前渠道索引 ${this.config.currentUpstream} 无效`)
    }
    if (currentUpstream.apiKeys.length === 0) {
      throw new Error(`当前渠道 "${currentUpstream.name || currentUpstream.serviceType}" 没有配置API密钥`)
    }
    return currentUpstream
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
      if (upstream.website) {
        console.log(`        官网: ${upstream.website}`)
      }
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
