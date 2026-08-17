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
              <div class="d-flex align-center" style="gap: 28px">
                <!-- 左：Goroutine 仪表盘 -->
                <div class="goroutine-box">
                  <div ref="goroutineGaugeEl" class="goroutine-gauge-chart" />
                  <div class="goroutine-label">Goroutines</div>
                </div>

                <!-- 右：进度条 -->
                <div class="flex-grow-1">
                  <div class="progress-item">
                    <div class="progress-head">
                      <span>内存使用</span>
                      <span class="progress-val">{{ heapMB }} MB</span>
                    </div>
                    <v-progress-linear :model-value="heapPct" height="8" rounded color="#818cf8" :bg-color="trackColor" />
                  </div>
                  <div class="progress-item">
                    <div class="progress-head">
                      <span>内存分配</span>
                      <span class="progress-val">{{ allocMB }} MB</span>
                    </div>
                    <v-progress-linear :model-value="allocPct" height="8" rounded color="#34d399" :bg-color="trackColor" />
                  </div>
                  <div class="progress-item">
                    <div class="progress-head">
                      <span>CPU 核数</span>
                      <span class="progress-val">{{ cpuCount }} 核</span>
                    </div>
                    <v-progress-linear :model-value="cpuPct" height="8" rounded color="#60a5fa" :bg-color="trackColor" />
                  </div>
                </div>
              </div>
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
import { ref, computed, nextTick, onMounted, onBeforeUnmount, watch } from 'vue'
import { init, use, type EChartsType } from 'echarts/core'
import { LineChart, GaugeChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useTheme } from 'vuetify'
import { overviewApi, adapterApi, type OverviewResp, type AdapterStatus, type DailyTokenUsageResp } from '@/api'
import { useToastStore } from '@/stores/toast'

use([LineChart, GaugeChart, GridComponent, TooltipComponent, CanvasRenderer])

const toastStore = useToastStore()
const theme = useTheme()
const loading = ref(true)
const overview = ref<OverviewResp | null>(null)
const adapterRunning = ref(false)
const adapterStatus = ref<AdapterStatus>({ running: false, listen_addr: '', self_id: 0, conn_count: 0, conn_ids: [], conns: [] })

// Token 用量折线图
const tokenDays = ref('7')
const chartEl = ref<HTMLDivElement | null>(null)
let chart: EChartsType | null = null

// 系统资源：Goroutine 仪表盘
const goroutineGaugeEl = ref<HTMLDivElement | null>(null)
let goroutineGaugeChart: EChartsType | null = null

// ===== 主题感知的图表色 =====
// 从 Vuetify 当前主题的 token 取色，深浅切换时图表随之更新（ECharts 无法直接用 CSS var）
function hexToRgb(hex: string): string {
  const short = hex.replace('#', '')
  const full = short.length === 3 ? short.split('').map(c => c + c).join('') : short
  const n = parseInt(full, 16)
  return `${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}`
}
function inkColor(alpha: number): string {
  const onSurface = theme.global.current.value.colors['on-surface']
  return `rgba(${hexToRgb(onSurface)}, ${alpha})`
}
function tokenColor(name: string): string {
  return theme.global.current.value.colors[name] ?? '#6366f1'
}
// 进度条轨道色：随主题在浅底/深底上都可辨
const trackColor = computed(() => `rgba(${hexToRgb(theme.global.current.value.colors['on-surface'])}, 0.12)`)

const lastTokenData = ref<DailyTokenUsageResp[]>([])

// 主题切换后重绘两张图
watch(
  () => theme.global.name.value,
  () => {
    if (lastTokenData.value.length) renderTokenChart(lastTokenData.value)
    if (goroutineGaugeEl.value) renderGauges()
  },
)

// 系统资源进度条数据（内存使用显示数值，非百分比）
const heapMB = computed(() => Math.round((overview.value?.mem_heap_inuse_bytes ?? 0) / 1048576))
const allocMB = computed(() => Math.round((overview.value?.mem_alloc_bytes ?? 0) / 1048576))
const cpuCount = computed(() => overview.value?.cpu_count ?? 0)
const goroutines = computed(() => overview.value?.goroutine_num ?? 0)

const maxMemMB = computed(() => Math.max(10, Math.ceil(Math.max(heapMB.value, allocMB.value) / 10) * 10))
const maxCpu = computed(() => Math.max(4, Math.ceil(cpuCount.value / 4) * 4))
const maxGoroutines = computed(() => Math.max(10, Math.ceil(goroutines.value / 10) * 10))

