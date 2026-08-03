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
          <v-select v-model="roleFilter" :items="[{title:'全部',value:''},{title:'User',value:'user'},{title:'Assistant',value:'assistant'},{title:'Tool',value:'tool'}]" label="Role 过滤" density="compact" hide-details />
        </v-col>
        <v-col cols="6" md="2">
          <v-text-field v-model.number="limit" label="每页数量" type="number" density="compact" hide-details />
        </v-col>
        <v-col cols="12" md="2">
          <v-btn color="primary" variant="tonal" block @click="fetch" :loading="loading">查询</v-btn>
        </v-col>
      </v-row>
    </v-card>

    <v-data-table v-if="chatAreaId" :headers="headers" :items="records" :loading="loading" :items-per-page="-1" hide-default-footer hover @click:row="openDetail">
      <template #item.role="{ item }">
        <v-chip size="small" variant="tonal" :color="roleColor(item.role)">{{ item.role }}</v-chip>
      </template>
      <template #item.content="{ item }">
        <div style="max-width:400px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" class="text-caption">{{ item.content }}</div>
      </template>
      <template #item.tool_calls="{ item }">
        <v-chip v-if="item.tool_calls" size="x-small" variant="tonal" color="info" @click.stop="openDetailById(item)">查看</v-chip>
      </template>
    </v-data-table>

    <div v-else class="empty-state"><v-icon class="empty-icon" color="secondary">mdi-message-text</v-icon><div class="empty-text">请先输入 Chat Area ID 进行查询</div></div>

    <!-- Pagination -->
    <div v-if="total > perPage" class="d-flex align-center justify-center mt-4">
      <v-pagination v-model="page" :length="pageCount" :total-visible="7" @update:model-value="fetch" />
    </div>

    <!-- 消息详情弹窗 -->
    <v-dialog v-model="detailDialog" max-width="900">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center justify-space-between pa-4">
          <div class="d-flex align-center" style="gap:12px">
            <v-chip size="small" variant="tonal" :color="roleColor(detail.role)">{{ detail.role }}</v-chip>
            <span class="text-body-1">消息详情</span>
          </div>
          <v-btn icon="mdi-close" size="small" variant="text" @click="detailDialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-0">
          <v-row no-gutters>
            <!-- 左：元信息 (1 份) -->
            <v-col cols="4" class="detail-left pa-4">
              <div class="detail-label">ID</div>
              <div class="detail-value mb-4">{{ detail.id }}</div>
              <div class="detail-label">Chat Area</div>
              <div class="detail-value mb-4">{{ detail.chat_area_id }}</div>
              <div class="detail-label">用户 QQ</div>
              <div class="detail-value mb-4">{{ detail.user_id }}</div>
              <div class="detail-label">Token 数</div>
              <div class="detail-value mb-4">{{ detail.token_count }}</div>
              <div class="detail-label">时间</div>
              <div class="detail-value">{{ formatTime(detail.created_at) }}</div>
            </v-col>

            <!-- 右：内容 + Tool Calls (2 份) -->
            <v-col cols="8" class="detail-right pa-4">
              <div class="text-caption text-medium-emphasis mb-2">内容</div>
              <div class="content-box">{{ detail.content }}</div>

              <div v-if="hasToolCalls" class="mt-4">
                <div class="text-caption text-medium-emphasis mb-2">Tool Calls (JSON)</div>
                <pre class="code-block">{{ toolCallsJson }}</pre>
              </div>
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider />
        <v-card-actions class="pa-4">
          <v-spacer />
          <v-btn variant="text" @click="detailDialog = false">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { chatRecordApi, chatAreaApi, type ChatRecordResp, type ChatAreaResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(false); const chatAreaId = ref(''); const roleFilter = ref(''); const limit = ref(20)
const records = ref<ChatRecordResp[]>([]); const total = ref(0); const page = ref(1)
const chatAreaItems = ref<{label: string; value: string}[]>([])
const detailDialog = ref(false)
const detail = ref<ChatRecordResp>(emptyDetail())

const headers = [
  { title: 'ID', key: 'id' }, { title: '用户 QQ', key: 'user_id' }, { title: 'Role', key: 'role' },
  { title: '内容', key: 'content' }, { title: 'Token', key: 'token_count' }, { title: 'Tool Calls', key: 'tool_calls' },
  { title: '时间', key: 'created_at' },
]

// 每页条数（防 0 / 空值）
const perPage = computed(() => Math.max(1, Number(limit.value) || 20))
// 总页数（防除零）
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / perPage.value)))

