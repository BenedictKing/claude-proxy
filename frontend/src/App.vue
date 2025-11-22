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
        <v-card-title class="text-h5 text-center mb-4">
          🔐 Claude Proxy 管理界面
        </v-card-title>
        
        <v-card-text>
          <v-alert
            v-if="authError"
            type="error"
            variant="tonal"
            class="mb-4"
          >
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
            
            <v-btn
              type="submit"
              color="primary"
              block
              size="large"
              class="mt-4"
              :loading="authLoading"
            >
              访问管理界面
            </v-btn>
          </v-form>
          
          <v-divider class="my-4" />
          
          <div class="text-body-2 text-center text-medium-emphasis">
            <p>💡 <strong>提示：</strong></p>
            <p>• 访问密钥在服务器的 <code>PROXY_ACCESS_KEY</code> 环境变量中设置</p>
            <p>• 密钥将安全保存在本地，下次访问将自动验证登录</p>
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>
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
        <div :class="$vuetify.display.mobile ? 'text-h6' : 'text-h5'" class="font-weight-bold mb-1 d-flex align-center">
          <span
            class="api-type-text"
            :class="{ 'active': activeTab === 'messages' }"
            @click="activeTab = 'messages'"
          >
            Claude
          </span>
          <span class="api-type-text separator">/</span>
          <span
            class="api-type-text"
            :class="{ 'active': activeTab === 'responses' }"
            @click="activeTab = 'responses'"
          >
            Codex
          </span>
          <span style="margin-left: 12px;">API Proxy</span>
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
      
      <!-- 注销按钮 -->
      <v-btn
        icon
        variant="text"
        @click="handleLogout"
        v-if="isAuthenticated"
        title="注销"
      >
        <v-icon>mdi-logout</v-icon>
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
                    <div class="text-h4 text-info font-weight-bold">{{ currentChannelsData.channels?.length || 0 }}</div>
                    <div class="text-subtitle-1 text-medium-emphasis">总渠道数</div>
                    <div class="text-caption text-medium-emphasis">已配置的API渠道</div>
                  </div>
                  <v-avatar size="60" color="info" variant="tonal">
                    <v-icon size="30" color="info">mdi-server-network</v-icon>
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
                    <div class="text-h6 text-success font-weight-bold text-truncate" style="max-width: 120px;">{{ getCurrentChannelName() }}</div>
                    <div class="text-subtitle-1 text-medium-emphasis">当前渠道</div>
                    <div class="text-caption text-success font-weight-medium">{{ currentChannelType }}</div>
                  </div>
                  <v-avatar size="60" color="success" variant="tonal">
                    <v-icon size="30" color="success">mdi-check-circle</v-icon>
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
                    <div class="text-h6 text-info font-weight-bold text-capitalize">{{ currentChannelsData.loadBalance || 'none' }}</div>
                    <div class="text-subtitle-1 text-medium-emphasis">API密钥分配</div>
                    <div class="text-caption text-medium-emphasis">当前渠道内密钥使用策略</div>
                  </div>
                  <v-avatar size="60" color="info" variant="tonal">
                    <v-icon size="30" color="info">mdi-swap-horizontal</v-icon>
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
                    <div class="text-h6 text-success font-weight-bold">运行中</div>
                    <div class="text-subtitle-1 text-medium-emphasis">系统状态</div>
                    <div class="text-caption text-medium-emphasis">服务正常运行</div>
                  </div>
                  <v-avatar size="60" color="success" variant="tonal">
                    <v-icon size="30" color="success">mdi-heart-pulse</v-icon>
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
                  color="info"
                  size="large"
                  @click="pingAllChannels"
                  prepend-icon="mdi-speedometer"
                  variant="outlined"
                  :loading="isPingingAll"
                >
                  测试全部延迟
                </v-btn>

                <v-btn
                  color="secondary"
                  size="large"
                  @click="refreshChannels"
                  prepend-icon="mdi-refresh"
                  variant="tonal"
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
                    API密钥分配: {{ currentChannelsData.loadBalance }}
                  </v-btn>
                </template>
                <v-list>
                  <v-list-item @click="updateLoadBalance('round-robin')">
                    <template v-slot:prepend>
                      <v-icon>mdi-rotate-right</v-icon>
                    </template>
                    <v-list-item-title>轮询 (Round Robin)</v-list-item-title>
                    <v-list-item-subtitle>按顺序依次使用当前渠道的API密钥</v-list-item-subtitle>
                  </v-list-item>
                  <v-list-item @click="updateLoadBalance('random')">
                    <template v-slot:prepend>
                      <v-icon>mdi-dice-6</v-icon>
                    </template>
                    <v-list-item-title>随机 (Random)</v-list-item-title>
                    <v-list-item-subtitle>随机选择当前渠道的API密钥</v-list-item-subtitle>
                  </v-list-item>
                  <v-list-item @click="updateLoadBalance('failover')">
                    <template v-slot:prepend>
                      <v-icon>mdi-backup-restore</v-icon>
                    </template>
                    <v-list-item-title>故障转移 (Failover)</v-list-item-title>
                    <v-list-item-subtitle>优先使用第一个密钥，失败时自动切换</v-list-item-subtitle>
                  </v-list-item>
                </v-list>
              </v-menu>
            </div>
          </v-card-text>
        </v-card>

        <!-- 渠道列表 -->
        <v-row v-if="channelsData.channels?.length" justify="start">
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
              :is-current="channel.index === currentChannelsData.current"
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
import { ref, onMounted, computed, watch } from 'vue'
import { useTheme } from 'vuetify'
import { api, type Channel, type ChannelsResponse } from './services/api'
import ChannelCard from './components/ChannelCard.vue'
import AddChannelModal from './components/AddChannelModal.vue'

