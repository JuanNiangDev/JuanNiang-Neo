<template>
  <div>
    <div class="page-header">
      <div class="page-title"><v-icon class="me-2" color="primary">mdi-image-multiple-outline</v-icon>图床</div>
      <div class="page-subtitle">机器人图床服务</div>
    </div>

    <div class="d-flex img-body">
      <!-- 左侧：虚拟文件夹（层级视图，仅一层） -->
      <v-card width="220" rounded="lg" class="me-4 folder-panel flex-shrink-0">
        <div class="d-flex align-center px-4 pt-3 pb-1">
          <v-icon size="16" class="me-2" color="primary">mdi-folder-multiple-outline</v-icon>
          <span class="text-subtitle-2 font-weight-bold">文件夹</span>
        </div>
        <v-list nav density="compact">
          <!-- 根目录（树顶级） -->
          <v-list-item
            prepend-icon="mdi-folder-home-outline"
            title="根目录 /"
            :active="currentFolder === '/'"
            active-color="primary"
            @click="selectFolder('/')"
          />
          <!-- 子文件夹：树状缩进 + 连接线 -->
          <div v-if="folders.length" class="tree-children">
            <v-list-item
              v-for="f in folders"
              :key="f.id"
              prepend-icon="mdi-folder-outline"
              :title="f.name"
              class="tree-leaf"
              :active="currentFolder === '/' + f.name"
              active-color="primary"
              @click="selectFolder('/' + f.name)"
            >
              <template #append>
                <v-btn icon="mdi-close" size="x-small" variant="text" density="compact" title="删除文件夹（图片移到根）" @click.stop="confirmDeleteFolder(f)" />
              </template>
            </v-list-item>
          </div>
        </v-list>
      </v-card>

      <!-- 右侧：工具栏 + 网格 -->
      <div class="flex-grow-1" style="min-width: 0">
        <div class="d-flex justify-space-between align-center mb-3">
          <div class="text-subtitle-1 font-weight-medium">
            <v-icon size="18" class="me-1" color="primary">{{ currentFolder === '/' ? 'mdi-folder-home-outline' : 'mdi-folder-outline' }}</v-icon>
            {{ currentFolder === '/' ? '根目录 /' : '文件夹 ' + currentFolder }}
            <span class="text-caption text-medium-emphasis ms-2">共 {{ total }} 张</span>
          </div>
          <div>
            <v-btn variant="tonal" prepend-icon="mdi-refresh" class="me-2" :loading="loading" @click="loadImages">刷新</v-btn>
            <v-btn variant="tonal" prepend-icon="mdi-folder-plus-outline" class="me-2" @click="folderDialog = true">新建文件夹</v-btn>
            <v-btn color="primary" prepend-icon="mdi-upload" @click="uploadDialog = true">上传图片</v-btn>
          </div>
        </div>

        <!-- 图片网格 -->
        <div v-if="images.length" class="image-grid">
          <v-card
            v-for="img in images"
            :key="img.id"
            rounded="lg"
            class="image-card"
            elevation="1"
            @click="openDetail(img)"
          >
            <v-img :src="imageFileUrl(img.id)" :alt="img.name" height="150" cover class="image-thumb" />
            <div class="pa-2 image-name" :title="img.name">{{ img.name }}</div>
          </v-card>
        </div>
        <v-card v-else rounded="lg" class="d-flex flex-column align-center justify-center pa-10" style="min-height: 240px">
          <v-icon size="48" color="medium-emphasis" class="mb-3">mdi-image-off-outline</v-icon>
          <div class="text-body-2 text-medium-emphasis">当前文件夹还没有图片，点右上角「上传图片」试试</div>
        </v-card>
      </div>
    </div>

    <!-- 分页（页面底部） -->
    <div class="d-flex justify-center mt-4">
      <v-pagination v-model="page" :length="pageCount" :total-visible="5" @update:model-value="loadImages" />
    </div>

    <!-- 上传弹窗 -->
    <v-dialog v-model="uploadDialog" max-width="520">
      <v-card rounded="lg">
        <v-card-title>上传图片</v-card-title>
        <v-card-text>
          <v-file-input
            v-model="uploadFile"
            label="选择图片（jpg / png / gif / webp，≤ 1.5MB）"
            accept="image/jpeg,image/png,image/gif,image/webp"
            show-size
            class="mb-3"
            @update:model-value="onFileChange"
          />
          <v-text-field v-model="uploadName" label="名称（可选，默认使用文件名）" class="mb-3" />
          <v-select
            v-model="uploadFolder"
            :items="folderOptions"
            label="目标文件夹"
            item-title="title"
            item-value="value"
          />
          <div v-if="fileTooLarge" class="text-caption text-error">图片大小不能超过 1.5MB</div>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="uploadDialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="uploading" :disabled="!uploadFile" @click="handleUpload">上传</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 新建文件夹弹窗 -->
    <v-dialog v-model="folderDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>新建虚拟文件夹</v-card-title>
        <v-card-text>
          <v-text-field v-model="folderName" label="文件夹名（仅一层，不支持嵌套）" @keyup.enter="handleCreateFolder" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="folderDialog = false">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="creatingFolder" :disabled="!folderName.trim()" @click="handleCreateFolder">创建</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 图片详情弹窗 -->
    <v-dialog v-model="detailDialog" max-width="720">
      <v-card v-if="detail" rounded="lg">
        <v-card-title class="d-flex align-center">
          <v-icon class="me-2" color="primary">mdi-image-outline</v-icon>
          <span class="text-truncate">{{ detail.name }}</span>
          <v-spacer />
          <v-btn icon="mdi-close" variant="text" size="small" @click="detailDialog = false" />
        </v-card-title>
        <v-card-text>
          <div class="text-center mb-4">
            <v-img :src="imageFileUrl(detail.id)" :alt="detail.name" max-height="360" contain rounded="lg" class="detail-preview" />
          </div>

          <v-row dense>
            <v-col cols="12" md="6">
              <v-text-field v-model="editName" label="名称" class="mb-2" />
              <v-select v-model="editFolder" :items="folderOptions" label="虚拟文件夹" item-title="title" item-value="value" class="mb-2" />
            </v-col>
            <v-col cols="12" md="6">
              <div class="meta-list text-body-2">
                <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">ID</span><span class="text-caption meta-code">{{ detail.id }}</span></div>
                <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">虚拟路径</span><span>{{ detail.folder }}</span></div>
                <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">大小</span><span>{{ formatSize(detail.size_bytes) }}</span></div>
                <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">类型</span><span>{{ detail.mime_type }}</span></div>
                <div class="d-flex justify-space-between py-1"><span class="text-medium-emphasis">上传时间</span><span>{{ new Date(detail.created_at).toLocaleString() }}</span></div>
              </div>
            </v-col>
          </v-row>

          <!-- 引用方式 -->
          <div class="mt-2">
            <div class="text-caption text-medium-emphasis mb-1">消息引用（复制后在消息中使用）：</div>
            <div class="d-flex align-center">
              <code class="flex-grow-1 ref-code">{{ `[CQ:image,file=imgs://${detail.id}]` }}</code>
              <v-btn size="small" variant="tonal" prepend-icon="mdi-content-copy" class="ms-2" @click="copyRef(detail)">复制</v-btn>
            </div>
          </div>
        </v-card-text>
        <v-card-actions>
          <v-btn color="error" variant="text" prepend-icon="mdi-delete" @click="confirmDeleteImage(detail)">删除</v-btn>
          <v-spacer />
          <v-btn variant="text" @click="detailDialog = false">关闭</v-btn>
          <v-btn color="primary" variant="tonal" :loading="saving" @click="handleSaveDetail">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 删除确认 -->
    <v-dialog v-model="deleteDialog" max-width="400">
      <v-card rounded="lg">
        <v-card-title>{{ deleteFolderTarget ? '删除文件夹' : '确认删除' }}</v-card-title>
        <v-card-text>
          <template v-if="deleteFolderTarget">
            确定删除文件夹「{{ deleteFolderTarget.name }}」吗？其下图片将自动移动到根目录 /。
          </template>
          <template v-else>确定删除这张图片吗？磁盘文件与记录将一并删除，此操作不可撤销。</template>
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
import { imageApi, imageFolderApi, imageFileUrl, type ImageResp, type ImageFolderResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()

const loading = ref(false)
const images = ref<ImageResp[]>([])
const folders = ref<ImageFolderResp[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 48
const currentFolder = ref('/')

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

// 上传
const uploadDialog = ref(false)
const uploadFile = ref<File | null>(null)
const uploadName = ref('')
const uploadFolder = ref('/')
const uploading = ref(false)
const fileTooLarge = ref(false)

// 新建文件夹
const folderDialog = ref(false)
const folderName = ref('')
const creatingFolder = ref(false)

// 详情
const detailDialog = ref(false)
const detail = ref<ImageResp | null>(null)
const editName = ref('')
const editFolder = ref('/')
const saving = ref(false)

// 删除
const deleteDialog = ref(false)
const deleteImageTarget = ref<ImageResp | null>(null)
const deleteFolderTarget = ref<ImageFolderResp | null>(null)
const deleting = ref(false)

const folderOptions = computed(() => [
  { title: '根目录 /', value: '/' },
  ...folders.value.map(f => ({ title: `文件夹 ${f.name}`, value: '/' + f.name })),
])

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

async function loadFolders() {
  try {
    const res = (await imageFolderApi.list()).data.data
    folders.value = res || []
  } catch (e: any) {
    toastStore.error(e?.message || '加载文件夹失败')
  }
}

async function loadImages() {
  loading.value = true
  try {
    const res = (await imageApi.list({ folder: currentFolder.value, page: page.value, page_size: pageSize })).data.data
    images.value = res.list || []
    total.value = res.total || 0
  } catch (e: any) {
    toastStore.error(e?.message || '加载图片失败')
  } finally {
    loading.value = false
  }
}

function selectFolder(folder: string) {
  currentFolder.value = folder
  page.value = 1
  loadImages()
}

function onFileChange(files: File | File[] | null) {
  const file = Array.isArray(files) ? (files[0] ?? null) : files
  fileTooLarge.value = !!file && file.size > 1536 * 1024
  if (file && !uploadName.value.trim()) {
    uploadName.value = file.name.replace(/\.[^.]+$/, '')
  }
}

async function handleUpload() {
  if (!uploadFile.value) return
  if (uploadFile.value.size > 1536 * 1024) {
    toastStore.error('图片大小不能超过 1.5MB')
    return
  }
  uploading.value = true
  try {
    await imageApi.upload(uploadFile.value, uploadName.value, uploadFolder.value)
    toastStore.success('上传成功')
    uploadDialog.value = false
    uploadFile.value = null
    uploadName.value = ''
    currentFolder.value = uploadFolder.value
    page.value = 1
    await loadImages()
    await loadFolders()
  } catch (e: any) {
    toastStore.error(e?.message || '上传失败')
  } finally {
    uploading.value = false
  }
}

async function handleCreateFolder() {
  const name = folderName.value.trim()
  if (!name) return
  creatingFolder.value = true
  try {
    await imageFolderApi.create(name)
    toastStore.success('文件夹已创建')
    folderDialog.value = false
    folderName.value = ''
    await loadFolders()
  } catch (e: any) {
    toastStore.error(e?.message || '创建失败')
  } finally {
    creatingFolder.value = false
  }
}

function openDetail(img: ImageResp) {
  detail.value = img
  editName.value = img.name
  editFolder.value = img.folder
  detailDialog.value = true
}

async function handleSaveDetail() {
  if (!detail.value) return
  saving.value = true
  try {
    const res = (await imageApi.update(detail.value.id, { name: editName.value, folder: editFolder.value })).data.data
    detail.value = res
    toastStore.success('已保存')
    await loadImages()
    await loadFolders()
  } catch (e: any) {
    toastStore.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function copyRef(img: ImageResp) {
  const text = `[CQ:image,file=imgs://${img.id}]`
  try {
    await navigator.clipboard.writeText(text)
    toastStore.success('引用已复制')
  } catch {
    // 非安全上下文 fallback
    const ta = document.createElement('textarea')
    ta.value = text
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
    toastStore.success('引用已复制')
  }
}

function confirmDeleteImage(img: ImageResp) {
  deleteImageTarget.value = img
  deleteFolderTarget.value = null
  deleteDialog.value = true
}

function confirmDeleteFolder(f: ImageFolderResp) {
  deleteFolderTarget.value = f
  deleteImageTarget.value = null
  deleteDialog.value = true
}

async function handleDelete() {
  deleting.value = true
  try {
    if (deleteFolderTarget.value) {
      await imageFolderApi.remove(deleteFolderTarget.value.id)
      toastStore.success('文件夹已删除')
      if (currentFolder.value === '/' + deleteFolderTarget.value.name) {
        currentFolder.value = '/'
        page.value = 1
      }
      deleteFolderTarget.value = null
      await loadFolders()
      await loadImages()
    } else if (deleteImageTarget.value) {
      await imageApi.remove(deleteImageTarget.value.id)
      toastStore.success('已删除')
      if (detail.value?.id === deleteImageTarget.value.id) {
        detailDialog.value = false
        detail.value = null
      }
      deleteImageTarget.value = null
      await loadImages()
    }
    deleteDialog.value = false
  } catch (e: any) {
    toastStore.error(e?.message || '删除失败')
  } finally {
    deleting.value = false
  }
}

onMounted(() => {
  loadFolders()
  loadImages()
})
</script>

<style scoped>
/* 左侧面板撑满可用高度 */
.img-body {
  min-height: calc(100vh - 220px);
  align-items: stretch;
}
.folder-panel {
  display: flex;
  flex-direction: column;
}
.folder-panel :deep(.v-list) {
  flex-grow: 1;
}
/* 树状视图：子文件夹缩进 + 左侧连接线 */
.folder-panel :deep(.tree-children) {
  margin-left: 22px;
  border-left: 1px solid rgba(128, 128, 128, 0.3);
  padding-left: 8px;
}
.folder-panel :deep(.tree-leaf) {
  position: relative;
}
.folder-panel :deep(.tree-leaf::before) {
  content: '';
  position: absolute;
  left: -14px;
  top: 50%;
  width: 10px;
  height: 1px;
  background: rgba(128, 128, 128, 0.3);
}

.image-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 14px;
}
.image-card {
  cursor: pointer;
  overflow: hidden;
  transition: transform 0.15s, box-shadow 0.15s;
}
.image-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.14) !important;
}
.image-thumb {
  background: rgba(128, 128, 128, 0.08);
}
.image-name {
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.detail-preview {
  background: rgba(128, 128, 128, 0.06);
}
.meta-code {
  max-width: 60%;
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
