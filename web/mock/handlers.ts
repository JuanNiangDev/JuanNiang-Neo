import type { MockHandler } from './plugin'

// ============================================================
// Mock Data Store (in-memory, resets on HMR)
// ============================================================

const UUID = () =>
  'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })

const now = () => new Date().toISOString()

// --- Providers ---
let providers = [
  { id: UUID(), created_at: now(), name: 'OpenAI GPT-4o', type: 'text_model', endpoint: 'https://api.openai.com/v1', token: 'sk-***', model: 'gpt-4o', temperature: 0.7, is_active: true },
  { id: UUID(), created_at: now(), name: 'DALL-E 3', type: 'image_model', endpoint: 'https://api.openai.com/v1', token: 'sk-***', model: 'dall-e-3', temperature: 0, is_active: true },
  { id: UUID(), created_at: now(), name: 'OpenAI Embedding', type: 'embedding_model', endpoint: 'https://api.openai.com/v1', token: 'sk-***', model: 'text-embedding-3-small', temperature: 0, is_active: false },
]

// --- MCP Servers ---
let mcpServers = [
  { id: UUID(), name: 'File System', server_url: 'http://localhost:9000/sse', headers: {}, timeout: 30000, retry_count: 3, tool_filter: [], auto_reconnect: true, is_active: true, created_at: now() },
  { id: UUID(), name: 'Web Search', server_url: 'http://localhost:9001/sse', headers: { 'X-API-Key': '***' }, timeout: 15000, retry_count: 2, tool_filter: ['search', 'fetch'], auto_reconnect: false, is_active: false, created_at: now() },
]

// --- Prompts ---
let prompts = [
  { id: UUID(), name: 'System Prompt', content: 'You are JuanNiang, a helpful AI assistant. You are polite, knowledgeable, and always ready to help.', type: 'system', is_active: true, variables: ['user_name', 'date'], created_at: now() },
  { id: UUID(), name: 'Personality: Tsundere', content: 'You have a tsundere personality. You act cold and dismissive but secretly care about the user.', type: 'personality', is_active: false, variables: [], created_at: now() },
  { id: UUID(), name: 'Custom: Code Review', content: 'You are a senior code reviewer. Review the following code and provide constructive feedback:\n\n{{code}}', type: 'custom', is_active: true, variables: ['code'], created_at: now() },
]

// --- Sessions ---
let sessions = [
  { id: UUID(), chat_area_id: UUID(), model: 'gpt-4o', token_usage: 45600, meta_data: { greeting: true }, created_at: now() },
  { id: UUID(), chat_area_id: UUID(), model: 'gpt-4o', token_usage: 12300, meta_data: {}, created_at: now() },
  { id: UUID(), chat_area_id: UUID(), model: 'gpt-4o', token_usage: 89200, meta_data: { memory_mode: 'long' }, created_at: now() },
]

// --- Skills ---
let skills = [
  { id: UUID(), name: 'Code Execute', description: 'Execute code in sandbox', keywords: ['run', 'execute', 'code'], regex_pattern: '', prompt_refs: [prompts[2].id], tool_refs: [UUID()], mcp_refs: [], is_active: true, is_system: false, priority: 10, created_at: now() },
  { id: UUID(), name: 'File Read', description: 'Read file from filesystem via MCP', keywords: ['read', 'file', 'cat'], regex_pattern: '^/read\\s+', prompt_refs: [], tool_refs: [], mcp_refs: [mcpServers[0].id], is_active: true, is_system: false, priority: 5, created_at: now() },
  { id: UUID(), name: 'System Greeting', description: 'Auto greet new users', keywords: [], regex_pattern: '', prompt_refs: [prompts[0].id], tool_refs: [], mcp_refs: [], is_active: true, is_system: true, priority: 0, created_at: now() },
]

