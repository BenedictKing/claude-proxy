<template>
  <v-dialog :model-value="show" @update:model-value="$emit('update:show', $event)" max-width="800" persistent>
    <v-card rounded="lg">
      <v-card-title class="d-flex align-center ga-3 pa-6" :class="headerClasses">
        <v-avatar :color="avatarColor" variant="flat" size="40">
          <v-icon :style="headerIconStyle" size="20">{{ isEditing ? 'mdi-pencil' : 'mdi-plus' }}</v-icon>
        </v-avatar>
        <div class="flex-grow-1">
          <div class="text-h5 font-weight-bold">
            {{ isEditing ? '编辑渠道' : '添加新渠道' }}
          </div>
          <div class="text-body-2" :class="subtitleClasses">
            {{ isEditing ? '修改渠道配置信息' : isQuickMode ? '快速批量添加 API 密钥' : '配置API渠道信息和密钥' }}
          </div>
        </div>
        <!-- 模式切换按钮（仅在添加模式显示） -->
        <v-btn v-if="!isEditing" variant="outlined" size="small" @click="toggleMode" class="mode-toggle-btn">
          <v-icon start size="16">{{ isQuickMode ? 'mdi-form-textbox' : 'mdi-lightning-bolt' }}</v-icon>
          {{ isQuickMode ? '详细配置' : '快速添加' }}
        </v-btn>
      </v-card-title>

      <v-card-text class="pa-6">
        <!-- 快速添加模式 -->
        <div v-if="!isEditing && isQuickMode">
          <v-textarea
            v-model="quickInput"
            label="输入内容"
            placeholder="每行输入一个 API Key 或 Base URL&#10;&#10;示例:&#10;sk-xxx-your-api-key&#10;sk-yyy-another-key&#10;https://api.example.com/v1"
            variant="outlined"
            rows="10"
            no-resize
            autofocus
            class="quick-input-textarea"
            @input="parseQuickInput"
          />

          <!-- 检测状态提示 -->
          <v-card variant="outlined" class="mt-4 detection-status-card" rounded="lg">
            <v-card-text class="pa-4">
              <div class="d-flex flex-column ga-3">
                <!-- Base URL 检测 -->
                <div class="d-flex align-center ga-3">
                  <v-icon :color="detectedBaseUrl ? 'success' : 'error'" size="20">
                    {{ detectedBaseUrl ? 'mdi-check-circle' : 'mdi-alert-circle' }}
                  </v-icon>
                  <div class="flex-grow-1">
                    <div class="text-body-2 font-weight-medium">Base URL</div>
                    <div class="text-caption" :class="detectedBaseUrl ? 'text-success' : 'text-error'">
                      {{ detectedBaseUrl || '请输入一个有效的 URL (https://...)' }}
                    </div>
                    <div v-if="detectedBaseUrl" class="text-caption text-medium-emphasis mt-1">
                      预期请求: {{ expectedRequestUrl }}
                    </div>
                  </div>
                  <v-chip v-if="detectedBaseUrl" size="x-small" color="success" variant="tonal"> 已检测 </v-chip>
                </div>

                <!-- API Keys 检测 -->
                <div class="d-flex align-center ga-3">
                  <v-icon :color="detectedApiKeys.length > 0 ? 'success' : 'error'" size="20">
                    {{ detectedApiKeys.length > 0 ? 'mdi-check-circle' : 'mdi-alert-circle' }}
                  </v-icon>
                  <div class="flex-grow-1">
                    <div class="text-body-2 font-weight-medium">API 密钥</div>
                    <div class="text-caption" :class="detectedApiKeys.length > 0 ? 'text-success' : 'text-error'">
                      {{
                        detectedApiKeys.length > 0
                          ? `已检测到 ${detectedApiKeys.length} 个密钥`
                          : '请至少输入一个 API Key'
                      }}
                    </div>
                  </div>
                  <v-chip v-if="detectedApiKeys.length > 0" size="x-small" color="success" variant="tonal">
                    {{ detectedApiKeys.length }} 个
                  </v-chip>
                </div>

                <!-- 渠道名称预览 -->
                <div class="d-flex align-center ga-3">
                  <v-icon color="primary" size="20">mdi-tag</v-icon>
                  <div class="flex-grow-1">
                    <div class="text-body-2 font-weight-medium">渠道名称</div>
                    <div class="text-caption text-primary font-weight-medium">
                      {{ generatedChannelName }}
                    </div>
                  </div>
                  <v-chip size="x-small" color="primary" variant="tonal"> 自动生成 </v-chip>
                </div>

                <!-- 渠道类型提示 -->
                <div class="d-flex align-center ga-3">
                  <v-icon color="info" size="20">mdi-information</v-icon>
                  <div class="flex-grow-1">
                    <div class="text-body-2 font-weight-medium">渠道类型</div>
                    <div class="text-caption text-medium-emphasis">
                      {{ props.channelType === 'responses' ? 'Responses (Codex)' : 'Claude (Messages)' }} -
                      {{ getDefaultServiceType() }}
                    </div>
                  </div>
                </div>
              </div>
            </v-card-text>
          </v-card>
        </div>

        <!-- 详细表单模式（原有表单） -->
        <v-form v-else ref="formRef" @submit.prevent="handleSubmit">
          <v-row>
            <!-- 基本信息 -->
            <v-col cols="12" md="6">
              <v-text-field
                v-model="form.name"
                label="渠道名称 *"
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
                label="服务类型 *"
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
                label="基础URL *"
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
                    <div class="text-caption text-medium-emphasis">
                      仅在自签名或域名不匹配时临时启用，生产环境请关闭
                    </div>
                  </div>
                </div>
                <v-switch inset color="warning" hide-details v-model="form.insecureSkipVerify" />
              </div>
            </v-col>

            <!-- 描述 -->
            <v-col cols="12">
              <v-textarea
                v-model="form.description"
                label="描述 (可选)"
                hint="可选的渠道描述..."
                persistent-hint
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
                  <v-chip size="small" color="secondary" variant="tonal"> 自动转换模型名称 </v-chip>
                </v-card-title>

                <v-card-text class="pt-2">
                  <div class="text-body-2 text-medium-emphasis mb-4">
                    {{ modelMappingHint }}
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
                          <v-btn size="small" color="error" icon variant="text" @click="removeModelMapping(source)">
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
                      :placeholder="targetModelPlaceholder"
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
              <v-card variant="outlined" rounded="lg" :color="form.apiKeys.length === 0 ? 'error' : undefined">
                <v-card-title class="d-flex align-center justify-space-between pa-4 pb-2">
                  <div class="d-flex align-center ga-2">
                    <v-icon :color="form.apiKeys.length > 0 ? 'primary' : 'error'">mdi-key</v-icon>
                    <span class="text-body-1 font-weight-bold">API密钥管理 *</span>
                    <v-chip v-if="form.apiKeys.length === 0" size="x-small" color="error" variant="tonal">
                      至少需要一个密钥
                    </v-chip>
                  </div>
                  <v-chip size="small" color="info" variant="tonal"> 可添加多个密钥用于负载均衡 </v-chip>
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
                          <v-icon size="small" :color="duplicateKeyIndex === index ? 'error' : 'primary'">
                            {{ duplicateKeyIndex === index ? 'mdi-alert' : 'mdi-key' }}
                          </v-icon>
                        </template>

                        <v-list-item-title>
                          <div class="d-flex align-center justify-space-between">
                            <code class="text-caption">{{ maskApiKey(key) }}</code>
                            <v-chip v-if="duplicateKeyIndex === index" size="x-small" color="error" variant="text">
                              重复密钥
                            </v-chip>
                          </div>
                        </v-list-item-title>

                        <template v-slot:append>
                          <div class="d-flex align-center ga-1">
                            <!-- 置顶/置底：仅首尾密钥显示 -->
                            <v-tooltip
                              v-if="index === form.apiKeys.length - 1 && form.apiKeys.length > 1"
                              text="置顶"
                              location="top"
                              :open-delay="150"
                              content-class="key-tooltip"
                            >
                              <template #activator="{ props: tooltipProps }">
                                <v-btn
                                  v-bind="tooltipProps"
                                  size="small"
                                  color="warning"
                                  icon
                                  variant="text"
                                  rounded="md"
                                  @click="moveApiKeyToTop(index)"
                                >
                                  <v-icon size="small">mdi-arrow-up-bold</v-icon>
                                </v-btn>
                              </template>
                            </v-tooltip>
                            <v-tooltip
                              v-if="index === 0 && form.apiKeys.length > 1"
                              text="置底"
                              location="top"
                              :open-delay="150"
                              content-class="key-tooltip"
                            >
                              <template #activator="{ props: tooltipProps }">
                                <v-btn
                                  v-bind="tooltipProps"
                                  size="small"
                                  color="warning"
                                  icon
                                  variant="text"
                                  rounded="md"
                                  @click="moveApiKeyToBottom(index)"
                                >
                                  <v-icon size="small">mdi-arrow-down-bold</v-icon>
                                </v-btn>
                              </template>
                            </v-tooltip>
                            <v-tooltip
                              :text="copiedKeyIndex === index ? '已复制!' : '复制密钥'"
                              location="top"
                              :open-delay="150"
                              content-class="key-tooltip"
                            >
                              <template #activator="{ props: tooltipProps }">
                                <v-btn
                                  v-bind="tooltipProps"
                                  size="small"
                                  :color="copiedKeyIndex === index ? 'success' : 'primary'"
                                  icon
                                  variant="text"
                                  @click="copyApiKey(key, index)"
                                >
                                  <v-icon size="small">{{
                                    copiedKeyIndex === index ? 'mdi-check' : 'mdi-content-copy'
                                  }}</v-icon>
                                </v-btn>
                              </template>
                            </v-tooltip>
                            <v-tooltip text="删除密钥" location="top" :open-delay="150" content-class="key-tooltip">
                              <template #activator="{ props: tooltipProps }">
                                <v-btn
                                  v-bind="tooltipProps"
                                  size="small"
                                  color="error"
                                  icon
                                  variant="text"
                                  @click="removeApiKey(index)"
                                >
                                  <v-icon size="small" color="error">mdi-close</v-icon>
                                </v-btn>
                              </template>
                            </v-tooltip>
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
                      @input="handleApiKeyInput"
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
        <v-btn variant="text" @click="handleCancel"> 取消 </v-btn>
        <v-btn
          v-if="!isEditing && isQuickMode"
          color="primary"
          variant="elevated"
          @click="handleQuickSubmit"
          :disabled="!isQuickFormValid"
          prepend-icon="mdi-check"
        >
          创建渠道
        </v-btn>
        <v-btn
          v-else
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
  channelType?: 'messages' | 'responses'
}

