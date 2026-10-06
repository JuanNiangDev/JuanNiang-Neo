<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-reply-circle</v-icon>系统回复设置</div>
      <div class="page-subtitle">控制机器人的回复行为与消息格式</div>
    </div>

    <v-row>
      <!-- 回复策略 -->
      <v-col cols="12" md="7">
        <v-card rounded="lg" elevation="1" class="h-100">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">回复策略</span></template>
            <template #subtitle>群聊/私聊消息路由方式</template>
          </v-card-item>
          <v-card-text>
            <v-form @submit.prevent="handleSave">
              <!-- 回复策略已收敛为仅支持按相关性回复：@/命令/提及名字必回，其余由 LLM 判断 -->
              <v-alert type="info" variant="tonal" class="mb-4" density="comfortable">
                <div class="text-subtitle-2 font-weight-bold">按相关性回复（当前唯一策略）</div>
                <div class="text-caption">
                  被@、触发插件命令或提及机器人名字时必回（规则快路径，不消耗 LLM 判断）；
                  其余群聊消息由 LLM 按相关性分数决定是否回复，判断结果带 Redis 缓存与冷却。
                </div>
              </v-alert>

              <div class="text-subtitle-2 font-weight-bold mb-2">相关性阈值</div>
              <div class="d-flex align-center" style="gap: 16px">
                <v-slider
                  v-model="form.relevance_threshold"
                  :min="0" :max="1" :step="0.05"
                  color="primary" thumb-label="always" hide-details style="flex: 1"
                />
                <v-text-field
                  v-model.number="form.relevance_threshold"
                  type="number" :min="0" :max="1" :step="0.05"
                  density="compact" style="width: 90px" hide-details variant="outlined"
                />
              </div>
              <div class="text-caption text-medium-emphasis mt-2">
                Agent 判断消息相关性 &ge; 阈值时才会回复。被@时自动绕过此判断。
                <br />当前阈值: {{ form.relevance_threshold.toFixed(2) }}
              </div>

              <v-divider class="my-3" />

              <div class="text-subtitle-2 font-weight-bold mb-2">机器人名字</div>
              <v-text-field
                v-model="form.bot_name"
                label="用于相关性判断，如「小卷」"
                placeholder="例如：小卷"
                density="comfortable" variant="outlined" hide-details clearable
              />

              <v-divider class="my-3" />

              <div class="text-subtitle-2 font-weight-bold mb-2">相关性检测模型</div>
              <v-select
                v-model="form.relevance_model"
                :items="textModelOptions"
                item-title="label"
                item-value="value"
                label="选择相关性判断使用的 Text 模型"
                placeholder="（默认 Text 模型）"
                density="comfortable" variant="outlined" clearable hide-details
              />

              <v-divider class="my-3" />

              <div class="text-subtitle-2 font-weight-bold mb-2">相关性判断超时（秒）</div>
              <v-text-field
                v-model.number="form.relevance_timeout"
                type="number" :min="1" :max="120"
                label="相关性判断 LLM 调用总超时（含等待，默认 10s）"
                density="comfortable" variant="outlined" hide-details
              />
              <div class="text-caption text-medium-emphasis mt-1">
                慢速提供商可调大到 15-30s；信号量等待与 LLM 调用共享该预算。
              </div>

              <v-divider class="my-3" />

              <div class="text-subtitle-2 font-weight-bold mb-2">判断失败策略</div>
              <v-select
                v-model="form.judge_fail_policy"
                :items="[
                  { label: '不回复 — 判断失败时保持沉默（默认）', value: 'drop' },
                  { label: '照常回复 — 判断失败时交给 Agent 回复', value: 'reply' },
                ]"
                item-title="label"
                item-value="value"
                label="相关性判断 LLM 调用失败时的处理"
                density="comfortable" variant="outlined" hide-details
              />
              <div class="text-caption text-medium-emphasis mt-1">
                LLM 接口超时/限流等瞬态故障时生效；未配置模型不算失败。
              </div>

              <div class="d-flex align-center" style="gap: 12px">
                <v-btn type="submit" color="primary" variant="tonal" :loading="saving" :disabled="capabilityLoading">
                  <v-icon class="me-1">mdi-content-save</v-icon> 保存
                </v-btn>
                <v-btn variant="tonal" color="info" @click="openPromptDialog">
                  <v-icon class="me-1">mdi-text-box-edit-outline</v-icon> 自定义判断提示词
                </v-btn>
              </div>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>

      <!-- 其他设置 -->
      <v-col cols="12" md="5">
        <v-card rounded="lg" elevation="1" class="h-100">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">其他设置</span></template>
            <template #subtitle>消息格式与 Agent 行为</template>
          </v-card-item>
          <v-card-text>
            <v-form @submit.prevent="handleSave">
              <!-- AgentLite -->
              <div class="d-flex align-start mb-4">
                <div class="flex-grow-1 me-3">
                  <div class="text-subtitle-2 font-weight-bold">AgentLite 模式</div>
                  <div class="text-caption text-medium-emphasis mt-1">
                    与正常模式一致，保留 ReAct 循环；仅禁用 MCP、沙箱和文生图工具。<br />
                    记忆、提示词、Skill 行为不受影响。
                  </div>
                </div>
                <v-switch v-model="form.agent_lite" color="primary" hide-details density="compact" />
              </div>

              <v-divider class="mb-4" />

              <!-- Strip Markdown -->
              <div class="d-flex align-start mb-4">
                <div class="flex-grow-1 me-3">
                  <div class="text-subtitle-2 font-weight-bold">去除 Markdown 格式</div>
                  <div class="text-caption text-medium-emphasis mt-1">
                    Agent 发送消息前去除加粗、斜体、代码块、链接等格式，发送纯文本到 QQ。
                  </div>
                </div>
                <v-switch v-model="form.strip_markdown" color="primary" hide-details density="compact" />
              </div>

              <v-btn type="submit" color="primary" variant="tonal" :loading="saving" :disabled="capabilityLoading">
                <v-icon class="me-1">mdi-content-save</v-icon> 保存
              </v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- Laya 表情决策 -->
    <v-row class="mt-1">
      <v-col cols="12">
        <v-card rounded="lg" elevation="1">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">Laya 表情决策</span></template>
            <template #subtitle>可选能力，关闭时保持原版表情发送流程</template>
          </v-card-item>
          <v-card-text>
            <v-form @submit.prevent="handleSave">
              <div class="d-flex align-start mb-4">
                <div class="flex-grow-1 me-3">
                  <div class="text-subtitle-2 font-weight-bold">启用自动表情</div>
                  <div class="text-caption text-medium-emphasis mt-1">Laya 请求失败或返回无效结果时自动回退原版文字回复。</div>
                </div>
                <v-switch v-model="form.laya_sticker_enabled" color="primary" hide-details density="compact" />
              </div>

              <v-row>
                <v-col cols="12" md="6">
                  <v-text-field v-model="form.laya_sticker_endpoint" label="决策 Endpoint" placeholder="https://..." density="comfortable" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" md="6">
                  <v-text-field v-model="form.laya_capabilities_endpoint" label="Capabilities Endpoint（可选）" placeholder="留空则按 Endpoint 推导 /capabilities" density="comfortable" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" md="4">
                  <v-text-field v-model="form.laya_sticker_model" label="模型 ID" density="comfortable" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" md="4">
                  <v-text-field v-model.number="form.laya_sticker_timeout" type="number" min="1" max="120" label="请求超时（秒）" density="comfortable" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" md="4">
                  <v-text-field v-model.number="form.laya_sticker_min_confidence" type="number" min="0" max="1" step="0.05" label="最低置信度（可选）" hint="仅服务端返回 confidence 时生效" persistent-hint density="comfortable" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" md="4">
                  <v-text-field v-model.number="form.laya_sticker_task_ttl_seconds" type="number" min="1" max="300" label="自动表情有效期（秒）" hint="过期表情自动丢弃" persistent-hint density="comfortable" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" md="4">
                  <v-text-field v-model="form.laya_sticker_api_key" type="password" autocomplete="new-password" label="API Key" :placeholder="form.laya_sticker_api_key_set ? '已设置，留空保持不变' : '未设置'" density="comfortable" variant="outlined" hide-details="auto" />
                  <v-checkbox v-model="form.laya_sticker_clear_api_key" label="明确清除已保存的 API Key" density="compact" hide-details />
                </v-col>
                <v-col cols="12" md="3">
                  <v-select v-model="form.laya_sticker_protocol_mode" :items="[{ title: 'systemone.v1（Laya 原生协议）', value: 'systemone.v1' }, { title: 'JSON（可配置）', value: 'json' }]" label="协议模式" density="comfortable" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" md="3">
                  <v-select v-model="form.laya_sticker_http_method" :items="['POST']" label="HTTP 方法" density="comfortable" variant="outlined" hide-details="auto" />
                </v-col>
                <template v-if="form.laya_sticker_protocol_mode === 'json'">
                  <v-col cols="12" md="3">
                    <v-text-field v-model="form.laya_sticker_response_category_path" label="类别 JSON Pointer" placeholder="/answers/sticker/choice" density="comfortable" variant="outlined" hide-details="auto" />
                  </v-col>
                  <v-col cols="12" md="3">
                    <v-text-field v-model="form.laya_sticker_response_confidence_path" label="置信度 JSON Pointer" placeholder="/answers/sticker/answer_confidence" density="comfortable" variant="outlined" hide-details="auto" />
                  </v-col>
                </template>
              </v-row>

              <v-textarea v-if="form.laya_sticker_protocol_mode === 'json'" v-model="form.laya_sticker_request_template" label="JSON 请求模板" placeholder='例如：{"model":"{{model}}","state":{"user_message":"{{user_message}}","assistant_reply":"{{assistant_reply}}"},"questions":{"sticker":{"type":"choice","instructions":"判断是否发送表情包并选择类别","criteria":{{criteria_map}}}}}' hint="criteria_map 会生成启用类别的 ID → 描述映射；criteria 保留为 ID 数组。" persistent-hint rows="5" auto-grow density="comfortable" variant="outlined" hide-details="auto" class="mt-2" />
              <v-alert v-else type="info" variant="tonal" density="comfortable" class="mt-2">
                原生协议模式：请求由客户端按 systemone.v1 结构自动生成（model + state + questions.sticker.criteria），无需填写 JSON 请求模板。只需配置 Endpoint、模型、API Key、超时、动态类别与表情标签映射。
              </v-alert>

              <div class="d-flex align-center justify-space-between flex-wrap ga-3 mt-4 mb-2">
                <div>
                  <div class="text-subtitle-2 font-weight-bold">类别映射</div>
                  <div class="text-caption text-medium-emphasis">类别 ID 由 Laya 返回，表情标签复用原版 Sticker 库；启用的 no_send 类别表示不发送。</div>
                </div>
                <v-btn size="small" variant="tonal" color="primary" @click="addCategory">
                  <v-icon start>mdi-plus</v-icon>添加类别
                </v-btn>
              </div>
              <v-alert v-if="form.laya_sticker_categories.length === 0" type="info" variant="tonal" density="comfortable" class="mb-3">启用 Laya 前至少配置一个启用的 no_send 类别。</v-alert>
              <v-row v-for="(category, index) in form.laya_sticker_categories" :key="category.rowKey" class="align-center mb-1">
                <v-col cols="12" sm="4" lg="2">
                  <v-text-field v-model="category.id" label="ID" density="compact" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" sm="8" lg="3">
                  <v-text-field v-model="category.description" label="说明" density="compact" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" md="8" lg="4">
                  <v-combobox v-model="category.sticker_tags" :items="stickerTagOptions" label="Sticker 标签" multiple chips closable-chips density="compact" variant="outlined" hide-details="auto" />
                </v-col>
                <v-col cols="12" md="4" lg="3">
                  <div class="category-actions">
                    <v-switch v-model="category.enabled" label="启用" color="primary" density="compact" hide-details />
                    <v-switch v-model="category.no_send" label="不发送" color="warning" density="compact" hide-details />
                    <v-btn icon="mdi-delete-outline" size="small" variant="text" color="error" :aria-label="`删除类别 ${category.id || index + 1}`" @click="removeCategory(index)" />
                  </div>
                </v-col>
              </v-row>

              <div class="d-flex align-center flex-wrap mt-4" style="gap: 12px">
                <v-btn type="submit" color="primary" variant="tonal" :loading="saving" :disabled="capabilityLoading">
                  <v-icon class="me-1">mdi-content-save</v-icon>保存回复设置
                </v-btn>
                <v-btn variant="tonal" color="info" :loading="capabilityLoading" :disabled="saving" @click="refreshCapabilities()">
                  <v-icon class="me-1">mdi-connection</v-icon>检测连接并获取能力
                </v-btn>
                <span v-if="form.laya_sticker_api_key_set" class="text-caption text-medium-emphasis">API Key 已设置</span>
              </div>
            </v-form>

            <v-alert v-if="capabilityPreview" type="info" variant="tonal" density="comfortable" class="mt-4">当前为表单检测的临时预览，未写入正式能力快照。保存回复设置后会按已保存配置重新获取能力。</v-alert>
            <v-alert v-if="form.laya_capability_error" type="warning" variant="tonal" density="comfortable" class="mt-4">最近一次能力刷新失败：{{ form.laya_capability_error }}</v-alert>
            <div v-if="form.laya_capability_fetched_at" class="text-caption text-medium-emphasis mt-2">{{ capabilityPreview ? '预览获取时间' : '最近成功获取' }}：{{ formatTime(form.laya_capability_fetched_at) }}</div>
            <v-card v-if="layaCapability" variant="outlined" class="mt-3">
              <v-card-text>
                <div class="d-flex align-center flex-wrap" style="gap: 8px">
                  <span class="text-subtitle-2 font-weight-bold">{{ capabilityPreview ? '服务能力预览' : '服务能力快照' }}</span>
                  <v-chip size="small" :color="capabilityProtocolColor(layaCapability.api?.protocol)" variant="tonal">协议：{{ layaCapability.api?.protocol || '未知' }}（{{ capabilityProtocolLabel(layaCapability.api?.protocol) }}）</v-chip>
                  <v-chip size="small" variant="tonal">决策路径：{{ layaCapability.api?.decision_path || '未知' }}</v-chip>
                </div>
                <div class="text-caption text-medium-emphasis mt-2" style="overflow-wrap: anywhere">来源 Endpoint：{{ layaCapability.source_endpoint || '未记录来源，请重新获取能力' }}</div>
                <div v-if="layaCapability.source_capabilities_endpoint" class="text-caption text-medium-emphasis" style="overflow-wrap: anywhere">能力地址：{{ layaCapability.source_capabilities_endpoint }}</div>
                <div v-for="model in (layaCapability.models || [])" :key="model.id" class="mt-3">
                  <div class="text-body-2 font-weight-medium">模型 {{ model.id }} · {{ model.loaded === true ? '已加载' : model.loaded === false ? '未加载' : '未提供状态' }}</div>
                  <div class="text-caption text-medium-emphasis">类别模式：{{ model.sticker?.category_mode || '未知' }}<span v-if="model.sticker?.max_categories">，服务端上限：{{ model.sticker.max_categories }}</span></div>
                  <div v-if="capabilityCategories(model).length" class="mt-1">
                    <span class="text-caption text-medium-emphasis me-2">服务端类别：</span>
                    <v-chip v-for="category in capabilityCategories(model)" :key="category.id" size="x-small" variant="outlined" class="me-1 mb-1">{{ category.id }}<span v-if="category.description"> · {{ category.description }}</span></v-chip>
                  </div>
                </div>
                <div v-if="layaCapability.models?.length" class="text-caption text-medium-emphasis mt-2">管理员类别不会因能力刷新自动删除或改写。</div>
                <div v-if="serverCategoryIDs().length" class="mt-2">
                  <span class="text-caption text-medium-emphasis me-2">类别差异：</span>
                  <span v-if="missingAdminCategoryIDs().length" class="me-2">服务端建议但尚未配置
                    <v-chip v-for="id in missingAdminCategoryIDs()" :key="`missing-${id}`" size="x-small" color="warning" variant="tonal" class="me-1">{{ id }}</v-chip>
                  </span>
                  <span v-if="unadvertisedAdminCategoryIDs().length">管理员已配置但服务端未列出
                    <v-chip v-for="id in unadvertisedAdminCategoryIDs()" :key="`extra-${id}`" size="x-small" color="info" variant="tonal" class="me-1">{{ id }}</v-chip>
                  </span>
                </div>
              </v-card-text>
            </v-card>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- 自定义判断提示词弹窗 -->
    <v-dialog v-model="promptDialog" max-width="720">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center justify-space-between pa-4">
          <span>自定义判断提示词</span>
          <v-btn icon="mdi-close" size="small" variant="text" @click="promptDialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-4">
          <v-textarea
            v-model="promptDraft"
            label="留空使用默认规则；填写后替换默认的「回复规则」"
            rows="8"
            density="comfortable" variant="outlined"
            placeholder="例如：&#10;- 只有直接@机器人或请求机器人办事的消息才算相关&#10;- 群友闲聊一律不回复（相关度 < 0.1）"
          />
          <div class="text-caption text-medium-emphasis">
            自定义提示词将替换相关性判断的「回复规则」部分，消息上下文仍会自动附加。
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions class="pa-4">
          <v-spacer />
          <v-btn variant="text" @click="promptDialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" @click="savePrompt">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useToastStore } from '@/stores/toast'
