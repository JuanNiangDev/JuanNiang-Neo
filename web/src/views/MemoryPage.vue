<template>
  <div>
    <div class="page-header"><div class="page-title">Memory 配置</div><div class="page-subtitle">管理短期/长期记忆配置（按 ChatArea）</div></div>

    <v-card rounded="lg" elevation="1" class="mb-4 pa-4">
      <v-row dense align="center">
        <v-col cols="12" md="8">
          <v-text-field v-model="chatAreaId" label="Chat Area ID" placeholder="输入 UUID" density="compact" hide-details @keydown.enter="fetchBoth" />
        </v-col>
        <v-col cols="12" md="4">
          <v-btn color="primary" variant="tonal" block @click="fetchBoth" :loading="loading">查询</v-btn>
        </v-col>
      </v-row>
    </v-card>

    <v-row v-if="chatAreaId">
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">短期记忆</span></template></v-card-item>
          <v-card-text>
            <v-skeleton-loader v-if="loading" type="text@3" />
            <v-form v-else>
              <v-text-field v-model.number="shortTerm.window_size" label="窗口大小" type="number" class="mb-3" />
              <v-switch v-model="shortTerm.auto_compact" label="自动压缩" color="primary" class="mb-3" />
              <v-btn color="primary" variant="tonal" block @click="saveShort" :loading="savingShort">保存短期记忆配置</v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">长期记忆</span></template></v-card-item>
          <v-card-text>
            <v-skeleton-loader v-if="loading" type="text@3" />
            <v-form v-else>
              <v-text-field v-model.number="longTerm.hot_area_size" label="热区大小" type="number" class="mb-3" />
              <v-text-field v-model.number="longTerm.hot_memory_ttl" label="TTL（秒）" type="number" class="mb-3" />
              <v-btn color="primary" variant="tonal" block @click="saveLong" :loading="savingLong">保存长期记忆配置</v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
    <div v-else class="empty-state"><v-icon class="empty-icon" color="secondary">mdi-memory</v-icon><div class="empty-text">请输入 Chat Area ID 加载配置</div></div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { memoryApi } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(false); const savingShort = ref(false); const savingLong = ref(false)
const chatAreaId = ref('')
const shortTerm = ref({ window_size: 20, auto_compact: false })
const longTerm = ref({ hot_area_size: 10, hot_memory_ttl: 86400 })

async function fetchBoth() { if (!chatAreaId.value) return; loading.value = true; try { const [st, lt] = await Promise.all([memoryApi.getShortTerm(chatAreaId.value), memoryApi.getLongTerm(chatAreaId.value)]); shortTerm.value = { window_size: st.data.data.window_size, auto_compact: st.data.data.auto_compact }; longTerm.value = { hot_area_size: lt.data.data.hot_area_size, hot_memory_ttl: lt.data.data.hot_memory_ttl } } catch (e: any) { toastStore.error('获取失败: ' + (e?.message || '')) } finally { loading.value = false } }
async function saveShort() { savingShort.value = true; try { await memoryApi.updateShortTerm(chatAreaId.value, shortTerm.value); toastStore.success('短期记忆配置已保存') } catch { toastStore.error('保存失败') } finally { savingShort.value = false } }
async function saveLong() { savingLong.value = true; try { await memoryApi.updateLongTerm(chatAreaId.value, longTerm.value); toastStore.success('长期记忆配置已保存') } catch { toastStore.error('保存失败') } finally { savingLong.value = false } }
</script>
