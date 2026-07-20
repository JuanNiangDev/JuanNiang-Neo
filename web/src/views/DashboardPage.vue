<template>
  <div>
    <div class="page-header">
      <div class="page-title">仪表盘</div>
      <div class="page-subtitle">系统运行状态与资源概览</div>
    </div>

    <v-skeleton-loader v-if="loading" type="table-row@8" />

    <template v-else>
      <!-- Stat Cards Row 1 -->
      <v-row dense>
        <v-col v-for="card in topStats" :key="card.label" cols="12" sm="6" md="3">
          <v-card class="stat-card pa-4" color="surface">
            <div class="d-flex align-center justify-space-between">
              <div>
                <div class="text-caption text-medium-emphasis">{{ card.label }}</div>
                <div class="text-h4 font-weight-bold mt-1">{{ card.value }}</div>
              </div>
              <div class="stat-icon" :class="card.color">
                <v-icon size="24">{{ card.icon }}</v-icon>
              </div>
            </div>
            <div class="text-caption text-medium-emphasis mt-2">{{ card.subtitle }}</div>
          </v-card>
        </v-col>
      </v-row>

      <!-- Stat Cards Row 2 -->
      <v-row dense class="mt-3">
        <v-col v-for="card in bottomStats" :key="card.label" cols="12" sm="6" md="3">
          <v-card class="stat-card pa-4" color="surface">
            <div class="d-flex align-center justify-space-between">
              <div>
                <div class="text-caption text-medium-emphasis">{{ card.label }}</div>
                <div class="text-h4 font-weight-bold mt-1">{{ card.value }}</div>
              </div>
              <div class="stat-icon" :class="card.color">
                <v-icon size="24">{{ card.icon }}</v-icon>
              </div>
            </div>
          </v-card>
        </v-col>
      </v-row>

      <!-- System & Services -->
      <v-row class="mt-3">
        <v-col cols="12" md="6" class="d-flex">
          <v-card rounded="lg" elevation="1" color="surface" class="flex-grow-1">
            <v-card-item><template #title><span class="text-h6 font-weight-bold">系统资源</span></template></v-card-item>
            <v-card-text>
              <v-list density="compact" color="surface">
                <v-list-item>
                  <template #prepend><v-icon color="blue" class="me-3">mdi-cpu-64-bit</v-icon></template>
                  <v-list-item-title>CPU / Goroutines</v-list-item-title>
                  <v-list-item-subtitle>{{ overview?.cpu_count ?? '-' }} 核 · {{ overview?.goroutine_num ?? 0 }} goroutines</v-list-item-subtitle>
                </v-list-item>
                <v-list-item>
                  <template #prepend><v-icon color="green" class="me-3">mdi-memory</v-icon></template>
                  <v-list-item-title>Heap Inuse / Alloc</v-list-item-title>
                  <v-list-item-subtitle>{{ formatBytes(overview?.mem_heap_inuse_bytes ?? 0) }} / {{ formatBytes(overview?.mem_alloc_bytes ?? 0) }}</v-list-item-subtitle>
                </v-list-item>
                <v-list-item>
                  <template #prepend><v-icon color="orange" class="me-3">mdi-harddisk</v-icon></template>
                  <v-list-item-title>系统内存</v-list-item-title>
                  <v-list-item-subtitle>{{ formatBytes(overview?.mem_sys_bytes ?? 0) }}</v-list-item-subtitle>
                </v-list-item>
                <v-list-item>
                  <template #prepend><v-icon color="purple" class="me-3">mdi-language-go</v-icon></template>
                  <v-list-item-title>Go 版本</v-list-item-title>
                  <v-list-item-subtitle>{{ overview?.go_version ?? '-' }}</v-list-item-subtitle>
                </v-list-item>
              </v-list>
            </v-card-text>
          </v-card>
        </v-col>

        <v-col cols="12" md="6" class="d-flex">
          <v-card rounded="lg" elevation="1" color="surface" class="flex-grow-1">
            <v-card-item><template #title><span class="text-h6 font-weight-bold">服务状态</span></template></v-card-item>
            <v-card-text class="pa-0">
              <v-table density="compact" hover>
                <thead>
                  <tr>
                    <th class="text-caption font-weight-bold">服务</th>
                    <th class="text-caption font-weight-bold">状态</th>
                    <th class="text-caption font-weight-bold">详情</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>
                      <span class="status-dot" :class="adapterRunning ? 'active' : 'inactive'" />
                      Adapter
                    </td>
                    <td>
                      <v-chip :color="adapterRunning ? 'success' : 'warning'" size="x-small" variant="tonal">
                        {{ adapterRunning ? 'Running' : 'Stopped' }}
                      </v-chip>
                    </td>
                    <td class="text-caption text-medium-emphasis">
                      {{ adapterRunning && adapterStatus.conn_count ? adapterStatus.conn_count + ' 连接' : '-' }}
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <span class="status-dot" :class="overview?.t2i_healthy ? 'active' : 'inactive'" />
                      T2I
                    </td>
                    <td>
                      <v-chip :color="overview?.t2i_active ? 'success' : 'grey'" size="x-small" variant="tonal">
                        {{ overview?.t2i_active ? 'Loaded' : 'Not Loaded' }}
                      </v-chip>
                    </td>
                    <td>
                      <v-chip :color="overview?.t2i_healthy ? 'success' : 'error'" size="x-small" variant="tonal">
                        {{ overview?.t2i_healthy ? 'Healthy' : 'Unhealthy' }}
                      </v-chip>
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <span class="status-dot" :class="overview?.sandbox_healthy ? 'active' : 'inactive'" />
                      Sandbox
                    </td>
                    <td>
                      <v-chip :color="overview?.sandbox_active ? 'success' : 'grey'" size="x-small" variant="tonal">
                        {{ overview?.sandbox_active ? 'Loaded' : 'Not Loaded' }}
                      </v-chip>
                    </td>
                    <td>
                      <v-chip :color="overview?.sandbox_healthy ? 'success' : 'error'" size="x-small" variant="tonal">
                        {{ overview?.sandbox_healthy ? 'Healthy' : 'Unhealthy' }}
                      </v-chip>
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <span class="status-dot active" />
                      API Server
                    </td>
                    <td>
                      <v-chip color="success" size="x-small" variant="tonal">Running</v-chip>
                    </td>
                    <td class="text-caption text-medium-emphasis">-</td>
                  </tr>
                </tbody>
              </v-table>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { overviewApi, adapterApi, type OverviewResp, type AdapterStatus } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true)