import { replyStrategyApi, providerApi, stickerTagApi, type ProviderResp, type LayaCategoryReq, type LayaCapabilitySnapshot } from '@/api'

const toastStore = useToastStore()
const saving = ref(false)
const promptDialog = ref(false)
const promptDraft = ref('')
const capabilityLoading = ref(false)
const capabilityPreview = ref(false)
const layaCapability = ref<LayaCapabilitySnapshot | null>(null)
const stickerTagOptions = ref<string[]>([])

type CategoryRow = LayaCategoryReq & { readonly rowKey: number }
let nextCategoryRowKey = 0

function createCategoryRow(category: LayaCategoryReq): CategoryRow {
  return { ...category, rowKey: nextCategoryRowKey++ }
}

// 显式构造 API DTO，避免把前端行标识发送到后端。
function categoryDTOs(): LayaCategoryReq[] {
  return form.value.laya_sticker_categories.map(({ id, description, sticker_tags, no_send, enabled }) => ({
    id, description, sticker_tags: [...sticker_tags], no_send, enabled,
  }))
}

type ReplyForm = {
  relevance_threshold: number
  bot_name: string
  strip_markdown: boolean
  agent_lite: boolean
  relevance_prompt: string
  relevance_model: string
  relevance_timeout: number
  judge_fail_policy: string
  laya_sticker_enabled: boolean
  laya_sticker_endpoint: string
  laya_capabilities_endpoint: string
  laya_sticker_api_key: string
  laya_sticker_api_key_set: boolean
  laya_sticker_clear_api_key: boolean
  laya_sticker_model: string
  laya_sticker_timeout: number
  laya_sticker_min_confidence: number
  laya_sticker_task_ttl_seconds: number
  laya_sticker_protocol_mode: string
  laya_sticker_http_method: string
  laya_sticker_request_template: string
  laya_sticker_response_category_path: string
  laya_sticker_response_confidence_path: string
  laya_sticker_categories: CategoryRow[]
  laya_capability_fetched_at: string | null
  laya_capability_error: string
}

