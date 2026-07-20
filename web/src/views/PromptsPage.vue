<template>
  <div>
    <div class="page-header">
      <div class="page-title">Prompt 管理</div>
      <div class="page-subtitle">管理 System / Personality / Custom Prompt</div>
    </div>
    <div class="d-flex justify-end mb-4"><v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增 Prompt</v-btn></div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.name="{ item }">
        <span>{{ displayName(item) }}</span>
        <v-chip v-if="item.is_system" size="x-small" variant="flat" color="secondary" class="ml-2">系统</v-chip>
      </template>
      <template #item.type="{ item }">
        <v-chip size="small" variant="tonal" :color="typeColor(item.type)">{{ typeLabel(item.type) }}</v-chip>
      </template>
      <template #item.is_active="{ item }">
        <v-switch :model-value="item.is_active" color="primary" density="compact" hide-details :disabled="item.is_system" @update:model-value="(v) => toggle(item.id, !!v)" />
      </template>
      <template #item.content="{ item }">
        <div class="content-preview text-caption">{{ item.content?.slice(0, 80) }}{{ (item.content?.length ?? 0) > 80 ? '…' : '' }}</div>
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-eye" size="small" variant="text" color="info" @click="openView(item)" />
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" :disabled="item.is_system" @click="openEdit(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" :disabled="item.is_system" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <!-- 查看 / 编辑 弹窗（左右 1:2 布局） -->
    <v-dialog v-model="detailDialog" max-width="1100">
      <v-card rounded="lg" class="prompt-detail-card">
        <v-card-title class="d-flex align-center justify-space-between pa-4">
          <span>{{ dialogMode === 'view' ? '查看 Prompt' : dialogMode === 'edit' ? '编辑 Prompt' : '新增 Prompt' }}</span>
          <v-btn icon="mdi-close" size="small" variant="text" @click="detailDialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-0">
          <v-row no-gutters>
            <!-- 左侧：名称 + 类型 (1 份) -->
            <v-col cols="4" class="left-panel pa-4">
              <div class="text-caption text-medium-emphasis mb-2">名称</div>
              <v-text-field
                v-if="dialogMode !== 'view'"
                v-model="form.name"
                density="compact"
                variant="outlined"
                placeholder="请输入名称"
                hide-details
                class="mb-4"
              />
              <div v-else class="text-h6 font-weight-bold mb-4">{{ displayName(form) }}</div>

              <div class="text-caption text-medium-emphasis mb-2">类型</div>
              <v-select
                v-if="dialogMode !== 'view'"
                v-model="form.type"
                :items="typeOptions"
                item-title="label"
                item-value="value"
                density="compact"
                variant="outlined"
                hide-details
                class="mb-4"
              />
              <div v-else>
                <v-chip size="small" variant="tonal" :color="typeColor(form.type)">{{ typeLabel(form.type) }}</v-chip>
                <v-chip v-if="form.is_system" size="small" variant="flat" color="secondary" class="ml-2">系统锁定</v-chip>
              </div>

              <div v-if="dialogMode === 'view'" class="mt-6">
                <div class="text-caption text-medium-emphasis mb-1">创建时间</div>
                <div class="text-body-2">{{ formatTime(form.created_at) }}</div>
              </div>

              <div v-if="dialogMode !== 'view'" class="mt-4">
                <v-switch v-model="form.is_active" label="激活" color="primary" density="compact" hide-details />
              </div>
            </v-col>

            <!-- 右侧：内容 (2 份) -->
            <v-col cols="8" class="right-panel pa-4">
              <div class="d-flex align-center justify-space-between mb-2">
                <div class="text-caption text-medium-emphasis">内容</div>
                <div v-if="dialogMode === 'view'" class="text-caption text-medium-emphasis">Markdown 渲染预览</div>
                <div v-else class="text-caption text-medium-emphasis">Markdown 源码编辑（等宽编辑器）</div>
              </div>

              <!-- 查看模式：markdown 渲染 -->
              <div v-if="dialogMode === 'view'" class="markdown-viewer" v-html="renderedContent" />

              <!-- 编辑模式：类 VSCode 编辑器 -->
              <div v-else class="code-editor-wrapper">
                <div class="code-editor">
                  <div class="line-numbers" ref="lineNumbersRef">
                    <div v-for="n in lineCount" :key="n" class="line-num">{{ n }}</div>
                  </div>
                  <textarea
                    ref="editorRef"
                    v-model="form.content"
                    class="code-textarea"
                    spellcheck="false"
                    @scroll="onEditorScroll"
                    @input="updateLineNumbers"
                    placeholder="在此输入提示词内容（支持 Markdown）"
                  />
                </div>
              </div>
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider />
        <v-card-actions class="pa-4">
          <v-spacer />
          <v-btn variant="text" @click="detailDialog = false">{{ dialogMode === 'view' ? '关闭' : '取消' }}</v-btn>
          <v-btn v-if="dialogMode !== 'view'" color="primary" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>确定要删除此 Prompt 吗？</v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted } from 'vue'
