<template>
  <v-container fluid>
    <div class="d-flex align-center justify-space-between mb-4">
      <div class="d-flex align-center">
        <h2 class="text-h5">Agent 循环</h2>
        <v-chip v-if="loops.length" class="ml-3" color="primary" size="small" label>{{ loops.length }} 个活跃循环</v-chip>
      </div>
      <v-btn variant="text" icon="mdi-refresh" @click="fetchLoops" :loading="loading" />
    </div>

    <v-card v-if="loops.length">
      <v-data-table
        :headers="headers"
        :items="loops"
        :loading="loading"
        :items-per-page="20"
        items-per-page-text="每页"
        class="elevation-0"
      >
        <template #item.message_type="{ item }">
          <v-chip size="small" :color="item.message_type === 'private' ? 'blue' : 'green'" label>
            {{ item.message_type === 'private' ? '私聊' : '群聊' }}
          </v-chip>
        </template>
        <template #item.user_msg="{ item }">
          <span class="text-body-2" style="max-width: 260px; display: inline-block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
            {{ item.user_msg || '-' }}
          </span>
        </template>
        <template #item.current_tool="{ item }">
          <v-chip v-if="item.current_tool" size="small" color="orange" variant="tonal" label>
            <v-icon size="14" class="mr-1">mdi-wrench</v-icon>{{ item.current_tool }}
          </v-chip>
          <span v-else class="text-grey text-caption">思考/生成中…</span>
        </template>
        <template #item.started_at="{ item }">
          {{ formatTime(item.started_at) }}
        </template>
        <template #item.duration="{ item }">
          <span class="text-body-2">{{ formatDuration(item.started_at) }}</span>
        </template>
      </v-data-table>
    </v-card>

    <v-card v-else class="pa-8 d-flex flex-column align-center">
      <v-icon size="48" color="secondary" class="mb-2">mdi-brain</v-icon>
      <div class="text-subtitle-1 text-medium-emphasis">当前没有活跃的 Agent 循环</div>
      <div class="text-caption text-medium-emphasis mt-1">收到群聊/私聊消息后，正在执行的 ReAct 循环会显示在这里（每 3 秒自动刷新）</div>
    </v-card>
  </v-container>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { agentLoopApi, type AgentLoopResp } from '@/api'

const loops = ref<AgentLoopResp[]>([])
const loading = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

const headers = [
  { title: '类型', key: 'message_type', width: 80, sortable: false },
  { title: '目标ID', key: 'target_id', width: 110, sortable: false },
  { title: '用户QQ', key: 'user_id', width: 110, sortable: false },
  { title: '用户消息', key: 'user_msg', width: 280, sortable: false },
  { title: '当前工具', key: 'current_tool', width: 160, sortable: false },
  { title: '开始时间', key: 'started_at', width: 180 },
  { title: '已运行', key: 'duration', width: 100, sortable: false },
]

async function fetchLoops() {
  loading.value = true
  try {
    const res = await agentLoopApi.list()
    loops.value = (res.data as any)?.data || []
  } catch (e) {
    console.error('Failed to fetch agent loops', e)
  } finally {
    loading.value = false
  }
}

function formatTime(t: string) {
  if (!t) return '-'
  return new Date(t).toLocaleString()
}

function formatDuration(startedAt: string) {
  if (!startedAt) return '-'
  const ms = Date.now() - new Date(startedAt).getTime()
  if (ms < 0) return '0s'
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ${s % 60}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}

onMounted(() => {
  fetchLoops()
  timer = setInterval(fetchLoops, 3000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>