const form = ref<ReplyForm>({
  relevance_threshold: 0.5,
  bot_name: '',
  strip_markdown: false,
  agent_lite: false,
  relevance_prompt: '',
  relevance_model: '',
  relevance_timeout: 10,
  judge_fail_policy: 'drop',
  laya_sticker_enabled: false,
  laya_sticker_endpoint: '',
  laya_capabilities_endpoint: '',
  laya_sticker_api_key: '',
  laya_sticker_api_key_set: false,
  laya_sticker_clear_api_key: false,
  laya_sticker_model: '',
  laya_sticker_timeout: 10,
  laya_sticker_min_confidence: 0,
  laya_sticker_task_ttl_seconds: 30,
  laya_sticker_protocol_mode: 'json',
  laya_sticker_http_method: 'POST',
  laya_sticker_request_template: '',
  laya_sticker_response_category_path: '/answers/sticker/choice',
  laya_sticker_response_confidence_path: '/answers/sticker/answer_confidence',
  laya_sticker_categories: [],
  laya_capability_fetched_at: null,
  laya_capability_error: '',
})

// 相关性检测可选的 Text 模型列表（仅 text_model 类型）
const textModelOptions = ref<{ label: string; value: string }[]>([])

async function loadModels() {
  try {
    const list = (await providerApi.list()).data.data || []
    textModelOptions.value = list
      .filter((p: ProviderResp) => p.type === 'text_model')
      .map((p: ProviderResp) => ({ label: `${p.name} · ${p.model}`, value: p.id }))
  } catch { /* ignore */ }
}

