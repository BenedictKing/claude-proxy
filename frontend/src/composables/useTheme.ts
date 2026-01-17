import { useTheme as useVuetifyTheme } from 'vuetify'

// 复古像素主题配置
export const RETRO_THEME = {
  name: '复古像素',
  font: '"Courier New", Consolas, "Liberation Mono", monospace'
}

export function useAppTheme() {
  const _vuetifyTheme = useVuetifyTheme()

  // 应用复古像素主题
  function applyRetroTheme() {
    document.documentElement.style.setProperty('--app-font', RETRO_THEME.font)
  }

  // 初始化
  function init() {
    applyRetroTheme()
  }

  return {
    init
  }
}
