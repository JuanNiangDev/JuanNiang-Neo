import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface ToastMessage {
  message: string
  color: 'success' | 'error' | 'warning' | 'info'
  timeout?: number
}

export const useToastStore = defineStore('toast', () => {
  const current = ref<ToastMessage | null>(null)
  const show = ref(false)

  function push(msg: ToastMessage) {
    current.value = msg
    show.value = true
    setTimeout(() => { show.value = false; current.value = null }, msg.timeout || 3000)
  }

  function success(message: string) { push({ message, color: 'success' }) }
  function error(message: string) { push({ message, color: 'error', timeout: 5000 }) }
  function warning(message: string) { push({ message, color: 'warning' }) }
  function info(message: string) { push({ message, color: 'info' }) }

  return { current, show, push, success, error, warning, info }
})
