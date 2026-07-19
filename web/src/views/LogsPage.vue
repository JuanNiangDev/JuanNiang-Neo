<template>
  <div>
    <div class="page-header">
      <div class="page-title">系统日志</div>
      <div class="page-subtitle">最近 250 条日志记录</div>
    </div>

    <div class="d-flex align-center mb-4" style="gap:12px">
      <v-btn color="primary" variant="tonal" prepend-icon="mdi-refresh" @click="fetch" :loading="loading">刷新</v-btn>
      <v-select v-model="levelFilter" :items="['','INFO','WARN','ERROR','DEBUG']" label="级别过滤" density="compact" hide-details style="max-width:150px" @update:model-value="applyFilter" />
      <v-spacer />
      <v-chip size="small" variant="tonal" color="grey">{{ filtered.length }} 条</v-chip>
    </div>

    <v-card rounded="lg" elevation="1">
      <v-card-text style="max-height: calc(100vh - 280px); overflow-y: auto">
        <div v-if="loading" class="d-flex justify-center py-8">
          <v-progress-circular indeterminate color="primary" />
        </div>
        <template v-else>
          <div v-for="(log, i) in filtered" :key="i" class="log-entry d-flex align-start py-2" style="gap:12px;border-bottom:1px solid rgba(0,0,0,0.04)">
            <div style="min-width:50px">
              <v-chip size="x-small" variant="tonal" :color="levelColor(log.level)">{{ log.level }}</v-chip>
            </div>
            <div style="min-width:160px" class="text-caption text-medium-emphasis">
              {{ formatTime(log.time) }}
            </div>
            <div class="text-body-2 flex-grow-1">{{ log.message }}</div>
            <div v-if="log.attrs && Object.keys(log.attrs).length > 0" class="text-caption text-medium-emphasis" style="max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">
              {{ JSON.stringify(log.attrs) }}
            </div>
          </div>
          <div v-if="filtered.length === 0" class="empty-state"><div class="empty-text">暂无日志</div></div>
        </template>
      </v-card-text>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { logApi, type LogEntryResp } from '@/api'
import { useToastStore } from '@/stores/toast'
import { format } from 'date-fns'

const toastStore = useToastStore()
const loading = ref(true); const logs = ref<LogEntryResp[]>([]); const levelFilter = ref('')

const filtered = computed(() => levelFilter.value ? logs.value.filter(l => l.level === levelFilter.value) : logs.value)
const levelColor = (lvl: string): string => ({ INFO:'info', WARN:'warning', ERROR:'error', DEBUG:'grey' })[lvl] || 'grey'

function formatTime(t: string) { try { return format(new Date(t), 'MM-dd HH:mm:ss') } catch { return t } }
function applyFilter() {}

async function fetch() { loading.value = true; try { logs.value = (await logApi.list()).data.data } catch { toastStore.error('获取日志失败') } finally { loading.value = false } }
onMounted(fetch)
</script>
