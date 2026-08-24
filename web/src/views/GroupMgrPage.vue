<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-shield-check-outline</v-icon>群管理</div>
      <div class="page-subtitle">系统级群违规检测：卡片文本化 → RAG 语义核实（首选）→ LLM 审核兜底，含图片刷屏/复读检测与三级惩罚</div>
    </div>

    <!-- 旧插件警告 -->
    <v-alert v-if="legacyPluginEnabled" type="warning" variant="tonal" class="mb-4">
      检测到旧版 Lua 插件 redrock_group_manager 仍处于启用状态，会与本系统功能双重检测导致重复处罚。请到「Plugin 管理」页停用该插件。
    </v-alert>

    <v-tabs v-model="tab" bg-color="transparent" class="mb-3">
      <v-tab value="overview"><v-icon class="me-1">mdi-view-dashboard-outline</v-icon>数据总览</v-tab>
      <v-tab value="params"><v-icon class="me-1">mdi-tune-variant</v-icon>参数设置</v-tab>
      <v-tab value="prompts"><v-icon class="me-1">mdi-text-box-edit-outline</v-icon>提示词设置</v-tab>
      <v-tab value="words"><v-icon class="me-1">mdi-format-list-bulleted-type</v-icon>词库管理</v-tab>
      <v-tab value="violations"><v-icon class="me-1">mdi-clipboard-alert-outline</v-icon>违规记录</v-tab>
    </v-tabs>

    <v-window v-model="tab">
      <!-- ================= 数据总览 ================= -->
      <v-window-item value="overview">
        <!-- 统计仪表盘 -->
        <v-card rounded="lg" elevation="1" class="mb-4">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">群管理统计（/groupstats 同源）</span></template>
            <template #append>
              <div class="d-flex align-center ga-2">
                <v-text-field v-model="statsGroupID" label="群号" density="compact" hide-details type="number" style="max-width: 180px" @keydown.enter="loadStats" />
                <v-btn color="primary" variant="tonal" :loading="statsLoading" @click="loadStats">查询</v-btn>
              </div>
            </template>
          </v-card-item>
          <v-card-text>
            <v-row v-if="stats" dense>
              <v-col v-for="s in statCards" :key="s.label" cols="6" sm="4" md="3" lg="2">
                <div class="stat-card pa-3">
                  <div class="text-h6 font-weight-bold">{{ s.value }}</div>
                  <div class="text-caption text-medium-emphasis">{{ s.label }}</div>
                </div>
              </v-col>
            </v-row>
            <div v-else class="text-caption text-medium-emphasis">输入群号查询统计（今日入群 / 刷屏 / 复读 / 广告 / 敏感 / 踢出）</div>
          </v-card-text>
        </v-card>

        <!-- 运行状态卡 -->
        <v-row>
          <v-col cols="12" md="4">
            <v-card rounded="lg" elevation="1" class="h-100">
              <v-card-item><template #title><span class="text-h6 font-weight-bold">运行状态</span></template></v-card-item>
              <v-card-text class="d-flex flex-column">
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">群管理</span><v-chip size="small" :color="form.enabled ? 'success' : 'default'">{{ form.enabled ? '已启用' : '已停用' }}</v-chip></div>
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">LLM 审核</span><v-chip size="small" :color="form.llm_review ? 'success' : 'default'">{{ form.llm_review ? '已启用' : '已停用' }}</v-chip></div>
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">排除群</span><span class="text-body-2">{{ form.exclude_groups.length ? `${form.exclude_groups.length} 个` : '无' }}</span></div>
              </v-card-text>
            </v-card>
          </v-col>
          <v-col cols="12" md="4">
            <v-card rounded="lg" elevation="1" class="h-100">
              <v-card-item><template #title><span class="text-h6 font-weight-bold">词库与样本</span></template></v-card-item>
              <v-card-text class="d-flex flex-column">
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">词条总数</span><span class="text-body-2">{{ words.length }}（黑 {{ wordCount('black') }} / 灰 {{ wordCount('gray') }} / 敏 {{ wordCount('sensitive') }}）</span></div>
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">RAG 样本</span><span class="text-body-2">{{ samples.length }} 条</span></div>
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">词条已同步 RAG</span><span class="text-body-2">{{ words.filter(w => w.rag_synced).length }} / {{ words.length }}</span></div>
              </v-card-text>
            </v-card>
          </v-col>
          <v-col cols="12" md="4">
            <v-card rounded="lg" elevation="1" class="h-100">
              <v-card-item><template #title><span class="text-h6 font-weight-bold">白名单与管理员</span></template></v-card-item>
              <v-card-text class="d-flex flex-column">
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">白名单 QQ</span><span class="text-body-2">{{ whitelistQQs.length }} 个</span></div>
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">手动管理员</span><span class="text-body-2">{{ adminQQs.length }} 个</span></div>
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">违规记录</span><span class="text-body-2">{{ violations.length }} 条</span></div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>

        <!-- 链路测试 -->
        <v-card rounded="lg" elevation="1" class="mt-4">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">链路测试（不处罚、不写库）</span></template></v-card-item>
          <v-card-text>
            <div class="d-flex align-center ga-2">
              <v-text-field v-model="testText" label="粘贴消息文本，查看判定流水" density="compact" hide-details class="flex-grow-1" @keydown.enter="runTest" />
              <v-btn color="primary" variant="tonal" :loading="testing" @click="runTest">测试</v-btn>
            </div>
            <div v-if="testReport" class="mt-3">
              <div class="d-flex flex-wrap ga-2 mb-2">
                <v-chip size="small" :color="testReport.rag_ok ? 'success' : 'default'">RAG 可用: {{ testReport.rag_ok }}</v-chip>
                <v-chip v-if="testReport.word" size="small" color="warning">命中词: {{ testReport.word }} ({{ testReport.word_cat }})</v-chip>
                <v-chip v-if="testReport.card" size="small" color="error">推荐卡片</v-chip>
                <v-chip v-if="testReport.rag_ok" size="small" color="info">RAG 分数: {{ testReport.rag_score.toFixed(3) }}</v-chip>
              </div>
              <v-alert :type="verdictColor(testReport.verdict)" density="compact" variant="tonal">
                <b>{{ verdictLabel(testReport.verdict) }}</b> —— {{ testReport.reason }}
              </v-alert>
            </div>
          </v-card-text>
        </v-card>
      </v-window-item>

      <!-- ================= 参数设置 ================= -->
      <v-window-item value="params">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">参数设置</span></template></v-card-item>
          <v-card-text>
            <v-row>
              <v-col cols="12" md="6">
                <div class="d-flex align-center justify-space-between py-1">
                  <span class="text-body-1">启用群管理</span>
                  <v-switch v-model="form.enabled" color="primary" hide-details @change="markDirty" />
                </div>
                <div class="d-flex align-center justify-space-between py-1">
                  <span class="text-body-1">LLM 审核</span>
                  <v-switch v-model="form.llm_review" color="primary" hide-details @change="markDirty" />
                </div>
              </v-col>
              <v-col cols="12" md="6">
                <div class="text-body-2 text-medium-emphasis mb-2">RAG 判定阈值（score ≥ high 直罚；low ~ high 送 LLM；LLM 异常 ≥ fallback 兜底直罚）</div>
                <div class="mb-1">
                  <div class="d-flex justify-space-between"><span class="text-body-2">高置信阈值</span><span class="text-body-2 font-weight-bold">{{ form.high_score.toFixed(2) }}</span></div>
                  <v-slider v-model="form.high_score" min="0" max="1" step="0.05" color="error" hide-details @update:model-value="markDirty" />
                </div>
                <div class="mb-1">
                  <div class="d-flex justify-space-between"><span class="text-body-2">模棱两可下限</span><span class="text-body-2 font-weight-bold">{{ form.low_score.toFixed(2) }}</span></div>
                  <v-slider v-model="form.low_score" min="0" max="1" step="0.05" color="warning" hide-details @update:model-value="markDirty" />
                </div>
                <div>
                  <div class="d-flex justify-space-between"><span class="text-body-2">LLM 异常兜底分</span><span class="text-body-2 font-weight-bold">{{ form.fallback_score.toFixed(2) }}</span></div>
                  <v-slider v-model="form.fallback_score" min="0" max="1" step="0.05" color="primary" hide-details @update:model-value="markDirty" />
                </div>
              </v-col>
              <v-col cols="12" md="6">
                <v-combobox v-model="form.exclude_groups" label="排除检测的群 ID（回车添加）" multiple chips hide-selected @update:model-value="markDirty" />
              </v-col>
              <v-col cols="12" md="6">
                <v-combobox v-model="whitelistQQs" label="白名单 QQ（不参与检测）" multiple chips hide-selected type="number" @update:model-value="markDirty" />
              </v-col>
              <v-col cols="12">
                <div class="text-subtitle-2 font-weight-bold mb-1">手动管理员 QQ</div>
                <div class="d-flex align-center ga-2">
                  <v-combobox v-model="adminQQs" label="管理员（群角色无法识别时生效）" multiple chips hide-selected type="number" class="flex-grow-1" @update:model-value="markDirty" />
                  <v-btn color="info" variant="tonal" prepend-icon="mdi-sync" :loading="syncingAdmins" @click="syncAdminsFromAdapter">从 Adapter 同步</v-btn>
                </div>
                <div class="text-caption text-medium-emphasis mt-1">「从 Adapter 同步」将系统管理员（Adapter.Admins 配置）合并到手动管理员列表。</div>
              </v-col>
            </v-row>
          </v-card-text>
          <v-card-actions class="pa-4 pt-0">
            <v-btn color="primary" variant="tonal" prepend-icon="mdi-content-save" :loading="savingCfg" @click="saveConfig">保存参数</v-btn>
            <v-spacer />
            <v-btn variant="text" prepend-icon="mdi-refresh" @click="loadAll">刷新</v-btn>
          </v-card-actions>
        </v-card>
      </v-window-item>

      <!-- ================= 提示词设置 ================= -->
      <v-window-item value="prompts">
        <v-card rounded="lg" elevation="1">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">提示词设置</span></template>
            <template #append>
              <div class="d-flex align-center ga-2">
                <v-select v-model="promptType" :items="promptTypeOptions" density="compact" hide-details style="max-width: 220px" @update:model-value="promptEditing = false" />
                <v-btn :color="promptEditing ? 'success' : 'primary'" variant="tonal" prepend-icon="mdi-pencil" @click="startEditPrompt">{{ promptEditing ? '保存' : '编辑' }}</v-btn>
                <v-btn v-if="promptEditing" variant="text" @click="promptEditing = false">取消</v-btn>
              </div>
            </template>
          </v-card-item>
          <v-card-text>
            <div class="text-caption text-medium-emphasis mb-2">当前类型：{{ promptTypeLabel }} —— {{ promptTypeHint }}</div>
            <v-textarea
              v-model="promptText"
              :readonly="!promptEditing"
              :rows="22"
              auto-grow
              :bg-color="promptEditing ? 'surface-variant' : undefined"
              :hint="promptEditing ? '修改后点击右上角「保存」' : '点击右上角「编辑」后可修改'"
              persistent-hint
            />
          </v-card-text>
        </v-card>
      </v-window-item>

      <!-- ================= 词库管理 ================= -->
      <v-window-item value="words">
        <v-card rounded="lg" elevation="1">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">词库管理</span></template>
            <template #append>
              <div class="d-flex align-center ga-2">
                <v-btn color="primary" variant="tonal" prepend-icon="mdi-plus" @click="wordDialog = true">添加词库</v-btn>
                <v-btn color="info" variant="tonal" prepend-icon="mdi-cloud-sync-outline" :loading="syncing" @click="syncRAG">同步向量数据库</v-btn>
              </div>
            </template>
          </v-card-item>
          <v-card-text>
            <v-data-table :headers="wordHeaders" :items="words" density="compact" :items-per-page="15" class="elevation-0">
              <template #item.category="{ item }">
                <v-chip size="x-small" :color="catColor(item.category)">{{ catLabel(item.category) }}</v-chip>
              </template>
              <template #item.rag_tag="{ item }">
                <span class="text-caption font-family-monospace">{{ item.rag_tag ? shortUUID(item.rag_tag) : '-' }}</span>
              </template>
              <template #item.rag_synced="{ item }">
                <v-chip size="x-small" :color="item.rag_synced ? 'success' : 'default'">{{ item.rag_synced ? '已同步' : '未同步' }}</v-chip>
              </template>
              <template #item.source="{ item }">
                <span class="text-caption">{{ item.source === 'system' ? '种子' : '导入' }}</span>
              </template>
              <template #item.actions="{ item }">
                <v-btn icon="mdi-delete-outline" size="x-small" variant="text" color="error" @click="deleteWord(item)" />
              </template>
            </v-data-table>
          </v-card-text>
        </v-card>

        <!-- 添加词库对话框 -->
        <v-dialog v-model="wordDialog" max-width="560">
          <v-card>
            <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-plus-circle-outline</v-icon>添加词库</v-card-title>
            <v-card-text>
              <v-select v-model="wordForm.category" :items="wordCatOptions" label="词的类型" density="compact" class="mb-3" />
              <v-radio-group v-model="wordForm.mode" density="compact" class="mb-1">
                <v-radio label="输入词条" value="input" />
                <v-radio label="从 txt 文件导入（一行一个）" value="file" />
              </v-radio-group>
              <v-textarea
                v-if="wordForm.mode === 'input'"
                v-model="wordForm.input"
                label="词条（可多行，每行一个）"
                rows="4"
                density="compact"
                class="mb-3"
                placeholder="例如：办校园卡"
              />
              <v-file-input
                v-else
                v-model="wordForm.file"
                label="选择 txt 文件"
                accept=".txt"
                density="compact"
                class="mb-3"
              />
            </v-card-text>
            <v-card-actions class="pa-4 pt-0">
              <v-btn color="primary" variant="tonal" :loading="addingWords" @click="submitWordDialog">确定添加</v-btn>
              <v-btn variant="text" @click="wordDialog = false">取消</v-btn>
            </v-card-actions>
          </v-card>
        </v-dialog>
      </v-window-item>

      <!-- ================= 违规记录 ================= -->
      <v-window-item value="violations">
        <v-card rounded="lg" elevation="1">
          <v-card-item><template #title><span class="text-h6 font-weight-bold">违规记录</span></template></v-card-item>
          <v-card-text>
            <v-data-table :headers="violationHeaders" :items="violations" density="compact" :items-per-page="15" class="elevation-0">
              <template #item.detection_path="{ item }">
                <v-chip size="x-small" :color="pathColor(item.detection_path)">{{ pathLabel(item.detection_path) }}</v-chip>
              </template>
              <template #item.llm_reason="{ item }">
                <v-btn
                  v-if="item.detection_path === 'llm'"
                  size="x-small"
                  variant="tonal"
                  color="primary"
                  prepend-icon="mdi-message-text-outline"
                  @click="reasonDialog = item"
                >查看原因</v-btn>
                <span v-else class="text-caption text-medium-emphasis">-</span>
              </template>
              <template #item.actions="{ item }">
                <v-btn icon="mdi-delete-outline" size="x-small" variant="text" color="error" @click="deleteViolation(item)" />
              </template>
            </v-data-table>
          </v-card-text>
        </v-card>

        <!-- LLM 原因对话框 -->
        <v-dialog v-model="reasonDialogOpen" max-width="560">
          <v-card>
            <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-message-text-outline</v-icon>LLM 判定原因</v-card-title>
            <v-card-text>
              <div class="text-body-2 text-medium-emphasis mb-2">群 {{ reasonDialog?.group_id }} · QQ {{ reasonDialog?.user_id }} · {{ reasonDialog?.username || '未知用户' }}</div>
              <v-sheet class="pa-3 rounded" variant="tonal" style="background: rgba(var(--v-theme-on-surface), 0.05)">
                <div class="text-body-2" style="white-space: pre-wrap">{{ reasonDialog?.llm_reason || '（无）' }}</div>
              </v-sheet>
            </v-card-text>
            <v-card-actions class="pa-4 pt-0">
              <v-btn variant="text" @click="reasonDialog = null">关闭</v-btn>
            </v-card-actions>
          </v-card>
        </v-dialog>
      </v-window-item>
    </v-window>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { groupMgrApi, pluginApi, type GroupMgrConfigResp, type GroupMgrWordResp, type GroupMgrSampleResp, type GroupMgrViolationResp, type GroupMgrStatsResp, type GroupMgrTestResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()

const tab = ref('overview')

// ---------- 配置 ----------
const form = ref<GroupMgrConfigResp>({
  enabled: false,
  llm_review: true,
  high_score: 0.75,
  low_score: 0.5,
  fallback_score: 0.6,
  exclude_groups: [],
  llm_criteria: '',
  llm_gray_prompt: '',
  llm_high_risk_prompt: '',
})
const savingCfg = ref(false)
function markDirty() { /* 参数保存为显式按钮，无需脏标记 */ }

async function loadConfig() {
  try {
    const res = (await groupMgrApi.getConfig()).data.data
    form.value = {
      enabled: res.enabled,
      llm_review: res.llm_review,
      high_score: res.high_score,
      low_score: res.low_score,
      fallback_score: res.fallback_score,
      exclude_groups: res.exclude_groups ?? [],
      llm_criteria: res.llm_criteria ?? '',
      llm_gray_prompt: res.llm_gray_prompt ?? '',
      llm_high_risk_prompt: res.llm_high_risk_prompt ?? '',
    }
  } catch (e: any) {
    toastStore.error(e?.message || '加载配置失败')
  }
}

async function saveConfig() {
  savingCfg.value = true
  try {
    await groupMgrApi.updateConfig({
      enabled: form.value.enabled,
      llm_review: form.value.llm_review,
      high_score: Number(form.value.high_score) || 0.75,
      low_score: Number(form.value.low_score) || 0.5,
      fallback_score: Number(form.value.fallback_score) || 0.6,
      exclude_groups: form.value.exclude_groups.filter((g) => /^\d+$/.test(String(g).trim())).map((g) => String(g).trim()),
      llm_criteria: form.value.llm_criteria,
      llm_gray_prompt: form.value.llm_gray_prompt,
      llm_high_risk_prompt: form.value.llm_high_risk_prompt,
    })
    toastStore.success('参数已保存，已热重载')
    await loadConfig()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    savingCfg.value = false
  }
}

// ---------- 提示词设置 ----------
const promptTypeOptions = [
  { title: '公共判定标准', value: 'llm_criteria' },
  { title: '常规审查提示词', value: 'llm_gray_prompt' },
  { title: '高危复核提示词', value: 'llm_high_risk_prompt' },
]
const promptType = ref('llm_criteria')
const promptEditing = ref(false)
const promptText = computed({
  get: () => form.value[promptType.value as keyof GroupMgrConfigResp] as string ?? '',
  set: (v: string) => { (form.value as any)[promptType.value] = v },
})
const promptTypeLabel = computed(() => promptTypeOptions.find(o => o.value === promptType.value)?.title ?? '')
const promptTypeHint = computed(() => ({
  llm_criteria: '拼接进两套提示词的公共判定标准',
  llm_gray_prompt: '灰色词 / 语义疑似消息的常规审查，倾向放行',
  llm_high_risk_prompt: '敏感/黑名单/卡片/RAG 高置信复核，倾向违规',
}[promptType.value] ?? ''))

function startEditPrompt() {
  if (!promptEditing.value) {
    promptEditing.value = true
    return
  }
  // 保存当前提示词（只改提示词字段）
  savePrompt()
}

async function savePrompt() {
  savingCfg.value = true
  try {
    await groupMgrApi.updateConfig({
      enabled: form.value.enabled,
      llm_review: form.value.llm_review,
      high_score: form.value.high_score,
      low_score: form.value.low_score,
      fallback_score: form.value.fallback_score,
      exclude_groups: form.value.exclude_groups,
      llm_criteria: form.value.llm_criteria,
      llm_gray_prompt: form.value.llm_gray_prompt,
      llm_high_risk_prompt: form.value.llm_high_risk_prompt,
    })
    toastStore.success('提示词已保存')
    promptEditing.value = false
    await loadConfig()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    savingCfg.value = false
  }
}

// ---------- 白名单 / 管理员 ----------
const whitelistQQs = ref<number[]>([])
const adminQQs = ref<number[]>([])
const savingList = ref(false)
const syncingAdmins = ref(false)

async function loadLists() {
  try {
    const wl = (await groupMgrApi.whitelist()).data.data
    whitelistQQs.value = wl?.qq_list ?? []
    const ad = (await groupMgrApi.admins()).data.data
    adminQQs.value = ad?.qq_list ?? []
  } catch (e: any) {
    toastStore.error(e?.message || '加载列表失败')
  }
}

async function syncAdminsFromAdapter() {
  syncingAdmins.value = true
  try {
    const res = (await groupMgrApi.syncAdminsFromAdapter()).data.data
    toastStore.success(`已从 Adapter 同步管理员，新增 ${res?.added ?? 0} 个`)
    await loadLists()
  } catch (e: any) {
    toastStore.error(e?.message || '同步失败')
  } finally {
    syncingAdmins.value = false
  }
}

// ---------- 词库 ----------
const words = ref<GroupMgrWordResp[]>([])
const samples = ref<GroupMgrSampleResp[]>([])
const syncing = ref(false)
const wordDialog = ref(false)
const addingWords = ref(false)
const wordForm = ref<{ category: string; mode: string; input: string; file: File[] }>({ category: 'gray', mode: 'input', input: '', file: [] })
const wordCatOptions = [
  { title: '黑色地带（无歧义广告）', value: 'black' },
  { title: '灰色地带（语义模糊）', value: 'gray' },
  { title: '敏感（色情/政治/脏话）', value: 'sensitive' },
]
const wordHeaders = [
  { title: '词条', key: 'word' },
  { title: '词的类型', key: 'category' },
  { title: 'UUID', key: 'rag_tag' },
  { title: 'RAG 同步状态', key: 'rag_synced' },
  { title: '来源', key: 'source' },
  { title: '操作', key: 'actions', sortable: false },
]

async function loadWords() {
  try {
    const res = (await groupMgrApi.words()).data.data
    words.value = res || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载词库失败')
  }
}

async function submitWordDialog() {
  const cat = wordForm.value.category
  if (wordForm.value.mode === 'input') {
    const lines = wordForm.value.input.split('\n').map(s => s.trim()).filter(Boolean)
    if (!lines.length) { toastStore.error('请输入词条'); return }
    addingWords.value = true
    try {
      for (const w of lines) await groupMgrApi.addWord(w, cat)
      toastStore.success(`已添加 ${lines.length} 个词条`)
      wordDialog.value = false
      wordForm.value.input = ''
      await loadWords()
    } catch (e: any) {
      toastStore.error(e?.message || '添加失败')
    } finally { addingWords.value = false }
  } else {
    if (!wordForm.value.file?.length) { toastStore.error('请选择 txt 文件'); return }
    addingWords.value = true
    try {
      const res = (await groupMgrApi.importWords(wordForm.value.file[0], cat)).data.data
      toastStore.success(`导入完成：成功 ${res?.imported ?? 0} 条，跳过 ${res?.skipped ?? 0} 条`)
      wordDialog.value = false
      wordForm.value.file = []
      await loadWords()
    } catch (e: any) {
      toastStore.error(e?.message || '导入失败')
    } finally { addingWords.value = false }
  }
}

async function deleteWord(w: GroupMgrWordResp) {
  try {
    await groupMgrApi.deleteWord(w.id)
    toastStore.success(`已删除 ${w.word}`)
    await loadWords()
  } catch (e: any) {
    toastStore.error(e?.message || '删除失败')
  }
}

async function syncRAG() {
  syncing.value = true
  try {
    const res = (await groupMgrApi.syncRAG()).data.data
    toastStore.success(`向量库同步完成：成功 ${res?.total ?? 0} 条，失败 ${res?.failed ?? 0} 条`)
    await loadWords()
  } catch (e: any) {
    toastStore.error(e?.message || '同步失败（RAG-Service 未配置或不可达）')
  } finally {
    syncing.value = false
  }
}

// ---------- 违规记录 ----------
const violations = ref<GroupMgrViolationResp[]>([])
const reasonDialog = ref<GroupMgrViolationResp | null>(null)
// v-model 需要合法成员表达式，用 computed 包装对话框显隐（关闭时置 null）
const reasonDialogOpen = computed({
  get: () => reasonDialog.value !== null,
  set: (v: boolean) => { if (!v) reasonDialog.value = null },
})
const violationHeaders = [
  { title: '群号', key: 'group_id' },
  { title: 'QQ号', key: 'user_id' },
  { title: '用户名', key: 'username' },
  { title: '次数', key: 'count' },
  { title: '分析类型', key: 'detection_path' },
  { title: 'LLM 原因', key: 'llm_reason', sortable: false },
  { title: '操作', key: 'actions', sortable: false },
]

async function loadViolations() {
  try {
    const res = (await groupMgrApi.violations()).data.data
    violations.value = res || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载违规记录失败')
  }
}

async function deleteViolation(v: GroupMgrViolationResp) {
  try {
    await groupMgrApi.deleteViolation(v.id)
    toastStore.success(`已重置 ${v.group_id}:${v.user_id} 违规记录`)
    await loadViolations()
  } catch (e: any) {
    toastStore.error(e?.message || '删除失败')
  }
}

// ---------- 统计 ----------
const statsGroupID = ref('')
const stats = ref<GroupMgrStatsResp | null>(null)
const statsLoading = ref(false)

const statCards = computed(() => {
  const s = stats.value
  if (!s) return []
  return [
    { label: '今日入群', value: s.join_today },
    { label: '刷屏警告', value: s.warns },
    { label: '刷屏禁言', value: s.mutes },
    { label: '复读警告', value: s.copy_warns },
    { label: '广告违规', value: s.ad },
    { label: '敏感违规', value: s.sensitive },
    { label: '踢出群聊', value: s.kicks },
  ]
})

async function loadStats() {
  const gid = Number(statsGroupID.value)
  if (!gid) { toastStore.error('请输入群号'); return }
  statsLoading.value = true
  try {
    const res = (await groupMgrApi.stats(gid)).data.data
    stats.value = res
  } catch (e: any) {
    toastStore.error(e?.message || '查询失败')
  } finally {
    statsLoading.value = false
  }
}

// ---------- 链路测试 ----------
const testText = ref('')
const testing = ref(false)
const testReport = ref<GroupMgrTestResp | null>(null)

async function runTest() {
  const t = testText.value.trim()
  if (!t) return
  testing.value = true
  try {
    const res = (await groupMgrApi.test(t)).data.data
    testReport.value = res
  } catch (e: any) {
    toastStore.error(e?.message || '测试失败')
  } finally {
    testing.value = false
  }
}

function verdictLabel(v: string) { return { punish: '直接处罚', review: '送 LLM 审核', pass: '放行' }[v] ?? v }
function verdictColor(v: string) { return v === 'punish' ? 'error' : v === 'review' ? 'warning' : 'success' }

// ---------- 展示辅助 ----------
function wordCount(cat: string) { return words.value.filter(w => w.category === cat).length }
function catLabel(c: string) { return { black: '黑色', gray: '灰色', sensitive: '敏感' }[c] ?? c }
function catColor(c: string) { return { black: 'error', gray: 'warning', sensitive: 'primary' }[c] ?? 'default' }
function pathLabel(p: string) { return { rag: 'RAG', llm: 'LLM', keyword: '关键词' }[p] ?? p }
function pathColor(p: string) { return { rag: 'info', llm: 'primary', keyword: 'warning' }[p] ?? 'default' }
function shortUUID(u: string) { return u ? `${u.slice(0, 8)}…${u.slice(-4)}` : '-' }

async function loadAll() {
  await Promise.all([loadConfig(), loadWords(), loadLists(), loadViolations(), loadSamples()])
}

// ---------- 样本（数据总览用） ----------
async function loadSamples() {
  try {
    const res = (await groupMgrApi.samples()).data.data
    samples.value = res || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载样本失败')
  }
}

// ---------- 旧插件检测 ----------
const legacyPluginEnabled = ref(false)
async function checkLegacyPlugin() {
  try {
    const res = await pluginApi.list()
    const list = (res.data.data || []) as any[]
    legacyPluginEnabled.value = list.some((p: any) => (p.name === 'redrock_group_manager' || p.dir === 'redrock_group_manager') && p.is_active)
  } catch {
    legacyPluginEnabled.value = false
  }
}

onMounted(() => {
  loadAll()
  checkLegacyPlugin()
})
</script>

<style scoped>
.stat-card {
  border-radius: 10px;
  background: rgba(var(--v-theme-on-surface), 0.04);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}
.font-family-monospace {
  font-family: 'Roboto Mono', monospace;
}
</style>
