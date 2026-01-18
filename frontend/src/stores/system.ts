import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { VersionInfo } from '@/services/version'

/**
 * 系统状态管理 Store
 *
 * 职责：
 * - 管理系统运行状态（running/error/connecting）
 * - 管理版本信息和版本检查状态
 * - 管理 Fuzzy 模式加载状态
 */
export const useSystemStore = defineStore('system', () => {
  // ===== 状态 =====

  // 系统连接状态
  type SystemStatus = 'running' | 'error' | 'connecting'
  const systemStatus = ref<SystemStatus>('connecting')

  // 版本信息
  const versionInfo = ref<VersionInfo>({
    currentVersion: '',
    latestVersion: null,
    isLatest: false,
    hasUpdate: false,
    releaseUrl: null,
    lastCheckTime: 0,
    status: 'checking',
  })

  // 版本检查加载状态
  const isCheckingVersion = ref(false)

  // Fuzzy 模式加载状态
  const fuzzyModeLoading = ref(false)
  const fuzzyModeLoadError = ref(false)

  // ===== 计算属性 =====

  const systemStatusText = computed(() => {
    switch (systemStatus.value) {
      case 'running':
        return '运行中'
      case 'error':
        return '连接失败'
      case 'connecting':
        return '连接中'
      default:
        return '未知'
    }
  })

  const systemStatusDesc = computed(() => {
    switch (systemStatus.value) {
      case 'running':
        return '服务正常运行'
      case 'error':
        return '无法连接后端'
      case 'connecting':
        return '正在连接后端'
      default:
        return ''
    }
  })

  // ===== 操作方法 =====

  /**
   * 设置系统状态
   */
  function setSystemStatus(status: SystemStatus) {
    systemStatus.value = status
  }

  /**
   * 设置版本信息
   */
  function setVersionInfo(info: VersionInfo) {
    versionInfo.value = info
  }

  /**
   * 更新当前版本号
   */
  function setCurrentVersion(version: string) {
    versionInfo.value.currentVersion = version
  }

  /**
   * 设置版本检查状态
   */
  function setCheckingVersion(checking: boolean) {
    isCheckingVersion.value = checking
  }

  /**
   * 设置 Fuzzy 模式加载状态
   */
  function setFuzzyModeLoading(loading: boolean) {
    fuzzyModeLoading.value = loading
  }

  /**
   * 设置 Fuzzy 模式加载错误状态
   */
  function setFuzzyModeLoadError(error: boolean) {
    fuzzyModeLoadError.value = error
  }

  /**
   * 重置系统状态
   */
  function resetSystemState() {
    systemStatus.value = 'connecting'
    versionInfo.value = {
      currentVersion: '',
      latestVersion: null,
      isLatest: false,
      hasUpdate: false,
      releaseUrl: null,
      lastCheckTime: 0,
      status: 'checking',
    }
    isCheckingVersion.value = false
    fuzzyModeLoading.value = false
    fuzzyModeLoadError.value = false
  }

  return {
    // 状态
    systemStatus,
    versionInfo,
    isCheckingVersion,
    fuzzyModeLoading,
    fuzzyModeLoadError,

    // 计算属性
    systemStatusText,
    systemStatusDesc,

    // 方法
    setSystemStatus,
    setVersionInfo,
    setCurrentVersion,
    setCheckingVersion,
    setFuzzyModeLoading,
    setFuzzyModeLoadError,
    resetSystemState,
  }
})
