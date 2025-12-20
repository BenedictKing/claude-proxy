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
        type="area"
        height="280"
        :options="chartOptions"
        :series="chartSeries"
      />
    </div>

    <!-- 底部 Key 快照卡片 -->
    <div v-if="hasData && keySnapshots.length > 0" class="key-snapshots mt-3">
      <v-row dense>
        <v-col v-for="(snapshot, index) in keySnapshots" :key="index" cols="auto">
          <v-card
            :color="snapshot.hasError ? 'error' : undefined"
            :class="snapshot.hasError ? 'text-white' : ''"
            class="snapshot-card"
            density="compact"
            variant="outlined"
          >
            <v-card-text class="pa-2">
              <div class="d-flex align-center ga-2 mb-1">
                <div class="key-color-dot" :style="{ backgroundColor: snapshot.color }" />
                <span class="text-caption font-weight-medium">{{ snapshot.keyMask }}</span>
              </div>
              <!-- 只在有 Token 数据时显示 -->
              <div v-if="snapshot.inputTokens > 0 || snapshot.outputTokens > 0" class="snapshot-stats">
                <div class="text-caption">
                  <span class="text-medium-emphasis">In:</span>
                  <span class="ml-1">{{ formatNumber(snapshot.inputTokens) }}</span>
                  <span class="text-medium-emphasis ml-2">Out:</span>
                  <span class="ml-1">{{ formatNumber(snapshot.outputTokens) }}</span>
                </div>
              </div>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useTheme } from 'vuetify'
import VueApexCharts from 'vue3-apexcharts'
import { api, type ChannelKeyMetricsHistoryResponse, type KeyHistoryData } from '../services/api'

// Register apexchart component
const apexchart = VueApexCharts

// Props
const props = defineProps<{
  channelId: number
  isResponses: boolean
}>()

// View mode type
type ViewMode = 'traffic' | 'tokens' | 'cache'
type Duration = '1h' | '6h' | '24h'

// Theme
const theme = useTheme()
const isDark = computed(() => theme.global.current.value.dark)

// State
const selectedView = ref<ViewMode>('traffic')
const selectedDuration = ref<Duration>('1h')
const isLoading = ref(false)
const historyData = ref<ChannelKeyMetricsHistoryResponse | null>(null)
const showError = ref(false)
const errorMessage = ref('')

// Auto refresh timer (1 minute interval)
const AUTO_REFRESH_INTERVAL = 60 * 1000
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null

const startAutoRefresh = () => {
  stopAutoRefresh()
  autoRefreshTimer = setInterval(() => {
    // Skip if already loading to prevent concurrent requests
    if (!isLoading.value) {
      refreshData()
    }
  }, AUTO_REFRESH_INTERVAL)
}

const stopAutoRefresh = () => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

// Key colors (与后端一致)
const keyColors = ['#3b82f6', '#f97316', '#10b981', '#8b5cf6', '#ec4899']

// Computed: check if has data
const hasData = computed(() => {
  if (!historyData.value) return false
  return historyData.value.keys &&
    historyData.value.keys.length > 0 &&
    historyData.value.keys.some(k => k.dataPoints && k.dataPoints.length > 0)
})

// Computed: get all data points flattened
const allDataPoints = computed(() => {
  if (!historyData.value?.keys) return []
  return historyData.value.keys.flatMap(k => k.dataPoints || [])
})

// Computed: chart options
const chartOptions = computed(() => {
  const mode = selectedView.value

  // 根据视图模式确定 yaxis 配置
  let yaxisConfig: any = {
    labels: {
      formatter: (val: number) => formatAxisValue(val, mode),
      style: { fontSize: '11px' }
    }
  }

  // 双向模式（Tokens I/O, Cache R/W）使用独立的正负范围
  if (mode === 'tokens' || mode === 'cache') {
    const { maxPositive, maxNegative } = getMaxValues(mode)
    yaxisConfig = {
      ...yaxisConfig,
      min: -maxNegative * 1.1,
      max: maxPositive * 1.1,
      tickAmount: 6
    }
  } else {
    yaxisConfig.min = 0
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
      type: 'gradient',
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
      curve: 'smooth',
      width: 2,
      // traffic 模式全用实线；双向模式(tokens/cache)：正向(Input/Read)实线，负向(Output/Write)虚线
      dashArray: getDashArray()
    },
    grid: {
      borderColor: isDark.value ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.1)',
      strokeDashArray: mode === 'traffic' ? 0 : 3,
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
    tooltip: {
      x: {
        format: 'MM-dd HH:mm'
      },
      y: {
        formatter: (val: number) => formatTooltipValue(val, mode)
      }
    },
    legend: {
      show: false
    },
    annotations: {
      yaxis: [{
        y: 0,
        borderColor: isDark.value ? 'rgba(255,255,255,0.3)' : 'rgba(0,0,0,0.3)',
        strokeDashArray: 5,
        opacity: 0.8
      }]
    }
  }
})

