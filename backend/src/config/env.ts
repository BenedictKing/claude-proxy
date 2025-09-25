import dotenv from 'dotenv'

// 加载环境变量
dotenv.config()

export interface EnvConfig {
  port: number
  nodeEnv: string
  proxyAccessKey: string
  loadBalanceStrategy: 'round-robin' | 'random' | 'failover'
  logLevel: 'error' | 'warn' | 'info' | 'debug'
  enableRequestLogs: boolean
  enableResponseLogs: boolean
  requestTimeout: number
  maxConcurrentRequests: number
  enableCors: boolean
  corsOrigin: string
  enableRateLimit: boolean
  rateLimitWindow: number
  rateLimitMaxRequests: number
  healthCheckEnabled: boolean
  healthCheckPath: string
}

class EnvConfigManager {
  private config: EnvConfig

  constructor() {
    this.config = this.loadConfig()
  }

  private loadConfig(): EnvConfig {
    return {
      port: parseInt(process.env.PORT || '3000'),
      nodeEnv: process.env.NODE_ENV || 'development',
      proxyAccessKey: process.env.PROXY_ACCESS_KEY || 'your-proxy-access-key',
      loadBalanceStrategy: (process.env.LOAD_BALANCE_STRATEGY || 'failover') as 'round-robin' | 'random' | 'failover',
      logLevel: (process.env.LOG_LEVEL || 'info') as 'error' | 'warn' | 'info' | 'debug',
      enableRequestLogs: process.env.ENABLE_REQUEST_LOGS !== 'false',
      enableResponseLogs: process.env.ENABLE_RESPONSE_LOGS !== 'false',
      requestTimeout: parseInt(process.env.REQUEST_TIMEOUT || '30000'),
      maxConcurrentRequests: parseInt(process.env.MAX_CONCURRENT_REQUESTS || '100'),
      enableCors: process.env.ENABLE_CORS !== 'false',
      corsOrigin: process.env.CORS_ORIGIN || '*',
      enableRateLimit: process.env.ENABLE_RATE_LIMIT === 'true',
      rateLimitWindow: parseInt(process.env.RATE_LIMIT_WINDOW || '60000'),
      rateLimitMaxRequests: parseInt(process.env.RATE_LIMIT_MAX_REQUESTS || '100'),
      healthCheckEnabled: process.env.HEALTH_CHECK_ENABLED !== 'false',
      healthCheckPath: process.env.HEALTH_CHECK_PATH || '/health'
    }
  }

  getConfig(): EnvConfig {
    return this.config
  }

  isDevelopment(): boolean {
    return this.config.nodeEnv === 'development'
  }

  isProduction(): boolean {
    return this.config.nodeEnv === 'production'
  }

  shouldLog(level: 'error' | 'warn' | 'info' | 'debug'): boolean {
    const levels = ['error', 'warn', 'info', 'debug']
    const currentLevelIndex = levels.indexOf(this.config.logLevel)
    const requestLevelIndex = levels.indexOf(level)
    return requestLevelIndex <= currentLevelIndex
  }
}

export const envConfigManager = new EnvConfigManager()
