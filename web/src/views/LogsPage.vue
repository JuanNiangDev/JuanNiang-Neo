<template>
  <div>
    <div class="page-header">
      <div class="page-title">系统日志</div>
      <div class="page-subtitle">最近 250 条日志记录</div>
    </div>

    <div class="d-flex align-center mb-4" style="gap:12px">
      <v-btn color="primary" variant="tonal" prepend-icon="mdi-refresh" @click="fetch" :loading="loading">刷新</v-btn>
      <v-select
        v-model="levelFilter"
        :items="levelOptions"
        item-title="label"
        item-value="value"
        label="级别过滤"
        density="compact"
        hide-details
        style="max-width:180px"
      />
      <v-spacer />
      <v-chip size="small" variant="tonal" color="grey">{{ filtered.length }} 条</v-chip>
    </div>

    <v-card rounded="lg" elevation="1">
      <v-data-table
        :headers="headers"
        :items="filtered"
        :loading="loading"
        item-value="rowKey"
        density="compact"
        hover
        @click:row="openDetail"
        :items-per-page="50"
      >
        <template #item.level="{ item }">
          <v-chip size="x-small" variant="tonal" :color="levelColor(item.level)">{{ item.level }}</v-chip>
        </template>
        <template #item.module="{ item }">
          <v-chip size="x-small" variant="flat" color="grey-darken-1" class="text-caption">{{ item.module || '-' }}</v-chip>
        </template>
        <template #item.time="{ item }">
          <span class="text-caption text-medium-emphasis">{{ formatTime(item.time) }}</span>
        </template>
        <template #item.message="{ item }">
          <div class="log-msg">{{ item.message }}</div>
        </template>
        <template #item.attrs="{ item }">
          <v-chip
            v-if="item.attrs && Object.keys(item.attrs).length > 0"
            size="x-small"
            variant="outlined"
            color="info"
          >
            <v-icon size="x-small" start>mdi-code-json</v-icon>
            {{ Object.keys(item.attrs).length }} 字段
          </v-chip>
          <span v-else class="text-caption text-medium-emphasis">-</span>
        </template>
      </v-data-table>
    </v-card>

    <!-- 详情弹窗 -->
    <v-dialog v-model="detailDialog" max-width="900">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center justify-space-between pa-4">
          <div class="d-flex align-center" style="gap:12px">
            <v-chip size="small" variant="tonal" :color="levelColor(detail.level)">{{ detail.level }}</v-chip>
            <span class="text-body-1">日志详情</span>
          </div>
          <v-btn icon="mdi-close" size="small" variant="text" @click="detailDialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-0">
          <v-row no-gutters>
            <!-- 左侧：元信息 (1 份) -->
            <v-col cols="4" class="detail-left pa-4">
              <div class="text-caption text-medium-emphasis mb-1">时间</div>
              <div class="text-body-2 mb-4">{{ formatTimeFull(detail.time) }}</div>

              <div class="text-caption text-medium-emphasis mb-1">级别</div>
              <div class="mb-4">
                <v-chip size="small" variant="tonal" :color="levelColor(detail.level)">{{ detail.level }}</v-chip>
              </div>

              <div class="text-caption text-medium-emphasis mb-1">模块</div>
              <div class="mb-4"><v-chip size="small" variant="flat" color="grey-darken-1">{{ detail.module || '-' }}</v-chip></div>

              <div class="text-caption text-medium-emphasis mb-1">消息</div>
              <div class="text-body-2 mb-4">{{ detail.message }}</div>

              <!-- Rich 诊断信息 (WARN/ERROR) -->
              <template v-if="detail.rich && Object.keys(detail.rich).length">
                <v-divider class="my-3" />
                <div class="text-caption text-medium-emphasis mb-2">诊断信息</div>
                <div v-if="detail.rich.caller_file" class="text-caption mb-1 font-weight-bold">调用位置</div>
                <code v-if="detail.rich.caller_file" class="text-caption d-block mb-2" style="word-break:break-all">{{ detail.rich.caller_file }}</code>
                <div v-if="detail.rich.goroutines" class="text-caption">Goroutines: <strong>{{ detail.rich.goroutines }}</strong></div>
              </template>
            </v-col>

            <!-- 右侧：JSON 详情 (2 份) -->
            <v-col cols="8" class="detail-right pa-4">
              <div class="d-flex align-center justify-space-between mb-2">
                <div class="text-caption text-medium-emphasis">属性 (JSON)</div>
                <v-btn
                  v-if="hasAttrs"
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
                  <pre class="code-area"><code ref="codeAreaRef">{{ jsonText }}</code></pre>
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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { logApi, type LogEntryResp } from '@/api'
import { useToastStore } from '@/stores/toast'
import { format } from 'date-fns'