async function load() {
  try {
    const res = await replyStrategyApi.get()
    const d = (res.data as any)?.data
    if (d) {
      form.value.relevance_threshold = d.relevance_threshold ?? 0.5
      form.value.bot_name = d.bot_name || ''
      form.value.strip_markdown = d.strip_markdown || false
      form.value.agent_lite = d.agent_lite || false
      form.value.relevance_prompt = d.relevance_prompt || ''
      form.value.relevance_model = d.relevance_model || ''
      form.value.relevance_timeout = d.relevance_timeout || 10
      form.value.judge_fail_policy = d.judge_fail_policy || 'drop'
      form.value.laya_sticker_enabled = d.laya_sticker_enabled || false
      form.value.laya_sticker_endpoint = d.laya_sticker_endpoint || ''
      form.value.laya_capabilities_endpoint = d.laya_capabilities_endpoint || ''
      form.value.laya_sticker_model = d.laya_sticker_model || ''
      form.value.laya_sticker_timeout = d.laya_sticker_timeout || 10
      form.value.laya_sticker_min_confidence = d.laya_sticker_min_confidence ?? 0
      form.value.laya_sticker_task_ttl_seconds = d.laya_sticker_task_ttl_seconds || 30
      form.value.laya_sticker_protocol_mode = d.laya_sticker_protocol_mode || 'json'
      form.value.laya_sticker_http_method = d.laya_sticker_http_method || 'POST'
      form.value.laya_sticker_request_template = d.laya_sticker_request_template || ''
      form.value.laya_sticker_response_category_path = d.laya_sticker_response_category_path || '/answers/sticker/choice'
      form.value.laya_sticker_response_confidence_path = d.laya_sticker_response_confidence_path || '/answers/sticker/answer_confidence'
      form.value.laya_sticker_categories = (d.laya_sticker_categories || []).map((item: LayaCategoryReq) => createCategoryRow({
        id: item.id || '', description: item.description || '', sticker_tags: [...(item.sticker_tags || [])], no_send: !!item.no_send, enabled: !!item.enabled,
      }))
      form.value.laya_sticker_api_key_set = !!d.laya_sticker_api_key_set
      form.value.laya_sticker_api_key = ''
      form.value.laya_sticker_clear_api_key = false
      form.value.laya_capability_fetched_at = d.laya_capability_fetched_at || null
      form.value.laya_capability_error = d.laya_capability_error || ''
      layaCapability.value = d.laya_capability_snapshot || null
      capabilityPreview.value = !!d.laya_capability_preview
    }
  } catch (_e: any) {}
}

