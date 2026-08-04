<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-calendar-month-outline</v-icon>摸鱼人日历</div>
      <div class="page-subtitle">每天定时生成日历图片（农历 / 本周进度 / 法定假日倒计时 / 金句 / 群务）发送到多个群</div>
    </div>

    <v-row>
      <!-- 运行状态 -->
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
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">目标群</span><span>{{ form.target_groups.length ? `${form.target_groups.length} 个群` : '未配置' }}</span></div>
              <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">上次执行</span><span>{{ lastRunText }}</span></div>
              <div class="py-1">
                <div class="text-medium-emphasis mb-1">上次错误</div>
                <div class="text-caption text-error" style="white-space: pre-wrap">{{ cfg.last_error || '无' }}</div>
              </div>
            </div>
            <v-alert v-if="!cfg.enabled" type="info" density="compact" variant="tonal" class="mt-3">
              未启用时不会定时发送，可先用「立即发送」测试效果。
            </v-alert>
            <v-btn color="success" variant="tonal" prepend-icon="mdi-send" :loading="triggering" class="mt-3" @click="handleTrigger">立即发送（测试）</v-btn>
          </v-card-text>
        </v-card>
      </v-col>

      <!-- 发送配置 -->
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
                <v-combobox
                  v-model="form.target_groups"
                  label="目标群号（回车添加多个群）"
                  placeholder="输入群号后回车，如 1076723599"
                  multiple
                  chips
                  hide-selected
                  @update:model-value="markDirty"
                />
              </v-col>
            </v-row>
            <div class="text-caption text-medium-emphasis">
              发送内容为富文本：@全体成员 +「今日份摸鱼人日历来了~」+ 日历图片（800×720，黑白纸张风格）。
            </div>
          </v-card-text>
          <v-card-actions class="pa-4 pt-0">
            <v-btn color="primary" variant="tonal" prepend-icon="mdi-content-save" :loading="saving" @click="handleSave">保存配置</v-btn>
            <v-spacer />
            <v-btn variant="text" prepend-icon="mdi-refresh" @click="load">刷新</v-btn>
          </v-card-actions>
        </v-card>

        <!-- 群务日历配置 -->
        <v-card rounded="lg" class="mt-4">
          <v-card-title class="py-3 d-flex align-center">
            <v-icon class="me-2" color="primary">mdi-calendar-edit</v-icon>群务配置（按天）
            <v-spacer />
            <v-btn icon="mdi-chevron-left" variant="text" size="small" @click="shiftMonth(-1)" />
            <span class="text-subtitle-1 mx-2" style="min-width: 96px; text-align: center">{{ viewYear }}年{{ viewMonth }}月</span>
            <v-btn icon="mdi-chevron-right" variant="text" size="small" @click="shiftMonth(1)" />
          </v-card-title>
          <v-card-text>
            <div class="cal-week">
              <span v-for="w in weekNames" :key="w" class="cal-week-cell">{{ w }}</span>
            </div>
            <div class="cal-grid">
              <div
                v-for="(cell, idx) in grid"
                :key="idx"
                class="cal-cell"
                :class="{ 'cal-cell-out': !cell.inMonth, 'cal-cell-today': cell.isToday, 'cal-cell-has': cell.content }"
                @click="openAffairEditor(cell)"
              >
                <div class="cal-day">{{ cell.day }}</div>
                <div class="cal-content" :title="cell.content">{{ cell.content || (cell.inMonth ? '＋' : '') }}</div>
              </div>
            </div>
            <div class="text-caption text-medium-emphasis mt-2">点击日期格子可设置 / 修改当天群务；已配置的日期会显示内容。</div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- 群务编辑弹窗 -->
    <v-dialog v-model="affairDialog" max-width="460">
      <v-card rounded="lg">
        <v-card-title>设置 {{ editingDate }} 群务</v-card-title>
        <v-card-text>
          <v-textarea v-model="affairContent" label="当天群务内容（留空可清除）" rows="4" auto-grow />
          <div class="text-caption text-medium-emphasis">会渲染在日历图片的「今日群务」区域；未配置的日期显示默认文案。</div>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" color="error" v-if="affairHasContent" @click="clearAffair">清除配置</v-btn>
          <v-btn variant="text" @click="affairDialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="savingAffair" @click="saveAffair">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
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
  target_groups: [],
  last_error: '',
})
const form = ref({ enabled: false, cron_expr: '0 30 9 * * *', target_groups: [] as string[] })
const timeStr = ref('09:30')
const dirty = ref(false)

const saving = ref(false)
const triggering = ref(false)

const lastRunText = computed(() => cfg.value.last_run_at ? new Date(cfg.value.last_run_at).toLocaleString() : '从未执行')

// ---------- 群务日历 ----------
const weekNames = ['一', '二', '三', '四', '五', '六', '日']
const viewYear = ref(new Date().getFullYear())
const viewMonth = ref(new Date().getMonth() + 1)
const affairsMap = ref<Record<string, string>>({})
const affairDialog = ref(false)
const editingDate = ref('')
const affairContent = ref('')
const affairHasContent = ref(false)
const savingAffair = ref(false)

