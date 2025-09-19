import { createVuetify } from 'vuetify'
import { aliases, mdi } from 'vuetify/iconsets/mdi'
import * as components from 'vuetify/components'
import * as directives from 'vuetify/directives'

// 引入样式
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'

// Align Vuetify colors to DaisyUI "emerald" (light)
const lightTheme = {
  dark: false,
  colors: {
    // Use DaisyUI emerald palette directly
    primary: '#66cc8a',
    secondary: '#377cfb',
    accent: '#f68067',
    // Keep semantic colors to Vuetify defaults to avoid ad-hoc mixing
    background: '#ffffff', // base-100
    surface: '#f9fafb'     // aligns with neutral-content tone
  }
}

// Align Vuetify colors to DaisyUI "night" (dark)
const darkTheme = {
  dark: true,
  colors: {
    primary: '#38BDF8',
    secondary: '#818CF8',
    accent: '#F471B5',
    info: '#0CA5E9',
    success: '#2DD4BF',
    warning: '#F4BF50',
    error: '#FB7085',
    background: '#0F172A', // base-100
    surface: '#1E293B'     // neutral
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
      dark: darkTheme
    }
  }
})
