<template>
  <div>
    <div class="page-header">
      <div class="page-title">ACL 规则管理</div>
      <div class="page-subtitle">管理聊天/工具/MCP 的访问控制规则</div>
    </div>
    <div class="d-flex justify-end mb-4"><v-btn color="primary" prepend-icon="mdi-plus" @click="openAdd">新增规则</v-btn></div>
    <v-data-table :headers="headers" :items="items" :loading="loading">
      <template #item.scope="{ item }"><v-chip size="small" variant="tonal" :color="item.scope==='chat'?'primary':item.scope==='tool'?'warning':'info'">{{ item.scope }}</v-chip></template>
      <template #item.permission="{ item }"><v-chip size="small" variant="tonal" :color="item.permission==='allow'?'success':'error'">{{ item.permission }}</v-chip></template>
      <template #item.target_type="{ item }">{{ item.target_type }}{{ item.target_type==='list' ? ' ('+(item.user_ids||[]).length+' 用户)' : '' }}</template>
      <template #item.actions="{ item }">
        <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="confirmDelete(item)" />
      </template>
    </v-data-table>

    <v-dialog v-model="dialog" max-width="560">
      <v-card rounded="lg">
        <v-card-title>新增 ACL 规则</v-card-title>
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
            <v-select v-model="form.scope" :items="['chat','tool','mcp']" label="范围" class="mb-3" @update:model-value="onScopeChange" />
            <v-select v-model="form.permission" :items="['allow','deny']" label="权限" class="mb-3" />
            <v-select v-model="form.target_type" :items="['all','list']" label="目标类型" class="mb-3" />
            <v-combobox
              v-if="form.target_type === 'list'"
              v-model="form.user_ids"
              label="目标 QQ 号列表"
              multiple
              chips
              closable-chips
              hint="回车添加, 支持多个 QQ 号"
              class="mb-3"
            />
            <v-select
              v-if="form.scope === 'tool'"
              v-model="form.tool_ids"
              :items="aclToolOptions"
              item-title="label"
              item-value="value"
              label="Tool 列表"
              multiple
              class="mb-3"
            />
            <v-select
              v-if="form.scope === 'mcp'"
              v-model="form.mcp_ids"
              :items="mcpOptions"
              item-title="label"
              item-value="value"
              label="MCP 服务器"
              multiple
              class="mb-3"
            />
          </v-form>
        </v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="dialog = false">取消</v-btn><v-btn color="primary" variant="tonal" @click="handleSave" :loading="saving">保存</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="deleteDialog" max-width="400"><v-card rounded="lg"><v-card-title>确认删除</v-card-title><v-card-text>确定要删除此 ACL 规则吗？</v-card-text><v-card-actions><v-spacer /><v-btn variant="text" @click="deleteDialog = false">取消</v-btn><v-btn color="error" variant="tonal" @click="handleDelete" :loading="deleting">删除</v-btn></v-card-actions></v-card></v-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { aclApi, chatAreaApi, toolApi, mcpApi, type ACLRuleResp, type AddACLRuleReq, type ChatAreaResp, type ToolConfigResp, type MCPServerResp } from '@/api'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
const loading = ref(true); const items = ref<ACLRuleResp[]>([]); const dialog = ref(false); const deleteDialog = ref(false)
const saving = ref(false); const deleting = ref(false); const deleteTarget = ref<ACLRuleResp | null>(null); const formRef = ref()
const chatAreaItems = ref<{label: string; value: string}[]>([])
const aclToolOptions = ref<{label: string; value: string}[]>([])
const mcpOptions = ref<{label: string; value: string}[]>([])

// 基础工具（不在 ACL 管理范围内）
const excludedTools = ['send_msg', 'delete_msg', 't2i', 'send_private_msg', 'send_group_msg', 'send_like']

const headers = [
  { title: 'ID', key: 'id' }, { title: 'Chat Area', key: 'chat_area_id' }, { title: '范围', key: 'scope' },
  { title: '权限', key: 'permission' }, { title: '目标', key: 'target_type' },
  { title: '操作', key: 'actions', align: 'center' as const, sortable: false },
]

const defaultForm = (): AddACLRuleReq => ({ chat_area_id: '', scope: 'chat', permission: 'allow', target_type: 'all', user_ids: [], tool_ids: [], mcp_ids: [] })
const form = ref<AddACLRuleReq>(defaultForm())

function onScopeChange() {
  if (form.value.scope !== 'tool') form.value.tool_ids = []
  if (form.value.scope !== 'mcp') form.value.mcp_ids = []
}

async function fetch() { loading.value = true; try { items.value = (await aclApi.list()).data.data } catch { toastStore.error('获取失败') } finally { loading.value = false } }
async function fetchChatAreas() { try { const list = (await chatAreaApi.list()).data.data || []; chatAreaItems.value = list.map((c: ChatAreaResp) => ({ label: `${c.area_type==='private'?'私聊':'群聊'} ${c.target_id} (${c.id.slice(0,8)})`, value: c.id })) } catch { toastStore.error('获取 ChatArea 列表失败') } }
async function fetchToolOptions() { try { const list = (await toolApi.list()).data.data || []; aclToolOptions.value = list.filter((t: ToolConfigResp) => !excludedTools.includes(t.name)).map((t: ToolConfigResp) => ({ label: `${t.name}${t.is_builtin ? ' (内置)' : ''}`, value: t.name })) } catch { /* ignore */ } }
async function fetchMcpOptions() { try { const list = (await mcpApi.list()).data.data || []; mcpOptions.value = list.map((m: MCPServerResp) => ({ label: m.name, value: m.name })) } catch { /* ignore */ } }

function openAdd() { form.value = defaultForm(); dialog.value = true }
async function handleSave() { saving.value = true; try { await aclApi.create(form.value); toastStore.success('已保存'); dialog.value = false; await fetch() } catch (e: any) { toastStore.error(e?.message || '保存失败') } finally { saving.value = false } }
function confirmDelete(item: ACLRuleResp) { deleteTarget.value = item; deleteDialog.value = true }
async function handleDelete() { if (!deleteTarget.value) return; deleting.value = true; try { await aclApi.delete(deleteTarget.value.id); toastStore.success('已删除'); deleteDialog.value = false; await fetch() } catch { toastStore.error('删除失败') } finally { deleting.value = false } }
onMounted(() => { fetch(); fetchChatAreas(); fetchToolOptions(); fetchMcpOptions() })
</script>