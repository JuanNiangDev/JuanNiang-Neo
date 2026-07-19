<template>
  <v-app-bar elevation="0" height="64" class="top-header" color="header-bg">
    <v-btn class="hidden-md-and-up ms-2" icon variant="text" size="small" @click="appStore.toggleSidebar()">
      <v-icon>mdi-menu</v-icon>
    </v-btn>

    <v-btn class="hidden-sm-and-down ms-3" icon variant="text" size="small" @click="appStore.toggleSidebar()">
      <v-icon>mdi-menu</v-icon>
    </v-btn>

    <div class="header-brand-box ms-3">
      <img v-if="logoExists" src="/banner_2.png" alt="Logo" style="height: 22px; width: auto; border-radius: 4px" @error="logoExists = false" />
      <span class="brand-text">JuanNiang-Neo</span>
    </div>

    <v-spacer />

    <v-btn variant="text" size="small" class="me-2" @click="handleRefresh" :loading="refreshing">
      <v-icon size="18" class="me-1">mdi-refresh</v-icon>
      <span class="hidden-xs">刷新数据</span>
    </v-btn>

    <v-menu offset="12" location="bottom end">
      <template #activator="{ props }">
        <v-btn v-bind="props" variant="text" class="me-3" style="min-width: 0; padding: 0 6px; height: 44px">
          <v-avatar size="34" class="me-2">
            <v-img src="/avatar.jpg" cover />
          </v-avatar>
          <span class="text-body-2 font-weight-medium hidden-xs">{{ username || 'Admin' }}</span>
          <v-icon size="16" class="ms-1">mdi-chevron-down</v-icon>
        </v-btn>
      </template>
      <v-card rounded="lg" min-width="160" elevation="8">
        <v-list density="compact" class="pa-1">
          <v-list-item prepend-icon="mdi-account-circle-outline" title="Admin" subtitle="管理员" />
          <v-divider class="my-1" />
          <v-list-item prepend-icon="mdi-key-change" title="修改密码" to="/settings" />
          <v-divider class="my-1" />
          <v-list-item prepend-icon="mdi-logout" title="退出登录" @click="handleLogout" />
        </v-list>
      </v-card>
    </v-menu>
  </v-app-bar>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

const username = ref(localStorage.getItem('username') || 'Admin')
const logoExists = ref(true)
const refreshing = ref(false)

async function handleRefresh() {
  refreshing.value = true
  // Trigger a global refresh event that pages can listen to
  window.dispatchEvent(new CustomEvent('global-refresh'))
  await new Promise(r => setTimeout(r, 600))
  refreshing.value = false
}

function handleLogout() {
  authStore.logout()
  router.push('/login')
}
</script>
