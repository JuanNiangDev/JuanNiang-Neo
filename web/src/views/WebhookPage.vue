<template>
  <div>
    <div class="page-header"><div class="page-title">Webhook 配置</div><div class="page-subtitle">Webhook 适配器配置与状态</div></div>

    <v-row>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">配置</span></template></v-card-item>
          <v-card-text>
            <v-form>
              <v-text-field v-model="form.addr" label="监听地址" class="mb-3" />
              <v-text-field v-model.number="form.port" label="端口" type="number" class="mb-3" />
              <v-text-field v-model="form.token" label="鉴权 Token" class="mb-3" type="password" />
              <v-switch v-model="form.enabled" label="启用" color="primary" class="mb-3" />
              <v-btn color="primary" variant="tonal" block @click="save" :loading="saving">保存配置</v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">运行状态</span></template></v-card-item>
          <v-card-text>
            <v-list density="compact">
              <v-list-item>
                <template #prepend><span class="status-dot" :class="config.running ? 'active' : 'inactive'" /></template>
                <v-list-item-title>运行状态</v-list-item-title>
                <v-list-item-subtitle>{{ config.running ? '运行中' : '已停止' }}</v-list-item-subtitle>
              </v-list-item>
              <v-list-item>
                <template #prepend><v-icon color="blue" class="me-3">mdi-ip</v-icon></template>
                <v-list-item-title>监听地址</v-list-item-title>
                <v-list-item-subtitle>{{ config.addr }}:{{ config.port }}</v-list-item-subtitle>
              </v-list-item>
            </v-list>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { webhookApi, type WebhookConfigResp, type UpdateWebhookConfigReq } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const saving = ref(false)
const config = ref<WebhookConfigResp>({ addr: '', port: 0, token: '', enabled: false, running: false })
const form = ref<UpdateWebhookConfigReq>({ addr: '', port: 8099, token: '', enabled: false })

async function fetchConfig() { try { const res = await webhookApi.getConfig(); config.value = res.data.data; form.value = { addr: config.value.addr, port: config.value.port, token: config.value.token, enabled: config.value.enabled } } catch { toastStore.error('获取配置失败') } }
async function save() { saving.value = true; try { const res = await webhookApi.updateConfig(form.value); config.value = res.data.data; toastStore.success('已保存') } catch { toastStore.error('保存失败') } finally { saving.value = false } }
onMounted(fetchConfig)
</script>