const props = withDefaults(defineProps<Props>(), {
  channelType: 'messages'
})

const emit = defineEmits<{
  'update:show': [value: boolean]
  save: [channel: Omit<Channel, 'index' | 'latency' | 'status'>, options?: { isQuickAdd?: boolean }]
}>()

// 主题
const theme = useTheme()

// 表单引用
const formRef = ref()

// 模式切换: 快速添加 vs 详细表单
const isQuickMode = ref(true)

// 快速添加模式的数据
const quickInput = ref('')
const detectedBaseUrl = ref('')
const detectedApiKeys = ref<string[]>([])

// 切换模式时，将快速模式检测到的值同步到详细表单，但不清空快速模式输入
const toggleMode = () => {
  if (isQuickMode.value) {
    // 从快速模式切换到详细模式：始终用检测到的值覆盖表单
    if (detectedBaseUrl.value) {
      form.baseUrl = detectedBaseUrl.value
    }
    if (detectedApiKeys.value.length > 0) {
      form.apiKeys = [...detectedApiKeys.value]
    }
    if (generatedChannelName.value) {
      form.name = generatedChannelName.value
    }
    form.serviceType = getDefaultServiceTypeValue()
  }
  // 切换回快速模式时不做任何清理，保留 quickInput 原有内容
  isQuickMode.value = !isQuickMode.value
}

