import type { ThemeDefinition } from 'vuetify'

// 两主题共用的语义色——统计/状态色，两态取值一致。
const sharedColors = {
  // 统计/状态色（stat-*：图标底、状态点等），品牌级语义，两态通用
  'stat-pink': '#ff5c8a',
  'stat-blue': '#60a5fa',
  'stat-green': '#34d399',
  'stat-orange': '#fbbf24',
  'stat-purple': '#a78bfa',
  'stat-teal': '#2dd4bf',
}

// 深色主题（默认）— 中性偏冷近黑 + 靛蓝/紫罗兰主色
export const JuanNiangThemeDark: ThemeDefinition = {
  dark: true,
  colors: {
    ...sharedColors,
    'background': '#0b0d14',
    'surface': '#12151d',
    'surface-bright': '#1a1e2a',
    'surface-light': '#171b25',
    'surface-variant': '#1f2430',
    'on-surface': '#e6e8ee',
    'on-surface-variant': '#9aa1b0',
    'primary': '#6366f1',
    'primary-darken-1': '#4f46e5',
    'secondary': '#64748b',
    'secondary-darken-1': '#475569',
    'accent': '#8b5cf6',
    'error': '#f87171',
    'info': '#60a5fa',
    'success': '#34d399',
    'warning': '#fbbf24',
    // 侧边栏：深色面板（深色模式）
    'sidebar-bg': '#0e1117',
    'sidebar-text': '#8b93a5',
    'sidebar-hover-text': '#e8ecf4',
    'header-bg': '#0e1117',
    'card-bg': '#171b25',
    // 代码区（CodeMirror / .code-block / 日志与 Prompt 代码框）：深色面板 + GitHub-dark 语法色
    'code-bg': '#161b22',
    'code-fg': '#cbd5e1',
    'code-property': '#79c0ff',
    'code-string': '#a5d6ff',
    'code-number': '#ffab70',
    'code-comment': '#8b949e',
  },
}

// 亮色主题 — 清透冷白 + 靛蓝主色（同品牌色系），MD3 分层的 surface 层级
export const JuanNiangTheme: ThemeDefinition = {
  dark: false,
  colors: {
    ...sharedColors,
    'background': '#f5f6fa',
    'surface': '#ffffff',
    'surface-bright': '#ffffff',
    'surface-light': '#fbfcfe',
    'surface-variant': '#eceef5',
    'on-surface': '#1b2233',
    'on-surface-variant': '#5d6678',
    'primary': '#4f46e5',
    'primary-darken-1': '#4338ca',
    'secondary': '#64748b',
    'secondary-darken-1': '#475569',
    'accent': '#7c3aed',
    'error': '#dc2626',
    'info': '#2563eb',
    'success': '#059669',
    'warning': '#d97706',
    // 侧边栏：白色面板（浅色模式，与 header/卡片同色系）
    'sidebar-bg': '#ffffff',
    'sidebar-text': '#475569',
    'sidebar-hover-text': '#1b2233',
    'header-bg': '#ffffff',
    'card-bg': '#ffffff',
    // 代码区：浅色面板 + GitHub-light 语法色（深浅切换时随 CSS 变量自动更新）
    'code-bg': '#f6f8fa',
    'code-fg': '#24292f',
    'code-property': '#0550ae',
    'code-string': '#0a3069',
    'code-number': '#116329',
    'code-comment': '#6e7781',
  },
}
