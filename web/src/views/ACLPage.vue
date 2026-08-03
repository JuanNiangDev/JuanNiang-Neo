<template>
  <div>
    <div class="page-header">
      <div class="page-title">ACL 黑名单管理</div>
      <div class="page-subtitle">禁止指定 QQ / 全部 QQ 使用 Agent 循环，命中黑名单的消息直接丢弃</div>
    </div>
    <div class="d-flex justify-end mb-4"><v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增黑名单</v-btn></div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.target_type="{ item }">
        <v-chip size="small" variant="tonal" :color="item.target_type==='all'?'error':'warning'">
          {{ item.target_type === 'all' ? '全部 QQ' : '指定 QQ' }}
        </v-chip>
      </template>
      <template #item.user_ids="{ item }">
        <div v-if="item.target_type === 'list'" class="d-flex flex-wrap" style="gap:4px">
          <v-chip v-for="uid in (item.user_ids || [])" :key="uid" size="x-small" variant="tonal" color="error">{{ uid }}</v-chip>
          <span v-if="!item.user_ids || item.user_ids.length === 0" class="text-caption text-medium-emphasis">(空)</span>
        </div>
        <span v-else class="text-caption text-medium-emphasis">—</span>
      </template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <v-dialog v-model="dialog" max-width="560">
      <v-card rounded="lg">
        <v-card-title>新增黑名单</v-card-title>
        <v-card-text>
          <v-form ref="formRef">
            <v-select
              v-model="form.chat_area_id"
              :items="chatAreaItems"
              item-title="label"
              item-value="value"
              label="Chat Area"
              class="mb-3"
              clearable
            />
            <v-select
              v-model="form.target_type"
              :items="[{ title: '全部 QQ', value: 'all' }, { title: '指定 QQ 列表', value: 'list' }]"
              label="黑名单范围"
              class="mb-3"
              hint="全部 = 禁止所有 QQ 使用 Agent 循环；指定 = 仅禁止以下 QQ"
              persistent-hint
            />
            <v-combobox
              v-if="form.target_type === 'list'"
              v-model="form.user_ids"
              label="QQ 号列表"
              multiple
              chips
              closable-chips
              hint="回车添加, 支持多个 QQ 号"
              class="mb-3"
            />
          </v-form>
        </v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="dialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400"><v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>确定要删除此黑名单吗？</v-card-text><v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card></v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { aclApi, chatAreaApi, type ACLRuleResp, type AddACLRuleReq, type ChatAreaResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<ACLRuleResp[]>([]); const dialog = ref(false); const deleteDialog = ref(false)
const saving = ref(false); const deleting = ref(false); const deleteTarget = ref<ACLRuleResp | null>(null); const formRef = ref()
const chatAreaItems = ref<{label: string; value: string}[]>([])

const headers = [
  { title: 'ID', key: 'id' },
  { title: 'Chat Area', key: 'chat_area_id' },
  { title: '黑名单范围', key: 'target_type' },
  { title: 'QQ 列表', key: 'user_ids' },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

type BlacklistTargetType = 'all' | 'list'
const defaultForm = () => ({ chat_area_id: '', target_type: 'all' as BlacklistTargetType, user_ids: [] as string[] })
const form = ref(defaultForm())

async function fetch() { loading.value = true; try { items.value = (await aclApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
async function fetchChatAreas() { try { const list = (await chatAreaApi.list()).data.data || []; chatAreaItems.value = list.map((c: ChatAreaResp) => ({ label: `${c.area_type==='private'?'私聊':'群聊'} ${c.target_id} (${c.id.slice(0,8)})`, value: c.id })) } catch { toastStore.error('获取 ChatArea 列表失败') } }

function openAdd() { form.value = defaultForm(); dialog.value = true }
async function handleSave() {
  if (!form.value.chat_area_id) { toastStore.error('请选择 Chat Area'); return }
  if (form.value.target_type === 'list' && (!form.value.user_ids || form.value.user_ids.length === 0)) {
    toastStore.error('请至少添加一个 QQ 号'); return
  }
  saving.value = true
  try {
    const data: AddACLRuleReq = {
      chat_area_id: form.value.chat_area_id,
      scope: 'chat',
      permission: 'deny',
      target_type: form.value.target_type,
      user_ids: form.value.user_ids,
    }
    await aclApi.create(data)
    toastStore.success('黑名单已保存')
    dialog.value = false
    await fetch()
  } catch (e: any) { toastStore.error(e?.message || '保存失败') } finally { saving.value = false }
}
function confirmDelete(item: ACLRuleResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() { if (!deleteTarget.value) return; deleting.value = true; try { await aclApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch { toastStore.error('删除失败') } finally { deleting.value = false } }
onMounted(() => { fetch(); fetchChatAreas() })
</script>