import { ref, computed, watch } from 'vue'
import { useTheme as useVuetifyTheme } from 'vuetify'

// 复古像素主题配置
export const RETRO_THEME = {
  name: '复古像素',
  radius: '0px',
  font: '"Courier New", Consolas, monospace'
}

export function useAppTheme() {
  const vuetifyTheme = useVuetifyTheme()

  // 应用复古像素主题
  function applyRetroTheme() {
    document.documentElement.style.setProperty('--app-radius', RETRO_THEME.radius)
    document.documentElement.style.setProperty('--app-font', RETRO_THEME.font)
    document.documentElement.dataset.theme = 'retro'
  }

  // 初始化
  function init() {
    applyRetroTheme()
  }

  return {
    init
  }
}
