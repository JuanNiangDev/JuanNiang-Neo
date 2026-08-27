<template>
  <div>
    <div class="page-header"><div class="page-title">RAG 向量检索</div><div class="page-subtitle">RAG-Service 配置与健康管理（记忆/知识语义检索）</div></div>

    <v-row>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">配置</span></template></v-card-item>
          <v-card-text>
            <v-form>
              <v-text-field v-model="form.base_url" label="服务地址" :placeholder="DEFAULT_RAG_URL" hint="RAG-Service 监听地址，默认 http://localhost:3000" class="mb-2" />
              <v-text-field v-model.number="form.timeout" label="超时 (秒)" type="number" class="mb-2" />
              <v-switch v-model="form.is_active" label="启用" color="primary" class="mb-2" />
              <v-btn color="primary" variant="tonal" block @click="save" :loading="saving">保存配置</v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1" class="h-100 d-flex flex-column">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">健康状态</span></template></v-card-item>
          <v-card-text class="flex-grow-1">
            <v-list density="compact">
              <v-list-item>
                <template #prepend><span class="status-dot" :class="config.healthy ? 'active' : 'error'" /></template>
                <v-list-item-title>健康状态</v-list-item-title>
                <v-list-item-subtitle>{{ config.healthy ? '健康' : '异常' }}</v-list-item-subtitle>
              </v-list-item>
            </v-list>
          </v-card-text>
          <v-card-actions class="px-4 pb-4">
            <v-btn variant="tonal" block @click="checkHealth" :loading="checking">
              <v-icon class="me-1">mdi-heart-pulse</v-icon> 检查健康
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>

    <!-- 服务信息面板（GET /info 数据展示） -->
    <v-card rounded="lg" elevation="1" class="mt-2">
      <v-card-item>
        <template #title><span class="text-h6 font-weight-bold">服务信息</span></template>
        <template #append>
          <v-chip v-if="infoReady" size="small" :color="info?.status === 'ok' ? 'success' : 'warning'" variant="tonal">
            status: {{ info?.status || '-' }}
          </v-chip>
        </template>
      </v-card-item>
      <v-card-text v-if="infoReady">
        <v-row>
          <v-col cols="12" md="4" class="service-info-col">
            <div class="text-subtitle-2 font-weight-bold mb-1">Embedding 模型</div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">模型</span><span>{{ info?.model?.model_name || '-' }}</span></div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">维度</span><span>{{ info?.model?.dim ?? '-' }}</span></div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">参数量</span><span>{{ formatParams(info?.model?.n_params) }}</span></div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">线程数</span><span>{{ info?.model?.n_threads ?? '-' }}</span></div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">上下文长度</span><span>{{ info?.model?.n_ctx ?? '-' }}</span></div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">就绪</span><span>{{ info?.model?.ready ? '✅ 是' : '❌ 否' }}</span></div>
            <div v-if="info?.model?.error" class="text-caption text-error mt-1">{{ info.model.error }}</div>
          </v-col>
          <v-col cols="12" md="4" class="service-info-col">
            <div class="text-subtitle-2 font-weight-bold mb-1">进程内存</div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">常驻内存 (RSS)</span><span>{{ formatKB(info?.memory?.rss_kb) }}</span></div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">虚拟内存</span><span>{{ formatKB(info?.memory?.vsize_kb) }}</span></div>
          </v-col>
          <v-col cols="12" md="4" class="service-info-col">
            <div class="text-subtitle-2 font-weight-bold mb-1">向量库规模</div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">Tag 总数</span><span>{{ totalTags }}</span></div>
            <div class="d-flex justify-space-between py-1 text-body-2"><span class="text-medium-emphasis">Chunk 总数</span><span>{{ totalChunks }}</span></div>
            <v-divider class="my-2" />
            <template v-if="scoopEntries.length">
              <div v-for="[name, st] in scoopEntries" :key="name" class="py-1 text-body-2">
                <div class="d-flex justify-space-between">
                  <span class="text-medium-emphasis">{{ scoopLabel(name) }}</span>
                  <span class="text-caption">{{ st.tags }} tag / {{ st.chunks }} chunk</span>
                </div>
              </div>
            </template>
            <div v-else class="text-caption text-medium-emphasis">（无分库数据）</div>
          </v-col>
        </v-row>
      </v-card-text>
      <v-card-text v-else class="text-caption text-medium-emphasis">
        尚未获取到服务信息，点击「健康状态」中的 检查健康 按钮或启用服务后再试。
      </v-card-text>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ragApi, type RAGConfigResp, type UpdateRAGConfigReq, type RAGInfoResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const DEFAULT_RAG_URL = 'http://localhost:3000'

