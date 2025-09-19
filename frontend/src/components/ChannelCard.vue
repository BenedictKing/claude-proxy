<template>
  <v-card
    class="channel-card h-100"
    :class="{ 'current-channel': isCurrent }"
    elevation="3"
    rounded="lg"
    hover
  >
    <v-card-title class="d-flex align-center justify-space-between pa-4 pb-2">
      <div class="text-h6 font-weight-bold text-truncate" style="max-width: 200px;">
        {{ channel.name }}
      </div>
      <div class="d-flex align-center ga-2">
        <v-chip
          :color="getServiceChipColor()"
          size="small"
          variant="elevated"
          density="compact"
        >
          {{ channel.serviceType.toUpperCase() }}
        </v-chip>
        <v-chip
          v-if="isCurrent"
          color="success"
          size="small"
          variant="elevated"
          density="compact"
        >
          <v-icon start size="small">mdi-check</v-icon>
          当前
        </v-chip>
      </div>
    </v-card-title>

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
        
        <div class="d-flex align-center ga-2">
          <v-icon size="16" color="medium-emphasis">mdi-key</v-icon>
          <span class="text-body-2 font-weight-medium">密钥数量:</span>
          <v-chip
            :color="channel.apiKeys.length ? 'success' : 'warning'"
            size="x-small"
            variant="tonal"
            density="compact"
          >
            {{ channel.apiKeys.length }}
          </v-chip>
        </div>
      </div>

      <!-- 状态和延迟 -->
      <div class="d-flex align-center justify-space-between mb-4">
        <div class="d-flex align-center ga-2">
          <span class="text-caption">状态:</span>
          <div class="d-flex align-center ga-1">
            <v-icon 
              :color="getStatusColor()"
              size="small"
            >
              {{ getStatusIcon() }}
            </v-icon>
            <span class="text-caption">{{ getStatusText() }}</span>
          </div>
        </div>
        
        <div v-if="channel.latency !== null" class="d-flex align-center ga-1">
          <span class="text-caption">延迟:</span>
          <v-chip
            :color="getLatencyColor()"
            size="x-small"
            variant="tonal"
            density="compact"
          >
            {{ channel.latency }}ms
          </v-chip>
        </div>
      </div>

      <!-- API密钥管理 -->
      <v-expansion-panels variant="accordion" class="mb-4">
        <v-expansion-panel>
          <v-expansion-panel-title>
            <div class="d-flex align-center ga-2">
              <v-icon size="small">mdi-key-chain</v-icon>
              <span class="text-body-2 font-weight-medium">API密钥管理</span>
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
      <div class="d-flex flex-wrap ga-2">
        <v-btn 
          v-if="!isCurrent"
          size="small"
          color="success"
          variant="tonal"
          @click="$emit('setCurrent', channel.index)"
          prepend-icon="mdi-check"
        >
          设为当前
        </v-btn>
        
        <v-btn
          size="small"
          color="info"
          variant="tonal"
          @click="$emit('ping', channel.index)"
          prepend-icon="mdi-speedometer"
        >
          测试延迟
        </v-btn>
        
        <v-btn
          size="small"
          color="warning"
          variant="tonal"
          @click="$emit('edit', channel)"
          prepend-icon="mdi-pencil"
        >
          编辑
        </v-btn>
        
        <v-btn
          size="small"
          color="error"
          variant="tonal"
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
    'unknown': '未知'
  }
  return textMap[props.channel.status || 'unknown']
}

// 从掩码的密钥获取原始密钥（用于删除操作）
const getOriginalKey = (maskedKey: string) => {
  return maskedKey
}
</script>

<style scoped>
.channel-card {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
  overflow: hidden;
}

.channel-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 25px rgba(0, 0, 0, 0.15) !important;
}

.channel-card.current-channel {
  border: 2px solid rgb(var(--v-theme-success));
  background: linear-gradient(135deg, 
    rgba(var(--v-theme-success), 0.08) 0%, 
    rgba(var(--v-theme-success), 0.03) 50%,
    rgba(var(--v-theme-success), 0.08) 100%);
  box-shadow: 
    0 0 0 2px rgba(var(--v-theme-success), 0.2),
    0 8px 32px rgba(var(--v-theme-success), 0.15),
    0 0 20px rgba(var(--v-theme-success), 0.1);
  transform: translateY(-1px) scale(1.02);
}

.channel-card.current-channel::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: linear-gradient(45deg, 
    transparent 30%, 
    rgba(var(--v-theme-success), 0.05) 50%,
    transparent 70%);
  pointer-events: none;
  animation: shimmer 3s ease-in-out infinite;
}

@keyframes shimmer {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}

.channel-card.current-channel:hover {
  transform: translateY(-3px) scale(1.02);
  box-shadow: 
    0 0 0 2px rgba(var(--v-theme-success), 0.3),
    0 12px 40px rgba(var(--v-theme-success), 0.2),
    0 0 30px rgba(var(--v-theme-success), 0.15);
}

/* 当前渠道标题发光效果 */
.channel-card.current-channel .v-card-title {
  position: relative;
}

.channel-card.current-channel .v-card-title::after {
  content: '';
  position: absolute;
  bottom: -2px;
  left: 0;
  right: 0;
  height: 2px;
  background: linear-gradient(90deg, 
    transparent, 
    rgba(var(--v-theme-success), 0.6), 
    transparent);
  border-radius: 1px;
}
</style>