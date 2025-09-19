<template>
  <v-app>
    <!-- 应用栏 -->
    <v-app-bar
      elevation="2"
      :color="currentTheme === 'dark' ? 'surface' : 'primary'"
      :height="$vuetify.display.mobile ? 72 : 88"
      class="app-header px-4"
    >
      <template #prepend>
        <v-icon 
          :class="$vuetify.display.mobile ? 'mr-3' : 'mr-4'" 
          :size="$vuetify.display.mobile ? 28 : 36"
        >
          mdi-rocket-launch
        </v-icon>
      </template>
      
      <v-app-bar-title class="d-flex flex-column justify-center">
        <div :class="$vuetify.display.mobile ? 'text-h6' : 'text-h5'" class="font-weight-bold mb-1">
          Claude API Proxy
        </div>
        <div class="text-body-2 opacity-90 d-none d-sm-block">
          智能API代理管理平台
        </div>
      </v-app-bar-title>

      <v-spacer></v-spacer>

      <!-- 主题切换 -->
      <v-btn
        icon
        variant="text"
        @click="toggleTheme"
      >
        <v-icon>{{ currentTheme === 'dark' ? 'mdi-weather-night' : 'mdi-white-balance-sunny' }}</v-icon>
      </v-btn>
    </v-app-bar>

    <!-- 主要内容 -->
    <v-main>
      <v-container fluid class="pa-6">
        <!-- 统计卡片 -->
        <v-row class="mb-6">
          <v-col cols="12" sm="6" md="3">
            <v-card elevation="3" class="h-100 stat-card" hover>
              <v-card-text class="pb-2">
                <div class="d-flex align-center justify-space-between">
                  <div>
                    <div class="text-h4 primary--text font-weight-bold">{{ channelsData.channels?.length || 0 }}</div>
                    <div class="text-subtitle-1 text-medium-emphasis">总渠道数</div>
                    <div class="text-caption text-medium-emphasis">已配置的API渠道</div>
                  </div>
                  <v-avatar size="60" color="primary">
                    <v-icon size="30" color="white">mdi-server-network</v-icon>
                  </v-avatar>
                </div>
              </v-card-text>
            </v-card>
          </v-col>

          <v-col cols="12" sm="6" md="3">
            <v-card elevation="3" class="h-100">
              <v-card-text class="pb-2">
                <div class="d-flex align-center justify-space-between">
                  <div>
                    <div class="text-h6 success--text font-weight-bold text-truncate" style="max-width: 120px;">{{ getCurrentChannelName() }}</div>
                    <div class="text-subtitle-1 text-medium-emphasis">当前渠道</div>
                    <div class="text-caption success--text font-weight-medium">{{ currentChannelType }}</div>
                  </div>
                  <v-avatar size="60" color="success">
                    <v-icon size="30" color="white">mdi-check-circle</v-icon>
                  </v-avatar>
                </div>
              </v-card-text>
            </v-card>
          </v-col>

          <v-col cols="12" sm="6" md="3">
            <v-card elevation="3" class="h-100">
              <v-card-text class="pb-2">
                <div class="d-flex align-center justify-space-between">
                  <div>
                    <div class="text-h6 info--text font-weight-bold text-capitalize">{{ channelsData.loadBalance || 'none' }}</div>
                    <div class="text-subtitle-1 text-medium-emphasis">负载均衡</div>
                    <div class="text-caption text-medium-emphasis">自动分配策略</div>
                  </div>
                  <v-avatar size="60" color="info">
                    <v-icon size="30" color="white">mdi-swap-horizontal</v-icon>
                  </v-avatar>
                </div>
              </v-card-text>
            </v-card>
          </v-col>

          <v-col cols="12" sm="6" md="3">
            <v-card elevation="3" class="h-100">
              <v-card-text class="pb-2">
                <div class="d-flex align-center justify-space-between">
                  <div>
                    <div class="text-h6 success--text font-weight-bold">运行中</div>
                    <div class="text-subtitle-1 text-medium-emphasis">系统状态</div>
                    <div class="text-caption text-medium-emphasis">服务正常运行</div>
                  </div>
                  <v-avatar size="60" color="success">
                    <v-icon size="30" color="white">mdi-heart-pulse</v-icon>
                  </v-avatar>
                </div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <!-- 操作按钮区域 -->
        <v-card elevation="2" class="mb-6" rounded="lg">
          <v-card-text>
            <div class="d-flex flex-column flex-sm-row gap-3 align-center justify-space-between">
              <div class="d-flex flex-wrap align-center ga-3">
                <v-btn
                  color="primary"
                  size="large"
                  @click="openAddChannelModal"
                  prepend-icon="mdi-plus"
                  variant="elevated"
                >
                  添加渠道
                </v-btn>
                
                <v-btn
                  color="success"
                  size="large"
                  @click="pingAllChannels"
                  prepend-icon="mdi-speedometer"
                  variant="elevated"
                  :loading="isPingingAll"
                >
                  测试全部延迟
                </v-btn>

                <v-btn
                  color="surface-variant"
                  size="large"
                  @click="refreshChannels"
                  prepend-icon="mdi-refresh"
                  variant="elevated"
                >
                  刷新
                </v-btn>
              </div>

              <!-- 负载均衡选择 -->
              <v-menu>
                <template v-slot:activator="{ props }">
                  <v-btn
                    v-bind="props"
                    color="secondary"
                    size="large"
                    append-icon="mdi-chevron-down"
                    variant="elevated"
                  >
                    负载均衡: {{ channelsData.loadBalance }}
                  </v-btn>
                </template>
                <v-list>
                  <v-list-item @click="updateLoadBalance('round-robin')">
                    <template v-slot:prepend>
                      <v-icon>mdi-rotate-right</v-icon>
                    </template>
                    <v-list-item-title>轮询 (Round Robin)</v-list-item-title>
                  </v-list-item>
                  <v-list-item @click="updateLoadBalance('random')">
                    <template v-slot:prepend>
                      <v-icon>mdi-dice-6</v-icon>
                    </template>
                    <v-list-item-title>随机 (Random)</v-list-item-title>
                  </v-list-item>
                  <v-list-item @click="updateLoadBalance('failover')">
                    <template v-slot:prepend>
                      <v-icon>mdi-backup-restore</v-icon>
                    </template>
                    <v-list-item-title>故障转移 (Failover)</v-list-item-title>
                  </v-list-item>
                </v-list>
              </v-menu>
            </div>
          </v-card-text>
        </v-card>

        <!-- 渠道列表 -->
        <v-row v-if="channelsData.channels?.length">
          <transition-group name="channel-list" tag="div" class="d-contents">
            <v-col
              v-for="channel in sortedChannels"
              :key="channel.index"
              cols="12"
              md="6"
              lg="4"
              xl="4"
              class="channel-col"
            >
            <ChannelCard
              :channel="channel"
              :is-current="channel.index === channelsData.current"
              @edit="editChannel"
              @delete="deleteChannel"
              @set-current="setCurrentChannel"
              @add-key="openAddKeyModal"
              @remove-key="removeApiKey"
              @ping="pingChannel"
              @toggle-pin="toggleChannelPin"
            />
            </v-col>
          </transition-group>
        </v-row>

        <!-- 空状态 -->
        <v-card v-else elevation="2" class="text-center pa-12" rounded="lg">
          <v-avatar size="120" color="primary" class="mb-6">
            <v-icon size="60" color="white">mdi-rocket-launch</v-icon>
          </v-avatar>
          <div class="text-h4 mb-4 font-weight-bold">暂无渠道配置</div>
          <div class="text-subtitle-1 text-medium-emphasis mb-8">还没有配置任何API渠道，请添加第一个渠道来开始使用代理服务</div>
          <v-btn
            color="primary"
            size="x-large"
            @click="openAddChannelModal"
            prepend-icon="mdi-plus"
            variant="elevated"
          >
            添加第一个渠道
          </v-btn>
        </v-card>
      </v-container>
    </v-main>

    <!-- 添加渠道模态框 -->
    <AddChannelModal
      v-model:show="showAddChannelModal"
      :channel="editingChannel"
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
import { ref, onMounted, computed } from 'vue'
import { useTheme } from 'vuetify'
import { api, type Channel, type ChannelsResponse } from './services/api'
import ChannelCard from './components/ChannelCard.vue'
import AddChannelModal from './components/AddChannelModal.vue'