async function loadStickerTags() {
  try {
    const list = (await stickerTagApi.list()).data.data || []
    stickerTagOptions.value = list.map((item: { name: string }) => item.name).filter(Boolean)
  } catch { /* optional suggestions */ }
}

function addCategory() {
  form.value.laya_sticker_categories.push(createCategoryRow({ id: '', description: '', sticker_tags: [], no_send: false, enabled: true }))
}

function removeCategory(index: number) {
  form.value.laya_sticker_categories.splice(index, 1)
}

function formatTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function capabilityCategories(model: NonNullable<LayaCapabilitySnapshot['models']>[number]) {
  return model.sticker?.category_mode === 'dynamic'
    ? (model.sticker.suggested_categories || [])
    : (model.sticker?.categories || [])
}

function serverCategoryIDs() {
  const ids = new Set<string>()
  for (const model of layaCapability.value?.models || []) {
    for (const category of capabilityCategories(model)) {
      if (category.id) ids.add(category.id)
    }
  }
  return [...ids]
}

function adminCategoryIDs() {
  return form.value.laya_sticker_categories.map(category => category.id).filter(Boolean)
}

function missingAdminCategoryIDs() {
  const admin = new Set(adminCategoryIDs())
  return serverCategoryIDs().filter(id => !admin.has(id))
}

