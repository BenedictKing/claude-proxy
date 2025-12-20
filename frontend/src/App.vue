<template>
  <v-app>
    <!-- 自动认证加载提示 - 只在真正进行自动认证时显示 -->
    <v-overlay
      :model-value="isAutoAuthenticating && !isInitialized"
      persistent
      class="align-center justify-center"
      scrim="black"
    >
      <v-card class="pa-6 text-center" max-width="400" rounded="lg">
        <v-progress-circular indeterminate :size="64" :width="6" color="primary" class="mb-4" />
        <div class="text-h6 mb-2">正在验证访问权限</div>
        <div class="text-body-2 text-medium-emphasis">使用保存的访问密钥进行身份验证...</div>
      </v-card>
    </v-overlay>

    <!-- 认证界面 -->
    <v-dialog v-model="showAuthDialog" persistent max-width="500">
      <v-card class="pa-4">
        <v-card-title class="text-h5 text-center mb-4"> 🔐 Claude Proxy 管理界面 </v-card-title>

        <v-card-text>
          <v-alert v-if="authError" type="error" variant="tonal" class="mb-4">
            {{ authError }}
          </v-alert>

          <v-form @submit.prevent="handleAuthSubmit">
            <v-text-field
              v-model="authKeyInput"
              label="访问密钥 (PROXY_ACCESS_KEY)"
              type="password"
              variant="outlined"
              prepend-inner-icon="mdi-key"
              :rules="[v => !!v || '请输入访问密钥']"
              required
              autofocus
              @keyup.enter="handleAuthSubmit"
            />

            <v-btn type="submit" color="primary" block size="large" class="mt-4" :loading="authLoading">
              访问管理界面
            </v-btn>
          </v-form>

          <v-divider class="my-4" />

          <v-alert type="info" variant="tonal" density="compact" class="mb-0">
            <div class="text-body-2">
              <p class="mb-2"><strong>🔒 安全提示：</strong></p>
              <ul class="ml-4 mb-0">
                <li>访问密钥在服务器的 <code>PROXY_ACCESS_KEY</code> 环境变量中设置</li>
                <li>密钥将安全保存在本地，下次访问将自动验证登录</li>
                <li>请勿与他人分享您的访问密钥</li>
                <li>如果怀疑密钥泄露，请立即更改服务器配置</li>
                <li>连续 {{ MAX_AUTH_ATTEMPTS }} 次认证失败将锁定 5 分钟</li>
              </ul>
            </div>
          </v-alert>
        </v-card-text>
      </v-card>
    </v-dialog>

    <!-- 应用栏 - 毛玻璃效果 -->
    <v-app-bar elevation="0" :height="$vuetify.display.mobile ? 56 : 72" class="app-header">
      <template #prepend>
        <div class="app-logo">
          <v-icon :size="$vuetify.display.mobile ? 22 : 32" color="white"> mdi-rocket-launch </v-icon>
        </div>
      </template>

      <!-- 自定义标题容器 - 替代 v-app-bar-title -->
      <div class="header-title">
        <div :class="$vuetify.display.mobile ? 'text-body-2' : 'text-h6'" class="font-weight-bold d-flex align-center">
          <span class="api-type-text" :class="{ active: activeTab === 'messages' }" @click="activeTab = 'messages'">
            Claude
          </span>
          <span class="api-type-text separator">/</span>
          <span class="api-type-text" :class="{ active: activeTab === 'responses' }" @click="activeTab = 'responses'">
            Codex
          </span>
          <span class="brand-text d-none d-sm-inline">API Proxy</span>
        </div>
      </div>

      <v-spacer></v-spacer>

      <!-- 版本信息 -->
      <div
        v-if="versionInfo.currentVersion"
        class="version-badge"
        :class="{
          'version-clickable': versionInfo.status === 'update-available' || versionInfo.status === 'latest',
          'version-checking': versionInfo.status === 'checking',
          'version-latest': versionInfo.status === 'latest',
          'version-update': versionInfo.status === 'update-available'
        }"
        @click="handleVersionClick"
      >
        <v-icon
          v-if="versionInfo.status === 'checking'"
          size="14"
          class="mr-1"
        >mdi-clock-outline</v-icon>
        <v-icon
          v-else-if="versionInfo.status === 'latest'"
          size="14"
          class="mr-1"
          color="success"
        >mdi-check-circle</v-icon>
        <v-icon
          v-else-if="versionInfo.status === 'update-available'"
          size="14"
          class="mr-1"
          color="warning"
        >mdi-alert</v-icon>
        <span class="version-text">{{ versionInfo.currentVersion }}</span>
        <template v-if="versionInfo.status === 'update-available' && versionInfo.latestVersion">
          <span class="version-arrow mx-1">→</span>
          <span class="version-latest-text">{{ versionInfo.latestVersion }}</span>
        </template>
      </div>

      <!-- 暗色模式切换 -->
      <v-btn icon variant="text" size="small" class="header-btn" @click="toggleDarkMode">
        <v-icon size="20">{{
          theme.global.current.value.dark ? 'mdi-weather-night' : 'mdi-white-balance-sunny'
        }}</v-icon>
      </v-btn>

      <!-- 注销按钮 -->
      <v-btn
        icon
        variant="text"
        size="small"
        class="header-btn"
        @click="handleLogout"
        v-if="isAuthenticated"
        title="注销"
      >
        <v-icon size="20">mdi-logout</v-icon>
      </v-btn>
    </v-app-bar>

    <!-- 主要内容 -->
    <v-main>
      <v-container fluid class="pa-4 pa-md-6">
        <!-- 统计卡片 - 玻璃拟态风格 -->
        <v-row class="mb-6 stat-cards-row">
          <v-col cols="6" sm="4">
            <div class="stat-card stat-card-info">
              <div class="stat-card-icon">
                <v-icon size="28">mdi-server-network</v-icon>
              </div>
              <div class="stat-card-content">
                <div class="stat-card-value">{{ currentChannelsData.channels?.length || 0 }}</div>
                <div class="stat-card-label">总渠道数</div>
                <div class="stat-card-desc">已配置的API渠道</div>
              </div>
              <div class="stat-card-glow"></div>
            </div>
          </v-col>

          <v-col cols="6" sm="4">
            <div class="stat-card stat-card-success">
              <div class="stat-card-icon">
                <v-icon size="28">mdi-check-circle</v-icon>
              </div>
              <div class="stat-card-content">
                <div class="stat-card-value">
                  {{ activeChannelCount }}<span class="stat-card-total">/{{ failoverChannelCount }}</span>
                </div>
                <div class="stat-card-label">活跃渠道</div>
                <div class="stat-card-desc">参与故障转移调度</div>
              </div>
              <div class="stat-card-glow"></div>
            </div>
          </v-col>

          <v-col cols="6" sm="4">
            <div class="stat-card stat-card-emerald">
              <div class="stat-card-icon pulse-animation">
                <v-icon size="28">mdi-heart-pulse</v-icon>
              </div>
              <div class="stat-card-content">
                <div class="stat-card-value">运行中</div>
                <div class="stat-card-label">系统状态</div>
                <div class="stat-card-desc">服务正常运行</div>
              </div>
              <div class="stat-card-glow"></div>
            </div>
          </v-col>
        </v-row>

        <!-- 操作按钮区域 - 现代化设计 -->
        <div class="action-bar mb-6">
          <div class="action-bar-left">
            <v-btn
              color="primary"
              size="large"
              @click="openAddChannelModal"
              prepend-icon="mdi-plus"
              class="action-btn action-btn-primary"
            >
              添加渠道
            </v-btn>

            <v-btn
              color="info"
              size="large"
              @click="pingAllChannels"
              prepend-icon="mdi-speedometer"
              variant="tonal"
              :loading="isPingingAll"
              class="action-btn"
            >
              测试延迟
            </v-btn>

            <v-btn size="large" @click="refreshChannels" prepend-icon="mdi-refresh" variant="text" class="action-btn">
              刷新
            </v-btn>
          </div>

          <div class="action-bar-right">
            <!-- Fuzzy 模式切换按钮 -->
            <v-tooltip location="bottom">
              <template v-slot:activator="{ props }">
                <v-btn
                  v-bind="props"
                  variant="tonal"
                  size="large"
                  @click="toggleFuzzyMode"
                  :loading="fuzzyModeLoading"
                  :disabled="fuzzyModeLoadError"
                  :color="fuzzyModeLoadError ? 'error' : (fuzzyModeEnabled ? 'warning' : 'default')"
                  class="action-btn"
                >
                  <v-icon start size="20">
                    {{ fuzzyModeLoadError ? 'mdi-alert-circle-outline' : (fuzzyModeEnabled ? 'mdi-shield-refresh' : 'mdi-shield-off-outline') }}
                  </v-icon>
                  Fuzzy
                </v-btn>
              </template>
              <span>{{ fuzzyModeLoadError ? '加载失败，请刷新页面' : (fuzzyModeEnabled ? 'Fuzzy 模式已启用：模糊处理错误，自动尝试所有渠道' : 'Fuzzy 模式已关闭：精确处理错误，透传上游响应') }}</span>
            </v-tooltip>

            <!-- 负载均衡选择 -->
            <v-menu>
              <template v-slot:activator="{ props }">
                <v-btn
                  v-bind="props"
                  variant="tonal"
                  size="large"
                  append-icon="mdi-chevron-down"
                  class="action-btn load-balance-btn"
                >
                  <v-icon start size="20">mdi-tune</v-icon>
                  {{ currentChannelsData.loadBalance }}
                </v-btn>
              </template>
              <v-list class="load-balance-menu" rounded="lg" elevation="8">
                <v-list-subheader>API密钥分配策略</v-list-subheader>
                <v-list-item
                  @click="updateLoadBalance('round-robin')"
                  :active="currentChannelsData.loadBalance === 'round-robin'"
                  rounded="lg"
                >
                  <template v-slot:prepend>
                    <v-avatar color="info" size="36" variant="tonal">
                      <v-icon size="20">mdi-rotate-right</v-icon>
                    </v-avatar>
                  </template>
                  <v-list-item-title class="font-weight-medium">轮询 (Round Robin)</v-list-item-title>
                  <v-list-item-subtitle>按顺序依次使用API密钥</v-list-item-subtitle>
                </v-list-item>
                <v-list-item
                  @click="updateLoadBalance('random')"
                  :active="currentChannelsData.loadBalance === 'random'"
                  rounded="lg"
                >
                  <template v-slot:prepend>
                    <v-avatar color="secondary" size="36" variant="tonal">
                      <v-icon size="20">mdi-dice-6</v-icon>
                    </v-avatar>
                  </template>
                  <v-list-item-title class="font-weight-medium">随机 (Random)</v-list-item-title>
                  <v-list-item-subtitle>随机选择API密钥</v-list-item-subtitle>
                </v-list-item>
                <v-list-item
                  @click="updateLoadBalance('failover')"
                  :active="currentChannelsData.loadBalance === 'failover'"
                  rounded="lg"
                >
                  <template v-slot:prepend>
                    <v-avatar color="warning" size="36" variant="tonal">
                      <v-icon size="20">mdi-backup-restore</v-icon>
                    </v-avatar>
                  </template>
                  <v-list-item-title class="font-weight-medium">故障转移 (Failover)</v-list-item-title>
                  <v-list-item-subtitle>优先第一个，失败时切换</v-list-item-subtitle>
                </v-list-item>
              </v-list>
            </v-menu>
          </div>
        </div>

        <!-- 渠道编排（高密度列表模式） -->
        <ChannelOrchestration
          v-if="currentChannelsData.channels?.length"
          ref="channelOrchestrationRef"
          :channels="currentChannelsData.channels"
          :current-channel-index="currentChannelsData.current ?? 0"
          :channel-type="activeTab"
          :dashboard-metrics="dashboardMetrics"
          :dashboard-stats="dashboardStats"
          @edit="editChannel"
          @delete="deleteChannel"
          @ping="pingChannel"
          @refresh="refreshChannels"
          @error="showErrorToast"
          @success="showSuccessToast"
          class="mb-6"
        />

        <!-- 空状态 -->
        <v-card v-if="!currentChannelsData.channels?.length" elevation="2" class="text-center pa-12" rounded="lg">
          <v-avatar size="120" color="primary" class="mb-6">
            <v-icon size="60" color="white">mdi-rocket-launch</v-icon>
          </v-avatar>
          <div class="text-h4 mb-4 font-weight-bold">暂无渠道配置</div>
          <div class="text-subtitle-1 text-medium-emphasis mb-8">
            还没有配置任何API渠道，请添加第一个渠道来开始使用代理服务
          </div>
          <v-btn color="primary" size="x-large" @click="openAddChannelModal" prepend-icon="mdi-plus" variant="elevated">
            添加第一个渠道
          </v-btn>
        </v-card>
      </v-container>
    </v-main>

    <!-- 添加渠道模态框 -->
    <AddChannelModal
      v-model:show="showAddChannelModal"
      :channel="editingChannel"
      :channel-type="activeTab"
      @save="saveChannel"
    />

    <!-- 添加API密钥对话框 -->
    <v-dialog v-model="showAddKeyModalRef" max-width="500">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center">
          <v-icon class="mr-3">mdi-key-plus</v-icon>
          添加API密钥
        </v-card-title>
        <v-card-text>
          <v-text-field
            v-model="newApiKey"
            label="API密钥"
            type="password"
            variant="outlined"
            density="comfortable"
            @keyup.enter="addApiKey"
            placeholder="输入API密钥"
          ></v-text-field>
        </v-card-text>
        <v-card-actions>
          <v-spacer></v-spacer>
          <v-btn @click="showAddKeyModalRef = false" variant="text">取消</v-btn>
          <v-btn @click="addApiKey" :disabled="!newApiKey.trim()" color="primary" variant="elevated">添加</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Toast通知 -->
    <v-snackbar
      v-for="toast in toasts"
      :key="toast.id"
      v-model="toast.show"
      :color="getToastColor(toast.type)"
      :timeout="3000"
      location="top right"
      variant="elevated"
    >
      <div class="d-flex align-center">
        <v-icon class="mr-3">{{ getToastIcon(toast.type) }}</v-icon>
        {{ toast.message }}
      </div>
    </v-snackbar>
  </v-app>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { useTheme } from 'vuetify'
