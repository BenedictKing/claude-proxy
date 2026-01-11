<template>
  <div class="key-trend-chart-container">
    <!-- Snackbar for error notification -->
    <v-snackbar v-model="showError" color="error" :timeout="3000" location="top">
      {{ errorMessage }}
      <template #actions>
        <v-btn variant="text" @click="showError = false">关闭</v-btn>
      </template>
    </v-snackbar>

    <!-- 头部：时间范围选择（左） + 视图切换（右） -->
    <div class="chart-header d-flex align-center justify-space-between mb-3">
      <div class="d-flex align-center ga-2">
        <!-- 时间范围选择器 -->
        <v-btn-toggle v-model="selectedDuration" mandatory density="compact" variant="outlined" divided :disabled="isLoading">
          <v-btn value="1h" size="x-small">1小时</v-btn>
          <v-btn value="6h" size="x-small">6小时</v-btn>
          <v-btn value="24h" size="x-small">24小时</v-btn>
          <v-btn value="today" size="x-small">今日</v-btn>
        </v-btn-toggle>

        <v-btn icon size="x-small" variant="text" @click="refreshData" :loading="isLoading" :disabled="isLoading">
          <v-icon size="small">mdi-refresh</v-icon>
        </v-btn>
      </div>

      <!-- 视图切换按钮 -->
      <v-btn-toggle v-model="selectedView" mandatory density="compact" variant="outlined" divided :disabled="isLoading">
        <v-btn value="traffic" size="x-small">
          <v-icon size="small" class="mr-1">mdi-chart-line</v-icon>
          流量
        </v-btn>
        <v-btn value="tokens" size="x-small">
          <v-icon size="small" class="mr-1">mdi-chart-line</v-icon>
          Token I/O
        </v-btn>
        <v-btn value="cache" size="x-small">
          <v-icon size="small" class="mr-1">mdi-database</v-icon>
          缓存 R/W
        </v-btn>
      </v-btn-toggle>
    </div>

    <!-- Loading state -->
    <div v-if="isLoading" class="d-flex justify-center align-center" style="height: 200px">
      <v-progress-circular indeterminate size="32" color="primary" />
    </div>

    <!-- Empty state -->
    <div v-else-if="!hasData" class="d-flex flex-column justify-center align-center text-medium-emphasis" style="height: 200px">
      <v-icon size="40" color="grey-lighten-1">mdi-chart-timeline-variant</v-icon>
      <div class="text-caption mt-2">选定时间范围内没有 Key 使用记录</div>
    </div>

    <!-- 图表区域 -->
    <div v-else class="chart-area">
      <apexchart
        ref="chartRef"
        type="area"
        height="280"
        :options="chartOptions"
        :series="chartSeries"
      />
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useTheme } from 'vuetify'
import VueApexCharts from 'vue3-apexcharts'
import type { ApexOptions } from 'apexcharts'
import { api, type ChannelKeyMetricsHistoryResponse } from '../services/api'

// Register apexchart component
const apexchart = VueApexCharts

// Props
const props = defineProps<{
  channelId: number
  channelType: 'messages' | 'responses' | 'gemini'
}>()

// View mode type
type ViewMode = 'traffic' | 'tokens' | 'cache'
type Duration = '1h' | '6h' | '24h' | 'today'

// LocalStorage keys for preferences (per channelType)
const getStorageKey = (channelType: string, key: string) => `keyTrendChart:${channelType}:${key}`

// Check if localStorage is available (SSR-safe)
const isLocalStorageAvailable = (): boolean => {
  try {
    return typeof window !== 'undefined' && window.localStorage !== undefined
  } catch {
    return false
  }
}

