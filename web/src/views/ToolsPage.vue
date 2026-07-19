<template>
  <div>
    <div class="page-header">
      <div class="page-title">Tool 管理</div>
      <div class="page-subtitle">管理内置/自定义工具（只支持启用/停用）</div>
    </div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.is_builtin="{ item }"><v-chip size="small" :color="item.is_builtin?'primary':'grey'" variant="tonal">{{ item.is_builtin ? '内置' : '自定义' }}</v-chip></template>
      <template #item.is_active="{ item }">
        <v-switch :model-value="item.is_active" :disabled="!item.is_builtin" color="primary" density="compact" hide-details @update:model-value="(v: boolean) => toggle(item.id, v)" />
      </template>
      <template #item.parameters="{ item }">
        <v-btn size="x-small" variant="tonal" color="info" @click="showParams(item)">查看 Schema</v-btn>
      </template>
    </v-data-table>

    <v-dialog v-model="paramsDialog" max-width="500">
      <v-card rounded="lg"><v-card-title>参数 Schema: {{ paramsTarget?.name }}</v-card-title>
        <v-card-text><pre class="code-block">{{ JSON.stringify(paramsTarget?.parameters, null, 2) }}</pre></v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="paramsDialog = false">关闭</v-btn></v-card-actions></v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { toolApi, type ToolConfigResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<ToolConfigResp[]>([])
const paramsDialog = ref(false); const paramsTarget = ref<ToolConfigResp | null>(null)

const headers = [
  { title: '名称', key: 'name' }, { title: '描述', key: 'description' }, { title: '超时(ms)', key: 'timeout' },
  { title: '类型', key: 'is_builtin' }, { title: '参数', key: 'parameters' },
  { title: 'Active', key: 'is_active', align: 'center' as const },
]

async function fetch() { loading.value = true; try { items.value = (await toolApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
async function toggle(id: string, v: boolean) { try { await toolApi.toggle(id, v); await fetch(); toastStore.success(v ? '已启用' : '已停用') } catch { toastStore.error('操作失败') } }
function showParams(item: ToolConfigResp) { paramsTarget.value = item; paramsDialog.value = true }
onMounted(fetch)
</script>
