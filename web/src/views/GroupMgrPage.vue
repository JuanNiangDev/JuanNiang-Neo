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

    <v-row>
      <!-- 运行配置 -->
      <v-col cols="12" md="4">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-power</v-icon>运行状态</v-card-title>
          <v-card-text>
            <div class="d-flex align-center justify-space-between mb-2">
              <span class="text-body-1">启用群管理</span>
              <v-switch v-model="form.enabled" color="primary" hide-details @change="markDirty" />
            </div>
            <div class="d-flex align-center justify-space-between mb-2">
              <span class="text-body-1">LLM 审核</span>
              <v-switch v-model="form.llm_review" color="primary" hide-details @change="markDirty" />
            </div>
            <v-alert v-if="!form.enabled" type="info" density="compact" variant="tonal" class="mt-2">
              未启用时本功能完全不介入消息处理（含统计与命令仍可用）。
            </v-alert>
          </v-card-text>
        </v-card>
      </v-col>

      <!-- 阈值配置 -->
      <v-col cols="12" md="8">
        <v-card rounded="lg">
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-tune-variant</v-icon>判定阈值（RAG 语义核实）</v-card-title>
          <v-card-text>
            <v-row dense>
              <v-col cols="12" md="4">
                <v-text-field v-model.number="form.high_score" label="高置信阈值" type="number" min="0" max="1" step="0.05" hint="≥ 此分直接处罚（无词命中同样直罚）" persistent-hint @update:model-value="markDirty" />
              </v-col>
              <v-col cols="12" md="4">
                <v-text-field v-model.number="form.low_score" label="模棱两可下限" type="number" min="0" max="1" step="0.05" hint="≤ 此分且无词/卡片命中则放行" persistent-hint @update:model-value="markDirty" />
              </v-col>
              <v-col cols="12" md="4">
                <v-text-field v-model.number="form.fallback_score" label="LLM 异常分数兜底" type="number" min="0" max="1" step="0.05" hint="LLM 挂 + 模棱两可时 ≥ 此分直罚" persistent-hint @update:model-value="markDirty" />
              </v-col>
              <v-col cols="12">
                <v-combobox v-model="form.exclude_groups" label="排除检测的群 ID（回车添加，这些群不跑任何检测）" multiple chips hide-selected @update:model-value="markDirty" />
              </v-col>
            </v-row>
            <div class="text-caption text-medium-emphasis mt-1">
              判定链：高置信直罚 → 模棱两可 LLM 审核 → 低置信有词/卡片 LLM 终审 → 低置信无词放行；RAG 不可用自动降级关键词路径。
            </div>
          </v-card-text>
          <v-card-actions class="pa-4 pt-0">
            <v-btn color="primary" variant="tonal" prepend-icon="mdi-content-save" :loading="savingCfg" @click="saveConfig">保存配置</v-btn>
            <v-spacer />
            <v-btn variant="text" prepend-icon="mdi-refresh" @click="loadConfig">刷新</v-btn>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>

    <!-- LLM 提示词 -->
    <v-card rounded="lg" class="mt-4">
      <v-card-title class="py-3">
        <v-icon class="me-2" color="primary">mdi-text-box-edit-outline</v-icon>LLM 审核提示词
        <v-spacer />
        <v-btn size="small" variant="text" color="warning" @click="resetPrompts">恢复默认</v-btn>
      </v-card-title>
      <v-card-text>
        <v-textarea v-model="form.llm_criteria" label="公共判定标准（拼接进两套提示词）" rows="8" auto-grow @update:model-value="markDirty" />
        <v-textarea v-model="form.llm_gray_prompt" label="常规审查提示词（灰色词 / 语义疑似，倾向放行）" rows="10" auto-grow class="mt-2" @update:model-value="markDirty" />
        <v-textarea v-model="form.llm_high_risk_prompt" label="高危复核提示词（敏感/黑名单/卡片/RAG 高置信复核，倾向违规）" rows="10" auto-grow class="mt-2" @update:model-value="markDirty" />
      </v-card-text>
      <v-card-actions class="pa-4 pt-0">
        <v-btn color="primary" variant="tonal" prepend-icon="mdi-content-save" :loading="savingCfg" @click="saveConfig">保存配置</v-btn>
      </v-card-actions>
    </v-card>

    <!-- 词库管理 -->
    <v-card rounded="lg" class="mt-4">
      <v-card-title class="py-3">
        <v-icon class="me-2" color="primary">mdi-format-list-bulleted-type</v-icon>词库管理
        <v-spacer />
        <v-btn size="small" color="primary" variant="tonal" prepend-icon="mdi-cloud-sync-outline" :loading="syncing" @click="syncRAG">同步向量库</v-btn>
      </v-card-title>
      <v-card-text>
        <v-tabs v-model="wordTab" bg-color="transparent" class="mb-2">
          <v-tab value="black">黑色地带（无歧义广告）</v-tab>
          <v-tab value="gray">灰色地带（语义模糊）</v-tab>
          <v-tab value="sensitive">敏感（色情/政治/脏话）</v-tab>
        </v-tabs>

        <div class="d-flex align-center ga-2 mb-2 flex-wrap">
          <v-text-field v-model="newWord" label="新增词条" density="compact" hide-details class="flex-grow-1" style="max-width: 320px" @keydown.enter="addWord" />
          <v-btn color="primary" variant="tonal" size="small" @click="addWord">添加</v-btn>
          <v-file-input v-model="importFile" label="从 txt 导入（一行一个）" density="compact" hide-details accept=".txt" style="max-width: 260px" />
          <v-btn color="info" variant="tonal" size="small" :loading="importing" @click="importWords">导入到当前分类</v-btn>
        </div>

        <div class="d-flex flex-wrap ga-1">
          <v-chip v-for="w in currentWords" :key="w.id" closable size="small" @click:close="deleteWord(w)">
            {{ w.word }}<span v-if="w.source === 'system'" class="text-caption text-medium-emphasis ms-1">(种子)</span>
          </v-chip>
          <span v-if="!currentWords.length" class="text-caption text-medium-emphasis">当前分类暂无词条</span>
        </div>
        <div class="text-caption text-medium-emphasis mt-2">词条统一小写去重；导入/新增的词条在 RAG 可用时自动写入向量库（种子样本），RAG 未配置则仅存词库。</div>
      </v-card-text>
    </v-card>

    <!-- 管理数据 -->
    <v-row class="mt-2">
      <v-col cols="12" md="4">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-account-check-outline</v-icon>白名单 QQ（不参与检测）</v-card-title>
          <v-card-text>
            <v-combobox v-model="whitelistQQs" label="白名单（回车添加，删除即移出）" multiple chips hide-selected type="number" />
          </v-card-text>
          <v-card-actions class="pa-4 pt-0">
            <v-btn color="primary" variant="tonal" size="small" :loading="savingList" @click="saveWhitelist">保存白名单</v-btn>
          </v-card-actions>
        </v-card>
      </v-col>
      <v-col cols="12" md="4">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-account-cog-outline</v-icon>手动管理员 QQ</v-card-title>
          <v-card-text>
            <v-combobox v-model="adminQQs" label="手动管理员（群角色无法识别时生效）" multiple chips hide-selected type="number" />
          </v-card-text>
          <v-card-actions class="pa-4 pt-0">
            <v-btn color="primary" variant="tonal" size="small" :loading="savingList" @click="saveAdmins">保存管理员</v-btn>
          </v-card-actions>
        </v-card>
      </v-col>
      <v-col cols="12" md="4">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-clipboard-alert-outline</v-icon>违规记录（群:QQ:次数）</v-card-title>
          <v-card-text style="max-height: 260px; overflow-y: auto">
            <div v-for="v in violations" :key="v.id" class="d-flex align-center py-1">
              <span class="text-body-2">{{ v.group_id }} : {{ v.user_id }} : {{ v.count }}</span>
              <v-spacer />
              <v-btn icon="mdi-delete-outline" size="x-small" variant="text" color="error" @click="deleteViolation(v)" />
            </div>
            <span v-if="!violations.length" class="text-caption text-medium-emphasis">暂无违规记录</span>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- 统计 + 样本库 -->
    <v-row class="mt-2">
      <v-col cols="12" md="5">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-chart-box-outline</v-icon>统计（/groupstats 同源）</v-card-title>
          <v-card-text>
            <div class="d-flex align-center ga-2 mb-3">
              <v-text-field v-model="statsGroupID" label="群号" density="compact" hide-details type="number" style="max-width: 220px" @keydown.enter="loadStats" />
              <v-btn color="primary" variant="tonal" size="small" :loading="statsLoading" @click="loadStats">查询</v-btn>
            </div>
            <div v-if="stats" class="meta-list text-body-2">
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">日期</span><span>{{ stats.date }}</span></div>
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">今日入群</span><span>{{ stats.join_today }} 人</span></div>
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">刷屏警告</span><span>{{ stats.warns }} 次</span></div>
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">刷屏禁言</span><span>{{ stats.mutes }} 次</span></div>
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">复读警告</span><span>{{ stats.copy_warns }} 次</span></div>
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">广告违规</span><span>{{ stats.ad }} 次</span></div>
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">敏感违规</span><span>{{ stats.sensitive }} 次</span></div>
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">踢出群聊</span><span>{{ stats.kicks }} 次</span></div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" md="7">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-database-search-outline</v-icon>RAG 样本库（学习闭环自动入库）</v-card-title>
          <v-card-text style="max-height: 320px; overflow-y: auto">
            <div v-for="s in samples" :key="s.id" class="d-flex align-center py-1">
              <v-chip size="x-small" :color="s.category === 'ad' ? 'warning' : 'error'" class="me-2">{{ s.category }}</v-chip>
              <span class="text-body-2 text-truncate" style="max-width: 55%">{{ s.text }}</span>
              <v-spacer />
              <span class="text-caption text-medium-emphasis me-2">命中 {{ s.hit_count }} · {{ s.source }}</span>
              <v-btn icon="mdi-delete-outline" size="x-small" variant="text" color="error" @click="deleteSample(s)" />
            </div>
            <span v-if="!samples.length" class="text-caption text-medium-emphasis">暂无样本</span>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- 链路测试 -->
    <v-card rounded="lg" class="mt-4 mb-6">
      <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-flask-outline</v-icon>链路测试（不处罚、不写库）</v-card-title>
      <v-card-text>
        <div class="d-flex align-center ga-2">
          <v-text-field v-model="testText" label="粘贴消息文本" density="compact" hide-details class="flex-grow-1" @keydown.enter="runTest" />
          <v-btn color="primary" variant="tonal" :loading="testing" @click="runTest">测试</v-btn>
        </div>
        <div v-if="testReport" class="mt-3">
          <div class="d-flex flex-wrap ga-2 mb-2">
            <v-chip size="small" :color="testReport.rag_ok ? 'success' : 'default'">RAG 可用: {{ testReport.rag_ok }}</v-chip>
            <v-chip v-if="testReport.word" size="small" color="warning">命中词: {{ testReport.word }} ({{ testReport.word_cat }})</v-chip>
            <v-chip v-if="testReport.card" size="small" color="error">推荐卡片</v-chip>
            <v-chip v-if="testReport.rag_ok" size="small" color="info">RAG 分数: {{ testReport.rag_score.toFixed(3) }}</v-chip>
          </div>
          <div v-if="testReport.rag_ok && testReport.rag_sample" class="text-caption text-medium-emphasis mb-2">最相似样本: {{ testReport.rag_sample }}</div>
          <v-alert :type="verdictColor(testReport.verdict)" density="compact" variant="tonal">
            <b>{{ verdictLabel(testReport.verdict) }}</b> —— {{ testReport.reason }}
          </v-alert>
        </div>
      </v-card-text>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { groupMgrApi, pluginApi, type GroupMgrConfigResp, type GroupMgrWordResp, type GroupMgrSampleResp, type GroupMgrViolationResp, type GroupMgrStatsResp, type GroupMgrTestResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()

const DEFAULT_PROMPTS = {
  llm_criteria: '',
  llm_gray_prompt: '',
  llm_high_risk_prompt: '',
}

const form = ref<GroupMgrConfigResp & { llm_criteria: string; llm_gray_prompt: string; llm_high_risk_prompt: string }>({
  enabled: false,
  llm_review: true,
  high_score: 0.75,
  low_score: 0.5,
  fallback_score: 0.6,
  exclude_groups: [],
  ...DEFAULT_PROMPTS,
})
const dirty = ref(false)
const savingCfg = ref(false)

function markDirty() { dirty.value = true }

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
    dirty.value = false
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
    toastStore.success('配置已保存，已热重载')
    dirty.value = false
    await loadConfig()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    savingCfg.value = false
  }
}

