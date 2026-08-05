<template>
  <div>
    <div class="page-header">
      <div class="page-title">Plugin 管理</div>
      <div class="page-subtitle">管理 Lua 插件，支持 ZIP 上传</div>
    </div>
    <div class="d-flex justify-end mb-4" style="gap:12px">
      <v-btn color="warning" variant="tonal" prepend-icon="mdi-refresh" @click="handleReloadAll" :loading="reloading">重载全部</v-btn>
      <v-btn color="primary" variant="tonal" prepend-icon="mdi-upload" @click="triggerUpload">上传 ZIP</v-btn>
      <input ref="fileInput" type="file" accept=".zip" style="display:none" @change="handleFile" />
    </div>

    <!-- 网格卡片布局 -->
    <div v-if="filteredItems.length === 0" class="pa-8 text-center text-medium-emphasis">
      <v-icon size="48" class="mb-2">mdi-puzzle-outline</v-icon>
      <div>暂无插件，点击右上角「上传 ZIP」添加</div>
    </div>
    <v-container fluid class="pa-0">
      <v-row class="d-flex flex-wrap">
        <v-col v-for="item in filteredItems" :key="item.id || item.name" cols="12" sm="6" md="4" lg="3">
          <v-card rounded="lg" elevation="1" class="plugin-card" @click="openDetail(item)">
            <div class="d-flex card-body">
              <!-- 左侧矩形图片区 -->
              <div class="card-thumb">
                <v-img
                  v-if="avatarSrc[item.id || item.name]"
                  :src="avatarSrc[item.id || item.name]"
                  cover
                  class="card-thumb-img"
                />
                <div v-else class="card-thumb-ph">
                  <v-icon size="32" color="primary">mdi-puzzle</v-icon>
                </div>
              </div>
              <!-- 右侧信息区 -->
              <div class="d-flex flex-column card-info flex-grow-1">
                <div class="d-flex align-center" style="gap:6px">
                  <div class="text-subtitle-1 font-weight-bold card-title text-truncate">{{ item.name }}</div>
                  <v-chip v-if="item.is_system" size="x-small" color="error" variant="tonal">系统</v-chip>
                  <v-chip size="x-small" variant="tonal" color="grey">v{{ item.version }}</v-chip>
                </div>
                <div class="text-caption text-medium-emphasis card-desc">{{ item.description || '无描述' }}</div>
                <div class="d-flex align-center justify-end card-actions">
                  <PluginEnableToggle
                    :model-value="!!item.is_active"
                    :disabled="!!item.is_system"
                    @update:model-value="(v) => toggle(item.id || item.name, v)"
                  />
                </div>
              </div>
            </div>
          </v-card>
        </v-col>
      </v-row>
    </v-container>

    <!-- 详情弹窗: README / 元数据+命令 / 配置 三页签 -->
    <!-- 注意：必须显式传 max-width，否则全局默认 VDialog.maxWidth=600 会把宽度压到 600 -->
    <v-dialog v-model="detailDialog" width="1100" max-width="calc(100vw - 32px)">
      <v-card rounded="lg" class="detail-card">
        <v-card-title class="d-flex align-center pa-4">
          <v-avatar size="40" rounded="lg" class="me-3">
            <v-img v-if="detailAvatar" :src="detailAvatar" contain />
            <v-icon v-else color="primary">mdi-puzzle</v-icon>
          </v-avatar>
          <div>
            <div class="text-subtitle-1 font-weight-bold">{{ detail?.name }}</div>
            <div class="text-caption text-medium-emphasis">v{{ detail?.version }} · by {{ detail?.author || '未知' }}</div>
          </div>
          <v-spacer />
          <v-btn icon="mdi-close" size="small" variant="text" @click="detailDialog = false" />
        </v-card-title>
        <v-divider />
        <div class="d-flex detail-main">
          <!-- 左侧纵向 Tab 栏 -->
          <v-tabs v-model="tab" direction="vertical" color="primary" class="detail-tabs">
            <v-tab value="readme"><v-icon start>mdi-text-box-outline</v-icon>说明</v-tab>
            <v-tab value="meta"><v-icon start>mdi-code-json</v-icon>元数据</v-tab>
            <v-tab value="config"><v-icon start>mdi-tune-variant</v-icon>配置</v-tab>
          </v-tabs>
          <v-divider vertical />
          <v-card-text class="px-4 pb-4 pt-4 detail-body">
            <!-- README -->
            <v-window v-model="tab">
            <v-window-item value="readme">
              <div v-if="readmeLoading" class="text-center pa-8"><v-progress-circular indeterminate /></div>
              <div v-else-if="readmeContent" class="markdown-body" v-html="renderedReadme" />
              <div v-else class="text-center pa-8 text-medium-emphasis">该插件没有说明文档</div>
            </v-window-item>

            <!-- 元数据 + 命令 -->
            <v-window-item value="meta">
              <v-row dense>
                <v-col cols="12" md="6">
                  <div class="text-caption text-medium-emphasis mb-1">名称</div>
                  <div class="text-body-2 mb-3">{{ detail?.name }}</div>
                  <div class="text-caption text-medium-emphasis mb-1">版本</div>
                  <div class="text-body-2 mb-3">{{ detail?.version }}</div>
                  <div class="text-caption text-medium-emphasis mb-1">作者</div>
                  <div class="text-body-2 mb-3">{{ detail?.author || '(未设置)' }}</div>
                  <div class="text-caption text-medium-emphasis mb-1">系统插件</div>
                  <div class="text-body-2 mb-3">
                    <v-chip size="x-small" :color="detail?.is_system ? 'error' : 'grey'" variant="tonal">{{ detail?.is_system ? '是' : '否' }}</v-chip>
                  </div>
                </v-col>
                <v-col cols="12" md="6">
                  <div class="text-caption text-medium-emphasis mb-1">描述</div>
                  <div class="text-body-2 mb-3" style="white-space: pre-wrap; word-break: break-word">{{ detail?.description || '(无描述)' }}</div>
                  <div class="text-caption text-medium-emphasis mb-1">权限</div>
                  <div class="d-flex flex-wrap mb-3" style="gap:4px">
                    <v-chip v-for="p in (detail?.permissions || [])" :key="p" size="x-small" variant="tonal" color="info">{{ p }}</v-chip>
                    <span v-if="!detail?.permissions || detail.permissions.length === 0" class="text-caption text-medium-emphasis">(无)</span>
                  </div>
                </v-col>
              </v-row>
              <v-divider class="my-4" />
              <div class="text-caption text-medium-emphasis mb-2">注册命令</div>
              <div v-if="detailCommands.length > 0">
                <v-data-table :headers="cmdHeaders" :items="detailCommands" density="compact" hide-default-footer :items-per-page="-1">
                  <template #item.path="{ item }">
                    <code class="cmd-code">/{{ (item.path || []).join(' ') }}</code>
                  </template>
                </v-data-table>
              </div>
              <div v-else class="text-caption text-medium-emphasis">该插件没有注册命令</div>
            </v-window-item>

            <!-- 配置 -->
            <v-window-item value="config">
              <div v-if="configLoading" class="text-center pa-8"><v-progress-circular indeterminate /></div>
              <div v-else-if="configItems.length === 0" class="text-center pa-8 text-medium-emphasis">暂无可配置项</div>
              <div v-else>
                <div v-for="cfg in configItems" :key="cfg.key" class="mb-4 config-item">
                  <div class="d-flex align-center justify-space-between">
                    <div>
                      <div class="text-body-2 font-weight-bold">{{ cfg.label || cfg.key }}</div>
                      <div v-if="cfg.description" class="text-caption text-medium-emphasis">{{ cfg.description }}</div>
                    </div>
                  </div>
                  <v-switch
                    v-if="cfg.type === 'bool'"
                    :model-value="!!configForm[cfg.key]"
                    color="primary"
                    density="compact"
                    hide-details
                    @update:model-value="(v) => setConfigValue(cfg.key, !!v)"
                  />
                  <v-text-field
                    v-else-if="cfg.type === 'string'"
                    :model-value="configForm[cfg.key]"
                    variant="outlined"
                    density="compact"
                    hide-details
                    persistent-placeholder
                    :placeholder="String(cfg.default ?? '')"
                    @update:model-value="(v) => setConfigValue(cfg.key, v)"
                  />
                  <div v-else-if="cfg.type === 'list'" class="list-editor">
                    <div v-for="(_, idx) in configForm[cfg.key] as any[]" :key="idx" class="d-flex align-center mb-2" style="gap:8px">
                      <v-text-field
                        :model-value="(configForm[cfg.key] as any[])[idx]"
                        variant="outlined"
                        density="compact"
                        hide-details
                        @update:model-value="(v) => setListValue(cfg.key, idx, v)"
                      />
                      <v-btn icon="mdi-minus" size="small" variant="text" color="error" @click="removeListValue(cfg.key, idx)" />
                    </div>
                    <v-btn size="small" variant="tonal" color="primary" prepend-icon="mdi-plus" @click="addListValue(cfg.key)">添加一项</v-btn>
                  </div>
                </div>
                <div class="d-flex justify-end mt-4">
                  <v-btn color="primary" :loading="savingConfig" @click="handleSaveConfig">保存配置</v-btn>
                </div>
              </div>
            </v-window-item>
          </v-window>
          </v-card-text>
        </div>
        <v-divider />
        <v-card-actions class="pa-4">
          <PluginEnableToggle
            :model-value="!!detail?.is_active"
            :disabled="!!detail?.is_system"
            @update:model-value="(v) => toggle(detailId, v)"
          />
          <v-spacer />
          <v-btn color="info" variant="tonal" prepend-icon="mdi-refresh" :loading="reloadingOne" @click="handleReloadOne">重载</v-btn>
          <v-btn color="error" variant="tonal" prepend-icon="mdi-delete" :disabled="detail?.is_system" @click="confirmDelete">删除</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>确定要删除插件「{{ deleteTarget?.name }}」吗？</v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { marked } from 'marked'
