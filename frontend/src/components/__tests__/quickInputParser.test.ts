/**
 * 快速添加渠道 - 输入解析测试
 *
 * 测试 isValidApiKey 和 isValidUrl 工具函数
 */

import { describe, it, expect } from 'vitest'
import { isValidApiKey, isValidUrl, parseQuickInput } from '../../utils/quickInputParser'

describe('API Key 识别', () => {
  describe('前缀格式 (xx-xxx / xx_xxx)', () => {
    it('应识别 sk- 前缀', () => {
      expect(isValidApiKey('sk-x')).toBe(true)
      expect(isValidApiKey('sk-111')).toBe(true)
      expect(isValidApiKey('sk-222')).toBe(true)
      expect(isValidApiKey('sk-proj-abc123')).toBe(true)
      expect(isValidApiKey('sk-ant-api03-xxxxxxxxxxxx')).toBe(true)
    })

    it('应识别 ut_ 前缀', () => {
      expect(isValidApiKey('ut_1')).toBe(true)
      expect(isValidApiKey('ut_abc123')).toBe(true)
      expect(isValidApiKey('ut_xxxxxxxxxxxxxxxx')).toBe(true)
    })

    it('应识别其他常见前缀', () => {
      expect(isValidApiKey('api-key123')).toBe(true)
      expect(isValidApiKey('key-abc123')).toBe(true)
      expect(isValidApiKey('cr_xxxxxxxxx')).toBe(true)
      expect(isValidApiKey('ms-xxxxxxxxx')).toBe(true)
    })

    it('不应识别单字母前缀', () => {
      expect(isValidApiKey('s-123')).toBe(false)
      expect(isValidApiKey('u_123')).toBe(false)
    })

    it('不应识别无分隔符的字符串', () => {
      expect(isValidApiKey('sk123')).toBe(false)
      expect(isValidApiKey('apikey')).toBe(false)
    })

    it('不应识别分隔符后无内容的字符串', () => {
      expect(isValidApiKey('sk-')).toBe(false)
      expect(isValidApiKey('ut_')).toBe(false)
    })
  })

  describe('Google API Key 格式', () => {
    it('应识别 AIza 开头的 key', () => {
      expect(isValidApiKey('AIzaSyDxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx')).toBe(true)
      expect(isValidApiKey('AIzaXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX')).toBe(true)
    })

    it('不应识别非 AIza 开头的类似格式', () => {
      expect(isValidApiKey('AIzbSyDxxx')).toBe(false)
      expect(isValidApiKey('Aiza1234567')).toBe(false)
    })
  })

  describe('JWT 格式', () => {
    it('应识别有效的 JWT', () => {
      const validJwt = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U'
      expect(isValidApiKey(validJwt)).toBe(true)
    })

    it('应识别简短但有效的 JWT 格式', () => {
      // 至少 20 字符，有两个点
      expect(isValidApiKey('eyJhbGciOiJIUzI1Ni.eyJzdWIiOiIxMjM0.xxx')).toBe(true)
    })

    it('不应识别只有一个点的 JWT', () => {
      expect(isValidApiKey('eyJhbGciOiJIUzI1NiIs.xxx')).toBe(false)
    })

    it('不应识别过短的 JWT', () => {
      expect(isValidApiKey('eyJ.xxx.yyy')).toBe(false)
    })
  })

  describe('长字符串格式 (≥32 字符)', () => {
    it('应识别 32+ 字符的纯字母数字字符串', () => {
      expect(isValidApiKey('a'.repeat(32))).toBe(true)
      expect(isValidApiKey('abcdefghijklmnopqrstuvwxyz123456')).toBe(true)
      expect(isValidApiKey('ABCDEFGHIJKLMNOPQRSTUVWXYZ123456')).toBe(true)
    })

    it('应识别包含下划线和横线的长字符串', () => {
      expect(isValidApiKey('abcdefghijklmnop_qrstuvwxyz-12345')).toBe(true)
    })

    it('不应识别少于 32 字符的无前缀字符串', () => {
      expect(isValidApiKey('a'.repeat(31))).toBe(false)
      expect(isValidApiKey('shortkey')).toBe(false)
    })

    it('不应识别包含特殊字符的字符串', () => {
      expect(isValidApiKey('a'.repeat(30) + '!@')).toBe(false)
      expect(isValidApiKey('abcdefghijklmnopqrstuvwxyz12345!')).toBe(false)
    })
  })

  describe('无效输入', () => {
    it('不应识别普通单词', () => {
      expect(isValidApiKey('hello')).toBe(false)
      expect(isValidApiKey('world')).toBe(false)
      expect(isValidApiKey('test')).toBe(false)
    })

    it('不应识别 URL', () => {
      expect(isValidApiKey('http://localhost')).toBe(false)
      expect(isValidApiKey('https://api.example.com')).toBe(false)
    })

    it('不应识别空字符串', () => {
      expect(isValidApiKey('')).toBe(false)
    })

    it('不应识别纯数字', () => {
      expect(isValidApiKey('12345678901234567890123456789012')).toBe(false)
    })
  })
})

