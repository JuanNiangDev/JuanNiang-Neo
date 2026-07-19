import { createVuetify } from 'vuetify'
import * as components from 'vuetify/components'
import * as directives from 'vuetify/directives'
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'
import { JuanNiangTheme, JuanNiangThemeDark } from '@/theme/theme'

export default createVuetify({
  components,
  directives,
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