import { pluginApi } from '@/api'
import { useToastStore } from '@/stores/toast'
import PluginEnableToggle from '@/components/shared/PluginEnableToggle.vue'

interface PluginItem {
  id?: string
  name: string
  version: string
  author?: string
  description?: string
  permissions?: string[]
  is_system?: boolean
  is_active?: boolean
  supports_cronjob?: boolean
  commands?: Array<{ path: string[]; description: string; usage: string; is_leaf: boolean }>
}

interface ConfigItem {
  key: string
  type: 'bool' | 'string' | 'list'
  label: string
  description?: string
  default?: any
  value?: any
  options?: string[]
}

const toastStore = useToastStore()
const loading = ref(true)
const reloading = ref(false)
const reloadingOne = ref(false)
const items = ref<PluginItem[]>([])
const filteredItems = computed(() => items.value)
const fileInput = ref<HTMLInputElement | null>(null)
const deleteDialog = ref(false)
const deleting = ref(false)
const deleteTarget = ref<PluginItem | null>(null)

// 头像缓存: id -> dataURL
const avatarSrc = ref<Record<string, string>>({})

// 详情弹窗
const detailDialog = ref(false)
const detail = ref<PluginItem | null>(null)
const detailId = computed(() => detail.value?.id || detail.value?.name || '')
const detailAvatar = computed(() => avatarSrc.value[detailId.value])
const tab = ref('readme')
// 请求序号：防止快速切换插件时旧请求覆盖新内容
let detailSeq = 0