const loadSavedPreferences = (channelType: string): { view: ViewMode; duration: Duration } => {
  if (!isLocalStorageAvailable()) {
    return { view: 'traffic', duration: '1h' }
  }
  try {
    const savedView = window.localStorage.getItem(getStorageKey(channelType, 'viewMode')) as ViewMode | null
    const savedDuration = window.localStorage.getItem(getStorageKey(channelType, 'duration')) as Duration | null
    return {
      view: savedView && ['traffic', 'tokens', 'cache'].includes(savedView) ? savedView : 'traffic',
      duration: savedDuration && ['1h', '6h', '24h', 'today'].includes(savedDuration) ? savedDuration : '1h'
    }
  } catch {
    return { view: 'traffic', duration: '1h' }
  }
}

const savePreference = (channelType: string, key: string, value: string) => {
  if (!isLocalStorageAvailable()) return
  try {
    window.localStorage.setItem(getStorageKey(channelType, key), value)
  } catch {
    // Ignore storage errors (quota exceeded, private mode, etc.)
  }
}

// Theme
const theme = useTheme()
const isDark = computed(() => theme.global.current.value.dark)

// Load saved preferences for current channelType
const savedPrefs = loadSavedPreferences(props.channelType)

// State
const selectedView = ref<ViewMode>(savedPrefs.view)
const selectedDuration = ref<Duration>(savedPrefs.duration)
const isLoading = ref(false)
const isRefreshing = ref(false) // includes auto refresh (silent) requests
const historyData = ref<ChannelKeyMetricsHistoryResponse | null>(null)
const showError = ref(false)
const errorMessage = ref('')

// Chart ref for updateSeries
const chartRef = ref<InstanceType<typeof VueApexCharts> | null>(null)

// request id for refreshData
let refreshRequestId = 0

// Auto refresh timer (2 seconds interval, same as global refresh)
const AUTO_REFRESH_INTERVAL = 2000
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null

const startAutoRefresh = () => {
  stopAutoRefresh()
  autoRefreshTimer = setInterval(() => {
    // Skip if already refreshing to prevent concurrent requests / stale overwrites
    if (!isRefreshing.value) {
      refreshData(true) // true = auto refresh, use updateSeries
    }
  }, AUTO_REFRESH_INTERVAL)
}

const stopAutoRefresh = () => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

// Key colors - 支持最多 10 个 key
const keyColors = [
  '#3b82f6', // 蓝色
  '#f97316', // 橙色
  '#10b981', // 绿色
  '#8b5cf6', // 紫色
  '#ec4899', // 粉色
  '#eab308', // 黄色
  '#06b6d4', // 青色
  '#f43f5e', // 玫红
  '#84cc16', // 酸橙绿
  '#6366f1', // 靛蓝
]

// 失败率阈值：超过此值显示红色背景
const FAILURE_RATE_THRESHOLD = 0.1 // 10%

// 聚合间隔配置（与后端保持一致）
// 1h = 1m, 6h = 5m, 24h = 15m, today = 动态计算
const AGGREGATION_INTERVALS: Record<Duration, number> = {
  '1h': 60000,    // 1 分钟
  '6h': 300000,   // 5 分钟
  '24h': 900000,  // 15 分钟
  'today': 300000 // 5 分钟（今日默认使用 5 分钟间隔）
}

// 根据时间范围获取聚合间隔
const getAggregationInterval = (duration: Duration): number => {
  const interval = AGGREGATION_INTERVALS[duration]
  if (interval === undefined) {
    console.warn(`[KeyTrendChart] Unknown duration "${duration}", falling back to 1m interval`)
    return 60000
  }
  return interval
}

// 将时间戳对齐到聚合桶（向下取整）
const alignToBucket = (timestamp: number, interval: number): number => {
  return Math.floor(timestamp / interval) * interval
}

// Computed: check if has data
const hasData = computed(() => {
  if (!historyData.value) return false
  return historyData.value.keys &&
    historyData.value.keys.length > 0 &&
    historyData.value.keys.some(k => k.dataPoints && k.dataPoints.length > 0)
})

