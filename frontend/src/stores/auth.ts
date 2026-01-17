import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

/**
 * 认证状态管理 Store
 *
 * 职责：
 * - 管理 API Key 的存储和读取
 * - 管理认证错误和安全状态（失败次数、锁定时间等）
 * - 管理认证 UI 状态（加载中、自动认证等）
 * - 提供响应式的认证状态
 * - 自动持久化到 localStorage
 */
export const useAuthStore = defineStore('auth', () => {
  // ===== 状态 =====

  // API Key
  const apiKey = ref<string | null>(null)

  // 认证错误消息
  const authError = ref('')

  // 认证失败次数
  const authAttempts = ref(0)

  // 认证锁定时间（存储时间戳）
  const authLockoutTime = ref<number | null>(null)

  // 自动认证进行中
  const isAutoAuthenticating = ref(true) // 初始化为true，防止登录框闪现

  // 初始化完成标志
  const isInitialized = ref(false)

  // 认证加载状态
  const authLoading = ref(false)

  // 认证输入框值
  const authKeyInput = ref('')

  // ===== 计算属性 =====

  const isAuthenticated = computed(() => !!apiKey.value)

  // 检查是否被锁定
  const isAuthLocked = computed(() => {
    if (!authLockoutTime.value) return false
    return Date.now() < authLockoutTime.value
  })

  // ===== 操作方法 =====

  function setApiKey(key: string | null) {
    apiKey.value = key
    // 同时保存到旧的 localStorage key 以保持兼容性
    if (key) {
      localStorage.setItem('proxyAccessKey', key)
    } else {
      localStorage.removeItem('proxyAccessKey')
    }
  }

  function clearAuth() {
    apiKey.value = null
    // 清除旧的 localStorage key
    localStorage.removeItem('proxyAccessKey')
  }

  function initializeAuth() {
    // 优先从旧的 localStorage key 读取（兼容性）
    const oldKey = localStorage.getItem('proxyAccessKey')
    if (oldKey) {
      apiKey.value = oldKey
      return
    }

    // 如果没有旧 key，尝试从 Pinia 持久化恢复
    // （由 persistedstate 插件自动处理）
  }

  function setAuthError(error: string) {
    authError.value = error
  }

  function incrementAuthAttempts() {
    authAttempts.value++
  }

  function resetAuthAttempts() {
    authAttempts.value = 0
  }

  function setAuthLockout(lockoutTime: Date | null) {
    authLockoutTime.value = lockoutTime ? lockoutTime.getTime() : null
  }

  function setAutoAuthenticating(value: boolean) {
    isAutoAuthenticating.value = value
  }

  function setInitialized(value: boolean) {
    isInitialized.value = value
  }

  function setAuthLoading(value: boolean) {
    authLoading.value = value
  }

  function setAuthKeyInput(value: string) {
    authKeyInput.value = value
  }

  return {
    // 状态
    apiKey,
    authError,
    authAttempts,
    authLockoutTime,
    isAutoAuthenticating,
    isInitialized,
    authLoading,
    authKeyInput,

    // 计算属性
    isAuthenticated,
    isAuthLocked,

    // 方法
    setApiKey,
    clearAuth,
    initializeAuth,
    setAuthError,
    incrementAuthAttempts,
    resetAuthAttempts,
    setAuthLockout,
    setAutoAuthenticating,
    setInitialized,
    setAuthLoading,
    setAuthKeyInput,
  }
}, {
  // 持久化配置
  persist: {
    key: 'claude-proxy-auth',
    storage: localStorage,
    // 仅持久化必要字段，排除瞬态 UI 状态和敏感输入
    pick: ['apiKey', 'authAttempts', 'authLockoutTime'],
  },
})
