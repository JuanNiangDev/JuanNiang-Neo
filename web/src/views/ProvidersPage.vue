<template>
  <div>
    <div class="page-header">
      <div class="page-title">Provider 管理</div>
      <div class="page-subtitle">管理 LLM 提供商配置（同类型仅一个 Active）</div>
    </div>
    <div class="d-flex justify-end mb-4">
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增 Provider</v-btn>
    </div>
    <v-data-table :headers="headers" :items="items" :loading="loading" items-per-page="20">
      <template #item.type="{ item }"><v-chip size="small" variant="tonal">{{ typeLabel(item.type) }}</v-chip></template>
      <template #item.enable_thinking="{ item }">
        <v-chip size="small" :color="item.enable_thinking ? 'primary' : 'default'" variant="tonal">{{ item.enable_thinking ? '开' : '关' }}</v-chip>
      </template>
      <template #item.is_active="{ item }">
        <v-switch :model-value="item.is_active" color="primary" density="compact" hide-details @update:model-value="(v) => toggle(item.id, !!v)" />
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" @click="openEdit(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <!-- Dialog -->
    <v-dialog v-model="dialog" max-width="560">
      <v-card rounded="lg">
        <v-card-title>{{ editing ? '编辑 Provider' : '新增 Provider' }}</v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-text-field v-model="form.name" label="名称" class="mb-3" />
            <v-select v-model="form.type" :items="types" label="类型" class="mb-3" />
            <v-text-field v-model="form.endpoint" label="API 地址" class="mb-3" />
            <v-text-field v-model="form.token" label="API Token" class="mb-3" />
            <v-text-field v-model="form.model" label="模型名" class="mb-3" />
            <v-text-field v-model="form.temperature" label="温度" type="number" step="0.1" class="mb-3" />
            <v-switch v-model="form.enable_thinking" label="模型思考" color="primary" class="mb-3" />
            <v-switch v-model="form.isActive" label="激活" color="primary" />
            <div class="text-caption text-medium-emphasis mt-2">
              模型思考开启后，请求会携带 thinking / enable_thinking 扩展参数（DeepSeek、通义千问等思考模式兼容）。
            </div>
          </v-form>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Delete confirm -->
    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>确认删除</v-card-title>
        <v-card-text>确定要删除此 Provider 吗？此操作不可撤销。</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="deleteDialog = false">取消</v-btn>
          <v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { providerApi, type ProviderResp, type AddProviderReq } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true)
const items = ref<ProviderResp[]>([])
const dialog = ref(false)
const deleteDialog = ref(false)
const editing = ref<string | null>(null)
const saving = ref(false)
const deleting = ref(false)
const deleteTarget = ref<ProviderResp | null>(null)
const formRef = ref()

const headers = [
  { title: '名称', key: 'name' },
  { title: '类型', key: 'type' },
  { title: '模型', key: 'model' },
  { title: '端点', key: 'endpoint' },
  { title: '温度', key: 'temperature' },
  { title: '思考', key: 'enable_thinking', align: 'center' as const },
  { title: 'Active', key: 'is_active', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const types = [
  { title: 'Text Model', value: 'text_model' },
  { title: 'Image Model', value: 'image_model' },
  { title: 'Embedding Model', value: 'embedding_model' },
]

const typeLabel = (t: string) => ({ text_model: 'Text', image_model: 'Image', embedding_model: 'Embedding' }[t] || t)

const defaultForm = (): AddProviderReq => ({ name: '', type: 'text_model', endpoint: '', token: '', model: '', temperature: 0.7, isActive: false, enable_thinking: false })
const form = ref<AddProviderReq>(defaultForm())

async function fetch() {
  loading.value = true
  try { items.value = (await providerApi.list()).data.data } catch (e: any) { toastStore.error('获取列表失败') } finally { loading.value = false }
}

function openAdd() { editing.value = null; form.value = defaultForm(); dialog.value = true }
function openEdit(item: ProviderResp) {
  editing.value = item.id
  form.value = { name: item.name, type: item.type, endpoint: item.endpoint, token: item.token, model: item.model, temperature: item.temperature, isActive: item.is_active, enable_thinking: item.enable_thinking }
  dialog.value = true
}

async function toggle(id: string, v: boolean) {
  try {
    await providerApi.toggle(id, v)
    toastStore.success(v ? '已启用' : '已停用')
    await fetch()
  } catch (e: any) { toastStore.error('操作失败') }
}

async function handleSave() {
  saving.value = true
  try {
    if (editing.value) { await providerApi.update(editing.value, form.value) } else { await providerApi.create(form.value) }
    toastStore.success(editing.value ? '已更新' : '已创建')
    dialog.value = false
    await fetch()
  } catch (e: any) { toastStore.error(e?.message || '保存失败') } finally { saving.value = false }
}

function confirmDelete(item: ProviderResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try { await providerApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch (e: any) { toastStore.error('删除失败') } finally { deleting.value = false }
}

onMounted(fetch)
</script>
