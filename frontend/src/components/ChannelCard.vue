<template>
  <v-card
    class="channel-card h-100"
    :class="{ 'current-channel': isCurrent }"
    :data-pinned="channel.pinned"
    elevation="0"
    rounded="xl"
    hover
  >
    <!-- 渐变头部背景 -->
    <div class="card-header-gradient">
      <v-card-title class="d-flex align-center justify-space-between pa-4 pb-3 position-relative">
        <div class="d-flex align-center ga-3">
          <!-- 服务类型图标 -->
          <div class="service-icon-wrapper">
            <v-icon 
              :color="getServiceIconColor()"
              size="24"
            >
              {{ getServiceIcon() }}
            </v-icon>
          </div>
          <div>
            <div class="text-h6 font-weight-bold channel-title">
              {{ channel.name }}
            </div>
            <div class="text-caption text-high-emphasis opacity-80">
              {{ getServiceDisplayName() }}
            </div>
          </div>
        </div>
        
        <div class="d-flex align-center ga-2">
          <!-- Pin 按钮 -->
          <v-btn
            size="small"
            variant="text"
            :color="channel.pinned ? 'warning' : 'grey'"
            class="pin-btn"
            rounded="lg"
            @click="$emit('togglePin', channel.index)"
          >
            <v-icon size="16">
              {{ channel.pinned ? 'mdi-pin' : 'mdi-pin-outline' }}
            </v-icon>
          </v-btn>
          
          <v-chip
            :color="getServiceChipColor()"
            size="small"
            variant="flat"
            density="comfortable"
            rounded="lg"
            class="service-chip"
          >
            {{ channel.serviceType.toUpperCase() }}
          </v-chip>
          <v-chip
            v-if="isCurrent"
            color="success"
            size="small"
            variant="flat"
            density="comfortable"
            rounded="lg"
            class="current-chip"
          >
            <v-icon start size="small">mdi-check-circle</v-icon>
            当前
          </v-chip>
        </div>
      </v-card-title>
    </div>

    <v-card-text class="px-4 py-2">
      <!-- 描述 -->
      <div v-if="channel.description" class="text-body-2 text-medium-emphasis mb-3">
        {{ channel.description }}
      </div>

      <!-- 基本信息 -->
      <div class="mb-4">
        <div class="d-flex align-center ga-2 mb-2">
          <v-icon size="16" color="medium-emphasis">mdi-web</v-icon>
          <span class="text-body-2 font-weight-medium">URL:</span>
          <div class="flex-1-1 text-truncate">
            <code class="text-caption bg-surface pa-1 rounded">{{ channel.baseUrl }}</code>
          </div>
        </div>
      </div>

      <!-- 状态和延迟 -->
      <div class="d-flex align-center justify-space-between mb-4">
        <div class="status-indicator">
          <v-tooltip :text="getStatusTooltip()" location="bottom">
            <template #activator="{ props }">
              <div class="status-badge cursor-help" v-bind="props" :class="`status-${channel.status || 'unknown'}`">
                <v-icon 
                  :color="getStatusColor()"
                  size="16"
                  class="status-icon"
                >
                  {{ getStatusIcon() }}
                </v-icon>
                <span class="status-text">{{ getStatusText() }}</span>
              </div>
            </template>
          </v-tooltip>
        </div>
        
        <div v-if="channel.latency !== null" class="latency-indicator">
          <div class="latency-badge" :class="`latency-${getLatencyLevel()}`">
            <v-icon size="14" class="latency-icon">mdi-speedometer</v-icon>
            <span class="latency-text">{{ channel.latency }}ms</span>
          </div>
        </div>
      </div>

      <!-- API密钥管理 -->
      <v-expansion-panels variant="accordion" rounded="lg" class="mb-4">
        <v-expansion-panel>
          <v-expansion-panel-title>
            <div class="d-flex align-center justify-space-between w-100">
              <div class="d-flex align-center ga-2">
                <v-icon size="small">mdi-key-chain</v-icon>
                <span class="text-body-2 font-weight-medium">API密钥管理</span>
              </div>
              <v-chip
                :color="channel.apiKeys.length ? 'success' : 'warning'"
                size="small"
                variant="tonal"
                density="compact"
                rounded="md"
                class="mr-4"
              >
                {{ channel.apiKeys.length }}
              </v-chip>
            </div>
          </v-expansion-panel-title>
          <v-expansion-panel-text>
            <div class="d-flex align-center justify-space-between mb-3">
              <span class="text-body-2 font-weight-medium">已配置的密钥</span>
              <v-btn
                size="small"
                color="primary"
                icon
                variant="elevated"
                rounded="lg"
                @click="$emit('addKey', channel.index)"
              >
                <v-icon>mdi-plus</v-icon>
              </v-btn>
            </div>
            
            <div v-if="channel.apiKeys.length" class="d-flex flex-column ga-2" style="max-height: 150px; overflow-y: auto;">
              <div 
                v-for="(key, index) in channel.apiKeys" 
                :key="index"
                class="d-flex align-center justify-space-between pa-2 bg-surface rounded"
              >
                <code class="text-caption flex-1-1 text-truncate mr-2">{{ key }}</code>
                <v-btn
                  size="x-small"
                  color="error"
                  icon
                  variant="text"
                  rounded="md"
                  @click="$emit('removeKey', channel.index, getOriginalKey(key))"
                >
                  <v-icon size="small">mdi-close</v-icon>
                </v-btn>
              </div>
            </div>
            
            <div v-else class="text-center py-4">
              <span class="text-body-2 text-medium-emphasis">暂无API密钥</span>
            </div>
          </v-expansion-panel-text>
        </v-expansion-panel>
      </v-expansion-panels>

      <!-- 操作按钮 -->
      <div class="action-buttons d-flex flex-wrap ga-2">
        <v-btn 
          v-if="!isCurrent"
          size="small"
          color="success"
          variant="flat"
          rounded="lg"
          class="action-btn primary-action"
          @click="$emit('setCurrent', channel.index)"
          prepend-icon="mdi-check-circle"
        >
          设为当前
        </v-btn>
        
        <v-btn
          size="small"
          color="primary"
          variant="outlined"
          rounded="lg"
          class="action-btn"
          @click="$emit('ping', channel.index)"
          prepend-icon="mdi-speedometer"
        >
          测试延迟
        </v-btn>
        
        <v-btn
          size="small"
          color="info"
          variant="outlined"
          rounded="lg"
          class="action-btn"
          @click="$emit('edit', channel)"
          prepend-icon="mdi-pencil"
        >
          编辑
        </v-btn>
        
        <v-btn
          size="small"
          color="error"
          variant="text"
          rounded="lg"
          class="action-btn danger-action"
          @click="$emit('delete', channel.index)"
          prepend-icon="mdi-delete"
        >
          删除
        </v-btn>
      </div>
    </v-card-text>
  </v-card>