// Computed: chart series data
const chartSeries = computed(() => {
  if (!historyData.value?.keys) return []

  const mode = selectedView.value
  const result: { name: string; data: { x: number; y: number }[] }[] = []

  historyData.value.keys.forEach((keyData, keyIndex) => {
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

      // 负向（Output/Creation）- 使用虚线
      result.push({
        name: `${keyData.keyMask} ${outLabel}`,
        data: keyData.dataPoints.map(dp => {
          let value = 0
          if (mode === 'tokens') {
            value = -dp.outputTokens
          } else {
            value = -dp.cacheCreationTokens
          }
          return { x: new Date(dp.timestamp).getTime(), y: value }
        })
      })
    }
  })

  return result
})

// Computed: Key 快照数据（当前时间窗口的汇总）
const keySnapshots = computed(() => {
  if (!historyData.value?.keys) return []

  const now = Date.now()
  const windowMs = getDurationMs(selectedDuration.value)
  const windowStart = now - windowMs

  return historyData.value.keys.map((keyData, index) => {
    const color = keyColors[index % keyColors.length]

    // 只计算时间窗口内的数据
    const windowData = keyData.dataPoints.filter(dp => {
      const ts = new Date(dp.timestamp).getTime()
      return ts >= windowStart && ts <= now
    })

    // 汇总统计
    const totalInput = windowData.reduce((sum, dp) => sum + dp.inputTokens, 0)
    const totalOutput = windowData.reduce((sum, dp) => sum + dp.outputTokens, 0)
    const totalRequests = windowData.reduce((sum, dp) => sum + dp.requestCount, 0)
    const totalFailures = windowData.reduce((sum, dp) => sum + dp.failureCount, 0)

    // 检查是否有错误（失败率 > 10%）
    const hasError = totalRequests > 0 && (totalFailures / totalRequests) > 0.1

    return {
      keyMask: keyData.keyMask,
      color,
      inputTokens: totalInput,
      outputTokens: totalOutput,
      hasError
    }
  })
})

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

// Helper: get max values for positive and negative directions separately
const getMaxValues = (mode: ViewMode): { maxPositive: number; maxNegative: number } => {
  if (!historyData.value?.keys) return { maxPositive: 1000, maxNegative: 1000 }

  let maxPositive = 0
  let maxNegative = 0
  for (const keyData of historyData.value.keys) {
    for (const dp of keyData.dataPoints) {
      switch (mode) {
        case 'tokens':
          maxPositive = Math.max(maxPositive, dp.inputTokens)
          maxNegative = Math.max(maxNegative, dp.outputTokens)
          break
        case 'cache':
          maxPositive = Math.max(maxPositive, dp.cacheReadTokens)
          maxNegative = Math.max(maxNegative, dp.cacheCreationTokens)
          break
      }
    }
  }
  return {
    maxPositive: maxPositive || 1000,
    maxNegative: maxNegative || 1000
  }
}

// Helper: get duration in milliseconds
const getDurationMs = (duration: Duration): number => {
  switch (duration) {
    case '1h': return 60 * 60 * 1000
    case '6h': return 6 * 60 * 60 * 1000
    case '24h': return 24 * 60 * 60 * 1000
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
const refreshData = async () => {
  isLoading.value = true
  errorMessage.value = ''
  try {
    if (props.isResponses) {
      historyData.value = await api.getResponsesChannelKeyMetricsHistory(props.channelId, selectedDuration.value)
    } else {
      historyData.value = await api.getChannelKeyMetricsHistory(props.channelId, selectedDuration.value)
    }
  } catch (error) {
    console.error('Failed to fetch key metrics history:', error)
    errorMessage.value = error instanceof Error ? error.message : '获取 Key 历史数据失败'
    showError.value = true
    historyData.value = null
  } finally {
    isLoading.value = false
  }
}

// Watchers
watch(selectedDuration, () => {
  refreshData()
})

watch(selectedView, () => {
  // View change doesn't need to refetch, just re-render chart
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

.key-color-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.snapshot-card {
  min-width: 120px;
  transition: all 0.2s ease;
}

.snapshot-card:hover {
  transform: translateY(-2px);
}

.snapshot-stats {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
</style>