function unadvertisedAdminCategoryIDs() {
  const server = new Set(serverCategoryIDs())
  return adminCategoryIDs().filter(id => !server.has(id))
}

function capabilityProtocolLabel(protocol?: string) {
  if (protocol === 'json') return '可配置 JSON'
  if (protocol === 'systemone.v1') return 'Laya 原生协议'
  return '当前适配器未知'
}

function capabilityProtocolColor(protocol?: string) {
  return protocol === 'json' || protocol === 'systemone.v1' ? 'success' : 'warning'
}

async function refreshCapabilities(savedConfig = false) {
  capabilityLoading.value = true
  try {
    const res = await replyStrategyApi.refreshLayaCapabilities(savedConfig ? undefined : {
      relevance_threshold: form.value.relevance_threshold,
      bot_name: form.value.bot_name,
      strip_markdown: form.value.strip_markdown,
      agent_lite: form.value.agent_lite,
      relevance_prompt: form.value.relevance_prompt,
      relevance_model: form.value.relevance_model,
      relevance_timeout: form.value.relevance_timeout,
      judge_fail_policy: form.value.judge_fail_policy,
      laya_sticker_enabled: form.value.laya_sticker_enabled,
      laya_sticker_endpoint: form.value.laya_sticker_endpoint,
      laya_capabilities_endpoint: form.value.laya_capabilities_endpoint,
      laya_sticker_api_key: form.value.laya_sticker_api_key,
      laya_sticker_clear_api_key: form.value.laya_sticker_clear_api_key,
      laya_sticker_model: form.value.laya_sticker_model,
      laya_sticker_timeout: form.value.laya_sticker_timeout,
      laya_sticker_protocol_mode: form.value.laya_sticker_protocol_mode,
      laya_sticker_http_method: form.value.laya_sticker_http_method,
      laya_sticker_request_template: form.value.laya_sticker_request_template,
      laya_sticker_response_category_path: form.value.laya_sticker_response_category_path,
      laya_sticker_response_confidence_path: form.value.laya_sticker_response_confidence_path,
      laya_sticker_categories: categoryDTOs(),
      laya_sticker_min_confidence: form.value.laya_sticker_min_confidence,
      laya_sticker_task_ttl_seconds: form.value.laya_sticker_task_ttl_seconds,
    })
    const d = (res.data as any)?.data
    if (d) {
      layaCapability.value = d.laya_capability_snapshot || null
      capabilityPreview.value = !!d.laya_capability_preview
      form.value.laya_capability_fetched_at = d.laya_capability_fetched_at || null
      form.value.laya_capability_error = d.laya_capability_error || ''
      form.value.laya_sticker_api_key_set = !!d.laya_sticker_api_key_set
    }
    toastStore.success(capabilityPreview.value ? 'Laya 能力预览完成，尚未保存' : 'Laya 正式能力快照已更新')
  } catch (e: any) {
    const message = e?.response?.data?.info || e?.message || '能力获取失败'
    if (savedConfig) form.value.laya_capability_error = message
    toastStore.error(savedConfig ? `配置已保存，能力刷新失败：${message}` : message)
  } finally { capabilityLoading.value = false }
}