</template>

<script setup lang="ts">
import type { Channel } from '../services/api'

interface Props {
  channel: Channel
  isCurrent: boolean
}

const props = defineProps<Props>()

defineEmits<{
  edit: [channel: Channel]
  delete: [channelId: number]
  setCurrent: [channelId: number]
  addKey: [channelId: number]
  removeKey: [channelId: number, apiKey: string]
  ping: [channelId: number]
  togglePin: [channelId: number]
}>()

// 获取服务类型对应的芯片颜色
const getServiceChipColor = () => {
  const colorMap: Record<string, string> = {
    'openai': 'primary',
    'openaiold': 'info',
    'claude': 'success',
    'gemini': 'warning'
  }
  return colorMap[props.channel.serviceType] || 'surface-variant'
}

// 获取延迟对应的颜色
const getLatencyColor = () => {
  if (!props.channel.latency) return 'surface-variant'
  
  if (props.channel.latency < 200) return 'success'
  if (props.channel.latency < 500) return 'warning'
  return 'error'
}

// 获取状态对应的颜色
const getStatusColor = () => {
  const colorMap: Record<string, string> = {
    'healthy': 'success',
    'error': 'error',
    'unknown': 'warning'
  }
  return colorMap[props.channel.status || 'unknown']
}

// 获取状态图标
const getStatusIcon = () => {
  const iconMap: Record<string, string> = {
    'healthy': 'mdi-check-circle',
    'error': 'mdi-alert-circle',
    'unknown': 'mdi-help-circle'
  }
  return iconMap[props.channel.status || 'unknown']
}

// 获取状态文本
const getStatusText = () => {
  const textMap: Record<string, string> = {
    'healthy': '健康',
    'error': '错误',
    'unknown': '未检测'
  }
  return textMap[props.channel.status || 'unknown']
}

// 状态解释文案（悬浮提示）
const getStatusTooltip = () => {
  const status = props.channel.status || 'unknown'
  if (status === 'healthy') return '连接正常：最近一次检测通过'
  if (status === 'error') return '连接异常：请检查基础 URL、网络或 API 密钥'
  return '尚未检测：请点击“测试延迟”进行检测'
}

// 从掩码的密钥获取原始密钥（用于删除操作）
const getOriginalKey = (maskedKey: string) => {
  return maskedKey
}

