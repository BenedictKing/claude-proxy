/**
 * 快速添加渠道 - 输入解析工具
 *
 * 用于识别 API Key 和 URL 格式
 */

/**
 * 检测字符串是否为有效的 API Key
 *
 * 支持的格式：
 * - 前缀格式：xx-xxx 或 xx_xxx（如 sk-xxx, ut_xxx, api-xxx）
 * - Google API Key：AIza 开头
 * - JWT 格式：eyJ 开头，包含两个点分隔的 base64 段
 * - 长字符串：≥32 字符的字母数字串（必须包含字母）
 */
export const isValidApiKey = (token: string): boolean => {
  // 常见 API Key 前缀格式（xx-xxx 或 xx_xxx 模式，前缀后至少有1个字符）
  if (/^[a-zA-Z]{2,}[-_][a-zA-Z0-9_-]+$/.test(token)) {
    return true
  }
  // Google API Key 格式
  if (/^AIza[a-zA-Z0-9_-]+$/.test(token)) {
    return true
  }
  // JWT 格式 (eyJ 开头，包含两个点分隔的 base64 段，总长度 >= 20)
  if (/^eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\./.test(token) && token.length >= 20) {
    return true
  }
  // 长度足够且包含字母的字符串（排除纯数字）
  if (token.length >= 32 && /^[a-zA-Z0-9_-]+$/.test(token) && /[a-zA-Z]/.test(token)) {
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
 * 按空白/逗号/分号/换行/引号（中英文）分割
 */
const extractTokens = (input: string): string[] => {
  return input
    .split(/[\n\s,;"\u201c\u201d'\u2018\u2019]+/)
    .filter(t => t.length > 0)
}

/**
 * 解析快速输入内容，提取 URL 和 API Keys
 *
 * 支持的格式：
 * 1. 纯文本：URL 和 API Key 以空白/逗号/分号分隔
 * 2. 引号包裹：从 "xxx" 或 'xxx' 中提取内容（支持 JSON 配置格式）
 */
export const parseQuickInput = (
  input: string
): {
  detectedBaseUrl: string
  detectedApiKeys: string[]
} => {
  let detectedBaseUrl = ''
  const detectedApiKeys: string[] = []

  const tokens = extractTokens(input)

  for (const token of tokens) {
    if (isValidUrl(token)) {
      if (!detectedBaseUrl) {
        const endsWithHash = token.endsWith('#')
        let url = endsWithHash ? token.slice(0, -1) : token
        url = url.replace(/\/$/, '')
        detectedBaseUrl = endsWithHash ? url + '#' : url
      }
      continue
    }

    if (isValidApiKey(token) && !detectedApiKeys.includes(token)) {
      detectedApiKeys.push(token)
    }
  }

  return { detectedBaseUrl, detectedApiKeys }
}
