<template>
  <div>
    <div class="page-header">
      <div class="page-title">Adapter 管理</div>
      <div class="page-subtitle">OneBot11 适配器状态与配置</div>
    </div>

    <v-row>
      <!-- Status Card -->
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1" class="mb-4">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">运行状态</span></template></v-card-item>
          <v-card-text>
            <v-list density="compact" lines="two">
              <v-list-item>
                <template #prepend><span class="status-dot" :class="status.running ? 'active' : 'inactive'" /></template>
                <v-list-item-title>运行状态</v-list-item-title>
                <v-list-item-subtitle>{{ status.running ? '运行中' : '已停止' }}</v-list-item-subtitle>
              </v-list-item>
              <v-list-item>
                <template #prepend><v-icon color="blue" class="me-3">mdi-ip</v-icon></template>
                <v-list-item-title>监听地址</v-list-item-title>
                <v-list-item-subtitle>{{ status.listen_addr }}</v-list-item-subtitle>
              </v-list-item>
              <v-list-item>
                <template #prepend><v-icon color="green" class="me-3">mdi-account</v-icon></template>
                <v-list-item-title>机器人 QQ</v-list-item-title>
                <v-list-item-subtitle>{{ status.self_id }}</v-list-item-subtitle>
              </v-list-item>
              <v-list-item>
                <template #prepend><v-icon color="orange" class="me-3">mdi-lan-connect</v-icon></template>
                <v-list-item-title>WebSocket 连接数</v-list-item-title>
                <v-list-item-subtitle>{{ status.conn_count }} ({{ status.conn_ids?.join(', ') }})</v-list-item-subtitle>
              </v-list-item>
            </v-list>
            <v-btn color="primary" variant="tonal" class="mt-3 me-2" @click="restart" :loading="restarting">
              <v-icon class="me-1">mdi-restart</v-icon> 重启 Adapter
            </v-btn>
            <v-btn variant="tonal" @click="fetchStatus" :loading="loading">
              <v-icon class="me-1">mdi-refresh</v-icon> 刷新
            </v-btn>
          </v-card-text>
        </v-card>
      </v-col>

      <!-- Config Card -->
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">配置</span></template></v-card-item>
          <v-card-text>
            <v-form>
              <v-text-field v-model="config.addr" label="监听地址" class="mb-3" />
              <v-text-field v-model.number="config.port" label="端口" type="number" class="mb-3" />
              <v-text-field v-model="config.token" label="Access Token" class="mb-3" />
              <v-combobox
                v-model="config.admin_qq_numbers"
                label="Admins (QQ 号列表)"
                multiple
                chips
                closable-chips
                hint="回车添加, 支持多个管理员 QQ"
                class="mb-3"
              />
              <v-switch v-model="config.enabled" label="启用" color="primary" class="mb-3" />
              <v-btn color="primary" variant="tonal" @click="saveConfig" :loading="saving" block>保存配置</v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { adapterApi, type AdapterStatus, type UpdateAdapterConfigReq } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const restarting = ref(false); const saving = ref(false)
const status = ref<AdapterStatus>({ running: false, listen_addr: '', self_id: 0, conn_count: 0, conn_ids: [] })
const config = ref<UpdateAdapterConfigReq>({ addr: '0.0.0.0', port: 8080, token: '', admin_qq_numbers: [], enabled: false })

async function fetchStatus() { loading.value = true; try { const res = await adapterApi.getStatus(); status.value = res.data.data } catch { toastStore.error('获取状态失败') } finally { loading.value = false } }
async function fetchConfig() { try { const res = await adapterApi.getConfig(); const c = res.data.data; config.value = { addr: c.addr || '0.0.0.0', port: c.port || 8080, token: c.token || '', admin_qq_numbers: c.admin_qq_numbers || [], enabled: c.enabled ?? false } } catch { toastStore.error('获取配置失败') } }
async function restart() { restarting.value = true; try { await adapterApi.restart(); toastStore.success('重启成功'); await fetchStatus() } catch { toastStore.error('重启失败') } finally { restarting.value = false } }
async function saveConfig() { saving.value = true; try { await adapterApi.updateConfig(config.value); toastStore.success('配置已保存'); await fetchStatus() } catch { toastStore.error('保存失败') } finally { saving.value = false } }
onMounted(() => { fetchStatus(); fetchConfig() })
</script>
