import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

/**
 * 认证状态管理 Store
 *
 * 职责：
 * - 管理 API Key 的存储和读取
 * - 提供响应式的认证状态
 * - 自动持久化到 localStorage
 */
export const useAuthStore = defineStore('auth', () => {
  // 状态
  const apiKey = ref<string | null>(null)

  // 计算属性
  const isAuthenticated = computed(() => !!apiKey.value)

  // 操作方法
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

  return {
    // 状态
    apiKey,
    // 计算属性
    isAuthenticated,
    // 方法
    setApiKey,
    clearAuth,
    initializeAuth,
  }
}, {
  // 持久化配置
  persist: {
    key: 'claude-proxy-auth',
    storage: localStorage,
  },
})
