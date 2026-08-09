<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-message-text-clock-outline</v-icon>定时消息</div>
      <div class="page-subtitle">积木式编排：触发器 → 消息块（一条消息多段）→ 延时块，按序执行</div>
    </div>

    <div class="d-flex justify-end mb-4">
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">新建定时任务</v-btn>
    </div>

    <v-data-table :headers="headers" :items="tasks" :loading="loading" :page="page" :items-per-page="pageSize" :items-length="total" @update:options="onPageChange">
      <template #item.name="{ item }">
        <div class="font-weight-medium">{{ item.name }}</div>
        <div class="text-caption text-medium-emphasis">{{ (item.blocks?.length || 0) }} 个编排块</div>
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

    <!-- 新建/编辑弹窗：积木式编排 -->
    <v-dialog v-model="dialog" max-width="900" scrollable>
      <v-card v-if="dialog" rounded="lg">
        <v-card-title>{{ editingId ? '编辑定时任务' : '新建定时任务' }}</v-card-title>
        <v-card-text>
          <v-row dense class="mb-2">
            <v-col cols="12" md="4">
              <v-text-field v-model="form.name" label="任务名称" class="mb-2" />
            </v-col>
            <v-col cols="12" md="4">
              <v-select v-model="form.target_type" :items="[{ title: '群聊', value: 'group' }, { title: '私聊', value: 'private' }]" label="目标类型" class="mb-2" />
            </v-col>
            <v-col cols="12" md="4">
              <v-text-field v-model="targetIDText" :label="form.target_type === 'group' ? '目标群号' : '目标 QQ'" type="number" class="mb-2" />
            </v-col>
            <v-col cols="12" class="d-flex align-center">
              <v-switch v-model="form.enabled" label="启用任务" color="primary" hide-details />
            </v-col>
          </v-row>

          <!-- 积木编排 -->
          <div class="orchestration">
            <!-- 触发器块 -->
            <div class="trigger-block">
              <div class="block-header">
                <v-icon size="18" class="me-1">mdi-flash</v-icon>
                <span class="font-weight-bold">触发器</span>
                <span class="text-caption text-medium-emphasis ms-2">任务从这里开始</span>
                <v-spacer />
                <v-text-field v-model="timeStr" label="发送时间" type="time" density="compact" hide-details style="max-width: 150px" class="me-2" @change="timeToCron" />
                <v-text-field v-model="form.cron_expr" label="cron（秒 分 时 日 月 周）" density="compact" hide-details style="max-width: 220px" />
              </div>
            </div>

            <template v-for="(block, bi) in form.blocks" :key="bi">
              <!-- 连接线 -->
              <div class="block-connector"><v-icon size="22">mdi-arrow-down</v-icon></div>

              <!-- 消息块 -->
              <div v-if="block.type === 'message'" class="block message-block">
                <div class="block-header">
                  <v-icon size="18" class="me-1" color="primary">mdi-message-text</v-icon>
                  <span class="font-weight-bold">消息块</span>
                  <span class="text-caption text-medium-emphasis ms-2">一条消息（含 {{ block.segments?.length || 0 }} 段）</span>
                  <v-spacer />
                  <v-btn icon="mdi-eye-outline" size="x-small" variant="text" color="info" title="预览渲染效果" @click="togglePreview(bi)" />
                  <v-btn size="x-small" variant="tonal" prepend-icon="mdi-plus" @click="addSegment(bi)">加段</v-btn>
                  <v-btn icon="mdi-arrow-up" size="x-small" variant="text" :disabled="bi === 0" @click="moveBlock(bi, -1)" />
                  <v-btn icon="mdi-arrow-down" size="x-small" variant="text" :disabled="bi === form.blocks.length - 1" @click="moveBlock(bi, 1)" />
                  <v-btn icon="mdi-delete" size="x-small" variant="text" color="error" @click="form.blocks.splice(bi, 1)" />
                </div>
                <div v-if="!block.segments?.length" class="text-caption text-medium-emphasis pa-2 text-center">（空消息块，点击「加段」添加内容）</div>
                <div v-for="(seg, si) in block.segments" :key="si" class="segment-row">
                  <span class="text-caption text-medium-emphasis me-1" style="min-width: 22px">{{ si + 1 }}</span>
                  <v-select
                    v-model="seg.type"
                    :items="segmentTypeOptions"
                    item-title="title"
                    item-value="value"
                    density="compact"
                    hide-details
                    style="max-width: 120px"
                    class="me-2"
                    @update:model-value="() => onSegmentTypeChange(seg)"
                  />
                  <!-- text -->
                  <v-text-field v-if="seg.type === 'text'" v-model="seg.content" label="文字" density="compact" hide-details class="flex-grow-1" />
                  <!-- face -->
                  <template v-else-if="seg.type === 'face'">
                    <v-btn size="small" variant="tonal" prepend-icon="mdi-emoticon-outline" class="me-2" @click="facePickerFor = { bi, si }">
                      {{ seg.content ? `表情 ${seg.content.match(/id=(\d+)/)?.[1] || ''}` : '选择表情' }}
                    </v-btn>
                    <span v-if="seg.content" class="text-caption">{{ seg.content }}</span>
                  </template>
                  <!-- image -->
                  <template v-else-if="seg.type === 'image'">
                    <v-select
                      v-model="seg.source"
                      :items="imageSourceOptions"
                      item-title="title"
                      item-value="value"
                      label="来源"
                      density="compact"
                      hide-details
                      style="max-width: 140px"
                      class="me-2"
                      @update:model-value="() => onImageSourceChange(seg)"
                    />
                    <v-text-field v-if="seg.source === 'url'" v-model="seg.content" label="图片 URL" density="compact" hide-details class="flex-grow-1" />
                    <v-textarea v-else-if="seg.source === 't2i'" v-model="seg.content" label="HTML 模板（T2I 渲染）" density="compact" hide-details rows="1" auto-grow class="flex-grow-1" />
                    <template v-else-if="seg.source === 'imgstore'">
                      <span class="text-caption me-2 text-truncate" style="max-width: 240px">{{ seg.content || '未选择图床图片' }}</span>
                      <v-btn size="small" variant="tonal" prepend-icon="mdi-image-multiple-outline" @click="openPicker(bi, si)">选择图床图片</v-btn>
                    </template>
                  </template>
                  <v-btn icon="mdi-delete" size="x-small" variant="text" color="error" class="ms-1" @click="block.segments?.splice(si, 1)" />
                </div>
                <!-- 渲染预览面板：按发送时「一段一行」规则展示效果 -->
                <div v-if="previewBlockIdx === bi" class="preview-panel">
                  <div class="preview-header">
                    <span class="text-caption text-medium-emphasis">渲染预览（每段独占一行）</span>
                    <v-btn icon="mdi-close" size="x-small" variant="text" title="关闭预览" @click="previewBlockIdx = null" />
                  </div>
                  <div class="preview-content">
                    <template v-for="(seg, si) in block.segments" :key="`p${si}`">
                      <div v-if="seg.type === 'text' && seg.content" class="preview-line preview-text">{{ seg.content }}</div>
                      <div v-else-if="seg.type === 'face' && seg.content" class="preview-line preview-face">
                        <img v-if="faceUrl(seg.content)" :src="faceUrl(seg.content) || ''" :alt="seg.content" />
                        <span v-else class="text-caption text-error">未知表情: {{ seg.content }}</span>
                      </div>
                      <div v-else-if="seg.type === 'image'" class="preview-line preview-image">
                        <img v-if="seg.source === 'url' && seg.content" :src="seg.content" />
                        <img v-else-if="seg.source === 'imgstore' && seg.content" :src="imageFileUrl(extractImgId(seg.content))" />
                        <iframe v-else-if="seg.source === 't2i' && seg.content" :srcdoc="seg.content" class="preview-t2i-frame" sandbox="allow-scripts" />
                        <span v-else class="text-caption text-medium-emphasis">（空图片段）</span>
                      </div>
                    </template>
                    <div v-if="!block.segments?.length" class="text-caption text-medium-emphasis">（空消息块）</div>
                  </div>
                </div>
              </div>

              <!-- 延时块 -->
              <div v-else-if="block.type === 'delay'" class="block delay-block">
                <div class="block-header">
                  <v-icon size="18" class="me-1" color="warning">mdi-timer</v-icon>
                  <span class="font-weight-bold">延时块</span>
                  <v-spacer />
                  <v-text-field v-model.number="block.delay_seconds" label="延时（秒）" type="number" density="compact" hide-details style="max-width: 140px" class="me-2" />
                  <v-btn icon="mdi-arrow-up" size="x-small" variant="text" :disabled="bi === 0" @click="moveBlock(bi, -1)" />
                  <v-btn icon="mdi-arrow-down" size="x-small" variant="text" :disabled="bi === form.blocks.length - 1" @click="moveBlock(bi, 1)" />
                  <v-btn icon="mdi-delete" size="x-small" variant="text" color="error" @click="form.blocks.splice(bi, 1)" />
                </div>
                <div class="text-caption text-medium-emphasis pa-2">等待 {{ block.delay_seconds || 0 }} 秒后继续下一个块</div>
              </div>

              <!-- 块后添加按钮 -->
              <div class="add-block">
                <v-btn size="x-small" variant="tonal" prepend-icon="mdi-message-text" class="me-1" @click="insertBlock(bi + 1, 'message')">消息块</v-btn>
                <v-btn size="x-small" variant="tonal" prepend-icon="mdi-timer" @click="insertBlock(bi + 1, 'delay')">延时块</v-btn>
              </div>
            </template>

            <template v-if="!form.blocks.length">
              <div class="block-connector"><v-icon size="22">mdi-arrow-down</v-icon></div>
              <div class="add-block">
                <v-btn size="small" variant="tonal" prepend-icon="mdi-message-text" class="me-1" @click="insertBlock(0, 'message')">添加消息块</v-btn>
                <v-btn size="small" variant="tonal" prepend-icon="mdi-timer" @click="insertBlock(0, 'delay')">添加延时块</v-btn>
              </div>
            </template>
          </div>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="saving" @click="handleSave">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 表情选择器弹窗 -->
    <v-dialog v-model="facePickerOpen" max-width="640">
      <v-card rounded="lg">
        <v-card-title>选择 CQ 码表情</v-card-title>
        <v-card-text>
          <div class="face-grid">
            <div
              v-for="f in faces"
              :key="f.id"
              class="face-item"
              :title="`id=${f.id}`"
              @click="pickFace(f.id)"
            >
              <img :src="f.url" :alt="f.id" />
            </div>
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>

    <!-- 图床图片选择器 -->
    <v-dialog v-model="pickerOpen" max-width="760">
      <v-card rounded="lg">
        <v-card-title>选择图床图片</v-card-title>
        <v-card-text>
          <v-select
            v-model="pickerFolder"
            :items="pickerFolderOptions"
            item-title="title"
            item-value="value"
            label="文件夹"
            density="compact"
            hide-details
            class="mb-3"
            @update:model-value="loadPickerImages"
          />
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
          <v-btn variant="text" @click="pickerOpen = false">取消</v-btn>
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
import { scheduledMessageApi, imageApi, imageFolderApi, imageFileUrl, type ScheduledMessageResp, type ScheduledBlock, type ScheduledSegment, type ImageResp, type ImageFolderResp } from '@/api'
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
  { title: '触发器 cron', key: 'cron_expr' },
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
  blocks: [] as ScheduledBlock[],
})
const timeStr = ref('09:00')
const targetIDText = ref('')
const saving = ref(false)
const triggeringId = ref<string | null>(null)

