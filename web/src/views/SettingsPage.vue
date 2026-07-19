<template>
  <div>
    <div class="page-header"><div class="page-title">系统设置</div><div class="page-subtitle">修改管理员密码</div></div>

    <v-row>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">修改密码</span></template></v-card-item>
          <v-card-text>
            <v-form @submit.prevent="handleChangePassword" ref="pwdForm">
              <v-text-field
                v-model="oldPassword"
                label="原密码"
                :type="showOld ? 'text' : 'password'"
                :append-inner-icon="showOld ? 'mdi-eye-off' : 'mdi-eye'"
                @click:append-inner="showOld = !showOld"
                prepend-inner-icon="mdi-lock-outline"
                class="mb-4"
                :rules="[v => !!v || '请输入原密码']"
              />
              <v-text-field
                v-model="newPassword"
                label="新密码"
                :type="showNew ? 'text' : 'password'"
                :append-inner-icon="showNew ? 'mdi-eye-off' : 'mdi-eye'"
                @click:append-inner="showNew = !showNew"
                prepend-inner-icon="mdi-lock-plus-outline"
                class="mb-4"
                :rules="[v => !!v || '请输入新密码', v => v.length >= 8 || '至少8位']"
              />
              <v-btn type="submit" color="primary" variant="tonal" block :loading="saving">
                <v-icon class="me-1">mdi-content-save</v-icon> 修改密码
              </v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">关于</span></template></v-card-item>
          <v-card-text>
            <v-list density="compact">
              <v-list-item prepend-icon="mdi-information" title="JuanNiang-Neo" subtitle="智能机器人管理面板" />
              <v-list-item prepend-icon="mdi-tag" title="版本" subtitle="v1.0.0" />
              <v-list-item prepend-icon="mdi-language-go" title="API 版本" subtitle="OpenAPI 3.0.3" />
            </v-list>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

const authStore = useAuthStore()
const toastStore = useToastStore()
const oldPassword = ref('')
const newPassword = ref('')
const showOld = ref(false); const showNew = ref(false)
const saving = ref(false)
const pwdForm = ref()

async function handleChangePassword() {
  const { valid } = await (pwdForm.value as any)?.validate?.() || { valid: true }
  if (!valid) return
  saving.value = true
  try {
    await authStore.changePassword(oldPassword.value, newPassword.value)
    toastStore.success('密码修改成功，请重新登录')
    setTimeout(() => authStore.logout(), 1500)
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || e?.message || '修改失败')
  } finally { saving.value = false }
}
</script>
