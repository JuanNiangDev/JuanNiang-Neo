<template>
  <div>
    <div class="page-header"><div class="page-title">T2I 配置</div><div class="page-subtitle">Text-to-Image 服务配置与健康状态</div></div>

    <v-row>
      <v-col cols="12" md="6">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">配置</span></template></v-card-item>
          <v-card-text>
            <v-form>
              <v-text-field v-model="form.base_url" label="服务地址" class="mb-3" />
              <v-text-field v-model.number="form.timeout" label="超时 (ms)" type="number" class="mb-3" />
              <v-switch v-model="form.is_active" label="启用" color="primary" class="mb-3" />
              <v-select
                v-model="form.selected_style"
                :items="styleOptions"
                label="渲染风格"
                hint="默认随机；选择指定风格后，文生图固定使用该风格"
                persistent-hint
                class="mb-3"
              />
              <v-btn color="primary" variant="tonal" block @click="save" :loading="saving">保存配置</v-btn>
            </v-form>
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
              <v-list-item>
                <template #prepend><span class="status-dot" :class="config.is_active ? 'active' : 'inactive'" /></template>
                <v-list-item-title>启用状态</v-list-item-title>
                <v-list-item-subtitle>{{ config.is_active ? '已启用' : '已停用' }}</v-list-item-subtitle>
              </v-list-item>
              <v-list-item>
                <template #prepend><span class="status-dot" :class="config.selected_style ? 'active' : 'inactive'" /></template>
                <v-list-item-title>当前风格</v-list-item-title>
                <v-list-item-subtitle>{{ currentStyleLabel }}</v-list-item-subtitle>
              </v-list-item>
            </v-list>
            <v-btn variant="tonal" class="mt-3" @click="checkHealth" :loading="checking" block>
              <v-icon class="me-1">mdi-heart-pulse</v-icon> 检查健康
            </v-btn>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { t2iApi, type T2IConfigResp, type UpdateT2IConfigReq, type T2IStyleItem } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const saving = ref(false); const checking = ref(false)
const config = ref<T2IConfigResp>({ base_url: '', timeout: 120000, is_active: false, healthy: false, selected_style: '' })
const form = ref<UpdateT2IConfigReq>({ base_url: '', timeout: 120000, is_active: false, selected_style: '' })
const styleList = ref<T2IStyleItem[]>([])

// 下拉选项：随机 + 全部风格
const styleOptions = computed(() => {
  const opts: { title: string; value: string }[] = [{ title: '随机（每次随机一种）', value: '' }]
  for (const s of styleList.value) {
    const tags = s.tags?.length ? ` · ${s.tags.join(' / ')}` : ''
    opts.push({ title: `${s.name}（${s.category}${tags}）`, value: s.name })
  }
  return opts
})

const currentStyleLabel = computed(() => {
  if (!config.value.selected_style) return '随机'
  const hit = styleList.value.find(s => s.name === config.value.selected_style)
  return hit ? `${hit.name}（${hit.category}）` : config.value.selected_style
})

async function fetchConfig() {
  try {
    const res = await t2iApi.getConfig()
    config.value = res.data.data
    form.value = {
      base_url: config.value.base_url,
      timeout: config.value.timeout,
      is_active: config.value.is_active,
      selected_style: config.value.selected_style ?? '',
    }
  } catch { toastStore.error('获取配置失败') }
}
async function fetchStyles() {
  try {
    const res = await t2iApi.listStyles()
    styleList.value = res.data.data ?? []
  } catch { toastStore.error('获取风格列表失败') }
}
async function save() {
  saving.value = true
  try {
    const res = await t2iApi.updateConfig(form.value)
    config.value = res.data.data
    toastStore.success('已保存')
  } catch { toastStore.error('保存失败') } finally { saving.value = false }
}
async function checkHealth() {
  checking.value = true
  try {
    const res = await t2iApi.health()
    config.value.healthy = res.data.data.healthy
    toastStore.info(config.value.healthy ? '健康检查通过' : '健康检查失败')
  } catch { toastStore.error('健康检查失败') } finally { checking.value = false }
}
onMounted(async () => { await fetchStyles(); await fetchConfig() })
</script>