// 表情选择器
const facePickerFor = ref<{ bi: number; si: number } | null>(null)
const facePickerOpen = computed({
  get: () => !!facePickerFor.value,
  set: (v: boolean) => { if (!v) facePickerFor.value = null },
})

// 图床选择器
const pickerTarget = ref<{ bi: number; si: number } | null>(null)
const pickerOpen = computed({
  get: () => !!pickerTarget.value,
  set: (v: boolean) => { if (!v) pickerTarget.value = null },
})
const pickerImages = ref<ImageResp[]>([])
const pickerFolders = ref<ImageFolderResp[]>([])
const pickerFolder = ref('/')
const pickerSelectedId = ref('')

const pickerFolderOptions = computed(() => [
  { title: '根目录 /', value: '/' },
  ...pickerFolders.value.map(f => ({ title: `文件夹 ${f.name}`, value: '/' + f.name })),
])

const deleteDialog = ref(false)
const deleteTarget = ref<ScheduledMessageResp | null>(null)
const deleting = ref(false)

// 渲染预览：哪个 block 的预览面板打开了（null=全部关闭）
const previewBlockIdx = ref<number | null>(null)
function togglePreview(bi: number) {
  previewBlockIdx.value = previewBlockIdx.value === bi ? null : bi
}

// 从 [CQ:face,id=66] 提取 66，返回本地对应的 face avif 资源 URL
function faceUrl(content: string): string | null {
  const m = content.match(/id=(\d+)/)
  if (!m) return null
  const face = faces.find(f => f.id === m![1])
  return face?.url || null
}

