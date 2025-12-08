<template>
  <div class="status-badge" :class="[statusClass, { 'has-metrics': showMetrics }]">
    <v-tooltip location="top" content-class="status-tooltip">
      <template #activator="{ props: tooltipProps }">
        <div class="badge-content" v-bind="tooltipProps">
          <v-icon :size="iconSize" class="status-icon">{{ statusIcon }}</v-icon>
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
/* =====================================================
   🎮 状态徽章 - 复古像素主题样式
   Neo-Brutalism: 直角、实体边框、高对比度
   ===================================================== */

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
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgb(var(--v-theme-on-surface));
  cursor: help;
  transition: all 0.1s ease;
}

.v-theme--dark .badge-content {
  border-color: rgba(255, 255, 255, 0.6);
}

.badge-content:hover {
  background: rgba(var(--v-theme-surface-variant), 0.8);
}

.status-label {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

/* 状态样式 - 高对比度实心边框 */
.status-active .badge-content {
  background: #bbf7d0;
  color: #166534;
  border-color: #166534;
}

.status-active .badge-content .status-icon {
  color: #166534 !important;
}

.v-theme--dark .status-active .badge-content {
  background: #166534;
  color: #bbf7d0;
  border-color: #bbf7d0;
}

.v-theme--dark .status-active .badge-content .status-icon {
  color: #bbf7d0 !important;
}

.status-suspended .badge-content {
  background: #fef3c7;
  color: #92400e;
  border-color: #92400e;
  animation: pixel-blink 1.5s step-end infinite;
}

.status-suspended .badge-content .status-icon {
  color: #92400e !important;
}

.v-theme--dark .status-suspended .badge-content {
  background: #92400e;
  color: #fef3c7;
  border-color: #fef3c7;
}

.v-theme--dark .status-suspended .badge-content .status-icon {
  color: #fef3c7 !important;
}

.status-disabled .badge-content {
  background: #e5e7eb;
  color: #6b7280;
  border-color: #6b7280;
}

.status-disabled .badge-content .status-icon {
  color: #6b7280 !important;
}

.v-theme--dark .status-disabled .badge-content {
  background: #374151;
  color: #9ca3af;
  border-color: #9ca3af;
}

.v-theme--dark .status-disabled .badge-content .status-icon {
  color: #9ca3af !important;
}

.status-error .badge-content {
  background: #fecaca;
  color: #991b1b;
  border-color: #991b1b;
}

.status-error .badge-content .status-icon {
  color: #991b1b !important;
}

.v-theme--dark .status-error .badge-content {
  background: #991b1b;
  color: #fecaca;
  border-color: #fecaca;
}

.v-theme--dark .status-error .badge-content .status-icon {
  color: #fecaca !important;
}

.status-unknown .badge-content {
  background: #e5e7eb;
  color: #6b7280;
  border-color: #6b7280;
}

.status-unknown .badge-content .status-icon {
  color: #6b7280 !important;
}

.v-theme--dark .status-unknown .badge-content {
  background: #374151;
  color: #9ca3af;
  border-color: #9ca3af;
}

.v-theme--dark .status-unknown .badge-content .status-icon {
  color: #9ca3af !important;
}

/* 手机端隐藏状态文字，改为像素点样式 */
@media (max-width: 600px) {
  .status-label {
    display: none;
  }

  .badge-content {
    padding: 0;
    background: transparent !important;
    border: none !important;
  }

  .badge-content .v-icon {
    font-size: 0 !important;
    width: 10px;
    height: 10px;
    margin-right: 10px;
    position: relative;
  }

  /* 活跃状态 - 绿色像素点 */
  .status-active .badge-content .v-icon {
    background: #10b981;
    border: 2px solid #065f46;
  }

  .status-active .badge-content .v-icon::after {
    content: '';
    position: absolute;
    top: -3px;
    left: -3px;
    width: 14px;
    height: 14px;
    background: rgba(16, 185, 129, 0.3);
    animation: pixel-pulse 1s step-end infinite;
  }

  /* 熔断状态 - 橙色像素点 */
  .status-suspended .badge-content .v-icon {
    background: #f59e0b;
    border: 2px solid #92400e;
  }

  .status-suspended .badge-content .v-icon::after {
    content: '';
    position: absolute;
    top: -3px;
    left: -3px;
    width: 14px;
    height: 14px;
    background: rgba(245, 158, 11, 0.3);
    animation: pixel-pulse 0.75s step-end infinite;
  }

  /* 禁用状态 - 灰色像素点 */
  .status-disabled .badge-content .v-icon,
  .status-unknown .badge-content .v-icon {
    background: #94a3b8;
    border: 2px solid #475569;
  }

  @keyframes pixel-pulse {
    0%, 100% {
      opacity: 1;
    }
    50% {
      opacity: 0.4;
    }
  }
}

/* 像素风格闪烁动画 */
@keyframes pixel-blink {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.6;
  }
}

.tooltip-content {
  max-width: 200px;
}
</style>

<!-- 非 scoped 样式 - 用于 teleport 到 body 的 tooltip -->
<style>
/* Status tooltip 样式 - 复古像素主题 */
.status-tooltip {
  background: #f5f5f5 !important;
  color: #1a1a1a !important;
  border: 1px solid #333 !important;
  border-radius: 0 !important;
  box-shadow: 3px 3px 0 rgba(0, 0, 0, 0.2) !important;
  padding: 8px 12px !important;
}

.v-theme--dark .status-tooltip {
  background: #2d2d2d !important;
  color: #f5f5f5 !important;
  border-color: #555 !important;
}
</style>
