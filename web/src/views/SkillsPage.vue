<template>
  <div>
    <div class="page-header">
      <div class="page-title">Skill 管理</div>
      <div class="page-subtitle">管理关键词/正则触发的工具与 Prompt 组合</div>
    </div>
    <div class="d-flex justify-end mb-4"><v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增 Skill</v-btn></div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.is_system="{ item }"><v-chip size="small" :color="item.is_system?'warning':'grey'" variant="tonal">{{ item.is_system ? '内置' : '用户' }}</v-chip></template>
      <template #item.is_active="{ item }"><v-chip size="small" :color="item.is_active?'success':'grey'" variant="tonal">{{ item.is_active ? '启用' : '停用' }}</v-chip></template>
      <template #item.keywords="{ item }">
        <v-chip v-for="kw in (item.keywords||[]).slice(0,3)" :key="kw" size="x-small" variant="outlined" class="me-1">{{ kw }}</v-chip>
        <span v-if="(item.keywords||[]).length > 3" class="text-caption">+{{ item.keywords.length - 3 }}</span>
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" @click="openEdit(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <v-dialog v-model="dialog" max-width="640">
      <v-card rounded="lg">
        <v-card-title>{{ editing ? '编辑 Skill' : '新增 Skill' }}</v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-text-field v-model="form.name" label="名称" class="mb-3" />
            <v-textarea v-model="form.description" label="描述" rows="2" class="mb-3" />
            <v-text-field v-model="form.regex_pattern" label="正则模式" class="mb-3" />
            <v-text-field v-model="form.priority" label="优先级" type="number" class="mb-3" />
            <v-switch v-model="form.is_active" label="激活" color="primary" />
            <v-switch v-model="form.is_system" label="系统内置" color="warning" />
          </v-form>
        </v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="dialog = false">取消</v-btn><v-btn color="primary" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400"><v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>确定要删除此 Skill 吗？</v-card-text><v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card></v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { skillApi, type SkillResp, type AddSkillReq } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<SkillResp[]>([]); const dialog = ref(false); const deleteDialog = ref(false)
const editing = ref<string | null>(null); const saving = ref(false); const deleting = ref(false)
const deleteTarget = ref<SkillResp | null>(null); const formRef = ref()

const headers = [
  { title: '名称', key: 'name' }, { title: '描述', key: 'description' }, { title: '关键词', key: 'keywords' },
  { title: '类型', key: 'is_system' }, { title: '优先级', key: 'priority' },
  { title: '状态', key: 'is_active' }, { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = (): AddSkillReq => ({ name: '', is_active: false, description: '', keywords: [], regex_pattern: '', prompt_ref: '', tool_refs: [], mcp_refs: [], is_system: false, priority: 0 })
const form = ref<AddSkillReq>(defaultForm())

async function fetch() { loading.value = true; try { items.value = (await skillApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
function openAdd() { editing.value = null; form.value = defaultForm(); dialog.value = true }
function openEdit(item: SkillResp) { editing.value = item.id; form.value = { name: item.name, is_active: item.is_active, description: item.description, keywords: item.keywords, regex_pattern: item.regex_pattern, prompt_ref: item.prompt_ref, tool_refs: item.tool_refs, mcp_refs: item.mcp_refs, is_system: item.is_system, priority: item.priority }; dialog.value = true }
async function handleSave() { saving.value = true; try { if (editing.value) await skillApi.update(editing.value, form.value); else await skillApi.create(form.value); toastStore.success(editing.value ? '已更新' : '已创建'); dialog.value = false; await fetch() } catch (e: any) { toastStore.error(e?.message || '保存失败') } finally { saving.value = false } }
function confirmDelete(item: SkillResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() { if (!deleteTarget.value) return; deleting.value = true; try { await skillApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch { toastStore.error('删除失败') } finally { deleting.value = false } }
onMounted(fetch)
</script>
