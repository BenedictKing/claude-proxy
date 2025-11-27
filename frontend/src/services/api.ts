// API服务模块

// 从环境变量读取配置
const getApiBase = () => {
  // 在生产环境中，API调用会直接请求当前域名
  if (import.meta.env.PROD) {
    return '/api'
  }

  // 在开发环境中，支持从环境变量配置后端地址
  const backendUrl = import.meta.env.VITE_BACKEND_URL
  const apiBasePath = import.meta.env.VITE_API_BASE_PATH || '/api'

  if (backendUrl) {
    return `${backendUrl}${apiBasePath}`
  }

  // fallback到默认配置
  return '/api'
}

const API_BASE = getApiBase()

// 打印当前API配置（仅开发环境）
if (import.meta.env.DEV) {
  console.log('🔗 API Configuration:', {
    API_BASE,
    BACKEND_URL: import.meta.env.VITE_BACKEND_URL,
    IS_DEV: import.meta.env.DEV,
    IS_PROD: import.meta.env.PROD
  })
}

export interface Channel {
  name: string
  serviceType: 'openai' | 'openaiold' | 'gemini' | 'claude' | 'responses'
  baseUrl: string
  apiKeys: string[]
  description?: string
  website?: string
  insecureSkipVerify?: boolean
  modelMapping?: Record<string, string>
  latency?: number
  status?: 'healthy' | 'error' | 'unknown'
  index: number
  pinned?: boolean
}

export interface ChannelsResponse {
  channels: Channel[]
  current: number
  loadBalance: string
}

export interface PingResult {
  success: boolean
  latency: number
  status: string
  error?: string
}

class ApiService {
  private apiKey: string | null = null

  // 设置API密钥
  setApiKey(key: string | null) {
    this.apiKey = key
  }

  // 获取当前API密钥
  getApiKey(): string | null {
    return this.apiKey
  }

  // 从URL查询参数获取密钥
  getKeyFromUrl(): string | null {
    const params = new URLSearchParams(window.location.search)
    return params.get('key')
  }

  // 初始化密钥（从URL或localStorage）
  initializeAuth() {
    // 优先从URL获取密钥
    const urlKey = this.getKeyFromUrl()
    if (urlKey) {
      this.setApiKey(urlKey)
      // 保存到localStorage以便下次使用
      localStorage.setItem('proxyAccessKey', urlKey)

      // 清理URL中的key参数以提高安全性
      const url = new URL(window.location.href)
      url.searchParams.delete('key')
      window.history.replaceState({}, '', url.toString())

      return urlKey
    }

    // 从localStorage获取保存的密钥
    const savedKey = localStorage.getItem('proxyAccessKey')
    if (savedKey) {
      this.setApiKey(savedKey)
      return savedKey
    }

    return null
  }

  // 清除认证信息
  clearAuth() {
    this.apiKey = null
    localStorage.removeItem('proxyAccessKey')
  }

  private async request(url: string, options: RequestInit = {}): Promise<any> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string>)
    }

    // 添加API密钥到请求头
    if (this.apiKey) {
      headers['x-api-key'] = this.apiKey
    }

    const response = await fetch(`${API_BASE}${url}`, {
      ...options,
      headers
    })

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Unknown error' }))

      // 如果是401错误，清除本地认证信息并提示用户重新登录
      if (response.status === 401) {
        this.clearAuth()
        // 记录认证失败(前端日志)
        console.warn('🔒 认证失败 - 时间:', new Date().toISOString())
        throw new Error('认证失败，请重新输入访问密钥')
      }

      throw new Error(error.error || error.message || 'Request failed')
    }

    return response.json()
  }

  async getChannels(): Promise<ChannelsResponse> {
    return this.request('/channels')
  }

  async addChannel(channel: Omit<Channel, 'index' | 'latency' | 'status'>): Promise<void> {
    await this.request('/channels', {
      method: 'POST',
      body: JSON.stringify(channel)
    })
  }

  async updateChannel(id: number, channel: Partial<Channel>): Promise<void> {
    await this.request(`/channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(channel)
    })
  }

  async deleteChannel(id: number): Promise<void> {
    await this.request(`/channels/${id}`, {
      method: 'DELETE'
    })
  }

  async setCurrentChannel(id: number): Promise<void> {
    await this.request(`/channels/${id}/current`, {
      method: 'POST'
    })
  }

  async addApiKey(channelId: number, apiKey: string): Promise<void> {
    await this.request(`/channels/${channelId}/keys`, {
      method: 'POST',
      body: JSON.stringify({ apiKey })
    })
  }

  async removeApiKey(channelId: number, apiKey: string): Promise<void> {
    await this.request(`/channels/${channelId}/keys/${encodeURIComponent(apiKey)}`, {
      method: 'DELETE'
    })
  }

  async pingChannel(id: number): Promise<PingResult> {
    return this.request(`/ping/${id}`)
  }

  async pingAllChannels(): Promise<Array<{ id: number; name: string; latency: number; status: string }>> {
    return this.request('/ping')
  }

  async updateLoadBalance(strategy: string): Promise<void> {
    await this.request('/loadbalance', {
      method: 'PUT',
      body: JSON.stringify({ strategy })
    })
  }

  async updateResponsesLoadBalance(strategy: string): Promise<void> {
    await this.request('/responses/loadbalance', {
      method: 'PUT',
      body: JSON.stringify({ strategy })
    })
  }

  // ============== Responses 渠道管理 API ==============

  async getResponsesChannels(): Promise<ChannelsResponse> {
    return this.request('/responses/channels')
  }

  async addResponsesChannel(channel: Omit<Channel, 'index' | 'latency' | 'status'>): Promise<void> {
    await this.request('/responses/channels', {
      method: 'POST',
      body: JSON.stringify(channel)
    })
  }

  async updateResponsesChannel(id: number, channel: Partial<Channel>): Promise<void> {
    await this.request(`/responses/channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(channel)
    })
  }

  async deleteResponsesChannel(id: number): Promise<void> {
    await this.request(`/responses/channels/${id}`, {
      method: 'DELETE'
    })
  }

  async setCurrentResponsesChannel(id: number): Promise<void> {
    await this.request(`/responses/channels/${id}/current`, {
      method: 'POST'
    })
  }

  async addResponsesApiKey(channelId: number, apiKey: string): Promise<void> {
    await this.request(`/responses/channels/${channelId}/keys`, {
      method: 'POST',
      body: JSON.stringify({ apiKey })
    })
  }

  async removeResponsesApiKey(channelId: number, apiKey: string): Promise<void> {
    await this.request(`/responses/channels/${channelId}/keys/${encodeURIComponent(apiKey)}`, {
      method: 'DELETE'
    })
  }
}

export const api = new ApiService()
export default api