// Vuetify主题
const theme = useTheme()

// 响应式数据
const channelsData = ref<ChannelsResponse>({ channels: [], current: -1, loadBalance: 'round-robin' })
const showAddChannelModal = ref(false)
const showAddKeyModalRef = ref(false)
const editingChannel = ref<Channel | null>(null)
const selectedChannelForKey = ref<number>(-1)
const newApiKey = ref('')
const isPingingAll = ref(false)
const currentTheme = ref<'light' | 'dark' | 'auto'>('auto')

// Pin状态管理 (使用localStorage持久化)
const PINNED_CHANNELS_KEY = 'claude-proxy-pinned-channels'
const pinnedChannels = ref<Set<number>>(new Set())

// 从localStorage加载pin状态
const loadPinnedChannels = () => {
  try {
    const saved = localStorage.getItem(PINNED_CHANNELS_KEY)
    if (saved) {
      const pinnedArray = JSON.parse(saved) as number[]
      pinnedChannels.value = new Set(pinnedArray)
    }
  } catch (error) {
    console.warn('加载pin状态失败:', error)
    pinnedChannels.value = new Set()
  }
}

// 保存pin状态到localStorage
const savePinnedChannels = () => {
  try {
    const pinnedArray = Array.from(pinnedChannels.value)
    localStorage.setItem(PINNED_CHANNELS_KEY, JSON.stringify(pinnedArray))
  } catch (error) {
    console.warn('保存pin状态失败:', error)
  }
}