const heapPct = computed(() => Math.min(100, Math.round((heapMB.value / maxMemMB.value) * 100)))
const allocPct = computed(() => Math.min(100, Math.round((allocMB.value / maxMemMB.value) * 100)))
const cpuPct = computed(() => Math.min(100, Math.round((cpuCount.value / maxCpu.value) * 100)))

async function fetchTokenUsage() {
  try {
    const res = await overviewApi.dailyTokenUsage(Number(tokenDays.value))
    renderTokenChart((res.data.data ?? []) as DailyTokenUsageResp[])
  } catch { toastStore.error('获取 Token 用量失败') }
}

function renderTokenChart(list: DailyTokenUsageResp[]) {
  if (!chartEl.value) return
  if (!chart) chart = init(chartEl.value)
  lastTokenData.value = list
  const primary = tokenColor('primary')
  chart.setOption({
    grid: { left: 16, right: 16, top: 36, bottom: 8, containLabel: true },
    tooltip: { trigger: 'axis', valueFormatter: (v: unknown) => `${Number(v).toLocaleString()} tokens` },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: list.map(i => i.date.slice(5)),
      axisLine: { lineStyle: { color: inkColor(0.25) } },
      axisLabel: { color: inkColor(0.65) },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: inkColor(0.65) },
      splitLine: { lineStyle: { color: inkColor(0.1) } },
    },
    series: [{
      name: 'Token',
      type: 'line',
      smooth: true,
      symbolSize: 6,
      data: list.map(i => i.token_count),
      lineStyle: { width: 2, color: primary },
      itemStyle: { color: primary },
      areaStyle: { color: `rgba(${hexToRgb(primary)}, 0.18)` },
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
  } catch { toastStore.error('获取数据失败') } finally { loading.value = false }
}

// Goroutine 环形仪表盘
function renderGauges() {
  if (!goroutineGaugeEl.value) return
  if (!goroutineGaugeChart) goroutineGaugeChart = init(goroutineGaugeEl.value)

  const value = goroutines.value
  const max = maxGoroutines.value

  goroutineGaugeChart.setOption({
    series: [{
      type: 'gauge',
      center: ['50%', '50%'],
      radius: '100%',
      startAngle: 210,
      endAngle: -30,
      min: 0,
      max,
      axisLine: { lineStyle: { width: 10, color: [[1, inkColor(0.08)]] } },
      progress: { show: true, width: 10, itemStyle: { color: tokenColor('stat-orange') } },
      pointer: { show: false },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: { show: false },
      detail: {
        valueAnimation: true,
        offsetCenter: [0, '0%'],
        fontSize: 20,
        fontWeight: 700,
        color: tokenColor('on-surface'),
      },
      data: [{ value }],
    }],
  })
}

async function onGlobalRefresh() {
  await fetchAll()
  // 需在 loading 置 false（DOM 已渲染出仪表盘容器）后再绘制
  await nextTick()
  renderGauges()
  await fetchTokenUsage()
}

function formatNumber(n: number) { return n >= 1_000_000 ? (n / 1_000_000).toFixed(1) + 'M' : n >= 1_000 ? (n / 1_000).toFixed(1) + 'K' : String(n) }

onMounted(async () => {
  await onGlobalRefresh()
  window.addEventListener('global-refresh', onGlobalRefresh)
  window.addEventListener('resize', onResize)
})

function onResize() { chart?.resize(); goroutineGaugeChart?.resize() }

onBeforeUnmount(() => {
  window.removeEventListener('global-refresh', onGlobalRefresh)
  window.removeEventListener('resize', onResize)
  chart?.dispose()
  chart = null
  goroutineGaugeChart?.dispose()
  goroutineGaugeChart = null
})
</script>

<style scoped>
.token-chart {
  width: 100%;
  height: 300px;
}
.goroutine-box {
  width: 150px;
  flex-shrink: 0;
  text-align: center;
}
.goroutine-gauge-chart {
  width: 150px;
  height: 150px;
}
.goroutine-label {
  margin-top: 6px;
  font-size: 12px;
  color: rgba(var(--v-theme-on-surface), 0.55);
}
.progress-item {
  margin-bottom: 22px;
}
.progress-item:last-child {
  margin-bottom: 0;
}
.progress-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
  color: rgba(var(--v-theme-on-surface), 0.7);
  margin-bottom: 8px;
}
.progress-val {
  font-family: 'Space Mono', 'JetBrains Mono', monospace;
  font-weight: 700;
  color: rgb(var(--v-theme-on-surface));
}
</style>