// README
const readmeLoading = ref(false)
const readmeContent = ref('')
const renderedReadme = computed(() => readmeContent.value ? marked.parse(readmeContent.value) as string : '')

// 配置
const configLoading = ref(false)
const configItems = ref<ConfigItem[]>([])
const configForm = ref<Record<string, any>>({})
const savingConfig = ref(false)

const cmdHeaders = [
  { title: '命令', key: 'path' },
  { title: '描述', key: 'description' },
  { title: '用法', key: 'usage' },
]
const detailCommands = computed(() => detail.value?.commands || [])

async function fetch() {
  loading.value = true
  try {
    const list = (await pluginApi.list()).data.data || []
    items.value = list
    // 预取启用插件的头像
    for (const item of list) {
      const id = item.id || item.name
      if (!avatarSrc.value[id]) {
        loadAvatar(id)
      }
    }
  }
  catch { toastStore.error('获取失败') }
  finally { loading.value = false }
}

async function loadAvatar(id: string) {
  try {
    const res = await pluginApi.avatar(id)
    // 无头像时后端返回 JSON blob，需过滤，仅接受图片
    if (!res.data || typeof res.data !== 'object' || !res.data.type || !String(res.data.type).startsWith('image/')) return
    const url = URL.createObjectURL(res.data)
    avatarSrc.value[id] = url
  } catch { /* 无头像 */ }
}

