<template>
  <v-container fluid>
    <div class="d-flex align-center justify-space-between mb-4">
      <h2 class="text-h5">后台任务</h2>
      <v-btn variant="text" icon="mdi-refresh" @click="fetchTasks" :loading="loading" />
    </div>

    <v-card>
      <v-data-table
        :headers="headers"
        :items="tasks"
        :loading="loading"
        :items-per-page="20"
        items-per-page-text="每页"
        class="elevation-0"
      >
        <template #item.status="{ item }">
          <v-chip
            :color="statusColor(item.status)"
            size="small"
            label
          >
            {{ item.status }}
          </v-chip>
        </template>
        <template #item.message_type="{ item }">
          <v-chip v-if="item.message_type" size="small" :color="item.message_type === 'private' ? 'blue' : 'green'" label>
            {{ item.message_type === 'private' ? '私聊' : '群聊' }}
          </v-chip>
          <span v-else class="text-grey">-</span>
        </template>
        <template #item.user_prompt="{ item }">
          <span class="text-body-2" style="max-width: 200px; display: inline-block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
            {{ item.user_prompt || '-' }}
          </span>
        </template>
        <template #item.created_at="{ item }">
          {{ formatTime(item.created_at) }}
        </template>
        <template #item.actions="{ item }">
          <v-btn variant="text" icon="mdi-eye" size="small" @click="showDetail(item)" />
        </template>
      </v-data-table>
    </v-card>

    <!-- Detail Dialog -->
    <v-dialog v-model="dialog" max-width="700">
      <v-card v-if="selected">
        <v-toolbar color="primary" dark>
          <v-toolbar-title>任务详情</v-toolbar-title>
          <v-spacer />
          <v-btn icon="mdi-close" variant="text" @click="dialog = false" />
        </v-toolbar>
        <v-card-text class="mt-4">
          <v-list density="compact">
            <v-list-item>
              <template #title>任务 ID</template>
              <template #subtitle><code class="text-caption">{{ selected.id }}</code></template>
            </v-list-item>
            <v-list-item>
              <template #title>状态</template>
              <template #subtitle>
                <v-chip :color="statusColor(selected.status)" size="small" label>{{ selected.status }}</v-chip>
              </template>
            </v-list-item>
            <v-list-item>
              <template #title>Chat Area</template>
              <template #subtitle>{{ selected.chat_area_id }}</template>
            </v-list-item>
            <v-list-item>
              <template #title>消息类型 / 目标</template>
              <template #subtitle>{{ selected.message_type || '-' }} / {{ selected.target_id || '-' }}</template>
            </v-list-item>
            <v-list-item>
              <template #title>用户提示</template>
              <template #subtitle>{{ selected.user_prompt || '-' }}</template>
            </v-list-item>
            <v-list-item>
              <template #title>创建时间</template>
              <template #subtitle>{{ formatTime(selected.created_at) }}</template>
            </v-list-item>
            <v-list-item>
              <template #title>更新时间</template>
              <template #subtitle>{{ formatTime(selected.updated_at) }}</template>
            </v-list-item>
          </v-list>

          <v-divider class="my-3" />

          <div class="text-subtitle-1 font-weight-bold mb-2">步骤:</div>
          <pre class="text-caption pa-3 rounded bg-grey-lighten-4" style="max-height: 150px; overflow: auto;">{{ formatJSON(selected.steps) }}</pre>

          <div class="text-subtitle-1 font-weight-bold mb-2 mt-3">结果:</div>
          <pre class="text-caption pa-3 rounded bg-grey-lighten-4" style="max-height: 250px; overflow: auto;">{{ formatJSON(selected.results) }}</pre>

          <div v-if="selected.results?._errors" class="mt-3">
            <v-alert type="error" density="compact">
              {{ selected.results._errors }}
            </v-alert>
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>
  </v-container>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { backgroundTaskApi } from '@/api'
import type { BackgroundTaskResp } from '@/api'

const tasks = ref<BackgroundTaskResp[]>([])
const loading = ref(false)
const dialog = ref(false)
const selected = ref<BackgroundTaskResp | null>(null)

const headers = [
  { title: 'ID', key: 'id', width: 100, sortable: false },
  { title: '状态', key: 'status', width: 100 },
  { title: '类型', key: 'message_type', width: 80 },
  { title: '目标ID', key: 'target_id', width: 120 },
  { title: '用户提示', key: 'user_prompt', width: 200 },
  { title: '创建时间', key: 'created_at', width: 180 },
  { title: ' ', key: 'actions', width: 60, sortable: false },
]

onMounted(fetchTasks)

async function fetchTasks() {
  loading.value = true
  try {
    const res = await backgroundTaskApi.list()
    const data = res.data as any
    tasks.value = data?.data || []
  } catch (e) {
    console.error('Failed to fetch background tasks', e)
  } finally {
    loading.value = false
  }
}

function showDetail(item: BackgroundTaskResp) {
  selected.value = item
  dialog.value = true
}

function statusColor(status: string) {
  switch (status) {
    case 'pending': return 'grey'
    case 'running': return 'blue'
    case 'done': return 'green'
    case 'failed': return 'red'
    default: return 'grey'
  }
}

function formatTime(t: string) {
  if (!t) return '-'
  return new Date(t).toLocaleString()
}

function formatJSON(obj: any) {
  return JSON.stringify(obj, null, 2)
}
</script>
