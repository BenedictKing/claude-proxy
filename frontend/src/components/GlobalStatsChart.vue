<template>
  <div class="global-stats-chart-container">
    <!-- Snackbar for error notification -->
    <v-snackbar v-model="showError" color="error" :timeout="3000" location="top">
      {{ errorMessage }}
      <template #actions>
        <v-btn variant="text" @click="showError = false">关闭</v-btn>
      </template>
    </v-snackbar>

    <!-- Header: Duration selector + View switcher -->
    <div class="chart-header d-flex align-center justify-space-between mb-3 flex-wrap ga-2">
      <div class="d-flex align-center ga-2">
        <!-- Duration selector -->
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

      <!-- View switcher -->
      <v-btn-toggle v-model="selectedView" mandatory density="compact" variant="outlined" divided :disabled="isLoading">
        <v-btn value="traffic" size="x-small">
          <v-icon size="small" class="mr-1">mdi-chart-line</v-icon>
          流量
        </v-btn>
        <v-btn value="tokens" size="x-small">
          <v-icon size="small" class="mr-1">mdi-chart-areaspline</v-icon>
          Token
        </v-btn>
      </v-btn-toggle>
    </div>

    <!-- Summary cards -->
    <div v-if="summary && !compact" class="summary-cards d-flex flex-wrap ga-2 mb-3">
      <div class="summary-card">
        <div class="summary-label">总请求</div>
        <div class="summary-value">{{ formatNumber(summary.totalRequests) }}</div>
      </div>
      <div class="summary-card">
        <div class="summary-label">成功率</div>
        <div class="summary-value" :class="{ 'text-success': summary.avgSuccessRate >= 95, 'text-warning': summary.avgSuccessRate >= 80 && summary.avgSuccessRate < 95, 'text-error': summary.avgSuccessRate < 80 }">
          {{ summary.avgSuccessRate.toFixed(1) }}%
        </div>
      </div>
      <div class="summary-card">
        <div class="summary-label">输入 Token</div>
        <div class="summary-value">{{ formatNumber(summary.totalInputTokens) }}</div>
      </div>
      <div class="summary-card">
        <div class="summary-label">输出 Token</div>
        <div class="summary-value">{{ formatNumber(summary.totalOutputTokens) }}</div>
      </div>
    </div>

    <!-- Compact summary (single line) -->
    <div v-if="summary && compact" class="compact-summary d-flex align-center ga-3 mb-2 text-caption">
      <span><strong>{{ formatNumber(summary.totalRequests) }}</strong> 请求</span>
      <span :class="{ 'text-success': summary.avgSuccessRate >= 95, 'text-warning': summary.avgSuccessRate >= 80 && summary.avgSuccessRate < 95, 'text-error': summary.avgSuccessRate < 80 }">
        <strong>{{ summary.avgSuccessRate.toFixed(1) }}%</strong> 成功
      </span>
      <span><strong>{{ formatNumber(summary.totalInputTokens) }}</strong> 输入</span>
      <span><strong>{{ formatNumber(summary.totalOutputTokens) }}</strong> 输出</span>
    </div>

    <!-- Loading state -->
    <div v-if="isLoading" class="d-flex justify-center align-center" :style="{ height: chartHeight + 'px' }">
      <v-progress-circular indeterminate size="32" color="primary" />
    </div>

    <!-- Empty state -->
    <div v-else-if="!hasData" class="d-flex flex-column justify-center align-center text-medium-emphasis" :style="{ height: chartHeight + 'px' }">
      <v-icon size="40" color="grey-lighten-1">mdi-chart-timeline-variant</v-icon>
      <div class="text-caption mt-2">选定时间范围内没有请求记录</div>
    </div>

    <!-- Chart -->
    <div v-else class="chart-area">
      <apexchart
        ref="chartRef"
        type="area"
        :height="chartHeight"
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
import { api, type GlobalStatsHistoryResponse, type GlobalHistoryDataPoint, type GlobalStatsSummary } from '../services/api'

// Register apexchart component
const apexchart = VueApexCharts

// Props
const props = withDefaults(defineProps<{
  apiType: 'messages' | 'responses' | 'gemini'
  compact?: boolean
}>(), {
  compact: false
})

// Types
type ViewMode = 'traffic' | 'tokens'
type Duration = '1h' | '6h' | '24h' | 'today'

