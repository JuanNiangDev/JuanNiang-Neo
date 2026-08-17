<template>
  <v-menu location="bottom end" offset="8" :close-on-content-click="true">
    <template #activator="{ props }">
      <v-btn
        v-bind="props"
        icon
        variant="text"
        size="small"
        class="theme-mode-trigger"
        aria-label="切换主题"
      >
        <v-icon size="20">{{ effectiveIcon }}</v-icon>
      </v-btn>
    </template>

    <v-card rounded="lg" min-width="176" elevation="8" class="theme-mode-menu">
      <v-list density="compact" class="pa-1" aria-label="主题模式">
        <v-list-item
          prepend-icon="mdi-brightness-auto"
          title="跟随系统"
          :active="store.mode === 'system'"
          :append-icon="store.mode === 'system' ? 'mdi-check' : undefined"
          active-color="primary"
          class="theme-mode-item"
          @click="store.setMode('system')"
        />
        <v-list-item
          prepend-icon="mdi-weather-night"
          title="深色"
          :active="store.mode === 'dark'"
          :append-icon="store.mode === 'dark' ? 'mdi-check' : undefined"
          active-color="primary"
          class="theme-mode-item"
          @click="store.setMode('dark')"
        />
        <v-list-item
          prepend-icon="mdi-white-balance-sunny"
          title="浅色"
          :active="store.mode === 'light'"
          :append-icon="store.mode === 'light' ? 'mdi-check' : undefined"
          active-color="primary"
          class="theme-mode-item"
          @click="store.setMode('light')"
        />
      </v-list>
    </v-card>
  </v-menu>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useThemeStore } from '@/stores/theme'

const store = useThemeStore()

// 触发器图标随「生效模式」显示（跟随系统时也显示实际生效的图标）
const effectiveIcon = computed(() =>
  store.effectiveDark ? 'mdi-weather-night' : 'mdi-white-balance-sunny',
)
</script>

<style scoped>
.theme-mode-item {
  border-radius: 8px;
  min-height: 38px;
}
</style>