const toastStore = useToastStore()
const loading = ref(true)
const logs = ref<(LogEntryResp & { rowKey: number })[]>([])
const levelFilter = ref('ALL')
const detailDialog = ref(false)
const detail = ref<LogEntryResp & { rowKey: number }>(emptyDetail())
const lineNumbersRef = ref<HTMLDivElement | null>(null)
const codeAreaRef = ref<HTMLElement | null>(null)

const levelOptions = [
  { label: 'ALL', value: 'ALL' },
  { label: 'INFO', value: 'INFO' },
  { label: 'WARN', value: 'WARN' },
  { label: 'ERROR', value: 'ERROR' },
  { label: 'DEBUG', value: 'DEBUG' },
]

const headers = [
  { title: '级别', key: 'level', width: '80px' },
  { title: '模块', key: 'module', width: '100px' },
  { title: '时间', key: 'time', width: '150px' },
  { title: '消息', key: 'message' },
  { title: '属性', key: 'attrs', width: '100px', align: 'center' as const },
]

const filtered = computed(() => {
  if (levelFilter.value === 'ALL' || !levelFilter.value) return logs.value
  return logs.value.filter(l => l.level === levelFilter.value)
})

const hasAttrs = computed(() => detail.value.attrs && Object.keys(detail.value.attrs).length > 0)

const jsonText = computed(() => {
  if (!detail.value.attrs || Object.keys(detail.value.attrs).length === 0) {
    return '// 无附加属性'
  }
  try {
    return JSON.stringify(detail.value.attrs, null, 2)
  } catch {
    return '// 序列化失败'
  }
})

const jsonLineCount = computed(() => jsonText.value.split('\n').length)

function emptyDetail(): LogEntryResp & { rowKey: number } {
  return { time: '', level: '', module: '', message: '', attrs: {}, rowKey: 0 }
}

const levelColor = (lvl: string): string =>
  (({ INFO: 'info', WARN: 'warning', ERROR: 'error', DEBUG: 'grey' }) as Record<string, string>)[lvl] || 'grey'

function formatTime(t: string) {
  try { return format(new Date(t), 'MM-dd HH:mm:ss') } catch { return t }
}

function formatTimeFull(t: string) {
  try { return format(new Date(t), 'yyyy-MM-dd HH:mm:ss.SSS') } catch { return t }
}

function openDetail(_e: unknown, payload: any) {
  // Vuetify 3 不同版本 @click:row 的 payload 结构不一致：
  //   - 3.4.x: payload = { item: <raw>, internalItem: {...}, ... }
  //   - 3.5+:  payload = { item: { value, raw, ... } }  或直接 raw
  // 此处兼容多种结构
  const raw = payload?.item?.raw ?? payload?.item?.value ?? payload?.item ?? payload?.row ?? payload
  if (!raw || !raw.time) return
  detail.value = { ...raw }
  detailDialog.value = true
  nextTick(() => syncScroll())
}

function syncScroll() {
  // pre 与行号区滚动同步
  const pre = codeAreaRef.value?.parentElement
  const ln = lineNumbersRef.value
  if (pre && ln) {
    pre.addEventListener('scroll', () => { ln.scrollTop = pre.scrollTop })
  }
}

async function copyJson() {
  try {
    await navigator.clipboard.writeText(jsonText.value)
    toastStore.success('已复制')
  } catch {
    toastStore.error('复制失败')
  }
}

async function fetch() {
  loading.value = true
  try {
    const list = ((await logApi.list()).data.data || []) as LogEntryResp[]
    logs.value = list.map((l: LogEntryResp, i: number) => ({ ...l, rowKey: i }))
  } catch {
    toastStore.error('获取日志失败')
  } finally { loading.value = false }
}

onMounted(fetch)
</script>

<style scoped>
.log-msg {
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}

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

/* 类 VSCode 只读代码框 */
.code-viewer-wrapper {
  border: 1px solid rgba(var(--v-theme-on-surface), 0.12);
  border-radius: 6px;
  overflow: hidden;
  background: #1e1e1e;
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
  background: #1e1e1e;
  color: #858585;
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.5;
  text-align: right;
  padding: 12px 8px 12px 0;
  overflow-y: hidden;
  user-select: none;
  border-right: 1px solid #2d2d30;
}

.line-num {
  height: calc(13px * 1.5);
}

.code-area {
  flex: 1;
  margin: 0;
  padding: 12px 16px;
  background: #1e1e1e;
  color: #d4d4d4;
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
  background: rgba(255, 255, 255, 0.18);
  border-radius: 4px;
}

:deep(.v-data-table__tr) {
  cursor: pointer;
}
:deep(.v-data-table__tr:hover) {
  background: rgba(var(--v-theme-primary), 0.04) !important;
}
</style>