// Computed: 计算每个时间点的加权平均成功率，用于背景色带
// 返回格式: { timestamp: number, failureRate: number }[]
const timePointFailureRates = computed(() => {
  if (!historyData.value?.keys?.length) return []

  // 获取当前聚合间隔，与 tooltip 保持一致
  const interval = getAggregationInterval(selectedDuration.value)

  // 按对齐后的时间戳聚合所有 key 的数据（与 tooltip 逻辑一致）
  const timeMap = new Map<number, { totalRequests: number; totalFailures: number }>()

  historyData.value.keys.forEach(keyData => {
    keyData.dataPoints?.forEach(dp => {
      const rawTs = new Date(dp.timestamp).getTime()
      // 使用 alignToBucket 对齐时间戳，确保与 tooltip 数据匹配
      const alignedTs = alignToBucket(rawTs, interval)
      const existing = timeMap.get(alignedTs) || { totalRequests: 0, totalFailures: 0 }
      existing.totalRequests += dp.requestCount
      existing.totalFailures += dp.failureCount
      timeMap.set(alignedTs, existing)
    })
  })

  // 转换为数组并计算失败率
  return Array.from(timeMap.entries())
    .map(([timestamp, data]) => ({
      timestamp,
      failureRate: data.totalRequests > 0 ? data.totalFailures / data.totalRequests : 0
    }))
    .sort((a, b) => a.timestamp - b.timestamp)
})

// Helper: 根据失败率计算透明度（失败率越高，颜色越深）
// 10% -> 0.08, 20% -> 0.15, 30% -> 0.22, 50% -> 0.35, 70% -> 0.48, 100% -> 0.65
const getFailureOpacity = (failureRate: number): number => {
  const minOpacity = 0.08
  const maxOpacity = 0.65
  // 从阈值开始计算，到 100% 时达到最大透明度
  const normalizedRate = Math.min((failureRate - FAILURE_RATE_THRESHOLD) / (1 - FAILURE_RATE_THRESHOLD), 1)
  return minOpacity + normalizedRate * (maxOpacity - minOpacity)
}

// Computed: 生成 ApexCharts annotations（红色背景色带，深浅随失败率变化）
const failureAnnotations = computed(() => {
  if (selectedView.value !== 'traffic') return [] // 只在流量模式显示

  const rates = timePointFailureRates.value
  if (rates.length === 0) return []

  const annotations: any[] = []

  // 根据当前时间范围获取聚合间隔（与后端保持一致）
  const DEFAULT_INTERVAL = getAggregationInterval(selectedDuration.value)
  // 最大间隔限制：默认间隔的 2 倍，防止数据稀疏时色带过大
  const MAX_INTERVAL = DEFAULT_INTERVAL * 2

  // 为每个超过阈值的点单独创建一个 annotation
  rates.forEach((point, index) => {
    if (point.failureRate >= FAILURE_RATE_THRESHOLD) {
      // 动态计算该点的时间间隔：优先使用与相邻点的实际间隔
      let interval = DEFAULT_INTERVAL
      if (rates.length > 1) {
        if (index > 0) {
          // 使用与前一个点的间隔
          interval = point.timestamp - rates[index - 1].timestamp
        } else if (index < rates.length - 1) {
          // 第一个点：使用与后一个点的间隔
          interval = rates[index + 1].timestamp - point.timestamp
        }
      }
      // 限制最大间隔，避免数据稀疏时色带过大
      interval = Math.min(interval, MAX_INTERVAL)

      annotations.push({
        x: point.timestamp - interval / 2,
        x2: point.timestamp + interval / 2,
        fillColor: '#ef4444',
        opacity: getFailureOpacity(point.failureRate),
        label: {
          text: ''
        }
      })
    }
  })

  return annotations
})

// Computed: get all data points flattened
const allDataPoints = computed(() => {
  if (!historyData.value?.keys) return []
  return historyData.value.keys.flatMap(k => k.dataPoints || [])
})

