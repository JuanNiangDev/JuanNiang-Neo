<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-calendar-month-outline</v-icon>摸鱼人日历</div>
      <div class="page-subtitle">每天定时生成日历图片（农历 / 本周进度 / 法定假日倒计时 / 金句 / 群务）发送到群</div>
    </div>

    <v-row>
      <!-- 状态卡片 -->
      <v-col cols="12" md="4">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-power</v-icon>运行状态</v-card-title>
          <v-card-text>
            <div class="d-flex align-center justify-space-between mb-4">
              <span class="text-body-1">启用摸鱼人日历</span>
              <v-switch v-model="form.enabled" color="primary" hide-details @change="markDirty" />
            </div>
            <div class="meta-list text-body-2">
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">发送时间</span><span>{{ form.cron_expr }}</span></div>
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">上次执行</span><span>{{ lastRunText }}</span></div>
              <div class="py-1">
                <div class="text-medium-emphasis mb-1">上次错误</div>
                <div class="text-caption text-error" style="white-space: pre-wrap">{{ cfg.last_error || '无' }}</div>
              </div>
            </div>
            <v-alert v-if="!cfg.enabled" type="info" density="compact" variant="tonal" class="mt-3">
              未启用时不会定时发送，可先用下方「立即发送」测试效果。
            </v-alert>
          </v-card-text>
        </v-card>
      </v-col>

      <!-- 配置卡片 -->
      <v-col cols="12" md="8">
        <v-card rounded="lg">
          <v-card-title class="py-3"><v-icon class="me-2" color="primary">mdi-tune-variant</v-icon>发送配置</v-card-title>
          <v-card-text>
            <v-row dense>
              <v-col cols="12" md="4">
                <v-text-field v-model="timeStr" label="每天发送时间" type="time" @change="timeToCron" />
              </v-col>
              <v-col cols="12" md="8">
                <v-text-field
                  v-model="form.cron_expr"
                  label="cron 表达式（6 字段秒级，如 0 30 9 * * *）"
                  hint="秒 分 时 日 月 周；修改时间后自动生成"
                  persistent-hint
                  @update:model-value="markDirty"
                />
              </v-col>
              <v-col cols="12">
                <v-text-field v-model="groupIDText" label="目标群号" type="number" placeholder="例如 1076723599" @update:model-value="markDirty" />
              </v-col>
              <v-col cols="12">
                <v-textarea v-model="form.group_affairs" label="今日群务内容" rows="2" hint="会渲染在日历图片的「今日群务」区域" persistent-hint @update:model-value="markDirty" />
              </v-col>
            </v-row>
            <div class="text-caption text-medium-emphasis mt-2">
              日历图片内容：标题 / 今日宜划水·忌内卷 / 日期与星期 / 农历 / 本周进度 / 距下一个法定假日倒计时 / 今日金句（一言 API）/ 今日群务 / 落款。
            </div>
          </v-card-text>
          <v-card-actions class="pa-4 pt-0">
            <v-btn color="primary" variant="tonal" prepend-icon="mdi-content-save" :loading="saving" @click="handleSave">保存配置</v-btn>
            <v-btn color="success" variant="tonal" prepend-icon="mdi-send" :loading="triggering" @click="handleTrigger">立即发送（测试）</v-btn>
            <v-spacer />
            <v-btn variant="text" prepend-icon="mdi-refresh" @click="load">刷新</v-btn>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { fishCalendarApi, type FishCalendarConfigResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()

const cfg = ref<FishCalendarConfigResp>({
  enabled: false,
  cron_expr: '0 30 9 * * *',
  target_group_id: 0,
  group_affairs: '',
  last_error: '',
})
const form = ref({ enabled: false, cron_expr: '0 30 9 * * *', target_group_id: 0, group_affairs: '' })
const timeStr = ref('09:30')
const groupIDText = ref('')
const dirty = ref(false)

const saving = ref(false)
const triggering = ref(false)

const lastRunText = computed(() => cfg.value.last_run_at ? new Date(cfg.value.last_run_at).toLocaleString() : '从未执行')

// cron "0 30 9 * * *" → "09:30"
function cronToTime(cronExpr: string) {
  const parts = cronExpr.trim().split(/\s+/)
  if (parts.length >= 3) {
    const h = parts[2].padStart(2, '0')
    const m = parts[1].padStart(2, '0')
    if (/^\d+$/.test(h) && /^\d+$/.test(m)) {
      timeStr.value = `${h}:${m}`
    }
  }
}

// "09:30" → cron "0 30 9 * * *"
function timeToCron() {
  const m = timeStr.value.match(/^(\d{1,2}):(\d{2})$/)
  if (!m) {
    toastStore.error('时间格式无效')
    return
  }
  const h = parseInt(m[1], 10)
  const min = parseInt(m[2], 10)
  if (h > 23 || min > 59) {
    toastStore.error('时间超出范围')
    return
  }
  form.value.cron_expr = `0 ${min} ${h} * * *`
  markDirty()
}

function markDirty() {
  dirty.value = true
}

async function load() {
  try {
    const res = (await fishCalendarApi.get()).data.data
    cfg.value = res
    form.value = {
      enabled: res.enabled,
      cron_expr: res.cron_expr,
      target_group_id: res.target_group_id,
      group_affairs: res.group_affairs,
    }
    groupIDText.value = res.target_group_id ? String(res.target_group_id) : ''
    cronToTime(res.cron_expr)
    dirty.value = false
  } catch (e: any) {
    toastStore.error(e?.message || '加载配置失败')
  }
}

async function handleSave() {
  const gid = parseInt(groupIDText.value, 10)
  if (!gid) {
    toastStore.error('请填写目标群号')
    return
  }
  saving.value = true
  try {
    await fishCalendarApi.update({
      enabled: form.value.enabled,
      cron_expr: form.value.cron_expr.trim(),
      target_group_id: gid,
      group_affairs: form.value.group_affairs,
    })
    toastStore.success('配置已保存' + (form.value.enabled ? '，调度已更新' : ''))
    dirty.value = false
    await load()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function handleTrigger() {
  triggering.value = true
  try {
    await fishCalendarApi.trigger()
    toastStore.success('已触发发送，稍后查看群消息')
    await load()
  } catch (e: any) {
    toastStore.error(e?.message || '触发失败')
  } finally {
    triggering.value = false
  }
}

onMounted(load)
</script>
