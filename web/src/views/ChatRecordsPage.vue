<template>
  <div>
    <div class="page-header"><div class="page-title">Chat Records</div><div class="page-subtitle">按 ChatArea 分页查询聊天记录</div></div>

    <!-- Filters -->
    <v-card rounded="lg" elevation="1" class="mb-4 pa-4">
      <v-row dense align="center">
        <v-col cols="12" md="5">
          <v-select
            v-model="chatAreaId"
            :items="chatAreaItems"
            item-title="label"
            item-value="value"
            label="Chat Area"
            placeholder="选择 ChatArea"
            density="compact"
            hide-details
            clearable
          />
        </v-col>
        <v-col cols="6" md="2">
          <v-select v-model="roleFilter" :items="[{title:'全部',value:''},{title:'User',value:'user'},{title:'Assistant',value:'assistant'},{title:'Tool',value:'tool'}]" label="Role 过滤" density="compact" hide-details @update:model-value="fetch" />
        </v-col>
        <v-col cols="6" md="2">
          <v-text-field v-model.number="limit" label="每页数量" type="number" density="compact" hide-details />
        </v-col>
        <v-col cols="12" md="2">
          <v-btn color="primary" variant="tonal" block @click="fetch" :loading="loading">查询</v-btn>
        </v-col>
      </v-row>
    </v-card>

    <v-data-table v-if="chatAreaId" :headers="headers" :items="records" :loading="loading" :items-per-page="limit">
      <template #item.role="{ item }">
        <v-chip size="small" variant="tonal" :color="item.role==='user'?'primary':item.role==='assistant'?'success':'warning'">{{ item.role }}</v-chip>
      </template>
      <template #item.content="{ item }">
        <div style="max-width:400px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" class="text-caption">{{ item.content }}</div>
      </template>
      <template #item.tool_calls="{ item }">
        <v-chip v-if="item.tool_calls" size="x-small" variant="tonal" color="info" @click="showToolCalls(item)">查看</v-chip>
      </template>
    </v-data-table>

    <div v-else class="empty-state"><v-icon class="empty-icon" color="secondary">mdi-message-text</v-icon><div class="empty-text">请先输入 Chat Area ID 进行查询</div></div>

    <!-- Pagination -->
    <v-pagination v-if="total > limit" v-model="page" :length="Math.ceil(total/limit)" class="mt-4" @update:model-value="fetch" />

    <v-dialog v-model="toolCallsDialog" max-width="500">
      <v-card rounded="lg"><v-card-title>Tool Calls 详情</v-card-title><v-card-text><pre class="code-block">{{ JSON.stringify(toolCallsTarget?.tool_calls, null, 2) }}</pre></v-card-text><v-card-actions><v-spacer /><v-btn variant="text" @click="toolCallsDialog = false">关闭</v-btn></v-card-actions></v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { chatRecordApi, chatAreaApi, type ChatRecordResp, type ChatAreaResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(false); const chatAreaId = ref(''); const roleFilter = ref(''); const limit = ref(20)
const records = ref<ChatRecordResp[]>([]); const total = ref(0); const page = ref(1)
const chatAreaItems = ref<{label: string; value: string}[]>([])
const toolCallsDialog = ref(false); const toolCallsTarget = ref<ChatRecordResp | null>(null)

const headers = [
  { title: 'ID', key: 'id' }, { title: '用户 QQ', key: 'user_id' }, { title: 'Role', key: 'role' },
  { title: '内容', key: 'content' }, { title: 'Token', key: 'token_count' }, { title: 'Tool Calls', key: 'tool_calls' },
  { title: '时间', key: 'created_at' },
]

async function fetchChatAreas() { try { const list = (await chatAreaApi.list()).data.data || []; chatAreaItems.value = list.map((c: ChatAreaResp) => ({ label: `${c.area_type==='private'?'私聊':'群聊'} ${c.target_id} (${c.id.slice(0,8)})`, value: c.id })) } catch { toastStore.error('获取 ChatArea 列表失败') } }
async function fetch() {
  if (!chatAreaId.value) return
  loading.value = true
  const offset = (page.value - 1) * limit.value
  try {
    const res = await chatRecordApi.list(chatAreaId.value, { limit: limit.value, offset, role: roleFilter.value || undefined })
    records.value = res.data.data.list; total.value = res.data.data.total
  } catch { toastStore.error('查询失败') } finally { loading.value = false }
}

function showToolCalls(item: ChatRecordResp) { toolCallsTarget.value = item; toolCallsDialog.value = true }
onMounted(fetchChatAreas)
</script>
