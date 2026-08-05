<template>
  <div>
    <div class="page-header d-flex align-center">
      <div class="flex-grow-1">
        <div class="page-title"><v-icon class="me-2" color="primary">mdi-storefront-outline</v-icon>Plugin 商店</div>
        <div class="page-subtitle">浏览并安装社区插件</div>
      </div>
      <div class="d-flex" style="gap:8px">
        <v-btn variant="tonal" color="primary" prepend-icon="mdi-refresh" :loading="loading" @click="fetchStore">刷新</v-btn>
        <v-btn variant="tonal" color="info" prepend-icon="mdi-cog" @click="settingsDialog = true">镜像源设置</v-btn>
      </div>
    </div>

    <!-- 网格卡片 -->
    <v-container fluid class="pa-0">
      <v-row>
        <v-col v-for="item in pagedItems" :key="item.path" cols="12" sm="6" md="4" lg="3">
          <v-card rounded="lg" elevation="1" class="store-card" @click="openDetail(item)">
            <div class="d-flex card-body">
              <!-- 左侧矩形图片区 -->
              <div class="card-thumb">
                <v-img
                  v-if="avatarSrc[item.path]"
                  :src="avatarSrc[item.path]"
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
                  <v-chip size="x-small" variant="tonal" color="grey">v{{ item.version }}</v-chip>
                </div>
                <div class="text-caption text-medium-emphasis card-desc">{{ item.description || '无描述' }}</div>
                <div class="d-flex align-center justify-end card-actions" style="gap:6px">
                  <v-chip size="x-small" variant="tonal" color="info" class="text-truncate" style="max-width: 45%">{{ item.author }}</v-chip>
                  <v-spacer />
                  <v-btn size="small" color="primary" variant="tonal" prepend-icon="mdi-download" :loading="installing === item.path" @click.stop="install(item)">安装</v-btn>
                </div>
              </div>
            </div>
          </v-card>
        </v-col>
      </v-row>
      <div v-if="storeItems.length === 0 && !loading" class="text-center pa-8 text-medium-emphasis">
        商店为空或无法连接，请检查镜像源设置
      </div>
    </v-container>

    <!-- 分页 -->
    <div v-if="storeItems.length > 0" class="d-flex align-center justify-space-between mt-4 flex-wrap" style="gap:12px">
      <div class="text-caption text-medium-emphasis">共 {{ storeItems.length }} 个插件</div>
      <v-pagination v-if="totalPages > 1" v-model="page" :length="totalPages" :total-visible="7" density="comfortable" />
      <v-select
        v-model="pageSize"
        :items="[12, 24, 48]"
        label="每页"
        variant="outlined"
        density="compact"
        hide-details
        style="max-width:110px"
      />
    </div>

    <!-- 详情弹窗 -->
    <v-dialog v-model="detailDialog" max-width="800" scrollable>
      <v-card rounded="lg" class="detail-card">
        <v-card-title class="d-flex align-center pa-4">
          <v-avatar size="40" rounded="lg" class="me-3">
            <v-img v-if="detailAvatar" :src="detailAvatar" contain />
            <v-icon v-else color="primary">mdi-puzzle</v-icon>
          </v-avatar>
          <div>
            <div class="text-subtitle-1 font-weight-bold">{{ detail?.name }}</div>
            <div class="text-caption text-medium-emphasis">v{{ detail?.version }} · by {{ detail?.author }}</div>
          </div>
          <v-spacer />
          <v-btn icon="mdi-close" size="small" variant="text" @click="detailDialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-4 detail-body" style="min-height:200px">
          <div v-if="readmeLoading" class="text-center pa-8"><v-progress-circular indeterminate /></div>
          <div v-else-if="readmeContent" class="markdown-body" v-html="renderedReadme" />
          <div v-else class="text-center pa-8 text-medium-emphasis">该插件没有说明文档</div>
        </v-card-text>
        <v-divider />
        <v-card-actions class="pa-4">
          <v-spacer />
          <v-btn color="primary" variant="tonal" prepend-icon="mdi-download" :loading="installing === detail?.path" @click="install(detail)">安装此插件</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 镜像源设置 -->
    <v-dialog v-model="settingsDialog" max-width="720">
      <v-card rounded="lg">
        <v-card-title>镜像源设置</v-card-title>
        <v-divider />
        <v-card-text>
          <v-form>
            <v-row>
              <v-col cols="12" md="4">
                <v-text-field v-model="configForm.repo_owner" label="仓库所有者" variant="outlined" density="compact" />
              </v-col>
              <v-col cols="12" md="4">
                <v-text-field v-model="configForm.repo_name" label="仓库名" variant="outlined" density="compact" />
              </v-col>
              <v-col cols="12" md="4">
                <v-text-field v-model="configForm.branch" label="分支" variant="outlined" density="compact" />
              </v-col>
            </v-row>
          </v-form>

          <div class="text-caption text-medium-emphasis mb-2 mt-4">生效镜像源（手动选择，不做自动切换）</div>
          <div class="d-flex align-center" style="gap:8px">
            <v-select
              v-model="selectedMirror"
              :items="mirrorSelectItems"
              label="选择生效镜像（空 = 自动按顺序尝试）"
              variant="outlined"
              density="compact"
              hide-details
              clearable
              @update:model-value="(v) => selectMirror(v || '')"
            />
            <v-btn color="success" variant="tonal" prepend-icon="mdi-access-point" :loading="testingMirror" @click="testMirror">测试</v-btn>
          </div>
          <div v-if="mirrorTestResult" class="mt-2" :class="mirrorTestOk ? 'text-success' : 'text-error'">
            <v-icon size="small" class="me-1">{{ mirrorTestOk ? 'mdi-check-circle' : 'mdi-alert-circle' }}</v-icon>
            {{ mirrorTestResult }}
          </div>

          <div class="text-caption text-medium-emphasis mb-2 mt-4">添加自定义镜像（下拉选择或手动输入新地址，需包含 {path} 占位符）</div>
          <div class="d-flex align-center" style="gap:8px">
            <v-combobox
              v-model="mirrorInput"
              :items="allMirrors"
              label="选择或输入镜像地址"
              variant="outlined"
              density="compact"
              hide-details
              clearable
            />
            <v-btn color="primary" variant="tonal" prepend-icon="mdi-plus" :disabled="!canAddMirror" @click="addMirror">添加</v-btn>
          </div>

          <div class="text-caption text-medium-emphasis mb-2 mt-4">可用镜像列表</div>
          <div v-for="m in allMirrors" :key="m" class="d-flex align-center mb-2" style="gap:8px">
            <v-icon size="small" :color="selectedMirror === m ? 'success' : 'grey'" class="me-1">
              {{ selectedMirror === m ? 'mdi-check-circle' : 'mdi-circle-outline' }}
            </v-icon>
            <code class="mirror-code flex-grow-1">{{ m }}</code>
            <v-btn v-if="isCustomMirror(m)" icon="mdi-delete" size="small" variant="text" color="error" @click="removeMirror(m)" />
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions class="pa-4">
          <v-spacer />
          <v-btn variant="text" @click="settingsDialog = false">关闭</v-btn>
          <v-btn color="primary" :loading="savingConfig" @click="saveConfig">保存仓库配置</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { marked } from 'marked'