// 获取服务类型图标
const getServiceIcon = () => {
  const iconMap: Record<string, string> = {
    'openai': 'mdi-robot',
    'openaiold': 'mdi-robot-outline',
    'claude': 'mdi-message-processing',
    'gemini': 'mdi-diamond-stone'
  }
  return iconMap[props.channel.serviceType] || 'mdi-api'
}

// 获取服务类型图标颜色
const getServiceIconColor = () => {
  const colorMap: Record<string, string> = {
    'openai': 'primary',
    'openaiold': 'info',
    'claude': 'orange',
    'gemini': 'purple'
  }
  return colorMap[props.channel.serviceType] || 'grey'
}

// 获取服务类型显示名称
const getServiceDisplayName = () => {
  const nameMap: Record<string, string> = {
    'openai': 'OpenAI API',
    'openaiold': 'OpenAI Legacy',
    'claude': 'Claude API',
    'gemini': 'Gemini API'
  }
  return nameMap[props.channel.serviceType] || 'Custom API'
}

// 获取延迟等级
const getLatencyLevel = () => {
  if (!props.channel.latency) return 'unknown'
  
  if (props.channel.latency < 200) return 'excellent'
  if (props.channel.latency < 500) return 'good'
  if (props.channel.latency < 1000) return 'fair'
  return 'poor'
}
</script>

<style scoped>
/* --- BASE STYLES (LIGHT MODE) --- */
.channel-card {
  transition: all 0.4s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
  overflow: hidden;
  background-color: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-theme-primary), 0.28);
  box-shadow: 
    0 4px 16px rgba(0, 0, 0, 0.05),
    0 1px 4px rgba(0, 0, 0, 0.02);
  border-radius: 16px;
}

.channel-card:not(.current-channel):hover {
  transform: translateY(-6px) scale(1.02);
  box-shadow: 
    0 20px 40px rgba(0, 0, 0, 0.1),
    0 8px 24px rgba(0, 0, 0, 0.06);
  border-color: rgba(var(--v-theme-primary), 0.5);
}

.card-header-gradient {
  background: linear-gradient(135deg,
    rgba(var(--v-theme-primary), 0.12) 0%,
    rgba(var(--v-theme-primary), 0.06) 50%,
    rgba(var(--v-theme-accent), 0.08) 100%);
  position: relative;
  border-top-left-radius: inherit;
  border-top-right-radius: inherit;
}

.service-icon-wrapper {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.9) 0%, rgba(255, 255, 255, 0.6) 100%);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.8);
  transition: all 0.3s ease;
}

.channel-card:hover .service-icon-wrapper {
  transform: scale(1.1);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
}

.service-chip, .current-chip {
  backdrop-filter: blur(10px);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.4);
}

/* --- CURRENT CHANNEL (LIGHT) --- */
.channel-card.current-channel {
  border-width: 2px !important;
  border-color: rgba(var(--v-theme-primary), 0.5) !important;
  box-shadow: 
    0 8px 32px rgba(var(--v-theme-success), 0.15),
    0 4px 16px rgba(0, 0, 0, 0.08);
  transform: translateY(-2px) scale(1.01);
}

.channel-card.current-channel .card-header-gradient {
  background: linear-gradient(135deg, 
    rgba(var(--v-theme-primary), 0.16) 0%, 
    rgba(var(--v-theme-primary), 0.08) 50%,
    rgba(139, 195, 74, 0.08) 100%);
}

.channel-card.current-channel:hover {
  transform: translateY(-8px) scale(1.03);
  box-shadow: 
    0 24px 48px rgba(var(--v-theme-primary), 0.22),
    0 12px 32px rgba(0, 0, 0, 0.12);
  border-color: rgba(var(--v-theme-primary), 0.65) !important;
}

/* --- INDICATORS (LIGHT) --- */
.status-badge, .latency-badge {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border-radius: 8px;
  font-size: 0.75rem;
  font-weight: 500;
}

.status-badge {
  background-color: rgba(0, 0, 0, 0.05);
}
.status-badge.status-healthy { color: rgb(var(--v-theme-success)); background-color: rgba(var(--v-theme-success), 0.12); }
.status-badge.status-error { color: rgb(var(--v-theme-error)); background-color: rgba(var(--v-theme-error), 0.12); }
.status-badge.status-unknown { color: rgb(var(--v-theme-secondary)); background-color: rgba(var(--v-theme-secondary), 0.12); }

