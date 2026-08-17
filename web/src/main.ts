import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import vuetify from './plugins/vuetify'
import { useThemeStore } from './stores/theme'
import './scss/style.scss'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
// 先实例化主题 store：同步应用 <html data-theme> 与 color-scheme，避免挂载前闪烁
useThemeStore(pinia)
app.use(router)
app.use(vuetify)
app.mount('#app')