// 从 imgs://abc 提取图片 id（用于 imageFileUrl 拼接预览 URL）
function extractImgId(content: string): string {
  const m = content.match(/^imgs:\/\/(.+)$/)
  return m ? m[1] : content
}

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
  return { type: 'text', content: '' }
}

function emptyBlock(type: string): ScheduledBlock {
  if (type === 'delay') return { type: 'delay', delay_seconds: 60 }
  return { type: 'message', segments: [emptySegment()] }
}

function insertBlock(index: number, type: string) {
  form.value.blocks.splice(index, 0, emptyBlock(type))
}

function addSegment(bi: number) {
  const block = form.value.blocks[bi]
  if (block.type === 'message') {
    if (!block.segments) block.segments = []
    block.segments.push(emptySegment())
  }
}

function onSegmentTypeChange(seg: ScheduledSegment) {
  seg.content = ''
  if (seg.type === 'image' && !seg.source) seg.source = 't2i'
}

function onImageSourceChange(seg: ScheduledSegment) {
  seg.content = ''
}

function moveBlock(idx: number, delta: number) {
  const arr = form.value.blocks
  const target = idx + delta
  if (target < 0 || target >= arr.length) return
  const tmp = arr[idx]
  arr[idx] = arr[target]
  arr[target] = tmp
}