// Computed: chart options
const chartOptions = computed<ApexOptions>(() => {
  const mode = selectedView.value

  // Token/Cache 模式使用双 Y 轴（左侧 Input/Read，右侧 Output/Write）
  // 解决数量级差异大（如 Input 几十K，Output 几百）导致小值不可见的问题
  let yaxisConfig: any
  if (mode === 'tokens' || mode === 'cache') {
    const keyCount = historyData.value?.keys?.length || 1
    yaxisConfig = []
    // 为每个 key 的 Input 和 Output 分别配置 Y 轴
    for (let i = 0; i < keyCount; i++) {
      // Input/Read - 左侧 Y 轴（只第一个显示标签）
      yaxisConfig.push({
        seriesName: historyData.value?.keys?.[i]?.keyMask
          ? `${historyData.value.keys[i].keyMask} ${mode === 'tokens' ? 'Input' : 'Cache Read'}`
          : undefined,
        show: i === 0,
        labels: {
          formatter: (val: number) => formatAxisValue(val, mode),
          style: { fontSize: '11px' }
        },
        min: 0
      })
      // Output/Write - 右侧 Y 轴（只第一个显示标签）
      yaxisConfig.push({
        seriesName: historyData.value?.keys?.[i]?.keyMask
          ? `${historyData.value.keys[i].keyMask} ${mode === 'tokens' ? 'Output' : 'Cache Write'}`
          : undefined,
        opposite: true,
        show: i === 0,
        labels: {
          formatter: (val: number) => formatAxisValue(val, mode),
          style: { fontSize: '11px' }
        },
        min: 0
      })
    }
  } else {
    yaxisConfig = {
      labels: {
        formatter: (val: number) => formatAxisValue(val, mode),
        style: { fontSize: '11px' }
      },
      min: 0
    }
  }

  return {
    chart: {
      toolbar: { show: false },
      zoom: { enabled: false },
      background: 'transparent',
      fontFamily: 'inherit',
      sparkline: { enabled: false },
      animations: {
        enabled: true,
        speed: 400,
        animateGradually: { enabled: true, delay: 150 },
        dynamicAnimation: { enabled: true, speed: 350 }
      }
    },
    theme: {
      mode: isDark.value ? 'dark' : 'light'
    },
    colors: getChartColors(),
    fill: {
      type: 'gradient' as const,
      gradient: {
        shadeIntensity: 1,
        opacityFrom: 0.4,
        opacityTo: 0.08,
        stops: [0, 90, 100]
      }
    },
    dataLabels: {
      enabled: false
    },
    stroke: {
      curve: 'smooth' as const,
      width: 2,
      // traffic 模式全用实线；tokens/cache 模式：Input/Read 实线，Output/Write 虚线
      dashArray: getDashArray()
    },
    grid: {
      borderColor: isDark.value ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.1)',
      padding: { left: 10, right: 10 }
    },
    xaxis: {
      type: 'datetime',
      labels: {
        datetimeUTC: false,
        format: selectedDuration.value === '1h' ? 'HH:mm' : 'HH:mm',
        style: { fontSize: '10px' }
      },
      axisBorder: { show: false },
      axisTicks: { show: false }
    },
    yaxis: yaxisConfig,
    annotations: {
      xaxis: failureAnnotations.value
    },
    tooltip: {
      x: {
        format: 'MM-dd HH:mm'
      },
      y: {
        formatter: (val: number) => formatTooltipValue(val, mode)
      },
      custom: mode === 'traffic' ? buildTrafficTooltip : undefined
    },
    legend: {
      show: false
    }
  }
})

