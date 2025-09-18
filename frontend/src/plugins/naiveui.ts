import { GlobalThemeOverrides } from 'naive-ui'

export const naiveUIThemeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#1976D2',
    primaryColorHover: '#1565C0',
    primaryColorPressed: '#0D47A1',
    primaryColorSuppl: '#42A5F5',
    infoColor: '#2196F3',
    successColor: '#4CAF50',
    warningColor: '#FF9800',
    errorColor: '#F44336',
    fontFamily: 'v-sans, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol"',
  },
  Card: {
    borderRadius: '12px',
    paddingMedium: '20px 24px',
  },
  Button: {
    borderRadius: '8px',
    heightMedium: '36px',
  },
  Input: {
    borderRadius: '8px',
    heightMedium: '36px',
  },
  Select: {
    peers: {
      InternalSelection: {
        borderRadius: '8px',
        heightMedium: '36px',
      }
    }
  }
}