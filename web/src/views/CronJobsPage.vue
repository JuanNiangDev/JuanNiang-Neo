<template>
  <div>
    <div class="page-header">
      <div class="page-title">CronJob 定时任务</div>
      <div class="page-subtitle">在指定时间触发已配置插件的 on_cronjob 回调</div>
    </div>
    <div class="d-flex justify-end mb-4">
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增任务</v-btn>
    </div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.is_active="{ item }">
        <v-switch :model-value="item.is_active" color="primary" density="compact" hide-details
          @update:model-value="(v: any) => toggle(item.id, !!v)" />
      </template>
      <template #item.plugin_ids="{ item }">
        <div class="d-flex flex-wrap" style="gap:4px" v-if="item.plugin_ids && item.plugin_ids.length > 0">
          <v-chip v-for="pid in item.plugin_ids" :key="pid" size="x-small" variant="tonal" color="success">{{ pid }}</v-chip>
        </div>
        <span v-else class="text-disabled">未配置</span>
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
    <v-dialog v-model="dialog" max-width="700">
      <v-card rounded="lg">
        <v-card-title>{{ editing ? '编辑 CronJob' : '新增 CronJob' }}</v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-text-field v-model="form.name" label="任务名称" class="mb-3" />
            <v-text-field v-model="form.cron_expr" label="Cron 表达式" class="mb-3"
              hint="秒 分 时 日 月 周。例如: 0 0 8 * * * 表示每天8点" persistent-hint />
            <v-switch v-model="form.is_active" label="启用" color="primary" class="mb-3" />

            <v-divider class="mb-3" />

            <v-select
              v-model="form.plugin_ids"
              :items="cronPlugins"
              item-title="name"
              item-value="id"
              label="触发插件"
              multiple
              chips
              closable-chips
              class="mb-3"
              hint="仅显示支持 on_cronjob 且已启用的插件"
              persistent-hint
            />
            <div class="text-caption text-medium-emphasis mb-1">Payload (JSON)</div>
            <div class="editor-wrapper mb-3">
              <Codemirror
                v-model="form.payload"
                :extensions="cmExtensions"
                :style="{ height: '200px' }"
                :indent-with-tab="true"
                :tab-size="2"
                placeholder='{"target_qq": 123456, "message": "定时提醒"}'
              />
            </div>
            <div v-if="payloadError" class="text-error text-caption mb-2">{{ payloadError }}</div>
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
import { ref, computed, onMounted } from 'vue'
import { Codemirror } from 'vue-codemirror'
import { json } from '@codemirror/lang-json'
import { basicSetup } from 'codemirror'
import { cronJobApi, type CronJobResp, type AddCronJobReq } from '@/api'
import { pluginApi } from '@/api'
import { useToastStore } from '@/stores/toast'
import { cmCodeMirrorTheme } from '@/theme/codemirrorTheme'

const toastStore = useToastStore()
const loading = ref(true)
const items = ref<CronJobResp[]>([])
const dialog = ref(false); const deleteDialog = ref(false)
const editing = ref<string | null>(null); const saving = ref(false); const deleting = ref(false)
const deleteTarget = ref<CronJobResp | null>(null); const formRef = ref()

const headers = [
  { title: '名称', key: 'name' },
  { title: 'Cron 表达式', key: 'cron_expr' },
  { title: '触发插件', key: 'plugin_ids' },
  { title: '上次执行', key: 'last_run_at' },
  { title: '上次错误', key: 'last_error' },
  { title: '启用', key: 'is_active', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = (): AddCronJobReq => ({
  name: '', cron_expr: '', is_active: true,
  plugin_ids: [], payload: '',
})

const form = ref<AddCronJobReq>(defaultForm())

// CodeMirror 配置（主题随深浅色自动适配）
const cmExtensions = [basicSetup, json(), ...cmCodeMirrorTheme()]

// JSON payload 实时校验
const payloadError = computed(() => {
  const v = form.value.payload?.trim()
  if (!v) return ''
  try { JSON.parse(v); return '' }
  catch (e: any) { return `JSON 格式错误: ${e.message}` }
})

// 获取支持 on_cronjob 且启用的插件列表
const cronPlugins = ref<Array<{ id: string; name: string }>>([])
async function loadCronPlugins() {
  try {
    const res = await pluginApi.list()
    const plugins = (res.data.data || []) as any[]
    cronPlugins.value = plugins
      .filter((p: any) => p.supports_cronjob && p.is_active)
      .map((p: any) => ({ id: p.id || p.name, name: `${p.name} (v${p.version})` }))
  } catch { /* ignore */ }
}

async function fetch() {
  loading.value = true
  try { items.value = (await cronJobApi.list()).data.data }
  catch { toastStore.error('获取失败') }
  finally { loading.value = false }
}
function openAdd() {
  editing.value = null; form.value = defaultForm()
  loadCronPlugins(); dialog.value = true
}
function openEdit(item: CronJobResp) {
  editing.value = item.id
  form.value = {
    name: item.name, cron_expr: item.cron_expr, is_active: item.is_active,
    plugin_ids: item.plugin_ids || [], payload: item.payload || '',
  }
  loadCronPlugins()
  dialog.value = true
}
async function toggle(id: string, v: boolean) {
  try { await cronJobApi.toggle(id, v); await fetch(); toastStore.success(v ? '已启用' : '已停用') }
  catch { toastStore.error('操作失败') }
}
async function handleSave() {
  if (!form.value.name?.trim()) { toastStore.error('请输入任务名称'); return }
  if (!form.value.cron_expr?.trim()) { toastStore.error('请输入 Cron 表达式'); return }
  if (!form.value.plugin_ids || form.value.plugin_ids.length === 0) { toastStore.error('请至少选择一个触发插件'); return }
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

<style scoped>
.editor-wrapper {
  border: 1px solid rgba(var(--v-theme-on-surface), 0.12);
  border-radius: 4px;
  overflow: hidden;
}
.editor-wrapper :deep(.cm-editor) {
  outline: none !important;
}
.editor-wrapper :deep(.cm-focused) {
  outline: none !important;
}
</style>