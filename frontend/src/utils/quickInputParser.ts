/**
 * 快速添加渠道 - 输入解析工具
 *
 * 用于识别 API Key 和 URL 格式
 */

/**
 * 检测字符串是否看起来像配置键名（全大写 + 下划线分隔的单词）
 * 例如：API_TIMEOUT_MS, ANTHROPIC_BASE_URL, CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC
 */
const looksLikeConfigKey = (token: string): boolean => {
  // 全大写字母 + 下划线，且由多个单词组成（至少包含一个下划线分隔的段）
  // 且每个段都是 2+ 字符的大写字母单词
  if (/^[A-Z][A-Z0-9]*(_[A-Z][A-Z0-9]*)+$/.test(token)) {
    return true
  }
  return false
}

/**
 * 各平台 API Key 格式的专用正则匹配
 *
 * 国际主流:
 * - OpenAI Legacy: sk-[a-zA-Z0-9]{48}
 * - OpenAI Project: sk-proj-[a-zA-Z0-9-]{100,}
 * - Anthropic: sk-ant-api03-[a-zA-Z0-9-]{80,}
 * - Google Gemini: AIza[0-9A-Za-z-_]{35}
 * - Azure OpenAI: 32位十六进制
 *
 * 新兴生态:
 * - Hugging Face: hf_[a-zA-Z0-9]{34}
 * - Groq: gsk_[a-zA-Z0-9]{52}
 * - Perplexity: pplx-[a-zA-Z0-9]{40,}
 * - Replicate: r8_[a-zA-Z0-9]+
 * - OpenRouter: sk-or-v1-[a-zA-Z0-9]{50,}
 *
 * 国内平台:
 * - DeepSeek/Moonshot/01.AI/SiliconFlow: sk-[a-zA-Z0-9]{48} (兼容 OpenAI)
 * - 智谱 AI: [a-z0-9]{32}\.[a-z0-9]+ (id.secret 格式)
 * - 火山引擎 Ark: UUID 格式
 * - 火山引擎 IAM: AK 开头
 */
const PLATFORM_KEY_PATTERNS: RegExp[] = [
  // OpenAI Project Key (新格式，最长，优先匹配)
  /^sk-proj-[a-zA-Z0-9_-]{50,}$/,
  // Anthropic Claude
  /^sk-ant-api03-[a-zA-Z0-9_-]{50,}$/,
  // OpenRouter (混合大小写字母数字)
  /^sk-or-v1-[a-zA-Z0-9]{50,}$/,
  // OpenAI Legacy / DeepSeek / Moonshot / 01.AI / SiliconFlow
  /^sk-[a-zA-Z0-9]{20,}$/,
  // Google Gemini/PaLM (通常 39 字符，允许一定范围)
  /^AIza[0-9A-Za-z_-]{30,}$/,
  // Hugging Face
  /^hf_[a-zA-Z0-9]{30,}$/,
  // Groq
  /^gsk_[a-zA-Z0-9]{40,}$/,
  // Perplexity
  /^pplx-[a-zA-Z0-9]{40,}$/,
  // Replicate
  /^r8_[a-zA-Z0-9]{20,}$/,
  // 智谱 AI (id.secret 格式)
  /^[a-zA-Z0-9]{20,}\.[a-zA-Z0-9]{10,}$/,
  // 火山引擎 Ark (UUID 格式)
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
  // 火山引擎 IAM AK
  /^AK[A-Z]{2,4}[a-zA-Z0-9]{20,}$/
]

/**
 * 检测字符串是否为有效的 API Key
 *
 * 支持的格式：
 * 1. 平台特定格式（优先匹配，准确度最高）
 * 2. 通用前缀格式：xx-xxx 或 xx_xxx（如 sk-xxx, ut_xxx, api-xxx）
 * 3. JWT 格式：eyJ 开头，包含两个点分隔的 base64 段
 * 4. 长随机字符串：≥32 字符的字母数字串（必须包含字母和数字）
 * 5. 宽松兜底：常见前缀 + 任意后缀（当以上都不匹配时）
 *
 * 排除的格式：
 * - 配置键名：全大写 + 下划线分隔（如 API_TIMEOUT_MS）
 */
export const isValidApiKey = (token: string): boolean => {
  // 首先排除配置键名格式
  if (looksLikeConfigKey(token)) {
    return false
  }

  // 1. 平台特定格式匹配（最准确）
  for (const pattern of PLATFORM_KEY_PATTERNS) {
    if (pattern.test(token)) {
      return true
    }
  }

  // 2. 通用前缀格式（前缀 2-6 字母 + 连字符/下划线 + 至少 10 字符后缀）
  // 后缀必须包含数字或混合大小写（随机特征）
  if (/^[a-zA-Z]{2,6}[-_][a-zA-Z0-9_-]{10,}$/.test(token)) {
    const suffix = token.replace(/^[a-zA-Z]{2,6}[-_]/, '')
    const hasDigit = /\d/.test(suffix)
    const hasMixedCase = /[a-z]/.test(suffix) && /[A-Z]/.test(suffix)
    if (hasDigit || hasMixedCase) {
      return true
    }
  }

  // 3. JWT 格式 (eyJ 开头，包含两个点分隔的 base64 段，总长度 >= 20)
  if (/^eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\./.test(token) && token.length >= 20) {
    return true
  }

  // 4. 长随机字符串（≥32 字符，必须同时包含字母和数字）
  if (token.length >= 32 && /^[a-zA-Z0-9_-]+$/.test(token) && /[a-zA-Z]/.test(token) && /\d/.test(token)) {
    return true
  }

  // 5. 宽松兜底：常见 API Key 前缀 + 任意后缀（至少 1 个字符）
  // 当以上严格规则都不匹配时，放松标准识别常见格式
  // 支持：sk-xxx, api-xxx, key-xxx, ut_xxx, hf_xxx, gsk_xxx 等
  if (/^(sk|api|key|ut|hf|gsk|cr|ms|r8|pplx)[-_].+$/i.test(token)) {
    return true
  }

  return false
}

