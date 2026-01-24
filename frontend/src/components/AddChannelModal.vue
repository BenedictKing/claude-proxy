<template>
  <v-dialog :model-value="show" max-width="800" persistent @update:model-value="$emit('update:show', $event)">
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
        <v-btn v-if="!isEditing" variant="outlined" size="small" class="mode-toggle-btn" @click="toggleMode">
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
                <div class="d-flex align-start ga-3">
                  <v-icon :color="detectedBaseUrls.length > 0 ? 'success' : 'error'" size="20" class="mt-1">
                    {{ detectedBaseUrls.length > 0 ? 'mdi-check-circle' : 'mdi-alert-circle' }}
                  </v-icon>
                  <div class="flex-grow-1">
                    <div class="text-body-2 font-weight-medium">Base URL</div>
                    <div v-if="detectedBaseUrls.length === 0" class="text-caption text-error">
                      请输入一个有效的 URL (https://...)
                    </div>
                    <div v-else class="d-flex flex-column ga-2 mt-1">
                      <div v-for="url in detectedBaseUrls" :key="url" class="base-url-item">
                        <div class="text-caption text-success">{{ url }}</div>
                        <div class="text-caption text-medium-emphasis">预期请求: {{ getExpectedRequestUrl(url) }}</div>
                      </div>
                    </div>
                  </div>
                  <v-chip v-if="detectedBaseUrls.length > 0" size="x-small" color="success" variant="tonal">
                    {{ detectedBaseUrls.length }} 个
                  </v-chip>
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
                      {{ props.channelType === 'gemini' ? 'Gemini' : props.channelType === 'responses' ? 'Responses (Codex)' : 'Claude (Messages)' }} -
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
              <v-textarea
                v-model="baseUrlsText"
                label="基础URL *"
                placeholder="每行一个 URL，支持多个 BaseURL&#10;例如：&#10;https://api.openai.com/v1&#10;https://api2.openai.com/v1"
                prepend-inner-icon="mdi-web"
                variant="outlined"
                density="comfortable"
                rows="3"
                no-resize
                :rules="[rules.required, rules.baseUrls]"
                required
                :error-messages="errors.baseUrl"
                hide-details="auto"
              />
              <!-- 固定高度的提示区域，防止布局跳动；有错误时不显示 -->
              <div v-show="formExpectedRequestUrls.length > 0 && !baseUrlHasError" class="base-url-hint">
                <div v-for="(item, index) in formExpectedRequestUrls" :key="index" class="expected-request-item">
                  <span class="text-caption text-medium-emphasis"> 预期请求: {{ item.expectedUrl }} </span>
                </div>
              </div>
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

            <!-- 模型重定向配置 -->
            <v-col v-if="form.serviceType" cols="12">
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
                    <br/>
                    <span class="text-caption text-primary">💡 点击目标模型输入框会自动获取上游支持的模型列表,每个 API Key 的检测状态会显示在密钥列表中</span>
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
                        <template #prepend>
                          <v-icon size="small" color="primary">mdi-arrow-right</v-icon>
                        </template>

                        <v-list-item-title>
                          <div class="d-flex align-center ga-2">
                            <code class="text-caption">{{ source }}</code>
                            <v-icon size="small" color="primary">mdi-arrow-right</v-icon>
                            <code class="text-caption">{{ target }}</code>
                          </div>
                        </v-list-item-title>

                        <template #append>
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
                    <v-combobox
                      v-model="newMapping.target"
                      label="目标模型名"
                      :placeholder="targetModelPlaceholder"
                      :items="targetModelOptions"
                      :loading="fetchingModels"
                      variant="outlined"
                      density="comfortable"
                      hide-details
                      class="flex-1-1"
                      clearable
                      @focus="handleTargetModelClick"
                      @keyup.enter="addModelMapping"
                    />
                    <v-btn
                      color="secondary"
                      variant="elevated"
                      :disabled="!isMappingInputValid"
                      @click="addModelMapping"
                    >
                      添加
                    </v-btn>
                  </div>
                  <!-- 错误提示 -->
                  <div v-if="fetchModelsError" class="text-error text-caption mt-2">
                    {{ fetchModelsError }}
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
                        <template #prepend>
                          <v-icon size="small" :color="duplicateKeyIndex === index ? 'error' : 'primary'">
                            {{ duplicateKeyIndex === index ? 'mdi-alert' : 'mdi-key' }}
                          </v-icon>
                        </template>

                        <v-list-item-title>
                          <div class="d-flex align-center justify-space-between">
                            <code class="text-caption">{{ maskApiKey(key) }}</code>
                            <div class="d-flex align-center ga-1">
                              <!-- Models 状态标签 -->
                              <v-chip
                                v-if="keyModelsStatus.get(key)?.loading"
                                size="x-small"
                                color="info"
                                variant="tonal"
                              >
                                <v-icon start size="12">mdi-loading</v-icon>
                                检测中...
                              </v-chip>
                              <v-chip
                                v-else-if="keyModelsStatus.get(key)?.success"
                                size="x-small"
                                color="success"
                                variant="tonal"
                              >
                                models {{ keyModelsStatus.get(key)?.statusCode }} ({{ keyModelsStatus.get(key)?.modelCount }} 个)
                              </v-chip>
                              <v-tooltip
                                v-else-if="keyModelsStatus.get(key)?.error"
                                :text="keyModelsStatus.get(key)?.error"
                                location="top"
                                max-width="300"
                              >
                                <template #activator="{ props: tooltipProps }">
                                  <v-chip
                                    v-bind="tooltipProps"
                                    size="x-small"
                                    color="error"
                                    variant="tonal"
                                  >
                                    models {{ keyModelsStatus.get(key)?.statusCode || 'ERR' }}
                                  </v-chip>
                                </template>
                              </v-tooltip>
                              <!-- 重复密钥标签 -->
                              <v-chip v-if="duplicateKeyIndex === index" size="x-small" color="error" variant="text">
                                重复密钥
                              </v-chip>
                            </div>
                          </div>
                        </v-list-item-title>

                        <template #append>
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
                      :error="!!apiKeyError"
                      :error-messages="apiKeyError"
                      class="flex-grow-1"
                      @keyup.enter="addApiKey"
                      @input="handleApiKeyInput"
                    />
                    <v-btn
                      color="primary"
                      variant="elevated"
                      size="large"
                      height="40"
                      :disabled="!newApiKey.trim()"
                      class="mt-1"
                      @click="addApiKey"
                    >
                      添加
                    </v-btn>
                  </div>
                </v-card-text>
              </v-card>
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
                <v-switch v-model="form.insecureSkipVerify" inset color="warning" hide-details />
              </div>
            </v-col>

            <!-- 低质量渠道标记 -->
            <v-col cols="12">
              <div class="d-flex align-center justify-space-between">
                <div class="d-flex align-center ga-2">
                  <v-icon color="info">mdi-speedometer-slow</v-icon>
                  <div>
                    <div class="text-body-1 font-weight-medium">低质量渠道</div>
                    <div class="text-caption text-medium-emphasis">
                      启用后强制本地估算 token 数量，偏差超过 5% 时使用本地值
                    </div>
                  </div>
                </div>
                <v-switch v-model="form.lowQuality" inset color="info" hide-details />
              </div>
            </v-col>

            <!-- 注入 Dummy Thought Signature（仅 Gemini 渠道显示） -->
            <v-col v-if="props.channelType === 'gemini'" cols="12">
              <div class="d-flex align-center justify-space-between">
                <div class="d-flex align-center ga-2">
                  <v-icon color="secondary">mdi-signature</v-icon>
                  <div>
                    <div class="text-body-1 font-weight-medium">注入 Dummy Thought Signature</div>
                    <div class="text-caption text-medium-emphasis">
                      为 functionCall 注入 dummy signature，兼容需要该字段的第三方 API（官方 API 请关闭）
                    </div>
                  </div>
                </div>
                <v-switch v-model="form.injectDummyThoughtSignature" inset color="secondary" hide-details />
              </div>
            </v-col>

            <!-- 移除 Thought Signature（仅 Gemini 渠道显示） -->
            <v-col v-if="props.channelType === 'gemini'" cols="12">
              <div class="d-flex align-center justify-space-between">
                <div class="d-flex align-center ga-2">
                  <v-icon color="error">mdi-close-circle</v-icon>
                  <div>
                    <div class="text-body-1 font-weight-medium">移除 Thought Signature</div>
                    <div class="text-caption text-medium-emphasis">
                      移除 functionCall 的 thought_signature 字段，兼容不支持该字段的旧版 Gemini API
                    </div>
                  </div>
                </div>
                <v-switch v-model="form.stripThoughtSignature" inset color="error" hide-details />
              </div>
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
          :disabled="!isQuickFormValid"
          prepend-icon="mdi-check"
          @click="handleQuickSubmit"
        >
          创建渠道
        </v-btn>
        <v-btn
          v-else
          color="primary"
          variant="elevated"
          :disabled="!isFormValid"
          prepend-icon="mdi-check"
          @click="handleSubmit"
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
import { fetchUpstreamModels, ApiError } from '../services/api'
import {
  isValidApiKey as _isValidApiKey,
  isValidUrl as _isValidQuickInputUrl,
  parseQuickInput as parseQuickInputUtil
} from '../utils/quickInputParser'

interface Props {
  show: boolean
  channel?: Channel | null
  channelType?: 'messages' | 'responses' | 'gemini'
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
const detectedBaseUrls = ref<string[]>([])
const detectedApiKeys = ref<string[]>([])
const detectedServiceType = ref<'openai' | 'gemini' | 'claude' | 'responses' | null>(null)

// 详细表单预期请求 URL 预览（防止输入时抖动）
const formBaseUrlPreview = ref('')
let formBaseUrlPreviewTimer: number | null = null

// 切换模式时，将快速模式检测到的值同步到详细表单，但不清空快速模式输入
const toggleMode = () => {
  if (isQuickMode.value) {
    // 从快速模式切换到详细模式：始终用检测到的值覆盖表单
    if (detectedBaseUrls.value.length > 0) {
      // 多个 BaseURL
      form.baseUrl = detectedBaseUrls.value[0]
      form.baseUrls = [...detectedBaseUrls.value]
      baseUrlsText.value = detectedBaseUrls.value.join('\n')
    } else if (detectedBaseUrl.value) {
      // 单个 BaseURL
      form.baseUrl = detectedBaseUrl.value
      form.baseUrls = []
      baseUrlsText.value = detectedBaseUrl.value
    }
    if (detectedApiKeys.value.length > 0) {
      form.apiKeys = [...detectedApiKeys.value]
    }
    if (generatedChannelName.value) {
      form.name = generatedChannelName.value
    }
    form.serviceType = detectedServiceType.value || getDefaultServiceTypeValue()
  }
  // 切换回快速模式时不做任何清理，保留 quickInput 原有内容
  isQuickMode.value = !isQuickMode.value
}

// 解析快速输入内容
const parseQuickInput = () => {
  const result = parseQuickInputUtil(quickInput.value)
  detectedBaseUrl.value = result.detectedBaseUrl
  detectedBaseUrls.value = result.detectedBaseUrls
  detectedApiKeys.value = result.detectedApiKeys
  detectedServiceType.value = result.detectedServiceType
}

// 获取默认服务类型
const getDefaultServiceType = (): string => {
  if (props.channelType === 'gemini') {
    return 'Gemini'
  }
  if (props.channelType === 'responses') {
    return 'Responses (原生接口)'
  }
  return 'Claude'
}

// 获取默认服务类型值
const getDefaultServiceTypeValue = (): 'openai' | 'gemini' | 'claude' | 'responses' => {
  if (props.channelType === 'gemini') {
    return 'gemini'
  }
  if (props.channelType === 'responses') {
    return 'responses'
  }
  return 'claude'
}

// 获取默认 Base URL
const _getDefaultBaseUrl = (): string => {
  if (props.channelType === 'gemini') {
    return 'https://generativelanguage.googleapis.com'
  }
  if (props.channelType === 'responses') {
    return 'https://api.openai.com/v1'
  }
  return 'https://api.anthropic.com'
}

// 快速模式表单验证
const isQuickFormValid = computed(() => {
  return detectedBaseUrls.value.length > 0 && detectedApiKeys.value.length > 0
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
const _expectedRequestUrl = computed(() => {
  if (!detectedBaseUrl.value) return ''

  let baseUrl = detectedBaseUrl.value
  const skipVersion = baseUrl.endsWith('#')
  if (skipVersion) {
    baseUrl = baseUrl.slice(0, -1)
  }

  // 检查是否已包含版本号
  const hasVersion = /\/v\d+[a-z]*$/.test(baseUrl)

  // 根据渠道类型和服务类型确定端点（与后端逻辑一致）
  const serviceType = detectedServiceType.value || getDefaultServiceTypeValue()
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
    // messages 渠道：根据检测到的服务类型决定端点
    if (serviceType === 'claude') {
      endpoint = '/messages'
    } else if (serviceType === 'gemini') {
      endpoint = '/models/{model}:generateContent'
    } else {
      endpoint = '/chat/completions'
    }
  }

  if (hasVersion || skipVersion) {
    return baseUrl + endpoint
  }
  // Gemini 使用 /v1beta，其他使用 /v1
  const versionPrefix = serviceType === 'gemini' ? '/v1beta' : '/v1'
  return baseUrl + versionPrefix + endpoint
})

// 生成单个 URL 的预期请求地址
const getExpectedRequestUrl = (inputBaseUrl: string): string => {
  if (!inputBaseUrl) return ''

  let baseUrl = inputBaseUrl
  const skipVersion = baseUrl.endsWith('#')
  if (skipVersion) {
    baseUrl = baseUrl.slice(0, -1)
  }

  const hasVersion = /\/v\d+[a-z]*$/.test(baseUrl)

  const serviceType = detectedServiceType.value || getDefaultServiceTypeValue()
  let endpoint = ''
  if (props.channelType === 'responses') {
    if (serviceType === 'responses') {
      endpoint = '/responses'
    } else if (serviceType === 'claude') {
      endpoint = '/messages'
    } else {
      endpoint = '/chat/completions'
    }
  } else {
    if (serviceType === 'claude') {
      endpoint = '/messages'
    } else if (serviceType === 'gemini') {
      endpoint = '/models/{model}:generateContent'
    } else {
      endpoint = '/chat/completions'
    }
  }

  if (hasVersion || skipVersion) {
    return baseUrl + endpoint
  }
  // Gemini 使用 /v1beta，其他使用 /v1
  const versionPrefix = serviceType === 'gemini' ? '/v1beta' : '/v1'
  return baseUrl + versionPrefix + endpoint
}

// 检测 baseUrl 是否有验证错误
const baseUrlHasError = computed(() => {
  const value = form.baseUrl
  if (!value) return true
  try {
    new URL(value)
    return false
  } catch {
    return true
  }
})

// 详细模式所有 URL 的预期请求（支持多 BaseURL）
const formExpectedRequestUrls = computed(() => {
  if (!form.serviceType) return []

  // 收集所有 URL
  const urls: string[] = []
  if (form.baseUrls && form.baseUrls.length > 0) {
    urls.push(...form.baseUrls)
  } else if (form.baseUrl) {
    urls.push(form.baseUrl)
  }

  if (urls.length === 0) return []

  // 根据 serviceType 确定端点
  let endpoint = ''
  if (props.channelType === 'responses') {
    if (form.serviceType === 'responses') {
      endpoint = '/responses'
    } else if (form.serviceType === 'claude') {
      endpoint = '/messages'
    } else {
      endpoint = '/chat/completions'
    }
  } else {
    // messages 渠道
    if (form.serviceType === 'claude') {
      endpoint = '/messages'
    } else if (form.serviceType === 'gemini') {
      endpoint = '/models/{model}:generateContent'
    } else {
      endpoint = '/chat/completions'
    }
  }

  // 为每个 URL 生成预期请求
  return urls
    .filter(url => url && isValidUrl(url.replace(/#$/, '')))
    .map(rawUrl => {
      let baseUrl = rawUrl.trim()
      const skipVersion = baseUrl.endsWith('#')
      if (skipVersion) {
        baseUrl = baseUrl.slice(0, -1)
      }
      baseUrl = baseUrl.replace(/\/$/, '')

      const hasVersion = /\/v\d+[a-z]*$/.test(baseUrl)

      // Gemini 使用 /v1beta，其他使用 /v1
      const versionPrefix = form.serviceType === 'gemini' ? '/v1beta' : '/v1'
      const expectedUrl = hasVersion || skipVersion ? baseUrl + endpoint : baseUrl + versionPrefix + endpoint

      return { baseUrl: rawUrl, expectedUrl }
    })
})

// 处理快速添加提交
const handleQuickSubmit = () => {
  if (!isQuickFormValid.value) return

  const channelData = {
    name: generatedChannelName.value,
    serviceType: detectedServiceType.value || getDefaultServiceTypeValue(),
    baseUrl: detectedBaseUrl.value,
    baseUrls: detectedBaseUrls.value,
    apiKeys: detectedApiKeys.value,
    modelMapping: {}
  }

  // 传递 isQuickAdd 标志，让 App.vue 知道需要进行后续处理
  emit('save', channelData, { isQuickAdd: true })
}

// 服务类型选项 - 根据渠道类型动态显示
const serviceTypeOptions = computed(() => {
  if (props.channelType === 'gemini') {
    return [
      { title: 'Gemini', value: 'gemini' },
      { title: 'OpenAI', value: 'openai' },
      { title: 'Claude', value: 'claude' }
    ]
  }
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
  if (props.channelType === 'gemini') {
    // Gemini API 常用模型别名
    return [
      { title: 'gemini-2', value: 'gemini-2' },
      { title: 'gemini-2.5-flash', value: 'gemini-2.5-flash' },
      { title: 'gemini-2.5-flash-lite', value: 'gemini-2.5-flash-lite' },
      { title: 'gemini-2.5-flash-image', value: 'gemini-2.5-flash-image' },
      { title: 'gemini-2.5-flash-preview-tts', value: 'gemini-2.5-flash-preview-tts' },
      { title: 'gemini-2.5-flash-native-audio-preview-12-2025', value: 'gemini-2.5-flash-native-audio-preview-12-2025' },
      { title: 'gemini-2.5-pro', value: 'gemini-2.5-pro' },
      { title: 'gemini-2.5-pro-preview-tts', value: 'gemini-2.5-pro-preview-tts' },
      { title: 'gemini-3-pro-preview', value: 'gemini-3-pro-preview' },
      { title: 'gemini-3-flash-preview', value: 'gemini-3-flash-preview' },
      { title: 'gemini-3-pro-image-preview', value: 'gemini-3-pro-image-preview' }
    ]
  }
  if (props.channelType === 'responses') {
    // Responses API (Codex) 常用模型名称
    return [
      { title: 'codex', value: 'codex' },
      { title: 'gpt-5', value: 'gpt-5' },
      { title: 'gpt-5.2-codex', value: 'gpt-5.2-codex' },
      { title: 'gpt-5.2', value: 'gpt-5.2' },
      { title: 'gpt-5.1-codex-max', value: 'gpt-5.1-codex-max' },
      { title: 'gpt-5.1-codex', value: 'gpt-5.1-codex' },
      { title: 'gpt-5.1-codex-mini', value: 'gpt-5.1-codex-mini' },
      { title: 'gpt-5.1', value: 'gpt-5.1' }
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
  if (props.channelType === 'gemini') {
    return '配置模型名称映射，将请求中的模型名重定向到目标模型。例如：将 "gemini-pro" 重定向到 "gemini-2.0-flash"'
  }
  if (props.channelType === 'responses') {
    return '配置模型名称映射，将请求中的模型名重定向到目标模型。例如：将 "o3" 重定向到 "gpt-5.1-codex-max"'
  } else {
    return '配置模型名称映射，将请求中的模型名重定向到目标模型。例如：将 "opus" 重定向到 "claude-3-5-sonnet"'
  }
})

const targetModelPlaceholder = computed(() => {
  if (props.channelType === 'gemini') {
    return '例如：gemini-2.0-flash'
  }
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
  baseUrls: [] as string[],
  website: '',
  insecureSkipVerify: false,
  lowQuality: false,
  injectDummyThoughtSignature: false,
  stripThoughtSignature: false,
  description: '',
  apiKeys: [] as string[],
  modelMapping: {} as Record<string, string>
})

// 多 BaseURL 文本输入（独立变量，保留用户输入的换行）
const baseUrlsText = ref('')

// 监听 baseUrlsText 变化，同步到 form（仅做基本同步，不修改用户输入）
watch(baseUrlsText, val => {
  const urls = val
    .split('\n')
    .map(s => s.trim())
    .filter(Boolean)
  if (urls.length === 0) {
    form.baseUrl = ''
    form.baseUrls = []
  } else if (urls.length === 1) {
    form.baseUrl = urls[0]
    form.baseUrls = []
  } else {
    form.baseUrl = urls[0]
    form.baseUrls = urls
  }
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

// 安全地获取字符串值（处理 v-select/v-combobox 可能返回对象的情况）
const getStringValue = (val: string | { title: string; value: string } | null | undefined): string => {
  if (!val) return ''
  if (typeof val === 'string') return val
  return val.value || ''
}

// 检查映射输入是否有效
const isMappingInputValid = computed(() => {
  const source = getStringValue(newMapping.source).trim()
  const target = getStringValue(newMapping.target).trim()
  return source && target
})

// 目标模型列表（从上游获取）
const targetModelOptions = ref<Array<{ title: string; value: string }>>([])
const fetchingModels = ref(false)
const fetchModelsError = ref('')
const hasTriedFetchModels = ref(false) // 标记是否已尝试获取过模型列表

// API Key 的 models 状态管理
interface KeyModelsStatus {
  loading: boolean
  success: boolean
  statusCode?: number
  error?: string
  modelCount?: number
}
const keyModelsStatus = ref<Map<string, KeyModelsStatus>>(new Map())

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
  },
  baseUrls: (value: string) => {
    if (!value) return '此字段为必填项'
    const urls = value
      .split('\n')
      .map(s => s.trim())
      .filter(Boolean)
    if (urls.length === 0) return '请至少输入一个 URL'
    for (const url of urls) {
      try {
        new URL(url)
      } catch {
        return `无效的 URL: ${url}`
      }
    }
    return true
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

const maskApiKey = (key: string): string => {
  if (key.length <= 10) return key.slice(0, 3) + '***' + key.slice(-2)
  return key.slice(0, 8) + '***' + key.slice(-5)
}

// 表单操作
const resetForm = () => {
  form.name = ''
  form.serviceType = ''
  form.baseUrl = ''
  form.baseUrls = []
  form.website = ''
  form.insecureSkipVerify = false
  form.lowQuality = false
  form.injectDummyThoughtSignature = false
  form.stripThoughtSignature = false
  form.description = ''
  form.apiKeys = []
  form.modelMapping = {}
  newApiKey.value = ''
  newMapping.source = ''
  newMapping.target = ''

  // 重置 baseUrlsText
  baseUrlsText.value = ''

  // 清空原始密钥映射
  originalKeyMap.value.clear()

  // 清空密钥错误状态
  apiKeyError.value = ''
  duplicateKeyIndex.value = -1

  // 清空模型缓存和状态
  targetModelOptions.value = []
  fetchingModels.value = false
  fetchModelsError.value = ''
  keyModelsStatus.value.clear()
  hasTriedFetchModels.value = false

  // 清除错误信息
  errors.name = ''
  errors.serviceType = ''
  errors.baseUrl = ''

  // 重置快速添加模式数据
  quickInput.value = ''
  detectedBaseUrl.value = ''
  detectedApiKeys.value = []
  detectedServiceType.value = null
  randomSuffix.value = generateRandomString(6)
}

const loadChannelData = (channel: Channel) => {
  form.name = channel.name
  form.serviceType = channel.serviceType
  form.baseUrl = channel.baseUrl
  form.baseUrls = channel.baseUrls || []
  form.website = channel.website || ''
  form.insecureSkipVerify = !!channel.insecureSkipVerify
  form.lowQuality = !!channel.lowQuality
  form.injectDummyThoughtSignature = !!channel.injectDummyThoughtSignature
  form.stripThoughtSignature = !!channel.stripThoughtSignature
  form.description = channel.description || ''

  // 同步 baseUrlsText（优先使用 baseUrls，否则使用 baseUrl）
  if (channel.baseUrls && channel.baseUrls.length > 0) {
    baseUrlsText.value = channel.baseUrls.join('\n')
  } else {
    baseUrlsText.value = channel.baseUrl || ''
  }

  // 直接存储原始密钥，不需要映射关系
  form.apiKeys = [...channel.apiKeys]

  // 清空原始密钥映射（现在不需要了）
  originalKeyMap.value.clear()

  form.modelMapping = { ...(channel.modelMapping || {}) }

  // 立即同步 baseUrl 到预览变量，避免等待 debounce
  formBaseUrlPreview.value = channel.baseUrl

  // 清空模型映射输入框
  newMapping.source = ''
  newMapping.target = ''

  // 清空模型缓存和状态（切换渠道时重置）
  targetModelOptions.value = []
  fetchingModels.value = false
  fetchModelsError.value = ''
  keyModelsStatus.value.clear()
  hasTriedFetchModels.value = false
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
  // 安全地获取字符串值（处理 v-select/v-combobox 可能返回对象的情况）
  const getStringValue = (val: string | { title: string; value: string } | null | undefined): string => {
    if (!val) return ''
    if (typeof val === 'string') return val
    return val.value || ''
  }

  const source = getStringValue(newMapping.source).trim()
  const target = getStringValue(newMapping.target).trim()

  if (source && target && !form.modelMapping[source]) {
    form.modelMapping[source] = target
    newMapping.source = ''
    newMapping.target = ''
  }
}

const removeModelMapping = (source: string) => {
  delete form.modelMapping[source]
}

// 处理目标模型输入框点击事件(仅在首次或有新 key 时触发请求)
const handleTargetModelClick = () => {
  // 如果已经尝试过获取且正在加载中,不重复触发
  if (hasTriedFetchModels.value || fetchingModels.value) {
    return
  }

  // 标记已尝试获取
  hasTriedFetchModels.value = true

  // 调用获取模型列表(内部有缓存逻辑)
  fetchTargetModels()
}

const fetchTargetModels = async () => {
  if (!form.baseUrl || form.apiKeys.length === 0) {
    fetchModelsError.value = '请先填写 Base URL 和至少一个 API Key'
    return
  }

  // 如果已经有模型列表且所有 key 都已检测过,直接返回(缓存)
  if (targetModelOptions.value.length > 0) {
    const allKeysChecked = form.apiKeys.every(key => keyModelsStatus.value.has(key))
    if (allKeysChecked) {
      return
    }
  }

  fetchingModels.value = true
  fetchModelsError.value = ''

  // 仅为未检测过的 API Key 发起请求
  const uncheckedKeys = form.apiKeys.filter(key => !keyModelsStatus.value.has(key))

  if (uncheckedKeys.length === 0) {
    fetchingModels.value = false
    return
  }

  // 为每个未检测的 API Key 检测 models 状态
  const keyPromises = uncheckedKeys.map(async (apiKey) => {
    keyModelsStatus.value.set(apiKey, { loading: true, success: false })

    try {
      const response = await fetchUpstreamModels(form.baseUrl, apiKey)

      keyModelsStatus.value.set(apiKey, {
        loading: false,
        success: true,
        statusCode: 200,
        modelCount: response.data.length
      })

      return response.data
    } catch (error) {
      let errorMsg = '未知错误'
      let statusCode = 0

      if (error instanceof ApiError) {
        errorMsg = error.message
        statusCode = error.status
      } else if (error instanceof Error) {
        errorMsg = error.message
      }

      keyModelsStatus.value.set(apiKey, {
        loading: false,
        success: false,
        statusCode,
        error: errorMsg
      })

      return []
    }
  })

  try {
    const results = await Promise.all(keyPromises)

    // 合并新获取的模型列表到现有列表
    const allModels = new Set<string>(targetModelOptions.value.map(opt => opt.value))
    results.forEach(models => {
      models.forEach(m => allModels.add(m.id))
    })

    targetModelOptions.value = Array.from(allModels)
      .sort()
      .map(id => ({ title: id, value: id }))

    // 如果所有 key 都失败了,显示错误
    const allFailed = form.apiKeys.every(key => {
      const status = keyModelsStatus.value.get(key)
      return status && !status.success
    })

    if (allFailed) {
      fetchModelsError.value = '所有 API Key 都无法获取模型列表,请检查 API 密钥列表中的错误信息'
    }
  } finally {
    fetchingModels.value = false
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return

  const { valid } = await formRef.value.validate()
  if (!valid) return

  // 直接使用原始密钥，不需要转换
  const processedApiKeys = form.apiKeys.filter(key => key.trim())

  // 处理 BaseURL：去重（忽略末尾 / 和 # 差异），并移除 UI 专用的尾部 #
  const seenUrls = new Set<string>()
  const deduplicatedUrls =
    form.baseUrls.length > 0
      ? form.baseUrls
          .map(url => url.trim().replace(/[#/]+$/, ''))
          .filter(Boolean)
          .filter(url => {
            const normalized = url.replace(/[#/]+$/, '')
            if (seenUrls.has(normalized)) return false
            seenUrls.add(normalized)
            return true
          })
      : [form.baseUrl.trim().replace(/[#/]+$/, '')].filter(Boolean)

  // 构建渠道数据
  const channelData: Omit<Channel, 'index' | 'latency' | 'status'> = {
    name: form.name.trim(),
    serviceType: form.serviceType as 'openai' | 'gemini' | 'claude' | 'responses',
    baseUrl: deduplicatedUrls[0] || '',
    website: form.website.trim(), // 空字符串也需要传递，以便清除已有值
    insecureSkipVerify: form.insecureSkipVerify,
    lowQuality: form.lowQuality,
    injectDummyThoughtSignature: form.injectDummyThoughtSignature,
    stripThoughtSignature: form.stripThoughtSignature,
    description: form.description.trim(),
    apiKeys: processedApiKeys,
    modelMapping: form.modelMapping
  }

  // 多 BaseURL 支持
  if (deduplicatedUrls.length > 1) {
    channelData.baseUrls = deduplicatedUrls
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

watch(
  () => form.baseUrl,
  value => {
    if (formBaseUrlPreviewTimer !== null) {
      window.clearTimeout(formBaseUrlPreviewTimer)
    }
    formBaseUrlPreviewTimer = window.setTimeout(() => {
      formBaseUrlPreview.value = value
    }, 200)
  },
  { immediate: true }
)

// ESC键监听
const handleKeydown = (event: Event) => {
  const keyboardEvent = event as KeyboardEvent
  if (keyboardEvent.key === 'Escape' && props.show) {
    handleCancel()
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  if (formBaseUrlPreviewTimer !== null) {
    window.clearTimeout(formBaseUrlPreviewTimer)
  }
})
</script>

<style scoped>
/* 基础URL下方的提示区域 - 固定高度防止布局跳动 */
.base-url-hint {
  min-height: 20px;
  padding: 4px 12px 8px;
  line-height: 1.25;
}

/* 多个预期请求项样式 */
.expected-request-item + .expected-request-item {
  margin-top: 2px;
}

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

/* 多 Base URL 项目样式 */
.base-url-item {
  padding: 6px 10px;
  background: rgba(var(--v-theme-surface-variant), 0.4);
  border-radius: 6px;
  border-left: 2px solid rgb(var(--v-theme-success));
}

.base-url-item + .base-url-item {
  margin-top: 4px;
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
