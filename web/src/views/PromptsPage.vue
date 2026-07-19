<template>
  <div>
    <div class="page-header">
      <div class="page-title">Prompt 管理</div>
      <div class="page-subtitle">管理 System / Personality / Custom Prompt 模板</div>
    </div>
    <div class="d-flex justify-end mb-4"><v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增 Prompt</v-btn></div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.type="{ item }">
        <v-chip size="small" variant="tonal" :color="item.type==='system'?'primary':item.type==='personality'?'warning':'info'">{{ item.type }}</v-chip>
      </template>
      <template #item.is_active="{ item }">
        <v-switch :model-value="item.is_active" color="primary" density="compact" hide-details @update:model-value="(v: boolean) => toggle(item.id, v)" />
      </template>
      <template #item.content="{ item }">
        <div style="max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" class="text-caption">{{ item.content }}</div>
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" @click="openEdit(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <v-dialog v-model="dialog" max-width="600">
      <v-card rounded="lg">
        <v-card-title>{{ editing ? '编辑 Prompt' : '新增 Prompt' }}</v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-text-field v-model="form.name" label="名称" class="mb-3" />
            <v-select v-model="form.type" :items="['system','personality','custom']" label="类型" class="mb-3" />
            <v-textarea v-model="form.content" label="内容" rows="6" class="mb-3" />
            <v-switch v-model="form.is_active" label="激活" color="primary" />
          </v-form>
        </v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="dialog = false">取消</v-btn><v-btn color="primary" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>确定要删除此 Prompt 吗？</v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { promptApi, type PromptResp, type AddPromptReq } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<PromptResp[]>([]); const dialog = ref(false); const deleteDialog = ref(false)
const editing = ref<string | null>(null); const saving = ref(false); const deleting = ref(false)
const deleteTarget = ref<PromptResp | null>(null); const formRef = ref()

const headers = [
  { title: '名称', key: 'name' }, { title: '类型', key: 'type' }, { title: '内容', key: 'content' },
  { title: '变量', key: 'variables' }, { title: 'Active', key: 'is_active', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = (): AddPromptReq => ({ name: '', content: '', type: 'system', is_active: false, variables: [] })
const form = ref<AddPromptReq>(defaultForm())

async function fetch() { loading.value = true; try { items.value = (await promptApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
function openAdd() { editing.value = null; form.value = defaultForm(); dialog.value = true }
function openEdit(item: PromptResp) { editing.value = item.id; form.value = { name: item.name, content: item.content, type: item.type, is_active: item.is_active, variables: item.variables }; dialog.value = true }
async function toggle(id: string, v: boolean) { try { await promptApi.toggle(id, v); await fetch(); toastStore.success(v ? '已启用' : '已停用') } catch { toastStore.error('操作失败') } }
async function handleSave() { saving.value = true; try { if (editing.value) await promptApi.update(editing.value, form.value); else await promptApi.create(form.value); toastStore.success(editing.value ? '已更新' : '已创建'); dialog.value = false; await fetch() } catch (e: any) { toastStore.error(e?.message || '保存失败') } finally { saving.value = false } }
function confirmDelete(item: PromptResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() { if (!deleteTarget.value) return; deleting.value = true; try { await promptApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch { toastStore.error('删除失败') } finally { deleting.value = false } }
onMounted(fetch)
</script>
