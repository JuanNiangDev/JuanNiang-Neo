<template>
  <div class="login-page">
    <div class="login-decor circle-1"></div>
    <div class="login-decor circle-2"></div>
    <div class="login-decor circle-3"></div>
    <div class="login-decor line-dots">
      <span v-for="i in 9" :key="i"></span>
    </div>

    <div class="login-banner-wrapper">
      <div class="login-banner-frame">
        <img src="/banner_1.jpg" alt="JuanNiang Banner" @error="(e: any) => e.target.style.display = 'none'" />
      </div>
    </div>

    <div class="login-form-container">
      <div class="login-logo">
        <div class="logo-row">
          <img src="/site_logo.png" alt="Logo" />
          <div>
            <h1>JuanNiang-Neo</h1>
            <p>智能管理面板</p>
          </div>
        </div>
      </div>

      <v-card variant="flat" color="transparent" class="pa-0">
        <v-card-text class="pa-0">
          <v-form @submit.prevent="handleLogin" ref="formRef">
            <v-text-field
              v-model="username"
              label="用户名"
              prepend-inner-icon="mdi-account-outline"
              variant="outlined"
              color="primary"
              class="mb-4 login-field"
              autocomplete="username"
            />

            <v-text-field
              v-model="password"
              label="密码"
              :type="showPwd ? 'text' : 'password'"
              prepend-inner-icon="mdi-lock-outline"
              :append-inner-icon="showPwd ? 'mdi-eye-off' : 'mdi-eye'"
              @click:append-inner="showPwd = !showPwd"
              variant="outlined"
              color="primary"
              class="mb-2 login-field"
              autocomplete="current-password"
              @keydown.enter="handleLogin"
            />

            <v-btn type="submit" block class="login-btn mt-6" :loading="loading" color="primary" size="large">
              登 录
            </v-btn>
          </v-form>

          <v-alert v-if="errorMsg" type="error" variant="tonal" border="start" class="mt-4" density="compact"
            closable @click:close="errorMsg = ''">
            {{ errorMsg }}
          </v-alert>
        </v-card-text>
      </v-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

const router = useRouter()
const authStore = useAuthStore()
const toastStore = useToastStore()

const username = ref('')
const password = ref('')
const showPwd = ref(false)
const loading = ref(false)
const errorMsg = ref('')
const formRef = ref()

async function handleLogin() {
  if (!username.value || !password.value) {
    errorMsg.value = '请输入用户名和密码'
    return
  }
  loading.value = true; errorMsg.value = ''
  try {
    await authStore.login(username.value, password.value)
    toastStore.success('登录成功')
    router.push('/dashboard')
  } catch (e: any) {
    errorMsg.value = e?.response?.data?.info || e?.message || '登录失败'
  } finally { loading.value = false }
}
</script>

<style scoped>
.login-field :deep(.v-field) {
  background: rgba(255, 255, 255, 0.04) !important;
  border-radius: 12px;
  position: relative;
  z-index: 10;
  margin-top: 9px;
}
.login-field :deep(.v-field__outline) {
  --v-field-border-opacity: 0.2;
  border-radius: 12px;
}
.login-field :deep(.v-field__outline::before) {
  border-color: rgba(255, 255, 255, 0.3) !important;
}
.login-field :deep(.v-field__outline::after) {
  border-color: rgb(var(--v-theme-primary)) !important;
}
.login-field :deep(.v-label) {
  color: rgba(255, 255, 255, 0.55) !important;
}
.login-field :deep(.v-field--focused .v-label),
.login-field :deep(.v-field--active .v-label) {
  color: rgb(var(--v-theme-primary)) !important;
}
.login-field :deep(input) {
  color: #fff !important;
}
.login-field :deep(.v-field__prepend-inner .v-icon),
.login-field :deep(.v-field__append-inner .v-icon) {
  color: rgba(255, 255, 255, 0.45) !important;
}
</style>
