<template>
  <v-dialog 
    :model-value="show"
    @update:model-value="$emit('update:show', $event)"
    max-width="800"
    persistent
  >
    <v-card rounded="lg">
      <v-card-title class="d-flex align-center ga-3 pa-6" :class="headerClasses">
        <v-avatar :color="avatarColor" variant="flat" size="40">
          <v-icon :style="headerIconStyle" size="20">{{ isEditing ? 'mdi-pencil' : 'mdi-plus' }}</v-icon>
        </v-avatar>
        <div>
          <div class="text-h5 font-weight-bold">
            {{ isEditing ? '编辑渠道' : '添加新渠道' }}
          </div>
          <div class="text-body-2" :class="subtitleClasses">配置API渠道信息和密钥</div>
        </div>
      </v-card-title>

      <v-card-text class="pa-6">
        <v-form ref="formRef" @submit.prevent="handleSubmit">
          <v-row>
            <!-- 基本信息 -->
            <v-col cols="12" md="6">
              <v-text-field
                v-model="form.name"
                label="渠道名称"
                placeholder="例如：GPT-4 渠道"
                prepend-inner-icon="mdi-tag"
                variant="outlined"
                density="comfortable"
                :rules="[rules.required]"
                required
                :error-messages="errors.name"
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-select
                v-model="form.serviceType"
                label="服务类型"
                :items="serviceTypeOptions"
                prepend-inner-icon="mdi-cog"
                variant="outlined"
                density="comfortable"
                :rules="[rules.required]"
                required
                :error-messages="errors.serviceType"
              />
            </v-col>

            <!-- 基础URL -->
            <v-col cols="12">
              <v-text-field
                v-model="form.baseUrl"
                label="基础URL"
                placeholder="例如：https://api.openai.com/v1"
                prepend-inner-icon="mdi-web"
                variant="outlined"
                density="comfortable"
                type="url"
                :rules="[rules.required, rules.url]"
                required
                :error-messages="errors.baseUrl"
                :hint="getUrlHint()"
                persistent-hint
              />
            </v-col>

            <!-- 官网/控制台（可选） -->
            <v-col cols="12">
              <v-text-field
                v-model="form.website"
                label="官网/控制台 (可选)"
                placeholder="例如：https://platform.openai.com"
                prepend-inner-icon="mdi-open-in-new"
                variant="outlined"
                density="comfortable"
                type="url"
                :rules="[rules.urlOptional]"
                :error-messages="errors.website"
              />
            </v-col>

            <!-- 跳过 TLS 证书验证 -->
            <v-col cols="12">
              <div class="d-flex align-center justify-space-between">
                <div class="d-flex align-center ga-2">
                  <v-icon color="warning">mdi-shield-alert</v-icon>
                  <div>
                    <div class="text-body-1 font-weight-medium">跳过 TLS 证书验证</div>
                    <div class="text-caption text-medium-emphasis">仅在自签名或域名不匹配时临时启用，生产环境请关闭</div>
                  </div>
                </div>
                <v-switch
                  inset
                  color="warning"
                  hide-details
                  v-model="form.insecureSkipVerify"
                />
              </div>
            </v-col>

            <!-- 描述 -->
            <v-col cols="12">
              <v-textarea
                v-model="form.description"
                label="描述 (可选)"
                placeholder="可选的渠道描述..."
                prepend-inner-icon="mdi-text"
                variant="outlined"
                density="comfortable"
                rows="3"
                no-resize
              />
            </v-col>

            <!-- 模型重定向配置 -->
            <v-col cols="12" v-if="form.serviceType">
              <v-card variant="outlined" rounded="lg">
                <v-card-title class="d-flex align-center justify-space-between pa-4 pb-2">
                  <div class="d-flex align-center ga-2">
                    <v-icon color="primary">mdi-swap-horizontal</v-icon>
                    <span class="text-body-1 font-weight-bold">模型重定向 (可选)</span>
                  </div>
                  <v-chip size="small" color="secondary" variant="tonal">
                    自动转换模型名称
                  </v-chip>
                </v-card-title>

                <v-card-text class="pt-2">
                  <div class="text-body-2 text-medium-emphasis mb-4">
                    配置模型名称映射，将请求中的模型名重定向到目标模型。例如：将 "opus" 重定向到 "claude-3-5-sonnet"
                  </div>

                  <!-- 现有映射列表 -->
                  <div v-if="Object.keys(form.modelMapping).length" class="mb-4">
                    <v-list density="compact" class="bg-transparent">
                      <v-list-item 
                        v-for="[source, target] in Object.entries(form.modelMapping)"
                        :key="source"
                        class="mb-2"
                        rounded="lg"
                        variant="tonal"
                        color="surface-variant"
                      >
                        <template v-slot:prepend>
                          <v-icon size="small" color="primary">mdi-arrow-right</v-icon>
                        </template>
                        
                        <v-list-item-title>
                          <div class="d-flex align-center ga-2">
                            <code class="text-caption">{{ source }}</code>
                            <v-icon size="small" color="primary">mdi-arrow-right</v-icon>
                            <code class="text-caption">{{ target }}</code>
                          </div>
                        </v-list-item-title>

                        <template v-slot:append>
                          <v-btn
                            size="small"
                            color="error"
                            icon
                            variant="text"
                            @click="removeModelMapping(source)"
                          >
                            <v-icon size="small" color="error">mdi-close</v-icon>
                          </v-btn>
                        </template>
                      </v-list-item>
                    </v-list>
                  </div>

                  <!-- 添加新映射 -->
                  <div class="d-flex align-center ga-2">
                    <v-select
                      v-model="newMapping.source"
                      label="源模型名"
                      :items="sourceModelOptions"
                      variant="outlined"
                      density="comfortable"
                      hide-details
                      class="flex-1-1"
                      placeholder="选择源模型名"
                    />
                    <v-icon color="primary">mdi-arrow-right</v-icon>
                    <v-text-field
                      v-model="newMapping.target"
                      label="目标模型名"
                      placeholder="例如：claude-3-5-sonnet"
                      variant="outlined"
                      density="comfortable"
                      hide-details
                      class="flex-1-1"
                      @keyup.enter="addModelMapping"
                    />
                    <v-btn
                      color="secondary"
                      variant="elevated"
                      @click="addModelMapping"
                      :disabled="!newMapping.source.trim() || !newMapping.target.trim()"
                    >
                      添加
                    </v-btn>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>

            <!-- API密钥管理 -->
            <v-col cols="12">
              <v-card variant="outlined" rounded="lg">
                <v-card-title class="d-flex align-center justify-space-between pa-4 pb-2">
                  <div class="d-flex align-center ga-2">
                    <v-icon color="primary">mdi-key</v-icon>
                    <span class="text-body-1 font-weight-bold">API密钥管理</span>
                  </div>
                  <v-chip size="small" color="info" variant="tonal">
                    可添加多个密钥用于负载均衡
                  </v-chip>
                </v-card-title>

                <v-card-text class="pt-2">
                  <!-- 现有密钥列表 -->
                  <div v-if="form.apiKeys.length" class="mb-4">
                    <v-list density="compact" class="bg-transparent">
                      <v-list-item 
                        v-for="(key, index) in form.apiKeys"
                        :key="index"
                        class="mb-2"
                        rounded="lg"
                        variant="tonal"
                        :color="duplicateKeyIndex === index ? 'error' : 'surface-variant'"
                        :class="{ 'animate-pulse': duplicateKeyIndex === index }"
                      >
                        <template v-slot:prepend>
                          <v-icon 
                            size="small" 
                            :color="duplicateKeyIndex === index ? 'error' : 'primary'"
                          >
                            {{ duplicateKeyIndex === index ? 'mdi-alert' : 'mdi-key' }}
                          </v-icon>
                        </template>
                        
                        <v-list-item-title>
                          <div class="d-flex align-center justify-space-between">
                            <code class="text-caption">{{ maskApiKey(key) }}</code>
                            <v-chip 
                              v-if="duplicateKeyIndex === index"
                              size="x-small"
                              color="error"
                              variant="text"
                            >
                              重复密钥
                            </v-chip>
                          </div>
                        </v-list-item-title>

                        <template v-slot:append>
                          <div class="d-flex align-center ga-1">
                            <v-tooltip :text="copiedKeyIndex === index ? '已复制!' : '复制密钥'" location="top" :open-delay="150">
                              <template #activator="{ props: tooltipProps }">
                                <v-btn
                                  v-bind="tooltipProps"
                                  size="small"
                                  :color="copiedKeyIndex === index ? 'success' : 'primary'"
                                  icon
                                  variant="text"
                                  @click="copyApiKey(key, index)"
                                >
                                  <v-icon size="small">{{ copiedKeyIndex === index ? 'mdi-check' : 'mdi-content-copy' }}</v-icon>
                                </v-btn>
                              </template>
                            </v-tooltip>
                            <v-btn
                              size="small"
                              color="error"
                              icon
                              variant="text"
                              @click="removeApiKey(index)"
                            >
                              <v-icon size="small" color="error">mdi-close</v-icon>
                            </v-btn>
                          </div>
                        </template>
                      </v-list-item>
                    </v-list>
                  </div>

                  <!-- 添加新密钥 -->
                  <div class="d-flex align-start ga-3">
                    <v-text-field
                      v-model="newApiKey"
                      label="添加新的API密钥"
                      placeholder="输入完整的API密钥"
                      prepend-inner-icon="mdi-plus"
                      variant="outlined"
                      density="comfortable"
                      type="password"
                      @keyup.enter="addApiKey"
                      :error="!!apiKeyError"
                      :error-messages="apiKeyError"
                      @input="apiKeyError = ''; duplicateKeyIndex = -1"
                      class="flex-grow-1"
                    />
                    <v-btn
                      color="primary"
                      variant="elevated"
                      size="large"
                      height="40"
                      @click="addApiKey"
                      :disabled="!newApiKey.trim()"
                      class="mt-1"
                    >
                      添加
                    </v-btn>
                  </div>
                </v-card-text>
              </v-card>
            </v-col>
          </v-row>
        </v-form>
      </v-card-text>

      <v-card-actions class="pa-6 pt-0">
        <v-spacer />
        <v-btn
          variant="text" 
          @click="handleCancel"
        >
          取消
        </v-btn>
        <v-btn
          color="primary"
          variant="elevated"
          @click="handleSubmit"
          :disabled="!isFormValid"
          prepend-icon="mdi-check"
        >
          {{ isEditing ? '更新渠道' : '创建渠道' }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onUnmounted } from 'vue'