function pickFace(id: string) {
  if (facePickerFor.value) {
    const { bi, si } = facePickerFor.value
    const seg = form.value.blocks[bi]?.segments?.[si]
    if (seg) seg.content = `[CQ:face,id=${id}]`
    facePickerFor.value = null
  }
}

async function loadPickerImages() {
  try {
    const res = (await imageApi.list({ folder: pickerFolder.value, page: 1, page_size: 100 })).data.data
    pickerImages.value = res.list || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载图床图片失败')
  }
}

async function loadPickerFolders() {
  try {
    const res = (await imageFolderApi.list()).data.data
    pickerFolders.value = res || []
  } catch (e: any) {
    // 静默失败，不影响主流程
  }
}

function openPicker(bi: number, si: number) {
  pickerTarget.value = { bi, si }
  pickerFolder.value = '/'
  pickerSelectedId.value = ''
  pickerOpen.value = true
  loadPickerImages()
  loadPickerFolders()
}

async function confirmPicker() {
  const img = pickerImages.value.find(i => i.id === pickerSelectedId.value)
  const t = pickerTarget.value
  if (img && t) {
    const seg = form.value.blocks[t.bi]?.segments?.[t.si]
    if (seg) seg.content = `imgs://${img.id}`
    pickerTarget.value = null
  }
}

