import type { ThemeDefinition } from 'vuetify'

// Shared across both themes
const sharedColors = {
  'stat-pink': '#e91e63',
  'stat-blue': '#2196f3',
  'stat-green': '#4caf50',
  'stat-orange': '#ff9800',
  'stat-purple': '#9c27b0',
  'stat-teal': '#009688',
}

export const JuanNiangTheme: ThemeDefinition = {
  dark: false,
  colors: {
    ...sharedColors,
    'background': '#f0f2f5',
    'surface': '#ffffff',
    'primary': '#e91e63',
    'primary-darken-1': '#c2185b',
    'secondary': '#37474f',
    'secondary-darken-1': '#263238',
    'accent': '#ff4081',
    'error': '#f44336',
    'info': '#2196f3',
    'success': '#4caf50',
    'warning': '#ff9800',
    'sidebar-bg': '#1e1e2d',
    'sidebar-text': '#a2a3b7',
    'header-bg': '#ffffff',
    'card-bg': '#ffffff',
  },
}

export const JuanNiangThemeDark: ThemeDefinition = {
  dark: true,
  colors: {
    ...sharedColors,
    'background': '#14141e',
    'surface': '#1e1e2d',
    'primary': '#ff6b9d',
    'primary-darken-1': '#e91e63',
    'secondary': '#455a64',
    'secondary-darken-1': '#37474f',
    'accent': '#ff80ab',
    'error': '#ff5252',
    'info': '#448aff',
    'success': '#69f0ae',
    'warning': '#ffd740',
    'sidebar-bg': '#151521',
    'sidebar-text': '#6e6e8a',
    'header-bg': '#1e1e2d',
    'card-bg': '#252538',
  },
}