import { useTheme } from 'vuetify'
import type { Channel } from '../services/api'

interface Props {
  show: boolean
  channel?: Channel | null
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  save: [channel: Omit<Channel, 'index' | 'latency' | 'status'>]
}>()

// 主题
const theme = useTheme()

// 表单引用
const formRef = ref()

// 服务类型选项
const serviceTypeOptions = [
  { title: 'OpenAI (新版API)', value: 'openai' },
  { title: 'OpenAI (兼容旧版)', value: 'openaiold' },
  { title: 'Claude', value: 'claude' },
  { title: 'Gemini', value: 'gemini' }
]

// 源模型选项 (Claude模型的常用别名)
const sourceModelOptions = [
  { title: 'opus', value: 'opus' },
  { title: 'sonnet', value: 'sonnet' },
  { title: 'haiku', value: 'haiku' }
]

// 表单数据
const form = reactive({
  name: '',
  serviceType: '' as 'openai' | 'openaiold' | 'gemini' | 'claude' | '',
  baseUrl: '',
  website: '',
  insecureSkipVerify: false,
  description: '',
  apiKeys: [] as string[],
  modelMapping: {} as Record<string, string>
})

// 原始密钥映射 (掩码密钥 -> 原始密钥)
const originalKeyMap = ref<Map<string, string>>(new Map())

