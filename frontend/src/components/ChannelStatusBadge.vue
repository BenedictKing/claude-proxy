<template>
  <div class="status-badge" :class="[statusClass, { 'has-metrics': showMetrics }]">
    <v-tooltip location="top">
      <template #activator="{ props: tooltipProps }">
        <div class="badge-content" v-bind="tooltipProps">
          <v-icon :size="iconSize" :color="statusColor">{{ statusIcon }}</v-icon>
          <span v-if="showLabel" class="status-label">{{ statusLabel }}</span>
        </div>
      </template>
      <div class="tooltip-content">
        <div class="font-weight-bold mb-1">{{ statusLabel }}</div>
        <template v-if="metrics">
          <div class="text-caption">
            <div>请求数: {{ metrics.requestCount }}</div>
            <div>成功率: {{ metrics.successRate?.toFixed(1) || 0 }}%</div>
            <div>连续失败: {{ metrics.consecutiveFailures }}</div>
            <div v-if="metrics.lastSuccessAt">最后成功: {{ formatTime(metrics.lastSuccessAt) }}</div>
            <div v-if="metrics.lastFailureAt">最后失败: {{ formatTime(metrics.lastFailureAt) }}</div>
          </div>
        </template>
        <div v-else class="text-caption text-medium-emphasis">暂无指标数据</div>
      </div>
    </v-tooltip>

    <!-- 熔断指示器 -->
    <v-badge
      v-if="isSuspended && metrics?.consecutiveFailures"
      :content="metrics.consecutiveFailures"
      color="error"
      :offset-x="-4"
      :offset-y="-4"
      class="failure-badge"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ChannelStatus, ChannelMetrics } from '../services/api'

const props = withDefaults(defineProps<{
  status: ChannelStatus | 'healthy' | 'error' | 'unknown'
  metrics?: ChannelMetrics
  showLabel?: boolean
  size?: 'small' | 'default' | 'large'
}>(), {
  showLabel: true,
  size: 'default'
})

// 状态配置映射
const STATUS_CONFIG: Record<string, { icon: string; color: string; label: string; class: string }> = {
  active: {
    icon: 'mdi-check-circle',
    color: 'success',
    label: '活跃',
    class: 'status-active'
  },
  healthy: {
    icon: 'mdi-check-circle',
    color: 'success',
    label: '健康',
    class: 'status-active'
  },
  suspended: {
    icon: 'mdi-pause-circle',
    color: 'warning',
    label: '熔断',
    class: 'status-suspended'
  },
  disabled: {
    icon: 'mdi-close-circle',
    color: 'error',
    label: '禁用',
    class: 'status-disabled'
  },
  error: {
    icon: 'mdi-alert-circle',
    color: 'error',
    label: '错误',
    class: 'status-error'
  },
  unknown: {
    icon: 'mdi-help-circle',
    color: 'grey',
    label: '未知',
    class: 'status-unknown'
  }
}

// 计算属性
const statusConfig = computed(() => {
  return STATUS_CONFIG[props.status] || STATUS_CONFIG.unknown
})

const statusIcon = computed(() => statusConfig.value.icon)
const statusColor = computed(() => statusConfig.value.color)
const statusLabel = computed(() => statusConfig.value.label)
const statusClass = computed(() => statusConfig.value.class)

const iconSize = computed(() => {
  switch (props.size) {
    case 'small': return 16
    case 'large': return 24
    default: return 20
  }
})

const isSuspended = computed(() => props.status === 'suspended')
const showMetrics = computed(() => !!props.metrics)

// 格式化时间
const formatTime = (dateStr: string): string => {
  const date = new Date(dateStr)
  const now = new Date()
  const diff = now.getTime() - date.getTime()

  if (diff < 60000) {
    return '刚刚'
  } else if (diff < 3600000) {
    return `${Math.floor(diff / 60000)} 分钟前`
  } else if (diff < 86400000) {
    return `${Math.floor(diff / 3600000)} 小时前`
  } else {
    return date.toLocaleDateString()
  }
}
</script>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  position: relative;
}

.badge-content {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border-radius: 16px;
  background: rgba(var(--v-theme-surface-variant), 0.5);
  cursor: help;
  transition: all 0.2s ease;
}

.badge-content:hover {
  background: rgba(var(--v-theme-surface-variant), 0.8);
}

.status-label {
  font-size: 12px;
  font-weight: 500;
}

/* 状态样式 */
.status-active .badge-content {
  background: rgba(var(--v-theme-success), 0.1);
  color: rgb(var(--v-theme-success));
}

.status-suspended .badge-content {
  background: rgba(var(--v-theme-warning), 0.15);
  color: rgb(var(--v-theme-warning));
  animation: pulse-warning 2s infinite;
}

.status-disabled .badge-content {
  background: rgba(var(--v-theme-error), 0.1);
  color: rgb(var(--v-theme-error));
}

.status-error .badge-content {
  background: rgba(var(--v-theme-error), 0.15);
  color: rgb(var(--v-theme-error));
}

.status-unknown .badge-content {
  background: rgba(var(--v-theme-grey), 0.1);
  color: rgb(var(--v-theme-grey));
}

/* 熔断闪烁动画 */
@keyframes pulse-warning {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.7;
  }
}

.tooltip-content {
  max-width: 200px;
}

.failure-badge {
  position: absolute;
  top: -4px;
  right: -4px;
}
</style>
