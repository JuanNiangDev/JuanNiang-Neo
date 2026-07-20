<template>
  <div>
    <div class="page-header">
      <div class="page-title">Plugin 管理</div>
      <div class="page-subtitle">管理 Lua 插件，支持 ZIP 上传</div>
    </div>
    <div class="d-flex justify-end mb-4" style="gap:12px">
      <v-btn color="primary" variant="tonal" prepend-icon="mdi-upload" @click="triggerUpload">上传 ZIP</v-btn>
      <input ref="fileInput" type="file" accept=".zip" style="display:none" @change="handleFile" />
    </div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.is_system="{ item }">
        <v-chip v-if="item.is_system" size="small" color="error" variant="tonal">系统</v-chip>
        <v-chip v-else size="small" color="grey" variant="tonal">普通</v-chip>
      </template>
      <template #item.permissions="{ item }">
        <div class="d-flex flex-wrap" style="gap:4px">
          <v-chip v-for="p in (item.permissions || [])" :key="p" size="x-small" variant="tonal" color="info">{{ p }}</v-chip>
          <span v-if="!item.permissions || item.permissions.length === 0" class="text-caption text-medium-emphasis">(无)</span>
        </div>
      </template>
      <template #item.is_active="{ item }">
        <v-switch
          :model-value="item.is_active"
          :disabled="item.is_system"
          color="primary"
          density="compact"
          hide-details
          @update:model-value="(v) => toggle(item.id || item.name, !!v)"
        />
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-eye" size="small" variant="text" color="info" @click="showDetail(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" :disabled="item.is_system" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <!-- 详情弹窗: yaml 元数据 + 命令列表 -->
    <v-dialog v-model="detailDialog" max-width="800">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center justify-space-between pa-4">
          <span class="text-body-1">{{ detail?.name }} 元数据</span>
          <v-btn icon="mdi-close" size="small" variant="text" @click="detailDialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-4">
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
            <v-data-table
              :headers="cmdHeaders"
              :items="detailCommands"
              density="compact"
              hide-default-footer
              :items-per-page="-1"
            >
              <template #item.path="{ item }">
                <code class="cmd-code">/{{ (item.path || []).join(' ') }}</code>
              </template>
            </v-data-table>
          </div>
          <div v-else class="text-caption text-medium-emphasis">该插件没有注册命令</div>
        </v-card-text>
        <v-divider />
        <v-card-actions class="pa-4">
          <v-spacer />
          <v-btn variant="text" @click="detailDialog = false">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>确定要删除此插件吗？</v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { pluginApi } from '@/api'
import { useToastStore } from '@/stores/toast'

interface PluginItem {
  id?: string
  name: string
  version: string
  author?: string
  description?: string
  permissions?: string[]
  is_system?: boolean
  is_active?: boolean
  commands?: Array<{ path: string[]; description: string; usage: string; is_leaf: boolean }>
}

const toastStore = useToastStore()
const loading = ref(true)
const items = ref<PluginItem[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
const deleteDialog = ref(false)
const deleting = ref(false)
const deleteTarget = ref<PluginItem | null>(null)
const detailDialog = ref(false)
const detail = ref<PluginItem | null>(null)

const headers = [
  { title: '名称', key: 'name' },
  { title: '版本', key: 'version' },
  { title: '权限', key: 'permissions' },
  { title: '类型', key: 'is_system' },
  { title: 'Active', key: 'is_active', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const cmdHeaders = [
  { title: '命令', key: 'path' },
  { title: '描述', key: 'description' },
  { title: '用法', key: 'usage' },
]

const detailCommands = computed(() => detail.value?.commands || [])

async function fetch() {
  loading.value = true
  try { items.value = (await pluginApi.list()).data.data || [] }
  catch { toastStore.error('获取失败') }
  finally { loading.value = false }
}

function triggerUpload() { fileInput.value?.click() }
async function handleFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  try { await pluginApi.upload(f); toastStore.success('上传成功'); await fetch() }
  catch (err: any) { toastStore.error(err?.message || '上传失败') }
}

async function toggle(id: string, v: boolean) {
  try { await pluginApi.toggle(id, v); await fetch(); toastStore.success(v ? '已启用' : '已停用') }
  catch (e: any) { toastStore.error(e?.response?.data?.info || '操作失败') }
}

function showDetail(item: PluginItem) {
  detail.value = item
  detailDialog.value = true
}

function confirmDelete(item: PluginItem) {
  if (item.is_system) {
    toastStore.error('系统插件不允许删除')
    return
  }
  deleteTarget.value = item
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
    await fetch()
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || '删除失败')
  } finally { deleting.value = false }
}

onMounted(fetch)
</script>

<style scoped>
.cmd-code {
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
  font-size: 12px;
  padding: 2px 6px;
  background: rgba(var(--v-theme-on-surface), 0.06);
  border-radius: 4px;
}
</style>
