import fs from 'fs'
import path from 'path'
import { redisCache } from './redis'
import { maskApiKey } from './utils'

export interface UpstreamConfig {
    baseUrl: string
    apiKeys: string[]
    serviceType: 'gemini' | 'openai' | 'openaiold' | 'claude' | 'custom'
    name?: string
    description?: string // 备注字段，用于记录渠道详细信息
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
    loadBalance: 'failover'
}

class ConfigManager {
    private config: Config
    private requestCount: number = 0
    private watcher: fs.FSWatcher | null = null

    constructor(enableWatcher: boolean = true) {
        this.config = this.loadConfig()
        if (enableWatcher) {
            this.startConfigWatcher()
            this.initRedis()
        }
    }

    private async initRedis(): Promise<void> {
        await redisCache.connect()
        // 订阅配置更新事件
        redisCache.subscribe('config:updated', (message: string) => {
            try {
                const newConfig = JSON.parse(message)
                this.config = newConfig
                console.log(`[${new Date().toISOString()}] 通过Redis收到配置更新`)
            } catch (error) {
                console.warn('Redis配置更新解析失败:', error)
            }
        })
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
        console.log(`[${new Date().toISOString()}] 配置已重载`)
        // 通过Redis发布配置更新（仅在有监听器时）
        if (this.watcher) {
            redisCache.publish('config:updated', JSON.stringify(this.config))
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

    removeUpstream(index: number): void {
        if (index >= 0 && index < this.config.upstream.length) {
            const removed = this.config.upstream.splice(index, 1)[0]
            if (this.config.currentUpstream >= this.config.upstream.length) {
                this.config.currentUpstream = Math.max(0, this.config.upstream.length - 1)
            }
            this.saveConfig(this.config)
            console.log(`已删除上游: ${removed.name || removed.serviceType}`)
        } else {
            throw new Error('无效的上游索引')
        }
    }

    updateUpstream(index: number, upstream: Partial<UpstreamConfig>): void {
        if (index >= 0 && index < this.config.upstream.length) {
            this.config.upstream[index] = { ...this.config.upstream[index], ...upstream }
            this.saveConfig(this.config)
            console.log(`已更新上游: ${this.config.upstream[index].name || this.config.upstream[index].serviceType}`)
        } else {
            throw new Error('无效的上游索引')
        }
    }

    addApiKey(index: number, apiKey: string): void {
        if (index >= 0 && index < this.config.upstream.length) {
            if (!this.config.upstream[index].apiKeys.includes(apiKey)) {
                this.config.upstream[index].apiKeys.push(apiKey)
                this.saveConfig(this.config)
                console.log(`已添加API密钥到上游 ${index}`)
            } else {
                console.log('API密钥已存在')
            }
        } else {
            throw new Error('无效的上游索引')
        }
    }

    removeApiKey(index: number, apiKey: string): void {
        if (index >= 0 && index < this.config.upstream.length) {
            const keyIndex = this.config.upstream[index].apiKeys.indexOf(apiKey)
            if (keyIndex > -1) {
                this.config.upstream[index].apiKeys.splice(keyIndex, 1)
                this.saveConfig(this.config)
                console.log(`已删除API密钥从上游 ${index}`)
            } else {
                console.log('API密钥不存在')
            }
        } else {
            throw new Error('无效的上游索引')
        }
    }

    setUpstream(indexOrName: number | string): void {
        let targetIndex: number
        
        if (typeof indexOrName === 'string') {
            // 按名称查找
            const found = this.config.upstream.findIndex(upstream => 
                upstream.name?.toLowerCase() === indexOrName.toLowerCase()
            )
            if (found === -1) {
                throw new Error(`未找到名称为 "${indexOrName}" 的上游`)
            }
            targetIndex = found
        } else {
            // 按索引查找
            if (indexOrName < 0 || indexOrName >= this.config.upstream.length) {
                throw new Error('无效的上游索引')
            }
            targetIndex = indexOrName
        }
        
        this.config.currentUpstream = targetIndex
        this.saveConfig(this.config)
        console.log(`已切换到上游: ${this.config.upstream[targetIndex].name || this.config.upstream[targetIndex].serviceType}`)
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

        const currentUpstream = this.config.upstream[this.config.currentUpstream]
        if (!currentUpstream || currentUpstream.apiKeys.length === 0) {
            // 如果当前选定的上游不可用，则抛出错误，而不是自动切换
            throw new Error(
                `当前选定的上游 "${currentUpstream?.name || this.config.currentUpstream}" 没有可用的API密钥`
            )
        }

        return currentUpstream
    }

    async getNextApiKey(upstream: UpstreamConfig): Promise<string> {
        if (upstream.apiKeys.length === 0) {
            throw new Error(`上游 "${upstream.name}" 没有可用的API密钥`)
        }

        const keys = upstream.apiKeys

        switch (this.config.loadBalance) {
            case 'round-robin': {
                if (redisCache.isAvailable()) {
                    const count = await redisCache.increment('config:request_count')
                    if (count > 0) {
                        // Redis的INCR从1开始，所以需要-1来匹配0索引的数组
                        return keys[(count - 1) % keys.length]
                    }
                }
                // Redis不可用或increment失败时的回退逻辑
                console.warn(`[${new Date().toISOString()}] Redis不可用或计数失败，回退到内存轮询`)
                this.requestCount++
                return keys[this.requestCount % keys.length]
            }
            case 'random':
                return keys[Math.floor(Math.random() * keys.length)]
            case 'failover':
            default:
                // 故障转移策略对于密钥级别，我们理解为总是使用第一个密钥。
                return keys[0]
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
            console.log(`        API密钥数量: ${keyCount}`)
            if (keyCount > 0) {
                console.log(`        API密钥: ${upstream.apiKeys.map(key => maskApiKey(key)).join(', ')}`)
            }
        })
    }
}

export const configManager = new ConfigManager(true)
export const configManagerCLI = new ConfigManager(false)