// LocalStorage keys for preferences (per apiType)
const getStorageKey = (apiType: string, key: string) => `globalStats:${apiType}:${key}`

// Load saved preferences from localStorage (per apiType)
const loadSavedPreferences = (apiType: string) => {
  const savedView = localStorage.getItem(getStorageKey(apiType, 'viewMode')) as ViewMode | null
  const savedDuration = localStorage.getItem(getStorageKey(apiType, 'duration')) as Duration | null
  return {
    view: savedView && ['traffic', 'tokens'].includes(savedView) ? savedView : 'traffic',
    duration: savedDuration && ['1h', '6h', '24h', 'today'].includes(savedDuration) ? savedDuration : '6h'
  }
}

// Save preference to localStorage
const savePreference = (apiType: string, key: string, value: string) => {
  localStorage.setItem(getStorageKey(apiType, key), value)
}

// Theme
const theme = useTheme()
const isDark = computed(() => theme.global.current.value.dark)

// Load saved preferences for current apiType
const savedPrefs = loadSavedPreferences(props.apiType)

// State (initialized from saved preferences)
const selectedView = ref<ViewMode>(savedPrefs.view)
const selectedDuration = ref<Duration>(savedPrefs.duration)
const isLoading = ref(false)
const historyData = ref<GlobalStatsHistoryResponse | null>(null)
const showError = ref(false)
const errorMessage = ref('')

// Chart ref for updateSeries
const chartRef = ref<InstanceType<typeof VueApexCharts> | null>(null)

// Auto refresh timer (2 seconds interval, same as KeyTrendChart)
const AUTO_REFRESH_INTERVAL = 2000
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null

const startAutoRefresh = () => {
  stopAutoRefresh()
  autoRefreshTimer = setInterval(() => {
    if (!isLoading.value) {
      refreshData(true)
    }
  }, AUTO_REFRESH_INTERVAL)
}

const stopAutoRefresh = () => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

// Chart height based on compact mode
const chartHeight = computed(() => props.compact ? 180 : 260)

// Summary data
const summary = computed<GlobalStatsSummary | null>(() => historyData.value?.summary || null)

// Check if has data
const hasData = computed(() => {
  if (!historyData.value?.dataPoints) return false
  return historyData.value.dataPoints.length > 0 &&
    historyData.value.dataPoints.some(dp => dp.requestCount > 0)
})

// Chart colors
const chartColors = {
  traffic: {
    primary: '#3b82f6',    // Blue for requests
    success: '#10b981',    // Green for success
    failure: '#ef4444'     // Red for failure
  },
  tokens: {
    input: '#8b5cf6',      // Purple for input
    output: '#f97316'      // Orange for output
  }
}

// Format number for display
const formatNumber = (num: number): string => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toFixed(0)
}

// Chart options
const chartOptions = computed<ApexOptions>(() => {
  const mode = selectedView.value

  return {
    chart: {
      toolbar: { show: false },
      zoom: { enabled: false },
      background: 'transparent',
      fontFamily: 'inherit',
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
    colors: mode === 'traffic'
      ? [chartColors.traffic.primary, chartColors.traffic.success]
      : [chartColors.tokens.input, chartColors.tokens.output],
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
      dashArray: mode === 'tokens' ? [0, 5] : [0, 0]
    },
    grid: {
      borderColor: isDark.value ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.1)',
      padding: { left: 10, right: 10 }
    },
    xaxis: {
      type: 'datetime',
      labels: {
        datetimeUTC: false,
        format: 'HH:mm',
        style: { fontSize: '10px' }
      },
      axisBorder: { show: false },
      axisTicks: { show: false }
    },
    yaxis: mode === 'tokens' ? [
      {
        seriesName: '输入 Token',
        labels: {
          formatter: (val: number) => formatNumber(val),
          style: { fontSize: '11px' }
        },
        min: 0
      },
      {
        seriesName: '输出 Token',
        opposite: true,
        labels: {
          formatter: (val: number) => formatNumber(val),
          style: { fontSize: '11px' }
        },
        min: 0
      }
    ] : {
      labels: {
        formatter: (val: number) => Math.round(val).toString(),
        style: { fontSize: '11px' }
      },
      min: 0
    },
    tooltip: {
      x: {
        format: 'MM-dd HH:mm'
      },
      y: {
        formatter: (val: number) => mode === 'traffic'
          ? `${Math.round(val)} 请求`
          : formatNumber(val)
      }
    },
    legend: {
      show: true,
      position: 'top' as const,
      horizontalAlign: 'right' as const,
      fontSize: '11px',
      markers: { size: 4 }
    }
  }
})