// Toast通知系统
interface Toast {
  id: number
  message: string
  type: 'success' | 'error' | 'warning' | 'info'
  show?: boolean
}
const toasts = ref<Toast[]>([])
let toastId = 0

// 计算属性
const getCurrentChannelName = () => {
  const current = channelsData.value.channels?.find(c => c.index === channelsData.value.current)
  return current?.name || current?.serviceType || '未设置'
}

const currentChannelType = computed(() => {
  const current = channelsData.value.channels?.find(c => c.index === channelsData.value.current)
  return current?.serviceType?.toUpperCase() || ''
})

// 自动排序渠道：当前渠道排在最前面，pinned渠道排在当前渠道后面
const sortedChannels = computed(() => {
  if (!channelsData.value.channels) return []
  
  const channels = [...channelsData.value.channels]
  
  // 排序逻辑：当前渠道 > pinned渠道 > 其他渠道
  return channels.sort((a, b) => {
    const aIsCurrent = a.index === channelsData.value.current
    const bIsCurrent = b.index === channelsData.value.current
    const aIsPinned = pinnedChannels.value.has(a.index)
    const bIsPinned = pinnedChannels.value.has(b.index)
    
    // 当前渠道始终排在最前面
    if (aIsCurrent && !bIsCurrent) return -1
    if (!aIsCurrent && bIsCurrent) return 1
    
    // 如果都不是当前渠道，则比较pin状态
    if (!aIsCurrent && !bIsCurrent) {
      // pinned渠道排在非pinned渠道前面
      if (aIsPinned && !bIsPinned) return -1
      if (!aIsPinned && bIsPinned) return 1
      
      // 同样pin状态下，按index排序
      return a.index - b.index
    }
    
    // 保持原有顺序
    return a.index - b.index
  })
})

