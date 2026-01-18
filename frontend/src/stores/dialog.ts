import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Channel } from '@/services/api'

/**
 * 对话框状态管理 Store
 *
 * 职责：
 * - 管理添加/编辑渠道对话框状态
 * - 管理添加 API 密钥对话框状态
 * - 管理对话框相关的临时数据（编辑中的渠道、新密钥等）
 */
export const useDialogStore = defineStore('dialog', () => {
  // ===== 状态 =====

  // 添加/编辑渠道对话框
  const showAddChannelModal = ref(false)
  const editingChannel = ref<Channel | null>(null)

  // 添加 API 密钥对话框
  const showAddKeyModal = ref(false)
  const selectedChannelForKey = ref<number>(-1)
  const newApiKey = ref('')

  // ===== 操作方法 =====

  /**
   * 打开添加渠道对话框
   */
  function openAddChannelModal() {
    editingChannel.value = null
    showAddChannelModal.value = true
  }

  /**
   * 打开编辑渠道对话框
   */
  function openEditChannelModal(channel: Channel) {
    editingChannel.value = channel
    showAddChannelModal.value = true
  }

  /**
   * 关闭渠道对话框
   */
  function closeAddChannelModal() {
    showAddChannelModal.value = false
    editingChannel.value = null
  }

  /**
   * 打开添加密钥对话框
   */
  function openAddKeyModal(channelId: number) {
    selectedChannelForKey.value = channelId
    newApiKey.value = ''
    showAddKeyModal.value = true
  }

  /**
   * 关闭密钥对话框
   */
  function closeAddKeyModal() {
    showAddKeyModal.value = false
    selectedChannelForKey.value = -1
    newApiKey.value = ''
  }

  /**
   * 重置所有对话框状态
   */
  function resetDialogState() {
    showAddChannelModal.value = false
    editingChannel.value = null
    showAddKeyModal.value = false
    selectedChannelForKey.value = -1
    newApiKey.value = ''
  }

  return {
    // 状态
    showAddChannelModal,
    editingChannel,
    showAddKeyModal,
    selectedChannelForKey,
    newApiKey,

    // 方法
    openAddChannelModal,
    openEditChannelModal,
    closeAddChannelModal,
    openAddKeyModal,
    closeAddKeyModal,
    resetDialogState,
  }
})