// Vuetify主题
const theme = useTheme()

// 响应式数据
const activeTab = ref<'messages' | 'responses'>('messages') // Tab 切换状态
const channelsData = ref<ChannelsResponse>({ channels: [], current: -1, loadBalance: 'round-robin' })
const responsesChannelsData = ref<ChannelsResponse>({ channels: [], current: -1, loadBalance: 'round-robin' }) // Responses渠道数据
const showAddChannelModal = ref(false)
const showAddKeyModalRef = ref(false)
const editingChannel = ref<Channel | null>(null)
const selectedChannelForKey = ref<number>(-1)
const newApiKey = ref('')
const isPingingAll = ref(false)
const currentTheme = ref<'light' | 'dark' | 'auto'>('auto')

// Pin状态管理 (使用localStorage持久化，Claude/Codex分开存储)
const PINNED_CHANNELS_KEYS: Record<'messages' | 'responses', string> = {
  messages: 'claude-proxy-pinned-channels',
  responses: 'codex-proxy-pinned-channels'
}
const pinnedChannels = ref<Record<'messages' | 'responses', Set<number>>>({
  messages: new Set(),
  responses: new Set()
})

const getPinnedSet = (tab: 'messages' | 'responses') => pinnedChannels.value[tab] || new Set<number>()

// 从localStorage加载pin状态
const loadPinnedChannels = (tab?: 'messages' | 'responses') => {
  const targets = tab ? [tab] : (['messages', 'responses'] as const)
  targets.forEach(t => {
    try {
      const saved = localStorage.getItem(PINNED_CHANNELS_KEYS[t])
      const loadedSet = saved ? new Set<number>(JSON.parse(saved) as number[]) : new Set<number>()
      pinnedChannels.value = { ...pinnedChannels.value, [t]: loadedSet }
    } catch (error) {
      console.warn('加载pin状态失败:', error)
      pinnedChannels.value = { ...pinnedChannels.value, [t]: new Set<number>() }
    }
  })
}