interface CalCell {
  date: string
  day: number
  inMonth: boolean
  isToday: boolean
  content: string
}

const grid = computed<CalCell[]>(() => {
  const year = viewYear.value
  const month = viewMonth.value
  const daysInMonth = new Date(year, month, 0).getDate()
  const firstDay = new Date(year, month - 1, 1).getDay() // 0=周日
  const leading = (firstDay + 6) % 7 // 周一开头
  const todayStr = new Date().toLocaleDateString('sv') // YYYY-MM-DD
  const cells: CalCell[] = []
  // 前导空白（上月）
  const prevDays = new Date(year, month - 1, 0).getDate()
  for (let i = leading - 1; i >= 0; i--) {
    const d = prevDays - i
    const date = fmtDate(new Date(year, month - 2, d))
    cells.push({ date, day: d, inMonth: false, isToday: date === todayStr, content: affairsMap.value[date] || '' })
  }
  for (let d = 1; d <= daysInMonth; d++) {
    const date = fmtDate(new Date(year, month - 1, d))
    cells.push({ date, day: d, inMonth: true, isToday: date === todayStr, content: affairsMap.value[date] || '' })
  }
  // 补齐到整周
  while (cells.length % 7 !== 0) {
    const last = cells[cells.length - 1]
    const d = Number(last.day) + 1
    const date = fmtDate(new Date(viewYear.value, viewMonth.value - 1, d))
    cells.push({ date, day: d, inMonth: false, isToday: false, content: '' })
  }
  return cells
})

function fmtDate(d: Date) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

async function loadAffairs() {
  const month = `${viewYear.value}-${String(viewMonth.value).padStart(2, '0')}`
  try {
    const res = (await fishCalendarApi.affairs(month)).data.data
    const map: Record<string, string> = {}
    for (const a of res || []) map[a.date] = a.content
    affairsMap.value = map
  } catch (e: any) {
    toastStore.error(e?.message || '加载群务失败')
  }
}

function shiftMonth(delta: number) {
  const d = new Date(viewYear.value, viewMonth.value - 1 + delta, 1)
  viewYear.value = d.getFullYear()
  viewMonth.value = d.getMonth() + 1
  loadAffairs()
}

function openAffairEditor(cell: CalCell) {
  if (!cell.inMonth) return
  editingDate.value = cell.date
  affairContent.value = cell.content
  affairHasContent.value = !!cell.content
  affairDialog.value = true
}

async function saveAffair() {
  savingAffair.value = true
  try {
    await fishCalendarApi.setAffair(editingDate.value, affairContent.value.trim())
    toastStore.success(affairContent.value.trim() ? '群务已设置' : '已清除当天群务')
    affairDialog.value = false
    await loadAffairs()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    savingAffair.value = false
  }
}

async function clearAffair() {
  affairContent.value = ''
  await saveAffair()
}

// ---------- 配置 ----------

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

function timeToCron() {
  const m = timeStr.value.match(/^(\d{1,2}):(\d{2})$/)
  if (!m) return
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
      target_groups: [...(res.target_groups || [])],
    }
    cronToTime(res.cron_expr)
    dirty.value = false
  } catch (e: any) {
    toastStore.error(e?.message || '加载配置失败')
  }
}

async function handleSave() {
  const groups = form.value.target_groups
    .map(g => String(g).trim())
    .filter(g => /^\d+$/.test(g))
  if (!groups.length) {
    toastStore.error('请至少填写一个目标群号')
    return
  }
  saving.value = true
  try {
    await fishCalendarApi.update({
      enabled: form.value.enabled,
      cron_expr: form.value.cron_expr.trim(),
      target_groups: groups,
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

onMounted(() => {
  load()
  loadAffairs()
})
</script>

<style scoped>
.cal-week {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 6px;
  margin-bottom: 6px;
}
.cal-week-cell {
  text-align: center;
  font-size: 12px;
  color: rgba(128, 128, 128, 0.8);
  padding: 4px 0;
}
.cal-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 6px;
}
.cal-cell {
  min-height: 64px;
  border: 1px solid rgba(128, 128, 128, 0.18);
  border-radius: 8px;
  padding: 4px 6px;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.cal-cell:hover {
  border-color: var(--v-theme-primary);
  background: rgba(var(--v-theme-primary), 0.06);
}
.cal-cell-out {
  opacity: 0.35;
}
.cal-cell-today {
  border-color: var(--v-theme-primary);
  background: rgba(var(--v-theme-primary), 0.08);
}
.cal-cell-has {
  border-color: rgba(var(--v-theme-primary), 0.6);
}
.cal-day {
  font-size: 12px;
  font-weight: 600;
  margin-bottom: 2px;
}
.cal-content {
  font-size: 11px;
  line-height: 1.3;
  color: rgba(128, 128, 128, 0.85);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  word-break: break-all;
}
</style>