import { storeApi } from '@/api'
import { useToastStore } from '@/stores/toast'

interface StoreItem {
  name: string
  version: string
  author: string
  description: string
  path: string
  image?: string
  has_config?: boolean
  has_readme?: boolean
}

const toastStore = useToastStore()
const loading = ref(false)
const storeItems = ref<StoreItem[]>([])

// 分页
const page = ref(1)
const pageSize = ref(12)
const totalPages = computed(() => Math.max(1, Math.ceil(storeItems.value.length / pageSize.value)))
const pagedItems = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return storeItems.value.slice(start, start + pageSize.value)
})
const avatarSrc = ref<Record<string, string>>({})
const installing = ref('')

const detailDialog = ref(false)
const detail = ref<StoreItem | null>(null)
const detailAvatar = computed(() => detail.value ? avatarSrc.value[detail.value.path] : '')
const readmeLoading = ref(false)
const readmeContent = ref('')
const renderedReadme = computed(() => readmeContent.value ? marked.parse(readmeContent.value) as string : '')

const settingsDialog = ref(false)
const configForm = ref({ repo_owner: '', repo_name: '', branch: 'main' })
const allMirrors = ref<string[]>([])
const customMirrors = ref<string[]>([])
const mirrorInput = ref('')
const selectedMirror = ref('')
const testingMirror = ref(false)
const mirrorTestResult = ref('')
const mirrorTestOk = ref(false)
const savingConfig = ref(false)