// 保存pin状态到localStorage
const savePinnedChannels = (tab: 'messages' | 'responses') => {
  try {
    const pinnedArray = Array.from(getPinnedSet(tab))
    localStorage.setItem(PINNED_CHANNELS_KEYS[tab], JSON.stringify(pinnedArray))
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

// 计算属性 - 根据当前Tab动态返回数据
const currentChannelsData = computed(() => {
  return activeTab.value === 'messages' ? channelsData.value : responsesChannelsData.value
})

const getCurrentChannelName = () => {
  const current = currentChannelsData.value.channels?.find(c => c.index === currentChannelsData.value.current)
  return current?.name || current?.serviceType || '未设置'
}

const currentChannelType = computed(() => {
  const current = currentChannelsData.value.channels?.find(c => c.index === currentChannelsData.value.current)
  return current?.serviceType?.toUpperCase() || ''
})

// 自动排序渠道：当前渠道排在最前面，pinned渠道排在当前渠道后面
const sortedChannels = computed(() => {
  const data = currentChannelsData.value
  if (!data.channels) return []

  const channels = [...data.channels]
  const pinnedSet = getPinnedSet(activeTab.value)

  // 排序逻辑：当前渠道 > pinned渠道 > 其他渠道
  return channels.sort((a, b) => {
    const aIsCurrent = a.index === data.current
    const bIsCurrent = b.index === data.current
    const aIsPinned = pinnedSet.has(a.index)
    const bIsPinned = pinnedSet.has(b.index)
    
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
  const tab = activeTab.value
  const next = new Set(getPinnedSet(tab))

  if (next.has(channelId)) {
    next.delete(channelId)
    showToast('渠道已取消置顶', 'info')
  } else {
    next.add(channelId)
    showToast('渠道已置顶', 'success')
  }

  pinnedChannels.value = { ...pinnedChannels.value, [tab]: next }
  savePinnedChannels(tab)
  updateChannelsPinnedStatus()
}

const isChannelPinned = (channelId: number): boolean => {
  return getPinnedSet(activeTab.value).has(channelId)
}

// 更新渠道的pinned状态
const updateChannelsPinnedStatus = () => {
  // 更新 Messages Tab 的渠道数据
  if (channelsData.value.channels) {
    const messagesPinned = getPinnedSet('messages')
    channelsData.value.channels.forEach(channel => {
      channel.pinned = messagesPinned.has(channel.index)
    })
  }

  // 更新 Codex Tab 的渠道数据
  if (responsesChannelsData.value.channels) {
    const responsesPinned = getPinnedSet('responses')
    responsesChannelsData.value.channels.forEach(channel => {
      channel.pinned = responsesPinned.has(channel.index)
    })
  }
}

// 主要功能函数
const refreshChannels = async () => {
  try {
    if (activeTab.value === 'messages') {
      channelsData.value = await api.getChannels()
    } else {
      responsesChannelsData.value = await api.getResponsesChannels()
    }
    updateChannelsPinnedStatus()
  } catch (error) {
    handleAuthError(error)
  }
}

const saveChannel = async (channel: Omit<Channel, 'index' | 'latency' | 'status'>) => {
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

const setCurrentChannel = async (channelId: number) => {
  try {
    if (activeTab.value === 'responses') {
      await api.setCurrentResponsesChannel(channelId)
    } else {
      await api.setCurrentChannel(channelId)
    }
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
    handleError(error, '添加API密钥失败')
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
    handleError(error, '删除API密钥失败')
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
    handleError(error, '延迟测试失败')
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
    handleError(error, '批量延迟测试失败')
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

// 认证状态管理
const isAuthenticated = ref(false)
const authError = ref('')
const authKeyInput = ref('')
const authLoading = ref(false)
const isAutoAuthenticating = ref(true) // 初始化为true，防止登录框闪现
const isInitialized = ref(false) // 添加初始化完成标志

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
  
  authLoading.value = true
  authError.value = ''
  
  try {
    // 设置密钥
    setAuthKey(authKeyInput.value.trim())
    
    // 测试API调用以验证密钥
    await api.getChannels()
    
    // 如果成功，加载数据
    loadPinnedChannels()
    await refreshChannels()
    
    authKeyInput.value = ''
  } catch (error: any) {
    // 认证失败
    isAuthenticated.value = false
    authError.value = error.message || '访问密钥验证失败'
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
    handleError(error, '操作失败')
  }
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
    // 加载pin状态
    loadPinnedChannels()

    // 加载渠道数据
    await refreshChannels()
  }
})

// 监听 Tab 切换，刷新对应数据
watch(activeTab, async () => {
  if (isAuthenticated.value) {
    await refreshChannels()
  }
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

/* API 类型切换文本样式：下划线高亮 */
.api-type-text {
  cursor: pointer;
  opacity: 0.55;
  transition:
    opacity 0.18s ease,
    transform 0.18s ease;
  padding: 2px 4px;
  display: inline-block;
  position: relative;
}

.api-type-text:not(.separator):hover {
  opacity: 0.85;
  transform: translateY(-0.5px);
}

.api-type-text.active {
  opacity: 1;
  transform: translateY(-0.5px);
  font-weight: 900;
}

.api-type-text.active::after {
  content: '';
  position: absolute;
  left: 4px;
  right: 4px;
  bottom: 0;
  height: 3px;
  border-radius: 999px;
  background-color: rgba(255, 255, 255, 0.9);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
}

.separator {
  opacity: 0.32;
  margin: 0 6px;
  cursor: default;
  padding: 0;
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
  max-width: 640px;
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