/**
 * 检测字符串是否为有效的 URL
 *
 * 要求：
 * - 必须以 http:// 或 https:// 开头
 * - 必须包含有效域名（域名段不能以横线开头或结尾）
 * - 支持末尾 # 标记（用于跳过自动添加 /v1）
 */
export const isValidUrl = (token: string): boolean => {
  // 域名段不能以横线开头或结尾，支持末尾 # 或 / 或直接结束
  return /^https?:\/\/[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*(:\d+)?(\/|#|$)/i.test(
    token
  )
}

/**
 * 从输入中提取所有 token
 * 按空白/逗号/分号/中文冒号/换行/引号（中英文）/等号/%20 分割
 */
const extractTokens = (input: string): string[] => {
  return input
    .replace(/%20/g, ' ')
    .split(/[\n\s,;，；：="\u201c\u201d'\u2018\u2019]+/)
    .filter(t => t.length > 0)
}

/**
 * 根据 URL 路径检测服务类型，并返回清理后的 baseUrl
 * /messages → claude, /chat/completions → openai, /responses → responses
 */
const detectServiceTypeAndCleanUrl = (
  url: string
): { serviceType: 'openai' | 'gemini' | 'claude' | 'responses' | null; cleanedUrl: string } => {
  try {
    const cleanUrl = url.replace(/#$/, '')
    const parsed = new URL(cleanUrl)
    const path = parsed.pathname.toLowerCase()

    // 检测端点并移除
    const endpoints = ['/messages', '/chat/completions', '/responses', '/generatecontent']
    for (const ep of endpoints) {
      if (path.includes(ep)) {
        // 移除端点路径，保留 /v1 等版本前缀
        const idx = path.indexOf(ep)
        parsed.pathname = path.slice(0, idx) || '/'
        const serviceType =
          ep === '/messages'
            ? 'claude'
            : ep === '/chat/completions'
              ? 'openai'
              : ep === '/responses'
                ? 'responses'
                : 'gemini'
        let result = parsed.toString().replace(/\/$/, '')
        if (url.endsWith('#')) result += '#'
        return { serviceType, cleanedUrl: result }
      }
    }
  } catch {
    // 忽略解析错误
  }
  return { serviceType: null, cleanedUrl: url }
}

// 保留导出以兼容可能的外部使用
export const detectServiceType = (url: string): 'openai' | 'gemini' | 'claude' | 'responses' | null => {
  return detectServiceTypeAndCleanUrl(url).serviceType
}

/** Base URL 最大数量限制 */
const MAX_BASE_URLS = 10

/**
 * 解析快速输入内容，提取 URL 和 API Keys
 *
 * 支持的格式：
 * 1. 纯文本：URL 和 API Key 以空白/逗号/分号/等号分隔
 * 2. 引号包裹：从 "xxx" 或 'xxx' 中提取内容（支持 JSON 配置格式）
 * 3. 多 Base URL：所有符合 HTTP 链接格式的都作为 baseUrl（最多 10 个）
 */
export const parseQuickInput = (
  input: string
): {
  detectedBaseUrl: string
  detectedBaseUrls: string[]
  detectedApiKeys: string[]
  detectedServiceType: 'openai' | 'gemini' | 'claude' | 'responses' | null
} => {
  const detectedBaseUrls: string[] = []
  let detectedServiceType: 'openai' | 'gemini' | 'claude' | 'responses' | null = null
  const detectedApiKeys: string[] = []

  const tokens = extractTokens(input)

  for (const token of tokens) {
    if (isValidUrl(token)) {
      // 限制最大 URL 数量
      if (detectedBaseUrls.length >= MAX_BASE_URLS) {
        continue
      }

      const endsWithHash = token.endsWith('#')
      let url = endsWithHash ? token.slice(0, -1) : token
      url = url.replace(/\/$/, '')
      const fullUrl = endsWithHash ? url + '#' : url

      // 检测协议并清理 URL（移除端点路径）
      const { serviceType, cleanedUrl } = detectServiceTypeAndCleanUrl(fullUrl)

      // 避免重复
      if (!detectedBaseUrls.includes(cleanedUrl)) {
        detectedBaseUrls.push(cleanedUrl)
        // 使用第一个 URL 的服务类型
        if (!detectedServiceType) {
          detectedServiceType = serviceType
        }
      }
      continue
    }

    if (isValidApiKey(token) && !detectedApiKeys.includes(token)) {
      detectedApiKeys.push(token)
    }
  }

  return {
    detectedBaseUrl: detectedBaseUrls[0] || '',
    detectedBaseUrls,
    detectedApiKeys,
    detectedServiceType
  }
}
