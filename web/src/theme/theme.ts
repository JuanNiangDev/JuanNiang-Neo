import type { ThemeDefinition } from 'vuetify'

// Shared across both themes — 语义色（现代控制台）
const sharedColors = {
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
    'sidebar-bg': '#0e1117',
    'sidebar-text': '#8b93a5',
    'header-bg': '#0e1117',
    'card-bg': '#171b25',
  },
}

// 亮色主题 — 干净的白底 + 同色系主色
export const JuanNiangTheme: ThemeDefinition = {
  dark: false,
  colors: {
    ...sharedColors,
    'background': '#f4f5fb',
    'surface': '#ffffff',
    'surface-bright': '#ffffff',
    'surface-light': '#fafbff',
    'surface-variant': '#eef1f8',
    'on-surface': '#1a2233',
    'on-surface-variant': '#5b6472',
    'primary': '#4f46e5',
    'primary-darken-1': '#4338ca',
    'secondary': '#64748b',
    'secondary-darken-1': '#475569',
    'accent': '#7c3aed',
    'error': '#dc2626',
    'info': '#2563eb',
    'success': '#059669',
    'warning': '#d97706',
    'sidebar-bg': '#0e1117',
    'sidebar-text': '#a3abc0',
    'header-bg': '#ffffff',
    'card-bg': '#ffffff',
  },
}