import { api, fetchHealth, type Channel, type ChannelsResponse, type ChannelMetrics, type ChannelDashboardResponse } from './services/api'
import { versionService, type VersionInfo } from './services/version'
import AddChannelModal from './components/AddChannelModal.vue'
import ChannelOrchestration from './components/ChannelOrchestration.vue'
import { useAppTheme } from './composables/useTheme'

// Vuetify主题
const theme = useTheme()

// 应用主题系统
const { init: initTheme } = useAppTheme()

// 渠道编排组件引用
const channelOrchestrationRef = ref<InstanceType<typeof ChannelOrchestration> | null>(null)

// 自动刷新定时器
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null
const AUTO_REFRESH_INTERVAL = 2000 // 2秒

// 响应式数据
const activeTab = ref<'messages' | 'responses'>('messages') // Tab 切换状态
const channelsData = ref<ChannelsResponse>({ channels: [], current: -1, loadBalance: 'round-robin' })
const responsesChannelsData = ref<ChannelsResponse>({ channels: [], current: -1, loadBalance: 'round-robin' }) // Responses渠道数据
// Dashboard 数据（合并的 metrics 和 stats）
const dashboardMetrics = ref<ChannelMetrics[]>([])
const dashboardStats = ref<ChannelDashboardResponse['stats'] | undefined>(undefined)
const showAddChannelModal = ref(false)
const showAddKeyModalRef = ref(false)
const editingChannel = ref<Channel | null>(null)
const selectedChannelForKey = ref<number>(-1)
const newApiKey = ref('')
const isPingingAll = ref(false)
const darkModePreference = ref<'light' | 'dark' | 'auto'>('auto')

