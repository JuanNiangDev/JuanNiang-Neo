<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-message-text-clock-outline</v-icon>定时消息</div>
      <div class="page-subtitle">定时发送多段消息：每段支持文字 / 图片（T2I / URL / 图床）/ CQ 码表情，段间可自定义延迟</div>
    </div>

    <div class="d-flex justify-end mb-4">
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">新建定时任务</v-btn>
    </div>

    <v-data-table :headers="headers" :items="tasks" :loading="loading" :items-per-page="pageSize" @update:options="onPageChange">
      <template #item.name="{ item }">
        <div class="font-weight-medium">{{ item.name }}</div>
        <div class="text-caption text-medium-emphasis">{{ item.segments?.length || 0 }} 段消息</div>
      </template>
      <template #item.cron_expr="{ item }">
        <code class="text-caption">{{ item.cron_expr }}</code>
      </template>
      <template #item.target="{ item }">
        <span :class="item.target_type === 'group' ? '' : 'text-medium-emphasis'">
          {{ item.target_type === 'group' ? '群 ' : '私聊 ' }}{{ item.target_id }}
        </span>
      </template>
      <template #item.enabled="{ item }">
        <v-switch :model-value="item.enabled" color="primary" hide-details density="compact" @update:model-value="(v: any) => toggle(item, !!v)" />
      </template>
      <template #item.last_run_at="{ item }">
        <div class="text-caption">{{ item.last_run_at ? new Date(item.last_run_at).toLocaleString() : '从未执行' }}</div>
        <div v-if="item.last_error" class="text-caption text-error" style="max-width: 200px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis" :title="item.last_error">{{ item.last_error }}</div>
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-send" size="small" variant="text" color="success" title="立即发送" :loading="triggeringId === item.id" @click="trigger(item)" />
        <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" title="编辑" @click="openEdit(item)" />
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" title="删除" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <!-- 新建/编辑弹窗 -->
    <v-dialog v-model="dialog" max-width="860" scrollable>
      <v-card v-if="dialog" rounded="lg">
        <v-card-title>{{ editingId ? '编辑定时任务' : '新建定时任务' }}</v-card-title>
        <v-card-text>
          <v-row dense>
            <v-col cols="12" md="4">
              <v-text-field v-model="form.name" label="任务名称" class="mb-2" />
            </v-col>
            <v-col cols="12" md="3">
              <v-text-field v-model="timeStr" label="发送时间" type="time" @change="timeToCron" />
            </v-col>
            <v-col cols="12" md="5">
              <v-text-field v-model="form.cron_expr" label="cron 表达式（秒 分 时 日 月 周）" hint="改时间自动生成" persistent-hint />
            </v-col>
            <v-col cols="12" md="4">
              <v-select v-model="form.target_type" :items="[{ title: '群聊', value: 'group' }, { title: '私聊', value: 'private' }]" label="目标类型" class="mb-2" />
            </v-col>
            <v-col cols="12" md="4">
              <v-text-field v-model="targetIDText" :label="form.target_type === 'group' ? '目标群号' : '目标 QQ'" type="number" class="mb-2" />
            </v-col>
            <v-col cols="12" md="4" class="d-flex align-center">
              <v-switch v-model="form.enabled" label="启用任务" color="primary" hide-details />
            </v-col>
          </v-row>

          <!-- 消息段编辑器 -->
          <div class="d-flex align-center justify-space-between mt-2 mb-2">
            <span class="text-subtitle-2 font-weight-bold">消息段（按顺序发送）</span>
            <v-btn size="small" variant="tonal" prepend-icon="mdi-plus" @click="addSegment">添加段</v-btn>
          </div>

          <div v-for="(seg, idx) in form.segments" :key="idx" class="segment-card">
            <div class="d-flex align-center">
              <span class="text-caption text-medium-emphasis me-2" style="min-width: 30px">#{{ idx + 1 }}</span>
              <v-select
                v-model="seg.type"
                :items="segmentTypeOptions"
                item-title="title"
                item-value="value"
                label="类型"
                density="compact"
                class="me-2"
                style="max-width: 130px"
                hide-details
                @update:model-value="onSegmentTypeChange(seg)"
              />
              <v-text-field
                v-model.number="seg.delay_seconds"
                label="延迟（秒，到下一段）"
                type="number"
                density="compact"
                hide-details
                class="me-2"
                style="max-width: 150px"
              />
              <v-spacer />
              <v-btn icon="mdi-arrow-up" size="small" variant="text" :disabled="idx === 0" @click="moveSegment(idx, -1)" />
              <v-btn icon="mdi-arrow-down" size="small" variant="text" :disabled="idx === form.segments.length - 1" @click="moveSegment(idx, 1)" />
              <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="form.segments.splice(idx, 1)" />
            </div>
            <div class="mt-2">
              <!-- text -->
              <v-textarea
                v-if="seg.type === 'text'"
                v-model="seg.content"
                label="文字内容"
                rows="2"
                auto-grow
                hide-details
              />
              <!-- face：表情选择器 -->
              <div v-else-if="seg.type === 'face'">
                <div class="text-caption text-medium-emphasis mb-1">
                  CQ 码表情：{{ seg.content || '（未选择）' }}
                  <v-btn v-if="seg.content" size="x-small" variant="text" @click="seg.content = ''">清除</v-btn>
                </div>
                <div class="face-grid">
                  <div
                    v-for="f in faces"
                    :key="f.id"
                    class="face-item"
                    :class="{ 'face-selected': seg.content === `[CQ:face,id=${f.id}]` }"
                    :title="`id=${f.id}`"
                    @click="seg.content = `[CQ:face,id=${f.id}]`"
                  >
                    <img :src="f.url" :alt="f.id" />
                  </div>
                </div>
              </div>
              <!-- image -->
              <div v-else-if="seg.type === 'image'">
                <v-select
                  v-model="seg.source"
                  :items="imageSourceOptions"
                  item-title="title"
                  item-value="value"
                  label="图片来源"
                  density="compact"
                  hide-details
                  class="mb-2"
                  style="max-width: 220px"
                />
                <v-textarea
                  v-if="seg.source === 't2i'"
                  v-model="seg.content"
                  label="HTML 模板（T2I 渲染成图片）"
                  rows="3"
                  auto-grow
                  hide-details
                />
                <v-text-field
                  v-else-if="seg.source === 'url'"
                  v-model="seg.content"
                  label="图片 URL"
                  hide-details
                  placeholder="https://..."
                />
                <div v-else-if="seg.source === 'imgstore'">
                  <div class="d-flex align-center">
                    <span class="text-body-2 me-2">{{ seg.content || '未选择图床图片' }}</span>
                    <v-btn size="small" variant="tonal" prepend-icon="mdi-image-multiple-outline" @click="openPickerFor(idx)">选择图床图片</v-btn>
                    <v-btn v-if="seg.content" size="small" variant="text" @click="seg.content = ''">清除</v-btn>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-if="!form.segments.length" class="text-body-2 text-medium-emphasis pa-4 text-center">还没有消息段，点「添加段」开始</div>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="saving" @click="handleSave">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 图床图片选择器 -->
    <v-dialog v-model="pickerDialog" max-width="760">
      <v-card rounded="lg">
        <v-card-title>选择图床图片</v-card-title>
        <v-card-text>
          <div v-if="pickerImages.length" class="picker-grid">
            <v-card
              v-for="img in pickerImages"
              :key="img.id"
              rounded="lg"
              class="picker-card"
              :class="{ 'picker-selected': pickerSelectedId === img.id }"
              @click="pickerSelectedId = img.id"
            >
              <v-img :src="imageFileUrl(img.id)" :alt="img.name" height="80" cover />
              <div class="pa-1 picker-name">{{ img.name }}</div>
            </v-card>
          </div>
          <div v-else class="text-body-2 text-medium-emphasis pa-6 text-center">图床暂无图片，请先到「图床」页面上传</div>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="pickerDialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" :disabled="!pickerSelectedId" @click="confirmPicker">确定</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 删除确认 -->
    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>确认删除</v-card-title>
        <v-card-text>确定删除定时任务「{{ deleteTarget?.name }}」吗？此操作不可撤销。</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="deleteDialog = false">取消</v-btn>
          <v-btn color="error" variant="tonal" :loading="deleting" @click="handleDelete">删除</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { scheduledMessageApi, imageApi, imageFileUrl, type ScheduledMessageResp, type ScheduledSegment, type ImageResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()

// ---------- CQ 码表情资源（face 目录，文件名即 CQ 码 id；emoji-* 为 unicode 表情，除外） ----------
const faceModules = import.meta.glob('@/assets/face/*.avif', { eager: true }) as Record<string, { default: string }>
const faces = Object.entries(faceModules)
  .map(([path, mod]) => {
    const name = path.split('/').pop()!.replace(/\.avif$/, '')
    if (name.startsWith('emoji-')) return null
    return { id: name, url: mod.default }
  })
  .filter((f): f is { id: string; url: string } => !!f)
  .sort((a, b) => parseInt(a.id, 10) - parseInt(b.id, 10))

const segmentTypeOptions = [
  { title: '文字', value: 'text' },
  { title: '图片', value: 'image' },
  { title: 'CQ 码表情', value: 'face' },
]
const imageSourceOptions = [
  { title: 'T2I 生成', value: 't2i' },
  { title: '图片 URL', value: 'url' },
  { title: '图床图片', value: 'imgstore' },
]

const headers = [
  { title: '任务', key: 'name' },
  { title: 'cron', key: 'cron_expr' },
  { title: '目标', key: 'target' },
  { title: '启用', key: 'enabled', align: 'center' as const },
  { title: '上次执行', key: 'last_run_at' },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const loading = ref(false)
const tasks = ref<ScheduledMessageResp[]>([])
const page = ref(1)
const pageSize = 20
const total = ref(0)

const dialog = ref(false)
const editingId = ref<string | null>(null)
const form = ref({
  name: '',
  enabled: true,
  cron_expr: '0 0 9 * * *',
  target_type: 'group',
  target_id: 0,
  segments: [] as ScheduledSegment[],
})
const timeStr = ref('09:00')
const targetIDText = ref('')
const saving = ref(false)
const triggeringId = ref<string | null>(null)

// 图床选择器
const pickerDialog = ref(false)
const pickerImages = ref<ImageResp[]>([])
const pickerSelectedId = ref('')
const pickerTargetIdx = ref(-1)

const deleteDialog = ref(false)
const deleteTarget = ref<ScheduledMessageResp | null>(null)
const deleting = ref(false)

async function fetchTasks() {
  loading.value = true
  try {
    const res = (await scheduledMessageApi.list({ page: page.value, page_size: pageSize })).data.data
    tasks.value = res.list || []
    total.value = res.total || 0
  } catch (e: any) {
    toastStore.error(e?.message || '加载任务失败')
  } finally {
    loading.value = false
  }
}

function onPageChange(opts: any) {
  page.value = opts.page || 1
  fetchTasks()
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
}

function cronToTime(expr: string) {
  const parts = expr.trim().split(/\s+/)
  if (parts.length >= 3 && /^\d+$/.test(parts[1]) && /^\d+$/.test(parts[2])) {
    timeStr.value = `${parts[2].padStart(2, '0')}:${parts[1].padStart(2, '0')}`
  }
}

function emptySegment(): ScheduledSegment {
  return { type: 'text', content: '', delay_seconds: 0 }
}

function addSegment() {
  form.value.segments.push(emptySegment())
}

function onSegmentTypeChange(seg: ScheduledSegment) {
  seg.content = ''
  if (seg.type === 'image' && !seg.source) seg.source = 't2i'
}

function moveSegment(idx: number, delta: number) {
  const arr = form.value.segments
  const target = idx + delta
  if (target < 0 || target >= arr.length) return
  const tmp = arr[idx]
  arr[idx] = arr[target]
  arr[target] = tmp
}

async function loadPickerImages() {
  try {
    const res = (await imageApi.list({ page: 1, page_size: 100 })).data.data
    pickerImages.value = res.list || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载图床图片失败')
  }
}

function openPickerFor(idx: number) {
  pickerTargetIdx.value = idx
  pickerSelectedId.value = ''
  pickerDialog.value = true
  loadPickerImages()
}

function confirmPicker() {
  const img = pickerImages.value.find(i => i.id === pickerSelectedId.value)
  if (img && pickerTargetIdx.value >= 0) {
    form.value.segments[pickerTargetIdx.value].content = `imgs://${img.id}`
    pickerDialog.value = false
  }
}

function openCreate() {
  editingId.value = null
  form.value = {
    name: '', enabled: true, cron_expr: '0 0 9 * * *',
    target_type: 'group', target_id: 0,
    segments: [emptySegment()],
  }
  timeStr.value = '09:00'
  targetIDText.value = ''
  dialog.value = true
}

function openEdit(t: ScheduledMessageResp) {
  editingId.value = t.id
  form.value = {
    name: t.name,
    enabled: t.enabled,
    cron_expr: t.cron_expr,
    target_type: t.target_type,
    target_id: t.target_id,
    segments: t.segments.map(s => ({ ...s, delay_seconds: s.delay_seconds || 0 })),
  }
  targetIDText.value = String(t.target_id || '')
  cronToTime(t.cron_expr)
  dialog.value = true
}

async function handleSave() {
  const gid = parseInt(targetIDText.value, 10)
  if (!form.value.name.trim()) {
    toastStore.error('请填写任务名称')
    return
  }
  if (!gid) {
    toastStore.error('请填写目标群号 / QQ')
    return
  }
  const segments = form.value.segments.filter(s => s.type && s.content.trim())
  if (!segments.length) {
    toastStore.error('至少需要一个有内容的消息段')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.value.name.trim(),
      enabled: form.value.enabled,
      cron_expr: form.value.cron_expr.trim(),
      target_type: form.value.target_type,
      target_id: gid,
      segments,
    }
    if (editingId.value) {
      await scheduledMessageApi.update(editingId.value, payload)
      toastStore.success('已保存')
    } else {
      await scheduledMessageApi.create(payload)
      toastStore.success('已创建')
    }
    dialog.value = false
    await fetchTasks()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function toggle(t: ScheduledMessageResp, enabled: boolean) {
  try {
    await scheduledMessageApi.toggle(t.id, enabled)
    t.enabled = enabled
    toastStore.success(enabled ? '已启用' : '已停用')
  } catch (e: any) {
    toastStore.error(e?.message || '操作失败')
  }
}

async function trigger(t: ScheduledMessageResp) {
  triggeringId.value = t.id
  try {
    await scheduledMessageApi.trigger(t.id)
    toastStore.success('已触发发送')
    await fetchTasks()
  } catch (e: any) {
    toastStore.error(e?.message || '触发失败')
  } finally {
    triggeringId.value = null
  }
}

function confirmDelete(t: ScheduledMessageResp) {
  deleteTarget.value = t
  deleteDialog.value = true
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await scheduledMessageApi.remove(deleteTarget.value.id)
    toastStore.success('已删除')
    deleteDialog.value = false
    await fetchTasks()
  } catch (e: any) {
    toastStore.error(e?.message || '删除失败')
  } finally {
    deleting.value = false
  }
}

onMounted(fetchTasks)
</script>

<style scoped>
.segment-card {
  border: 1px solid rgba(128, 128, 128, 0.2);
  border-radius: 10px;
  padding: 12px;
  margin-bottom: 10px;
}
.face-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(44px, 1fr));
  gap: 6px;
  max-height: 180px;
  overflow-y: auto;
  padding: 4px;
}
.face-item {
  border: 2px solid transparent;
  border-radius: 8px;
  cursor: pointer;
  padding: 2px;
  text-align: center;
}
.face-item img {
  width: 36px;
  height: 36px;
  object-fit: contain;
}
.face-item:hover {
  border-color: rgba(128, 128, 128, 0.4);
}
.face-item.face-selected {
  border-color: var(--v-theme-primary);
  background: rgba(var(--v-theme-primary), 0.08);
}
.picker-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
  gap: 10px;
  max-height: 380px;
  overflow-y: auto;
}
.picker-card {
  cursor: pointer;
  overflow: hidden;
  border: 2px solid transparent;
}
.picker-card.picker-selected {
  border-color: var(--v-theme-primary);
}
.picker-name {
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
