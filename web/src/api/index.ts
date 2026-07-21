import client from './client'

// ======== Types ========
export interface LoginReq { username: string; password: string }
export interface ChangePasswordReq { old_password: string; new_password: string }
export interface TokenResp { token: string }

export interface AdapterConnDetail { id: number; ip: string; self_id: number }
export interface AdapterStatus { running: boolean; listen_addr: string; self_id: number; conn_count: number; conn_ids: number[]; conns: AdapterConnDetail[] }
export interface UpdateAdapterConfigReq { addr: string; port: number; token: string; admin_qq_numbers: string[]; enabled: boolean }

export interface ProviderResp { id: string; created_at: string; name: string; type: string; endpoint: string; token: string; model: string; temperature: number; is_active: boolean }
export interface AddProviderReq { name: string; type: string; endpoint: string; token: string; model: string; temperature?: number; isActive: boolean }

export interface MCPServerResp { id: string; name: string; server_url: string; headers: Record<string, any>; timeout: number; retry_count: number; tool_filter: string[]; auto_reconnect: boolean; is_active: boolean; created_at: string }
export interface AddMCPServerReq { name: string; server_url: string; headers?: Record<string, any>; timeout?: number; retry_count?: number; tool_filter?: string[]; auto_reconnect?: boolean; is_active: boolean }

export interface ShortTermMemoryResp { id: string; chat_area_id: string; window_size: number; auto_compact: boolean; created_at: string }
export interface LongTermMemoryResp { id: string; chat_area_id: string; hot_area_size: number; hot_memory_ttl: number; created_at: string }

export interface PromptResp { id: string; name: string; content: string; type: string; is_active: boolean; is_system: boolean; created_at: string }
export interface AddPromptReq { name: string; content: string; type: string; is_active: boolean }

export interface SessionResp { id: string; chat_area_id: string; model: string; token_usage: number; meta_data: Record<string, any>; created_at: string }

export interface SkillResp { id: string; name: string; description: string; keywords: string[]; regex_pattern: string; prompt_ref: string; tool_refs: string[]; mcp_refs: string[]; is_active: boolean; is_system: boolean; priority: number; created_at: string }
export interface AddSkillReq { name: string; description?: string; keywords?: string[]; regex_pattern?: string; prompt_ref?: string; tool_refs?: string[]; mcp_refs?: string[]; is_active: boolean; is_system?: boolean; priority?: number }

export interface ToolConfigResp { id: string; name: string; description: string; parameters: Record<string, any>; timeout: number; is_active: boolean; is_builtin: boolean; created_at: string }

export interface PluginResp { id: string; name: string; version: string; path: string; config: Record<string, any>; is_active: boolean; created_at: string }

export interface ACLRuleResp { id: number; chat_area_id: string; scope: string; permission: string; target_type: string; user_ids: string[]; tool_ids: string[]; mcp_ids: string[]; created_at: string }
export interface AddACLRuleReq { chat_area_id: string; scope: string; permission: string; target_type: string; user_ids?: string[]; tool_ids?: string[]; mcp_ids?: string[] }

export interface ChatAreaResp { id: string; area_type: string; target_id: number; created_at: string }

export interface ChatRecordResp { id: number; chat_area_id: string; user_id: number; role: string; content: string; token_count: number; tool_calls: any; created_at: string }
export interface ChatRecordListResp { total: number; list: ChatRecordResp[] }

export interface OverviewResp { chat_area_count: number; mcp_count: number; adapter_count: number; plugin_count: number; provider_count: number; skill_count: number; session_count: number; total_token_usage: number; cpu_count: number; goroutine_num: number; mem_alloc_bytes: number; mem_sys_bytes: number; mem_heap_inuse_bytes: number; go_version: string; t2i_active: boolean; t2i_healthy: boolean; sandbox_active: boolean; sandbox_healthy: boolean }

export interface T2IConfigResp { base_url: string; timeout: number; is_active: boolean; healthy: boolean }
export interface UpdateT2IConfigReq { base_url: string; timeout?: number; is_active: boolean }

export interface SandboxConfigResp { base_url: string; api_key: string; timeout: number; is_active: boolean; healthy: boolean }
export interface UpdateSandboxConfigReq { base_url: string; api_key: string; timeout?: number; is_active: boolean }

export interface WebhookConfigResp { addr: string; port: number; token: string; enabled: boolean; running: boolean }
export interface UpdateWebhookConfigReq { addr: string; port: number; token: string; enabled: boolean }

export interface LogEntryResp { time: string; level: string; message: string; attrs: Record<string, any> }

// ======== Auth ========
export const authApi = {
  login: (data: LoginReq) => client.post('/login', data),
  changePassword: (data: ChangePasswordReq) => client.post('/change-password', data),
}

// ======== Adapter ========
export const adapterApi = {
  getStatus: () => client.get('/adapter'),
  getConfig: () => client.get('/adapter/config'),
  updateConfig: (data: UpdateAdapterConfigReq) => client.put('/adapter', data),
  restart: () => client.post('/adapter/restart'),
}

// ======== Providers ========
export const providerApi = {
  list: () => client.get('/providers'),
  get: (id: string) => client.get(`/providers/${id}`),
  create: (data: AddProviderReq) => client.post('/providers', data),
  update: (id: string, data: AddProviderReq) => client.put(`/providers/${id}`, data),
  delete: (id: string) => client.delete(`/providers/${id}`),
  toggle: (id: string, is_active: boolean) => client.put(`/providers/${id}/toggle`, { is_active }),
}