// Fuzzy 模式状态
const fuzzyModeEnabled = ref(true)
const fuzzyModeLoading = ref(false)

// 版本信息
const versionInfo = ref<VersionInfo>({
  currentVersion: '',
  latestVersion: null,
  isLatest: false,
  hasUpdate: false,
  releaseUrl: null,
  lastCheckTime: 0,
  status: 'checking'
})
const isCheckingVersion = ref(false)

// Toast通知系统
interface Toast {
  id: number
  message: string
  type: 'success' | 'error' | 'warning' | 'info'
  show?: boolean
}
const toasts = ref<Toast[]>([])
let toastId = 0

// 计算属性 - 根据当前Tab动态返回数据
const currentChannelsData = computed(() => {
  return activeTab.value === 'messages' ? channelsData.value : responsesChannelsData.value
})

// 计算属性：活跃渠道数（仅 active 状态）
const activeChannelCount = computed(() => {
  const data = currentChannelsData.value
  if (!data.channels) return 0
  return data.channels.filter(ch => ch.status === 'active').length
})

// 计算属性：参与故障转移的渠道数（active + suspended）
const failoverChannelCount = computed(() => {
  const data = currentChannelsData.value
  if (!data.channels) return 0
  return data.channels.filter(ch => ch.status !== 'disabled').length
})

// Toast工具函数
const getToastColor = (type: string) => {
  const colorMap: Record<string, string> = {
    success: 'success',
    error: 'error',
    warning: 'warning',
    info: 'info'
  }
  return colorMap[type] || 'info'
}

const getToastIcon = (type: string) => {
  const iconMap: Record<string, string> = {
    success: 'mdi-check-circle',
    error: 'mdi-alert-circle',
    warning: 'mdi-alert',
    info: 'mdi-information'
  }
  return iconMap[type] || 'mdi-information'
}

// 工具函数
const showToast = (message: string, type: 'success' | 'error' | 'warning' | 'info' = 'info') => {
  const toast: Toast = { id: ++toastId, message, type, show: true }
  toasts.value.push(toast)
  setTimeout(() => {
    const index = toasts.value.findIndex(t => t.id === toast.id)
    if (index > -1) toasts.value.splice(index, 1)
  }, 3000)
}

const handleError = (error: unknown, defaultMessage: string) => {
  const message = error instanceof Error ? error.message : defaultMessage
  showToast(message, 'error')
  console.error(error)
}

// 直接显示错误消息（供子组件事件使用）
const showErrorToast = (message: string) => {
  showToast(message, 'error')
}

// 直接显示成功消息（供子组件事件使用）
const showSuccessToast = (message: string) => {
  showToast(message, 'info')
}

// 主要功能函数
const refreshChannels = async () => {
  try {
    // 使用合并的 dashboard 接口
    const dashboard = await api.getChannelDashboard(activeTab.value)

    if (activeTab.value === 'messages') {
      channelsData.value = {
        channels: dashboard.channels,
        current: channelsData.value.current, // 保留当前选中状态
        loadBalance: dashboard.loadBalance
      }
    } else {
      responsesChannelsData.value = {
        channels: dashboard.channels,
        current: responsesChannelsData.value.current, // 保留当前选中状态
        loadBalance: dashboard.loadBalance
      }
    }

    // 同时更新 metrics 和 stats
    dashboardMetrics.value = dashboard.metrics
    dashboardStats.value = dashboard.stats
  } catch (error) {
    handleAuthError(error)
  }
}

const saveChannel = async (channel: Omit<Channel, 'index' | 'latency' | 'status'>, options?: { isQuickAdd?: boolean }) => {
  try {
    const isResponses = activeTab.value === 'responses'
    if (editingChannel.value) {
      if (isResponses) {
        await api.updateResponsesChannel(editingChannel.value.index, channel)
      } else {
        await api.updateChannel(editingChannel.value.index, channel)
      }
      showToast('渠道更新成功', 'success')
    } else {
      if (isResponses) {
        await api.addResponsesChannel(channel)
      } else {
        await api.addChannel(channel)
      }
      showToast('渠道添加成功', 'success')

      // 快速添加模式：将新渠道设为第一优先级并设置5分钟促销期
      if (options?.isQuickAdd) {
        await refreshChannels() // 先刷新获取新渠道的 index
        const data = isResponses ? responsesChannelsData.value : channelsData.value

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

            if (isResponses) {
              await api.reorderResponsesChannels(newOrder)
            } else {
              await api.reorderChannels(newOrder)
            }

            // 2. 设置5分钟促销期（300秒）
            if (isResponses) {
              await api.setResponsesChannelPromotion(newChannel.index, 300)
            } else {
              await api.setChannelPromotion(newChannel.index, 300)
            }

            showToast(`渠道 ${channel.name} 已设为最高优先级，5分钟内优先使用`, 'info')
          } catch (err) {
            console.warn('设置快速添加优先级失败:', err)
            // 不影响主流程，只是提示
          }
        }
      }
    }
    showAddChannelModal.value = false
    editingChannel.value = null
    await refreshChannels()
  } catch (error) {
    handleAuthError(error)
  }
}