// 检测单个 token 是否为有效的 API Key
const isValidApiKey = (token: string): boolean => {
  // 常见 API Key 前缀格式
  if (/^(sk-|cr_|ms-|key-|api-|AIza)/i.test(token)) {
    return true
  }
  // 长度足够且只包含合法字符的也可能是 key
  if (token.length >= 32 && /^[a-zA-Z0-9_-]+$/.test(token)) {
    return true
  }
  return false
}

// 解析快速输入内容
const parseQuickInput = () => {
  // 统一按换行、空格、逗号、分号分割，然后 trim
  const tokens = quickInput.value
    .split(/[\n\s,;]+/)
    .map(t => t.trim())
    .filter(t => t.length > 0)

  // 重置检测结果
  detectedBaseUrl.value = ''
  detectedApiKeys.value = []

  for (const token of tokens) {
    // 检测 URL (http:// 或 https:// 开头)
    if (/^https?:\/\//i.test(token)) {
      // 只取第一个检测到的 URL
      if (!detectedBaseUrl.value) {
        // 保留 # 结尾（用于跳过自动添加 /v1），但移除末尾斜杠
        const endsWithHash = token.endsWith('#')
        let url = endsWithHash ? token.slice(0, -1) : token
        url = url.replace(/\/$/, '')
        detectedBaseUrl.value = endsWithHash ? url + '#' : url
      }
      continue
    }

    // 检测 API Key
    if (isValidApiKey(token) && !detectedApiKeys.value.includes(token)) {
      detectedApiKeys.value.push(token)
    }
  }
}

