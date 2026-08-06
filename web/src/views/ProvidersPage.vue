<template>
  <div>
    <div class="page-header">
      <div class="page-title">Provider 管理</div>
      <div class="page-subtitle">管理 LLM 提供商配置（同类型仅一个 Active）</div>
    </div>
    <div class="d-flex justify-end mb-4">
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增 Provider</v-btn>
    </div>
    <v-data-table :headers="headers" :items="items" :loading="loading" items-per-page="20">
      <template #item.type="{ item }"><v-chip size="small" variant="tonal">{{ typeLabel(item.type) }}</v-chip></template>
      <template #item.api_mode="{ item }">
        <v-chip size="small" variant="tonal">{{ apiModeLabel(item.api_mode) }}</v-chip>
      </template>
      <template #item.enable_thinking="{ item }">
        <v-chip size="small" :color="thinkingOn(item) ? 'primary' : 'default'" variant="tonal">{{ thinkingOn(item) ? '开' : '关' }}</v-chip>
      </template>
      <template #item.is_active="{ item }">
        <v-switch :model-value="item.is_active" color="primary" density="compact" hide-details @update:model-value="(v) => toggle(item.id, !!v)" />
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-sync" size="small" variant="text" color="secondary" title="测试连接" :loading="testingId === item.id" @click="testItem(item)" />
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" @click="openEdit(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <!-- Dialog -->
    <v-dialog v-model="dialog" max-width="960">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center">
          <span class="me-auto">{{ editing ? '编辑 Provider' : '新增 Provider' }}</span>
          <v-btn v-if="editing" variant="tonal" color="secondary" prepend-icon="mdi-sync" :loading="testing" @click="testForm">测试连接</v-btn>
        </v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-row>
              <v-col cols="12" sm="6">
                <v-text-field v-model="form.name" label="名称" @update:model-value="onNameChange" />
              </v-col>
              <v-col cols="12" sm="6">
                <v-select v-model="form.type" :items="types" label="类型" />
              </v-col>
            </v-row>

            <v-row>
              <v-col cols="12" sm="6">
                <v-select v-model="form.api_mode" :items="apiModes" label="协议模式" />
              </v-col>
              <v-col cols="12" sm="6">
                <v-select v-model="form.url_mode" :items="urlModes" label="URL 模式" />
              </v-col>
            </v-row>

            <v-row>
              <v-col cols="12" sm="6">
                <v-select v-model="selectedPresetKey" :items="presetOptions" label="厂商预设（可选）" clearable @update:model-value="onPresetSelect" />
              </v-col>
              <v-col cols="12" sm="6">
                <v-select v-if="selectedPreset" v-model="selectedProtocolIdx" :items="protocolOptions" label="协议" @update:model-value="onProtocolSelect" />
              </v-col>
            </v-row>
            <div v-if="selectedPresetNote" class="text-caption text-warning mb-3">{{ selectedPresetNote }}</div>

            <v-row>
              <v-col cols="12" sm="6">
                <v-text-field v-model="form.endpoint" label="API 地址" hint="可填 base URL 或完整端点（以协议后缀结尾自动识别）" persistent-hint />
              </v-col>
              <v-col cols="12" sm="6">
                <v-text-field v-model="form.token" label="API Token" type="password" />
              </v-col>
            </v-row>

            <v-row>
              <v-col cols="12" sm="6">
                <v-text-field v-model="form.model" label="模型名" />
              </v-col>
              <v-col cols="12" sm="6">
                <v-select v-model="form.auth_header" :items="authHeaders" label="认证头" clearable hint="空 = 按协议默认" persistent-hint />
              </v-col>
            </v-row>

            <v-divider class="my-2" />
            <div class="text-subtitle-2 mb-2">基础参数</div>
            <v-row>
              <v-col cols="12" sm="6" md="4">
                <v-text-field v-model="form.temperature" label="温度" type="number" step="0.1" />
              </v-col>
              <v-col cols="12" sm="6" md="4">
                <v-text-field v-model="form.max_tokens" label="Max Tokens（0=默认）" type="number" />
              </v-col>
              <v-col cols="12" sm="6" md="4">
                <v-switch v-model="form.isActive" label="激活" color="primary" />
              </v-col>
            </v-row>

            <template v-if="show.thinking">
              <v-divider class="my-2" />
              <div class="text-subtitle-2 mb-2">思考（Thinking）</div>
              <v-row>
                <v-col cols="12" sm="6" md="4">
                  <v-select v-model="form.thinking_effort" :items="thinkingEfforts" label="思考档位" />
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-text-field v-model="form.thinking_budget" label="Thinking Budget（0=默认）" type="number" />
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-switch v-model="form.enable_thinking" label="模型思考（旧开关）" color="primary" />
                </v-col>
              </v-row>
            </template>

            <v-divider class="my-2" />
            <div class="text-subtitle-2 mb-2">高级采样参数（可选）</div>
            <v-row>
              <v-col v-if="show.topP" cols="12" sm="6" md="4">
                <v-text-field v-model="form.top_p" label="Top P" type="number" step="0.05" />
              </v-col>
              <v-col v-if="show.topK" cols="12" sm="6" md="4">
                <v-text-field v-model="form.top_k" label="Top K" type="number" />
              </v-col>
              <v-col v-if="show.freqPresence" cols="12" sm="6" md="4">
                <v-text-field v-model="form.frequency_penalty" label="Frequency Penalty" type="number" step="0.1" />
              </v-col>
              <v-col v-if="show.freqPresence" cols="12" sm="6" md="4">
                <v-text-field v-model="form.presence_penalty" label="Presence Penalty" type="number" step="0.1" />
              </v-col>
              <v-col v-if="show.repetition" cols="12" sm="6" md="4">
                <v-text-field v-model="form.repetition_penalty" label="Repetition Penalty" type="number" step="0.1" />
              </v-col>
            </v-row>

            <div class="text-caption text-medium-emphasis mt-2">
              思考档位支持 off/low/medium/high，按厂商矩阵适配（DeepSeek/智谱/Kimi/通义/阶跃/MiniMax 等）。
            </div>
          </v-form>

          <!-- 测试结果状态 -->
          <v-alert v-if="testResult !== null" :type="testResult.ok ? 'success' : 'error'" variant="tonal" class="mt-3" dense>
            <template #prepend><v-icon>{{ testResult.ok ? 'mdi-check-circle' : 'mdi-alert-circle' }}</v-icon></template>
            <div class="text-caption">{{ testResult.message }}</div>
          </v-alert>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn v-if="!editing" variant="tonal" color="secondary" prepend-icon="mdi-sync" :loading="testing" @click="testForm">测试连接</v-btn>
          <v-btn variant="text" @click="dialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Delete confirm -->
    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>确认删除</v-card-title>
        <v-card-text>确定要删除此 Provider 吗？此操作不可撤销。</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="deleteDialog = false">取消</v-btn>
          <v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { providerApi, providerPresetsApi, type ProviderResp, type ProviderPreset, type AddProviderReq, type TestProviderResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true)
const items = ref<ProviderResp[]>([])
const dialog = ref(false)
const deleteDialog = ref(false)
const editing = ref<string | null>(null)
const saving = ref(false)
const deleting = ref(false)
const testing = ref(false)
const testingId = ref<string | null>(null)
const testResult = ref<TestProviderResp | null>(null)
const deleteTarget = ref<ProviderResp | null>(null)
const formRef = ref()

const presets = ref<ProviderPreset[]>([])
const selectedPresetKey = ref<string | null>(null)
const selectedProtocolIdx = ref<number | null>(null)

const headers = [
  { title: '名称', key: 'name' },
  { title: '类型', key: 'type' },
  { title: '模型', key: 'model' },
  { title: '协议', key: 'api_mode', align: 'center' as const },
  { title: '端点', key: 'endpoint' },
  { title: '思考', key: 'enable_thinking', align: 'center' as const },
  { title: 'Active', key: 'is_active', align: 'center' as const },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const types = [
  { title: 'Text Model', value: 'text_model' },
  { title: 'Image Model', value: 'image_model' },
  { title: 'Embedding Model', value: 'embedding_model' },
]

const apiModes = [
  { title: 'OpenAI 兼容 (chat_completions)', value: 'chat_completions' },
  { title: 'Anthropic Messages', value: 'anthropic_messages' },
  { title: 'OpenAI Responses', value: 'openai_responses' },
  { title: 'Gemini Native', value: 'gemini_native' },
]

const authHeaders = [
  { title: 'Bearer', value: 'bearer' },
  { title: 'x-api-key', value: 'x-api-key' },
  { title: 'api-key', value: 'api-key' },
]

const urlModes = [
  { title: '自动（base + 协议后缀）', value: 'auto' },
  { title: '完全自定义（原样使用）', value: 'exact' },
]

const thinkingEfforts = [
  { title: '关闭', value: 'off' },
  { title: '低', value: 'low' },
  { title: '中', value: 'medium' },
  { title: '高', value: 'high' },
]

const typeLabel = (t: string) => ({ text_model: 'Text', image_model: 'Image', embedding_model: 'Embedding' }[t] || t)
const apiModeLabel = (m: string) => ({ chat_completions: 'OpenAI', anthropic_messages: 'Anthropic', openai_responses: 'Responses', gemini_native: 'Gemini' }[m] || m || 'OpenAI')
const thinkingOn = (item: ProviderResp) => item.thinking_effort === 'off' ? false : (item.enable_thinking || item.thinking_effort !== '')

const presetOptions = computed(() => presets.value.map(p => ({ title: p.name, value: p.key })))
const selectedPreset = computed(() => presets.value.find(p => p.key === selectedPresetKey.value) || null)
const protocolOptions = computed(() => selectedPreset.value ? selectedPreset.value.protocols.map((p, i) => ({ title: apiModeLabel(p.api_mode), value: i })) : [])
const selectedPresetNote = computed(() => {
  const p = selectedPreset.value
  if (!p || selectedProtocolIdx.value == null) return ''
  return p.protocols[selectedProtocolIdx.value]?.note || ''
})

// 按协议模式 + 厂商参数化各配置项是否适用（选择提供商时隐藏不适用的配置）。
const show = computed(() => {
  const mode = form.value.api_mode
  const pk = form.value.provider_key
  const anthro = mode === 'anthropic_messages'
  const gemini = mode === 'gemini_native'
  const resp = mode === 'openai_responses'
  const chat = mode === 'chat_completions'
  return {
    thinking: anthro || gemini || resp || chat,
    topP: !resp,
    topK: anthro || gemini || pk === 'minimax',
    freqPresence: chat || resp,
    repetition: gemini || pk === 'minimax' || pk === 'xiaomi',
  }
})

const defaultForm = (): AddProviderReq => ({
  name: '', type: 'text_model', endpoint: '', token: '', model: '', temperature: 0.7,
  isActive: false, enable_thinking: false, api_mode: 'chat_completions', thinking_effort: 'off',
  thinking_budget: 0, max_tokens: 0, top_p: null, top_k: null, frequency_penalty: null,
  presence_penalty: null, repetition_penalty: null, provider_key: '', auth_header: '', url_mode: 'auto',
})
const form = ref<AddProviderReq>(defaultForm())

async function fetch() {
  loading.value = true
  try { items.value = (await providerApi.list()).data.data } catch (e: any) { toastStore.error('获取列表失败') } finally { loading.value = false }
}

async function fetchPresets() {
  try { presets.value = (await providerPresetsApi.list()).data.data } catch (e: any) { /* 预设可选，失败静默 */ }
}

function openAdd() {
  editing.value = null
  form.value = defaultForm()
  selectedPresetKey.value = null
  selectedProtocolIdx.value = null
  testResult.value = null
  dialog.value = true
}
function openEdit(item: ProviderResp) {
  editing.value = item.id
  form.value = {
    name: item.name, type: item.type, endpoint: item.endpoint, token: item.token, model: item.model,
    temperature: item.temperature, isActive: item.is_active, enable_thinking: item.enable_thinking,
    api_mode: item.api_mode || 'chat_completions', thinking_effort: item.thinking_effort || 'off',
    thinking_budget: item.thinking_budget || 0, max_tokens: item.max_tokens || 0,
    top_p: item.top_p, top_k: item.top_k, frequency_penalty: item.frequency_penalty,
    presence_penalty: item.presence_penalty, repetition_penalty: item.repetition_penalty,
    provider_key: item.provider_key || '', auth_header: item.auth_header || '', url_mode: item.url_mode || 'auto',
  }
  selectedPresetKey.value = item.provider_key || null
  selectedProtocolIdx.value = null
  testResult.value = null
  dialog.value = true
}

// 名称关键词命中预设厂商时自动带出 provider_key，便于 thinking 矩阵生效。
function onNameChange(v: string | null) {
  if (!v || editing.value) return
  const n = v.toLowerCase()
  const match = presets.value.find(p => n.includes(p.key) || n.includes(p.name.toLowerCase()))
  if (match) form.value.provider_key = match.key
}

function onPresetSelect(key: string | null) {
  selectedProtocolIdx.value = null
  if (!key) return
  form.value.provider_key = key
  const p = presets.value.find(x => x.key === key)
  if (p && p.protocols.length) selectedProtocolIdx.value = 0
}

function onProtocolSelect(idx: number | null) {
  const p = selectedPreset.value
  if (!p || idx == null) return
  const proto = p.protocols[idx]
  form.value.api_mode = proto.api_mode
  form.value.endpoint = proto.base_url
  form.value.auth_header = proto.auth_header
  form.value.url_mode = 'auto'
}

async function toggle(id: string, v: boolean) {
  try {
    await providerApi.toggle(id, v)
    toastStore.success(v ? '已启用' : '已停用')
    await fetch()
  } catch (e: any) { toastStore.error('操作失败') }
}

async function handleSave() {
  saving.value = true
  try {
    if (editing.value) { await providerApi.update(editing.value, form.value) } else { await providerApi.create(form.value) }
    toastStore.success(editing.value ? '已更新' : '已创建')
    dialog.value = false
    await fetch()
  } catch (e: any) { toastStore.error(e?.message || '保存失败') } finally { saving.value = false }
}

function reqFromItem(item: ProviderResp): AddProviderReq {
  return {
    name: item.name, type: item.type, endpoint: item.endpoint, token: item.token, model: item.model,
    temperature: item.temperature, isActive: item.is_active, enable_thinking: item.enable_thinking,
    api_mode: item.api_mode || 'chat_completions', thinking_effort: item.thinking_effort || 'off',
    thinking_budget: item.thinking_budget || 0, max_tokens: item.max_tokens || 0,
    top_p: item.top_p, top_k: item.top_k, frequency_penalty: item.frequency_penalty,
    presence_penalty: item.presence_penalty, repetition_penalty: item.repetition_penalty,
    provider_key: item.provider_key || '', auth_header: item.auth_header || '', url_mode: item.url_mode || 'auto',
  }
}

async function runTest(payload: AddProviderReq) {
  testing.value = true
  try {
    const res = (await providerApi.test(payload)).data.data as TestProviderResp
    testResult.value = res
    if (res.ok) {
      toastStore.success('连接成功')
    } else {
      toastStore.error(res.message || '连接失败')
    }
  } catch (e: any) {
    testResult.value = { ok: false, message: e?.message || '测试失败' }
    toastStore.error('测试失败')
  } finally { testing.value = false }
}

// 测试新增/编辑表单中的当前配置（不落库）。
function testForm() { runTest({ ...form.value }) }

// 测试列表中已保存的 Provider。
async function testItem(item: ProviderResp) {
  testingId.value = item.id
  try {
    const res = (await providerApi.test(reqFromItem(item))).data.data as TestProviderResp
    if (res.ok) {
      toastStore.success('连接成功')
    } else {
      toastStore.error(res.message || '连接失败')
    }
  } catch (e: any) {
    toastStore.error(e?.message || '测试失败')
  } finally { testingId.value = null }
}

function confirmDelete(item: ProviderResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try { await providerApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch (e: any) { toastStore.error('删除失败') } finally { deleting.value = false }
}

onMounted(() => { fetch(); fetchPresets() })
</script>