const editChannel = (channel: Channel) => {
  editingChannel.value = channel
  showAddChannelModal.value = true
}

const deleteChannel = async (channelId: number) => {
  if (!confirm('确定要删除这个渠道吗？')) return

  try {
    if (activeTab.value === 'responses') {
      await api.deleteResponsesChannel(channelId)
    } else {
      await api.deleteChannel(channelId)
    }
    showToast('渠道删除成功', 'success')
    await refreshChannels()
  } catch (error) {
    handleAuthError(error)
  }
}

const openAddChannelModal = () => {
  editingChannel.value = null
  showAddChannelModal.value = true
}

const openAddKeyModal = (channelId: number) => {
  selectedChannelForKey.value = channelId
  newApiKey.value = ''
  showAddKeyModalRef.value = true
}

const addApiKey = async () => {
  if (!newApiKey.value.trim()) return

  try {
    if (activeTab.value === 'responses') {
      await api.addResponsesApiKey(selectedChannelForKey.value, newApiKey.value.trim())
    } else {
      await api.addApiKey(selectedChannelForKey.value, newApiKey.value.trim())
    }
    showToast('API密钥添加成功', 'success')
    showAddKeyModalRef.value = false
    newApiKey.value = ''
    await refreshChannels()
  } catch (error) {
    showToast(`添加API密钥失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

const removeApiKey = async (channelId: number, apiKey: string) => {
  if (!confirm('确定要删除这个API密钥吗？')) return

  try {
    if (activeTab.value === 'responses') {
      await api.removeResponsesApiKey(channelId, apiKey)
    } else {
      await api.removeApiKey(channelId, apiKey)
    }
    showToast('API密钥删除成功', 'success')
    await refreshChannels()
  } catch (error) {
    showToast(`删除API密钥失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

const pingChannel = async (channelId: number) => {
  try {
    const result = await api.pingChannel(channelId)
    const data = activeTab.value === 'messages' ? channelsData.value : responsesChannelsData.value
    const channel = data.channels?.find(c => c.index === channelId)
    if (channel) {
      channel.latency = result.latency
      channel.status = result.success ? 'healthy' : 'error'
    }
    showToast(`延迟测试完成: ${result.latency}ms`, result.success ? 'success' : 'warning')
  } catch (error) {
    showToast(`延迟测试失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

const pingAllChannels = async () => {
  if (isPingingAll.value) return

  isPingingAll.value = true
  try {
    const results = await api.pingAllChannels()
    const data = activeTab.value === 'messages' ? channelsData.value : responsesChannelsData.value
    results.forEach(result => {
      const channel = data.channels?.find(c => c.index === result.id)
      if (channel) {
        channel.latency = result.latency
        channel.status = result.status as 'healthy' | 'error'
      }
    })
    showToast('全部渠道延迟测试完成', 'success')
  } catch (error) {
    showToast(`批量延迟测试失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  } finally {
    isPingingAll.value = false
  }
}

const updateLoadBalance = async (strategy: string) => {
  try {
    if (activeTab.value === 'messages') {
      await api.updateLoadBalance(strategy)
      channelsData.value.loadBalance = strategy
    } else {
      await api.updateResponsesLoadBalance(strategy)
      responsesChannelsData.value.loadBalance = strategy
    }
    showToast(`负载均衡策略已更新为: ${strategy}`, 'success')
  } catch (error) {
    showToast(`更新负载均衡策略失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

// Fuzzy 模式管理
const fuzzyModeLoadError = ref(false) // 加载失败标记

const loadFuzzyModeStatus = async () => {
  fuzzyModeLoadError.value = false
  try {
    const { fuzzyModeEnabled: enabled } = await api.getFuzzyMode()
    fuzzyModeEnabled.value = enabled
  } catch (e) {
    console.error('Failed to load fuzzy mode status:', e)
    fuzzyModeLoadError.value = true
    // 加载失败时不使用默认值，保持 UI 显示未知状态
    showToast('加载 Fuzzy 模式状态失败，请刷新页面重试', 'warning')
  }
}

const toggleFuzzyMode = async () => {
  if (fuzzyModeLoadError.value) {
    showToast('Fuzzy 模式状态未知，请先刷新页面', 'warning')
    return
  }
  fuzzyModeLoading.value = true
  try {
    await api.setFuzzyMode(!fuzzyModeEnabled.value)
    fuzzyModeEnabled.value = !fuzzyModeEnabled.value
    showToast(`Fuzzy 模式已${fuzzyModeEnabled.value ? '启用' : '关闭'}`, 'success')
  } catch (e) {
    showToast(`切换 Fuzzy 模式失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
  } finally {
    fuzzyModeLoading.value = false
  }
}

// 主题管理
const toggleDarkMode = () => {
  const newMode = darkModePreference.value === 'dark' ? 'light' : 'dark'
  setDarkMode(newMode)
}

const setDarkMode = (themeName: 'light' | 'dark' | 'auto') => {
  darkModePreference.value = themeName
  const apply = (isDark: boolean) => {
    theme.global.name.value = isDark ? 'dark' : 'light'
  }

  if (themeName === 'auto') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    apply(prefersDark)
  } else {
    apply(themeName === 'dark')
  }

  localStorage.setItem('theme', themeName)
}

// 认证状态管理
const isAuthenticated = ref(false)
const authError = ref('')
const authKeyInput = ref('')
const authLoading = ref(false)
const isAutoAuthenticating = ref(true) // 初始化为true，防止登录框闪现
const isInitialized = ref(false) // 添加初始化完成标志

// 认证尝试限制
const authAttempts = ref(0)
const MAX_AUTH_ATTEMPTS = 5
const authLockoutTime = ref<Date | null>(null)

// 控制认证对话框显示
const showAuthDialog = computed({
  get: () => {
    // 只有在初始化完成后，且未认证，且不在自动认证中时，才显示对话框
    return isInitialized.value && !isAuthenticated.value && !isAutoAuthenticating.value
  },
  set: () => {} // 防止外部修改，认证状态只能通过内部逻辑控制
})

// 初始化认证 - 只负责从存储获取密钥
const initializeAuth = () => {
  const key = api.initializeAuth()
  return key
}

// 自动验证保存的密钥
const autoAuthenticate = async () => {
  const savedKey = initializeAuth()
  if (!savedKey) {
    // 没有保存的密钥，显示登录对话框
    authError.value = '请输入访问密钥以继续'
    isAutoAuthenticating.value = false
    isInitialized.value = true
    return false
  }

  // 有保存的密钥，尝试自动认证
  try {
    // 尝试调用API验证密钥是否有效
    await api.getChannels()

    // 密钥有效，设置认证状态
    isAuthenticated.value = true
    authError.value = ''

    return true
  } catch (error: any) {
    // 密钥无效或过期
    console.warn('自动认证失败:', error.message)

    // 清除无效的密钥
    api.clearAuth()

    // 显示登录对话框，提示用户重新输入
    isAuthenticated.value = false
    authError.value = '保存的访问密钥已失效，请重新输入'

    return false
  } finally {
    isAutoAuthenticating.value = false
    isInitialized.value = true
  }
}

// 手动设置密钥（用于重新认证）
const setAuthKey = (key: string) => {
  api.setApiKey(key)
  localStorage.setItem('proxyAccessKey', key)
  isAuthenticated.value = true
  authError.value = ''
  // 重新加载数据
  refreshChannels()
}

// 处理认证提交
const handleAuthSubmit = async () => {
  if (!authKeyInput.value.trim()) {
    authError.value = '请输入访问密钥'
    return
  }

  // 检查是否被锁定
  if (authLockoutTime.value && new Date() < authLockoutTime.value) {
    const remainingSeconds = Math.ceil((authLockoutTime.value.getTime() - Date.now()) / 1000)
    authError.value = `认证尝试次数过多，请在 ${remainingSeconds} 秒后重试`
    return
  }

  authLoading.value = true
  authError.value = ''

  try {
    // 设置密钥
    setAuthKey(authKeyInput.value.trim())

    // 测试API调用以验证密钥
    await api.getChannels()

    // 认证成功，重置计数器
    authAttempts.value = 0
    authLockoutTime.value = null

    // 如果成功，加载数据
    await refreshChannels()

    authKeyInput.value = ''

    // 记录认证成功(前端日志)
    if (import.meta.env.DEV) {
      console.info('✅ 认证成功 - 时间:', new Date().toISOString())
    }
  } catch (error: any) {
    // 认证失败
    authAttempts.value++

    // 记录认证失败(前端日志)
    console.warn('🔒 认证失败 - 尝试次数:', authAttempts.value, '时间:', new Date().toISOString())

    // 如果尝试次数过多，锁定5分钟
    if (authAttempts.value >= MAX_AUTH_ATTEMPTS) {
      authLockoutTime.value = new Date(Date.now() + 5 * 60 * 1000)
      authError.value = '认证尝试次数过多，请在5分钟后重试'
    } else {
      authError.value = `访问密钥验证失败 (剩余尝试次数: ${MAX_AUTH_ATTEMPTS - authAttempts.value})`
    }

    isAuthenticated.value = false
    api.clearAuth()
  } finally {
    authLoading.value = false
  }
}

// 处理注销
const handleLogout = () => {
  api.clearAuth()
  isAuthenticated.value = false
  authError.value = '请输入访问密钥以继续'
  channelsData.value = { channels: [], current: 0, loadBalance: 'failover' }
  showToast('已安全注销', 'info')
}

// 处理认证失败
const handleAuthError = (error: any) => {
  if (error.message && error.message.includes('认证失败')) {
    isAuthenticated.value = false
    authError.value = '访问密钥无效或已过期，请重新输入'
  } else {
    showToast(`操作失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

// 版本检查
const checkVersion = async () => {
  if (isCheckingVersion.value) return

  isCheckingVersion.value = true
  try {
    // 先获取当前版本
    const health = await fetchHealth()
    const currentVersion = health.version?.version || ''

    if (currentVersion) {
      versionService.setCurrentVersion(currentVersion)
      versionInfo.value.currentVersion = currentVersion

      // 检查 GitHub 最新版本
      const result = await versionService.checkForUpdates()
      versionInfo.value = result
    } else {
      versionInfo.value.status = 'error'
    }
  } catch (error) {
    console.warn('Version check failed:', error)
    versionInfo.value.status = 'error'
  } finally {
    isCheckingVersion.value = false
  }
}

// 版本点击处理
const handleVersionClick = () => {
  if (
    (versionInfo.value.status === 'update-available' || versionInfo.value.status === 'latest') &&
    versionInfo.value.releaseUrl
  ) {
    window.open(versionInfo.value.releaseUrl, '_blank', 'noopener,noreferrer')
  }
}

// 初始化
onMounted(async () => {
  // 初始化复古像素主题
  document.documentElement.dataset.theme = 'retro'
  initTheme()

  // 加载保存的暗色模式偏好
  const savedMode = (localStorage.getItem('theme') as 'light' | 'dark' | 'auto') || 'auto'
  setDarkMode(savedMode)

  // 监听系统主题变化
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  const handlePref = () => {
    if (darkModePreference.value === 'auto') setDarkMode('auto')
  }
  mediaQuery.addEventListener('change', handlePref)

  // 版本检查（独立于认证，静默执行）
  checkVersion()

  // 检查是否有保存的密钥
  const savedKey = localStorage.getItem('proxyAccessKey')

  if (savedKey) {
    // 有保存的密钥，开始自动认证
    isAutoAuthenticating.value = true
    isInitialized.value = false
  } else {
    // 没有保存的密钥，直接显示登录对话框
    isAutoAuthenticating.value = false
    isInitialized.value = true
  }

  // 尝试自动认证
  const authenticated = await autoAuthenticate()

  if (authenticated) {
    // 加载渠道数据
    await refreshChannels()
    // 加载 Fuzzy 模式状态
    await loadFuzzyModeStatus()
    // 启动自动刷新
    startAutoRefresh()
  }
})

// 启动自动刷新定时器
const startAutoRefresh = () => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
  }
  autoRefreshTimer = setInterval(async () => {
    if (isAuthenticated.value) {
      try {
        // 使用合并的 dashboard 接口，减少网络请求
        const dashboard = await api.getChannelDashboard(activeTab.value)

        // 更新渠道数据，保留当前选中状态
        if (activeTab.value === 'messages') {
          channelsData.value = {
            channels: dashboard.channels,
            current: channelsData.value.current, // 保留当前选中状态
            loadBalance: dashboard.loadBalance
          }
        } else {
          responsesChannelsData.value = {
            channels: dashboard.channels,
            current: responsesChannelsData.value.current, // 保留当前选中状态
            loadBalance: dashboard.loadBalance
          }
        }

        // 更新 metrics 和 stats
        dashboardMetrics.value = dashboard.metrics
        dashboardStats.value = dashboard.stats
      } catch (error) {
        // 静默处理错误，避免刷新失败时干扰用户
        console.warn('自动刷新失败:', error)
      }
    }
  }, AUTO_REFRESH_INTERVAL)
}

// 停止自动刷新定时器
const stopAutoRefresh = () => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

// 监听 Tab 切换，刷新对应数据
watch(activeTab, async () => {
  if (isAuthenticated.value) {
    // 使用 dashboard 接口刷新所有数据
    try {
      const dashboard = await api.getChannelDashboard(activeTab.value)
      if (activeTab.value === 'messages') {
        channelsData.value = {
          channels: dashboard.channels,
          current: channelsData.value.current, // 保留当前选中状态
          loadBalance: dashboard.loadBalance
        }
      } else {
        responsesChannelsData.value = {
          channels: dashboard.channels,
          current: responsesChannelsData.value.current, // 保留当前选中状态
          loadBalance: dashboard.loadBalance
        }
      }
      dashboardMetrics.value = dashboard.metrics
      dashboardStats.value = dashboard.stats
    } catch (error) {
      console.error('切换 Tab 刷新失败:', error)
    }
  }
})

// 监听认证状态变化
watch(isAuthenticated, newValue => {
  if (newValue) {
    startAutoRefresh()
  } else {
    stopAutoRefresh()
  }
})

// 在组件卸载时清除定时器
onUnmounted(() => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
})
</script>

<style scoped>
/* =====================================================
   🎮 复古像素 (Retro Pixel) 主题样式系统
   Neo-Brutalism: 直角、粗黑边框、硬阴影、等宽字体
   ===================================================== */

/* ----- 应用栏 - 复古像素风格 ----- */
.app-header {
  background: rgb(var(--v-theme-surface)) !important;
  border-bottom: 2px solid rgb(var(--v-theme-on-surface));
  transition: none;
  padding: 0 16px !important;
}

.v-theme--dark .app-header {
  background: rgb(var(--v-theme-surface)) !important;
  border-bottom: 2px solid rgba(255, 255, 255, 0.8);
}

/* 修复 Header 布局 */
.app-header :deep(.v-toolbar__prepend) {
  margin-inline-end: 4px !important;
}

.app-header .v-toolbar-title {
  overflow: hidden !important;
  min-width: 0 !important;
  flex: 1 !important;
}

.app-header :deep(.v-toolbar__content) {
  overflow: visible !important;
}

.app-header :deep(.v-toolbar__content > .v-toolbar-title) {
  min-width: 0 !important;
  margin-inline-start: 0 !important;
  margin-inline-end: auto !important;
}

.app-header :deep(.v-toolbar-title__placeholder) {
  width: 100%;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.app-logo {
  width: 42px;
  height: 42px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgb(var(--v-theme-primary));
  border: 2px solid rgb(var(--v-theme-on-surface));
  box-shadow: 3px 3px 0 0 rgb(var(--v-theme-on-surface));
  margin-right: 8px;
}

.v-theme--dark .app-logo {
  border-color: rgba(255, 255, 255, 0.8);
  box-shadow: 3px 3px 0 0 rgba(255, 255, 255, 0.8);
}

/* 自定义标题容器 */
.header-title {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.api-type-text {
  cursor: pointer;
  opacity: 0.5;
  transition: all 0.1s ease;
  padding: 4px 8px;
  position: relative;
}

.api-type-text:not(.separator):hover {
  opacity: 0.8;
  background: rgba(var(--v-theme-primary), 0.15);
}

.api-type-text.active {
  opacity: 1;
  font-weight: 700;
  color: rgb(var(--v-theme-primary));
  background: rgba(var(--v-theme-primary), 0.1);
  border: 1px solid rgb(var(--v-theme-on-surface));
}

.v-theme--dark .api-type-text.active {
  border-color: rgba(255, 255, 255, 0.6);
}

.separator {
  opacity: 0.25;
  margin: 0 2px;
  cursor: default;
  padding: 0;
}

.brand-text {
  margin-left: 10px;
  color: rgb(var(--v-theme-primary));
  font-weight: 700;
}

.header-btn {
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 2px 2px 0 0 rgb(var(--v-theme-on-surface)) !important;
  margin-left: 4px;
  transition: all 0.1s ease !important;
}

.v-theme--dark .header-btn {
  border-color: rgba(255, 255, 255, 0.6) !important;
  box-shadow: 2px 2px 0 0 rgba(255, 255, 255, 0.6) !important;
}

.header-btn:hover {
  background: rgba(var(--v-theme-primary), 0.1);
  transform: translate(-1px, -1px);
  box-shadow: 3px 3px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.header-btn:active {
  transform: translate(2px, 2px) !important;
  box-shadow: none !important;
}

/* ----- 版本信息徽章 ----- */
.version-badge {
  display: flex;
  align-items: center;
  padding: 4px 10px;
  margin-right: 8px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12px;
  border: 2px solid rgb(var(--v-theme-on-surface));
  background: rgb(var(--v-theme-surface));
  transition: all 0.15s ease;
}

.version-badge.version-clickable {
  cursor: pointer;
}

.version-badge.version-clickable:hover {
  transform: translateY(-1px);
  box-shadow: 3px 3px 0 0 rgb(var(--v-theme-on-surface));
}

.version-badge.version-checking {
  opacity: 0.7;
}

.version-badge.version-latest {
  border-color: rgb(var(--v-theme-success));
}

.version-badge.version-update {
  border-color: rgb(var(--v-theme-warning));
  background: rgba(var(--v-theme-warning), 0.1);
}

.version-text {
  color: rgb(var(--v-theme-on-surface));
}

.version-arrow {
  color: rgb(var(--v-theme-warning));
  font-weight: bold;
}

.version-latest-text {
  color: rgb(var(--v-theme-warning));
  font-weight: bold;
}

.v-theme--dark .version-badge {
  border-color: rgba(255, 255, 255, 0.6);
}

.v-theme--dark .version-badge.version-latest {
  border-color: rgb(var(--v-theme-success));
}

.v-theme--dark .version-badge.version-update {
  border-color: rgb(var(--v-theme-warning));
}

/* ----- 统计卡片 - 复古像素风格 ----- */
.stat-cards-row {
  margin-top: -8px;
}

.stat-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  margin: 2px;
  background: rgb(var(--v-theme-surface));
  border: 2px solid rgb(var(--v-theme-on-surface));
  box-shadow: 6px 6px 0 0 rgb(var(--v-theme-on-surface));
  transition: all 0.1s ease;
  overflow: hidden;
  min-height: 100px;
}
.stat-card:hover {
  transform: translate(-2px, -2px);
  box-shadow: 8px 8px 0 0 rgb(var(--v-theme-on-surface));
  border: 2px solid rgb(var(--v-theme-on-surface));
}

.stat-card:active {
  transform: translate(2px, 2px);
  box-shadow: 2px 2px 0 0 rgb(var(--v-theme-on-surface));
}

.v-theme--dark .stat-card {
  background: rgb(var(--v-theme-surface));
  border-color: rgba(255, 255, 255, 0.8);
  box-shadow: 6px 6px 0 0 rgba(255, 255, 255, 0.8);
}
.v-theme--dark .stat-card:hover {
  box-shadow: 8px 8px 0 0 rgba(255, 255, 255, 0.8);
  border-color: rgba(255, 255, 255, 0.8);
}

.v-theme--dark .stat-card:active {
  box-shadow: 2px 2px 0 0 rgba(255, 255, 255, 0.8);
}

.stat-card-icon {
  width: 56px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 2px solid rgb(var(--v-theme-on-surface));
  background: rgba(var(--v-theme-primary), 0.15);
  transition: transform 0.1s ease;
}

.v-theme--dark .stat-card-icon {
  border-color: rgba(255, 255, 255, 0.6);
}

.stat-card:hover .stat-card-icon {
  transform: scale(1.05);
}

.stat-card-content {
  flex: 1;
  min-width: 0;
}

.stat-card-value {
  font-size: 1.75rem;
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: -0.5px;
}

.stat-card-total {
  font-size: 1rem;
  font-weight: 500;
  opacity: 0.6;
}

.stat-card-label {
  font-size: 0.875rem;
  font-weight: 600;
  margin-top: 2px;
  opacity: 0.85;
  text-transform: uppercase;
}

.stat-card-desc {
  font-size: 0.75rem;
  opacity: 0.6;
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 隐藏光晕效果 */
.stat-card-glow {
  display: none;
}

/* 统计卡片颜色变体 */
.stat-card-info .stat-card-icon {
  background: #3b82f6;
  color: white;
}
.stat-card-info .stat-card-value {
  color: #3b82f6;
}
.v-theme--dark .stat-card-info .stat-card-value {
  color: #60a5fa;
}

.stat-card-success .stat-card-icon {
  background: #10b981;
  color: white;
}
.stat-card-success .stat-card-value {
  color: #10b981;
}
.v-theme--dark .stat-card-success .stat-card-value {
  color: #34d399;
}

.stat-card-primary .stat-card-icon {
  background: #6366f1;
  color: white;
}
.stat-card-primary .stat-card-value {
  color: #6366f1;
}
.v-theme--dark .stat-card-primary .stat-card-value {
  color: #818cf8;
}

.stat-card-emerald .stat-card-icon {
  background: #059669;
  color: white;
}
.stat-card-emerald .stat-card-value {
  color: #059669;
}
.v-theme--dark .stat-card-emerald .stat-card-value {
  color: #34d399;
}

/* =========================================
   复古像素主题 - 全局样式覆盖
   ========================================= */

/* 全局背景 */
.v-application {
  background-color: #fffbeb !important;
  font-family: 'Courier New', Consolas, monospace !important;
}

.v-theme--dark .v-application,
.v-theme--dark.v-application {
  background-color: rgb(var(--v-theme-background)) !important;
}

.v-main {
  background-color: #fffbeb !important;
}

.v-theme--dark .v-main {
  background-color: rgb(var(--v-theme-background)) !important;
}

/* 统计卡片图标配色 */
.stat-card-icon .v-icon {
  color: white !important;
}

.stat-card-emerald .stat-card-icon .v-icon {
  color: white !important;
}

/* 主按钮 - 复古像素风格 */
.action-btn-primary {
  background: rgb(var(--v-theme-primary)) !important;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
  color: white !important;
}

.action-btn-primary:hover {
  transform: translate(-1px, -1px);
  box-shadow: 5px 5px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.action-btn-primary:active {
  transform: translate(2px, 2px) !important;
  box-shadow: none !important;
}

.v-theme--dark .action-btn-primary {
  border-color: rgba(255, 255, 255, 0.8) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.8) !important;
}

/* 渠道编排容器 */
.channel-orchestration {
  background: transparent !important;
  box-shadow: none !important;
  border: none !important;
}

/* 渠道列表卡片样式 */
.channel-list .channel-row {
  background: rgb(var(--v-theme-surface)) !important;
  margin-bottom: 0;
  padding: 14px 12px 14px 28px !important;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
  min-height: 48px !important;
  position: relative;
}

.v-theme--dark .channel-list .channel-row {
  border-color: rgba(255, 255, 255, 0.7) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
}

.channel-list .channel-row:active {
  transform: translate(2px, 2px);
  box-shadow: none !important;
  transition: transform 0.1s;
}

/* 序号角标 */
.channel-row .priority-number {
  position: absolute !important;
  top: -1px !important;
  left: -1px !important;
  background: rgb(var(--v-theme-surface)) !important;
  color: rgb(var(--v-theme-on-surface)) !important;
  font-size: 10px !important;
  font-weight: 700 !important;
  padding: 2px 8px !important;
  border: 1px solid rgb(var(--v-theme-on-surface)) !important;
  border-top: none !important;
  border-left: none !important;
  width: auto !important;
  height: auto !important;
  margin: 0 !important;
  box-shadow: none !important;
  text-transform: uppercase;
}

.v-theme--dark .channel-row .priority-number {
  border-color: rgba(255, 255, 255, 0.5) !important;
}

/* 拖拽手柄 */
.drag-handle {
  opacity: 0.3;
  padding: 8px;
  margin-left: -8px;
}

/* 渠道名称 */
.channel-name {
  font-size: 14px !important;
  font-weight: 700 !important;
  color: rgb(var(--v-theme-on-surface));
}

.channel-name .text-caption.text-medium-emphasis {
  background: rgb(var(--v-theme-surface-variant));
  padding: 2px 6px;
  font-size: 10px !important;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface)) !important;
  border: 1px solid rgb(var(--v-theme-on-surface));
  text-transform: uppercase;
}

.v-theme--dark .channel-name .text-caption.text-medium-emphasis {
  border-color: rgba(255, 255, 255, 0.5);
}

/* 隐藏描述文字 */
.channel-name .text-disabled {
  display: none !important;
}

/* 隐藏指标和密钥数 */
.channel-metrics,
.channel-keys {
  display: none !important;
}

/* --- 备用资源池 --- */
.inactive-pool {
  background: rgb(var(--v-theme-surface)) !important;
  border: 2px dashed rgb(var(--v-theme-on-surface)) !important;
  padding: 8px !important;
  margin-top: 12px;
}

.v-theme--dark .inactive-pool {
  border-color: rgba(255, 255, 255, 0.5) !important;
}

.inactive-channel-row {
  background: rgb(var(--v-theme-surface)) !important;
  margin: 6px !important;
  padding: 12px !important;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 3px 3px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.v-theme--dark .inactive-channel-row {
  border-color: rgba(255, 255, 255, 0.6) !important;
  box-shadow: 3px 3px 0 0 rgba(255, 255, 255, 0.6) !important;
}

.inactive-channel-row .channel-info-main {
  color: rgb(var(--v-theme-on-surface)) !important;
  font-weight: 600;
}

/* ----- 操作按钮区域 ----- */
.action-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 20px;
  background: rgb(var(--v-theme-surface));
  border: 2px solid rgb(var(--v-theme-on-surface));
  box-shadow: 6px 6px 0 0 rgb(var(--v-theme-on-surface));
}

.v-theme--dark .action-bar {
  background: rgb(var(--v-theme-surface));
  border-color: rgba(255, 255, 255, 0.8);
  box-shadow: 6px 6px 0 0 rgba(255, 255, 255, 0.8);
}

.action-bar-left {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.action-bar-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.action-btn {
  font-weight: 600;
  letter-spacing: 0.3px;
  text-transform: uppercase;
  transition: all 0.1s ease;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.v-theme--dark .action-btn {
  border-color: rgba(255, 255, 255, 0.7) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
}

.action-btn:hover {
  transform: translate(-1px, -1px);
  box-shadow: 5px 5px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.action-btn:active {
  transform: translate(2px, 2px) !important;
  box-shadow: none !important;
}

.load-balance-btn {
  text-transform: uppercase;
}

.load-balance-menu {
  min-width: 300px;
  padding: 8px;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.v-theme--dark .load-balance-menu {
  border-color: rgba(255, 255, 255, 0.7) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
}

.load-balance-menu .v-list-item {
  margin-bottom: 4px;
  padding: 12px 16px;
}

.load-balance-menu .v-list-item:last-child {
  margin-bottom: 0;
}

/* =========================================
   手机端专属样式 (≤600px)
   ========================================= */
@media (max-width: 600px) {
  /* --- 主容器内边距缩小 --- */
  .v-main .v-container {
    padding-left: 8px !important;
    padding-right: 8px !important;
  }

  /* --- 顶部导航栏 --- */
  .app-header {
    padding: 0 12px !important;
    background: rgb(var(--v-theme-surface)) !important;
    border-bottom: 2px solid rgb(var(--v-theme-on-surface)) !important;
    box-shadow: none !important;
  }

  .v-theme--dark .app-header {
    border-bottom-color: rgba(255, 255, 255, 0.7) !important;
  }

  .app-logo {
    width: 32px;
    height: 32px;
    margin-right: 8px;
    box-shadow: 2px 2px 0 0 rgb(var(--v-theme-on-surface));
  }

  .v-theme--dark .app-logo {
    box-shadow: 2px 2px 0 0 rgba(255, 255, 255, 0.7);
  }

  .api-type-text {
    padding: 2px 6px;
  }

  .api-type-text.active {
    color: rgb(var(--v-theme-primary)) !important;
    font-weight: 800 !important;
  }

  .brand-text {
    display: none;
  }

  /* --- 统计卡片优化 --- */
  .stat-card {
    padding: 14px 12px;
    gap: 10px;
    min-height: auto;
    background: rgb(var(--v-theme-surface)) !important;
    box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
    border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  }

  .v-theme--dark .stat-card {
    box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
    border-color: rgba(255, 255, 255, 0.7) !important;
  }

  .stat-card-icon {
    width: 36px;
    height: 36px;
  }

  .stat-card-icon .v-icon {
    font-size: 18px !important;
  }

  .stat-card-value {
    font-size: 1.35rem;
    font-weight: 800 !important;
    line-height: 1.2;
    color: rgb(var(--v-theme-on-surface));
    letter-spacing: -0.5px;
  }

  .stat-card-label {
    font-size: 0.7rem;
    color: rgba(var(--v-theme-on-surface), 0.6);
    font-weight: 500;
    text-transform: uppercase;
  }

  .stat-card-desc {
    display: none;
  }

  .stat-cards-row {
    margin-bottom: 12px !important;
    margin-left: -4px !important;
    margin-right: -4px !important;
  }

  .stat-cards-row .v-col {
    padding: 4px !important;
  }

  /* --- 操作按钮区域 --- */
  .action-bar {
    flex-direction: column;
    gap: 10px;
    padding: 12px !important;
    box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
  }

  .v-theme--dark .action-bar {
    box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
  }

  .action-bar-left {
    width: 100%;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
  }

  .action-bar-left .action-btn {
    width: 100%;
    justify-content: center;
  }

  /* 刷新按钮独占一行 */
  .action-bar-left .action-btn:nth-child(3) {
    grid-column: 1 / -1;
  }

  .action-bar-right {
    width: 100%;
  }

  .action-bar-right .load-balance-btn {
    width: 100%;
    justify-content: center;
  }

  /* --- 渠道编排容器 --- */
  .channel-orchestration .v-card-title {
    display: none !important;
  }

  .channel-orchestration > .v-divider {
    display: none !important;
  }

  /* 隐藏"故障转移序列"标题区域 */
  .channel-orchestration .px-4.pt-3.pb-2 > .d-flex.mb-2 {
    display: none !important;
  }

  /* --- 渠道列表卡片化 --- */
  .channel-list .channel-row:active {
    transform: translate(2px, 2px);
    box-shadow: none !important;
    transition: transform 0.1s;
  }

  /* --- 通用优化 --- */
  .v-chip {
    font-weight: 600;
    border: 1px solid rgb(var(--v-theme-on-surface));
    text-transform: uppercase;
  }

  .v-theme--dark .v-chip {
    border-color: rgba(255, 255, 255, 0.5);
  }

  /* 隐藏分割线 */
  .channel-orchestration .v-divider {
    display: none !important;
  }
}

/* 心跳动画 - 简化为简单闪烁 */
.pulse-animation {
  animation: pixel-blink 1s step-end infinite;
}

@keyframes pixel-blink {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.7;
  }
}

/* ----- 响应式调整 ----- */
@media (min-width: 768px) {
  .app-header {
    padding: 0 24px !important;
  }
}

@media (min-width: 1024px) {
  .app-header {
    padding: 0 32px !important;
  }
}

/* ----- 渠道列表动画 ----- */
.d-contents {
  display: contents;
}

.channel-col {
  transition: all 0.2s ease;
  max-width: 640px;
}

.channel-list-enter-active,
.channel-list-leave-active {
  transition: all 0.2s ease;
}

.channel-list-enter-from {
  opacity: 0;
  transform: translateY(10px);
}

.channel-list-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

.channel-list-move {
  transition: transform 0.2s ease;
}
</style>

<!-- 全局样式 - 复古像素主题 -->
<style>
/* 复古像素主题 - 全局样式 */
.v-application {
  font-family: 'Courier New', Consolas, 'Liberation Mono', monospace !important;
}

/* 所有按钮复古像素风格 */
.v-btn:not(.v-btn--icon) {
  border-radius: 0 !important;
  text-transform: uppercase !important;
  font-weight: 600 !important;
}

/* 所有卡片复古像素风格 */
.v-card {
  border-radius: 0 !important;
}

/* 所有 Chip 复古像素风格 */
.v-chip {
  border-radius: 0 !important;
  font-weight: 600;
  text-transform: uppercase;
}

/* 输入框复古像素风格 */
.v-text-field .v-field {
  border-radius: 0 !important;
}

/* 对话框复古像素风格 */
.v-dialog .v-card {
  border: 2px solid currentColor !important;
  box-shadow: 6px 6px 0 0 currentColor !important;
}

/* 菜单复古像素风格 */
.v-menu > .v-overlay__content > .v-list {
  border-radius: 0 !important;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.v-theme--dark .v-menu > .v-overlay__content > .v-list {
  border-color: rgba(255, 255, 255, 0.7) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
}

/* Snackbar 复古像素风格 */
.v-snackbar__wrapper {
  border-radius: 0 !important;
  border: 2px solid currentColor !important;
  box-shadow: 4px 4px 0 0 currentColor !important;
}

/* 状态徽章复古像素风格 */
.status-badge .badge-content {
  border-radius: 0 !important;
  border: 1px solid rgb(var(--v-theme-on-surface));
}

.v-theme--dark .status-badge .badge-content {
  border-color: rgba(255, 255, 255, 0.6);
}
</style>