const toastStore = useToastStore()
const saving = ref(false); const checking = ref(false)
const config = ref<RAGConfigResp>({ base_url: '', timeout: 30, is_active: false, healthy: false })
const form = ref<UpdateRAGConfigReq>({ base_url: DEFAULT_RAG_URL, timeout: 30, is_active: false })
const info = ref<RAGInfoResp | null>(null)
const infoReady = ref(false)

function formatKB(kb?: number) {
  if (!kb) return '-'
  if (kb >= 1024 * 1024) return (kb / 1024 / 1024).toFixed(1) + ' GB'
  if (kb >= 1024) return (kb / 1024).toFixed(1) + ' MB'
  return kb + ' KB'
}

function formatParams(n?: number) {
  if (!n) return '-'
  if (n >= 1e9) return (n / 1e9).toFixed(2) + ' B'
  if (n >= 1e6) return (n / 1e6).toFixed(1) + ' M'
  return String(n)
}

// ---------- 向量库规模（各 scoop 汇总） ----------
// 后端 /info 返回 scoops: { scoop名: { tags, chunks } }，总数由前端汇总
const scoopEntries = computed(() => Object.entries(info.value?.scoops ?? {}))
const totalTags = computed(() => scoopEntries.value.reduce((s, [, st]) => s + (st.tags || 0), 0) || '-')
const totalChunks = computed(() => scoopEntries.value.reduce((s, [, st]) => s + (st.chunks || 0), 0) || '-')

// scoopLabel 分库名 → 中文展示名（未知分库原样显示）
function scoopLabel(name: string) {
  const labels: Record<string, string> = {
    knowledge: '知识库',
    groupmgr: '群管理语录',
    memory: '长期记忆',
  }
  return labels[name] || name
}

async function fetchConfig() {
  try {
    const res = await ragApi.getConfig()
    config.value = res.data.data
    form.value = {
      base_url: config.value.base_url || DEFAULT_RAG_URL,
      timeout: config.value.timeout || 30,
      is_active: config.value.is_active,
    }
  } catch { toastStore.error('获取配置失败') }
}
async function save() {
  saving.value = true
  try {
    const res = await ragApi.updateConfig(form.value)
    config.value = res.data.data
    toastStore.success(config.value.healthy ? '已保存，RAG 服务健康' : '已保存（健康检查失败，将走降级路径）')
    await refreshStatus()
  } catch { toastStore.error('保存失败') } finally { saving.value = false }
}
async function refreshStatus() {
  checking.value = true
  try {
    const res = await ragApi.health()
    config.value.healthy = res.data.data.healthy
    const infoRes = await ragApi.info()
    info.value = infoRes.data.data
    infoReady.value = true
  } catch { toastStore.error('状态刷新失败') } finally { checking.value = false }
}
// 健康检查按钮（与 T2I/Sandbox 一致）：探活 + 刷新服务信息 + toast 结果
async function checkHealth() {
  checking.value = true
  try {
    const res = await ragApi.health()
    config.value.healthy = res.data.data.healthy
    const infoRes = await ragApi.info()
    info.value = infoRes.data.data
    infoReady.value = true
    toastStore.info(config.value.healthy ? '健康检查通过' : '健康检查失败')
  } catch { toastStore.error('健康检查失败') } finally { checking.value = false }
}
onMounted(async () => { await fetchConfig(); if (config.value.is_active) await refreshStatus() })
</script>

<style scoped>
/* 服务信息三板块 md+ 断点竖线分隔 */
@media (min-width: 960px) {
  .service-info-col:not(:first-child) {
    border-left: 1px solid rgba(var(--v-theme-on-surface), 0.12);
    padding-left: 24px;
  }
}
</style>