.latency-badge {
  font-weight: 600;
}
.latency-badge.latency-excellent { color: #2e7d32; background: rgba(76, 175, 80, 0.1); }
.latency-badge.latency-good { color: #f57c00; background: rgba(255, 193, 7, 0.1); }
.latency-badge.latency-fair { color: #e65100; background: rgba(255, 152, 0, 0.1); }
.latency-badge.latency-poor { color: #c62828; background: rgba(244, 67, 54, 0.1); }

/* --- PIN BUTTON (LIGHT) --- */
.pin-btn {
  min-width: 32px !important; width: 32px; height: 32px;
  border-radius: 12px !important;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.pin-btn:hover {
  transform: scale(1.1);
}

/* --- KEYFRAMES --- */
@keyframes shimmer {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}

@keyframes slideInUp {
  from { opacity: 0; transform: translateY(30px); }
  to { opacity: 1; transform: translateY(0); }
}

.channel-card {
  animation: slideInUp 0.6s ease-out;
}

/* 
██████╗ ██╗  ██╗██████╗  ██╗  ██╗
██╔══██╗██║  ██║██╔══██╗██║ ██╔╝
██║  ██║███████║██████╔╝█████╔╝ 
██║  ██║██╔══██║██╔══██╗██╔═██╗ 
██████╔╝██║  ██║██║  ██║██║  ██╗
╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝
*/
/* Prefer Vuetify theme class over media query to honor manual toggles */
.v-theme--dark .channel-card {
  border: 1px solid rgba(var(--v-theme-primary), 0.45);
  box-shadow:
    0 4px 24px rgba(0, 0, 0, 0.28),
    0 1px 8px rgba(0, 0, 0, 0.18);
}

.v-theme--dark .channel-card:not(.current-channel):hover {
  border-color: rgba(var(--v-theme-primary), 0.65);
  box-shadow:
    0 20px 40px rgba(0, 0, 0, 0.36),
    0 8px 24px rgba(0, 0, 0, 0.24);
}

.v-theme--dark .card-header-gradient {
  background: linear-gradient(135deg,
    rgba(var(--v-theme-primary), 0.18) 0%,
    rgba(var(--v-theme-primary), 0.10) 50%,
    rgba(156, 39, 176, 0.16) 100%);
}

.v-theme--dark .service-icon-wrapper {
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.12) 0%, rgba(255, 255, 255, 0.08) 100%);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.24);
  border: 1px solid rgba(255, 255, 255, 0.15);
}

.v-theme--dark .channel-card:hover .service-icon-wrapper {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.32);
  border-color: rgba(255, 255, 255, 0.2);
}

.v-theme--dark .service-chip,
.v-theme--dark .current-chip {
  border: 1px solid rgba(255, 255, 255, 0.15);
}

/* --- CURRENT CHANNEL (DARK) --- */
.v-theme--dark .channel-card.current-channel {
  border-width: 2px !important;
  border-color: rgba(var(--v-theme-primary), 0.7) !important;
  box-shadow:
    0 8px 32px rgba(var(--v-theme-primary), 0.3),
    0 4px 16px rgba(0, 0, 0, 0.32);
}

.v-theme--dark .channel-card.current-channel .card-header-gradient {
  background: linear-gradient(135deg,
    rgba(var(--v-theme-primary), 0.24) 0%,
    rgba(var(--v-theme-primary), 0.14) 50%,
    rgba(139, 195, 74, 0.18) 100%);
}

.v-theme--dark .channel-card.current-channel:hover {
  box-shadow:
    0 24px 48px rgba(var(--v-theme-primary), 0.34),
    0 12px 32px rgba(0, 0, 0, 0.36);
  border-color: rgba(var(--v-theme-primary), 0.85) !important;
}

/* --- INDICATORS (DARK) --- */
.v-theme--dark .status-badge {
  background-color: rgba(255, 255, 255, 0.1);
}
.v-theme--dark .status-badge.status-healthy { color: #b6e3be; background-color: rgba(52, 211, 153, 0.2); }
.v-theme--dark .status-badge.status-error { color: #f4b4b4; background-color: rgba(248, 113, 113, 0.22); }
.v-theme--dark .status-badge.status-unknown { color: #cbd5e1; background-color: rgba(148, 163, 184, 0.2); }

.v-theme--dark .latency-badge.latency-excellent { color: #b6e3be; background: rgba(52, 211, 153, 0.25); }
.v-theme--dark .latency-badge.latency-good { color: #fde68a; background: rgba(251, 191, 36, 0.22); }
.v-theme--dark .latency-badge.latency-fair { color: #fcd49b; background: rgba(251, 146, 60, 0.25); }
.v-theme--dark .latency-badge.latency-poor { color: #f4b4b4; background: rgba(248, 113, 113, 0.28); }
</style>
