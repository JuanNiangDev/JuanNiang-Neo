<template>
  <div>
    <div class="page-header">
      <div class="page-title">MCP 服务器管理</div>
      <div class="page-subtitle">管理 Model Context Protocol 服务器连接</div>
    </div>
    <div class="d-flex justify-end mb-4">
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增 MCP</v-btn>
    </div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.is_active="{ item }">
        <v-switch :model-value="item.is_active" color="primary" density="compact" hide-details @update:model-value="(v: any) => toggle(item.id, !!v)" />
      </template>
      <template #item.auto_reconnect="{ item }">
        <v-chip :color="item.auto_reconnect ? 'success' : 'grey'" size="small" variant="tonal">{{ item.auto_reconnect ? '是' : '否' }}</v-chip>
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" @click="openEdit(item)" />
        <v-btn icon="mdi-check-circle" size="small" variant="text" color="success" :loading="checkingId === item.id" @click="doCheck(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <v-dialog v-model="dialog" max-width="640">
      <v-card rounded="lg">
        <v-card-title>{{ editing ? '编辑 MCP' : '新增 MCP' }}</v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-text-field v-model="form.name" label="名称" class="mb-3" />
            <v-text-field v-model="form.server_url" label="SSE 端点 URL" class="mb-3" />
            <v-text-field v-model="form.timeout" label="超时 (ms)" type="number" class="mb-3" />
            <v-text-field v-model="form.retry_count" label="重试次数" type="number" class="mb-3" />

            <!-- Headers 键值对编辑器 -->
            <div class="mb-3">
              <div class="d-flex align-center mb-2">
                <span class="text-caption font-weight-bold mr-2">Headers</span>
                <v-btn icon="mdi-plus-circle" size="x-small" variant="text" color="primary" @click="addHeader" />
              </div>
              <div v-for="(h, idx) in form.headerPairs" :key="idx" class="d-flex align-center mb-1" style="gap: 8px">
                <v-text-field v-model="h.key" label="Key" density="compact" hide-details style="flex: 1" />
                <v-text-field v-model="h.value" label="Value" density="compact" hide-details style="flex: 1" />
                <v-btn icon="mdi-close-circle" size="x-small" variant="text" color="error" @click="removeHeader(idx)" />
              </div>
            </div>

            <v-switch v-model="form.auto_reconnect" label="自动重连" color="primary" class="mb-2" />
            <v-switch v-model="form.is_active" label="激活" color="primary" />
          </v-form>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>确认删除</v-card-title>
        <v-card-text>确定要删除此 MCP 服务器吗？</v-card-text>
        <v-card-actions>
          <v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn>
          <v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { mcpApi, type MCPServerResp, type AddMCPServerReq } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true)
const checkingId = ref<string | null>(null)
const items = ref<MCPServerResp[]>([])
const dialog = ref(false); const deleteDialog = ref(false)
const editing = ref<string | null>(null); const saving = ref(false); const deleting = ref(false)
const deleteTarget = ref<MCPServerResp | null>(null); const formRef = ref()

const headers = [
  { title: '名称', key: 'name' }, { title: 'URL', key: 'server_url' }, { title: '超时(ms)', key: 'timeout' },
  { title: '重试', key: 'retry_count' }, { title: '自动重连', key: 'auto_reconnect' },
  { title: 'Active', key: 'is_active', align: 'center' as const }, { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = (): AddMCPServerReq & { headerPairs: { key: string; value: string }[] } => ({
  name: '', server_url: '', timeout: 30000, retry_count: 3, tool_filter: [], auto_reconnect: true, is_active: false,
  headerPairs: [],
})

const form = ref<ReturnType<typeof defaultForm>>(defaultForm())

function addHeader() { form.value.headerPairs.push({ key: '', value: '' }) }
function removeHeader(idx: number) { form.value.headerPairs.splice(idx, 1) }

function headersToObj(): Record<string, any> {
  const obj: Record<string, any> = {}
  for (const h of form.value.headerPairs) {
    if (h.key.trim()) obj[h.key.trim()] = h.value
  }
  return obj
}

function objToHeaderPairs(obj: Record<string, any>): { key: string; value: string }[] {
  return Object.entries(obj).map(([key, value]) => ({ key, value: String(value) }))
}

async function fetch() { loading.value = true; try { items.value = (await mcpApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
function openAdd() { editing.value = null; form.value = defaultForm(); dialog.value = true }
function openEdit(item: MCPServerResp) {
  editing.value = item.id
  form.value = {
    name: item.name, server_url: item.server_url,
    timeout: item.timeout, retry_count: item.retry_count, tool_filter: item.tool_filter,
    auto_reconnect: item.auto_reconnect, is_active: item.is_active,
    headerPairs: objToHeaderPairs(item.headers || {}),
  }
  dialog.value = true
}
async function toggle(id: string, v: boolean) { try { await mcpApi.toggle(id, v); await fetch(); toastStore.success(v ? '已启用' : '已停用') } catch { toastStore.error('操作失败') } }
async function handleSave() {
  saving.value = true
  try {
    const data: AddMCPServerReq = {
      name: form.value.name, server_url: form.value.server_url,
      headers: headersToObj(), timeout: form.value.timeout, retry_count: form.value.retry_count,
      tool_filter: form.value.tool_filter, auto_reconnect: form.value.auto_reconnect, is_active: form.value.is_active,
    }
    if (editing.value) await mcpApi.update(editing.value, data)
    else await mcpApi.create(data)
    toastStore.success(editing.value ? '已更新' : '已创建')
    dialog.value = false
    await fetch()
  } catch (e: any) { toastStore.error(e?.message || '保存失败') } finally { saving.value = false }
}
function confirmDelete(item: MCPServerResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() { if (!deleteTarget.value) return; deleting.value = true; try { await mcpApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch { toastStore.error('删除失败') } finally { deleting.value = false } }
async function doCheck(item: MCPServerResp) {
  checkingId.value = item.id
  try {
    const { data: resp } = await mcpApi.check(item.id)
    if (resp.data?.healthy) {
      toastStore.success(`${item.name}: 连接正常`)
    } else {
      toastStore.error(`${item.name}: ${resp.data?.error || '连接失败'}`)
    }
  } catch (e: any) {
    toastStore.error(`${item.name}: ${e?.message || '检测失败'}`)
  } finally {
    checkingId.value = null
  }
}
onMounted(fetch)
</script>
