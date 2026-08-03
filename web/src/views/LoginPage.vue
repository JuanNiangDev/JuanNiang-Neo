<template>
  <div class="login-page login-ui">
    <div class="login-shell">
      <!-- 左：Logo(左上角) + 原图(居中带框) 2:1 -->
      <section class="banner-panel">
        <div class="login-logo">
          <img src="/site_logo.png" alt="Logo" />
          <div>
            <h1>JuanNiang-Neo</h1>
            <p>智能体管理面板</p>
          </div>
        </div>

        <div class="banner-frame">
          <img src="/banner_1.jpg" alt="JuanNiang Banner" @error="(e: any) => e.target.style.display = 'none'" />
        </div>
      </section>

      <!-- 右：登录表单 -->
      <section class="auth-panel">
        <div class="auth-card">
          <div class="auth-kicker">AUTHORIZE / 登录</div>
          <h2 class="auth-title">欢迎回来</h2>
          <p class="auth-sub">登录以访问管理控制台</p>

          <v-form @submit.prevent="handleLogin" ref="formRef">
            <v-text-field
              v-model="username"
              label="用户名"
              prepend-inner-icon="mdi-account-outline"
              variant="outlined"
              color="primary"
              class="mb-4 auth-field"
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
              class="mb-2 auth-field"
              autocomplete="current-password"
              @keydown.enter="handleLogin"
            />
            <v-btn type="submit" block class="auth-btn mt-6" :loading="loading" size="large">
              进入控制台
            </v-btn>
          </v-form>

          <v-alert v-if="errorMsg" type="error" variant="tonal" border="start" class="mt-4" density="compact"
            closable @click:close="errorMsg = ''">
            {{ errorMsg }}
          </v-alert>
        </div>
      </section>
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
/* ============ 设计令牌 ============ */
.login-ui {
  --accent: #6366f1;
  --ink: #e8ecf4;
  --ink-dim: #8b93a5;
  --line: rgba(255, 255, 255, 0.08);
  --mono: 'Space Mono', 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
  --display: 'Rajdhani', 'PingFang SC', 'Microsoft YaHei', sans-serif;
}

/* ============ 根容器（覆盖 shared 的 .login-page） ============ */
.login-page.login-ui {
  display: block;
  min-height: 100vh;
  background: #07080d;
  position: relative;
  overflow: hidden;
  font-family: var(--display);
}

/* ============ 布局：左图 2 : 右表单 1 ============ */
.login-shell {
  position: relative;
  z-index: 2;
  min-height: 100vh;
  display: grid;
  grid-template-columns: 2fr 1fr;
}

/* ============ 左：Logo + 居中带框图片 ============ */
.banner-panel {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: 48px;
  background: radial-gradient(900px 500px at 20% 10%, rgba(99, 102, 241, 0.12), transparent 60%),
              radial-gradient(700px 500px at 90% 95%, rgba(139, 92, 246, 0.1), transparent 55%),
              #0a0c12;
}

/* Logo 位于左上角 */
.login-logo {
  position: absolute;
  top: 40px;
  left: 48px;
  display: flex;
  align-items: center;
  gap: 12px;
  img {
    width: 44px;
    height: 44px;
    border-radius: 11px;
    box-shadow: 0 0 22px rgba(99, 102, 241, 0.35);
  }
  h1 {
    font-size: 21px;
    font-weight: 700;
    color: var(--ink);
    margin: 0;
    letter-spacing: 0.2px;
  }
  p {
    font-size: 12.5px;
    color: var(--ink-dim);
    margin: 2px 0 0;
  }
}

/* 居中带渐变描边框（参考旧版） */
.banner-frame {
  position: relative;
  border-radius: 24px;
  padding: 6px;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.35), rgba(139, 92, 246, 0.25));
  box-shadow: 0 32px 64px rgba(0, 0, 0, 0.4), 0 0 80px rgba(99, 102, 241, 0.1);
  img {
    display: block;
    max-width: min(76vw, 560px);
    max-height: 70vh;
    border-radius: 20px;
    object-fit: cover;
  }
}

/* ============ 右：登录表单 ============ */
.auth-panel {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 40px;
  border-left: 1px solid var(--line);
  background: linear-gradient(180deg, #0b0d14 0%, #07080d 100%);
}
.auth-card {
  width: 100%;
  max-width: 380px;
}
.auth-kicker {
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 2px;
  color: rgba(99, 102, 241, 0.85);
  margin-bottom: 12px;
}
.auth-title {
  font-family: var(--display);
  font-weight: 700;
  font-size: 30px;
  letter-spacing: 0.5px;
  color: var(--ink);
  margin: 0 0 6px;
}
.auth-sub {
  font-size: 14px;
  color: var(--ink-dim);
  margin: 0 0 28px;
}

/* 输入框终端风格 */
.auth-field :deep(.v-field) {
  background: rgba(255, 255, 255, 0.03) !important;
  border-radius: 10px;
}
.auth-field :deep(.v-field__outline) {
  --v-field-border-opacity: 0.25;
}
.auth-field :deep(.v-field__outline::before) {
  border-color: rgba(255, 255, 255, 0.18) !important;
}
.auth-field :deep(.v-field__outline::after) {
  border-color: var(--accent) !important;
}
.auth-field :deep(.v-label) {
  color: rgba(255, 255, 255, 0.5) !important;
}
.auth-field :deep(.v-field--focused .v-label),
.auth-field :deep(.v-field--active .v-label) {
  color: var(--accent) !important;
}
.auth-field :deep(input) {
  color: var(--ink) !important;
  font-family: var(--mono);
}
.auth-field :deep(.v-field__prepend-inner .v-icon),
.auth-field :deep(.v-field__append-inner .v-icon) {
  color: rgba(255, 255, 255, 0.4) !important;
}

/* 登录按钮 */
.auth-btn {
  height: 50px !important;
  font-size: 15px !important;
  font-weight: 600 !important;
  letter-spacing: 1px !important;
  font-family: var(--display);
  background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%) !important;
  border-radius: 12px !important;
  box-shadow: 0 10px 30px rgba(99, 102, 241, 0.35) !important;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.auth-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 14px 36px rgba(99, 102, 241, 0.5) !important;
}

/* ============ 响应式 ============ */
@media (max-width: 960px) {
  .login-shell { grid-template-columns: 1fr; }
  .banner-panel { display: none; }
  .auth-panel { border-left: none; min-height: 100vh; padding: 40px 24px; }
}
</style>