// Build chart series from data
const buildChartSeries = (data: ChannelKeyMetricsHistoryResponse | null) => {
  if (!data?.keys) return []

  const mode = selectedView.value
  const result: { name: string; data: { x: number; y: number }[] }[] = []

  data.keys.forEach((keyData, keyIndex) => {
    const color = keyColors[keyIndex % keyColors.length]

    if (mode === 'traffic') {
      // 单向模式：只显示请求数
      result.push({
        name: keyData.keyMask,
        data: keyData.dataPoints.map(dp => ({
          x: new Date(dp.timestamp).getTime(),
          y: dp.requestCount
        }))
      })
    } else {
      // 双向模式：每个 key 创建两个 series（Input/Output 或 Read/Creation）
      const inLabel = mode === 'tokens' ? 'Input' : 'Cache Read'
      const outLabel = mode === 'tokens' ? 'Output' : 'Cache Write'

      // 正向（Input/Read）
      result.push({
        name: `${keyData.keyMask} ${inLabel}`,
        data: keyData.dataPoints.map(dp => {
          let value = 0
          if (mode === 'tokens') {
            value = dp.inputTokens
          } else {
            value = dp.cacheReadTokens
          }
          return { x: new Date(dp.timestamp).getTime(), y: value }
        })
      })

      // Output/Write - 使用虚线区分
      result.push({
        name: `${keyData.keyMask} ${outLabel}`,
        data: keyData.dataPoints.map(dp => {
          let value = 0
          if (mode === 'tokens') {
            value = dp.outputTokens
          } else {
            value = dp.cacheCreationTokens
          }
          return { x: new Date(dp.timestamp).getTime(), y: value }
        })
      })
    }
  })

  return result
}

// Computed: chart series data
const chartSeries = computed(() => buildChartSeries(historyData.value))

// Helper: format number for display
const formatNumber = (num: number): string => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toFixed(0)
}

// Helper: format axis value based on view mode
const formatAxisValue = (val: number, mode: ViewMode): string => {
  switch (mode) {
    case 'traffic':
      return Math.round(val).toString()
    case 'tokens':
    case 'cache':
      return formatNumber(Math.abs(val))
    default:
      return val.toString()
  }
}

// Helper: format tooltip value
const formatTooltipValue = (val: number, mode: ViewMode): string => {
  switch (mode) {
    case 'traffic':
      return `${Math.round(val)} 请求`
    case 'tokens':
    case 'cache':
      return formatNumber(Math.abs(val))
    default:
      return val.toString()
  }
}

