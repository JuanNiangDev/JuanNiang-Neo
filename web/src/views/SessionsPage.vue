<template>
  <div>
    <div class="page-header">
      <div class="page-title">Session 管理</div>
      <div class="page-subtitle">查看与删除会话状态</div>
    </div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.token_usage="{ item }">{{ formatNumber(item.token_usage) }}</template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>
    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>删除 Session 将同时清除 Redis 消息缓存。</v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { sessionApi, type SessionResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<SessionResp[]>([])
const deleteDialog = ref(false); const deleting = ref(false); const deleteTarget = ref<SessionResp | null>(null)

const headers = [
  { title: 'ID', key: 'id' }, { title: 'Chat Area ID', key: 'chat_area_id' }, { title: '模型', key: 'model' },
  { title: 'Token 用量', key: 'token_usage' }, { title: '创建时间', key: 'created_at' },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

function formatNumber(n: number) { return n >= 1000 ? (n / 1000).toFixed(1) + 'K' : String(n) }
async function fetch() { loading.value = true; try { items.value = (await sessionApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
function confirmDelete(item: SessionResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() { if (!deleteTarget.value) return; deleting.value = true; try { await sessionApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch { toastStore.error('删除失败') } finally { deleting.value = false } }
onMounted(fetch)
</script>
