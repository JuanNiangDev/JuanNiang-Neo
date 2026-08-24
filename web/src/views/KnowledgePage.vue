<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-book-open-variant</v-icon>知识库</div>
      <div class="page-subtitle">对话前自动匹配并注入提示词（首选 RAG 语义检索，未配置时降级 SQL 匹配）</div>
    </div>

    <div class="d-flex justify-end mb-4" style="gap: 12px">
      <v-btn variant="tonal" color="info" prepend-icon="mdi-database-sync" @click="syncVector" :loading="syncing">同步向量库</v-btn>
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增知识</v-btn>
    </div>
    <v-progress-linear v-if="syncProgress.active" color="info" :model-value="100" indeterminate height="4" class="mb-2 rounded" />
    <div v-if="syncProgress.active" class="text-caption text-info mb-2">向量同步中：已写入 {{ syncProgress.done }} 条{{ syncProgress.failed > 0 ? `，失败 ${syncProgress.failed} 条` : '' }}</div>

    <v-data-table-server :headers="headers" :items="items" :loading="loading" :page="page" :items-per-page="pageSize" :items-length="total" :items-per-page-options="[10, 20, 50]" @update:options="onPageChange">
      <template #item.title="{ item }">
        <span class="font-weight-medium">{{ item.title || '(无标题)' }}</span>
      </template>
      <template #item.content="{ item }">
        <span class="text-body-2 text-medium-emphasis" style="max-width: 320px; display: inline-block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; vertical-align: middle">{{ item.content }}</span>
      </template>
      <template #item.keywords="{ item }">
        <div v-if="item.keyword_status === 'ready' && item.keywords?.length">
          <v-chip v-for="kw in item.keywords.slice(0, 5)" :key="kw" size="x-small" variant="tonal" color="primary" class="me-1 mb-1">{{ kw }}</v-chip>
          <span v-if="item.keywords.length > 5" class="text-caption text-medium-emphasis">+{{ item.keywords.length - 5 }}</span>
        </div>
        <span v-else class="text-caption text-medium-emphasis">-</span>
      </template>
      <template #item.keyword_status="{ item }">
        <v-chip size="small" :color="statusColor(item.keyword_status)" variant="tonal">
          <v-progress-circular v-if="item.keyword_status === 'pending'" indeterminate size="14" width="2" class="me-1" />
          {{ statusLabel(item.keyword_status) }}
        </v-chip>
      </template>
      <template #item.actions="{ item }">
        <v-btn v-if="item.keyword_status === 'failed'" icon="mdi-refresh" size="small" variant="text" color="warning" title="重试提取关键词" :loading="extractingId === item.id" @click="reExtract(item)" />
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" title="编辑" @click="openEdit(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" title="删除" @click="confirmDelete(item)" />
      </template>
    </v-data-table-server>

    <!-- 新增/编辑弹窗 -->

    <!-- 新增/编辑弹窗 -->
    <v-dialog v-model="dialog" max-width="640">
      <v-card rounded="lg">
        <v-card-title>{{ editing ? '编辑知识' : '新增知识' }}</v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-text-field v-model="form.title" label="标题（可选）" class="mb-3" />
            <v-textarea v-model="form.content" label="知识内容" rows="8" counter class="mb-2" />
            <div class="text-caption text-medium-emphasis">
              保存后系统会异步提取关键词用于对话匹配；提取完成前该条暂不参与匹配。
            </div>
          </v-form>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" @click="handleSave" :loading="saving" :disabled="!form.content.trim()">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 删除确认 -->
    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>确认删除</v-card-title>
        <v-card-text>确定要删除这条知识吗？此操作不可撤销。</v-card-text>
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
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { knowledgeApi, type KnowledgeResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const syncing = ref(false)
// SSE 流式同步进度（知识量大时避免单次 HTTP 超时）
const syncProgress = ref({ active: false, done: 0, failed: 0 })

// 手动全量同步知识库到 RAG 向量库：SSE 流式（GET /knowledge/vector-sync/stream），逐批推送进度
async function syncVector() {
  syncing.value = true
  syncProgress.value = { active: true, done: 0, failed: 0 }
  try {
    const token = localStorage.getItem('token') || ''
    const res = await window.fetch('/api/v1/knowledge/vector-sync/stream', {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (!res.ok || !res.body) throw new Error('向量同步失败（RAG-Service 未配置或不可达）')

    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buf += decoder.decode(value, { stream: true })
      const blocks = buf.split('\n\n')
      buf = blocks.pop() ?? ''
      for (const block of blocks) {
        const dataLine = block.split('\n').find(l => l.startsWith('data:'))
        if (!dataLine) continue
        let data: any
        try { data = JSON.parse(dataLine.slice(5).trim()) } catch { continue }
        if (data.message) { toastStore.warning(data.message); return } // RAG 未启用
        if (data.total !== undefined) {
          // done 事件
          toastStore.success(`向量同步完成：共 ${data.total} 条，成功 ${data.synced}，失败 ${data.failed}`)
          syncProgress.value = { active: false, done: data.synced, failed: data.failed ?? 0 }
          return
        }
        if (data.done !== undefined) {
          syncProgress.value = { active: true, done: data.done, failed: data.failed ?? 0 }
        }
      }
    }
    throw new Error('同步中断（连接关闭）')
  } catch (e: any) {
    syncProgress.value.active = false
    toastStore.error(e?.message || e?.response?.data?.info || '向量同步失败')
  } finally { syncing.value = false }
}

const loading = ref(true)
const saving = ref(false)
const deleting = ref(false)
const extractingId = ref<string | null>(null)
const items = ref<KnowledgeResp[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const dialog = ref(false)
const deleteDialog = ref(false)
const editing = ref<string | null>(null)
const deleteTarget = ref<KnowledgeResp | null>(null)

// 后端列表仅支持分页（固定排序），所有列禁用排序，避免 v-data-table-server
// 触发无效的排序请求/误导性排序 UI。
const headers = [
  { title: '标题', key: 'title', sortable: false },
  { title: '内容', key: 'content', sortable: false },
  { title: '关键词', key: 'keywords', sortable: false },
  { title: '提取状态', key: 'keyword_status', align: 'center' as const, sortable: false },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = () => ({ title: '', content: '' })
const form = ref(defaultForm())

const statusLabel = (s: string) => ({ pending: '提取中', ready: '已就绪', failed: '失败' }[s] || s)
const statusColor = (s: string) => ({ pending: 'warning', ready: 'success', failed: 'error' }[s] || 'default')

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

async function fetch(silent = false) {
  if (!silent) loading.value = true
  try {
    const res = (await knowledgeApi.list(page.value, pageSize.value)).data.data
    items.value = res.list || []
    total.value = res.total || 0
    if (hasPending()) startPolling()
    else stopPolling()
  } catch (e: any) {
    toastStore.error(e?.message || '获取列表失败')
  } finally {
    if (!silent) loading.value = false
  }
}

function onPageChange(opts: any) {
  page.value = opts.page || 1
  if (opts.itemsPerPage) pageSize.value = opts.itemsPerPage
  fetch()
}

function openAdd() {
  editing.value = null
  form.value = defaultForm()
  dialog.value = true
}

function openEdit(item: KnowledgeResp) {
  editing.value = item.id
  form.value = { title: item.title, content: item.content }
  dialog.value = true
}

async function handleSave() {
  saving.value = true
  try {
    if (editing.value) {
      await knowledgeApi.update(editing.value, form.value)
      toastStore.success('已更新，关键词重新提取中')
    } else {
      await knowledgeApi.create(form.value)
      toastStore.success('已新增，关键词提取中')
    }
    dialog.value = false
    await fetch()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function confirmDelete(item: KnowledgeResp) {
  deleteTarget.value = item
  deleteDialog.value = true
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await knowledgeApi.delete(deleteTarget.value.id)
    toastStore.success('已删除')
    deleteDialog.value = false
    await fetch()
  } catch (e: any) {
    toastStore.error(e?.message || '删除失败')
  } finally {
    deleting.value = false
  }
}

async function reExtract(item: KnowledgeResp) {
  extractingId.value = item.id
  try {
    await knowledgeApi.reExtract(item.id)
    toastStore.success('已重新提交关键词提取')
    item.keyword_status = 'pending'
    startPolling()
  } catch (e: any) {
    toastStore.error(e?.message || '操作失败')
  } finally {
    extractingId.value = null
  }
}

// ---------- 提取状态轮询 ----------
// 异步提取不阻塞消息处理，前端定时拉取列表直到没有 pending 条目。
let pollTimer: ReturnType<typeof setInterval> | null = null

function hasPending() {
  return items.value.some(i => i.keyword_status === 'pending')
}

function startPolling() {
  if (pollTimer) return
  pollTimer = setInterval(async () => {
    await fetch(true)
  }, 3000)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

onMounted(() => fetch())
onUnmounted(stopPolling)
</script>