// 获取默认服务类型
const getDefaultServiceType = (): string => {
  if (props.channelType === 'responses') {
    return 'Responses (原生接口)'
  }
  return 'Claude'
}

// 获取默认服务类型值
const getDefaultServiceTypeValue = (): 'openai' | 'gemini' | 'claude' | 'responses' => {
  if (props.channelType === 'responses') {
    return 'responses'
  }
  return 'claude'
}

// 获取默认 Base URL
const getDefaultBaseUrl = (): string => {
  if (props.channelType === 'responses') {
    return 'https://api.openai.com/v1'
  }
  return 'https://api.anthropic.com'
}

// 快速模式表单验证
const isQuickFormValid = computed(() => {
  return detectedBaseUrl.value.length > 0 && detectedApiKeys.value.length > 0
})

// 生成随机字符串
const generateRandomString = (length: number): string => {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
  let result = ''
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return result
}

// 从 URL 提取二级域名
const extractDomain = (url: string): string => {
  try {
    const hostname = new URL(url).hostname
    // 移除 www. 前缀
    const cleanHost = hostname.replace(/^www\./, '')
    const parts = cleanHost.split('.')

    // 处理特殊情况
    if (parts.length <= 1) {
      // localhost 等单段域名
      return cleanHost
    } else if (parts.length === 2) {
      // example.com → example
      return parts[0]
    } else {
      // api.openai.com → openai (取倒数第二段)
      return parts[parts.length - 2]
    }
  } catch {
    return 'channel'
  }
}

// 随机后缀和生成的渠道名称
const randomSuffix = ref(generateRandomString(6))

const generatedChannelName = computed(() => {
  if (!detectedBaseUrl.value) {
    return `channel-${randomSuffix.value}`
  }
  const domain = extractDomain(detectedBaseUrl.value)
  return `${domain}-${randomSuffix.value}`
})

