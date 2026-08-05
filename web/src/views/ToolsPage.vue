<template>
  <div>
    <div class="page-header">
      <div class="page-title">Tool 管理</div>
      <div class="page-subtitle">内置工具运行时常驻 (仅展示), 自定义工具支持启停</div>
    </div>
    <v-alert type="info" variant="tonal" class="mb-4" density="compact">
      <strong>仅管理员</strong>：开启后该工具只能由 Admins（管理员列表）内的人触发，防止提示词注入诱导 Agent 执行敏感操作（踢人/禁言/撤回等）。内置群管理工具默认开启。
    </v-alert>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.is_builtin="{ item }">
        <v-chip size="small" :color="item.is_builtin ? 'primary' : 'grey'" variant="tonal">{{ item.is_builtin ? '内置' : '自定义' }}</v-chip>
      </template>
      <template #item.is_active="{ item }">
        <v-chip size="small" :color="item.is_active ? 'success' : 'default'" variant="tonal">{{ item.is_active ? '启用' : '停用' }}</v-chip>
      </template>
      <template #item.admin_only="{ item }">
        <v-switch
          :model-value="item.admin_only"
          color="error"
          density="compact"
          hide-details
          :loading="adminOnlyLoading === item.id"
          @update:model-value="(v) => toggleAdminOnly(item, !!v)"
        />
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-eye" size="small" variant="text" color="info" @click="showDetail(item)" />
      </template>
    </v-data-table>

    <!-- 详情弹窗 -->
    <v-dialog v-model="detailDialog" max-width="640">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center justify-space-between pa-4">
          <span class="text-body-1">Tool 详情</span>
          <v-btn icon="mdi-close" size="small" variant="text" @click="detailDialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-4">
          <div class="text-caption text-medium-emphasis mb-1">名称</div>
          <div class="text-body-1 font-weight-medium mb-4">{{ detail?.name }}</div>

          <div class="text-caption text-medium-emphasis mb-1">描述</div>
          <div class="text-body-2 mb-4" style="white-space: pre-wrap; word-break: break-word">{{ detail?.description || '(无描述)' }}</div>

          <div class="d-flex align-center mb-2" style="gap:12px">
            <v-chip size="small" :color="detail?.is_builtin ? 'primary' : 'grey'" variant="tonal">{{ detail?.is_builtin ? '内置' : '自定义' }}</v-chip>
            <v-chip size="small" :color="detail?.is_active ? 'success' : 'default'" variant="tonal">{{ detail?.is_active ? '启用' : '停用' }}</v-chip>
          </div>

          <div v-if="detail?.parameters && Object.keys(detail.parameters).length > 0" class="mt-4">
            <div class="text-caption text-medium-emphasis mb-1">参数 Schema</div>
            <pre class="code-block">{{ JSON.stringify(detail.parameters, null, 2) }}</pre>
          </div>
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
import { ref, onMounted } from 'vue'
import { toolApi, type ToolConfigResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true)
const items = ref<ToolConfigResp[]>([])
const detailDialog = ref(false)
const detail = ref<ToolConfigResp | null>(null)
const adminOnlyLoading = ref<string | null>(null)

const headers = [
  { title: '名称', key: 'name' },
  { title: '描述', key: 'description' },
  { title: '类型', key: 'is_builtin' },
  { title: '仅管理员', key: 'admin_only', align: 'center' as const },
  { title: 'Active', key: 'is_active', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

async function fetch() {
  loading.value = true
  try { items.value = (await toolApi.list()).data.data || [] }
  catch { toastStore.error('获取失败') }
  finally { loading.value = false }
}

function showDetail(item: ToolConfigResp) {
  detail.value = item
  detailDialog.value = true
}

async function toggleAdminOnly(item: ToolConfigResp, v: boolean) {
  adminOnlyLoading.value = item.id
  try {
    await toolApi.updateAdminOnly(item.id, v)
    item.admin_only = v
    toastStore.success(v ? `已开启「${item.name}」仅管理员` : `已关闭「${item.name}」仅管理员`)
  } catch {
    toastStore.error('操作失败')
  } finally {
    adminOnlyLoading.value = null
  }
}

onMounted(fetch)
</script>

<style scoped>
.code-block {
  background: #1e1e1e;
  color: #d4d4d4;
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.5;
  padding: 12px 16px;
  border-radius: 6px;
  overflow-x: auto;
  white-space: pre;
  margin: 0;
}
</style>
