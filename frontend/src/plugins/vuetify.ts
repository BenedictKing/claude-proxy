import { createVuetify } from 'vuetify'
import { aliases, mdi } from 'vuetify/iconsets/mdi'
import * as components from 'vuetify/components'
import * as directives from 'vuetify/directives'

// 引入样式
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'

// 🎨 精心设计的现代化配色方案
// Light Theme - 清新专业，柔和渐变
const lightTheme = {
  dark: false,
  colors: {
    // 主色调 - 现代蓝紫渐变感
    primary: '#6366F1', // Indigo - 沉稳专业
    secondary: '#8B5CF6', // Violet - 辅助强调
    accent: '#EC4899', // Pink - 活力点缀

    // 语义色彩 - 清晰易辨
    info: '#3B82F6', // Blue
    success: '#10B981', // Emerald
    warning: '#F59E0B', // Amber
    error: '#EF4444', // Red

    // 表面色 - 柔和分层
    background: '#F8FAFC', // Slate-50
    surface: '#FFFFFF', // Pure white cards
    'surface-variant': '#F1F5F9', // Slate-100 for secondary surfaces
    'on-surface': '#1E293B', // Slate-800
    'on-background': '#334155' // Slate-700
  }
}

// Dark Theme - 深邃优雅，护眼舒适
const darkTheme = {
  dark: true,
  colors: {
    // 主色调 - 亮度适中，不刺眼
    primary: '#818CF8', // Indigo-400
    secondary: '#A78BFA', // Violet-400
    accent: '#F472B6', // Pink-400

    // 语义色彩 - 暗色适配
    info: '#60A5FA', // Blue-400
    success: '#34D399', // Emerald-400
    warning: '#FBBF24', // Amber-400
    error: '#F87171', // Red-400

    // 表面色 - 深色层次分明
    background: '#0F172A', // Slate-900
    surface: '#1E293B', // Slate-800
    'surface-variant': '#334155', // Slate-700
    'on-surface': '#F1F5F9', // Slate-100
    'on-background': '#E2E8F0' // Slate-200
  }
}

export default createVuetify({
  components,
  directives,
  icons: {
    defaultSet: 'mdi',
    aliases,
    sets: {
      mdi
    }
  },
  theme: {
    defaultTheme: 'light',
    themes: {
      light: lightTheme,
      dark: darkTheme
    }
  }
})