// 内置镜像前缀（用于区分自定义镜像）
const builtinMirrorPrefixes = [
  'https://raw.githubusercontent.com',
  'https://ghproxy',
  'https://gh-proxy',
  'https://raw.gitmirror',
  'https://cdn.jsdelivr',
]

// 生效镜像下拉选项：空值 = 自动按顺序尝试
const mirrorSelectItems = computed(() => [
  { title: '自动选择（默认）', value: '' },
  ...allMirrors.value.map((m) => ({ title: m, value: m })),
])

const isCustomMirror = (m: string) => customMirrors.value.includes(m)

const canAddMirror = computed(() => {
  const m = (mirrorInput.value || '').trim()
  return m !== '' && m.includes('{path}') && !allMirrors.value.includes(m)
})

async function fetchStore() {
  loading.value = true
  // 清空头像缓存（释放旧 blob URL），确保仓库更新图片后点刷新可见
  for (const k in avatarSrc.value) URL.revokeObjectURL(avatarSrc.value[k])
  avatarSrc.value = {}
  try {
    const list = (await storeApi.list()).data.data || []
    storeItems.value = list
    for (const item of list) {
      if (!avatarSrc.value[item.path]) loadAvatar(item.path)
    }
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || '拉取商店失败')
    storeItems.value = []
  } finally { loading.value = false }
}

async function loadAvatar(path: string) {
  try {
    const res = await storeApi.avatar(path)
    // 无头像时后端返回 JSON blob，需过滤，仅接受图片
    if (!res.data || typeof res.data !== 'object' || !res.data.type || !String(res.data.type).startsWith('image/')) return
    avatarSrc.value[path] = URL.createObjectURL(res.data)
  } catch { /* 无头像 */ }
}

async function openDetail(item: StoreItem) {
  detail.value = item
  detailDialog.value = true
  readmeContent.value = ''
  readmeLoading.value = true
  try {
    const res = await storeApi.readme(item.path)
    readmeContent.value = res.data?.data?.content || ''
  } catch { readmeContent.value = '' }
  finally { readmeLoading.value = false }
}

async function install(item: StoreItem | null) {
  if (!item) return
  installing.value = item.path
  try {
    await storeApi.install(item.path)
    toastStore.success(`已安装 ${item.name}`)
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || '安装失败')
  } finally { installing.value = '' }
}

async function fetchConfig() {
  try {
    const res = await storeApi.config()
    const data = res.data?.data || {}
    const cfg = data.config || {}
    configForm.value = {
      repo_owner: cfg.repo_owner || 'JuanNiangDev',
      repo_name: cfg.repo_name || 'JuanNiang-Plugins',
      branch: cfg.branch || 'main',
    }
    allMirrors.value = (data.mirrors || []).slice()
    customMirrors.value = (data.mirrors || []).filter((m: string) => !builtinMirrorPrefixes.some((p) => m.startsWith(p)))
    selectedMirror.value = cfg.selected_mirror || ''
  } catch { /* ignore */ }
}

