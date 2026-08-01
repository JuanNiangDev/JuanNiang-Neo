<template>
  <div>
    <div class="page-header">
      <div class="page-title">Skill 管理</div>
      <div class="page-subtitle">管理关键词/正则触发的工具与 Prompt 组合</div>
    </div>
    <div class="d-flex justify-end mb-4"><v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增 Skill</v-btn></div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.is_active="{ item }"><v-chip size="small" :color="item.is_active?'success':'grey'" variant="tonal">{{ item.is_active ? '启用' : '停用' }}</v-chip></template>
      <template #item.keywords="{ item }">
        <v-chip v-for="kw in (item.keywords||[]).slice(0,3)" :key="kw" size="x-small" variant="outlined" class="me-1">{{ kw }}</v-chip>
        <span v-if="(item.keywords||[]).length > 3" class="text-caption">+{{ item.keywords.length - 3 }}</span>
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-eye" size="small" variant="text" color="info" @click="openView(item)" />
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" @click="openEdit(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <!-- 新增/编辑对话框 -->
    <v-dialog v-model="dialog" max-width="960">
      <v-card rounded="lg">
        <v-card-title>{{ editing ? '编辑 Skill' : '新增 Skill' }}</v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-row>
              <!-- 左侧：设置选项 -->
              <v-col cols="4">
                <v-text-field v-model="form.name" label="名称" density="compact" class="mb-3" />
                <v-text-field v-model="form.regex_pattern" label="正则模式" density="compact" class="mb-3" />
                <v-text-field v-model="form.priority" label="优先级" type="number" density="compact" class="mb-3" />
                <v-select
                  v-model="form.prompt_refs"
                  :items="promptOptions"
                  item-title="label"
                  item-value="value"
                  label="Prompt 引用"
                  multiple
                  chips
                  closable-chips
                  density="compact"
                  class="mb-3"
                />
                <v-combobox
                  v-model="form.keywords"
                  label="关键词"
                  multiple
                  chips
                  closable-chips
                  hint="回车添加关键词"
                  density="compact"
                  class="mb-3"
                />
                <v-select
                  v-model="form.tool_refs"
                  :items="toolOptions"
                  item-title="label"
                  item-value="value"
                  label="工具引用"
                  multiple
                  density="compact"
                  class="mb-3"
                />
                <v-select
                  v-model="form.mcp_refs"
                  :items="mcpOptions"
                  item-title="label"
                  item-value="value"
                  label="MCP 引用"
                  multiple
                  density="compact"
                  class="mb-3"
                />
                <v-switch v-model="form.is_active" label="激活" color="primary" density="compact" hide-details />
              </v-col>

              <!-- 右侧：描述 Markdown 编辑器 -->
              <v-col cols="8">
                <div class="text-subtitle-2 mb-2">描述 (Markdown)</div>
                <v-textarea
                  v-model="form.description"
                  label="Markdown 编辑"
                  rows="16"
                  hide-details
                  class="markdown-editor"
                />
              </v-col>
            </v-row>
          </v-form>
        </v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="dialog = false">取消</v-btn><v-btn color="primary" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 查看详情对话框 -->
    <v-dialog v-model="viewDialog" max-width="960">
      <v-card rounded="lg">
        <v-card-title>Skill 详情</v-card-title>
        <v-card-text>
          <v-row>
            <v-col cols="4">
              <div class="mb-3"><strong>名称:</strong> {{ viewItem?.name }}</div>
              <div class="mb-3"><strong>优先级:</strong> {{ viewItem?.priority }}</div>
              <div class="mb-3"><strong>状态:</strong> <v-chip size="small" :color="viewItem?.is_active?'success':'grey'" variant="tonal">{{ viewItem?.is_active ? '启用' : '停用' }}</v-chip></div>
              <div class="mb-3"><strong>正则模式:</strong> {{ viewItem?.regex_pattern || '-' }}</div>
              <div class="mb-3"><strong>Prompt 引用:</strong> {{ (viewItem?.prompt_refs||[]).join(', ') || '-' }}</div>
              <div class="mb-3">
                <strong>关键词:</strong>
                <template v-if="(viewItem?.keywords||[]).length">
                  <v-chip v-for="kw in viewItem?.keywords" :key="kw" size="x-small" variant="outlined" class="me-1 mt-1">{{ kw }}</v-chip>
                </template>
                <span v-else>-</span>
              </div>
              <div class="mb-3"><strong>工具引用:</strong> {{ (viewItem?.tool_refs||[]).join(', ') || '-' }}</div>
              <div class="mb-3"><strong>MCP 引用:</strong> {{ (viewItem?.mcp_refs||[]).join(', ') || '-' }}</div>
            </v-col>
            <v-col cols="8">
              <div class="text-subtitle-2 mb-2">描述</div>
              <div class="markdown-preview border rounded pa-3" style="min-height:200px" v-html="renderMarkdown(viewItem?.description || '')" />
            </v-col>
          </v-row>
        </v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="viewDialog = false">关闭</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400"><v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>确定要删除此 Skill 吗？</v-card-text><v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card></v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { skillApi, toolApi, mcpApi, promptApi, type SkillResp, type AddSkillReq, type ToolConfigResp, type MCPServerResp, type PromptResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<SkillResp[]>([]); const dialog = ref(false); const deleteDialog = ref(false); const viewDialog = ref(false)
const editing = ref<string | null>(null); const saving = ref(false); const deleting = ref(false)
const deleteTarget = ref<SkillResp | null>(null); const viewItem = ref<SkillResp | null>(null); const formRef = ref()
const toolOptions = ref<{label: string; value: string}[]>([])
const mcpOptions = ref<{label: string; value: string}[]>([])
const promptOptions = ref<{label: string; value: string}[]>([])

const headers = [
  { title: '名称', key: 'name' }, { title: '描述', key: 'description' }, { title: '关键词', key: 'keywords' },
  { title: '优先级', key: 'priority' },
  { title: '状态', key: 'is_active' }, { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = (): AddSkillReq => ({ name: '', is_active: false, description: '', keywords: [], regex_pattern: '', prompt_refs: [], tool_refs: [], mcp_refs: [], priority: 0 })
const form = ref<AddSkillReq>(defaultForm())

async function fetch() { loading.value = true; try { items.value = (await skillApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
async function fetchToolOptions() { try { const list = (await toolApi.list()).data.data || []; toolOptions.value = list.map((t: ToolConfigResp) => ({ label: `${t.name}${t.is_builtin ? ' (内置)' : ''}`, value: t.name })) } catch { /* ignore */ } }
async function fetchMcpOptions() { try { const list = (await mcpApi.list()).data.data || []; mcpOptions.value = list.map((m: MCPServerResp) => ({ label: m.name, value: m.name })) } catch { /* ignore */ } }
async function fetchPromptOptions() { try { const list = (await promptApi.list()).data.data || []; promptOptions.value = list.map((p: PromptResp) => ({ label: `${p.name}${p.is_system ? ' (系统)' : ''}`, value: p.id })) } catch { /* ignore */ } }
function openAdd() { editing.value = null; form.value = defaultForm(); dialog.value = true }
function openEdit(item: SkillResp) { editing.value = item.id; form.value = { name: item.name, is_active: item.is_active, description: item.description, keywords: item.keywords, regex_pattern: item.regex_pattern, prompt_refs: item.prompt_refs, tool_refs: item.tool_refs, mcp_refs: item.mcp_refs, priority: item.priority }; dialog.value = true }
function openView(item: SkillResp) { viewItem.value = item; viewDialog.value = true }
async function handleSave() { saving.value = true; try { if (editing.value) await skillApi.update(editing.value, form.value); else await skillApi.create(form.value); toastStore.success(editing.value ? '已更新' : '已创建'); dialog.value = false; await fetch() } catch (e: any) { toastStore.error(e?.message || '保存失败') } finally { saving.value = false } }
function confirmDelete(item: SkillResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() { if (!deleteTarget.value) return; deleting.value = true; try { await skillApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch { toastStore.error('删除失败') } finally { deleting.value = false } }

// 简单 Markdown 渲染
function renderMarkdown(md: string): string {
  if (!md) return ''
  let html = md
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  // 代码块 (``` ... ```)
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_m, lang, code) => `<pre><code>${code}</code></pre>`)
  // 行内代码 (`code`)
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>')
  // 标题
  html = html.replace(/^### (.+)$/gm, '<h3>$1</h3>')
  html = html.replace(/^## (.+)$/gm, '<h2>$1</h2>')
  html = html.replace(/^# (.+)$/gm, '<h1>$1</h1>')
  // 粗体
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
  // 斜体
  html = html.replace(/\*(.+?)\*/g, '<em>$1</em>')
  // 链接
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>')
  // 列表项
  html = html.replace(/^- (.+)$/gm, '<li>$1</li>')
  // 换行
  html = html.replace(/\n\n/g, '</p><p>')
  html = html.replace(/\n/g, '<br/>')
  html = '<p>' + html + '</p>'
  return html
}

onMounted(() => { fetch(); fetchToolOptions(); fetchMcpOptions(); fetchPromptOptions() })
</script>

<style scoped>
.markdown-editor :deep(textarea) {
  font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace;
  font-size: 0.9rem;
  line-height: 1.5;
}
.markdown-preview :deep(h1) { font-size: 1.25rem; margin: 0.5rem 0; }
.markdown-preview :deep(h2) { font-size: 1.1rem; margin: 0.4rem 0; }
.markdown-preview :deep(h3) { font-size: 1rem; margin: 0.3rem 0; }
.markdown-preview :deep(pre) { background: #f5f5f5; padding: 0.5rem; border-radius: 4px; overflow-x: auto; }
.markdown-preview :deep(code) { background: #f5f5f5; padding: 0.1rem 0.3rem; border-radius: 3px; font-size: 0.9em; }
.markdown-preview :deep(pre code) { background: none; padding: 0; }
.markdown-preview :deep(li) { margin-left: 1.2rem; }
.markdown-preview :deep(a) { color: #1976d2; }
</style>
