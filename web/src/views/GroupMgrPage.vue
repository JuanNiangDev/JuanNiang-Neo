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

    <div class="d-flex align-center mb-3">
      <v-tabs v-model="tab" bg-color="transparent" class="flex-grow-1">
        <v-tab value="overview"><v-icon class="me-1">mdi-view-dashboard-outline</v-icon>数据总览</v-tab>
        <v-tab value="params"><v-icon class="me-1">mdi-tune-variant</v-icon>参数设置</v-tab>
        <v-tab value="prompts"><v-icon class="me-1">mdi-text-box-edit-outline</v-icon>提示词设置</v-tab>
        <v-tab value="words"><v-icon class="me-1">mdi-format-list-bulleted-type</v-icon>词库管理</v-tab>
        <v-tab value="violations"><v-icon class="me-1">mdi-clipboard-alert-outline</v-icon>违规记录</v-tab>
      </v-tabs>
      <v-btn color="primary" variant="tonal" prepend-icon="mdi-flask-outline" @click="openTestDialog">链路测试</v-btn>
    </div>

    <v-window v-model="tab">
      <!-- ================= 数据总览 ================= -->
      <v-window-item value="overview">
        <!-- 统计仪表盘 -->
        <v-card rounded="lg" elevation="1" class="mb-4">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">群管理统计</span></template>
            <template #append>
              <v-select
                v-model="statsGroupID"
                :items="groupOptions"
                label="选择群"
                density="compact"
                hide-details
                style="max-width: 240px"
                @update:model-value="loadStats"
              />
            </template>
          </v-card-item>
          <v-card-text>
            <v-row v-if="stats" dense>
              <v-col v-for="s in statCards" :key="s.label" cols="6" sm="4" md="3" lg="2">
                <div class="stat-card pa-3">
                  <v-icon :color="s.color" class="me-1" size="22">{{ s.icon }}</v-icon>
                  <span class="text-h6 font-weight-bold">{{ s.value }}</span>
                  <div class="text-caption text-medium-emphasis">{{ s.label }}</div>
                </div>
              </v-col>
            </v-row>
            <div v-else class="text-caption text-medium-emphasis">选择群查看统计（今日入群 / 刷屏 / 复读 / 广告 / 敏感 / 踢出）</div>
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
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">系统管理员</span><span class="text-body-2">{{ systemAdminCount }} 个</span></div>
                <div class="d-flex justify-space-between py-2"><span class="text-medium-emphasis">违规记录</span><span class="text-body-2">{{ violations.length }} 条</span></div>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>
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
                <div class="d-flex align-center ga-2 mt-2">
                  <v-btn color="warning" variant="tonal" prepend-icon="mdi-cancel" @click="openExcludeDialog">排除群设置</v-btn>
                  <v-btn color="primary" variant="tonal" prepend-icon="mdi-shield-plus-outline" @click="openWhitelistDialog">白名单设置</v-btn>
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
            </v-row>
            <v-divider class="my-3" />
            <v-row>
              <v-col cols="12" md="6">
                <div class="text-body-2 text-medium-emphasis mb-2">图片刷屏检测</div>
                <div class="d-flex align-center justify-space-between py-1">
                  <span class="text-body-2">时间窗口（秒）</span>
                  <v-text-field v-model.number="form.img_spam_window" type="number" min="1" density="compact" hide-details style="max-width:140px" @update:model-value="markDirty" />
                </div>
                <div class="d-flex align-center justify-space-between py-1">
                  <span class="text-body-2">触发警告图片数</span>
                  <v-text-field v-model.number="form.img_spam_threshold" type="number" min="1" density="compact" hide-details style="max-width:140px" @update:model-value="markDirty" />
                </div>
                <div class="d-flex align-center justify-space-between py-1">
                  <span class="text-body-2">重复刷屏禁言时长（秒）</span>
                  <v-text-field v-model.number="form.img_mute_duration" type="number" min="1" density="compact" hide-details style="max-width:140px" @update:model-value="markDirty" />
                </div>
              </v-col>
              <v-col cols="12" md="6">
                <div class="d-flex align-center justify-space-between py-1">
                  <span class="text-body-1">复读检测</span>
                  <v-switch v-model="form.enable_copy_check" color="primary" hide-details @change="markDirty" />
                </div>
                <div class="d-flex align-center justify-space-between py-1">
                  <span class="text-body-2">复读触发人数</span>
                  <v-text-field v-model.number="form.copy_threshold" type="number" min="1" density="compact" hide-details style="max-width:140px" @update:model-value="markDirty" />
                </div>
                <div class="d-flex align-center justify-space-between py-1">
                  <span class="text-body-2">二次违规禁言时长（秒）</span>
                  <v-text-field v-model.number="form.violation_mute_seconds" type="number" min="1" density="compact" hide-details style="max-width:140px" @update:model-value="markDirty" />
                </div>
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
            <template #subtitle>
              <v-progress-linear
                v-if="syncProgress.active"
                color="info"
                :model-value="syncProgress.percent"
                height="6"
                class="mt-1 rounded"
              />
              <div v-if="syncProgress.active" class="text-caption text-info mt-1">
                向量库同步中：已写入 {{ syncProgress.done }} 条{{ syncProgress.failed > 0 ? `，失败 ${syncProgress.failed} 条` : '' }}
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

      <!-- ================= 链路测试对话框（Tab 栏最右按钮） ================= -->
      <v-dialog v-model="testDialog" max-width="640">
        <v-card>
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-flask-outline</v-icon>链路测试</v-card-title>
          <v-card-text>
            <div class="d-flex align-center ga-2">
              <v-text-field v-model="testText" label="粘贴消息文本，查看判定流水（不处罚、不写库）" density="compact" hide-details class="flex-grow-1" @keydown.enter="runTest" />
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
          <v-card-actions class="pa-4 pt-0">
            <v-btn variant="text" @click="testDialog = false">关闭</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <!-- ================= 排除群设置对话框 ================= -->
      <v-dialog v-model="excludeDialog" max-width="480">
        <v-card>
          <v-card-title class="py-3"><v-icon class="me-2" color="warning">mdi-cancel</v-icon>排除检测的群</v-card-title>
          <v-card-text>
            <div class="text-caption text-medium-emphasis mb-3">这些群不跑任何群管理检测与处罚。</div>
            <div class="d-flex align-center ga-2 mb-3">
              <v-text-field v-model="excludeInput" label="群号" density="compact" hide-details type="number" @keydown.enter="addExclude" />
              <v-btn color="primary" variant="tonal" size="small" @click="addExclude">添加</v-btn>
            </div>
            <v-list density="compact" max-height="320" style="overflow-y: auto">
              <v-list-item v-for="g in excludeDraft" :key="g">
                <v-list-item-title>群 {{ g }}</v-list-item-title>
                <template #append>
                  <v-btn icon="mdi-delete-outline" size="x-small" variant="text" color="error" @click="removeExclude(g)" />
                </template>
              </v-list-item>
              <div v-if="!excludeDraft.length" class="text-caption text-medium-emphasis pa-2">暂无排除群</div>
            </v-list>
          </v-card-text>
          <v-card-actions class="pa-4 pt-0">
            <v-btn color="primary" variant="tonal" :loading="savingCfg" @click="saveExclude">保存</v-btn>
            <v-btn variant="text" @click="excludeDialog = false">取消</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <!-- ================= 白名单设置对话框 ================= -->
      <v-dialog v-model="whitelistDialog" max-width="480">
        <v-card>
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-shield-plus-outline</v-icon>白名单 QQ</v-card-title>
          <v-card-text>
            <div class="text-caption text-medium-emphasis mb-3">白名单 QQ 不参与任何违规检测（加入时自动清违规记录并解禁言）。</div>
            <div class="d-flex align-center ga-2 mb-3">
              <v-text-field v-model="whitelistInput" label="QQ 号" density="compact" hide-details type="number" @keydown.enter="addWhitelist" />
              <v-btn color="primary" variant="tonal" size="small" @click="addWhitelist">添加</v-btn>
            </div>
            <v-list density="compact" max-height="320" style="overflow-y: auto">
              <v-list-item v-for="qq in whitelistDraft" :key="qq">
                <v-list-item-title>{{ qq }}</v-list-item-title>
                <template #append>
                  <v-btn icon="mdi-delete-outline" size="x-small" variant="text" color="error" @click="removeWhitelist(qq)" />
                </template>
              </v-list-item>
              <div v-if="!whitelistDraft.length" class="text-caption text-medium-emphasis pa-2">暂无白名单</div>
            </v-list>
          </v-card-text>
          <v-card-actions class="pa-4 pt-0">
            <v-btn color="primary" variant="tonal" :loading="savingList" @click="saveWhitelist">保存</v-btn>
            <v-btn variant="text" @click="whitelistDialog = false">取消</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>
    </v-window>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { groupMgrApi, pluginApi, chatAreaApi, adapterApi, type GroupMgrConfigResp, type GroupMgrWordResp, type GroupMgrSampleResp, type GroupMgrViolationResp, type GroupMgrStatsResp, type GroupMgrTestResp, type ChatAreaResp } from '@/api'
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
  img_spam_window: 2,
  img_spam_threshold: 3,
  img_mute_duration: 60,
  enable_copy_check: true,
  copy_threshold: 3,
  violation_mute_seconds: 1800,
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
      img_spam_window: res.img_spam_window ?? 2,
      img_spam_threshold: res.img_spam_threshold ?? 3,
      img_mute_duration: res.img_mute_duration ?? 60,
      enable_copy_check: res.enable_copy_check ?? true,
      copy_threshold: res.copy_threshold ?? 3,
      violation_mute_seconds: res.violation_mute_seconds ?? 1800,
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
      img_spam_window: Number(form.value.img_spam_window) || 2,
      img_spam_threshold: Number(form.value.img_spam_threshold) || 3,
      img_mute_duration: Number(form.value.img_mute_duration) || 60,
      enable_copy_check: form.value.enable_copy_check,
      copy_threshold: Number(form.value.copy_threshold) || 3,
      violation_mute_seconds: Number(form.value.violation_mute_seconds) || 1800,
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
      img_spam_window: form.value.img_spam_window,
      img_spam_threshold: form.value.img_spam_threshold,
      img_mute_duration: form.value.img_mute_duration,
      enable_copy_check: form.value.enable_copy_check,
      copy_threshold: form.value.copy_threshold,
      violation_mute_seconds: form.value.violation_mute_seconds,
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