// 新API密钥输入
const newApiKey = ref('')

// 密钥重复检测状态
const apiKeyError = ref('')
const duplicateKeyIndex = ref(-1)

// 复制功能相关状态
const copiedKeyIndex = ref<number | null>(null)

// 新模型映射输入
const newMapping = reactive({
  source: '',
  target: ''
})

// 表单验证错误
const errors = reactive({
  name: '',
  serviceType: '',
  baseUrl: '',
  website: ''
})

// 验证规则
const rules = {
  required: (value: string) => !!value || '此字段为必填项',
  url: (value: string) => {
    try {
      new URL(value)
      return true
    } catch {
      return '请输入有效的URL'
    }
  },
  urlOptional: (value: string) => {
    if (!value) return true
    try { new URL(value); return true } catch { return '请输入有效的URL' }
  }
}

// 计算属性
const isEditing = computed(() => !!props.channel)

// 动态header样式
const headerClasses = computed(() => {
  const isDark = theme.global.current.value.dark
  // Dark: keep neutral surface header; Light: use brand primary header
  return isDark ? 'bg-surface text-high-emphasis' : 'bg-primary text-white'
})

const avatarColor = computed(() => 'primary')

// Use Vuetify theme "on-primary" token so icon isn't fixed white
const headerIconStyle = computed(() => ({
  color: 'rgb(var(--v-theme-on-primary))'
}))

