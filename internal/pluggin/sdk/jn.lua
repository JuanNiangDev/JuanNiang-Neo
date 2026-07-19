-- ====================================================================
-- JuanNiang-Neo Lua Plugin SDK
-- ====================================================================
-- 该文件由 Go 二进制内嵌 (//go:embed sdk/jn.lua)，启动时落盘到
-- data/pluggins/sdk/jn.lua。插件通过 `local jn = require("jn")` 引入，
-- 即可在支持 Lua Language Server (sumneko) 的 IDE 中获得完整代码提示。
--
-- SDK 仅捕获 Go 注入的全局表 (log/json/onebot11/http/.../agent) 并重新
-- 暴露为模块字段，不引入额外行为。命令注册通过 jn.command.register
-- 委托到 Go 侧的 __jn_internal.register_command 实现。
-- ====================================================================

local M = {}

-- ====================================================================
-- 事件类型
-- ====================================================================

---@class jn.Event OneBot11 消息事件
---@field post_type string 事件类型 ("message")
---@field message_type string "private" | "group"
---@field user_id number 发送者 QQ 号
---@field group_id number 群号 (private 时为 0)
---@field raw_message string 原始消息文本
---@field admins string[] 系统管理员 QQ 号列表
---@field webhook table? webhook 事件专属字段

-- ====================================================================
-- log 日志
-- ====================================================================

---@class jn.Logger
---@field info fun(msg: string) 记录 INFO 级日志
---@field warn fun(msg: string) 记录 WARN 级日志
---@field error fun(msg: string) 记录 ERROR 级日志
M.log = log

-- ====================================================================
-- json 编解码
-- ====================================================================

---@class jn.JSON
---@field encode fun(value: any): string 将 Lua 值编码为 JSON 字符串
---@field decode fun(str: string): any 将 JSON 字符串解码为 Lua 值
M.json = json

-- ====================================================================
-- onebot11 OneBot11 协议接口 (需要 onebot11 权限)
-- ====================================================================

---@class jn.OneBot11
---@field send_private_msg fun(user_id: number, message: string): boolean, string?
---@field send_group_msg fun(group_id: number, message: string): boolean, string?
---@field delete_msg fun(message_id: number): boolean, string?
---@field get_group_info fun(group_id: number): table, string?
---@field get_group_member_list fun(group_id: number): table[], string?
---@field get_group_member_info fun(group_id: number, user_id: number): table, string?
---@field get_group_honor_info fun(group_id: number): table, string?
---@field kick_group_member fun(group_id: number, user_id: number, reject_add?: boolean): boolean, string?
---@field ban_group_member fun(group_id: number, user_id: number, duration: number): boolean, string?
---@field set_group_whole_ban fun(group_id: number, enable: boolean): boolean, string?
---@field set_group_card fun(group_id: number, user_id: number, card: string): boolean, string?
---@field handle_friend_request fun(flag: string, approve: boolean, remark: string): boolean, string?
---@field handle_group_request fun(flag: string, sub_type: string, approve: boolean, reason: string): boolean, string?
---@field get_login_info fun(): table, string?
---@field get_stranger_info fun(user_id: number): table, string?
---@field get_friend_list fun(): table[], string?
---@field get_group_list fun(): table[], string?
---@field send_like fun(user_id: number, times: number): boolean, string?
---@field get_status fun(): table, string?
---@field get_version_info fun(): table, string?
M.onebot11 = onebot11

-- ====================================================================
-- http HTTP 请求 (需要 http 权限)
-- ====================================================================

---@class jn.HTTPResponse
---@field status number HTTP 状态码
---@field body string 响应正文

---@class jn.HTTP
---@field get fun(url: string): jn.HTTPResponse, string?
---@field post fun(url: string, content_type?: string, body?: string): jn.HTTPResponse, string?
M.http = http

-- ====================================================================
-- database 数据库访问 (需要 database 权限，表名自动加 pluggin_<name>_ 前缀)
-- ====================================================================

---@class jn.Database
---@field query fun(sql: string): table[], string?
---@field exec fun(sql: string): number, string?
M.database = database

-- ====================================================================
-- cache Redis 缓存 (需要 cache 权限，key 自动加 pluggin:<name>: 前缀)
-- ====================================================================

---@class jn.Cache
---@field get fun(key: string): any
---@field set fun(key: string, value: any, ttl?: number): boolean, string?
---@field del fun(key: string): boolean, string?
---@field exists fun(key: string): number
M.cache = cache

-- ====================================================================
-- t2i 文生图 (需要 t2i 权限)
-- ====================================================================

---@class jn.T2I
---@field generate fun(html: string): string, string? 生成图片，返回图片 ID
---@field generate_url fun(html: string): string, string? 生成图片，返回 URL
---@field toggle fun(active: boolean): boolean, string? 启用/停用 T2I 服务
---@field is_active fun(): boolean
---@field get_config fun(): table, string?
M.t2i = t2i

-- ====================================================================
-- sandbox 代码沙箱 (需要 sandbox 权限)
-- ====================================================================

---@class jn.Sandbox
---@field create fun(): table, string? 返回 {sandbox_id=string, status=string}
---@field exec_shell fun(sandbox_id: string, command: string): string, number|string  返回 (output, exit_code|err)
---@field exec_python fun(sandbox_id: string, code: string): string, string 返回 (output, error_str)
---@field toggle fun(active: boolean): boolean, string? 启用/停用 Sandbox 服务
---@field is_active fun(): boolean
---@field get_config fun(): table, string?
M.sandbox = sandbox

-- ====================================================================
-- agent Agent 操作接口 (需要 agent 权限)
-- ====================================================================

---@class jn.ProviderInfo
---@field id string
---@field name string
---@field type string "text_model" | "image_model" | "embedding_model"
---@field model string
---@field active boolean

---@class jn.MCPInfo
---@field id string
---@field name string
---@field url string
---@field active boolean

---@class jn.ToolInfo
---@field name string
---@field description string
---@field builtin boolean
---@field long_running boolean
---@field active boolean

---@class jn.ChatArea
---@field post_type string
---@field message_type string
---@field user_id number
---@field group_id number
---@field chat_area_id string

---@class jn.Agent
---@field get_providers fun(): table[], string?
---@field get_mcp_servers fun(): table[], string?
---@field get_skills fun(): table[], string?
---@field get_sessions fun(): table[], string?
---@field get_prompts fun(): table[], string?
---@field get_tools fun(): table[], string?
---@field get_plugins fun(): table[], string?
---@field set_provider_active fun(id: string, active: boolean): boolean, string?
---@field set_mcp_active fun(id: string, active: boolean): boolean, string?
---@field list_mcps fun(): table[], string?
---@field toggle_mcp fun(id: string, active: boolean): boolean, string?
---@field list_tools fun(): table[], string?
---@field toggle_tool fun(name: string, active: boolean): boolean, string?
---@field list_runtime_providers fun(): table[], string?
---@field switch_provider fun(id: string): boolean, string?
---@field get_current_chat_area fun(): jn.ChatArea
---@field compact_memory fun(): string, string?
M.agent = agent

-- ====================================================================
-- command 多级命令注册
-- ====================================================================

---@class jn.CommandOpts
---@field description string 命令描述（用于 /help）
---@field usage string 用法示例 (如 "/system provider switch <id>")

---@class jn.Command
-- 注册一条命令。path 可以是字符串 ("foo bar") 或字符串数组 ({"foo", "bar"})。
-- handler 签名: function(args: string[], event: jn.Event): consumed: boolean, reply: string?
--   - args: 命令路径之后的所有空格分隔参数
--   - event: 触发命令的事件上下文
--   - consumed: 是否消费此命令 (true 跳过 Agent 处理)
--   - reply: 若非空，由系统自动回复给用户
---@field register fun(path: string|string[], handler: fun(args: string[], event: jn.Event):boolean, string?, opts?: jn.CommandOpts): boolean, string?
M.command = {
    register = function(path, handler, opts)
        if __jn_internal and __jn_internal.register_command then
            return __jn_internal.register_command(path, handler, opts or {})
        end
        return false, "command API not available"
    end,
}

return M
