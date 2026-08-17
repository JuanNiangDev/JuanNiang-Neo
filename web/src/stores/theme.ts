import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

export type ThemeMode = 'system' | 'dark' | 'light'

const STORAGE_KEY = 'theme-mode'

function loadMode(): ThemeMode {
  const raw = localStorage.getItem(STORAGE_KEY)
  return raw === 'dark' || raw === 'light' || raw === 'system' ? raw : 'system'
}

/**
 * 深浅色模式：跟随系统 / 深色 / 浅色。
 * 用户选择持久化到 localStorage（theme-mode），并同步到
 * <html data-theme="dark|light"> 与 color-scheme（供原生滚动条/表单控件匹配）。
 */
export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(loadMode())
  const systemDark = ref(window.matchMedia('(prefers-color-scheme: dark)').matches)

  const systemQuery = window.matchMedia('(prefers-color-scheme: dark)')
  systemQuery.addEventListener('change', (e: MediaQueryListEvent) => {
    systemDark.value = e.matches
  })

  // 生效的模式（'system' 时按系统偏好解析）
  const effectiveDark = computed(
    () => (mode.value === 'system' ? systemDark.value : mode.value === 'dark'),
  )
  const effectiveThemeName = computed(
    () => (effectiveDark.value ? 'JuanNiangThemeDark' : 'JuanNiangTheme'),
  )

  function applyDocument() {
    const dark = effectiveDark.value
    document.documentElement.dataset.theme = dark ? 'dark' : 'light'
    document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
  }

  function setMode(next: ThemeMode) {
    mode.value = next
    localStorage.setItem(STORAGE_KEY, next)
    applyDocument()
  }

  // 跟随系统时，系统偏好变化也要同步 <html data-theme> 与 color-scheme
  watch(effectiveDark, applyDocument)

  // store 实例化时立即同步一次文档状态（main.ts 中先于挂载调用，避免闪烁）
  applyDocument()

  return { mode, effectiveDark, effectiveThemeName, setMode }
})