function openCreate() {
  previewBlockIdx.value = null
  editingId.value = null
  form.value = {
    name: '', enabled: true, cron_expr: '0 0 9 * * *',
    target_type: 'group', target_id: 0,
    blocks: [emptyBlock('message')],
  }
  timeStr.value = '09:00'
  targetIDText.value = ''
  dialog.value = true
}

function openEdit(t: ScheduledMessageResp) {
  previewBlockIdx.value = null
  editingId.value = t.id
  form.value = {
    name: t.name,
    enabled: t.enabled,
    cron_expr: t.cron_expr,
    target_type: t.target_type,
    target_id: t.target_id,
    blocks: (t.blocks || []).map(b => ({
      type: b.type,
      segments: b.type === 'message' ? (b.segments || []).map(s => ({ ...s })) : undefined,
      delay_seconds: b.type === 'delay' ? b.delay_seconds : undefined,
    })),
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
  if (!form.value.blocks.length) {
    toastStore.error('请至少添加一个编排块')
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
      blocks: form.value.blocks,
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
.orchestration {
  padding: 4px 0;
}
/* 触发器块 */
.trigger-block {
  background: rgba(var(--v-theme-primary), 0.1);
  border: 2px solid var(--v-theme-primary);
  border-radius: 12px;
  padding: 10px 14px;
}
.block-header {
  display: flex;
  align-items: center;
  gap: 4px;
}
/* 连接线 */
.block-connector {
  display: flex;
  justify-content: center;
  padding: 2px 0;
  color: rgba(128, 128, 128, 0.5);
}
/* 消息块 */
.message-block {
  border: 1.5px solid rgba(var(--v-theme-primary), 0.55);
  border-radius: 12px;
  padding: 10px 14px;
  background: rgba(var(--v-theme-primary), 0.04);
}
/* 延时块 */
.delay-block {
  border: 1.5px dashed rgba(255, 152, 0, 0.6);
  border-radius: 12px;
  padding: 10px 14px;
  background: rgba(255, 152, 0, 0.05);
}
.segment-row {
  display: flex;
  align-items: center;
  padding: 6px 4px;
  border-top: 1px dashed rgba(128, 128, 128, 0.25);
  gap: 6px;
}
.segment-row:first-of-type {
  border-top: none;
}
/* 块后添加按钮 */
.add-block {
  display: flex;
  justify-content: center;
  padding: 2px 0 8px;
  opacity: 0.75;
}
.add-block:hover {
  opacity: 1;
}
.face-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(44px, 1fr));
  gap: 6px;
  max-height: 360px;
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
/* 渲染预览面板 */
.preview-panel {
  margin-top: 8px;
  border: 1px dashed rgba(var(--v-theme-info), 0.5);
  border-radius: 8px;
  background: rgba(var(--v-theme-info), 0.04);
  padding: 8px 12px;
}
.preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}
.preview-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.preview-line {
  display: flex;
  align-items: flex-start;
  min-height: 28px;
  padding: 2px 0;
}
.preview-text {
  white-space: pre-wrap;
  word-break: break-word;
  width: 100%;
}
.preview-face img {
  width: 28px;
  height: 28px;
  object-fit: contain;
}
.preview-image img {
  max-width: 100%;
  max-height: 200px;
  object-fit: contain;
  border-radius: 6px;
}
.preview-t2i-frame {
  width: 100%;
  height: 220px;
  border: 1px solid rgba(128, 128, 128, 0.3);
  border-radius: 6px;
  background: white;
}
</style>