import { promptApi, type PromptResp, type AddPromptReq } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true)
const items = ref<PromptResp[]>([])
const detailDialog = ref(false)
const dialogMode = ref<'view' | 'edit' | 'add'>('view')
const deleteDialog = ref(false)
const editing = ref<string | null>(null)
const saving = ref(false)
const deleting = ref(false)
const deleteTarget = ref<PromptResp | null>(null)
const editorRef = ref<HTMLTextAreaElement | null>(null)
const lineNumbersRef = ref<HTMLDivElement | null>(null)

const typeOptions = [
  { label: 'Personality', value: 'personality' },
  { label: 'Custom', value: 'custom' },
]

const headers = [
  { title: '名称', key: 'name' },
  { title: '类型', key: 'type' },
  { title: '内容预览', key: 'content' },
  { title: 'Active', key: 'is_active', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = (): AddPromptReq => ({ name: '', content: '', type: 'personality', is_active: false })
const form = ref<AddPromptReq & { is_system?: boolean; created_at?: string }>(defaultForm() as any)

const lineCount = computed(() => {
  const c = form.value.content ?? ''
  return c.split('\n').length
})

const renderedContent = computed(() => renderMarkdown(form.value.content ?? ''))

function displayName(item: any): string {
  if (item.name === '__system_locked__') return '系统提示词'
  return item.name
}

function typeLabel(t: string): string {
  if (t === 'system') return 'System'
  if (t === 'personality') return 'Personality'
  if (t === 'custom') return 'Custom'
  return t
}

function typeColor(t: string): string {
  if (t === 'system') return 'primary'
  if (t === 'personality') return 'warning'
  return 'info'
}

function formatTime(t?: string): string {
  if (!t) return '-'
  try {
    const d = new Date(t)
    return d.toLocaleString('zh-CN', { hour12: false })
  } catch { return t }
}

async function fetch() {
  loading.value = true
  try { items.value = (await promptApi.list()).data.data }
  catch { toastStore.error('获取失败') }
  finally { loading.value = false }
}

function openView(item: PromptResp) {
  dialogMode.value = 'view'
  form.value = { ...item } as any
  detailDialog.value = true
}

function openAdd() {
  dialogMode.value = 'add'
  editing.value = null
  form.value = defaultForm() as any
  detailDialog.value = true
  nextTick(() => updateLineNumbers())
}

function openEdit(item: PromptResp) {
  dialogMode.value = 'edit'
  editing.value = item.id
  form.value = { name: item.name, content: item.content, type: item.type, is_active: item.is_active } as any
  detailDialog.value = true
  nextTick(() => updateLineNumbers())
}

async function toggle(id: string, v: boolean) {
  try { await promptApi.toggle(id, v); await fetch(); toastStore.success(v ? '已启用' : '已停用') }
  catch { toastStore.error('操作失败') }
}

async function handleSave() {
  if (!form.value.name?.trim()) { toastStore.error('请输入名称'); return }
  if (!form.value.content?.trim()) { toastStore.error('请输入内容'); return }
  saving.value = true
  try {
    if (editing.value) await promptApi.update(editing.value, form.value)
    else await promptApi.create(form.value)
    toastStore.success(editing.value ? '已更新' : '已创建')
    detailDialog.value = false
    await fetch()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally { saving.value = false }
}

function confirmDelete(item: PromptResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await promptApi.delete(deleteTarget.value.id)
    toastStore.success('已删除')
    deleteDialog.value = false
    await fetch()
  } catch { toastStore.error('删除失败') }
  finally { deleting.value = false }
}

function onEditorScroll() {
  if (editorRef.value && lineNumbersRef.value) {
    lineNumbersRef.value.scrollTop = editorRef.value.scrollTop
  }
}

function updateLineNumbers() {
  // 触发 lineCount 重算（v-for 会响应）
}

// ---------- 轻量 Markdown 渲染器 ----------
function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function renderMarkdown(src: string): string {
  if (!src) return '<div class="md-empty">（空内容）</div>'
  const lines = src.replace(/\r\n/g, '\n').split('\n')
  const out: string[] = []
  let i = 0
  let inUl = false
  let inOl = false
  const closeLists = () => {
    if (inUl) { out.push('</ul>'); inUl = false }
    if (inOl) { out.push('</ol>'); inOl = false }
  }
  while (i < lines.length) {
    const line = lines[i]
    // 代码块
    if (line.startsWith('```')) {
      closeLists()
      const lang = line.slice(3).trim()
      const code: string[] = []
      i++
      while (i < lines.length && !lines[i].startsWith('```')) { code.push(lines[i]); i++ }
      i++ // 跳过结束符
      out.push(`<pre class="md-code"><code data-lang="${escapeHtml(lang)}">${escapeHtml(code.join('\n'))}</code></pre>`)
      continue
    }
    // 标题
    const hm = line.match(/^(#{1,6})\s+(.*)$/)
    if (hm) {
      closeLists()
      const lvl = hm[1].length
      out.push(`<h${lvl}>${renderInline(hm[2])}</h${lvl}>`)
      i++
      continue
    }
    // 无序列表
    const ulm = line.match(/^[-*+]\s+(.*)$/)
    if (ulm) {
      if (!inUl) { closeLists(); out.push('<ul>'); inUl = true }
      out.push(`<li>${renderInline(ulm[1])}</li>`)
      i++
      continue
    }
    // 有序列表
    const olm = line.match(/^\d+\.\s+(.*)$/)
    if (olm) {
      if (!inOl) { closeLists(); out.push('<ol>'); inOl = true }
      out.push(`<li>${renderInline(olm[1])}</li>`)
      i++
      continue
    }
    // 引用
    if (line.startsWith('> ')) {
      closeLists()
      out.push(`<blockquote>${renderInline(line.slice(2))}</blockquote>`)
      i++
      continue
    }
    // 分隔线
    if (/^(-{3,}|\*{3,}|_{3,})$/.test(line.trim())) {
      closeLists()
      out.push('<hr/>')
      i++
      continue
    }
    // 空行
    if (line.trim() === '') { closeLists(); i++; continue }
    // 普通段落
    closeLists()
    out.push(`<p>${renderInline(line)}</p>`)
    i++
  }
  closeLists()
  return out.join('\n')
}

function renderInline(s: string): string {
  // 先 escape 再处理 inline
  let r = escapeHtml(s)
  // 行内代码（先处理，避免内部被其他规则破坏）
  r = r.replace(/`([^`]+)`/g, '<code class="md-inline-code">$1</code>')
  // 加粗
  r = r.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  // 斜体
  r = r.replace(/(^|[^*])\*([^*]+)\*/g, '$1<em>$2</em>')
  // 链接
  r = r.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>')
  return r
}

onMounted(fetch)
</script>

<style scoped>
.content-preview {
  max-width: 360px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prompt-detail-card :deep(.left-panel) {
  background: rgba(var(--v-theme-surface-variant), 0.05);
  border-right: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  min-height: 480px;
}

.prompt-detail-card :deep(.right-panel) {
  min-height: 480px;
}

/* Markdown 渲染 */
.markdown-viewer {
  font-size: 14px;
  line-height: 1.7;
  max-height: 540px;
  overflow: auto;
  padding: 12px;
  background: rgba(var(--v-theme-surface), 0.6);
  border-radius: 8px;
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}

.markdown-viewer :deep(h1),
.markdown-viewer :deep(h2),
.markdown-viewer :deep(h3),
.markdown-viewer :deep(h4),
.markdown-viewer :deep(h5),
.markdown-viewer :deep(h6) {
  font-weight: 700;
  margin: 12px 0 8px;
  line-height: 1.3;
}
.markdown-viewer :deep(h1) { font-size: 1.5rem; }
.markdown-viewer :deep(h2) { font-size: 1.3rem; }
.markdown-viewer :deep(h3) { font-size: 1.15rem; }
.markdown-viewer :deep(h4) { font-size: 1rem; }
.markdown-viewer :deep(h5) { font-size: 0.95rem; }
.markdown-viewer :deep(h6) { font-size: 0.9rem; color: rgba(var(--v-theme-on-surface), 0.6); }

.markdown-viewer :deep(p) { margin: 6px 0; }
.markdown-viewer :deep(ul),
.markdown-viewer :deep(ol) { margin: 6px 0; padding-left: 24px; }
.markdown-viewer :deep(li) { margin: 3px 0; }

.markdown-viewer :deep(.md-code) {
  background: rgba(var(--v-theme-on-surface), 0.05);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 6px;
  padding: 10px 12px;
  overflow: auto;
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', 'Monaco', monospace;
  font-size: 12.5px;
  line-height: 1.5;
  margin: 8px 0;
}

.markdown-viewer :deep(.md-inline-code) {
  background: rgba(var(--v-theme-on-surface), 0.08);
  padding: 1px 5px;
  border-radius: 3px;
  font-family: 'JetBrains Mono', 'Consolas', monospace;
  font-size: 0.9em;
}

.markdown-viewer :deep(blockquote) {
  border-left: 3px solid rgba(var(--v-theme-primary), 0.5);
  padding: 4px 12px;
  margin: 8px 0;
  color: rgba(var(--v-theme-on-surface), 0.7);
}

.markdown-viewer :deep(hr) {
  border: none;
  border-top: 1px solid rgba(var(--v-theme-on-surface), 0.12);
  margin: 12px 0;
}

.markdown-viewer :deep(a) {
  color: rgb(var(--v-theme-primary));
  text-decoration: none;
}
.markdown-viewer :deep(a:hover) { text-decoration: underline; }

.markdown-viewer :deep(.md-empty) {
  color: rgba(var(--v-theme-on-surface), 0.4);
  font-style: italic;
}

/* 类 VSCode 编辑器 */
.code-editor-wrapper {
  border: 1px solid rgba(var(--v-theme-on-surface), 0.12);
  border-radius: 6px;
  overflow: hidden;
  background: #1e1e1e;
}

.code-editor {
  display: flex;
  max-height: 480px;
  min-height: 320px;
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

.code-textarea {
  flex: 1;
  background: #1e1e1e;
  color: #d4d4d4;
  border: none;
  outline: none;
  resize: none;
  padding: 12px 16px;
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre;
  overflow: auto;
  tab-size: 2;
}

.code-textarea::placeholder {
  color: #6a6a6a;
}

.code-textarea::-webkit-scrollbar,
.markdown-viewer::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
.code-textarea::-webkit-scrollbar-thumb,
.markdown-viewer::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.18);
  border-radius: 4px;
}
</style>
