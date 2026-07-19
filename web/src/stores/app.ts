import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAppStore = defineStore('app', () => {
  const sidebarOpen = ref(true)

  function toggleSidebar() { sidebarOpen.value = !sidebarOpen.value }
  function setSidebar(v: boolean) { sidebarOpen.value = v }

  return { sidebarOpen, toggleSidebar, setSidebar }
})