const overview = ref<OverviewResp | null>(null)
const adapterRunning = ref(false)
const adapterStatus = ref<AdapterStatus>({ running: false, listen_addr: '', self_id: 0, conn_count: 0, conn_ids: [], conns: [] })

const topStats = computed(() => [
  { label: 'Chat Areas', value: overview.value?.chat_area_count ?? 0, subtitle: '活跃聊天区域', icon: 'mdi-forum', color: 'pink' },
  { label: 'Providers', value: overview.value?.provider_count ?? 0, subtitle: 'LLM 提供商', icon: 'mdi-brain', color: 'blue' },
  { label: 'Sessions', value: overview.value?.session_count ?? 0, subtitle: '活跃会话', icon: 'mdi-chat-processing', color: 'green' },
  { label: 'Token 用量', value: formatNumber(overview.value?.total_token_usage ?? 0), subtitle: '累计 Token 消耗', icon: 'mdi-chart-line', color: 'orange' },
])

const bottomStats = computed(() => [
  { label: 'MCP 服务', value: overview.value?.mcp_count ?? 0, icon: 'mdi-server-network', color: 'purple' },
  { label: 'Skills', value: overview.value?.skill_count ?? 0, icon: 'mdi-lightning-bolt', color: 'teal' },
  { label: 'Plugins', value: overview.value?.plugin_count ?? 0, icon: 'mdi-puzzle', color: 'pink' },
  { label: 'Adapter', value: adapterRunning.value ? 'Running' : 'Stopped', icon: 'mdi-connection', color: adapterRunning.value ? 'green' : 'orange' as any },
])

async function fetchAll() {
  loading.value = true
  try {
    const [ov, ad] = await Promise.all([
      overviewApi.get(),
      adapterApi.getStatus().catch(() => null),
    ])
    overview.value = ov.data.data
    if (ad?.data?.data) {
      adapterStatus.value = ad.data.data
      adapterRunning.value = ad.data.data.running
    }
  } catch { toastStore.error('获取数据失败') } finally { loading.value = false }
}

function formatNumber(n: number) { return n >= 1_000_000 ? (n / 1_000_000).toFixed(1) + 'M' : n >= 1_000 ? (n / 1_000).toFixed(1) + 'K' : String(n) }
function formatBytes(bytes: number) { return bytes >= 1_073_741_824 ? (bytes / 1_073_741_824).toFixed(2) + ' GB' : bytes >= 1_048_576 ? (bytes / 1_048_576).toFixed(1) + ' MB' : bytes >= 1_024 ? (bytes / 1_024).toFixed(0) + ' KB' : bytes + ' B' }

onMounted(() => { fetchAll(); window.addEventListener('global-refresh', fetchAll) })
onBeforeUnmount(() => { window.removeEventListener('global-refresh', fetchAll) })
</script>
