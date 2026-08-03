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
              <v-radio-group v-model="form.strategy" class="mb-4">
                <v-radio label="完全不回复 — 不处理任何消息" value="never_reply" color="error" />
                <v-radio label="仅@我时回复 — 只有被@时才交给 Plugin 和 Agent" value="at_only" color="warning" />
                <v-radio label="完全回复 — 正常处理所有消息（默认）" value="always" color="success" />
                <v-radio label="按相关性回复 — 由 Agent 判断消息是否相关后再回复" value="relevance" color="primary" />
              </v-radio-group>

              <v-expand-transition>
                <v-card v-if="form.strategy === 'relevance'" variant="outlined" class="mb-4 pa-3" rounded="lg">
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
                </v-card>
              </v-expand-transition>

              <v-btn type="submit" color="primary" variant="tonal" :loading="saving" :disabled="!form.strategy">
                <v-icon class="me-1">mdi-content-save</v-icon> 保存
              </v-btn>
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

              <v-btn type="submit" color="primary" variant="tonal" :loading="saving">
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
import { ref, onMounted } from 'vue'
import { useToastStore } from '@/stores/toast'
import { replyStrategyApi } from '@/api'

const toastStore = useToastStore()
const saving = ref(false)

const form = ref({
  strategy: 'always',
  relevance_threshold: 0.5,
  bot_name: '',
  strip_markdown: false,
  agent_lite: false,
})

async function load() {
  try {
    const res = await replyStrategyApi.get()
    const d = (res.data as any)?.data
    if (d) {
      form.value.strategy = d.strategy || 'always'
      form.value.relevance_threshold = d.relevance_threshold ?? 0.5
      form.value.bot_name = d.bot_name || ''
      form.value.strip_markdown = d.strip_markdown || false
      form.value.agent_lite = d.agent_lite || false
    }
  } catch (_e: any) {}
}

async function handleSave() {
  saving.value = true
  try {
    await replyStrategyApi.update({
      strategy: form.value.strategy,
      relevance_threshold: form.value.relevance_threshold,
      bot_name: form.value.bot_name,
      strip_markdown: form.value.strip_markdown,
      agent_lite: form.value.agent_lite,
    })
    toastStore.success('回复设置已保存')
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || e?.message || '保存失败')
  } finally { saving.value = false }
}

onMounted(() => { load() })
</script>