// --- Tools ---
const tools = [
  { id: UUID(), name: 'web_search', description: 'Search the web for information', parameters: { type: 'object', properties: { query: { type: 'string', description: 'The search query' } }, required: ['query'] }, timeout: 30000, is_active: true, is_builtin: true, created_at: now() },
  { id: UUID(), name: 'code_execute', description: 'Execute code in a sandbox', parameters: { type: 'object', properties: { language: { type: 'string' }, code: { type: 'string' } }, required: ['language', 'code'] }, timeout: 60000, is_active: true, is_builtin: true, created_at: now() },
  { id: UUID(), name: 'image_generate', description: 'Generate an image using AI', parameters: { type: 'object', properties: { prompt: { type: 'string' }, size: { type: 'string' } }, required: ['prompt'] }, timeout: 120000, is_active: false, is_builtin: true, created_at: now() },
  { id: UUID(), name: 'send_qq_message', description: 'Send a message via QQ', parameters: { type: 'object', properties: { target_id: { type: 'number' }, message: { type: 'string' } }, required: ['target_id', 'message'] }, timeout: 10000, is_active: true, is_builtin: false, created_at: now() },
]

// --- Plugins ---
let plugins = [
  { id: 'weather-plugin', name: 'weather-plugin', version: '1.2.0', path: 'data/pluggins/weather-plugin/', config: { api_key: '***', default_city: 'Beijing' }, is_active: true, created_at: now() },
  { id: 'translate-plugin', name: 'translate-plugin', version: '0.5.0', path: 'data/pluggins/translate-plugin/', config: {}, is_active: false, created_at: now() },
  { id: 'scheduler-plugin', name: 'scheduler-plugin', version: '1.0.1', path: 'data/pluggins/scheduler-plugin/', config: { timezone: 'Asia/Shanghai' }, is_active: true, created_at: now() },
]

// --- ACL Rules ---
let aclRules = [
  { id: 1, chat_area_id: UUID(), scope: 'chat', permission: 'allow', target_type: 'all', user_ids: [], created_at: now() },
  { id: 2, chat_area_id: UUID(), scope: 'tool', permission: 'deny', target_type: 'list', user_ids: ['123456', '789012'], created_at: now() },
  { id: 3, chat_area_id: UUID(), scope: 'mcp', permission: 'allow', target_type: 'list', user_ids: ['111111'], created_at: now() },
]
let aclIdCounter = 4

// --- Chat Areas ---
const chatAreas = [
  { id: UUID(), area_type: 'private', target_id: 123456789, created_at: now() },
  { id: UUID(), area_type: 'private', target_id: 987654321, created_at: now() },
  { id: UUID(), area_type: 'group', target_id: 555666777, created_at: now() },
  { id: UUID(), area_type: 'group', target_id: 888999000, created_at: now() },
  { id: UUID(), area_type: 'private', target_id: 111222333, created_at: now() },
]

// --- Chat Records ---
function generateChatRecords(chatAreaId: string, count: number) {
  const roles = ['user', 'assistant', 'user', 'assistant', 'tool']
  const messages = [
    '你好！今天天气怎么样？',
    '你好！根据最新的气象数据，今天北京天气晴朗，温度在 22°C 到 30°C 之间，适合户外活动。',
    '帮我查一下最新的新闻',
    '好的，我通过搜索工具为您找到了以下新闻...',
    '',
    '请帮我生成一张猫咪的图片',
    '抱歉，图片生成功能当前未启用。您可以先在 T2I 设置中配置图片生成服务。',
  ]
  const records: any[] = []
  for (let i = 0; i < count; i++) {
    const role = roles[i % roles.length]
    records.push({
      id: i + 1,
      chat_area_id: chatAreaId,
      user_id: role === 'user' ? 123456789 : 0,
      role,
      content: messages[i % messages.length] || `Message ${i + 1}`,
      token_count: Math.floor(Math.random() * 500) + 10,
      tool_calls: role === 'tool' ? { name: 'web_search', result: '...' } : null,
      created_at: new Date(Date.now() - (count - i) * 60000).toISOString(),
    })
  }
  return records
}

// --- Adapter ---
let adapterState = {
  running: true,
  listen_addr: '0.0.0.0:8080',
  self_id: 1234567890,
  conn_count: 2,
  conn_ids: [111222333, 444555666],
}

