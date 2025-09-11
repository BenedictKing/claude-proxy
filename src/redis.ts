import { createClient } from 'redis'

export class RedisCache {
    private client: any
    private isConnected: boolean = false

    constructor() {
        this.client = createClient({
            url: process.env.REDIS_URL || 'redis://localhost:6379/1'
        })

        this.client.on('error', (err: Error) => {
            console.warn('Redis连接错误:', err.message)
            this.isConnected = false
        })

        this.client.on('connect', () => {
            console.log('Redis连接已建立 (使用DB 1)')
            this.isConnected = true
        })
    }

    async connect(): Promise<void> {
        try {
            await this.client.connect()
            this.isConnected = true
        } catch (error) {
            console.warn('Redis连接失败，将使用内存模式:', error)
            this.isConnected = false
        }
    }

    async disconnect(): Promise<void> {
        if (this.isConnected) {
            await this.client.disconnect()
            this.isConnected = false
        }
    }

    async get(key: string): Promise<string | null> {
        if (!this.isConnected) return null
        try {
            return await this.client.get(key)
        } catch (error) {
            console.warn('Redis GET失败:', error)
            return null
        }
    }

    async set(key: string, value: string, ttl?: number): Promise<void> {
        if (!this.isConnected) return
        try {
            if (ttl) {
                await this.client.setEx(key, ttl, value)
            } else {
                await this.client.set(key, value)
            }
        } catch (error) {
            console.warn('Redis SET失败:', error)
        }
    }

    async del(key: string): Promise<void> {
        if (!this.isConnected) return
        try {
            await this.client.del(key)
        } catch (error) {
            console.warn('Redis DEL失败:', error)
        }
    }

    async increment(key: string): Promise<number> {
        if (!this.isConnected) return 0
        try {
            // 使用 INCR 命令实现原子递增
            return await this.client.incr(key)
        } catch (error) {
            console.warn('Redis INCR失败:', error)
            return 0
        }
    }

    async publish(channel: string, message: string): Promise<void> {
        if (!this.isConnected) return
        try {
            await this.client.publish(channel, message)
        } catch (error) {
            console.warn('Redis PUBLISH失败:', error)
        }
    }

    subscribe(channel: string, callback: (message: string) => void): void {
        if (!this.isConnected) return
        try {
            const subscriber = this.client.duplicate()
            subscriber.subscribe(channel, (message: string) => {
                callback(message)
            })
        } catch (error) {
            console.warn('Redis SUBSCRIBE失败:', error)
        }
    }

    isAvailable(): boolean {
        return this.isConnected
    }
}

export const redisCache = new RedisCache()
