import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api, type Channel, type ChannelsResponse, type ChannelMetrics, type ChannelDashboardResponse } from '@/services/api'

/**
 * 渠道数据管理 Store
 *
 * 职责：
 * - 管理三种 API 类型的渠道数据（Messages/Responses/Gemini）
 * - 管理渠道指标和统计数据
 * - 提供渠道操作方法（添加、编辑、删除、测试延迟等）
 * - 管理自动刷新定时器
 */
export const useChannelStore = defineStore('channel', () => {
  // ===== 状态 =====

  // 当前选中的 API 类型
  type ApiTab = 'messages' | 'responses' | 'gemini'
  const activeTab = ref<ApiTab>('messages')

  // 路由同步：从路由读取当前类型
  const router = useRouter()
  const currentChannelType = computed(() => {
    const route = router.currentRoute.value
    const type = route.params.type as ApiTab
    return (type === 'messages' || type === 'responses' || type === 'gemini') ? type : 'messages'
  })

  // 监听路由变化，同步 activeTab（确保兼容性）
  watch(currentChannelType, (newType) => {
    activeTab.value = newType
  }, { immediate: true })

  // 三种 API 类型的渠道数据
  const channelsData = ref<ChannelsResponse>({
    channels: [],
    current: -1,
    loadBalance: 'round-robin'
  })

  const responsesChannelsData = ref<ChannelsResponse>({
    channels: [],
    current: -1,
    loadBalance: 'round-robin'
  })

  const geminiChannelsData = ref<ChannelsResponse>({
    channels: [],
    current: -1,
    loadBalance: 'round-robin'
  })

  // Dashboard 数据缓存结构（每个 tab 独立缓存）
  interface DashboardCache {
    metrics: ChannelMetrics[]
    stats: ChannelDashboardResponse['stats'] | undefined
    recentActivity: ChannelDashboardResponse['recentActivity'] | undefined
  }

  const dashboardCache = ref<Record<ApiTab, DashboardCache>>({
    messages: {
      metrics: [],
      stats: undefined,
      recentActivity: undefined
    },
    responses: {
      metrics: [],
      stats: undefined,
      recentActivity: undefined
    },
    gemini: {
      metrics: [],
      stats: undefined,
      recentActivity: undefined
    }
  })

  // 批量延迟测试加载状态
  const isPingingAll = ref(false)

  // 最后一次刷新状态（用于 systemStatus 更新）
  const lastRefreshSuccess = ref(true)

  // 自动刷新定时器（串行 setTimeout，避免重入）
  let autoRefreshTimer: ReturnType<typeof setTimeout> | null = null
  let autoRefreshRunning = false
  const AUTO_REFRESH_INTERVAL = 2000 // 2秒

  // 刷新并发控制：同一时间只允许一个 refresh 在跑；期间再次调用会被合并成一次后续刷新
  let refreshLoopPromise: Promise<void> | null = null
  let refreshRequested = false

  // ===== 计算属性 =====

  // 根据当前 Tab 返回对应的渠道数据
  const currentChannelsData = computed(() => {
    switch (activeTab.value) {
      case 'messages': return channelsData.value
      case 'responses': return responsesChannelsData.value
      case 'gemini': return geminiChannelsData.value
      default: return channelsData.value
    }
  })

  // 根据当前 Tab 返回对应的 Dashboard 数据（独立缓存，避免切换闪烁）
  const currentDashboardMetrics = computed(() => dashboardCache.value[activeTab.value].metrics)
  const currentDashboardStats = computed(() => dashboardCache.value[activeTab.value].stats)
  const currentDashboardRecentActivity = computed(() => dashboardCache.value[activeTab.value].recentActivity)

  // 活跃渠道数（仅 active 状态）
  const activeChannelCount = computed(() => {
    const data = currentChannelsData.value
    if (!data.channels) return 0
    return data.channels.filter(ch => ch.status === 'active' || ch.status === undefined || ch.status === '').length
  })

  // 参与故障转移的渠道数（active + suspended）
  const failoverChannelCount = computed(() => {
    const data = currentChannelsData.value
    if (!data.channels) return 0
    return data.channels.filter(ch => ch.status !== 'disabled').length
  })

  // ===== 辅助方法 =====

  // 合并渠道数据，保留本地的延迟测试结果
  const LATENCY_VALID_DURATION = 5 * 60 * 1000 // 5 分钟有效期

  function mergeChannelsWithLocalData(newChannels: Channel[], existingChannels: Channel[] | undefined): Channel[] {
    if (!existingChannels) return newChannels

    const now = Date.now()
    return newChannels.map(newCh => {
      const existingCh = existingChannels.find(ch => ch.index === newCh.index)
      // 只有在 5 分钟有效期内才保留本地延迟测试结果
      if (existingCh?.latencyTestTime && (now - existingCh.latencyTestTime) < LATENCY_VALID_DURATION) {
        return {
          ...newCh,
          latency: existingCh.latency,
          latencyTestTime: existingCh.latencyTestTime
        }
      }
      return newCh
    })
  }

  // ===== 操作方法 =====

  /**
   * 刷新渠道数据
   */
  async function refreshChannels() {
    refreshRequested = true
    if (refreshLoopPromise) return refreshLoopPromise

    const doRefresh = async (tab: ApiTab) => {
      try {
        // Gemini 使用专用的 dashboard API（降级实现）
        if (tab === 'gemini') {
          const dashboard = await api.getGeminiChannelDashboard()
          geminiChannelsData.value = {
            channels: mergeChannelsWithLocalData(dashboard.channels, geminiChannelsData.value.channels),
            current: geminiChannelsData.value.current,
            loadBalance: dashboard.loadBalance
          }
          // 更新 Gemini tab 的独立缓存
          dashboardCache.value.gemini = {
            metrics: dashboard.metrics,
            stats: dashboard.stats,
            recentActivity: dashboard.recentActivity
          }
          lastRefreshSuccess.value = true
          return
        }

        // Messages / Responses 使用合并的 dashboard 接口
        const dashboard = await api.getChannelDashboard(tab)

        if (tab === 'messages') {
          channelsData.value = {
            channels: mergeChannelsWithLocalData(dashboard.channels, channelsData.value.channels),
            current: channelsData.value.current, // 保留当前选中状态
            loadBalance: dashboard.loadBalance
          }
          // 更新 Messages tab 的独立缓存
          dashboardCache.value.messages = {
            metrics: dashboard.metrics,
            stats: dashboard.stats,
            recentActivity: dashboard.recentActivity
          }
        } else {
          responsesChannelsData.value = {
            channels: mergeChannelsWithLocalData(dashboard.channels, responsesChannelsData.value.channels),
            current: responsesChannelsData.value.current, // 保留当前选中状态
            loadBalance: dashboard.loadBalance
          }
          // 更新 Responses tab 的独立缓存
          dashboardCache.value.responses = {
            metrics: dashboard.metrics,
            stats: dashboard.stats,
            recentActivity: dashboard.recentActivity
          }
        }

        lastRefreshSuccess.value = true
      } catch (error) {
        lastRefreshSuccess.value = false
        throw error
      }
    }

    refreshLoopPromise = (async () => {
      try {
        while (refreshRequested) {
          refreshRequested = false
          const tab = activeTab.value
          await doRefresh(tab)
        }
      } finally {
        refreshLoopPromise = null
      }
    })()

    return refreshLoopPromise
  }

  /**
   * 保存渠道（添加或更新）
   */
  async function saveChannel(
    channel: Omit<Channel, 'index' | 'latency' | 'status'>,
    editingChannelIndex: number | null,
    options?: { isQuickAdd?: boolean }
  ) {
    const isResponses = activeTab.value === 'responses'
    const isGemini = activeTab.value === 'gemini'

    if (editingChannelIndex !== null) {
      // 更新现有渠道
      if (isGemini) {
        await api.updateGeminiChannel(editingChannelIndex, channel)
      } else if (isResponses) {
        await api.updateResponsesChannel(editingChannelIndex, channel)
      } else {
        await api.updateChannel(editingChannelIndex, channel)
      }
      return { success: true, message: '渠道更新成功' }
    } else {
      // 添加新渠道
      if (isGemini) {
        await api.addGeminiChannel(channel)
      } else if (isResponses) {
        await api.addResponsesChannel(channel)
      } else {
        await api.addChannel(channel)
      }

      // 快速添加模式：将新渠道设为第一优先级并设置5分钟促销期
      if (options?.isQuickAdd) {
        await refreshChannels() // 先刷新获取新渠道的 index
        const data = isGemini ? geminiChannelsData.value : (isResponses ? responsesChannelsData.value : channelsData.value)

        // 找到新添加的渠道（应该是列表中 index 最大的 active 状态渠道）
        const activeChannels = data.channels?.filter(ch => ch.status !== 'disabled') || []
        if (activeChannels.length > 0) {
          // 新添加的渠道会分配到最大的 index
          const newChannel = activeChannels.reduce((max, ch) => ch.index > max.index ? ch : max, activeChannels[0])

          try {
            // 1. 重新排序：将新渠道放到第一位
            const otherIndexes = activeChannels
              .filter(ch => ch.index !== newChannel.index)
              .sort((a, b) => (a.priority ?? a.index) - (b.priority ?? b.index))
              .map(ch => ch.index)
            const newOrder = [newChannel.index, ...otherIndexes]

            if (isGemini) {
              await api.reorderGeminiChannels(newOrder)
            } else if (isResponses) {
              await api.reorderResponsesChannels(newOrder)
            } else {
              await api.reorderChannels(newOrder)
            }

            // 2. 设置5分钟促销期（300秒）
            if (isGemini) {
              await api.setGeminiChannelPromotion(newChannel.index, 300)
            } else if (isResponses) {
              await api.setResponsesChannelPromotion(newChannel.index, 300)
            } else {
              await api.setChannelPromotion(newChannel.index, 300)
            }

            return {
              success: true,
              message: '渠道添加成功',
              quickAddMessage: `渠道 ${channel.name} 已设为最高优先级，5分钟内优先使用`
            }
          } catch (err) {
            console.warn('设置快速添加优先级失败:', err)
            // 不影响主流程
          }
        }
      }

      return { success: true, message: '渠道添加成功' }
    }
  }

  /**
   * 删除渠道
   */
  async function deleteChannel(channelId: number) {
    if (activeTab.value === 'gemini') {
      await api.deleteGeminiChannel(channelId)
    } else if (activeTab.value === 'responses') {
      await api.deleteResponsesChannel(channelId)
    } else {
      await api.deleteChannel(channelId)
    }
    await refreshChannels()
    return { success: true, message: '渠道删除成功' }
  }

  /**
   * 测试单个渠道延迟
   */
  async function pingChannel(channelId: number) {
    const result = activeTab.value === 'gemini'
      ? await api.pingGeminiChannel(channelId)
      : await api.pingChannel(channelId)

    const data = activeTab.value === 'gemini'
      ? geminiChannelsData.value
      : (activeTab.value === 'messages' ? channelsData.value : responsesChannelsData.value)

    const channel = data.channels?.find(c => c.index === channelId)
    if (channel) {
      channel.latency = result.latency
      channel.latencyTestTime = Date.now()  // 记录测试时间，用于 5 分钟后清除
    }

    return { success: true }
  }

  /**
   * 批量测试所有渠道延迟
   */
  async function pingAllChannels() {
    if (isPingingAll.value) return { success: false, message: '正在测试中' }

    isPingingAll.value = true
    try {
      const results = activeTab.value === 'gemini'
        ? await api.pingAllGeminiChannels()
        : await api.pingAllChannels()

      const data = activeTab.value === 'gemini'
        ? geminiChannelsData.value
        : (activeTab.value === 'messages' ? channelsData.value : responsesChannelsData.value)

      const now = Date.now()
      results.forEach(result => {
        const channel = data.channels?.find(c => c.index === result.id)
        if (channel) {
          channel.latency = result.latency
          channel.latencyTestTime = now  // 记录测试时间，用于 5 分钟后清除
        }
      })

      return { success: true }
    } finally {
      isPingingAll.value = false
    }
  }

  /**
   * 更新负载均衡策略
   */
  async function updateLoadBalance(strategy: string) {
    if (activeTab.value === 'gemini') {
      await api.updateGeminiLoadBalance(strategy)
      geminiChannelsData.value.loadBalance = strategy
    } else if (activeTab.value === 'messages') {
      await api.updateLoadBalance(strategy)
      channelsData.value.loadBalance = strategy
    } else {
      await api.updateResponsesLoadBalance(strategy)
      responsesChannelsData.value.loadBalance = strategy
    }
    return { success: true, message: `负载均衡策略已更新为: ${strategy}` }
  }

  /**
   * 启动自动刷新定时器
   */
  function startAutoRefresh() {
    stopAutoRefresh()
    autoRefreshRunning = true

    const tick = async () => {
      if (!autoRefreshRunning) return
      try {
        await refreshChannels()
      } catch (error) {
        console.warn('自动刷新失败:', error)
      } finally {
        if (autoRefreshRunning) {
          autoRefreshTimer = setTimeout(() => {
            void tick()
          }, AUTO_REFRESH_INTERVAL)
        }
      }
    }

    autoRefreshTimer = setTimeout(() => {
      void tick()
    }, AUTO_REFRESH_INTERVAL)
  }

  /**
   * 停止自动刷新定时器
   */
  function stopAutoRefresh() {
    autoRefreshRunning = false
    if (!autoRefreshTimer) return
    clearTimeout(autoRefreshTimer)
    autoRefreshTimer = null
  }

  /**
   * 清空所有渠道数据（用于注销）
   */
  function clearChannels() {
    channelsData.value = {
      channels: [],
      current: -1,
      loadBalance: 'round-robin'
    }
    responsesChannelsData.value = {
      channels: [],
      current: -1,
      loadBalance: 'round-robin'
    }
    geminiChannelsData.value = {
      channels: [],
      current: -1,
      loadBalance: 'round-robin'
    }

    // 清空所有 tab 的独立缓存
    dashboardCache.value = {
      messages: {
        metrics: [],
        stats: undefined,
        recentActivity: undefined
      },
      responses: {
        metrics: [],
        stats: undefined,
        recentActivity: undefined
      },
      gemini: {
        metrics: [],
        stats: undefined,
        recentActivity: undefined
      }
    }

    // 重置状态标志，避免注销后状态残留
    lastRefreshSuccess.value = true
    isPingingAll.value = false
  }

  // ===== 返回公开接口 =====
  return {
    // 状态
    activeTab,
    channelsData,
    responsesChannelsData,
    geminiChannelsData,
    isPingingAll,
    lastRefreshSuccess,

    // 计算属性
    currentChannelsData,
    currentDashboardMetrics,
    currentDashboardStats,
    currentDashboardRecentActivity,
    activeChannelCount,
    failoverChannelCount,

    // 方法
    refreshChannels,
    saveChannel,
    deleteChannel,
    pingChannel,
    pingAllChannels,
    updateLoadBalance,
    startAutoRefresh,
    stopAutoRefresh,
    clearChannels,
  }
})