// 预期请求 URL（模拟后端逻辑）
const expectedRequestUrl = computed(() => {
  if (!detectedBaseUrl.value) return ''

  let baseUrl = detectedBaseUrl.value
  const skipVersion = baseUrl.endsWith('#')
  if (skipVersion) {
    baseUrl = baseUrl.slice(0, -1)
  }

  // 检查是否已包含版本号
  const hasVersion = /\/v\d+[a-z]*$/.test(baseUrl)

  // 根据渠道类型和服务类型确定端点（与后端逻辑一致）
  const serviceType = getDefaultServiceTypeValue()
  let endpoint = ''
  if (props.channelType === 'responses') {
    // responses 渠道根据 serviceType 决定端点
    if (serviceType === 'responses') {
      endpoint = '/responses'
    } else if (serviceType === 'claude') {
      endpoint = '/messages'
    } else {
      endpoint = '/chat/completions'
    }
  } else {
    // messages 渠道：claude 用 /messages，其他用 /chat/completions
    endpoint = '/messages' // 快速添加默认是 claude
  }

  if (hasVersion || skipVersion) {
    return baseUrl + endpoint
  }
  return baseUrl + '/v1' + endpoint
})

// 处理快速添加提交
const handleQuickSubmit = () => {
  if (!isQuickFormValid.value) return

  const channelData = {
    name: generatedChannelName.value,
    serviceType: getDefaultServiceTypeValue(),
    baseUrl: detectedBaseUrl.value,
    apiKeys: detectedApiKeys.value,
    modelMapping: {}
  }

  // 传递 isQuickAdd 标志，让 App.vue 知道需要进行后续处理
  emit('save', channelData, { isQuickAdd: true })
}

// 服务类型选项 - 根据渠道类型动态显示
const serviceTypeOptions = computed(() => {
  if (props.channelType === 'responses') {
    return [
      { title: 'Responses (原生接口)', value: 'responses' },
      { title: 'OpenAI', value: 'openai' },
      { title: 'Claude', value: 'claude' }
    ]
  } else {
    return [
      { title: 'OpenAI', value: 'openai' },
      { title: 'Claude', value: 'claude' },
      { title: 'Gemini', value: 'gemini' }
    ]
  }
})

// 全部源模型选项 - 根据渠道类型动态显示
const allSourceModelOptions = computed(() => {
  if (props.channelType === 'responses') {
    // Responses API (Codex) 常用模型名称
    return [
      { title: 'codex', value: 'codex' },
      { title: 'gpt-5.2-codex', value: 'gpt-5.2-codex' },
      { title: 'gpt-5.1-codex-max', value: 'gpt-5.1-codex-max' },
      { title: 'gpt-5.1-codex', value: 'gpt-5.1-codex' },
      { title: 'gpt-5.1-codex-mini', value: 'gpt-5.1-codex-mini' },
      { title: 'gpt-5.2', value: 'gpt-5.2' },
      { title: 'gpt-5.1', value: 'gpt-5.1' },
      { title: 'gpt-5', value: 'gpt-5' }
    ]
  } else {
    // Messages API (Claude) 常用模型别名
    return [
      { title: 'opus', value: 'opus' },
      { title: 'sonnet', value: 'sonnet' },
      { title: 'haiku', value: 'haiku' }
    ]
  }
})

// 可选的源模型选项 - 过滤掉已配置的模型
const sourceModelOptions = computed(() => {
  const configuredModels = Object.keys(form.modelMapping)
  return allSourceModelOptions.value.filter(opt => !configuredModels.includes(opt.value))
})

// 模型重定向的示例文本 - 根据渠道类型动态显示
const modelMappingHint = computed(() => {
  if (props.channelType === 'responses') {
    return '配置模型名称映射，将请求中的模型名重定向到目标模型。例如：将 "o3" 重定向到 "gpt-5.1-codex-max"'
  } else {
    return '配置模型名称映射，将请求中的模型名重定向到目标模型。例如：将 "opus" 重定向到 "claude-3-5-sonnet"'
  }
})

const targetModelPlaceholder = computed(() => {
  if (props.channelType === 'responses') {
    return '例如：gpt-5.1-codex-max'
  } else {
    return '例如：claude-3-5-sonnet'
  }
})

