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
      <template #item.is_active="{ item }">
        <v-switch :model-value="item.is_active" color="primary" density="compact" hide-details @update:model-value="(v) => toggle(item.id, !!v)" />
      </template>
      <template #item.config="{ item }">
        <v-chip v-if="item.config && Object.keys(item.config).length > 0" size="small" variant="tonal" color="info" @click="showConfig(item)">查看配置</v-chip>
        <span v-else class="text-caption text-medium-emphasis">空</span>
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <v-dialog v-model="configDialog" max-width="500">
      <v-card rounded="lg">
        <v-card-title>插件配置: {{ configTarget?.name }}</v-card-title>
        <v-card-text><pre class="code-block">{{ JSON.stringify(configTarget?.config, null, 2) }}</pre></v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="configDialog = false">关闭</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>确定要删除此插件吗？</v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { pluginApi, type PluginResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<PluginResp[]>([]); const fileInput = ref<HTMLInputElement | null>(null)
const configDialog = ref(false); const configTarget = ref<PluginResp | null>(null)
const deleteDialog = ref(false); const deleting = ref(false); const deleteTarget = ref<PluginResp | null>(null)

const headers = [
  { title: '名称', key: 'name' }, { title: '版本', key: 'version' }, { title: '路径', key: 'path' },
  { title: '配置', key: 'config' }, { title: 'Active', key: 'is_active', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

async function fetch() { loading.value = true; try { items.value = (await pluginApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
function triggerUpload() { fileInput.value?.click() }
async function handleFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]; if (!f) return
  try { await pluginApi.upload(f); toastStore.success('上传成功'); await fetch() } catch (err: any) { toastStore.error(err?.message || '上传失败') }
}
async function toggle(id: string, v: boolean) { try { await pluginApi.toggle(id, v); await fetch(); toastStore.success(v ? '已启用' : '已停用') } catch { toastStore.error('操作失败') } }
function showConfig(item: PluginResp) { configTarget.value = item; configDialog.value = true }
function confirmDelete(item: PluginResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() { if (!deleteTarget.value) return; deleting.value = true; try { await pluginApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch { toastStore.error('删除失败') } finally { deleting.value = false } }
onMounted(fetch)
</script>