// 提示词恢复默认：清空后端则回落内嵌默认值
function resetPrompts() {
  form.value.llm_criteria = ''
  form.value.llm_gray_prompt = ''
  form.value.llm_high_risk_prompt = ''
  markDirty()
  toastStore.success('已清空，保存后回落内嵌默认提示词')
}

// ---- 词库 ----
const wordTab = ref('black')
const words = ref<GroupMgrWordResp[]>([])
const newWord = ref('')
const importFile = ref<File[]>([])
const importing = ref(false)
const syncing = ref(false)

const currentWords = computed(() => words.value.filter((w) => w.category === wordTab.value))

async function loadWords() {
  try {
    const res = (await groupMgrApi.words()).data.data
    words.value = res || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载词库失败')
  }
}

async function addWord() {
  const w = newWord.value.trim().toLowerCase()
  if (!w) return
  try {
    await groupMgrApi.addWord(w, wordTab.value)
    newWord.value = ''
    toastStore.success('已添加（RAG 可用时同步写入向量库）')
    await loadWords()
  } catch (e: any) {
    toastStore.error(e?.message || '添加失败')
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

async function importWords() {
  if (!importFile.value?.length) {
    toastStore.error('请先选择 txt 文件')
    return
  }
  importing.value = true
  try {
    const res = (await groupMgrApi.importWords(importFile.value[0], wordTab.value)).data.data
    toastStore.success(`导入完成：成功 ${res?.imported ?? 0} 条，跳过 ${res?.skipped ?? 0} 条`)
    importFile.value = []
    await loadWords()
  } catch (e: any) {
    toastStore.error(e?.message || '导入失败')
  } finally {
    importing.value = false
  }
}

async function syncRAG() {
  syncing.value = true
  try {
    const res = (await groupMgrApi.syncRAG()).data.data
    toastStore.success(`向量库同步完成：成功 ${res?.total ?? 0} 条，失败 ${res?.failed ?? 0} 条`)
  } catch (e: any) {
    toastStore.error(e?.message || '同步失败（RAG-Service 未配置或不可达）')
  } finally {
    syncing.value = false
  }
}

// ---- 白名单 / 管理员 ----
const whitelistQQs = ref<number[]>([])
const adminQQs = ref<number[]>([])
const savingList = ref(false)

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

async function saveWhitelist() {
  savingList.value = true
  try {
    await groupMgrApi.updateWhitelist(whitelistQQs.value.map(Number).filter((n) => !isNaN(n)))
    toastStore.success('白名单已保存')
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    savingList.value = false
  }
}

async function saveAdmins() {
  savingList.value = true
  try {
    await groupMgrApi.updateAdmins(adminQQs.value.map(Number).filter((n) => !isNaN(n)))
    toastStore.success('管理员列表已保存')
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    savingList.value = false
  }
}

// ---- 违规记录 ----
const violations = ref<GroupMgrViolationResp[]>([])

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

// ---- 统计 ----
const statsGroupID = ref('')
const stats = ref<GroupMgrStatsResp | null>(null)
const statsLoading = ref(false)

async function loadStats() {
  const gid = Number(statsGroupID.value)
  if (!gid) {
    toastStore.error('请输入群号')
    return
  }
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

// ---- 样本库 ----
const samples = ref<GroupMgrSampleResp[]>([])

async function loadSamples() {
  try {
    const res = (await groupMgrApi.samples()).data.data
    samples.value = res || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载样本失败')
  }
}

async function deleteSample(s: GroupMgrSampleResp) {
  try {
    await groupMgrApi.deleteSample(s.id)
    toastStore.success('样本已删除（RAG 双删）')
    await loadSamples()
  } catch (e: any) {
    toastStore.error(e?.message || '删除失败')
  }
}

// ---- 链路测试 ----
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

function verdictLabel(v: string) {
  return { punish: '直接处罚', review: '送 LLM 审核', pass: '放行' }[v] ?? v
}
function verdictColor(v: string) {
  return v === 'punish' ? 'error' : v === 'review' ? 'warning' : 'success'
}

// ---- 旧插件检测 ----
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
  loadConfig()
  loadWords()
  loadLists()
  loadViolations()
  loadSamples()
  checkLegacyPlugin()
})
</script>
