<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-brain</v-icon>Planner 设置</div>
      <div class="page-subtitle">控制机器人的回复决策：规则打分权重 + LLM 规划</div>
    </div>

    <v-row>
      <!-- 打分权重 -->
      <v-col cols="12" md="8">
        <v-card rounded="lg" elevation="1">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">规则打分权重</span></template>
            <template #subtitle>消息综合评分 &ge; 阈值时进入 LLM 规划</template>
          </v-card-item>
          <v-card-text>
            <v-form @submit.prevent="handleSave">
              <!-- 阈值 -->
              <div class="text-subtitle-2 font-weight-bold mb-2">总阈值</div>
              <div class="d-flex align-center mb-4" style="gap:16px">
                <v-slider v-model="form.threshold" :min="0" :max="1" :step="0.05" color="primary" thumb-label="always" hide-details style="flex:1" />
                <v-text-field v-model.number="form.threshold" type="number" :min="0" :max="1" :step="0.05" density="compact" style="width:80px" hide-details variant="outlined" />
              </div>
              <div class="text-caption text-medium-emphasis mb-6">
                消息评分 &ge; 阈值时触发回复。值越低越容易回复，默认 0.30。
              </div>

              <!-- 各维度权重 -->
              <div class="text-subtitle-2 font-weight-bold mb-3">各维度权重</div>

              <div v-for="dim in dimensions" :key="dim.key" class="mb-4">
                <div class="d-flex align-center justify-space-between mb-1">
                  <div>
                    <span class="text-body-2 font-weight-bold">{{ dim.label }}</span>
                    <span class="text-caption text-medium-emphasis ml-2">{{ dim.desc }}</span>
                  </div>
                  <span class="text-caption font-weight-bold" :class="dim.color">{{ (form.weights[dim.key] as number).toFixed(2) }}</span>
                </div>
                <v-slider
                  v-model="form.weights[dim.key]"
                  :min="0" :max="1" :step="0.05"
                  :color="dim.sliderColor"
                  hide-details
                  thumb-label="always"
                />
              </div>

              <v-divider class="my-4" />

              <v-btn type="submit" color="primary" variant="tonal" :loading="saving">
                <v-icon class="me-1">mdi-content-save</v-icon> 保存 Planner 配置
              </v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>

      <!-- 其他设置 -->
      <v-col cols="12" md="4">
        <v-card rounded="lg" elevation="1">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">消息格式</span></template>
            <template #subtitle>AgentLite &amp; Markdown &amp; 静默检测</template>
          </v-card-item>
          <v-card-text>
            <v-form @submit.prevent="handleSave">
              <div class="d-flex align-start mb-4">
                <div class="flex-grow-1 me-3">
                  <div class="text-subtitle-2 font-weight-bold">AgentLite 模式</div>
                  <div class="text-caption text-medium-emphasis mt-1">关闭工具/MCP，消息直接发给 LLM 回复后结束。</div>
                </div>
                <v-switch v-model="form.agent_lite" color="primary" hide-details density="compact" />
              </div>
              <v-divider class="mb-4" />

              <div class="d-flex align-start mb-4">
                <div class="flex-grow-1 me-3">
                  <div class="text-subtitle-2 font-weight-bold">去除 Markdown</div>
                  <div class="text-caption text-medium-emphasis mt-1">发送前去除加粗/斜体/代码块/链接等格式，发送纯文本。</div>
                </div>
                <v-switch v-model="form.strip_markdown" color="primary" hide-details density="compact" />
              </div>
              <v-divider class="mb-4" />

              <div class="d-flex align-start mb-4">
                <div class="flex-grow-1 me-3">
                  <div class="text-subtitle-2 font-weight-bold">跳过静默检测</div>
                  <div class="text-caption text-medium-emphasis mt-1">
                    调试用。开启后 __NO_REPLY__ 等静默输出也会发送到 QQ。<br/>
                    正常使用请关闭。
                  </div>
                </div>
                <v-switch v-model="form.skip_silence_check" color="warning" hide-details density="compact" />
              </div>

              <v-btn type="submit" color="primary" variant="tonal" :loading="saving" block class="mt-4">
                <v-icon class="me-1">mdi-content-save</v-icon> 保存
              </v-btn>
            </v-form>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { useToastStore } from '@/stores/toast'
import { plannerApi, replyStrategyApi, type PlannerConfigResp } from '@/api'

const toastStore = useToastStore()
const saving = ref(false)

interface FormState {
  threshold: number
  weights: Record<string, number>
  agent_lite: boolean
  strip_markdown: boolean
  skip_silence_check: boolean
}

const form = reactive<FormState>({
  threshold: 0.30,
  weights: { mention: 0.35, keyword: 0.25, context: 0.20, quality: 0.10, history: 0.10 },
  agent_lite: false,
  strip_markdown: false,
  skip_silence_check: false,
})

const dimensions = [
  { key: 'mention', label: '@提及', desc: '@或名字出现在消息中', color: 'text-green', sliderColor: 'green' },
  { key: 'keyword', label: '关键词', desc: '匹配 Skill 关键词 / 疑问词', color: 'text-blue', sliderColor: 'blue' },
  { key: 'context', label: '上下文', desc: '近期有 bot 发言', color: 'text-purple', sliderColor: 'purple' },
  { key: 'quality', label: '内容质量', desc: '消息长度 & 非纯表情', color: 'text-orange', sliderColor: 'orange' },
  { key: 'history', label: '历史互动', desc: '积极互动比例', color: 'text-teal', sliderColor: 'teal' },
]

async function load() {
  try {
    const [planner, reply] = await Promise.all([
      plannerApi.getConfig(),
      replyStrategyApi.get(),
    ])
    const pd = (planner.data as any)?.data
    if (pd) {
      form.threshold = pd.threshold ?? 0.30
      if (pd.weights) {
        form.weights.mention = pd.weights.mention ?? 0.35
        form.weights.keyword = pd.weights.keyword ?? 0.25
        form.weights.context = pd.weights.context ?? 0.20
        form.weights.quality = pd.weights.quality ?? 0.10
        form.weights.history = pd.weights.history ?? 0.10
      }
    }
    const rd = (reply.data as any)?.data
    if (rd) {
      form.agent_lite = rd.agent_lite || false
      form.strip_markdown = rd.strip_markdown || false
      form.skip_silence_check = rd.skip_silence_check || false
    }
  } catch (_e: any) {}
}

async function handleSave() {
  saving.value = true
  try {
    await Promise.all([
      plannerApi.updateConfig({
        threshold: form.threshold,
        weights: {
          mention: form.weights.mention,
          keyword: form.weights.keyword,
          context: form.weights.context,
          quality: form.weights.quality,
          history: form.weights.history,
        },
      }),
      replyStrategyApi.update({
        strategy: 'always',
        relevance_threshold: 0.5,
        bot_name: '',
        strip_markdown: form.strip_markdown,
        agent_lite: form.agent_lite,
        skip_silence_check: form.skip_silence_check,
      }),
    ])
    toastStore.success('Planner 配置已保存')
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || e?.message || '保存失败')
  } finally { saving.value = false }
}

onMounted(() => { load() })
</script>