// Helper: build custom tooltip for traffic mode (shows success/failure breakdown)
const buildTrafficTooltip = ({ series, seriesIndex, dataPointIndex, w }: any): string => {
  if (!historyData.value?.keys) return ''

  const timestamp = w.globals.seriesX[seriesIndex][dataPointIndex]
  const date = new Date(timestamp)
  const timeStr = date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })

  // 收集该时间点所有 key 的数据
  const keyStats: { keyMask: string; success: number; failure: number; total: number; color: string }[] = []
  let grandTotal = 0
  let grandFailure = 0

  // HTML 转义函数，防止 XSS
  const escapeHtml = (str: string): string => {
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;')
  }

  // 获取当前聚合间隔，用于时间戳对齐匹配
  const interval = getAggregationInterval(selectedDuration.value)
  const alignedTimestamp = alignToBucket(timestamp, interval)

  historyData.value.keys.forEach((keyData, keyIndex) => {
    // 使用 filter 累加同一时间桶内的所有数据点（防御性编程）
    const matchingPoints = keyData.dataPoints?.filter(p => {
      const dpTimestamp = new Date(p.timestamp).getTime()
      return alignToBucket(dpTimestamp, interval) === alignedTimestamp
    }) || []

    if (matchingPoints.length > 0) {
      // 累加所有匹配点的统计数据
      const aggregated = matchingPoints.reduce(
        (acc, dp) => ({
          success: acc.success + dp.successCount,
          failure: acc.failure + dp.failureCount,
          total: acc.total + dp.requestCount
        }),
        { success: 0, failure: 0, total: 0 }
      )

      if (aggregated.total > 0) {
        keyStats.push({
          keyMask: escapeHtml(keyData.keyMask),
          success: aggregated.success,
          failure: aggregated.failure,
          total: aggregated.total,
          color: keyColors[keyIndex % keyColors.length]
        })
        grandTotal += aggregated.total
        grandFailure += aggregated.failure
      }
    }
  })

  if (keyStats.length === 0) return ''

  const grandFailureRate = grandTotal > 0 ? (grandFailure / grandTotal * 100).toFixed(1) : '0'
  const hasFailure = grandFailure > 0

  // 构建 HTML
  let html = `<div style="padding: 8px 12px; font-size: 12px;">`
  html += `<div style="font-weight: 600; margin-bottom: 6px; color: ${hasFailure ? '#ef4444' : 'inherit'};">${timeStr}</div>`

  // 每个 key 的详情
  keyStats.forEach(stat => {
    const failureRate = stat.total > 0 ? (stat.failure / stat.total * 100).toFixed(0) : '0'
    const hasKeyFailure = stat.failure > 0
    html += `<div style="display: flex; align-items: center; margin: 4px 0;">`
    html += `<span style="width: 10px; height: 10px; border-radius: 50%; background: ${stat.color}; margin-right: 6px;"></span>`
    html += `<span style="flex: 1;">${stat.keyMask}</span>`
    html += `<span style="margin-left: 12px; font-weight: 500;">${stat.total}</span>`
    if (hasKeyFailure) {
      html += `<span style="margin-left: 6px; color: #ef4444; font-size: 11px;">(${stat.failure}失败, ${failureRate}%)</span>`
    }
    html += `</div>`
  })

  // 汇总行（如果有多个 key）
  if (keyStats.length > 1) {
    html += `<div style="border-top: 1px solid rgba(128,128,128,0.3); margin-top: 6px; padding-top: 6px; font-weight: 600;">`
    html += `<span>合计: ${grandTotal} 请求</span>`
    if (hasFailure) {
      html += `<span style="color: #ef4444; margin-left: 8px;">${grandFailure} 失败 (${grandFailureRate}%)</span>`
    }
    html += `</div>`
  }

  html += `</div>`
  return html
}

// Helper: get duration in milliseconds
const getDurationMs = (duration: Duration): number => {
  switch (duration) {
    case '1h': return 60 * 60 * 1000
    case '6h': return 6 * 60 * 60 * 1000
    case '24h': return 24 * 60 * 60 * 1000
    case 'today': {
      // 计算从今日 0 点到现在的毫秒数
      const now = new Date()
      const startOfDay = new Date(now.getFullYear(), now.getMonth(), now.getDate())
      return now.getTime() - startOfDay.getTime()
    }
    default: return 6 * 60 * 60 * 1000
  }
}

// Helper: get dash array for stroke style
// traffic 模式：全部实线
// tokens/cache 模式：每个 key 有两个 series（正向实线、负向虚线）
const getDashArray = (): number | number[] => {
  if (selectedView.value === 'traffic') {
    return 0 // 全部实线
  }
  // 双向模式：每个 key 产生 2 个 series [正向实线, 负向虚线]
  const keyCount = historyData.value?.keys?.length || 0
  const dashArray: number[] = []
  for (let i = 0; i < keyCount; i++) {
    dashArray.push(0)  // 正向（Input/Read）- 实线
    dashArray.push(5)  // 负向（Output/Write）- 虚线
  }
  return dashArray.length > 0 ? dashArray : 0
}

// Helper: get chart colors aligned with series count
// traffic 模式：每个 key 一个 series，一种颜色
// tokens/cache 模式：每个 key 两个 series（Input/Output），使用相同颜色
const getChartColors = (): string[] => {
  const keyCount = historyData.value?.keys?.length || 0
  if (keyCount === 0) return keyColors

  if (selectedView.value === 'traffic') {
    // 流量模式：每个 key 一种颜色
    return historyData.value!.keys.map((_, i) => keyColors[i % keyColors.length])
  }
  // 双向模式：每个 key 复制颜色（Input 和 Output 同色）
  const colors: string[] = []
  for (let i = 0; i < keyCount; i++) {
    const color = keyColors[i % keyColors.length]
    colors.push(color)  // 正向
    colors.push(color)  // 负向（同色）
  }
  return colors
}

