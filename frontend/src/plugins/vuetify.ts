import { createVuetify } from 'vuetify'
import { aliases, mdi } from 'vuetify/iconsets/mdi'
import * as components from 'vuetify/components'
import * as directives from 'vuetify/directives'

// 引入样式
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'

const lightTheme = {
  dark: false,
  colors: {
    primary: '#1976D2',
    secondary: '#424242',
    accent: '#82B1FF',
    error: '#FF5252',
    info: '#2196F3',
    success: '#4CAF50',
    warning: '#FFC107',
    background: '#FAFAFA',
    surface: '#FFFFFF',
  }
}

const darkTheme = {
  dark: true,
  colors: {
    primary: '#42a5f5', // 提高亮度以增加对比度
    secondary: '#757575',
    accent: '#FF4081',
    error: '#FF5252',
    info: '#29B6F6',
    success: '#66BB6A',
    warning: '#FFA726',
    background: '#1a1a1a', // 更深的背景
    surface: '#2c2c2c', // 稍亮的卡片表面
  }
}

export default createVuetify({
  components,
  directives,
  icons: {
    defaultSet: 'mdi',
    aliases,
    sets: {
      mdi,
    },
  },
  theme: {
    defaultTheme: 'light',
    themes: {
      light: lightTheme,
      dark: darkTheme,
    },
  },
})