const subtitleClasses = computed(() => 'text-medium-emphasis')

const isFormValid = computed(() => {
  return form.name.trim() && 
         form.serviceType && 
         form.baseUrl.trim() && 
         isValidUrl(form.baseUrl)
})

// 工具函数
const isValidUrl = (url: string): boolean => {
  try {
    new URL(url)
    return true
  } catch {
    return false
  }
}

const getUrlHint = (): string => {
  const hints: Record<string, string> = {
    'openai': '通常为：https://api.openai.com/v1',
    'openaiold': '通常为：https://api.openai.com/v1',
    'claude': '通常为：https://api.anthropic.com',
    'gemini': '通常为：https://generativelanguage.googleapis.com/v1'
  }
  return hints[form.serviceType] || '请输入完整的API基础URL'
}

const maskApiKey = (key: string): string => {
  if (key.length <= 10) return key.slice(0, 3) + '***' + key.slice(-2)
  return key.slice(0, 8) + '***' + key.slice(-5)
}

// 表单操作
const resetForm = () => {
  form.name = ''
  form.serviceType = ''
  form.baseUrl = ''
  form.website = ''
  form.insecureSkipVerify = false
  form.description = ''
  form.apiKeys = []
  form.modelMapping = {}
  newApiKey.value = ''
  newMapping.source = ''
  newMapping.target = ''
  
  // 清空原始密钥映射
  originalKeyMap.value.clear()
  
  // 清空密钥错误状态
  apiKeyError.value = ''
  duplicateKeyIndex.value = -1
  
  // 清除错误信息
  errors.name = ''
  errors.serviceType = ''
  errors.baseUrl = ''
}

const loadChannelData = (channel: Channel) => {
  form.name = channel.name
  form.serviceType = channel.serviceType
  form.baseUrl = channel.baseUrl
  form.website = channel.website || ''
  form.insecureSkipVerify = !!channel.insecureSkipVerify
  form.description = channel.description || ''
  
  // 直接存储原始密钥，不需要映射关系
  form.apiKeys = [...channel.apiKeys]
  
  // 清空原始密钥映射（现在不需要了）
  originalKeyMap.value.clear()
  
  form.modelMapping = { ...(channel.modelMapping || {}) }
}

