<template>
  <v-app :theme="themeStore.effectiveThemeName">
    <RouterView />

    <v-snackbar v-if="toastStore.show" v-model="toastStore.show" :color="toastStore.current?.color"
      :timeout="toastStore.current?.timeout || 3000" location="top right" close-on-back>
      <div class="d-flex align-center">
        <v-icon v-if="toastStore.current?.color === 'success'" class="me-2" size="18">mdi-check-circle</v-icon>
        <v-icon v-else-if="toastStore.current?.color === 'error'" class="me-2" size="18">mdi-alert-circle</v-icon>
        <v-icon v-else-if="toastStore.current?.color === 'warning'" class="me-2" size="18">mdi-alert</v-icon>
        <v-icon v-else class="me-2" size="18">mdi-information</v-icon>
        <span>{{ toastStore.current?.message }}</span>
      </div>
      <template #actions>
        <v-btn icon="mdi-close" variant="text" size="small" @click="toastStore.show = false" />
      </template>
    </v-snackbar>
  </v-app>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { RouterView } from 'vue-router'
import { useTheme } from 'vuetify'
import { useToastStore } from '@/stores/toast'
import { useThemeStore } from '@/stores/theme'

const toastStore = useToastStore()
const themeStore = useThemeStore()

// 同步 Vuetify 全局主题名：<v-app :theme> 只是组件级覆盖，不更新 useTheme().global.name，
// 导致依赖 theme.global.name 的图表/取色（如 DashboardPage 的 ECharts 轴色）在浅色模式下
// 仍取深色主题的 on-surface（白色）→ 浅底白字不可见。
const vuetifyTheme = useTheme()
watch(
  () => themeStore.effectiveThemeName,
  (name) => { vuetifyTheme.global.name.value = name },
  { immediate: true },
)
</script>