// ---------- 白名单 ----------
const whitelistQQs = ref<number[]>([])
const savingList = ref(false)

async function loadLists() {
  try {
    const wl = (await groupMgrApi.whitelist()).data.data
    whitelistQQs.value = wl?.qq_list ?? []
  } catch (e: any) {
    toastStore.error(e?.message || '加载列表失败')
  }
}

// ---------- 系统管理员（Adapter.Admins，只读展示） ----------
const systemAdminCount = ref(0)
async function loadSystemAdmins() {
  try {
    const c = (await adapterApi.getConfig()).data.data
    systemAdminCount.value = (c?.admin_qq_numbers || []).length
  } catch {
    systemAdminCount.value = 0
  }
}

// ---------- 词库 ----------
const words = ref<GroupMgrWordResp[]>([])
const samples = ref<GroupMgrSampleResp[]>([])
const syncing = ref(false)
// SSE 流式同步进度（词条量大时避免单次 HTTP 超时，逐批推送实时进度）
const syncProgress = ref({ active: false, done: 0, failed: 0, percent: 0 })
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

// syncRAG 同步向量库：SSE 流式（GET /group-mgr/sync-rag/stream），逐批推送进度，
// 词条量大不再单次 HTTP 请求超时。fetch + ReadableStream 解析（EventSource 无法带 JWT）。
async function syncRAG() {
  syncing.value = true
  syncProgress.value = { active: true, done: 0, failed: 0, percent: 0 }
  try {
    const token = localStorage.getItem('token') || ''
    const res = await fetch('/api/v1/group-mgr/sync-rag/stream', {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (!res.ok || !res.body) throw new Error('同步失败（RAG-Service 未配置或不可达）')

    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buf += decoder.decode(value, { stream: true })
      const blocks = buf.split('\n\n')
      buf = blocks.pop() ?? ''
      for (const block of blocks) {
        const dataLine = block.split('\n').find(l => l.startsWith('data:'))
        if (!dataLine) continue
        let data: any
        try { data = JSON.parse(dataLine.slice(5).trim()) } catch { continue }
        if (data.error) throw new Error(data.error)
        if (data.total !== undefined) {
          // done 事件：同步结束
          toastStore.success(`向量库同步完成：成功 ${data.total} 条，失败 ${data.failed} 条`)
          syncProgress.value = { active: false, done: data.total, failed: data.failed ?? 0, percent: 100 }
          await loadWords()
          return
        }
        if (data.done !== undefined) {
          syncProgress.value = { active: true, done: data.done, failed: data.failed ?? 0, percent: 0 }
        }
      }
    }
    throw new Error('同步中断（连接关闭）')
  } catch (e: any) {
    syncProgress.value.active = false
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
const groupOptions = ref<{ title: string; value: number }[]>([])
const statsGroupID = ref<number | null>(null)
const stats = ref<GroupMgrStatsResp | null>(null)
const statsLoading = ref(false)

const statCards = computed(() => {
  const s = stats.value
  if (!s) return []
  return [
    { label: '今日入群', value: s.join_today, icon: 'mdi-account-plus-outline', color: 'primary' },
    { label: '刷屏警告', value: s.warns, icon: 'mdi-image-multiple-outline', color: 'warning' },
    { label: '刷屏禁言', value: s.mutes, icon: 'mdi-account-cancel-outline', color: 'error' },
    { label: '复读警告', value: s.copy_warns, icon: 'mdi-repeat', color: 'info' },
    { label: '广告违规', value: s.ad, icon: 'mdi-bullhorn-outline', color: 'deep-orange' },
    { label: '敏感违规', value: s.sensitive, icon: 'mdi-alert-octagon-outline', color: 'purple' },
    { label: '踢出群聊', value: s.kicks, icon: 'mdi-account-remove-outline', color: 'red' },
  ]
})

// 群下拉选项：ChatArea 中 area_type=group 的列表
async function loadGroups() {
  try {
    const list = (await chatAreaApi.list()).data.data || []
    const groups = list.filter((c: ChatAreaResp) => c.area_type === 'group')
    groupOptions.value = groups.map((c: ChatAreaResp) => ({ title: `群 ${c.target_id}`, value: Number(c.target_id) }))
    // 切到界面时默认展示第一个群聊
    if (groupOptions.value.length && !groupOptions.value.some(o => o.value === statsGroupID.value)) {
      statsGroupID.value = groupOptions.value[0].value
      await loadStats()
    }
  } catch {
    groupOptions.value = []
  }
}

async function loadStats() {
  const gid = statsGroupID.value
  if (!gid) { stats.value = null; return }
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

// ---------- 链路测试（Tab 栏最右按钮 → 对话框） ----------
const testDialog = ref(false)
const testText = ref('')
const testing = ref(false)
const testReport = ref<GroupMgrTestResp | null>(null)

function openTestDialog() {
  testDialog.value = true
}

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

// ---------- 排除群设置对话框 ----------
const excludeDialog = ref(false)
const excludeInput = ref('')
const excludeDraft = ref<string[]>([])

function openExcludeDialog() {
  excludeDraft.value = [...form.value.exclude_groups]
  excludeInput.value = ''
  excludeDialog.value = true
}

function addExclude() {
  const g = excludeInput.value.trim()
  if (!/^\d+$/.test(g)) return
  if (!excludeDraft.value.includes(g)) excludeDraft.value.push(g)
  excludeInput.value = ''
}

function removeExclude(g: string) {
  excludeDraft.value = excludeDraft.value.filter(x => x !== g)
}

async function saveExclude() {
  savingCfg.value = true
  try {
    await groupMgrApi.updateConfig({
      enabled: form.value.enabled,
      llm_review: form.value.llm_review,
      high_score: form.value.high_score,
      low_score: form.value.low_score,
      fallback_score: form.value.fallback_score,
      img_spam_window: form.value.img_spam_window,
      img_spam_threshold: form.value.img_spam_threshold,
      img_mute_duration: form.value.img_mute_duration,
      enable_copy_check: form.value.enable_copy_check,
      copy_threshold: form.value.copy_threshold,
      violation_mute_seconds: form.value.violation_mute_seconds,
      exclude_groups: excludeDraft.value,
      llm_criteria: form.value.llm_criteria,
      llm_gray_prompt: form.value.llm_gray_prompt,
      llm_high_risk_prompt: form.value.llm_high_risk_prompt,
    })
    form.value.exclude_groups = [...excludeDraft.value]
    toastStore.success('排除群已保存')
    excludeDialog.value = false
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    savingCfg.value = false
  }
}

// ---------- 白名单设置对话框 ----------
const whitelistDialog = ref(false)
const whitelistInput = ref('')
const whitelistDraft = ref<number[]>([])

function openWhitelistDialog() {
  whitelistDraft.value = [...whitelistQQs.value]
  whitelistInput.value = ''
  whitelistDialog.value = true
}

function addWhitelist() {
  const qq = Number(whitelistInput.value)
  if (!qq) return
  if (!whitelistDraft.value.includes(qq)) whitelistDraft.value.push(qq)
  whitelistInput.value = ''
}

function removeWhitelist(qq: number) {
  whitelistDraft.value = whitelistDraft.value.filter(x => x !== qq)
}

async function saveWhitelist() {
  savingList.value = true
  try {
    await groupMgrApi.updateWhitelist(whitelistDraft.value)
    whitelistQQs.value = [...whitelistDraft.value]
    toastStore.success('白名单已保存')
    whitelistDialog.value = false
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    savingList.value = false
  }
}

// ---------- 展示辅助 ----------
function wordCount(cat: string) { return words.value.filter(w => w.category === cat).length }
function catLabel(c: string) { return { black: '黑色', gray: '灰色', sensitive: '敏感' }[c] ?? c }
function catColor(c: string) { return { black: 'error', gray: 'warning', sensitive: 'primary' }[c] ?? 'default' }
function pathLabel(p: string) { return { rag: 'RAG', llm: 'LLM', keyword: '关键词' }[p] ?? p }
function pathColor(p: string) { return { rag: 'info', llm: 'primary', keyword: 'warning' }[p] ?? 'default' }
function shortUUID(u: string) { return u ? `${u.slice(0, 8)}…${u.slice(-4)}` : '-' }

async function loadAll() {
  await Promise.all([loadConfig(), loadWords(), loadLists(), loadViolations(), loadSamples(), loadSystemAdmins()])
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
  loadGroups()
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