const addApiKey = () => {
  const key = newApiKey.value.trim()
  if (!key) return

  // 重置错误状态
  apiKeyError.value = ''
  duplicateKeyIndex.value = -1

  // 检查是否与现有密钥重复
  const duplicateIndex = findDuplicateKeyIndex(key)
  if (duplicateIndex !== -1) {
    apiKeyError.value = '该密钥已存在'
    duplicateKeyIndex.value = duplicateIndex
    // 清除输入框，让用户重新输入
    newApiKey.value = ''
    return
  }

  // 直接存储原始密钥
  form.apiKeys.push(key)
  newApiKey.value = ''
}

// 检查密钥是否重复，返回重复密钥的索引，如果没有重复返回-1
const findDuplicateKeyIndex = (newKey: string): number => {
  return form.apiKeys.findIndex(existingKey => existingKey === newKey)
}

const removeApiKey = (index: number) => {
  form.apiKeys.splice(index, 1)

  // 如果删除的是当前高亮的重复密钥，清除高亮状态
  if (duplicateKeyIndex.value === index) {
    duplicateKeyIndex.value = -1
    apiKeyError.value = ''
  } else if (duplicateKeyIndex.value > index) {
    // 如果删除的密钥在高亮密钥之前，调整高亮索引
    duplicateKeyIndex.value--
  }
}

// 复制API密钥到剪贴板
const copyApiKey = async (key: string, index: number) => {
  try {
    await navigator.clipboard.writeText(key)
    copiedKeyIndex.value = index

    // 2秒后重置复制状态
    setTimeout(() => {
      copiedKeyIndex.value = null
    }, 2000)
  } catch (err) {
    console.error('复制密钥失败:', err)
    // 降级方案：使用传统的复制方法
    const textArea = document.createElement('textarea')
    textArea.value = key
    textArea.style.position = 'fixed'
    textArea.style.left = '-999999px'
    textArea.style.top = '-999999px'
    document.body.appendChild(textArea)
    textArea.focus()
    textArea.select()

    try {
      document.execCommand('copy')
      copiedKeyIndex.value = index

      setTimeout(() => {
        copiedKeyIndex.value = null
      }, 2000)
    } catch (err) {
      console.error('降级复制方案也失败:', err)
    } finally {
      textArea.remove()
    }
  }
}

const addModelMapping = () => {
  const source = newMapping.source.trim()
  const target = newMapping.target.trim()
  if (source && target && !form.modelMapping[source]) {
    form.modelMapping[source] = target
    newMapping.source = ''
    newMapping.target = ''
  }
}

const removeModelMapping = (source: string) => {
  delete form.modelMapping[source]
}

const handleSubmit = async () => {
  if (!formRef.value) return
  
  const { valid } = await formRef.value.validate()
  if (!valid) return
  
  // 直接使用原始密钥，不需要转换
  const processedApiKeys = form.apiKeys.filter(key => key.trim())
  
  // 类型断言，因为表单验证已经确保serviceType不为空
  const channelData = {
    name: form.name.trim(),
    serviceType: form.serviceType as 'openai' | 'openaiold' | 'gemini' | 'claude',
    baseUrl: form.baseUrl.trim().replace(/\/$/, ''), // 移除末尾斜杠
    website: form.website.trim() || undefined,
    insecureSkipVerify: form.insecureSkipVerify || undefined,
    description: form.description.trim(),
    apiKeys: processedApiKeys,
    modelMapping: form.modelMapping
  }
  
  emit('save', channelData)
}

const handleCancel = () => {
  emit('update:show', false)
  resetForm()
}

// 监听props变化
watch(() => props.show, (newShow) => {
  if (newShow) {
    if (props.channel) {
      loadChannelData(props.channel)
    } else {
      resetForm()
    }
  }
})

watch(() => props.channel, (newChannel) => {
  if (newChannel && props.show) {
    loadChannelData(newChannel)
  }
})

// ESC键监听
const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && props.show) {
    handleCancel()
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.animate-pulse {
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.7;
  }
}
</style>