async function selectMirror(m: string) {
  try {
    await storeApi.selectMirror(m)
    selectedMirror.value = m
    toastStore.success(m ? '已切换到指定镜像源' : '已恢复默认自动选择')
    mirrorTestResult.value = ''
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || e?.response?.data?.data?.error_detail || '切换失败')
  }
}

async function testMirror() {
  // 优先测试下拉中选中的生效镜像，否则测试输入框中的地址
  const m = (selectedMirror.value || mirrorInput.value || '').trim()
  if (!m) return
  testingMirror.value = true
  mirrorTestResult.value = ''
  try {
    const res = await storeApi.testMirror(m)
    const latency = res.data?.data?.latency_ms ?? 0
    mirrorTestOk.value = true
    mirrorTestResult.value = `连接成功，延迟 ${latency} ms`
  } catch (e: any) {
    mirrorTestOk.value = false
    mirrorTestResult.value = `连接失败：${e?.response?.data?.data?.error_detail || e?.response?.data?.info || e?.message || '未知错误'}`
  } finally { testingMirror.value = false }
}

async function addMirror() {
  const m = (mirrorInput.value || '').trim()
  if (!m) return
  try {
    await storeApi.addMirror(m)
    toastStore.success('已添加镜像')
    mirrorInput.value = ''
    await fetchConfig()
  } catch (e: any) { toastStore.error(e?.response?.data?.info || e?.message || '添加失败') }
}

async function removeMirror(m: string) {
  try {
    await storeApi.removeMirror(m)
    toastStore.success('已删除镜像')
    await fetchConfig()
  } catch (e: any) { toastStore.error(e?.response?.data?.info || '删除失败') }
}

async function saveConfig() {
  savingConfig.value = true
  try {
    await storeApi.saveConfig(configForm.value)
    toastStore.success('已保存，可回到商店刷新')
  } catch (e: any) { toastStore.error(e?.response?.data?.info || '保存失败') }
  finally { savingConfig.value = false }
}

onMounted(() => {
  fetchStore()
  fetchConfig()
})

// 列表变化后页码可能越界（刷新/安装导致条数变化），自动修正；切换每页条数时回到第一页
watch(storeItems, () => {
  if (page.value > totalPages.value) page.value = totalPages.value
})
watch(pageSize, () => { page.value = 1 })
</script>

<style scoped>
.store-card { cursor: pointer; transition: transform 0.15s ease, box-shadow 0.15s ease; }
.store-card:hover { transform: translateY(-2px); box-shadow: 0 4px 16px rgba(0,0,0,0.12) !important; }

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

/* 弹窗内容超高时仅内容区内部滚动，固定头部与底部按钮 */
.detail-card { max-height: 85vh; display: flex; flex-direction: column; overflow: hidden; }
.detail-body { flex: 1 1 auto; min-height: 0; overflow-y: auto; }

.markdown-body { word-break: break-word; overflow-x: auto; }
.markdown-body :deep(table) { border-collapse: collapse; width: 100%; margin: 8px 0; font-size: 13px; }
.markdown-body :deep(th), .markdown-body :deep(td) { border: 1px solid rgba(var(--v-theme-on-surface), 0.15); padding: 6px 10px; text-align: left; vertical-align: top; }
.markdown-body :deep(th) { background: rgba(var(--v-theme-on-surface), 0.05); font-weight: 600; white-space: nowrap; }
.markdown-body :deep(tr:nth-child(even)) { background: rgba(var(--v-theme-on-surface), 0.03); }
.markdown-body :deep(h1), .markdown-body :deep(h2), .markdown-body :deep(h3) { margin-top: 0.8em; margin-bottom: 0.4em; }
.markdown-body :deep(pre) { background: rgba(var(--v-theme-on-surface), 0.06); padding: 12px; border-radius: 6px; overflow: auto; }
.markdown-body :deep(code) { font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace; font-size: 12px; }
.markdown-body :deep(img) { max-width: 100%; }
.mirror-code { font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace; font-size: 12px; padding: 4px 8px; background: rgba(var(--v-theme-on-surface), 0.06); border-radius: 4px; word-break: break-all; }
</style>