<template>
  <div>
    <div class="page-header"><div class="page-title">系统设置</div><div class="page-subtitle">修改管理员密码、回复策略等</div></div>

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

    <!-- 回复策略 -->
    <v-row>
      <v-col cols="12">
        <v-card rounded="lg" elevation="1">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">系统回复策略</span></template>
            <template #subtitle>控制机器人在群聊和私聊中的回复行为</template>
          </v-card-item>
          <v-card-text>
            <v-form @submit.prevent="handleSaveStrategy" ref="strategyForm">
              <v-radio-group v-model="form.strategy" class="mb-4">
                <v-radio label="完全不回复 — 不处理任何消息（默认）" value="never_reply" color="error" />
                <v-radio label="仅@我时回复 — 只有被@时才交给 Plugin 和 Agent" value="at_only" color="warning" />
                <v-radio label="完全回复 — 正常处理所有消息" value="always" color="success" />
                <v-radio label="仅 Plugin — 只给 Plugin 处理，不给 Agent" value="plugin_only" color="info" />
                <v-radio label="按相关性回复 — 由 Agent 判断消息是否相关后再回复" value="relevance" color="primary" />
              </v-radio-group>

              <v-expand-transition>
                <v-card v-if="form.strategy === 'relevance'" variant="outlined" class="mb-4 pa-3" rounded="lg">
                  <div class="text-subtitle-2 font-weight-bold mb-2">相关性阈值</div>
                  <div class="d-flex align-center" style="gap: 16px">
                    <v-slider
                      v-model="form.relevance_threshold"
                      :min="0"
                      :max="1"
                      :step="0.05"
                      color="primary"
                      thumb-label="always"
                      hide-details
                      style="flex: 1"
                    />
                    <v-text-field
                      v-model.number="form.relevance_threshold"
                      type="number"
                      :min="0"
                      :max="1"
                      :step="0.05"
                      density="compact"
                      style="width: 90px"
                      hide-details
                      variant="outlined"
                    />
                  </div>
                  <div class="text-caption text-medium-emphasis mt-2">
                    Agent 判断消息相关性 &ge; 阈值时才会回复。被@时自动绕过此判断。
                    <br />当前阈值: {{ form.relevance_threshold.toFixed(2) }}
                  </div>
                </v-card>
              </v-expand-transition>

              <v-btn type="submit" color="primary" variant="tonal" :loading="strategySaving" :disabled="!form.strategy">
                <v-icon class="me-1">mdi-content-save</v-icon> 保存策略
              </v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { replyStrategyApi } from '@/api'

const authStore = useAuthStore()
const toastStore = useToastStore()
const oldPassword = ref('')
const newPassword = ref('')
const showOld = ref(false); const showNew = ref(false)
const saving = ref(false)
const pwdForm = ref()
const strategySaving = ref(false)

const form = ref({
  strategy: 'never_reply',
  relevance_threshold: 0.5,
})

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

async function loadStrategy() {
  try {
    const res = await replyStrategyApi.get()
    const d = (res.data as any)?.data
    if (d) {
      form.value.strategy = d.strategy || 'never_reply'
      form.value.relevance_threshold = d.relevance_threshold ?? 0.5
    }
  } catch (e: any) {
    // 首次加载可能无配置，使用默认值
  }
}

async function handleSaveStrategy() {
  strategySaving.value = true
  try {
    await replyStrategyApi.update({
      strategy: form.value.strategy,
      relevance_threshold: form.value.relevance_threshold,
    })
    toastStore.success('回复策略已保存')
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || e?.message || '保存失败')
  } finally { strategySaving.value = false }
}

onMounted(() => {
  loadStrategy()
})
</script>