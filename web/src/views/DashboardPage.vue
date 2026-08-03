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

      <!-- Token 用量折线图 -->
      <v-row class="mt-3">
        <v-col cols="12">
          <v-card rounded="lg" elevation="1" color="surface">
            <v-card-item>
              <template #title>
                <div class="d-flex align-center justify-space-between">
                  <span class="text-h6 font-weight-bold">Token 用量趋势</span>
                  <v-btn-toggle v-model="tokenDays" density="compact" variant="tonal" color="primary" @update:model-value="fetchTokenUsage">
                    <v-btn value="7" size="small">近7天</v-btn>
                    <v-btn value="15" size="small">近15天</v-btn>
                    <v-btn value="30" size="small">近30天</v-btn>
                  </v-btn-toggle>
                </div>
              </template>
            </v-card-item>
            <v-card-text>
              <div ref="chartEl" class="token-chart" />
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <!-- System & Services -->
      <v-row class="mt-3">
        <v-col cols="12" md="6" class="d-flex">
          <v-card rounded="lg" elevation="1" color="surface" class="flex-grow-1">
            <v-card-item>
              <template #title>
                <div class="d-flex align-center justify-space-between">
                  <span class="text-h6 font-weight-bold">系统资源</span>
                  <span class="text-caption text-medium-emphasis">{{ overview?.go_version ?? '-' }}</span>
                </div>
              </template>
            </v-card-item>
            <v-card-text>
              <div ref="gaugeEl" class="gauge-chart" />
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
import { init, use, type EChartsType } from 'echarts/core'
import { LineChart, GaugeChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { overviewApi, adapterApi, type OverviewResp, type AdapterStatus, type DailyTokenUsageResp } from '@/api'
import { useToastStore } from '@/stores/toast'

use([LineChart, GaugeChart, GridComponent, TooltipComponent, CanvasRenderer])

const toastStore = useToastStore()
const loading = ref(true)
const overview = ref<OverviewResp | null>(null)
const adapterRunning = ref(false)
const adapterStatus = ref<AdapterStatus>({ running: false, listen_addr: '', self_id: 0, conn_count: 0, conn_ids: [], conns: [] })

// Token 用量折线图
const tokenDays = ref('7')
const chartEl = ref<HTMLDivElement | null>(null)
let chart: EChartsType | null = null

// 系统资源仪表盘
const gaugeEl = ref<HTMLDivElement | null>(null)
let gaugeChart: EChartsType | null = null

async function fetchTokenUsage() {
  try {
    const res = await overviewApi.dailyTokenUsage(Number(tokenDays.value))
    renderTokenChart((res.data.data ?? []) as DailyTokenUsageResp[])
  } catch { toastStore.error('获取 Token 用量失败') }
}

function renderTokenChart(list: DailyTokenUsageResp[]) {
  if (!chartEl.value) return
  if (!chart) chart = init(chartEl.value)
  chart.setOption({
    grid: { left: 16, right: 16, top: 36, bottom: 8, containLabel: true },
    tooltip: { trigger: 'axis', valueFormatter: (v: unknown) => `${Number(v).toLocaleString()} tokens` },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: list.map(i => i.date.slice(5)),
      axisLine: { lineStyle: { color: 'rgba(255,255,255,0.25)' } },
      axisLabel: { color: 'rgba(255,255,255,0.65)' },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: 'rgba(255,255,255,0.65)' },
      splitLine: { lineStyle: { color: 'rgba(255,255,255,0.1)' } },
    },
    series: [{
      name: 'Token',
      type: 'line',
      smooth: true,
      symbolSize: 6,
      data: list.map(i => i.token_count),
      lineStyle: { width: 2, color: '#6366f1' },
      itemStyle: { color: '#6366f1' },
      areaStyle: { color: 'rgba(99,102,241,0.18)' },
    }],
  })
}

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
    renderGauges()
  } catch { toastStore.error('获取数据失败') } finally { loading.value = false }
}

// 系统资源仪表盘（2x2 环形仪表）
function renderGauges() {
  if (!gaugeEl.value) return
  if (!gaugeChart) gaugeChart = init(gaugeEl.value)

  const o = overview.value
  const sys = o?.mem_sys_bytes ?? 0
  const heap = o?.mem_heap_inuse_bytes ?? 0
  const alloc = o?.mem_alloc_bytes ?? 0
  const goroutines = o?.goroutine_num ?? 0
  const cpu = o?.cpu_count ?? 0

  const memPct = sys > 0 ? Math.min(100, Math.round((heap / sys) * 100)) : 0
  const allocMB = Math.round(alloc / 1048576)
  const heapMB = Math.round(heap / 1048576)
  const maxAllocMB = Math.max(10, Math.ceil(Math.max(allocMB, heapMB) / 10) * 10)
  const maxGoroutines = Math.max(10, Math.ceil(goroutines / 10) * 10)
  const maxCpu = Math.max(8, Math.ceil(cpu / 8) * 8)

  const gauges = [
    { name: '内存使用率', value: memPct, max: 100, unit: '%', color: '#818cf8' },
    { name: '内存分配', value: allocMB, max: maxAllocMB, unit: 'MB', color: '#34d399' },
    { name: 'Goroutines', value: goroutines, max: maxGoroutines, unit: '', color: '#fbbf24' },
    { name: 'CPU 核数', value: cpu, max: maxCpu, unit: '核', color: '#60a5fa' },
  ]

  const centers = [
    ['22%', '30%'], ['78%', '30%'],
    ['22%', '78%'], ['78%', '78%'],
  ]

  gaugeChart.setOption({
    series: gauges.map((g, i) => ({
      type: 'gauge',
      center: centers[i],
      radius: '56%',
      startAngle: 210,
      endAngle: -30,
      min: 0,
      max: g.max,
      axisLine: { lineStyle: { width: 8, color: [[1, 'rgba(255,255,255,0.08)']] } },
      progress: { show: true, width: 8, itemStyle: { color: g.color } },
      pointer: { show: false },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: { show: false },
      title: { offsetCenter: [0, '72%'], fontSize: 12, color: 'rgba(255,255,255,0.55)' },
      detail: {
        valueAnimation: true,
        offsetCenter: [0, '0%'],
        fontSize: 18,
        fontWeight: 700,
        color: '#e8ecf4',
        formatter: (v: number) => `${v}${g.unit ? ' ' + g.unit : ''}`,
      },
      data: [{ value: g.value, name: g.name }],
    })),
  })
}

async function onGlobalRefresh() {
  await fetchAll()
  await fetchTokenUsage()
}

function formatNumber(n: number) { return n >= 1_000_000 ? (n / 1_000_000).toFixed(1) + 'M' : n >= 1_000 ? (n / 1_000).toFixed(1) + 'K' : String(n) }

onMounted(async () => {
  await onGlobalRefresh()
  window.addEventListener('global-refresh', onGlobalRefresh)
  window.addEventListener('resize', onResize)
})

function onResize() { chart?.resize(); gaugeChart?.resize() }

onBeforeUnmount(() => {
  window.removeEventListener('global-refresh', onGlobalRefresh)
  window.removeEventListener('resize', onResize)
  chart?.dispose()
  chart = null
  gaugeChart?.dispose()
  gaugeChart = null
})
</script>

<style scoped>
.token-chart {
  width: 100%;
  height: 300px;
}
.gauge-chart {
  width: 100%;
  height: 300px;
}
</style>
