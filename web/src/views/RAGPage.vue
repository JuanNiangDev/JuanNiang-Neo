<template>
  <div>
    <div class="page-header"><div class="page-title">RAG 向量检索</div><div class="page-subtitle">RAG-Service 配置与健康管理（记忆/知识语义检索）</div></div>

    <v-row>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">配置</span></template></v-card-item>
          <v-card-text>
            <v-form>
              <v-text-field v-model="form.base_url" label="服务地址" hint="RAG-Service 监听地址，默认 http://127.0.0.1:3000" class="mb-2" />
              <v-text-field v-model.number="form.timeout" label="超时 (秒)" type="number" class="mb-2" />
              <v-switch v-model="form.is_active" label="启用" color="primary" class="mb-2" />
              <v-btn color="primary" variant="tonal" block @click="save" :loading="saving">保存配置</v-btn>
            </v-form>
            <v-alert type="info" variant="tonal" class="mt-4" density="comfortable">
              <div class="text-caption">
                未启用或服务不可达时，<b>记忆召回</b>自动降级为 pg_trgm 语义匹配、<b>知识库检索</b>降级为 SQL 匹配，
                不影响正常对话。启用后知识库可在「知识库」页手动「同步向量库」。
              </div>
            </v-alert>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">健康状态</span></template></v-card-item>
          <v-card-text>
            <v-list density="compact">
              <v-list-item>
                <template #prepend><span class="status-dot" :class="config.healthy ? 'active' : 'error'" /></template>
                <v-list-item-title>健康状态</v-list-item-title>
                <v-list-item-subtitle>{{ config.healthy ? '健康' : '异常' }}</v-list-item-subtitle>
              </v-list-item>
              <v-list-item v-if="infoReady">
                <template #prepend><span class="status-dot" :class="info?.model?.ready ? 'active' : 'error'" /></template>
                <v-list-item-title>Embedding 模型</v-list-item-title>
                <v-list-item-subtitle>
                  <template v-if="info?.model?.ready">{{ info?.model?.model_name || 'bge' }}（{{ info?.model?.dim }} 维，{{ info?.model?.n_threads }} 线程）</template>
                  <template v-else>{{ info?.model?.error || '未就绪' }}</template>
                </v-list-item-subtitle>
              </v-list-item>
              <v-list-item v-if="infoReady">
                <v-list-item-title>向量规模</v-list-item-title>
                <v-list-item-subtitle>{{ info?.tags }} tag / {{ info?.chunks }} 块 / RSS {{ formatKB(info?.memory?.rss_kb) }}</v-list-item-subtitle>
              </v-list-item>
            </v-list>
            <v-btn variant="tonal" class="mt-3" @click="refreshStatus" :loading="checking" block>
              <v-icon class="me-1">mdi-heart-pulse</v-icon> 检查健康 / 刷新状态
            </v-btn>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-row class="mt-2">
      <v-col cols="12">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">RAG-Service 部署提示</span></template></v-card-item>
          <v-card-text class="text-body-2" style="line-height: 1.9">
            <code>JuanNiang-RAG-Service</code> 是独立的 Rust 服务（bge 模型进程内推理，零外部依赖），先下载模型再启动：<br />
            <code class="text-primary">make download && cargo run --release</code>（默认监听 <code>127.0.0.1:3000</code>，可用 <code>RAG_PORT</code> 等环境变量覆盖）
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ragApi, type RAGConfigResp, type UpdateRAGConfigReq, type RAGInfoResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const saving = ref(false); const checking = ref(false)
const config = ref<RAGConfigResp>({ base_url: '', timeout: 30, is_active: false, healthy: false })
const form = ref<UpdateRAGConfigReq>({ base_url: '', timeout: 30, is_active: false })
const info = ref<RAGInfoResp | null>(null)
const infoReady = ref(false)

function formatKB(kb?: number) {
  if (!kb) return '-'
  if (kb >= 1024 * 1024) return (kb / 1024 / 1024).toFixed(1) + ' GB'
  if (kb >= 1024) return (kb / 1024).toFixed(1) + ' MB'
  return kb + ' KB'
}

async function fetchConfig() {
  try {
    const res = await ragApi.getConfig()
    config.value = res.data.data
    form.value = { base_url: config.value.base_url, timeout: config.value.timeout, is_active: config.value.is_active }
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
onMounted(async () => { await fetchConfig(); if (config.value.is_active) await refreshStatus() })
</script>