function openPromptDialog() {
  promptDraft.value = form.value.relevance_prompt
  promptDialog.value = true
}

function savePrompt() {
  form.value.relevance_prompt = promptDraft.value
  promptDialog.value = false
  toastStore.success('提示词已更新，点击「保存」生效')
}

async function handleSave() {
  saving.value = true
  try {
      const response = await replyStrategyApi.update({
      relevance_threshold: form.value.relevance_threshold,
      bot_name: form.value.bot_name,
      strip_markdown: form.value.strip_markdown,
      agent_lite: form.value.agent_lite,
      relevance_prompt: form.value.relevance_prompt,
      relevance_model: form.value.relevance_model,
        relevance_timeout: form.value.relevance_timeout || 10,
        judge_fail_policy: form.value.judge_fail_policy,
        laya_sticker_enabled: form.value.laya_sticker_enabled,
        laya_sticker_endpoint: form.value.laya_sticker_endpoint,
        laya_capabilities_endpoint: form.value.laya_capabilities_endpoint,
        laya_sticker_api_key: form.value.laya_sticker_api_key,
        laya_sticker_clear_api_key: form.value.laya_sticker_clear_api_key,
        laya_sticker_model: form.value.laya_sticker_model,
        laya_sticker_timeout: form.value.laya_sticker_timeout || 10,
        laya_sticker_min_confidence: form.value.laya_sticker_min_confidence || 0,
        laya_sticker_task_ttl_seconds: form.value.laya_sticker_task_ttl_seconds || 30,
        laya_sticker_protocol_mode: form.value.laya_sticker_protocol_mode,
        laya_sticker_http_method: form.value.laya_sticker_http_method,
        laya_sticker_request_template: form.value.laya_sticker_request_template,
        laya_sticker_response_category_path: form.value.laya_sticker_response_category_path,
        laya_sticker_response_confidence_path: form.value.laya_sticker_response_confidence_path,
        laya_sticker_categories: categoryDTOs(),
      })
      form.value.laya_sticker_api_key = ''
      form.value.laya_sticker_clear_api_key = false
      const saved = (response.data as any)?.data
      if (saved) {
        form.value.laya_sticker_api_key_set = !!saved.laya_sticker_api_key_set
        layaCapability.value = saved.laya_capability_snapshot || null
        capabilityPreview.value = false
        form.value.laya_capability_fetched_at = saved.laya_capability_fetched_at || null
        form.value.laya_capability_error = saved.laya_capability_error || ''
      }
    toastStore.success('回复设置已保存')
    if (saved?.laya_sticker_endpoint || saved?.laya_capabilities_endpoint) {
      await refreshCapabilities(true)
    }
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || e?.message || '保存失败')
  } finally { saving.value = false }
}

onMounted(() => { load(); loadModels(); loadStickerTags() })
</script>

<style scoped>
.category-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px 16px;
}

.category-actions > * {
  flex: 0 0 auto;
}

.category-actions :deep(.v-label) {
  white-space: nowrap;
}
</style>