function triggerUpload() { fileInput.value?.click() }
async function handleFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  try { await pluginApi.upload(f); toastStore.success('上传成功'); await fetch() }
  catch (err: any) { toastStore.error(err?.message || '上传失败') }
}

async function handleReloadAll() {
  reloading.value = true
  try { await pluginApi.reloadAll(); toastStore.success('已重载全部插件'); await fetch() }
  catch (e: any) { toastStore.error(e?.response?.data?.info || '重载失败') }
  finally { reloading.value = false }
}

async function toggle(id: string, v: boolean) {
  try { await pluginApi.toggle(id, v); await fetch(); toastStore.success(v ? '已启用' : '已停用') }
  catch (e: any) { toastStore.error(e?.response?.data?.info || '操作失败') }
}

async function openDetail(item: PluginItem) {
  const seq = ++detailSeq
  detail.value = item
  detailDialog.value = true
  tab.value = 'readme'
  readmeContent.value = ''
  configItems.value = []
  configForm.value = {}
  loadDetailReadme(item, seq)
  loadDetailConfig(item, seq)
}

async function loadDetailReadme(item: PluginItem, seq?: number) {
  const id = item.id || item.name
  readmeLoading.value = true
  try {
    const res = await pluginApi.readme(id)
    if (seq !== undefined && seq !== detailSeq) return // 已切换插件，丢弃过期响应
    readmeContent.value = res.data?.data?.content || ''
  } catch { if (seq === undefined || seq === detailSeq) readmeContent.value = '' }
  finally { if (seq === undefined || seq === detailSeq) readmeLoading.value = false }
}

async function loadDetailConfig(item: PluginItem, seq?: number) {
  const id = item.id || item.name
  configLoading.value = true
  try {
    const res = await pluginApi.config(id)
    if (seq !== undefined && seq !== detailSeq) return // 已切换插件，丢弃过期响应
    const schema: ConfigItem[] = res.data?.data?.schema || []
    const values: Record<string, any> = res.data?.data?.values || {}
    configItems.value = schema
    configForm.value = {}
    for (const cfg of schema) {
      configForm.value[cfg.key] = values[cfg.key] ?? cfg.default ?? normalizeDefault(cfg.type)
    }
  } catch { if (seq === undefined || seq === detailSeq) configItems.value = [] }
  finally { if (seq === undefined || seq === detailSeq) configLoading.value = false }
}

function normalizeDefault(type: string) {
  if (type === 'list') return []
  if (type === 'bool') return false
  return ''
}

function setConfigValue(key: string, v: any) {
  configForm.value[key] = v
}
function setListValue(key: string, idx: number, v: any) {
  const arr = [...(configForm.value[key] as any[])]
  arr[idx] = v
  configForm.value[key] = arr
}
function addListValue(key: string) {
  if (!Array.isArray(configForm.value[key])) configForm.value[key] = []
  ;(configForm.value[key] as any[]).push('')
}
function removeListValue(key: string, idx: number) {
  const arr = [...(configForm.value[key] as any[])]
  arr.splice(idx, 1)
  configForm.value[key] = arr
}

async function handleSaveConfig() {
  const id = detailId.value
  if (!id) return
  savingConfig.value = true
  try {
    await pluginApi.saveConfig(id, configForm.value)
    toastStore.success('配置已保存，插件已重载')
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || '保存失败')
  } finally { savingConfig.value = false }
}

async function handleReloadOne() {
  const id = detailId.value
  if (!id) return
  reloadingOne.value = true
  try { await pluginApi.reload(id); toastStore.success('已重载'); await fetch() }
  catch (e: any) { toastStore.error(e?.response?.data?.info || '重载失败') }
  finally { reloadingOne.value = false }
}