// 表单数据
const form = reactive({
  name: '',
  serviceType: '' as 'openai' | 'gemini' | 'claude' | 'responses' | '',
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

// 处理 API 密钥输入事件
const handleApiKeyInput = () => {
  apiKeyError.value = ''
  duplicateKeyIndex.value = -1
}

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
    try {
      new URL(value)
      return true
    } catch {
      return '请输入有效的URL'
    }
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

const subtitleClasses = computed(() => {
  const isDark = theme.global.current.value.dark
  // Dark mode: use medium emphasis; Light mode: use white with opacity for primary bg
  return isDark ? 'text-medium-emphasis' : 'text-white-subtitle'
})

const isFormValid = computed(() => {
  return (
    form.name.trim() && form.serviceType && form.baseUrl.trim() && isValidUrl(form.baseUrl) && form.apiKeys.length > 0
  )
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
    responses: '通常为：https://api.openai.com/v1',
    openai: '通常为：https://api.openai.com/v1',
    claude: '通常为：https://api.anthropic.com',
    gemini: '通常为：https://generativelanguage.googleapis.com/v1'
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

  // 重置快速添加模式数据
  quickInput.value = ''
  detectedBaseUrl.value = ''
  detectedApiKeys.value = []
  randomSuffix.value = generateRandomString(6)
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

// 将指定密钥移到最上方
const moveApiKeyToTop = (index: number) => {
  if (index <= 0 || index >= form.apiKeys.length) return
  const [key] = form.apiKeys.splice(index, 1)
  form.apiKeys.unshift(key)
  duplicateKeyIndex.value = -1
  copiedKeyIndex.value = null
}

// 将指定密钥移到最下方
const moveApiKeyToBottom = (index: number) => {
  if (index < 0 || index >= form.apiKeys.length - 1) return
  const [key] = form.apiKeys.splice(index, 1)
  form.apiKeys.push(key)
  duplicateKeyIndex.value = -1
  copiedKeyIndex.value = null
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
    serviceType: form.serviceType as 'openai' | 'gemini' | 'claude' | 'responses',
    baseUrl: form.baseUrl.trim().replace(/\/$/, ''), // 移除末尾斜杠
    website: form.website.trim() || undefined,
    insecureSkipVerify: form.insecureSkipVerify,
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
watch(
  () => props.show,
  newShow => {
    if (newShow) {
      // 无论是编辑还是新增，都先清理密钥错误状态
      apiKeyError.value = ''
      duplicateKeyIndex.value = -1

      if (props.channel) {
        // 编辑模式：使用表单模式
        isQuickMode.value = false
        loadChannelData(props.channel)
      } else {
        // 添加模式：默认使用快速模式
        isQuickMode.value = true
        resetForm()
      }
    }
  }
)

watch(
  () => props.channel,
  newChannel => {
    if (newChannel && props.show) {
      loadChannelData(newChannel)
    }
  }
)

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
/* 浅色模式下副标题使用白色带透明度 */
.text-white-subtitle {
  color: rgba(255, 255, 255, 0.85) !important;
}

.animate-pulse {
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.7;
  }
}

:deep(.key-tooltip) {
  color: rgba(var(--v-theme-on-surface), 0.92);
  background-color: rgba(var(--v-theme-surface), 0.98);
  border: 1px solid rgba(var(--v-theme-primary), 0.45);
  font-weight: 600;
  letter-spacing: 0.2px;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.06);
}

/* 快速添加模式样式 */
.quick-input-textarea :deep(textarea) {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 13px;
  line-height: 1.6;
}

.detection-status-card {
  background: rgba(var(--v-theme-surface-variant), 0.3);
}

.mode-toggle-btn {
  text-transform: none;
}

/* 亮色模式下按钮在 primary 背景上显示白色 */
.bg-primary .mode-toggle-btn {
  color: white !important;
  border-color: rgba(255, 255, 255, 0.7) !important;
}

.bg-primary .mode-toggle-btn:hover {
  background-color: rgba(255, 255, 255, 0.15) !important;
  border-color: white !important;
}
</style>