// --- Adapter Config ---
let adapterConfig = {
  addr: '0.0.0.0',
  port: 8080,
  token: 'onebot-token-123',
  admin_qq_numbers: ['123456789'],
  enabled: true,
}

// --- Memory configs cache ---
const memoryConfigs: Record<string, any> = {}

// --- T2I ---
let t2iConfig = { base_url: 'http://localhost:7860', timeout: 120000, is_active: false, healthy: false }

// --- Sandbox ---
let sandboxConfig = { base_url: 'http://localhost:8888', api_key: '***', timeout: 60000, is_active: true, healthy: true }

// --- Webhook ---
let webhookConfig = { addr: '0.0.0.0', port: 8099, token: 'webhook-token', enabled: false, running: false }

// --- Auth mock ---
const VALID_TOKEN = 'mock-jwt-token-juanniang'

// ============================================================
// Response helpers
// ============================================================
const ok = (data: any = null) => ({ status: 0, info: 'OK', data })
const err = (status: number, info: string) => ({ status, info, data: null })

// ============================================================
// Route Handlers
// ============================================================
export const mockHandlers: MockHandler[] = [
  // ============ Auth ============
  {
    method: 'POST', path: '/login',
    handler({ body }) {
      if (body?.username === 'admin' && body?.password === 'Admin123') {
        return ok({ token: VALID_TOKEN })
      }
      return err(40002, '用户名或密码错误')
    }
  },
  {
    method: 'POST', path: '/change-password',
    handler({ body }) {
      if (body?.old_password === 'Admin123') {
        return ok(null)
      }
      return err(40005, '原密码错误')
    }
  },

  // ============ Adapter ============
  {
    method: 'GET', path: '/adapter',
    handler() { return ok({ ...adapterState }) }
  },
  {
    method: 'PUT', path: '/adapter',
    handler({ body }) {
      adapterConfig = { ...adapterConfig, ...body }
      adapterState = { ...adapterState, listen_addr: `${body.addr}:${body.port}`, running: body.enabled }
      return ok(null)
    }
  },
  {
    method: 'POST', path: '/adapter/restart',
    handler() {
      adapterState.running = true
      return ok(null)
    }
  },

  // ============ Providers ============
  {
    method: 'GET', path: '/providers',
    handler() { return ok(providers) }
  },
  {
    method: 'GET', path: '/providers/:id',
    handler({ params }) {
      const p = providers.find((p) => p.id === params.id)
      return p ? ok(p) : err(40009, 'provider 不存在')
    }
  },
  {
    method: 'POST', path: '/providers',
    handler({ body }) {
      if (body.isActive) {
        providers.forEach((p) => { if (p.type === body.type) p.is_active = false })
      }
      const p = { id: UUID(), created_at: now(), name: body.name, type: body.type, endpoint: body.endpoint, token: body.token, model: body.model, temperature: body.temperature ?? 0.7, is_active: body.isActive }
      providers.push(p)
      return ok(p)
    }
  },
  {
    method: 'PUT', path: '/providers/:id',
    handler({ params, body }) {
      const idx = providers.findIndex((p) => p.id === params.id)
      if (idx === -1) return err(40009, 'provider 不存在')
      if (body.isActive) {
        providers.forEach((p) => { if (p.id !== params.id && p.type === providers[idx].type) p.is_active = false })
      }
      providers[idx] = { ...providers[idx], ...body, id: params.id, created_at: providers[idx].created_at }
      return ok(providers[idx])
    }
  },
  {
    method: 'DELETE', path: '/providers/:id',
    handler({ params }) {
      providers = providers.filter((p) => p.id !== params.id)
      return ok(null)
    }
  },
  {
    method: 'PUT', path: '/providers/:id/toggle',
    handler({ params, body }) {
      const idx = providers.findIndex((p) => p.id === params.id)
      if (idx === -1) return err(40009, 'provider 不存在')
      if (body.is_active) {
        providers.forEach((p) => { if (p.id !== params.id && p.type === providers[idx].type) p.is_active = false })
      }
      providers[idx].is_active = body.is_active
      return ok(null)
    }
  },

  // ============ MCP ============
  {
    method: 'GET', path: '/mcp',
    handler() { return ok(mcpServers) }
  },
  {
    method: 'GET', path: '/mcp/:id',
    handler({ params }) {
      const m = mcpServers.find((m) => m.id === params.id)
      return m ? ok(m) : err(40010, 'MCP 服务器不存在')
    }
  },
  {
    method: 'POST', path: '/mcp',
    handler({ body }) {
      const m = { id: UUID(), name: body.name, server_url: body.server_url, headers: body.headers || {}, timeout: body.timeout || 30000, retry_count: body.retry_count || 3, tool_filter: body.tool_filter || [], auto_reconnect: body.auto_reconnect ?? true, is_active: body.is_active, created_at: now() }
      mcpServers.push(m)
      return ok(m)
    }
  },
  {
    method: 'PUT', path: '/mcp/:id',
    handler({ params, body }) {
      const idx = mcpServers.findIndex((m) => m.id === params.id)
      if (idx === -1) return err(40010, 'MCP 服务器不存在')
      mcpServers[idx] = { ...mcpServers[idx], ...body, id: params.id, created_at: mcpServers[idx].created_at }
      return ok(mcpServers[idx])
    }
  },
  {
    method: 'DELETE', path: '/mcp/:id',
    handler({ params }) {
      mcpServers = mcpServers.filter((m) => m.id !== params.id)
      return ok(null)
    }
  },
  {
    method: 'PUT', path: '/mcp/:id/toggle',
    handler({ params, body }) {
      const idx = mcpServers.findIndex((m) => m.id === params.id)
      if (idx === -1) return err(40010, 'MCP 服务器不存在')
      mcpServers[idx].is_active = body.is_active
      return ok(null)
    }
  },

  // ============ Memory ============
  {
    method: 'GET', path: '/memory/:chatAreaID/short-term',
    handler({ params }) {
      if (!memoryConfigs[`st_${params.chatAreaID}`]) {
        memoryConfigs[`st_${params.chatAreaID}`] = { id: UUID(), chat_area_id: params.chatAreaID, window_size: 20, auto_compact: false, created_at: now() }
      }
      return ok(memoryConfigs[`st_${params.chatAreaID}`])
    }
  },
  {
    method: 'PUT', path: '/memory/:chatAreaID/short-term',
    handler({ params, body }) {
      memoryConfigs[`st_${params.chatAreaID}`] = { id: UUID(), chat_area_id: params.chatAreaID, window_size: body.window_size, auto_compact: body.auto_compact, created_at: now() }
      return ok(memoryConfigs[`st_${params.chatAreaID}`])
    }
  },
  {
    method: 'GET', path: '/memory/:chatAreaID/long-term',
    handler({ params }) {
      if (!memoryConfigs[`lt_${params.chatAreaID}`]) {
        memoryConfigs[`lt_${params.chatAreaID}`] = { id: UUID(), chat_area_id: params.chatAreaID, hot_area_size: 10, hot_memory_ttl: 86400, created_at: now() }
      }
      return ok(memoryConfigs[`lt_${params.chatAreaID}`])
    }
  },
  {
    method: 'PUT', path: '/memory/:chatAreaID/long-term',
    handler({ params, body }) {
      memoryConfigs[`lt_${params.chatAreaID}`] = { id: UUID(), chat_area_id: params.chatAreaID, hot_area_size: body.hot_area_size, hot_memory_ttl: body.hot_memory_ttl, created_at: now() }
      return ok(memoryConfigs[`lt_${params.chatAreaID}`])
    }
  },

  // ============ Prompts ============
  {
    method: 'GET', path: '/prompts',
    handler() { return ok(prompts) }
  },
  {
    method: 'POST', path: '/prompts',
    handler({ body }) {
      const p = { id: UUID(), name: body.name, content: body.content, type: body.type, is_active: body.is_active, variables: body.variables || [], created_at: now() }
      prompts.push(p)
      return ok(p)
    }
  },
  {
    method: 'PUT', path: '/prompts/:id',
    handler({ params, body }) {
      const idx = prompts.findIndex((p) => p.id === params.id)
      if (idx === -1) return err(40019, 'Prompt 不存在')
      prompts[idx] = { ...prompts[idx], ...body, id: params.id, created_at: prompts[idx].created_at }
      return ok(prompts[idx])
    }
  },
  {
    method: 'DELETE', path: '/prompts/:id',
    handler({ params }) {
      prompts = prompts.filter((p) => p.id !== params.id)
      return ok(null)
    }
  },
  {
    method: 'PUT', path: '/prompts/:id/toggle',
    handler({ params, body }) {
      const idx = prompts.findIndex((p) => p.id === params.id)
      if (idx === -1) return err(40019, 'Prompt 不存在')
      prompts[idx].is_active = body.is_active
      return ok(null)
    }
  },

  // ============ Sessions ============
  {
    method: 'GET', path: '/sessions',
    handler() { return ok(sessions) }
  },
  {
    method: 'GET', path: '/sessions/:id',
    handler({ params }) {
      const s = sessions.find((s) => s.id === params.id)
      return s ? ok(s) : err(40011, 'Session 不存在')
    }
  },
  {
    method: 'DELETE', path: '/sessions/:id',
    handler({ params }) {
      sessions = sessions.filter((s) => s.id !== params.id)
      return ok(null)
    }
  },

  // ============ Skills ============
  {
    method: 'GET', path: '/skills',
    handler() { return ok(skills) }
  },
  {
    method: 'POST', path: '/skills',
    handler({ body }) {
      const s = { id: UUID(), name: body.name, description: body.description || '', keywords: body.keywords || [], regex_pattern: body.regex_pattern || '', prompt_refs: body.prompt_refs || [], tool_refs: body.tool_refs || [], mcp_refs: body.mcp_refs || [], is_active: body.is_active, is_system: body.is_system ?? false, priority: body.priority ?? 0, created_at: now() }
      skills.push(s)
      return ok(s)
    }
  },
  {
    method: 'PUT', path: '/skills/:id',
    handler({ params, body }) {
      const idx = skills.findIndex((s) => s.id === params.id)
      if (idx === -1) return err(40018, 'Skill 不存在')
      skills[idx] = { ...skills[idx], ...body, id: params.id, created_at: skills[idx].created_at }
      return ok(skills[idx])
    }
  },
  {
    method: 'DELETE', path: '/skills/:id',
    handler({ params }) {
      skills = skills.filter((s) => s.id !== params.id)
      return ok(null)
    }
  },

  // ============ Tools ============
  {
    method: 'GET', path: '/tools',
    handler() { return ok(tools) }
  },
  {
    method: 'PUT', path: '/tools/:id/toggle',
    handler({ params, body }) {
      const t = tools.find((t) => t.id === params.id)
      if (!t) return err(40020, 'Tool 不存在')
      t.is_active = body.is_active
      return ok(null)
    }
  },

  // ============ Plugins ============
  {
    method: 'GET', path: '/plugins',
    handler() { return ok(plugins) }
  },
  {
    method: 'POST', path: '/plugins/upload',
    handler() { return ok({ name: 'new-plugin', status: 'loaded' }) }
  },
  {
    method: 'PUT', path: '/plugins/:id/toggle',
    handler({ params, body }) {
      const idx = plugins.findIndex((p) => p.id === params.id)
      if (idx === -1) return err(40021, 'Plugin 不存在')
      plugins[idx].is_active = body.is_active
      return ok(null)
    }
  },
  {
    method: 'DELETE', path: '/plugins/:id',
    handler({ params }) {
      plugins = plugins.filter((p) => p.id !== params.id)
      return ok(null)
    }
  },

  // ============ ACL ============
  {
    method: 'GET', path: '/acl',
    handler() { return ok(aclRules) }
  },
  {
    method: 'POST', path: '/acl',
    handler({ body }) {
      const existing = aclRules.findIndex((r) => r.chat_area_id === body.chat_area_id && r.scope === body.scope)
      const rule = { id: aclIdCounter++, chat_area_id: body.chat_area_id, scope: body.scope, permission: body.permission, target_type: body.target_type, user_ids: body.user_ids || [], created_at: now() }
      if (existing !== -1) {
        aclRules[existing] = { ...rule, id: aclRules[existing].id }
        return ok(aclRules[existing])
      }
      aclRules.push(rule)
      return ok(rule)
    }
  },
  {
    method: 'DELETE', path: '/acl/:id',
    handler({ params }) {
      aclRules = aclRules.filter((r) => r.id !== Number(params.id))
      return ok(null)
    }
  },

  // ============ Chat Areas ============
  {
    method: 'GET', path: '/chat-areas',
    handler() { return ok(chatAreas) }
  },

  // ============ Chat Records ============
  {
    method: 'GET', path: '/chat-records/:chatAreaID',
    handler({ params, query }) {
      const all = generateChatRecords(params.chatAreaID, 50)
      const limit = Number(query.limit) || 20
      const offset = Number(query.offset) || 0
      const role = query.role
      let filtered = role ? all.filter((r) => r.role === role) : all
      const total = filtered.length
      const list = filtered.slice(offset, offset + limit)
      return ok({ total, list })
    }
  },
  {
    method: 'GET', path: '/chat-records/:chatAreaID/token-usage',
    handler({ params }) {
      const s = sessions.find((s) => s.chat_area_id === params.chatAreaID)
      return ok(s || { id: UUID(), chat_area_id: params.chatAreaID, model: 'gpt-4o', token_usage: 0, meta_data: {}, created_at: now() })
    }
  },

  // ============ Overview ============
  {
    method: 'GET', path: '/overview',
    handler() {
      return ok({
        chat_area_count: chatAreas.length,
        mcp_count: mcpServers.length,
        adapter_count: 1,
        plugin_count: plugins.length,
        provider_count: providers.length,
        skill_count: skills.length,
        session_count: sessions.length,
        total_token_usage: sessions.reduce((sum, s) => sum + s.token_usage, 0),
        cpu_count: 8,
        goroutine_num: 47,
        mem_alloc_bytes: 33554432,
        mem_sys_bytes: 67108864,
        mem_heap_inuse_bytes: 16777216,
        go_version: 'go1.25',
        t2i_active: t2iConfig.is_active,
        t2i_healthy: t2iConfig.healthy,
        sandbox_active: sandboxConfig.is_active,
        sandbox_healthy: sandboxConfig.healthy,
      })
    }
  },

  // ============ T2I ============
  {
    method: 'GET', path: '/t2i/config',
    handler() { return ok({ ...t2iConfig }) }
  },
  {
    method: 'PUT', path: '/t2i/config',
    handler({ body }) {
      t2iConfig = { ...t2iConfig, ...body }
      return ok({ ...t2iConfig })
    }
  },
  {
    method: 'GET', path: '/t2i/health',
    handler() { return ok({ healthy: t2iConfig.healthy }) }
  },

  // ============ Sandbox ============
  {
    method: 'GET', path: '/sandbox/config',
    handler() { return ok({ ...sandboxConfig }) }
  },
  {
    method: 'PUT', path: '/sandbox/config',
    handler({ body }) {
      sandboxConfig = { ...sandboxConfig, ...body }
      return ok({ ...sandboxConfig })
    }
  },
  {
    method: 'GET', path: '/sandbox/health',
    handler() { return ok({ healthy: sandboxConfig.healthy }) }
  },

  // ============ Webhook ============
  {
    method: 'GET', path: '/webhook/config',
    handler() { return ok({ ...webhookConfig }) }
  },
  {
    method: 'PUT', path: '/webhook/config',
    handler({ body }) {
      webhookConfig = { ...webhookConfig, ...body, running: body.enabled }
      return ok({ ...webhookConfig })
    }
  },

  // ============ Logs ============
  {
    method: 'GET', path: '/logs',
    handler() {
      const entries = [
        { level: 'INFO', module: 'main', message: 'JuanNiang-Neo 启动中...', attrs: { version: '1.0.4' } },
        { level: 'INFO', module: 'adapter', message: 'adapter 已启动', attrs: { addr: '0.0.0.0:8081' } },
        { level: 'INFO', module: 'agent', message: 'HagoCenter 已启动', attrs: {} },
        { level: 'WARN', module: 'webhook', message: 'Webhook 配置加载失败', attrs: { err: 'EOF' }, rich: { caller_file: 'cmd/server/main.go:125', goroutines: 42 } },
        { level: 'INFO', module: 'mcp', message: 'MCP 加载完成', attrs: { count: 3 } },
        { level: 'ERROR', module: 'sandbox', message: 'Sandbox 连接失败', attrs: { err: 'connection refused', base_url: 'http://localhost:8888' }, rich: { caller_file: 'infrastructure/sandbox/handler/client.go:42', goroutines: 87 } },
        { level: 'INFO', module: 'planner', message: 'Planner 打分通过', attrs: { score: 0.65 } },
        { level: 'DEBUG', module: 'gorm', message: 'SQL', attrs: { elapsed_ms: 12, rows: 1, sql: 'SELECT * FROM skills' } },
        { level: 'WARN', module: 'drainer', message: 'SQL 慢查询', attrs: { elapsed_ms: 345, rows: 500, sql: 'SELECT * FROM chat_records' }, rich: { caller_file: 'infrastructure/postgres/client.go:83', goroutines: 56 } },
        { level: 'INFO', module: 'replyer', message: '发送文字消息成功', attrs: { target_type: 'group' } },
      ]
      return ok(entries.map((e, i) => ({
        time: new Date(Date.now() - (entries.length - i) * 30000).toISOString(),
        ...e,
      })))
    }
  },

  // ============ Reply Strategy ============
  {
    method: 'GET', path: '/reply-strategy',
    handler() { return ok({ strategy: 'always', relevance_threshold: 0.5, bot_name: '', strip_markdown: false, agent_lite: false, skip_silence_check: false }) }
  },
  {
    method: 'PUT', path: '/reply-strategy',
    handler({ body }) { return ok({ ...body }) }
  },

  // ============ Planner ============
  {
    method: 'GET', path: '/planner/config',
    handler() { return ok({ threshold: 0.30, weights: { mention: 0.35, keyword: 0.25, context: 0.20, quality: 0.10, history: 0.10 } }) }
  },
  {
    method: 'PUT', path: '/planner/config',
    handler({ body }) { return ok({ ...body }) }
  },

  // ============ Memory GC ============
  {
    method: 'GET', path: '/memory/gc',
    handler() { return ok({ enable: true, cold_threshold: 7, max_per_agent: 1000, interval_mins: 60 }) }
  },
  {
    method: 'PUT', path: '/memory/gc',
    handler({ body }) { return ok({ ...body }) }
  },
  {
    method: 'POST', path: '/memory/gc/run',
    handler() { return ok({ status: 'triggered' }) }
  },

  // ============ Splitter ============
  {
    method: 'GET', path: '/splitter/config',
    handler() { return ok({ max_segments: 5, auto_split: true, enable_typo: false, typo_rate: 0.03, strip_markdown: false }) }
  },
  {
    method: 'PUT', path: '/splitter/config',
    handler({ body }) { return ok({ ...body }) }
  },

  // ============ Learners ============
  {
    method: 'GET', path: '/learners',
    handler() { return ok({ behavior_enabled: true, expression_enabled: true, jargon_enabled: true, learn_interval: 1, max_concurrent_learn: 2 }) }
  },
  {
    method: 'PUT', path: '/learners',
    handler({ body }) { return ok({ ...body }) }
  },

  // ============ Auto BgTask ============
  {
    method: 'GET', path: '/tools/auto-bgtask',
    handler() { return ok({ marked_tools: ['send_group_msg', 'generate_image'] }) }
  },
  {
    method: 'PUT', path: '/tools/:id/mark-bgtask',
    handler({ params, body }) { return ok({ tool: params.id, is_long_running: body.is_long_running }) }
  },
]