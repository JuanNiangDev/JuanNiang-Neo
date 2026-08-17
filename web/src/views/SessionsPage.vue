<template>
  <div>
    <div class="page-header">
      <div class="page-title">Session 管理</div>
      <div class="page-subtitle">查看与删除会话状态</div>
    </div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.token_usage="{ item }">{{ formatNumber(item.token_usage) }}</template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-eye" size="small" variant="text" color="primary" @click="openDetail(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <!-- 详情弹窗: 只显示元数据, 不含聊天记录 -->
    <v-dialog v-model="detailDialog" max-width="900">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center justify-space-between pa-4">
          <div class="d-flex align-center" style="gap:12px">
            <v-icon color="primary">mdi-eye</v-icon>
            <span class="text-body-1">Session 元数据</span>
          </div>
          <v-btn icon="mdi-close" size="small" variant="text" @click="detailDialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-0">
          <v-row no-gutters>
            <!-- 左侧: 基本元信息 (1 份) -->
            <v-col cols="4" class="detail-left pa-4">
              <div class="text-caption text-medium-emphasis mb-1">Session ID</div>
              <div class="text-body-2 mb-4">{{ detail.id }}</div>

              <div class="text-caption text-medium-emphasis mb-1">Chat Area ID</div>
              <div class="text-body-2 mb-4">{{ detail.chat_area_id }}</div>

              <div class="text-caption text-medium-emphasis mb-1">模型</div>
              <div class="text-body-2 mb-4">{{ detail.model }}</div>

              <div class="text-caption text-medium-emphasis mb-1">Token 用量</div>
              <div class="text-body-2 mb-4">{{ formatNumber(detail.token_usage) }}</div>

              <div class="text-caption text-medium-emphasis mb-1">创建时间</div>
              <div class="text-body-2">{{ formatTime(detail.created_at) }}</div>
            </v-col>

            <!-- 右侧: meta_data JSON (2 份) -->
            <v-col cols="8" class="detail-right pa-4">
              <div class="d-flex align-center justify-space-between mb-2">
                <div class="text-caption text-medium-emphasis">元数据 (JSON, 不含聊天记录)</div>
                <v-btn
                  v-if="hasMeta"
                  size="x-small"
                  variant="text"
                  color="primary"
                  prepend-icon="mdi-content-copy"
                  @click="copyJson"
                >复制</v-btn>
              </div>
              <!-- 类 VSCode 只读代码框 -->
              <div class="code-viewer-wrapper">
                <div class="code-viewer">
                  <div class="line-numbers" ref="lineNumbersRef">
                    <div v-for="n in jsonLineCount" :key="n" class="line-num">{{ n }}</div>
                  </div>
                  <pre class="code-area" ref="codeAreaRef"><code>{{ jsonText }}</code></pre>
                </div>
              </div>
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider />
        <v-card-actions class="pa-4">
          <v-spacer />
          <v-btn variant="text" @click="detailDialog = false">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>删除 Session 将同时清除 Redis 消息缓存。</v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { sessionApi, type SessionResp } from '@/api'
import { useToastStore } from '@/stores/toast'
import { format } from 'date-fns'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<SessionResp[]>([])
const deleteDialog = ref(false); const deleting = ref(false); const deleteTarget = ref<SessionResp | null>(null)
const detailDialog = ref(false)
const detail = ref<SessionResp>(emptyDetail())
const lineNumbersRef = ref<HTMLDivElement | null>(null)
const codeAreaRef = ref<HTMLElement | null>(null)

const headers = [
  { title: 'ID', key: 'id' }, { title: 'Chat Area ID', key: 'chat_area_id' }, { title: '模型', key: 'model' },
  { title: 'Token 用量', key: 'token_usage' }, { title: '创建时间', key: 'created_at' },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

function emptyDetail(): SessionResp {
  return { id: '', chat_area_id: '', model: '', token_usage: 0, meta_data: {}, created_at: '' }
}

function formatNumber(n: number) { return n >= 1000 ? (n / 1000).toFixed(1) + 'K' : String(n) }
function formatTime(t: string) { try { return format(new Date(t), 'yyyy-MM-dd HH:mm:ss') } catch { return t } }

const hasMeta = computed(() => detail.value.meta_data && Object.keys(detail.value.meta_data).length > 0)
const jsonText = computed(() => {
  if (!detail.value.meta_data || Object.keys(detail.value.meta_data).length === 0) {
    return '// 无元数据'
  }
  try { return JSON.stringify(detail.value.meta_data, null, 2) } catch { return '// 序列化失败' }
})
const jsonLineCount = computed(() => jsonText.value.split('\n').length)

function openDetail(item: SessionResp) {
  detail.value = { ...item, meta_data: { ...(item.meta_data || {}) } }
  detailDialog.value = true
  nextTick(() => syncScroll())
}

function syncScroll() {
  const pre = codeAreaRef.value?.parentElement
  const ln = lineNumbersRef.value
  if (pre && ln) {
    pre.addEventListener('scroll', () => { ln.scrollTop = pre.scrollTop })
  }
}

async function copyJson() {
  try { await navigator.clipboard.writeText(jsonText.value); toastStore.success('已复制') } catch { toastStore.error('复制失败') }
}

async function fetch() { loading.value = true; try { items.value = (await sessionApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
function confirmDelete(item: SessionResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() { if (!deleteTarget.value) return; deleting.value = true; try { await sessionApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch { toastStore.error('删除失败') } finally { deleting.value = false } }
onMounted(fetch)
</script>

<style scoped>
.detail-left {
  background: rgba(var(--v-theme-surface-variant), 0.05);
  border-right: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  min-height: 360px;
}

.detail-right {
  min-height: 360px;
  display: flex;
  flex-direction: column;
}

.code-viewer-wrapper {
  border: 1px solid rgba(var(--v-theme-on-surface), 0.12);
  border-radius: 6px;
  overflow: hidden;
  background: rgb(var(--v-theme-code-bg));
  flex: 1;
  display: flex;
  flex-direction: column;
}

.code-viewer {
  display: flex;
  max-height: 420px;
  min-height: 280px;
}

.line-numbers {
  flex: 0 0 48px;
  background: rgb(var(--v-theme-code-bg));
  color: rgba(var(--v-theme-code-fg), 0.55);
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.5;
  text-align: right;
  padding: 12px 8px 12px 0;
  overflow-y: hidden;
  user-select: none;
  border-right: 1px solid rgba(var(--v-theme-code-fg), 0.18);
}

.line-num {
  height: calc(13px * 1.5);
}

.code-area {
  flex: 1;
  margin: 0;
  padding: 12px 16px;
  background: rgb(var(--v-theme-code-bg));
  color: rgb(var(--v-theme-code-fg));
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.5;
  overflow: auto;
  white-space: pre;
  tab-size: 2;
}

.code-area code {
  background: transparent;
  color: inherit;
  font: inherit;
  padding: 0;
}

.code-area::-webkit-scrollbar,
.line-numbers::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
.code-area::-webkit-scrollbar-thumb,
.line-numbers::-webkit-scrollbar-thumb {
  background: rgba(var(--v-theme-code-fg), 0.3);
  border-radius: 4px;
}
</style>
