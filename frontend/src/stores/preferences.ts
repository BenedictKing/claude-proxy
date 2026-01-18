import { defineStore } from 'pinia'
import { ref } from 'vue'

/**
 * 用户偏好设置 Store
 *
 * 职责：
 * - 管理暗色模式偏好（light/dark/auto）
 * - 管理 Fuzzy 模式开关
 * - 管理全局统计面板展开状态
 * - 自动持久化到 localStorage
 */
export const usePreferencesStore = defineStore('preferences', () => {
  // ===== 状态 =====

  // 暗色模式偏好
  const darkModePreference = ref<'light' | 'dark' | 'auto'>('auto')

  // Fuzzy 模式开关
  const fuzzyModeEnabled = ref(true)

  // 全局统计面板展开状态
  const showGlobalStats = ref(false)

  // ===== 操作方法 =====

  /**
   * 设置暗色模式
   */
  function setDarkMode(mode: 'light' | 'dark' | 'auto') {
    darkModePreference.value = mode
  }

  /**
   * 切换暗色模式（循环切换）
   */
  function toggleDarkMode() {
    const modes: Array<'light' | 'dark' | 'auto'> = ['light', 'dark', 'auto']
    const currentIndex = modes.indexOf(darkModePreference.value)
    const nextIndex = (currentIndex + 1) % modes.length
    darkModePreference.value = modes[nextIndex]
  }

  /**
   * 设置 Fuzzy 模式
   */
  function setFuzzyMode(enabled: boolean) {
    fuzzyModeEnabled.value = enabled
  }

  /**
   * 切换 Fuzzy 模式
   */
  function toggleFuzzyMode() {
    fuzzyModeEnabled.value = !fuzzyModeEnabled.value
  }

  /**
   * 切换全局统计面板
   */
  function toggleGlobalStats() {
    showGlobalStats.value = !showGlobalStats.value
  }

  return {
    // 状态
    darkModePreference,
    fuzzyModeEnabled,
    showGlobalStats,

    // 方法
    setDarkMode,
    toggleDarkMode,
    setFuzzyMode,
    toggleFuzzyMode,
    toggleGlobalStats,
  }
}, {
  // 持久化配置
  persist: {
    key: 'claude-proxy-preferences',
    // 使用条件判断避免在非浏览器环境（SSR、Node 测试）中崩溃
    storage: typeof window !== 'undefined' ? localStorage : undefined,
  },
})