// Toast工具函数
const getToastColor = (type: string) => {
  const colorMap: Record<string, string> = {
    'success': 'success',
    'error': 'error',
    'warning': 'warning',
    'info': 'info'
  }
  return colorMap[type] || 'info'
}

const getToastIcon = (type: string) => {
  const iconMap: Record<string, string> = {
    'success': 'mdi-check-circle',
    'error': 'mdi-alert-circle',
    'warning': 'mdi-alert',
    'info': 'mdi-information'
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

// Pin相关函数
const toggleChannelPin = (channelId: number) => {
  if (pinnedChannels.value.has(channelId)) {
    pinnedChannels.value.delete(channelId)
    showToast('渠道已取消置顶', 'info')
  } else {
    pinnedChannels.value.add(channelId)
    showToast('渠道已置顶', 'success')
  }
  savePinnedChannels()
  updateChannelsPinnedStatus()
}

const isChannelPinned = (channelId: number): boolean => {
  return pinnedChannels.value.has(channelId)
}

// 更新渠道的pinned状态
const updateChannelsPinnedStatus = () => {
  if (channelsData.value.channels) {
    channelsData.value.channels.forEach(channel => {
      channel.pinned = pinnedChannels.value.has(channel.index)
    })
  }
}

// 主要功能函数
const refreshChannels = async () => {
  try {
    channelsData.value = await api.getChannels()
    updateChannelsPinnedStatus()
  } catch (error) {
    handleError(error, '获取渠道列表失败')
  }
}

const saveChannel = async (channel: Omit<Channel, 'index' | 'latency' | 'status'>) => {
  try {
    if (editingChannel.value) {
      await api.updateChannel(editingChannel.value.index, channel)
      showToast('渠道更新成功', 'success')
    } else {
      await api.addChannel(channel)
      showToast('渠道添加成功', 'success')
    }
    showAddChannelModal.value = false
    editingChannel.value = null
    await refreshChannels()
  } catch (error) {
    handleError(error, editingChannel.value ? '更新渠道失败' : '添加渠道失败')
  }
}

const editChannel = (channel: Channel) => {
  editingChannel.value = channel
  showAddChannelModal.value = true
}

const deleteChannel = async (channelId: number) => {
  if (!confirm('确定要删除这个渠道吗？')) return
  
  try {
    await api.deleteChannel(channelId)
    showToast('渠道删除成功', 'success')
    await refreshChannels()
  } catch (error) {
    handleError(error, '删除渠道失败')
  }
}

const openAddChannelModal = () => {
  editingChannel.value = null
  showAddChannelModal.value = true
}

const setCurrentChannel = async (channelId: number) => {
  try {
    await api.setCurrentChannel(channelId)
    showToast('当前渠道设置成功', 'success')
    await refreshChannels()
  } catch (error) {
    handleError(error, '设置当前渠道失败')
  }
}

const openAddKeyModal = (channelId: number) => {
  selectedChannelForKey.value = channelId
  newApiKey.value = ''
  showAddKeyModalRef.value = true
}

const addApiKey = async () => {
  if (!newApiKey.value.trim()) return
  
  try {
    await api.addApiKey(selectedChannelForKey.value, newApiKey.value.trim())
    showToast('API密钥添加成功', 'success')
    showAddKeyModalRef.value = false
    newApiKey.value = ''
    await refreshChannels()
  } catch (error) {
    handleError(error, '添加API密钥失败')
  }
}

const removeApiKey = async (channelId: number, apiKey: string) => {
  if (!confirm('确定要删除这个API密钥吗？')) return
  
  try {
    await api.removeApiKey(channelId, apiKey)
    showToast('API密钥删除成功', 'success')
    await refreshChannels()
  } catch (error) {
    handleError(error, '删除API密钥失败')
  }
}

const pingChannel = async (channelId: number) => {
  try {
    const result = await api.pingChannel(channelId)
    const channel = channelsData.value.channels?.find(c => c.index === channelId)
    if (channel) {
      channel.latency = result.latency
      channel.status = result.success ? 'healthy' : 'error'
    }
    showToast(`延迟测试完成: ${result.latency}ms`, result.success ? 'success' : 'warning')
  } catch (error) {
    handleError(error, '延迟测试失败')
  }
}

const pingAllChannels = async () => {
  if (isPingingAll.value) return
  
  isPingingAll.value = true
  try {
    const results = await api.pingAllChannels()
    results.forEach(result => {
      const channel = channelsData.value.channels?.find(c => c.index === result.id)
      if (channel) {
        channel.latency = result.latency
        channel.status = result.status as 'healthy' | 'error'
      }
    })
    showToast('全部渠道延迟测试完成', 'success')
  } catch (error) {
    handleError(error, '批量延迟测试失败')
  } finally {
    isPingingAll.value = false
  }
}

const updateLoadBalance = async (strategy: string) => {
  try {
    await api.updateLoadBalance(strategy)
    channelsData.value.loadBalance = strategy
    showToast(`负载均衡策略已更新为: ${strategy}`, 'success')
  } catch (error) {
    handleError(error, '更新负载均衡策略失败')
  }
}

// 主题管理
const toggleTheme = () => {
  const newTheme = currentTheme.value === 'dark' ? 'light' : 'dark'
  setTheme(newTheme)
}

const setTheme = (themeName: 'light' | 'dark' | 'auto') => {
  currentTheme.value = themeName
  const apply = (isDark: boolean) => {
    // Sync Vuetify theme
    theme.global.name.value = isDark ? 'dark' : 'light'
    // Sync DaisyUI theme on <html data-theme="...">
    const daisyTheme = isDark ? 'night' : 'emerald'
    document.documentElement.setAttribute('data-theme', daisyTheme)
  }

  if (themeName === 'auto') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    apply(prefersDark)
  } else {
    apply(themeName === 'dark')
  }

  localStorage.setItem('theme', themeName)
}

// 初始化
onMounted(async () => {
  // 加载保存的主题
  const savedTheme = localStorage.getItem('theme') as 'light' | 'dark' | 'auto' || 'auto'
  setTheme(savedTheme)
  
  // 监听系统主题变化
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  const handlePref = () => { if (currentTheme.value === 'auto') setTheme('auto') }
  mediaQuery.addEventListener('change', handlePref)
  
  // 加载pin状态
  loadPinnedChannels()
  
  // 加载渠道数据
  await refreshChannels()
})
</script>

<style scoped>
.app-header {
  transition: height 0.3s ease;
  padding-left: 16px !important;
  padding-right: 16px !important;
}

.app-header .v-toolbar-title {
  overflow: visible !important;
  width: auto !important;
}

/* 响应式内边距调整 */
@media (min-width: 768px) {
  .app-header {
    padding-left: 24px !important;
    padding-right: 24px !important;
  }
}

@media (min-width: 1024px) {
  .app-header {
    padding-left: 32px !important;
    padding-right: 32px !important;
  }
}

/* 确保在不同屏幕尺寸下的文本对齐 */
@media (max-width: 600px) {
  .app-header .v-toolbar-title .text-h6,
  .app-header .v-toolbar-title .text-h5 {
    line-height: 1.2;
  }
  .app-header {
    padding-left: 12px !important;
    padding-right: 12px !important;
  }
}

/* 渠道列表动画效果 */
.d-contents {
  display: contents;
}

.channel-col {
  transition: all 0.4s ease;
  max-width: 550px;
  margin: 0 auto;
}

.channel-list-enter-active,
.channel-list-leave-active {
  transition: all 0.4s ease;
}

.channel-list-enter-from {
  opacity: 0;
  transform: translateY(30px) scale(0.95);
}

.channel-list-leave-to {
  opacity: 0;
  transform: translateY(-30px) scale(0.95);
}

.channel-list-move {
  transition: transform 0.4s ease;
}
</style>
