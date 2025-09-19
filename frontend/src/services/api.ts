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
  serviceType: 'openai' | 'openaiold' | 'gemini' | 'claude'
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
  private async request(url: string, options: RequestInit = {}): Promise<any> {
    const response = await fetch(`${API_BASE}${url}`, {
      headers: {
        'Content-Type': 'application/json',
        ...options.headers
      },
      ...options
    })

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Unknown error' }))
      throw new Error(error.error || 'Request failed')
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
}

export const api = new ApiService()
export default api