// Build chart series
const chartSeries = computed(() => {
  if (!historyData.value?.dataPoints) return []

  const dataPoints = historyData.value.dataPoints
  const mode = selectedView.value

  if (mode === 'traffic') {
    return [
      {
        name: '总请求',
        data: dataPoints.map(dp => ({
          x: new Date(dp.timestamp).getTime(),
          y: dp.requestCount
        }))
      },
      {
        name: '成功',
        data: dataPoints.map(dp => ({
          x: new Date(dp.timestamp).getTime(),
          y: dp.successCount
        }))
      }
    ]
  } else {
    return [
      {
        name: '输入 Token',
        data: dataPoints.map(dp => ({
          x: new Date(dp.timestamp).getTime(),
          y: dp.inputTokens
        }))
      },
      {
        name: '输出 Token',
        data: dataPoints.map(dp => ({
          x: new Date(dp.timestamp).getTime(),
          y: dp.outputTokens
        }))
      }
    ]
  }
})

// Fetch data
const refreshData = async (isAutoRefresh = false) => {
  if (!isAutoRefresh) {
    isLoading.value = true
  }
  errorMessage.value = ''

  try {
    let newData: GlobalStatsHistoryResponse
    if (props.apiType === 'messages') {
      newData = await api.getMessagesGlobalStats(selectedDuration.value)
    } else if (props.apiType === 'gemini') {
      newData = await api.getGeminiGlobalStats(selectedDuration.value)
    } else {
      newData = await api.getResponsesGlobalStats(selectedDuration.value)
    }

    // Check if we can use updateSeries for smooth update
    const canUpdateInPlace = isAutoRefresh &&
      chartRef.value &&
      historyData.value?.dataPoints?.length === newData.dataPoints?.length

    if (canUpdateInPlace) {
      historyData.value = newData
      const series = chartSeries.value
      chartRef.value?.updateSeries(series, false)
    } else {
      historyData.value = newData
    }
  } catch (error) {
    console.error('Failed to fetch global stats:', error)
    errorMessage.value = error instanceof Error ? error.message : '获取全局统计数据失败'
    showError.value = true
    historyData.value = null
  } finally {
    if (!isAutoRefresh) {
      isLoading.value = false
    }
  }
}

// Watchers
watch(selectedDuration, (newVal) => {
  savePreference(props.apiType, 'duration', newVal)
  refreshData()
})

watch(selectedView, (newVal) => {
  savePreference(props.apiType, 'viewMode', newVal)
})

watch(() => props.apiType, (newApiType) => {
  // Load preferences for the new apiType
  const prefs = loadSavedPreferences(newApiType)
  selectedView.value = prefs.view
  selectedDuration.value = prefs.duration
  refreshData()
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
  refreshData,
  startAutoRefresh,
  stopAutoRefresh
})
</script>

<style scoped>
.global-stats-chart-container {
  padding: 12px 16px;
}

.summary-cards {
  display: flex;
  flex-wrap: wrap;
}

.summary-card {
  flex: 1 1 auto;
  min-width: 80px;
  padding: 8px 12px;
  background: rgba(var(--v-theme-surface-variant), 0.3);
  border-radius: 6px;
  text-align: center;
}

.v-theme--dark .summary-card {
  background: rgba(var(--v-theme-surface-variant), 0.2);
}

.summary-label {
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.6);
  margin-bottom: 2px;
}

.summary-value {
  font-size: 16px;
  font-weight: 600;
}

.compact-summary {
  padding: 4px 8px;
  background: rgba(var(--v-theme-surface-variant), 0.2);
  border-radius: 4px;
}

.chart-header {
  flex-wrap: wrap;
  gap: 8px;
}

.chart-area {
  margin-top: 8px;
}

/* Responsive adjustments */
@media (max-width: 600px) {
  .summary-card {
    min-width: 70px;
    padding: 6px 8px;
  }

  .summary-value {
    font-size: 14px;
  }
}
</style>
