<template>
  <div>
    <div class="page-header"><div class="page-title">Chat Areas</div><div class="page-subtitle">消息驱动的自动创建聊天区域</div></div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.area_type="{ item }">
        <v-chip size="small" variant="tonal" :color="item.area_type==='private'?'primary':'info'">{{ item.area_type==='private'?'私聊':'群聊' }}</v-chip>
      </template>
    </v-data-table>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { chatAreaApi, type ChatAreaResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<ChatAreaResp[]>([])
const headers = [
  { title: 'ID', key: 'id' }, { title: '类型', key: 'area_type' }, { title: '目标 QQ/群号', key: 'target_id' }, { title: '创建时间', key: 'created_at' },
]

async function fetch() { loading.value = true; try { items.value = (await chatAreaApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
onMounted(fetch)
</script>
