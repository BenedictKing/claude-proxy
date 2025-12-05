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
    <v-app-bar elevation="0" :height="$vuetify.display.mobile ? 64 : 72" class="app-header">
      <template #prepend>
        <div class="app-logo">
          <v-icon :size="$vuetify.display.mobile ? 26 : 32" color="primary"> mdi-rocket-launch </v-icon>
        </div>
      </template>

      <v-app-bar-title class="d-flex flex-column justify-center">
        <div
          :class="$vuetify.display.mobile ? 'text-subtitle-1' : 'text-h6'"
          class="font-weight-bold d-flex align-center"
        >
          <span class="api-type-text" :class="{ active: activeTab === 'messages' }" @click="activeTab = 'messages'">
            Claude
          </span>
          <span class="api-type-text separator">/</span>
          <span class="api-type-text" :class="{ active: activeTab === 'responses' }" @click="activeTab = 'responses'">
            Codex
          </span>
          <span class="brand-text">API Proxy</span>
        </div>
      </v-app-bar-title>

      <v-spacer></v-spacer>

      <!-- 主题切换 -->
      <v-btn icon variant="text" size="small" class="header-btn" @click="toggleTheme">
        <v-icon size="20">{{ currentTheme === 'dark' ? 'mdi-weather-night' : 'mdi-white-balance-sunny' }}</v-icon>
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
        <!-- 统计卡片 - 现代玻璃拟态风格 -->
        <v-row class="mb-6 stat-cards-row">
          <v-col cols="12" sm="6" lg="3">
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

          <v-col cols="12" sm="6" lg="3">
            <div class="stat-card stat-card-success">
              <div class="stat-card-icon">
                <v-icon size="28">mdi-check-circle</v-icon>
              </div>
              <div class="stat-card-content">
                <div class="stat-card-value">
                  {{ activeChannelCount
                  }}<span class="stat-card-total">/{{ currentChannelsData.channels?.length || 0 }}</span>
                </div>
                <div class="stat-card-label">活跃渠道</div>
                <div class="stat-card-desc">参与故障转移调度</div>
              </div>
              <div class="stat-card-glow"></div>
            </div>
          </v-col>

          <v-col cols="12" sm="6" lg="3">
            <div class="stat-card stat-card-primary">
              <div class="stat-card-icon">
                <v-icon size="28">mdi-swap-horizontal</v-icon>
              </div>
              <div class="stat-card-content">
                <div class="stat-card-value text-capitalize">{{ currentChannelsData.loadBalance || 'none' }}</div>
                <div class="stat-card-label">API密钥分配</div>
                <div class="stat-card-desc">当前渠道内密钥策略</div>
              </div>
              <div class="stat-card-glow"></div>
            </div>
          </v-col>

          <v-col cols="12" sm="6" lg="3">
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
          :channels="currentChannelsData.channels"
          :current-channel-index="currentChannelsData.current"
          :channel-type="activeTab"
          @edit="editChannel"
          @delete="deleteChannel"
          @ping="pingChannel"
          @refresh="refreshChannels"
          @error="showErrorToast"
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
import { ref, onMounted, computed, watch } from 'vue'
import { useTheme } from 'vuetify'
import { api, type Channel, type ChannelsResponse } from './services/api'
import AddChannelModal from './components/AddChannelModal.vue'
import ChannelOrchestration from './components/ChannelOrchestration.vue'

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