describe('URL 识别', () => {
  describe('有效 URL', () => {
    it('应识别 localhost', () => {
      expect(isValidUrl('http://localhost')).toBe(true)
      expect(isValidUrl('http://localhost/')).toBe(true)
      expect(isValidUrl('http://localhost:3000')).toBe(true)
      expect(isValidUrl('http://localhost:3000/')).toBe(true)
      expect(isValidUrl('http://localhost:5688/v1')).toBe(true)
    })

    it('应识别域名', () => {
      expect(isValidUrl('https://api.openai.com')).toBe(true)
      expect(isValidUrl('https://api.openai.com/')).toBe(true)
      expect(isValidUrl('https://api.openai.com/v1')).toBe(true)
      expect(isValidUrl('https://api.anthropic.com/v1')).toBe(true)
    })

    it('应识别带端口的域名', () => {
      expect(isValidUrl('http://example.com:8080')).toBe(true)
      expect(isValidUrl('https://api.example.com:443/v1')).toBe(true)
    })

    it('应识别 IP 地址', () => {
      expect(isValidUrl('http://127.0.0.1')).toBe(true)
      expect(isValidUrl('http://192.168.1.1:8080')).toBe(true)
    })

    it('应识别子域名', () => {
      expect(isValidUrl('https://api.v2.example.com')).toBe(true)
      expect(isValidUrl('https://a.b.c.d.example.com/path')).toBe(true)
    })
  })

  describe('无效 URL', () => {
    it('不应识别不完整的 URL', () => {
      expect(isValidUrl('http://')).toBe(false)
      expect(isValidUrl('https://')).toBe(false)
      expect(isValidUrl('http:///')).toBe(false)
    })

    it('不应识别无协议的 URL', () => {
      expect(isValidUrl('localhost')).toBe(false)
      expect(isValidUrl('api.openai.com')).toBe(false)
      expect(isValidUrl('//api.openai.com')).toBe(false)
    })

    it('不应识别无效协议', () => {
      expect(isValidUrl('ftp://example.com')).toBe(false)
      expect(isValidUrl('ws://example.com')).toBe(false)
    })

    it('不应识别无效域名格式', () => {
      expect(isValidUrl('http://-example.com')).toBe(false)
      expect(isValidUrl('http://example-.com')).toBe(false)
    })
  })
})

describe('综合解析场景', () => {
  it('应正确解析 URL + 多个 API Key', () => {
    const input = `
      https://api.openai.com/v1
      sk-key1
      sk-key2
      sk-key3
    `
    const result = parseQuickInput(input)
    expect(result.detectedBaseUrl).toBe('https://api.openai.com/v1')
    expect(result.detectedApiKeys).toEqual(['sk-key1', 'sk-key2', 'sk-key3'])
  })

  it('应正确解析 localhost URL', () => {
    const input = 'http://localhost:5688 sk-111 sk-222'
    const result = parseQuickInput(input)
    expect(result.detectedBaseUrl).toBe('http://localhost:5688')
    expect(result.detectedApiKeys).toEqual(['sk-111', 'sk-222'])
  })

  it('应正确解析混合分隔符', () => {
    const input = 'https://api.example.com, sk-key1; ut_key2, api-key3'
    const result = parseQuickInput(input)
    expect(result.detectedBaseUrl).toBe('https://api.example.com')
    expect(result.detectedApiKeys).toEqual(['sk-key1', 'ut_key2', 'api-key3'])
  })

  it('应忽略不完整的 URL', () => {
    const input = 'http:// sk-key1'
    const result = parseQuickInput(input)
    expect(result.detectedBaseUrl).toBe('')
    expect(result.detectedApiKeys).toEqual(['sk-key1'])
  })

  it('应只取第一个 URL', () => {
    const input = 'https://first.com https://second.com sk-key'
    const result = parseQuickInput(input)
    expect(result.detectedBaseUrl).toBe('https://first.com')
    expect(result.detectedApiKeys).toEqual(['sk-key'])
  })

  it('应去重 API Key', () => {
    const input = 'sk-key1 sk-key1 sk-key2'
    const result = parseQuickInput(input)
    expect(result.detectedApiKeys).toEqual(['sk-key1', 'sk-key2'])
  })

  it('应保留 # 结尾（跳过版本号）', () => {
    const input = 'https://api.example.com/anthropic# sk-key'
    const result = parseQuickInput(input)
    expect(result.detectedBaseUrl).toBe('https://api.example.com/anthropic#')
  })

  it('应保留无路径的 # 结尾', () => {
    const input = 'https://api.example.com# sk-key'
    const result = parseQuickInput(input)
    expect(result.detectedBaseUrl).toBe('https://api.example.com#')
  })

  it('应移除末尾斜杠', () => {
    const input = 'https://api.example.com/ sk-key'
    const result = parseQuickInput(input)
    expect(result.detectedBaseUrl).toBe('https://api.example.com')
  })

  it('应正确处理 JWT 格式的 key', () => {
    const jwt = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U'
    const input = `https://api.example.com ${jwt}`
    const result = parseQuickInput(input)
    expect(result.detectedBaseUrl).toBe('https://api.example.com')
    expect(result.detectedApiKeys).toEqual([jwt])
  })
})
