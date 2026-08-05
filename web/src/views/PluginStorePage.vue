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
        <v-col v-for="item in storeItems" :key="item.path" cols="12" sm="6" md="4" lg="3">
          <v-card rounded="lg" elevation="1" class="store-card" @click="openDetail(item)">
            <div class="d-flex flex-column align-center pa-4">
              <v-avatar size="72" rounded="lg" class="mb-3" style="background: rgba(var(--v-theme-primary), 0.08)">
                <v-img v-if="avatarSrc[item.path]" :src="avatarSrc[item.path]" contain />
                <v-icon v-else size="40" color="primary">mdi-puzzle</v-icon>
              </v-avatar>
              <div class="text-subtitle-1 font-weight-bold text-center">{{ item.name }}</div>
              <div class="text-caption text-medium-emphasis text-center store-desc">{{ item.description || '无描述' }}</div>
              <div class="d-flex align-center mt-2" style="gap:6px">
                <v-chip size="x-small" variant="tonal" color="grey">v{{ item.version }}</v-chip>
                <v-chip size="x-small" variant="tonal" color="info">{{ item.author }}</v-chip>
              </div>
            </div>
            <v-divider />
            <v-card-actions>
              <v-spacer />
              <v-btn size="small" color="primary" variant="tonal" prepend-icon="mdi-download" :loading="installing === item.path" @click.stop="install(item)">安装</v-btn>
            </v-card-actions>
          </v-card>
        </v-col>
      </v-row>
      <div v-if="storeItems.length === 0 && !loading" class="text-center pa-8 text-medium-emphasis">
        商店为空或无法连接，请检查镜像源设置
      </div>
    </v-container>

    <!-- 详情弹窗 -->
    <v-dialog v-model="detailDialog" max-width="800" scrollable>
      <v-card rounded="lg">
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
        <v-card-text class="pa-4" style="min-height:200px">
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
    <v-dialog v-model="settingsDialog" max-width="700">
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

          <div class="text-caption text-medium-emphasis mb-2 mt-2">自定义镜像源（已内置 GitHub / ghproxy / gitmirror / jsDelivr）</div>
          <div v-for="m in customMirrors" :key="m" class="d-flex align-center mb-2" style="gap:8px">
            <code class="mirror-code flex-grow-1">{{ m }}</code>
            <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="removeMirror(m)" />
          </div>
          <div class="d-flex align-center" style="gap:8px">
            <v-text-field v-model="newMirror" label="新镜像地址（含 {path} 占位符）" variant="outlined" density="compact" hide-details />
            <v-btn color="primary" variant="tonal" prepend-icon="mdi-plus" @click="addMirror">添加</v-btn>
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
import { ref, computed, onMounted } from 'vue'
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
const customMirrors = ref<string[]>([])
const newMirror = ref('')
const savingConfig = ref(false)

async function fetchStore() {
  loading.value = true
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
    customMirrors.value = data.mirrors?.filter((m: string) => !m.startsWith('https://raw.githubusercontent') && !m.startsWith('https://ghproxy') && !m.startsWith('https://gh-proxy') && !m.startsWith('https://raw.gitmirror') && !m.startsWith('https://cdn.jsdelivr')) || []
  } catch { /* ignore */ }
}

async function addMirror() {
  const m = newMirror.value.trim()
  if (!m) return
  try {
    await storeApi.addMirror(m)
    toastStore.success('已添加镜像')
    newMirror.value = ''
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
</script>

<style scoped>
.store-card { cursor: pointer; transition: transform 0.15s ease, box-shadow 0.15s ease; }
.store-card:hover { transform: translateY(-2px); box-shadow: 0 4px 16px rgba(0,0,0,0.12) !important; }
.store-desc {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  min-height: 32px;
}
.markdown-body { word-break: break-word; }
.markdown-body :deep(h1), .markdown-body :deep(h2), .markdown-body :deep(h3) { margin-top: 0.8em; margin-bottom: 0.4em; }
.markdown-body :deep(pre) { background: rgba(var(--v-theme-on-surface), 0.06); padding: 12px; border-radius: 6px; overflow: auto; }
.markdown-body :deep(code) { font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace; font-size: 12px; }
.markdown-body :deep(img) { max-width: 100%; }
.mirror-code { font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace; font-size: 12px; padding: 4px 8px; background: rgba(var(--v-theme-on-surface), 0.06); border-radius: 4px; word-break: break-all; }
</style>