// ======== MCP ========
export const mcpApi = {
  list: () => client.get('/mcp'),
  get: (id: string) => client.get(`/mcp/${id}`),
  create: (data: AddMCPServerReq) => client.post('/mcp', data),
  update: (id: string, data: AddMCPServerReq) => client.put(`/mcp/${id}`, data),
  delete: (id: string) => client.delete(`/mcp/${id}`),
  toggle: (id: string, is_active: boolean) => client.put(`/mcp/${id}/toggle`, { is_active }),
  check: (id: string) => client.get(`/mcp/${id}/check`),
}

// ======== Memory ========
export const memoryApi = {
  getShortTerm: (chatAreaID: string) => client.get(`/memory/${chatAreaID}/short-term`),
  updateShortTerm: (chatAreaID: string, data: { window_size: number; auto_compact: boolean }) => client.put(`/memory/${chatAreaID}/short-term`, data),
  getLongTerm: (chatAreaID: string) => client.get(`/memory/${chatAreaID}/long-term`),
  updateLongTerm: (chatAreaID: string, data: { hot_area_size: number; hot_memory_ttl: number }) => client.put(`/memory/${chatAreaID}/long-term`, data),
}

// ======== Prompts ========
export const promptApi = {
  list: () => client.get('/prompts'),
  create: (data: AddPromptReq) => client.post('/prompts', data),
  update: (id: string, data: AddPromptReq) => client.put(`/prompts/${id}`, data),
  delete: (id: string) => client.delete(`/prompts/${id}`),
  toggle: (id: string, is_active: boolean) => client.put(`/prompts/${id}/toggle`, { is_active }),
}

// ======== Sessions ========
export const sessionApi = {
  list: () => client.get('/sessions'),
  get: (id: string) => client.get(`/sessions/${id}`),
  delete: (id: string) => client.delete(`/sessions/${id}`),
}

// ======== Skills ========
export const skillApi = {
  list: () => client.get('/skills'),
  create: (data: AddSkillReq) => client.post('/skills', data),
  update: (id: string, data: AddSkillReq) => client.put(`/skills/${id}`, data),
  delete: (id: string) => client.delete(`/skills/${id}`),
}

// ======== Tools ========
export const toolApi = {
  list: () => client.get('/tools'),
  toggle: (id: string, is_active: boolean) => client.put(`/tools/${id}/toggle`, { is_active }),
}

// ======== Plugins ========
export const pluginApi = {
  list: () => client.get('/plugins'),
  upload: (file: File) => { const fd = new FormData(); fd.append('file', file); return client.post('/plugins/upload', fd, { headers: { 'Content-Type': 'multipart/form-data' } }) },
  toggle: (id: string, is_active: boolean) => client.put(`/plugins/${id}/toggle`, { is_active }),
  delete: (id: string) => client.delete(`/plugins/${id}`),
}

// ======== ACL ========
export const aclApi = {
  list: () => client.get('/acl'),
  create: (data: AddACLRuleReq) => client.post('/acl', data),
  delete: (id: number) => client.delete(`/acl/${id}`),
}

// ======== Chat Areas ========
export const chatAreaApi = {
  list: () => client.get('/chat-areas'),
}

// ======== Chat Records ========
export const chatRecordApi = {
  list: (chatAreaID: string, params?: { limit?: number; offset?: number; role?: string }) => client.get(`/chat-records/${chatAreaID}`, { params }),
  tokenUsage: (chatAreaID: string) => client.get(`/chat-records/${chatAreaID}/token-usage`),
}

// ======== Overview ========
export const overviewApi = {
  get: () => client.get('/overview'),
}

// ======== T2I ========
export const t2iApi = {
  getConfig: () => client.get('/t2i/config'),
  updateConfig: (data: UpdateT2IConfigReq) => client.put('/t2i/config', data),
  health: () => client.get('/t2i/health'),
}

// ======== Sandbox ========
export const sandboxApi = {
  getConfig: () => client.get('/sandbox/config'),
  updateConfig: (data: UpdateSandboxConfigReq) => client.put('/sandbox/config', data),
  health: () => client.get('/sandbox/health'),
}

// ======== Webhook ========
export const webhookApi = {
  getConfig: () => client.get('/webhook/config'),
  updateConfig: (data: UpdateWebhookConfigReq) => client.put('/webhook/config', data),
}

// ======== Logs ========
export const logApi = {
  list: () => client.get('/logs'),
}

// ======== Background Tasks ========
export interface BackgroundTaskResp {
  id: string
  chat_area_id: string
  status: string
  message_type: string
  target_id: number
  user_prompt: string
  steps: Record<string, any>
  results: Record<string, any>
  created_at: string
  updated_at: string
}

export const backgroundTaskApi = {
  list: () => client.get('/background-tasks'),
  get: (id: string) => client.get(`/background-tasks/${id}`),
}

// ======== CronJob ========
export interface CronJobResp {
  id: string; name: string; cron_expr: string; message: string; message_type: string
  target_id: number; is_active: boolean; last_run_at?: string; last_error: string
  created_at: string; updated_at: string
}
export interface AddCronJobReq {
  name: string; cron_expr: string; message: string; message_type: string
  target_id: number; is_active: boolean
}

export const cronJobApi = {
  list: () => client.get('/cronjobs'),
  get: (id: string) => client.get(`/cronjobs/${id}`),
  create: (data: AddCronJobReq) => client.post('/cronjobs', data),
  update: (id: string, data: AddCronJobReq) => client.put(`/cronjobs/${id}`, data),
  delete: (id: string) => client.delete(`/cronjobs/${id}`),
  toggle: (id: string, is_active: boolean) => client.put(`/cronjobs/${id}/toggle`, { is_active }),
}
