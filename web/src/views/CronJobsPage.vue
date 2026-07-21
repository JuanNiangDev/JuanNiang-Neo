<template>
  <div>
    <div class="page-header">
      <div class="page-title">CronJob 定时任务</div>
      <div class="page-subtitle">在指定时间模拟 OneBot11 事件，注入 Agent 自动处理</div>
    </div>
    <div class="d-flex justify-end mb-4">
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增任务</v-btn>
    </div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.is_active="{ item }">
        <v-switch :model-value="item.is_active" color="primary" density="compact" hide-details
          @update:model-value="(v: any) => toggle(item.id, !!v)" />
      </template>
      <template #item.message_type="{ item }">
        <v-chip :color="item.message_type === 'group' ? 'info' : 'warning'" size="small" variant="tonal">
          {{ item.message_type === 'group' ? '群聊' : '私聊' }}
        </v-chip>
      </template>
      <template #item.last_run_at="{ item }">
        <span v-if="item.last_run_at">{{ new Date(item.last_run_at).toLocaleString() }}</span>
        <span v-else class="text-disabled">未执行</span>
      </template>
      <template #item.last_error="{ item }">
        <span v-if="item.last_error" class="text-error text-caption">{{ item.last_error }}</span>
        <span v-else class="text-disabled">-</span>
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" @click="openEdit(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <!-- 新增/编辑弹窗 -->
    <v-dialog v-model="dialog" max-width="640">
      <v-card rounded="lg">
        <v-card-title>{{ editing ? '编辑 CronJob' : '新增 CronJob' }}</v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-text-field v-model="form.name" label="任务名称" class="mb-3" />
            <v-text-field v-model="form.cron_expr" label="Cron 表达式" class="mb-3"
              hint="秒 分 时 日 月 周。例如: 0 0 8 * * * 表示每天8点" persistent-hint />
            <v-select v-model="form.message_type" :items="msgTypeOptions" label="消息类型" class="mb-3" />
            <v-text-field v-model.number="form.target_id" label="目标 ID (QQ号/群号)" type="number" class="mb-3" />
            <v-textarea v-model="form.message" label="消息内容" class="mb-3" rows="3"
              hint="该文本将作为用户消息发送给 Agent，Agent 会据此生成回复。不会发送 Markdown 格式。" />
            <v-switch v-model="form.is_active" label="启用" color="primary" />
          </v-form>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 删除确认 -->
    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>确认删除</v-card-title>
        <v-card-text>确定要删除此定时任务吗？</v-card-text>
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
import { cronJobApi, type CronJobResp, type AddCronJobReq } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true)
const items = ref<CronJobResp[]>([])
const dialog = ref(false); const deleteDialog = ref(false)
const editing = ref<string | null>(null); const saving = ref(false); const deleting = ref(false)
const deleteTarget = ref<CronJobResp | null>(null); const formRef = ref()

const msgTypeOptions = [
  { title: '私聊', value: 'private' },
  { title: '群聊', value: 'group' },
]

const headers = [
  { title: '名称', key: 'name' },
  { title: 'Cron 表达式', key: 'cron_expr' },
  { title: '消息类型', key: 'message_type' },
  { title: '目标 ID', key: 'target_id' },
  { title: '上次执行', key: 'last_run_at' },
  { title: '上次错误', key: 'last_error' },
  { title: '启用', key: 'is_active', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = (): AddCronJobReq => ({
  name: '', cron_expr: '', message: '', message_type: 'private', target_id: 0, is_active: true,
})

const form = ref<AddCronJobReq>(defaultForm())

async function fetch() {
  loading.value = true
  try { items.value = (await cronJobApi.list()).data.data }
  catch { toastStore.error('获取失败') }
  finally { loading.value = false }
}
function openAdd() { editing.value = null; form.value = defaultForm(); dialog.value = true }
function openEdit(item: CronJobResp) {
  editing.value = item.id
  form.value = {
    name: item.name, cron_expr: item.cron_expr, message: item.message,
    message_type: item.message_type, target_id: item.target_id, is_active: item.is_active,
  }
  dialog.value = true
}
async function toggle(id: string, v: boolean) {
  try { await cronJobApi.toggle(id, v); await fetch(); toastStore.success(v ? '已启用' : '已停用') }
  catch { toastStore.error('操作失败') }
}
async function handleSave() {
  saving.value = true
  try {
    if (editing.value) await cronJobApi.update(editing.value, form.value)
    else await cronJobApi.create(form.value)
    toastStore.success(editing.value ? '已更新' : '已创建')
    dialog.value = false
    await fetch()
  } catch (e: any) { toastStore.error(e?.message || '保存失败') }
  finally { saving.value = false }
}
function confirmDelete(item: CronJobResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try { await cronJobApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() }
  catch { toastStore.error('删除失败') }
  finally { deleting.value = false }
}
onMounted(fetch)
</script>
