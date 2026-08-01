<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-memory</v-icon>记忆管理</div>
      <div class="page-subtitle">短期记忆 / 长期记忆 / 垃圾回收 / 学习引擎</div>
    </div>

    <v-card rounded="lg" elevation="1" class="mb-4 pa-4">
      <v-row dense align="center">
        <v-col cols="12" md="6">
          <v-select
            v-model="chatAreaId"
            :items="chatAreaItems"
            item-title="label" item-value="value"
            label="Chat Area" placeholder="选择 ChatArea" density="compact" hide-details clearable
          />
        </v-col>
        <v-col cols="12" md="3">
          <v-btn color="primary" variant="tonal" block @click="fetchBoth" :loading="loadingMem">查询记忆</v-btn>
        </v-col>
      </v-row>
    </v-card>

    <v-row v-if="chatAreaId">
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1" class="mb-4">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">短期记忆</span></template></v-card-item>
          <v-card-text>
            <v-skeleton-loader v-if="loadingMem" type="text@3" />
            <v-form v-else @submit.prevent="saveShort">
              <v-text-field v-model.number="shortTerm.window_size" label="窗口大小" type="number" density="compact" class="mb-3" />
              <v-switch v-model="shortTerm.auto_compact" label="自动压缩" color="primary" density="compact" class="mb-3" />
              <v-btn color="primary" variant="tonal" block type="submit" :loading="savingShort">保存</v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1" class="mb-4">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">长期记忆</span></template></v-card-item>
          <v-card-text>
            <v-skeleton-loader v-if="loadingMem" type="text@3" />
            <v-form v-else @submit.prevent="saveLong">
              <v-text-field v-model.number="longTerm.hot_area_size" label="热区大小" type="number" density="compact" class="mb-3" />
              <v-text-field v-model.number="longTerm.hot_memory_ttl" label="TTL（秒）" type="number" density="compact" class="mb-3" />
              <v-btn color="primary" variant="tonal" block type="submit" :loading="savingLong">保存</v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
    <div v-if="!chatAreaId" class="empty-state mb-4"><v-icon class="empty-icon" color="secondary">mdi-memory</v-icon><div class="empty-text">选择 Chat Area 加载记忆配置</div></div>

    <!-- GC 配置 -->
    <v-card rounded="lg" elevation="1" class="mb-4">
      <v-card-item>
        <template #title><span class="text-h6 font-weight-bold">记忆垃圾回收</span></template>
        <template #subtitle>定时清理冷记忆，保持记忆库精简</template>
      </v-card-item>
      <v-card-text>
        <v-form @submit.prevent="saveGC">
          <v-row>
            <v-col cols="6" md="3">
              <v-switch v-model="gcForm.enable" label="启用 GC" color="primary" density="compact" hide-details />
            </v-col>
            <v-col cols="6" md="3">
              <v-text-field v-model.number="gcForm.cold_threshold" label="冷记忆阈值(天)" type="number" density="compact" hide-details />
            </v-col>
            <v-col cols="6" md="3">
              <v-text-field v-model.number="gcForm.max_per_agent" label="最大记忆数" type="number" density="compact" hide-details />
            </v-col>
            <v-col cols="6" md="3">
              <v-text-field v-model.number="gcForm.interval_mins" label="间隔(分)" type="number" density="compact" hide-details />
            </v-col>
          </v-row>
          <div class="d-flex mt-3" style="gap:12px">
            <v-btn color="primary" variant="tonal" type="submit" :loading="savingGC">保存 GC 配置</v-btn>
            <v-btn color="warning" variant="outlined" @click="runGC" :loading="runningGC">
              <v-icon class="me-1">mdi-delete-sweep</v-icon>手动触发 GC
            </v-btn>
          </div>
        </v-form>
      </v-card-text>
    </v-card>

    <!-- 学习引擎配置 -->
    <v-card rounded="lg" elevation="1">
      <v-card-item>
        <template #title><span class="text-h6 font-weight-bold">启发式学习</span></template>
        <template #subtitle>行为/表达/黑话三类自主学习引擎</template>
      </v-card-item>
      <v-card-text>
        <v-form @submit.prevent="saveLearner">
          <v-row>
            <v-col cols="6" md="3">
              <v-switch v-model="learnerForm.behavior_enabled" label="行为学习" color="green" density="compact" hide-details />
            </v-col>
            <v-col cols="6" md="3">
              <v-switch v-model="learnerForm.expression_enabled" label="表达学习" color="blue" density="compact" hide-details />
            </v-col>
            <v-col cols="6" md="3">
              <v-switch v-model="learnerForm.jargon_enabled" label="黑话学习" color="purple" density="compact" hide-details />
            </v-col>
            <v-col cols="6" md="3">
              <v-text-field v-model.number="learnerForm.learn_interval" label="学习间隔(轮)" type="number" density="compact" hide-details />
            </v-col>
          </v-row>
          <v-btn color="primary" variant="tonal" type="submit" :loading="savingLearner" class="mt-3">保存学习配置</v-btn>
        </v-form>
      </v-card-text>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { memoryApi, memoryGCApi, learnerApi, chatAreaApi, type ChatAreaResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loadingMem = ref(false); const savingShort = ref(false); const savingLong = ref(false)
const savingGC = ref(false); const runningGC = ref(false); const savingLearner = ref(false)
const chatAreaId = ref('')
const chatAreaItems = ref<{label: string; value: string}[]>([])
const shortTerm = ref({ window_size: 20, auto_compact: false })
const longTerm = ref({ hot_area_size: 10, hot_memory_ttl: 86400 })
const gcForm = ref({ enable: true, cold_threshold: 7, max_per_agent: 1000, interval_mins: 60 })
const learnerForm = ref({ behavior_enabled: true, expression_enabled: true, jargon_enabled: true, learn_interval: 1, max_concurrent_learn: 2 })

async function fetchChatAreas() {
  try {
    const list = (await chatAreaApi.list()).data.data || []
    chatAreaItems.value = list.map((c: ChatAreaResp) => ({ label: `${c.area_type==='private'?'私聊':'群聊'} ${c.target_id} (${c.id.slice(0,8)})`, value: c.id }))
  } catch { toastStore.error('获取 ChatArea 列表失败') }
}
async function fetchBoth() {
  if (!chatAreaId.value) return; loadingMem.value = true
  try {
    const [st, lt] = await Promise.all([memoryApi.getShortTerm(chatAreaId.value), memoryApi.getLongTerm(chatAreaId.value)])
    shortTerm.value = { window_size: st.data.data.window_size, auto_compact: st.data.data.auto_compact }
    longTerm.value = { hot_area_size: lt.data.data.hot_area_size, hot_memory_ttl: lt.data.data.hot_memory_ttl }
  } catch (e: any) { toastStore.error('获取失败: ' + (e?.message || '')) } finally { loadingMem.value = false }
}
async function saveShort() { savingShort.value = true; try { await memoryApi.updateShortTerm(chatAreaId.value, shortTerm.value); toastStore.success('已保存') } catch { toastStore.error('保存失败') } finally { savingShort.value = false } }
async function saveLong() { savingLong.value = true; try { await memoryApi.updateLongTerm(chatAreaId.value, longTerm.value); toastStore.success('已保存') } catch { toastStore.error('保存失败') } finally { savingLong.value = false } }

async function loadGC() {
  try {
    const d = (await memoryGCApi.getConfig()).data?.data
    if (d) gcForm.value = { enable: d.enable ?? true, cold_threshold: d.cold_threshold ?? 7, max_per_agent: d.max_per_agent ?? 1000, interval_mins: d.interval_mins ?? 60 }
  } catch { /* ignore */ }
}
async function saveGC() { savingGC.value = true; try { await memoryGCApi.updateConfig(gcForm.value); toastStore.success('GC 配置已保存') } catch { toastStore.error('保存失败') } finally { savingGC.value = false } }
async function runGC() { runningGC.value = true; try { await memoryGCApi.run(); toastStore.success('GC 已触发') } catch { toastStore.error('GC 触发失败') } finally { runningGC.value = false } }

async function loadLearner() {
  try {
    const d = (await learnerApi.getConfig()).data?.data
    if (d) learnerForm.value = { behavior_enabled: d.behavior_enabled ?? true, expression_enabled: d.expression_enabled ?? true, jargon_enabled: d.jargon_enabled ?? true, learn_interval: d.learn_interval ?? 1, max_concurrent_learn: d.max_concurrent_learn ?? 2 }
  } catch { /* ignore */ }
}
async function saveLearner() { savingLearner.value = true; try { await learnerApi.updateConfig(learnerForm.value); toastStore.success('学习配置已保存') } catch { toastStore.error('保存失败') } finally { savingLearner.value = false } }

onMounted(() => { fetchChatAreas(); loadGC(); loadLearner() })
</script>
