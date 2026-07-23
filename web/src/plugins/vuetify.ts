import { createVuetify } from 'vuetify'
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'
import { JuanNiangTheme, JuanNiangThemeDark } from '@/theme/theme'

// vite-plugin-vuetify 的 autoImport 自动按需导入组件和指令，无需手动注册
export default createVuetify({
  theme: {
    defaultTheme: 'JuanNiangThemeDark',
    themes: {
      JuanNiangTheme,
      JuanNiangThemeDark,
    },
  },
  defaults: {
    VCard: { rounded: 'lg', elevation: 1 },
    VBtn: { rounded: 'lg' },
    VTextField: { variant: 'outlined', density: 'comfortable', rounded: 'lg' },
    VSelect: { variant: 'outlined', density: 'comfortable', rounded: 'lg' },
    VTextarea: { variant: 'outlined', density: 'comfortable', rounded: 'lg' },
    VSnackbar: { location: 'top right', rounded: 'lg' },
    VChip: { rounded: 'lg' },
    VDialog: { maxWidth: 600 },
    VTooltip: { location: 'top' },
    VDataTable: { hover: true },
  },
})
