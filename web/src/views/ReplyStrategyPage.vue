<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-reply-circle</v-icon>系统回复策略</div>
      <div class="page-subtitle">控制机器人在群聊和私聊中的回复行为</div>
    </div>

    <v-row>
      <v-col cols="12" md="8">
        <v-card rounded="lg" elevation="1">
          <v-card-item>
            <template #title><span class="text-h6 font-weight-bold">回复策略</span></template>
          </v-card-item>
          <v-card-text>
            <v-form @submit.prevent="handleSave" ref="formRef">
              <v-radio-group v-model="form.strategy" class="mb-4">
                <v-radio label="完全不回复 — 不处理任何消息" value="never_reply" color="error" />
                <v-radio label="仅@我时回复 — 只有被@时才交给 Plugin 和 Agent" value="at_only" color="warning" />
                <v-radio label="完全回复 — 正常处理所有消息（默认）" value="always" color="success" />
                <v-radio label="仅 Plugin — 只给 Plugin 处理，不给 Agent" value="plugin_only" color="info" />
                <v-radio label="按相关性回复 — 由 Agent 判断消息是否相关后再回复" value="relevance" color="primary" />
              </v-radio-group>

              <v-expand-transition>
                <v-card v-if="form.strategy === 'relevance'" variant="outlined" class="mb-4 pa-3" rounded="lg">
                  <div class="text-subtitle-2 font-weight-bold mb-2">相关性阈值</div>
                  <div class="d-flex align-center" style="gap: 16px">
                    <v-slider
                      v-model="form.relevance_threshold"
                      :min="0"
                      :max="1"
                      :step="0.05"
                      color="primary"
                      thumb-label="always"
                      hide-details
                      style="flex: 1"
                    />
                    <v-text-field
                      v-model.number="form.relevance_threshold"
                      type="number"
                      :min="0"
                      :max="1"
                      :step="0.05"
                      density="compact"
                      style="width: 90px"
                      hide-details
                      variant="outlined"
                    />
                  </div>
                  <div class="text-caption text-medium-emphasis mt-2">
                    Agent 判断消息相关性 &ge; 阈值时才会回复。被@时自动绕过此判断。
                    <br />当前阈值: {{ form.relevance_threshold.toFixed(2) }}
                  </div>
                </v-card>
              </v-expand-transition>

              <v-btn type="submit" color="primary" variant="tonal" :loading="saving" :disabled="!form.strategy">
                <v-icon class="me-1">mdi-content-save</v-icon> 保存策略
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
})

async function load() {
  try {
    const res = await replyStrategyApi.get()
    const d = (res.data as any)?.data
    if (d) {
      form.value.strategy = d.strategy || 'always'
      form.value.relevance_threshold = d.relevance_threshold ?? 0.5
    }
  } catch (_e: any) {}
}

async function handleSave() {
  saving.value = true
  try {
    await replyStrategyApi.update({
      strategy: form.value.strategy,
      relevance_threshold: form.value.relevance_threshold,
    })
    toastStore.success('回复策略已保存')
  } catch (e: any) {
    toastStore.error(e?.response?.data?.info || e?.message || '保存失败')
  } finally { saving.value = false }
}

onMounted(() => { load() })
</script>