// 计算属性：活跃渠道数（非 disabled 状态）
const activeChannelCount = computed(() => {
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

// 主要功能函数
const refreshChannels = async () => {
  try {
    if (activeTab.value === 'messages') {
      channelsData.value = await api.getChannels()
    } else {
      responsesChannelsData.value = await api.getResponsesChannels()
    }
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
    console.info('✅ 认证成功 - 时间:', new Date().toISOString())
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

// 初始化
onMounted(async () => {
  // 加载保存的主题
  const savedTheme = (localStorage.getItem('theme') as 'light' | 'dark' | 'auto') || 'auto'
  setTheme(savedTheme)

  // 监听系统主题变化
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  const handlePref = () => {
    if (currentTheme.value === 'auto') setTheme('auto')
  }
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
/* =====================================================
   🎨 现代化 UI 样式系统
   ===================================================== */

/* ----- 应用栏 - 毛玻璃效果 ----- */
.app-header {
  background: rgba(var(--v-theme-surface), 0.8) !important;
  backdrop-filter: blur(20px) saturate(180%);
  -webkit-backdrop-filter: blur(20px) saturate(180%);
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  transition: all 0.3s ease;
  padding: 0 16px !important;
}

.v-theme--dark .app-header {
  background: rgba(var(--v-theme-surface), 0.75) !important;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.app-header .v-toolbar-title {
  overflow: visible !important;
  width: auto !important;
}

.app-logo {
  width: 42px;
  height: 42px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(var(--v-theme-primary), 0.15), rgba(var(--v-theme-secondary), 0.1));
  border-radius: 12px;
  margin-right: 12px;
}

.brand-text {
  margin-left: 10px;
  background: linear-gradient(135deg, rgb(var(--v-theme-primary)), rgb(var(--v-theme-secondary)));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.header-btn {
  border-radius: 10px !important;
  margin-left: 4px;
}

.header-btn:hover {
  background: rgba(var(--v-theme-primary), 0.1);
}

/* ----- API Tab 切换样式 ----- */
.api-type-text {
  cursor: pointer;
  opacity: 0.5;
  transition: all 0.2s ease;
  padding: 4px 8px;
  border-radius: 6px;
  position: relative;
}

.api-type-text:not(.separator):hover {
  opacity: 0.8;
  background: rgba(var(--v-theme-primary), 0.08);
}

.api-type-text.active {
  opacity: 1;
  font-weight: 700;
  color: rgb(var(--v-theme-primary));
}

.api-type-text.active::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: 0;
  height: 2px;
  border-radius: 999px;
  background: linear-gradient(90deg, rgb(var(--v-theme-primary)), rgb(var(--v-theme-secondary)));
}

.separator {
  opacity: 0.25;
  margin: 0 2px;
  cursor: default;
  padding: 0;
}

/* ----- 统计卡片 - 玻璃拟态 ----- */
.stat-cards-row {
  margin-top: -8px;
}

.stat-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  border-radius: 16px;
  background: rgba(var(--v-theme-surface), 0.7);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  overflow: hidden;
  min-height: 100px;
}

.stat-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.12);
}

.v-theme--dark .stat-card {
  background: rgba(var(--v-theme-surface), 0.5);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.v-theme--dark .stat-card:hover {
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.4);
}

.stat-card-icon {
  width: 56px;
  height: 56px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  transition: transform 0.3s ease;
}

.stat-card:hover .stat-card-icon {
  transform: scale(1.1);
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
}

.stat-card-desc {
  font-size: 0.75rem;
  opacity: 0.6;
  margin-top: 2px;
}

.stat-card-glow {
  position: absolute;
  width: 120px;
  height: 120px;
  border-radius: 50%;
  filter: blur(40px);
  opacity: 0.4;
  right: -20px;
  top: -20px;
  transition: opacity 0.3s ease;
  pointer-events: none;
}

.stat-card:hover .stat-card-glow {
  opacity: 0.6;
}

/* 统计卡片颜色变体 */
.stat-card-info .stat-card-icon {
  background: linear-gradient(135deg, #3b82f6, #60a5fa);
  color: white;
}
.stat-card-info .stat-card-value {
  color: #3b82f6;
}
.stat-card-info .stat-card-glow {
  background: #3b82f6;
}
.v-theme--dark .stat-card-info .stat-card-value {
  color: #60a5fa;
}

.stat-card-success .stat-card-icon {
  background: linear-gradient(135deg, #10b981, #34d399);
  color: white;
}
.stat-card-success .stat-card-value {
  color: #10b981;
}
.stat-card-success .stat-card-glow {
  background: #10b981;
}
.v-theme--dark .stat-card-success .stat-card-value {
  color: #34d399;
}

.stat-card-primary .stat-card-icon {
  background: linear-gradient(135deg, #6366f1, #818cf8);
  color: white;
}
.stat-card-primary .stat-card-value {
  color: #6366f1;
}
.stat-card-primary .stat-card-glow {
  background: #6366f1;
}
.v-theme--dark .stat-card-primary .stat-card-value {
  color: #818cf8;
}

.stat-card-emerald .stat-card-icon {
  background: linear-gradient(135deg, #059669, #10b981);
  color: white;
}
.stat-card-emerald .stat-card-value {
  color: #059669;
}
.stat-card-emerald .stat-card-glow {
  background: #059669;
}
.v-theme--dark .stat-card-emerald .stat-card-value {
  color: #34d399;
}

/* ----- 操作按钮区域 ----- */
.action-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 20px;
  background: rgba(var(--v-theme-surface), 0.7);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 16px;
}

.v-theme--dark .action-bar {
  background: rgba(var(--v-theme-surface), 0.5);
  border: 1px solid rgba(255, 255, 255, 0.06);
}

.action-bar-left {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.action-bar-right {
  display: flex;
  align-items: center;
}

.action-btn {
  border-radius: 12px !important;
  font-weight: 600;
  letter-spacing: 0.3px;
  transition: all 0.2s ease;
}

.action-btn:hover {
  transform: translateY(-1px);
}

.action-btn-primary {
  box-shadow: 0 4px 14px rgba(99, 102, 241, 0.35) !important;
}

.action-btn-primary:hover {
  box-shadow: 0 6px 20px rgba(99, 102, 241, 0.45) !important;
}

.load-balance-btn {
  text-transform: capitalize;
}

.load-balance-menu {
  min-width: 300px;
  padding: 8px;
}

.load-balance-menu .v-list-item {
  margin-bottom: 4px;
  padding: 12px 16px;
}

.load-balance-menu .v-list-item:last-child {
  margin-bottom: 0;
}

@media (max-width: 600px) {
  .action-bar {
    flex-direction: column;
    align-items: stretch;
    padding: 12px 16px;
  }

  .action-bar-left,
  .action-bar-right {
    justify-content: center;
  }

  .action-btn {
    flex: 1;
    min-width: 0;
  }
}

/* 心跳动画 */
.pulse-animation {
  animation: pulse 2s ease-in-out infinite;
}

@keyframes pulse {
  0%,
  100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.05);
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

@media (max-width: 600px) {
  .app-header {
    padding: 0 12px !important;
  }

  .app-logo {
    width: 36px;
    height: 36px;
    border-radius: 10px;
    margin-right: 8px;
  }

  .stat-card {
    padding: 16px;
    gap: 12px;
  }

  .stat-card-icon {
    width: 48px;
    height: 48px;
    border-radius: 12px;
  }

  .stat-card-value {
    font-size: 1.5rem;
  }
}

/* ----- 渠道列表动画 ----- */
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