function confirmDelete() {
  if (detail.value?.is_system) { toastStore.error('系统插件不允许删除'); return }
  deleteTarget.value = detail.value
  deleteDialog.value = true
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    const id = deleteTarget.value.id || deleteTarget.value.name
    await pluginApi.delete(id)
    toastStore.success('已删除')
    deleteDialog.value = false
    detailDialog.value = false
    await fetch()
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || '删除失败')
  } finally { deleting.value = false }
}

watch(() => detailDialog.value, (open) => {
  if (!open) {
    deleteTarget.value = null
  }
})

onMounted(fetch)
</script>

<style scoped>
.plugin-card { cursor: pointer; transition: transform 0.15s ease, box-shadow 0.15s ease; }
.plugin-card:hover { transform: translateY(-2px); box-shadow: 0 4px 16px rgba(0,0,0,0.12) !important; }

/* 横向卡片：左图右文 */
.card-body { display: flex; min-height: 124px; }
.card-thumb {
  width: 124px;
  min-width: 124px;
  border-radius: 12px 0 0 12px;
  overflow: hidden;
  display: flex;
}
.card-thumb :deep(.v-img) { width: 100%; height: 100%; }
.card-thumb-ph {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(var(--v-theme-primary), 0.08);
}
.card-info { padding: 10px 12px; min-width: 0; }
.card-title { line-height: 1.3; }
.card-desc {
  flex: 1 1 auto;
  margin-top: 4px;
  min-height: 0;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  word-break: break-word;
}
.card-actions { margin-top: 8px; }

.markdown-body { word-break: break-word; overflow-x: auto; }
.markdown-body :deep(table) { border-collapse: collapse; width: 100%; margin: 8px 0; font-size: 13px; }
.markdown-body :deep(th), .markdown-body :deep(td) { border: 1px solid rgba(var(--v-theme-on-surface), 0.15); padding: 6px 10px; text-align: left; vertical-align: top; }
.markdown-body :deep(th) { background: rgba(var(--v-theme-on-surface), 0.05); font-weight: 600; white-space: nowrap; }
.markdown-body :deep(tr:nth-child(even)) { background: rgba(var(--v-theme-on-surface), 0.03); }
.markdown-body :deep(h1), .markdown-body :deep(h2), .markdown-body :deep(h3) { margin-top: 0.8em; margin-bottom: 0.4em; }
.markdown-body :deep(pre) { background: rgba(var(--v-theme-on-surface), 0.06); padding: 12px; border-radius: 6px; overflow: auto; }
.markdown-body :deep(code) { font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace; font-size: 12px; }
.markdown-body :deep(img) { max-width: 100%; }
.cmd-code { font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace; font-size: 12px; padding: 2px 6px; background: rgba(var(--v-theme-on-surface), 0.06); border-radius: 4px; }
/* 弹窗尺寸固定：宽 1100（小屏自适应），高 80vh，不随内容变化；整体居中 */
.detail-card {
  width: 100%;
  height: 80vh;
  min-height: 420px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 中部：左侧纵向 tab + 右侧内容区 */
.detail-main {
  flex: 1 1 auto;
  min-height: 0;
}
.detail-tabs {
  width: 120px;
  min-width: 120px;
  flex-shrink: 0;
  padding: 8px 0;
}
.detail-tabs :deep(.v-slide-group__container) { overflow-y: auto; }
.detail-body {
  flex: 1 1 0;
  min-width: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.detail-body :deep(.v-window) { flex: 1 1 auto; min-height: 0; }
.detail-body :deep(.v-window__container) { height: 100%; }
.detail-body :deep(.v-window-item) { height: 100%; overflow-y: auto; }

/* 第一个 Tab 页内容与 tab 栏的间距：首元素去掉自带 margin，仅保留内容区 padding */
.markdown-body :deep(h1:first-child),
.markdown-body :deep(h2:first-child),
.markdown-body :deep(h3:first-child),
.markdown-body :deep(p:first-child) { margin-top: 0; }
.config-item { border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.08); padding-bottom: 12px; }
</style>