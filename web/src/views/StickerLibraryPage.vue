<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-emoticon-outline</v-icon>表情包库</div>
      <div class="page-subtitle">基于图床的表情管理：发送时用 <code class="text-caption">stk://&lt;表情ID&gt;</code>（Plugin / Agent 自动解析）</div>
    </div>

    <!-- 工具栏 -->
    <div class="d-flex justify-space-between align-center mb-3">
      <div class="d-flex align-center" style="max-width: 420px; flex-grow: 1">
        <v-text-field
          v-model="keyword"
          label="搜索表情（名称 / 简介模糊匹配）"
          prepend-inner-icon="mdi-magnify"
          density="compact"
          hide-details
          clearable
          class="me-2"
          @keyup.enter="search"
          @click:clear="clearSearch"
        />
        <v-btn variant="tonal" prepend-icon="mdi-magnify" @click="search">搜索</v-btn>
      </div>
      <div>
        <v-btn variant="tonal" prepend-icon="mdi-tag-plus-outline" class="me-2" @click="tagDialog = true">新建标签</v-btn>
        <v-btn color="primary" prepend-icon="mdi-emoticon-plus-outline" @click="openCreate">新建表情</v-btn>
      </div>
    </div>

    <!-- 标签筛选 -->
    <div class="d-flex align-center flex-wrap mb-3 tag-bar">
      <v-chip
        :color="currentTag === '' ? 'primary' : undefined"
        variant="tonal"
        class="me-2 mb-1"
        @click="selectTag('')"
      >全部</v-chip>
      <v-chip
        v-for="t in tags"
        :key="t.id"
        :color="currentTag === t.name ? 'primary' : undefined"
        variant="tonal"
        class="me-2 mb-1"
        closable
        @click="selectTag(t.name)"
        @click:close="confirmDeleteTag(t)"
      >{{ t.name }}</v-chip>
    </div>

    <!-- 表情网格 -->
    <div v-if="stickers.length" class="sticker-grid">
      <v-card
        v-for="s in stickers"
        :key="s.id"
        rounded="lg"
        class="sticker-card"
        elevation="1"
        @click="openDetail(s)"
      >
        <v-img :src="imageFileUrl(s.image_id)" :alt="s.name" height="140" cover class="sticker-thumb" />
        <div class="pa-2">
          <div class="sticker-name" :title="s.name">{{ s.name }}</div>
          <div class="d-flex flex-wrap mt-1">
            <v-chip v-for="t in s.tags.slice(0, 3)" :key="t" size="x-small" variant="tonal" color="primary" class="me-1 mb-1">{{ t }}</v-chip>
          </div>
        </div>
      </v-card>
    </div>
    <v-card v-else rounded="lg" class="d-flex flex-column align-center justify-center pa-10" style="min-height: 240px">
      <v-icon size="48" color="medium-emphasis" class="mb-3">mdi-emoticon-outline</v-icon>
      <div class="text-body-2 text-medium-emphasis">还没有表情，点右上角「新建表情」从图床图片创建</div>
    </v-card>

    <!-- 分页 -->
    <div class="d-flex justify-center mt-4">
      <v-pagination v-model="page" :length="pageCount" :total-visible="5" @update:model-value="loadStickers" />
    </div>

    <!-- 新建/编辑表情弹窗 -->
    <v-dialog v-model="editDialog" max-width="720">
      <v-card v-if="editDialog" rounded="lg">
        <v-card-title>{{ editingId ? '编辑表情' : '新建表情' }}</v-card-title>
        <v-card-text>
          <v-row dense>
            <v-col cols="12" md="6">
              <v-text-field v-model="form.name" label="名称" class="mb-2" />
              <v-textarea v-model="form.desc" label="简介（支持模糊匹配）" rows="3" class="mb-2" />
              <v-combobox
                v-model="form.tags"
                label="标签（可输入创建）"
                :items="tags.map(t => t.name)"
                multiple
                chips
                small-chips
                class="mb-2"
              />
            </v-col>
            <v-col cols="12" md="6">
              <div class="text-caption text-medium-emphasis mb-1">选择图床图片（{{ selectedImage ? '已选 1 张' : '未选择' }}）</div>
              <div v-if="selectedImage" class="d-flex align-center mb-2">
                <v-img :src="imageFileUrl(selectedImage.id)" height="60" width="60" cover rounded="lg" class="me-2" />
                <div>
                  <div class="text-body-2">{{ selectedImage.name }}</div>
                  <div class="text-caption text-medium-emphasis">{{ formatSize(selectedImage.size_bytes) }}</div>
                </div>
              </div>
              <v-btn variant="tonal" size="small" prepend-icon="mdi-image-multiple-outline" @click="pickerDialog = true">
                {{ selectedImage ? '更换图片' : '选择图片' }}
              </v-btn>
            </v-col>
          </v-row>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="editDialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="saving" :disabled="!form.name.trim() || !selectedImage" @click="handleSave">保存</v-btn>
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
              @click="pickImage(img)"
            >
              <v-img :src="imageFileUrl(img.id)" :alt="img.name" height="90" cover class="picker-thumb" />
              <div class="pa-1 picker-name" :title="img.name">{{ img.name }}</div>
            </v-card>
          </div>
          <div v-else class="text-body-2 text-medium-emphasis pa-6 text-center">图床暂无图片，请先到「图床」页面上传</div>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="pickerDialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" :disabled="!pickerSelectedId" @click="confirmPick">确定</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 表情详情弹窗 -->
    <v-dialog v-model="detailDialog" max-width="640">
      <v-card v-if="detail" rounded="lg">
        <v-card-title class="d-flex align-center">
          <v-icon class="me-2" color="primary">mdi-emoticon-outline</v-icon>
          <span class="text-truncate">{{ detail.name }}</span>
          <v-spacer />
          <v-btn icon="mdi-close" variant="text" size="small" @click="detailDialog = false" />
        </v-card-title>
        <v-card-text>
          <div class="text-center mb-3">
            <v-img :src="imageFileUrl(detail.image_id)" :alt="detail.name" max-height="280" contain rounded="lg" class="detail-preview" />
          </div>
          <div class="d-flex flex-wrap mb-3">
            <v-chip v-for="t in detail.tags" :key="t" size="small" variant="tonal" color="primary" class="me-1 mb-1">{{ t }}</v-chip>
          </div>
          <div class="text-body-2 mb-3" style="white-space: pre-wrap">{{ detail.desc || '（无简介）' }}</div>
          <div class="meta-list text-body-2">
            <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">表情 ID（短 UUID）</span><span class="text-caption meta-code">{{ detail.id }}</span></div>
            <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">图床图片 ID</span><span class="text-caption meta-code">{{ detail.image_id }}</span></div>
            <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">创建时间</span><span>{{ new Date(detail.created_at).toLocaleString() }}</span></div>
          </div>
          <div class="mt-2">
            <div class="text-caption text-medium-emphasis mb-1">消息引用（Plugin / Agent 发送表情用）：</div>
            <div class="d-flex align-center">
              <code class="flex-grow-1 ref-code">{{ `[CQ:image,file=stk://${detail.id}]` }}</code>
              <v-btn size="small" variant="tonal" prepend-icon="mdi-content-copy" class="ms-2" @click="copyRef(detail.id)">复制</v-btn>
            </div>
          </div>
        </v-card-text>
        <v-card-actions>
          <v-btn color="error" variant="text" prepend-icon="mdi-delete" @click="confirmDelete(detail)">删除</v-btn>
          <v-spacer />
          <v-btn variant="text" @click="detailDialog = false">关闭</v-btn>
          <v-btn color="primary" variant="tonal" @click="openEdit(detail)">编辑</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 新建标签弹窗 -->
    <v-dialog v-model="tagDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>新建标签</v-card-title>
        <v-card-text>
          <v-text-field v-model="tagName" label="标签名" @keyup.enter="handleCreateTag" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="tagDialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="creatingTag" :disabled="!tagName.trim()" @click="handleCreateTag">创建</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 删除确认 -->
    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>{{ deleteTagTarget ? '删除标签' : '确认删除' }}</v-card-title>
        <v-card-text>
          <template v-if="deleteTagTarget">确定删除标签「{{ deleteTagTarget.name }}」吗？所有表情中的该标签将一并移除。</template>
          <template v-else>确定删除这个表情吗？此操作不可撤销。</template>
        </v-card-text>
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
import { stickerApi, stickerTagApi, imageApi, imageFileUrl, type StickerResp, type StickerTagResp, type ImageResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()

const loading = ref(false)
const stickers = ref<StickerResp[]>([])
const tags = ref<StickerTagResp[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 24
const currentTag = ref('')
const keyword = ref('')

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

// 新建/编辑
const editDialog = ref(false)
const editingId = ref<string | null>(null)
const form = ref({ name: '', desc: '', tags: [] as string[] })
const selectedImage = ref<ImageResp | null>(null)
const saving = ref(false)

// 图床图片选择器
const pickerDialog = ref(false)
const pickerImages = ref<ImageResp[]>([])
const pickerSelectedId = ref('')
const pickerLoaded = ref(false)

// 详情
const detailDialog = ref(false)
const detail = ref<StickerResp | null>(null)

// 标签
const tagDialog = ref(false)
const tagName = ref('')
const creatingTag = ref(false)

// 删除
const deleteDialog = ref(false)
const deleteTarget = ref<StickerResp | null>(null)
const deleteTagTarget = ref<StickerTagResp | null>(null)
const deleting = ref(false)

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

async function loadTags() {
  try {
    const res = (await stickerTagApi.list()).data.data
    tags.value = res || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载标签失败')
  }
}

async function loadStickers() {
  loading.value = true
  try {
    const res = (await stickerApi.list({
      tag: currentTag.value || undefined,
      keyword: keyword.value || undefined,
      page: page.value,
      page_size: pageSize,
    })).data.data
    stickers.value = res.list || []
    total.value = res.total || 0
  } catch (e: any) {
    toastStore.error(e?.message || '加载表情失败')
  } finally {
    loading.value = false
  }
}

function selectTag(tag: string) {
  currentTag.value = tag
  page.value = 1
  loadStickers()
}

function search() {
  page.value = 1
  loadStickers()
}

function clearSearch() {
  keyword.value = ''
  page.value = 1
  loadStickers()
}

async function loadPickerImages() {
  if (pickerLoaded.value) return
  try {
    const res = (await imageApi.list({ page: 1, page_size: 100 })).data.data
    pickerImages.value = res.list || []
    pickerLoaded.value = true
  } catch (e: any) {
    toastStore.error(e?.message || '加载图床图片失败')
  }
}

function openCreate() {
  editingId.value = null
  form.value = { name: '', desc: '', tags: [] }
  selectedImage.value = null
  pickerSelectedId.value = ''
  pickerLoaded.value = false
  editDialog.value = true
  loadPickerImages()
}

function openEdit(s: StickerResp) {
  editingId.value = s.id
  form.value = { name: s.name, desc: s.desc || '', tags: [...(s.tags || [])] }
  // 从图床拉取对应图片信息用于展示
  imageApi.get(s.image_id).then(res => {
    selectedImage.value = res.data.data
  }).catch(() => {
    selectedImage.value = null
  })
  pickerSelectedId.value = s.image_id
  pickerLoaded.value = false
  editDialog.value = true
  loadPickerImages()
}

function pickImage(img: ImageResp) {
  pickerSelectedId.value = img.id
}

function confirmPick() {
  const img = pickerImages.value.find(i => i.id === pickerSelectedId.value)
  if (img) {
    selectedImage.value = img
    pickerDialog.value = false
  }
}

async function handleSave() {
  if (!form.value.name.trim() || !selectedImage.value) return
  saving.value = true
  try {
    const payload = {
      name: form.value.name.trim(),
      desc: form.value.desc.trim(),
      tags: form.value.tags,
    }
    if (editingId.value) {
      await stickerApi.update(editingId.value, payload)
      toastStore.success('已保存')
    } else {
      await stickerApi.create({ image_id: selectedImage.value.id, ...payload })
      toastStore.success('已创建')
    }
    editDialog.value = false
    await loadStickers()
    await loadTags()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function openDetail(s: StickerResp) {
  detail.value = s
  detailDialog.value = true
}

async function copyRef(id: string) {
  const text = `[CQ:image,file=stk://${id}]`
  try {
    await navigator.clipboard.writeText(text)
    toastStore.success('引用已复制')
  } catch {
    const ta = document.createElement('textarea')
    ta.value = text
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
    toastStore.success('引用已复制')
  }
}

async function handleCreateTag() {
  const name = tagName.value.trim()
  if (!name) return
  creatingTag.value = true
  try {
    await stickerTagApi.create(name)
    toastStore.success('标签已创建')
    tagDialog.value = false
    tagName.value = ''
    await loadTags()
  } catch (e: any) {
    toastStore.error(e?.message || '创建失败')
  } finally {
    creatingTag.value = false
  }
}

function confirmDelete(s: StickerResp) {
  deleteTarget.value = s
  deleteTagTarget.value = null
  deleteDialog.value = true
}

function confirmDeleteTag(t: StickerTagResp) {
  deleteTagTarget.value = t
  deleteTarget.value = null
  deleteDialog.value = true
}

async function handleDelete() {
  deleting.value = true
  try {
    if (deleteTagTarget.value) {
      await stickerTagApi.remove(deleteTagTarget.value.id)
      toastStore.success('标签已删除')
      if (currentTag.value === deleteTagTarget.value.name) {
        currentTag.value = ''
        page.value = 1
      }
      deleteTagTarget.value = null
      await loadTags()
      await loadStickers()
    } else if (deleteTarget.value) {
      await stickerApi.remove(deleteTarget.value.id)
      toastStore.success('已删除')
      if (detail.value?.id === deleteTarget.value.id) {
        detailDialog.value = false
        detail.value = null
      }
      deleteTarget.value = null
      await loadStickers()
    }
    deleteDialog.value = false
  } catch (e: any) {
    toastStore.error(e?.message || '删除失败')
  } finally {
    deleting.value = false
  }
}

onMounted(() => {
  loadTags()
  loadStickers()
})
</script>

<style scoped>
.tag-bar {
  min-height: 40px;
}
.sticker-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 14px;
}
.sticker-card {
  cursor: pointer;
  overflow: hidden;
  transition: transform 0.15s, box-shadow 0.15s;
}
.sticker-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.14) !important;
}
.sticker-thumb {
  background: rgba(128, 128, 128, 0.08);
}
.sticker-name {
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
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
.detail-preview {
  background: rgba(128, 128, 128, 0.06);
}
.meta-code {
  max-width: 55%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  direction: rtl;
  text-align: left;
}
.ref-code {
  font-size: 12px;
  background: rgba(128, 128, 128, 0.08);
  border-radius: 6px;
  padding: 6px 10px;
  overflow-x: auto;
  white-space: nowrap;
}
</style>
