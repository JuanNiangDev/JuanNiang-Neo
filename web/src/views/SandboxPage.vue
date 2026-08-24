<template>
  <div>
    <div class="page-header"><div class="page-title">Sandbox 配置</div><div class="page-subtitle">代码沙箱服务配置与健康管理</div></div>

    <v-row>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">配置</span></template></v-card-item>
          <v-card-text>
            <v-form>
              <v-text-field v-model="form.base_url" label="服务地址" class="mb-3" />
              <v-text-field v-model="form.api_key" label="API Key" class="mb-3" type="password" />
              <v-text-field v-model.number="form.timeout" label="超时 (ms)" type="number" class="mb-3" />
              <v-switch v-model="form.is_active" label="启用" color="primary" class="mb-3" />
              <v-btn color="primary" variant="tonal" block @click="save" :loading="saving">保存配置</v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1" class="h-100">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">健康状态</span></template></v-card-item>
          <v-card-text class="d-flex flex-column">
            <v-list density="compact">
              <v-list-item>
                <template #prepend><span class="status-dot" :class="config.healthy ? 'active' : 'error'" /></template>
                <v-list-item-title>健康状态</v-list-item-title>
                <v-list-item-subtitle>{{ config.healthy ? '健康' : '异常' }}</v-list-item-subtitle>
              </v-list-item>
            </v-list>
            <v-btn variant="tonal" class="mt-auto pt-1" @click="checkHealth" :loading="checking" block>
              <v-icon class="me-1">mdi-heart-pulse</v-icon> 检查健康
            </v-btn>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { sandboxApi, type SandboxConfigResp, type UpdateSandboxConfigReq } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const saving = ref(false); const checking = ref(false)
const config = ref<SandboxConfigResp>({ base_url: '', api_key: '', timeout: 60000, is_active: false, healthy: false })
const form = ref<UpdateSandboxConfigReq>({ base_url: '', api_key: '', timeout: 60000, is_active: false })

async function fetchConfig() { try { const res = await sandboxApi.getConfig(); config.value = res.data.data; form.value = { base_url: config.value.base_url, api_key: config.value.api_key, timeout: config.value.timeout, is_active: config.value.is_active } } catch { toastStore.error('获取配置失败') } }
async function save() { saving.value = true; try { const res = await sandboxApi.updateConfig(form.value); config.value = res.data.data; toastStore.success('已保存') } catch { toastStore.error('保存失败') } finally { saving.value = false } }
async function checkHealth() { checking.value = true; try { const res = await sandboxApi.health(); config.value.healthy = res.data.data.healthy; toastStore.info(config.value.healthy ? '健康检查通过' : '健康检查失败') } catch { toastStore.error('健康检查失败') } finally { checking.value = false } }
onMounted(fetchConfig)
</script>
