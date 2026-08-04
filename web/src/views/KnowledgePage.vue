<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-book-open-variant</v-icon>知识库</div>
      <div class="page-subtitle">SQL 驱动知识库：对话前自动模糊匹配并注入提示词</div>
    </div>

    <div class="d-flex justify-space-between align-center mb-4">
      <v-btn
        :color="showList ? 'secondary' : 'primary'"
        :variant="showList ? 'tonal' : 'flat'"
        :prepend-icon="showList ? 'mdi-eye-off' : 'mdi-eye'"
        @click="showList = !showList"
      >
        {{ showList ? '收起知识列表' : '查看知识列表' }}
      </v-btn>
      <v-btn variant="tonal" prepend-icon="mdi-refresh" :loading="statsLoading" @click="loadStats">刷新统计</v-btn>
    </div>

    <!-- 统计区域：LRU 缓存 + 词云 + 命中排行（非实时，点开/刷新拉取一次） -->
    <v-row>
      <!-- LRU 检索缓存 -->
      <v-col cols="12" md="4">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="d-flex align-center py-3">
            <v-icon class="me-2" color="primary">mdi-cache</v-icon>LRU 检索缓存
            <v-spacer />
            <v-chip size="small" variant="tonal">{{ lru.length }} / 50</v-chip>
          </v-card-title>
          <v-card-text>
            <template v-if="lru.length">
              <div v-for="e in lru" :key="e.key" class="d-flex align-center py-1 lru-row">
                <span class="text-body-2 text-truncate" style="max-width: 60%" :title="e.key">{{ e.key }}</span>
                <v-spacer />
                <span class="text-caption text-medium-emphasis me-3">命中 {{ e.hits }}</span>
                <span class="text-caption text-medium-emphasis">结果 {{ e.item_count }}</span>
              </div>
            </template>
            <div v-else class="text-body-2 text-medium-emphasis">
              暂无缓存 —— 对话触发知识检索后自动产生（缓存最近 50 条查询）
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <!-- 关键词词云 -->
      <v-col cols="12" md="4">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="py-3">
            <v-icon class="me-2" color="primary">mdi-cloud-tags</v-icon>关键词词云
          </v-card-title>
          <v-card-text>
            <div v-if="cloud.length" class="d-flex flex-wrap align-center justify-center cloud-wrap">
              <span
                v-for="c in cloud"
                :key="c.keyword"
                class="font-weight-medium cloud-tag"
                :style="cloudStyle(c)"
                :title="`${c.keyword}（${c.count} 条）`"
              >{{ c.keyword }}</span>
            </div>
            <div v-else class="text-body-2 text-medium-emphasis">暂无关键词 —— 知识提取完成后展示</div>
          </v-card-text>
        </v-card>
      </v-col>

      <!-- 关键词命中排行 -->
      <v-col cols="12" md="4">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="py-3">
            <v-icon class="me-2" color="primary">mdi-trophy-outline</v-icon>关键词命中排行
          </v-card-title>
          <v-card-text>
            <template v-if="hitRank.length">
              <div v-for="(h, idx) in hitRank" :key="h.keyword" class="d-flex align-center py-1">
                <span class="text-caption text-medium-emphasis" style="width: 26px">{{ idx + 1 }}</span>
                <span class="text-body-2 me-2 rank-keyword" :title="h.keyword">{{ h.keyword }}</span>
                <v-progress-linear :model-value="hitPercent(h)" height="6" rounded class="flex-grow-1 me-2" color="primary" />
                <span class="text-caption text-medium-emphasis" style="width: 42px; text-align: right">{{ h.hit_count }}</span>
              </div>
            </template>
            <div v-else class="text-body-2 text-medium-emphasis">暂无命中记录 —— 对话中命中知识关键词后统计</div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- 知识列表（默认隐藏，点击按钮显示） -->
    <v-card v-if="showList" rounded="lg" class="mt-4">
      <v-card-text>
        <div class="d-flex justify-end mb-3">
          <v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增知识</v-btn>
        </div>

        <v-data-table :headers="headers" :items="items" :loading="loading" :items-per-page="pageSize" :items-per-page-options="[10, 20, 50]" @update:options="onPageChange">
          <template #item.title="{ item }">
            <span class="font-weight-medium">{{ item.title || '(无标题)' }}</span>
          </template>
          <template #item.content="{ item }">
            <span class="text-body-2 text-medium-emphasis" style="max-width: 320px; display: inline-block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; vertical-align: bottom">{{ item.content }}</span>
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
        </v-data-table>
      </v-card-text>
    </v-card>

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
import { ref, onMounted, onUnmounted } from 'vue'
import { knowledgeApi, type KnowledgeResp, type KnowledgeLRUEntry, type KeywordCount, type KeywordHit } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
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

// ---------- 统计（非实时，进入页面/刷新时拉取一次） ----------
const statsLoading = ref(false)
const lru = ref<KnowledgeLRUEntry[]>([])
const cloud = ref<KeywordCount[]>([])
const hitRank = ref<KeywordHit[]>([])
const showList = ref(false)

const headers = [
  { title: '标题', key: 'title' },
  { title: '内容', key: 'content' },
  { title: '关键词', key: 'keywords' },
  { title: '提取状态', key: 'keyword_status', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = () => ({ title: '', content: '' })
const form = ref(defaultForm())

const statusLabel = (s: string) => ({ pending: '提取中', ready: '已就绪', failed: '失败' }[s] || s)
const statusColor = (s: string) => ({ pending: 'warning', ready: 'success', failed: 'error' }[s] || 'default')

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

async function loadStats() {
  statsLoading.value = true
  try {
    const res = (await knowledgeApi.stats()).data.data
    lru.value = res.lru || []
    cloud.value = res.cloud || []
    hitRank.value = res.hit_rank || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载统计失败')
  } finally {
    statsLoading.value = false
  }
}

// 词云：按出现条数比例缩放字号/颜色深浅
function cloudStyle(c: KeywordCount) {
  const max = Math.max(...cloud.value.map(x => x.count), 1)
  const ratio = c.count / max
  const size = 14 + Math.round(ratio * 20)
  const opacity = 0.55 + ratio * 0.45
  const hue = 200 + Math.round(ratio * 60)
  return { fontSize: `${size}px`, opacity, color: `hsl(${hue}, 70%, 42%)` }
}

// 命中排行进度条：相对最高值
function hitPercent(h: KeywordHit) {
  const max = Math.max(...hitRank.value.map(x => x.hit_count), 1)
  return (h.hit_count / max) * 100
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
  if (!form.value.content.trim()) return
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
    await loadStats()
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
    await loadStats()
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

onMounted(() => {
  fetch()
  loadStats()
})
onUnmounted(stopPolling)
</script>

<style scoped>
.lru-row {
  border-bottom: 1px dashed rgba(128, 128, 128, 0.25);
}
.lru-row:last-child {
  border-bottom: none;
}
.cloud-wrap {
  min-height: 130px;
  gap: 8px 14px;
}
.cloud-tag {
  line-height: 1.4;
  transition: transform 0.15s;
  cursor: default;
}
.cloud-tag:hover {
  transform: scale(1.1);
}
.rank-keyword {
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