// Fetch data
const refreshData = async (isAutoRefresh = false) => {
  // Prevent out-of-order responses from overwriting newer state
  const requestId = ++refreshRequestId
  isRefreshing.value = true

  // Auto refresh uses silent update without loading state
  if (!isAutoRefresh) {
    isLoading.value = true
  }
  errorMessage.value = ''
  try {
    let newData: ChannelKeyMetricsHistoryResponse
    if (props.channelType === 'responses') {
      newData = await api.getResponsesChannelKeyMetricsHistory(props.channelId, selectedDuration.value)
    } else if (props.channelType === 'gemini') {
      newData = await api.getGeminiChannelKeyMetricsHistory(props.channelId, selectedDuration.value)
    } else {
      newData = await api.getChannelKeyMetricsHistory(props.channelId, selectedDuration.value)
    }

    // Ignore stale response
    if (requestId !== refreshRequestId) return

    // Check if we can use updateSeries (same keys structure)
    const canUpdateInPlace = isAutoRefresh &&
      chartRef.value &&
      historyData.value?.keys?.length === newData.keys?.length &&
      historyData.value?.keys?.every((k, i) => k.keyMask === newData.keys[i].keyMask)

    if (canUpdateInPlace) {
      // Update data in place and use updateSeries for smooth update
      historyData.value = newData
      const newSeries = buildChartSeries(newData)
      chartRef.value?.updateSeries(newSeries, false) // false = no animation reset
    } else {
      // Full update (initial load or structure changed)
      historyData.value = newData
    }
  } catch (error) {
    // Ignore stale error
    if (requestId !== refreshRequestId) return

    console.error('Failed to fetch key metrics history:', error)
    errorMessage.value = error instanceof Error ? error.message : '获取 Key 历史数据失败'
    showError.value = true
    historyData.value = null
  } finally {
    // Only let the latest request update flags
    if (requestId === refreshRequestId) {
      isRefreshing.value = false
      if (!isAutoRefresh) {
        isLoading.value = false
      }
    }
  }
}

// Watchers
watch(selectedDuration, () => {
  savePreference(props.channelType, 'duration', selectedDuration.value)
  refreshData()
}, { flush: 'sync' })

watch(selectedView, () => {
  savePreference(props.channelType, 'viewMode', selectedView.value)
  // View change doesn't need to refetch, just re-render chart
}, { flush: 'sync' })

// Watch channelType changes to reload preferences and refresh data
watch(() => props.channelType, (newChannelType) => {
  const prefs = loadSavedPreferences(newChannelType)
  const oldDuration = selectedDuration.value
  selectedView.value = prefs.view
  selectedDuration.value = prefs.duration
  historyData.value = null
  // Only explicitly refresh if duration didn't change (otherwise duration watcher handles it)
  if (oldDuration === prefs.duration) {
    refreshData()
  }
})

// Initial load and start auto refresh
onMounted(() => {
  refreshData()
  startAutoRefresh()
})

// Cleanup timer on unmount
onUnmounted(() => {
  stopAutoRefresh()
})

// Expose refresh method
defineExpose({
  refreshData
})
</script>

<style scoped>
.key-trend-chart-container {
  padding: 12px 16px;
  background: rgba(var(--v-theme-surface-variant), 0.3);
  border-top: 1px dashed rgba(var(--v-theme-on-surface), 0.2);
}

.v-theme--dark .key-trend-chart-container {
  background: rgba(var(--v-theme-surface-variant), 0.2);
  border-top-color: rgba(255, 255, 255, 0.15);
}

.chart-header {
  flex-wrap: wrap;
  gap: 8px;
}

.chart-area {
  margin-top: 8px;
}
</style>
