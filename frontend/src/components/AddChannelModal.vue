<template>
  <v-dialog 
    :model-value="show"
    @update:model-value="$emit('update:show', $event)"
    max-width="800"
    persistent
  >
    <v-card rounded="lg">
      <v-card-title class="d-flex align-center ga-3 pa-6 bg-primary text-white">
        <v-avatar color="white" variant="tonal" size="40">
          <v-icon color="primary">{{ isEditing ? 'mdi-pencil' : 'mdi-plus' }}</v-icon>
        </v-avatar>
        <div>
          <div class="text-h5 font-weight-bold">
            {{ isEditing ? '编辑渠道' : '添加新渠道' }}
          </div>
          <div class="text-body-2 text-blue-lighten-1">配置API渠道信息和密钥</div>
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
                        color="surface-variant"
                      >
                        <template v-slot:prepend>
                          <v-icon size="small" color="medium-emphasis">mdi-key</v-icon>
                        </template>
                        
                        <v-list-item-title>
                          <code class="text-caption">{{ maskApiKey(key) }}</code>
                        </v-list-item-title>

                        <template v-slot:append>
                          <v-btn
                            size="small"
                            color="error"
                            icon
                            variant="text"
                            @click="removeApiKey(index)"
                          >
                            <v-icon size="small">mdi-close</v-icon>
                          </v-btn>
                        </template>
                      </v-list-item>
                    </v-list>
                  </div>

                  <!-- 添加新密钥 -->
                  <div class="d-flex align-center ga-2">
                    <v-text-field
                      v-model="newApiKey"
                      label="添加新的API密钥"
                      placeholder="输入完整的API密钥"
                      prepend-inner-icon="mdi-plus"
                      variant="outlined"
                      density="comfortable"
                      type="password"
                      @keyup.enter="addApiKey"
                      hide-details
                    />
                    <v-btn
                      color="primary"
                      variant="elevated"
                      @click="addApiKey"
                      :disabled="!newApiKey.trim()"
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
import { ref, reactive, computed, watch } from 'vue'
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

// 表单引用
const formRef = ref()

// 服务类型选项
const serviceTypeOptions = [
  { title: 'OpenAI (新版API)', value: 'openai' },
  { title: 'OpenAI (兼容旧版)', value: 'openaiold' },
  { title: 'Claude', value: 'claude' },
  { title: 'Gemini', value: 'gemini' }
]

// 表单数据
const form = reactive({
  name: '',
  serviceType: '' as 'openai' | 'openaiold' | 'gemini' | 'claude' | '',
  baseUrl: '',
  description: '',
  apiKeys: [] as string[]
})

// 新API密钥输入
const newApiKey = ref('')

// 表单验证错误
const errors = reactive({
  name: '',
  serviceType: '',
  baseUrl: ''
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
  }
}

// 计算属性
const isEditing = computed(() => !!props.channel)

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
  form.description = ''
  form.apiKeys = []
  newApiKey.value = ''
  
  // 清除错误信息
  errors.name = ''
  errors.serviceType = ''
  errors.baseUrl = ''
}

const loadChannelData = (channel: Channel) => {
  form.name = channel.name
  form.serviceType = channel.serviceType
  form.baseUrl = channel.baseUrl
  form.description = channel.description || ''
  form.apiKeys = [...channel.apiKeys]
}

const addApiKey = () => {
  const key = newApiKey.value.trim()
  if (key && !form.apiKeys.includes(key)) {
    form.apiKeys.push(key)
    newApiKey.value = ''
  }
}

const removeApiKey = (index: number) => {
  form.apiKeys.splice(index, 1)
}

const handleSubmit = async () => {
  if (!formRef.value) return
  
  const { valid } = await formRef.value.validate()
  if (!valid) return
  
  // 类型断言，因为表单验证已经确保serviceType不为空
  const channelData = {
    name: form.name.trim(),
    serviceType: form.serviceType as 'openai' | 'openaiold' | 'gemini' | 'claude',
    baseUrl: form.baseUrl.trim().replace(/\/$/, ''), // 移除末尾斜杠
    description: form.description.trim(),
    apiKeys: form.apiKeys.filter(key => key.trim())
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
</script>