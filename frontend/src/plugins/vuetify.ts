import { createVuetify } from 'vuetify'
import { h } from 'vue'
import type { IconSet, IconProps, ThemeDefinition } from 'vuetify'
import * as components from 'vuetify/components'
import * as directives from 'vuetify/directives'

// 引入样式
import 'vuetify/styles'

// 从 @mdi/js 按需导入使用的图标 (SVG)
// 📝 维护说明: 新增图标时需要:
//    1. 从 @mdi/js 添加导入 (驼峰命名，如 mdiNewIcon)
//    2. 在 iconMap 中添加映射 (如 'new-icon': mdiNewIcon)
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
  mdiChevronUp,
  mdiChevronLeft,
  mdiChevronRight,
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
  mdiMenuDown,
  mdiMenuUp,
  mdiCheckboxMarked,
  mdiCheckboxBlankOutline,
  mdiMinusBox,
  mdiCircle,
  mdiRadioboxMarked,
  mdiRadioboxBlank,
  mdiStar,
  mdiStarOutline,
  mdiStarHalf,
  mdiPageFirst,
  mdiPageLast,
  mdiUnfoldMoreHorizontal,
  mdiLoading,
  mdiClockOutline,
  mdiCalendar,
  mdiPaperclip,
  mdiEyedropper,
  mdiShieldRefresh,
  mdiShieldOffOutline,
  mdiAlertCircleOutline,
  mdiChartLine,
  mdiDatabase,
} from '@mdi/js'

// 图标名称到 SVG path 的映射 (使用 kebab-case)
const iconMap: Record<string, string> = {
  // Vuetify 内部使用的图标别名
  'complete': mdiCheck,
  'cancel': mdiCloseCircle,
  'close': mdiClose,
  'delete': mdiDelete,
  'clear': mdiClose,
  'success': mdiCheckCircle,
  'info': mdiInformation,
  'warning': mdiAlert,
  'error': mdiAlertCircle,
  'prev': mdiChevronLeft,
  'next': mdiChevronRight,
  'checkboxOn': mdiCheckboxMarked,
  'checkboxOff': mdiCheckboxBlankOutline,
  'checkboxIndeterminate': mdiMinusBox,
  'delimiter': mdiCircle,
  'sortAsc': mdiArrowUpBold,
  'sortDesc': mdiArrowDownBold,
  'expand': mdiChevronDown,
  'menu': mdiMenuDown,
  'subgroup': mdiMenuDown,
  'dropdown': mdiMenuDown,
  'radioOn': mdiRadioboxMarked,
  'radioOff': mdiRadioboxBlank,
  'edit': mdiPencil,
  'ratingEmpty': mdiStarOutline,
  'ratingFull': mdiStar,
  'ratingHalf': mdiStarHalf,
  'loading': mdiLoading,
  'first': mdiPageFirst,
  'last': mdiPageLast,
  'unfold': mdiUnfoldMoreHorizontal,
  'file': mdiPaperclip,
  'plus': mdiPlus,
  'minus': mdiMinusBox,
  'calendar': mdiCalendar,
  'treeviewCollapse': mdiMenuDown,
  'treeviewExpand': mdiMenuUp,
  'eyeDropper': mdiEyedropper,

  // 布局与导航
  'swap-vertical-bold': mdiSwapVerticalBold,
  'drag-vertical': mdiDragVertical,
  'open-in-new': mdiOpenInNew,
  'chevron-down': mdiChevronDown,
  'chevron-up': mdiChevronUp,
  'chevron-left': mdiChevronLeft,
  'chevron-right': mdiChevronRight,
  'dots-vertical': mdiDotsVertical,
  'logout': mdiLogout,
  'archive-outline': mdiArchiveOutline,
  'menu-down': mdiMenuDown,
  'menu-up': mdiMenuUp,

  // 操作按钮
  'pencil': mdiPencil,
  'refresh': mdiRefresh,
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
  'alert-circle-outline': mdiAlertCircleOutline,
  'close-circle': mdiCloseCircle,
  'help-circle': mdiHelpCircle,
  'alert': mdiAlert,

  // 防护盾牌图标
  'shield-refresh': mdiShieldRefresh,
  'shield-off-outline': mdiShieldOffOutline,

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
  'clock-outline': mdiClockOutline,
  'paperclip': mdiPaperclip,
  'eye-dropper': mdiEyedropper,

  // 主题切换
  'weather-night': mdiWeatherNight,
  'white-balance-sunny': mdiWhiteBalanceSunny,

  // 服务类型图标
  'robot': mdiRobot,
  'robot-outline': mdiRobotOutline,
  'message-processing': mdiMessageProcessing,
  'diamond-stone': mdiDiamondStone,
  'api': mdiApi,

  // 复选框和单选框
  'checkbox-marked': mdiCheckboxMarked,
  'checkbox-blank-outline': mdiCheckboxBlankOutline,
  'minus-box': mdiMinusBox,
  'radiobox-marked': mdiRadioboxMarked,
  'radiobox-blank': mdiRadioboxBlank,

  // 评分
  'star': mdiStar,
  'star-outline': mdiStarOutline,
  'star-half': mdiStarHalf,

  // 分页
  'page-first': mdiPageFirst,
  'page-last': mdiPageLast,

  // 其他
  'unfold-more-horizontal': mdiUnfoldMoreHorizontal,
  'circle': mdiCircle,

  // 图表与数据
  'chart-line': mdiChartLine,
  'database': mdiDatabase,
}

// 自定义 SVG iconset - 处理 mdi-xxx 字符串格式
const customSvgIconSet: IconSet = {
  component: (props: IconProps) => {
    // 获取图标名称，去掉 mdi- 前缀
    let iconName = props.icon as string
    if (iconName.startsWith('mdi-')) {
      iconName = iconName.substring(4)
    }

    // 查找对应的 SVG path
    const svgPath = iconMap[iconName]

    if (!svgPath) {
      if (import.meta.env.DEV) {
        console.warn(`[Vuetify Icon] 未找到图标: ${iconName}，请在 vuetify.ts 的 iconMap 中添加映射`)
      }
      return h('span', `[${iconName}]`)
    }

    return h('svg', {
      class: 'v-icon__svg',
      xmlns: 'http://www.w3.org/2000/svg',
      viewBox: '0 0 24 24',
      role: 'img',
      'aria-hidden': 'true',
      style: {
        fontSize: 'inherit',
        width: '1em',
        height: '1em',
      },
    }, [
      h('path', {
        d: svgPath,
        fill: 'currentColor',
      })
    ])
  }
}

// 🎨 精心设计的现代化配色方案
// Light Theme - 清新专业，柔和渐变
const lightTheme: ThemeDefinition = {
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
const darkTheme: ThemeDefinition = {
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
    sets: {
      mdi: customSvgIconSet
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