async function fetchChatAreas() { try { const list = (await chatAreaApi.list()).data.data || []; chatAreaItems.value = list.map((c: ChatAreaResp) => ({ label: `${c.area_type==='private'?'私聊':'群聊'} ${c.target_id} (${c.id.slice(0,8)})`, value: c.id })) } catch { toastStore.error('获取 ChatArea 列表失败') } }
async function fetch() {
  if (!chatAreaId.value) return
  loading.value = true
  const offset = (page.value - 1) * perPage.value
  try {
    const res = await chatRecordApi.list(chatAreaId.value, { limit: perPage.value, offset, role: roleFilter.value || undefined })
    records.value = res.data.data.list; total.value = res.data.data.total
  } catch { toastStore.error('查询失败') } finally { loading.value = false }
}

// 切片越界时回退到最后一页
watch(page, (p) => { if (p > pageCount.value) page.value = Math.max(1, pageCount.value) })

// 筛选条件变化时重置到第一页并重新查询
watch(chatAreaId, () => { page.value = 1; fetch() })
watch(roleFilter, () => { page.value = 1; fetch() })
// 每页数量变化仅重置页码（由「查询」按钮触发请求）
watch(limit, () => { page.value = 1 })

function emptyDetail(): ChatRecordResp {
  return { id: 0, chat_area_id: '', user_id: 0, role: '', content: '', token_count: 0, tool_calls: null, created_at: '' }
}

const roleColor = (r: string): string => (r === 'user' ? 'primary' : r === 'assistant' ? 'success' : 'warning')

const hasToolCalls = computed(() => !!detail.value.tool_calls)

const toolCallsJson = computed(() => {
  if (!detail.value.tool_calls) return '// 无 Tool Calls'
  try { return JSON.stringify(detail.value.tool_calls, null, 2) } catch { return '// 序列化失败' }
})

function formatTime(t: string): string {
  try { return new Date(t).toLocaleString('zh-CN', { hour12: false }) } catch { return t }
}

function openDetailById(item: ChatRecordResp) {
  detail.value = { ...item }
  detailDialog.value = true
}

// 兼容 Vuetify 3 不同版本 @click:row 的 payload 结构
function openDetail(_e: unknown, payload: any) {
  const raw = payload?.item?.raw ?? payload?.item?.value ?? payload?.item ?? payload?.row ?? payload
  if (!raw?.id) return
  openDetailById(raw)
}

onMounted(fetchChatAreas)
</script>

<style scoped>
.detail-left {
  background: rgba(var(--v-theme-surface-variant), 0.05);
  border-right: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  min-height: 360px;
}
.detail-right {
  min-height: 360px;
  display: flex;
  flex-direction: column;
}
.detail-label {
  font-size: 12px;
  color: rgba(var(--v-theme-on-surface), 0.5);
  margin-bottom: 4px;
}
.detail-value {
  font-size: 13px;
  color: rgb(var(--v-theme-on-surface));
  word-break: break-all;
}
.content-box {
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 320px;
  overflow-y: auto;
  padding: 12px;
  background: rgba(var(--v-theme-on-surface), 0.04);
  border-radius: 8px;
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}
:deep(.v-data-table__tr) {
  cursor: pointer;
}
:deep(.v-data-table__tr:hover) {
  background: rgba(var(--v-theme-primary), 0.04) !important;
}
</style>
