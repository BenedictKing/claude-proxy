import { createVuetify } from 'vuetify'
import { aliases as defaultAliases, mdi } from 'vuetify/iconsets/mdi-svg'
import * as components from 'vuetify/components'
import * as directives from 'vuetify/directives'

// 引入样式
import 'vuetify/styles'

// 从 @mdi/js 按需导入使用的图标 (SVG)
// 📝 维护说明: 新增图标时需要:
//    1. 从 @mdi/js 添加导入 (驼峰命名，如 mdiNewIcon)
//    2. 在 customAliases 中添加映射 (如 'new-icon': mdiNewIcon)
//    图标查找: https://pictogrammers.com/library/mdi/
import {
  mdiSwapVerticalBold,
  mdiPlayCircle,
  mdiDragVertical,
  mdiOpenInNew,
  mdiKey,
  mdiRefresh,
  mdiDotsVertical,
  mdiPencil,
  mdiSpeedometer,
  mdiRocketLaunch,
  mdiPauseCircle,
  mdiStopCircle,
  mdiDelete,
  mdiPlaylistRemove,
  mdiArchiveOutline,
  mdiPlus,
  mdiCheckCircle,
  mdiAlertCircle,
  mdiHelpCircle,
  mdiCloseCircle,
  mdiTag,
  mdiInformation,
  mdiCog,
  mdiWeb,
  mdiShieldAlert,
  mdiText,
  mdiSwapHorizontal,
  mdiArrowRight,
  mdiClose,
  mdiArrowUpBold,
  mdiArrowDownBold,
  mdiCheck,
  mdiContentCopy,
  mdiAlert,
  mdiWeatherNight,
  mdiWhiteBalanceSunny,
  mdiLogout,
  mdiServerNetwork,
  mdiHeartPulse,
  mdiChevronDown,
  mdiTune,
  mdiRotateRight,
  mdiDice6,
  mdiBackupRestore,
  mdiKeyPlus,
  mdiPin,
  mdiPinOutline,
  mdiKeyChain,
  mdiRobot,
  mdiRobotOutline,
  mdiMessageProcessing,
  mdiDiamondStone,
  mdiApi,
  mdiLightningBolt,
  mdiFormTextbox,
} from '@mdi/js'

// 自定义图标别名映射 (mdi-xxx 字符串 -> SVG path)
const customAliases = {
  ...defaultAliases,
  // 布局与导航
  'swap-vertical-bold': mdiSwapVerticalBold,
  'drag-vertical': mdiDragVertical,
  'open-in-new': mdiOpenInNew,
  'chevron-down': mdiChevronDown,
  'dots-vertical': mdiDotsVertical,
  'logout': mdiLogout,
  'archive-outline': mdiArchiveOutline,

  // 操作按钮
  'plus': mdiPlus,
  'pencil': mdiPencil,
  'delete': mdiDelete,
  'refresh': mdiRefresh,
  'close': mdiClose,
  'check': mdiCheck,
  'content-copy': mdiContentCopy,
  'arrow-up-bold': mdiArrowUpBold,
  'arrow-down-bold': mdiArrowDownBold,
  'arrow-right': mdiArrowRight,
  'swap-horizontal': mdiSwapHorizontal,
  'rotate-right': mdiRotateRight,
  'backup-restore': mdiBackupRestore,

  // 状态图标
  'play-circle': mdiPlayCircle,
  'pause-circle': mdiPauseCircle,
  'stop-circle': mdiStopCircle,
  'check-circle': mdiCheckCircle,
  'alert-circle': mdiAlertCircle,
  'close-circle': mdiCloseCircle,
  'help-circle': mdiHelpCircle,
  'alert': mdiAlert,

  // 功能图标
  'key': mdiKey,
  'key-plus': mdiKeyPlus,
  'key-chain': mdiKeyChain,
  'speedometer': mdiSpeedometer,
  'rocket-launch': mdiRocketLaunch,
  'playlist-remove': mdiPlaylistRemove,
  'tag': mdiTag,
  'information': mdiInformation,
  'cog': mdiCog,
  'web': mdiWeb,
  'shield-alert': mdiShieldAlert,
  'text': mdiText,
  'tune': mdiTune,
  'dice-6': mdiDice6,
  'heart-pulse': mdiHeartPulse,
  'server-network': mdiServerNetwork,
  'pin': mdiPin,
  'pin-outline': mdiPinOutline,
  'lightning-bolt': mdiLightningBolt,
  'form-textbox': mdiFormTextbox,

  // 主题切换
  'weather-night': mdiWeatherNight,
  'white-balance-sunny': mdiWhiteBalanceSunny,

  // 服务类型图标
  'robot': mdiRobot,
  'robot-outline': mdiRobotOutline,
  'message-processing': mdiMessageProcessing,
  'diamond-stone': mdiDiamondStone,
  'api': mdiApi,
}

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
    aliases: customAliases,
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
