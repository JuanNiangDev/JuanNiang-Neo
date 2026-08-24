<template>
  <div>
    <div class="page-header"><div class="page-title">Memory 配置</div><div class="page-subtitle">管理短期/长期记忆配置（按 ChatArea）</div></div>

    <v-card rounded="lg" elevation="1" class="mb-4 pa-4">
      <v-row dense align="center">
          <v-col cols="12" md="8">
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
          <v-col cols="12" md="4">
            <v-btn color="primary" variant="tonal" block @click="fetchBoth" :loading="loading">查询</v-btn>
          </v-col>
        </v-row>

        <v-row dense align="center" class="mt-1">
          <v-col cols="12">
            <v-alert type="info" variant="tonal" density="compact">
              长期记忆向量同步：将 Postgres 内全部长期记忆批量写入 RAG-Service 向量库（幂等），补齐 Compact 双写之前的历史记忆。
            </v-alert>
            <v-btn color="primary" variant="tonal" prepend-icon="mdi-cloud-sync-outline" :loading="syncingRAG" class="mt-2" @click="syncRAG">同步向量库</v-btn>
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
import { ref, onMounted } from 'vue'
import { memoryApi, chatAreaApi, type ChatAreaResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(false); const savingShort = ref(false); const savingLong = ref(false)
const chatAreaId = ref('')
const chatAreaItems = ref<{label: string; value: string}[]>([])
const shortTerm = ref({ window_size: 20, auto_compact: false })
const longTerm = ref({ hot_area_size: 10, hot_memory_ttl: 86400 })

async function fetchChatAreas() { try { const list = (await chatAreaApi.list()).data.data || []; chatAreaItems.value = list.map((c: ChatAreaResp) => ({ label: `${c.area_type==='private'?'私聊':'群聊'} ${c.target_id} (${c.id.slice(0,8)})`, value: c.id })) } catch { toastStore.error('获取 ChatArea 列表失败') } }
async function fetchBoth() { if (!chatAreaId.value) return; loading.value = true; try { const [st, lt] = await Promise.all([memoryApi.getShortTerm(chatAreaId.value), memoryApi.getLongTerm(chatAreaId.value)]); shortTerm.value = { window_size: st.data.data.window_size, auto_compact: st.data.data.auto_compact }; longTerm.value = { hot_area_size: lt.data.data.hot_area_size, hot_memory_ttl: lt.data.data.hot_memory_ttl } } catch (e: any) { toastStore.error('获取失败: ' + (e?.message || '')) } finally { loading.value = false } }
async function saveShort() { savingShort.value = true; try { await memoryApi.updateShortTerm(chatAreaId.value, shortTerm.value); toastStore.success('短期记忆配置已保存') } catch { toastStore.error('保存失败') } finally { savingShort.value = false } }
async function saveLong() { savingLong.value = true; try { await memoryApi.updateLongTerm(chatAreaId.value, longTerm.value); toastStore.success('长期记忆配置已保存') } catch { toastStore.error('保存失败') } finally { savingLong.value = false } }
const syncingRAG = ref(false)
async function syncRAG() { syncingRAG.value = true; try { const res = (await memoryApi.syncRAG()).data.data; if (res?.ready) { toastStore.success(`记忆向量同步完成：成功 ${res.synced} 条，失败 ${res.failed} 条（共 ${res.total} 条）`) } else { toastStore.error(res?.message || 'RAG 未启用，无法同步') } } catch (e: any) { toastStore.error('同步失败: ' + (e?.message || '')) } finally { syncingRAG.value = false } }
onMounted(fetchChatAreas)
</script>
