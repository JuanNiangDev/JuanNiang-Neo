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

-- Go 运行时注入的全局变量，IDE 无法感知，关闭 undefined-global 检查。
---@diagnostic disable: undefined-global

local M = {}

-- ====================================================================
-- 事件类型
-- ====================================================================

---@class jn.Event OneBot11 消息/通知/请求/定时/webhook 事件
---@field post_type string 事件类型 ("message" | "notice" | "request" | "timer" | "webhook")
---@field message_type string "private" | "group"
---@field user_id number 发送者/操作者 QQ 号
---@field group_id number 群号
---@field raw_message string 原始消息文本
---@field admins string[] 系统管理员 QQ 号列表
---@field webhook table? webhook 事件专属字段
---@field payload table? on_timer_call 携带的 CronJob payload
---
--- notice 事件字段:
---@field notice_type string? 通知类型: group_upload / group_admin / group_decrease / group_increase / group_ban / friend_add / group_recall / friend_recall / notify
---@field sub_type string? 子类型
---@field operator_id number? 操作者 QQ
---@field target_id number? 被操作者 QQ（禁言/踢人等）
---@field duration number? 禁言时长（秒）
---@field file table? 群文件上传信息 {id, name, size, busid}
---
--- request 事件字段:
---@field request_type string? 请求类型: friend / group
---@field comment string? 验证消息
---@field flag string? 请求标识（用于同意/拒绝）
---
--- message 事件附加字段:
---@field message_id number? 消息 ID
---@field sender table? 发送者信息 {user_id, nickname, sex, age, card}
---
--- 消息段格式 (用于 send_private_msg / send_group_msg 第二个参数):
--- 传入 Lua 数组，每项为 {type="text|image|face|at|...", data={...}}:
---   type="text"   → data={text="Hello"}
---   type="image"  → data={file="http://..."}  支持 URL / base64:// / 相对路径
---   type="face"   → data={id="1"}       CQ 表情 ID
---   type="at"     → data={qq="123456"}  @某人（仅群聊有效）
---
--- 图片 file 字段支持三种来源:
---   1. URL:     "http://..." 或 "https://..."
---   2. Base64:  "base64://..."
---   3. 相对路径: "img/photo.png" → 自动从插件目录读取并转 base64
---   (也可用 jn.onebot11.read_file_base64(path) 手动读取)

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
---@field send_private_msg fun(user_id: number, message: string|table): boolean, string? 异步发送私聊消息，不阻塞
---@field send_group_msg fun(group_id: number, message: string|table): boolean, string? 异步发送群消息，不阻塞
---@field send_private_msg_sync fun(user_id: number, message: string|table): boolean, string? 同步发送私聊消息，等待结果
---@field send_group_msg_sync fun(group_id: number, message: string|table): boolean, string? 同步发送群消息，等待结果
---@field delete_msg fun(message_id: number|string): boolean, string? 撤回消息（事件表的 message_id 为字符串）
---@field get_msg fun(message_id: number|string): table, string? 根据消息 ID 获取消息完整内容
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
---@field get_async fun(url: string, ctx?: table): number 异步 GET，立即返回 req_id；完成后引擎调用插件入口 on_http_response(req_id, ctx, result, err)
---@field post_async fun(url: string, content_type?: string, body?: string, ctx?: table): number 异步 POST（最后一个 table 参数视为 ctx）
M.http = http

-- 异步回调约定（引擎级异步注册表，kind "http"）：
-- 插件定义全局函数 on_http_response(req_id, ctx, result, err)，引擎在请求完成后
-- 串行调用（与事件派发互斥）。result={status, body}；err 为 nil 表示成功。
-- ctx 为调用时传入的现场表（原样带回，未传则 nil），用于延续调用前的业务状态。

-- ====================================================================
-- database 数据库访问 (需要 database 权限，表名自动加 pluggin_<name>_ 前缀)
-- ====================================================================

---@class jn.Database
---@field query fun(sql: string): table[], string?
---@field exec fun(sql: string): number, string?
M.database = database

-- ====================================================================
-- sql 工具：直接拼接 SQL 时的字符串转义
-- ====================================================================
-- jn.database 只暴露裸 SQL 接口（query/exec），不会自动转义。
-- 凡把用户输入拼进 SQL 字符串字面量，都必须先用 sql_escape 转义，
-- 否则存在 SQL 注入风险。数字字段（user_id/group_id 等）用 %d 格式
-- 即可保证安全，无需转义。

---@class jn.SQL
---@field escape fun(s: any): string PostgreSQL 字符串字面量单引号转义（'→''），返回可直接放进 '...' 的安全字符串
M.sql = {
    escape = function(s)
        return tostring(s):gsub("'", "''")
    end,
}

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

--- generate / generate_url 的可选 options 表（键名与 T2I 服务 GenerateOptions 的 JSON 字段一致）：
---   type                      string   图片格式 "jpeg" | "png"（默认 png）
---   quality                   number   压缩质量（仅 jpeg 有效）
---   omit_background           boolean  透明背景（png）
---   full_page                 boolean  整页截图（默认 true；false 时按 viewport 尺寸截图）
---   viewport_width            number   视口宽度（px）
---   viewport_height           number   视口高度（px）
---   scale                     string   "css" | "device"
---   animations                string   "allow" | "disabled"
---   caret                     string   "hide" | "initial"
---   device_scale_factor_level string   "normal" | "high" | "ultra"
---   timeout                   number   渲染超时（秒）
---@class jn.T2I
---@field generate fun(html: string, options?: table): string, string? 生成图片，返回图片 ID
---@field generate_url fun(html: string, options?: table): string, string? 生成图片，返回 URL
---@field generate_async fun(html: string, options?: table, ctx?: table): number 异步生成（默认超时 120s，opts.timeout 可覆盖），立即返回 req_id；完成后引擎调用插件入口 on_t2i_response(req_id, ctx, result, err)
---@field generate_url_async fun(html: string, options?: table, ctx?: table): number 异步生成 URL
---@field toggle fun(active: boolean): boolean, string?
---@field is_active fun(): boolean
---@field get_config fun(): table, string?
M.t2i = t2i

-- 异步回调约定（引擎级异步注册表，kind "t2i"）：
-- 插件定义全局函数 on_t2i_response(req_id, ctx, result, err)，引擎在渲染完成后
-- 串行调用（与事件派发互斥）。result 为图片 ID 或公开 URL；err 为 nil 表示成功。
-- ctx 为调用时传入的现场表（原样带回，未传则 nil）。

-- ====================================================================
-- sandbox 代码沙箱 (需要 sandbox 权限)
-- ====================================================================

---@class jn.Sandbox
---@field create fun(): table, string? 返回 {sandbox_id=string, status=string}
---@field exec_shell fun(sandbox_id: string, command: string): string, number|string  返回 (output, exit_code|err)
---@field exec_python fun(sandbox_id: string, code: string): string, string 返回 (output, error_str)
---@field create_async fun(ctx?: table): number 异步创建，立即返回 req_id；完成后引擎调用插件入口 on_sandbox_response(req_id, ctx, result, err)
---@field exec_shell_async fun(sandbox_id: string, command: string, ctx?: table): number 异步执行 shell（默认超时 120s）
---@field exec_python_async fun(sandbox_id: string, code: string, ctx?: table): number 异步执行 python
---@field list fun(): table[], string? 列出已有沙箱实例
---@field delete fun(sandbox_id: string): boolean, string? 删除指定沙箱
---@field toggle fun(active: boolean): boolean, string? 启用/停用 Sandbox 服务
---@field is_active fun(): boolean
---@field get_config fun(): table, string?
M.sandbox = sandbox

-- 异步回调约定（引擎级异步注册表，kind "sandbox"）：
-- 插件定义全局函数 on_sandbox_response(req_id, ctx, result, err)，引擎在执行完成后
-- 串行调用（与事件派发互斥）。result 按调用方法不同：create→{sandbox_id,status}、
-- exec_shell→{output,exit_code}、exec_python→{output,error}；err 为 nil 表示成功。
-- ctx 为调用时传入的现场表（原样带回，未传则 nil）。

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
-- llm LLM 调用 (需要 llm 权限)
-- ====================================================================
-- 通过 Bot 自身启用的文本模型 Provider 调用 LLM：模型 / 采样参数 / 密钥
-- 全部复用 Bot 配置，插件不接触任何密钥。适合内容审查等二次判断场景。
-- 高频路径请使用 chat_async（异步，不阻塞事件循环与其它插件）。

---@class jn.LLM
---@field available fun(): boolean 当前是否有可用的文本模型 Provider
---@field chat fun(messages: string|table, opts?: table): string?, string? 同步调用，返回 (content, err)；适合命令等低频路径
---@field chat_async fun(messages: string|table, opts?: table): number 异步提交，立即返回 req_id（失败返回 0）；完成后引擎调用插件入口 on_chat_response(req_id, content, err)
---@field messages table 消息参数：单字符串（role=user）或数组，元素为字符串（role=user）或 {role="system|user|assistant", content="..."}
---@field opts table 选项：{temperature=?, max_tokens=?, timeout=?秒}，缺省回退 Bot Provider 配置（默认超时 60s）
---
--- 异步回调约定（引擎级异步注册表，kind "chat"）：
--- 插件定义全局函数 on_chat_response(req_id, content, err)，引擎在 LLM 返回后
--- 串行调用（与事件派发互斥）。err 为 nil 表示成功；req_id 与 chat_async 的
--- 返回值一致，可用于关联请求上下文（如查表取回本次审查的事件/关键词）。
M.llm = llm

-- ====================================================================
-- config 动态配置 (无需权限，默认注入)
-- ====================================================================

---@class jn.ConfigItem
---@field key string 配置键
---@field type string "bool" | "string" | "list"
---@field label string 展示名
---@field description string? 说明
---@field default any 默认值
---@field value any 当前值

---@class jn.Config
---@field get fun(key: string): any 读取配置值（value 优先，回退 default）
---@field all fun(): table<string, any> 读取全部配置键值
---@field schema fun(): jn.ConfigItem[] 读取完整 schema
M.config = config

-- ====================================================================
-- file 插件目录内文本文件读写 (需要 file 权限)
-- ====================================================================
-- 所有路径均相对于插件自身目录 (data/pluggins/<插件名>/)，禁止绝对路径
-- 与 .. 越权访问。适用于 txt/json/log/csv 等文本文件。
-- 行号均为 1 起；read_line 越界返回 nil（非错误，可用于循环读取）。

---@class jn.File
---@field read fun(path: string): string?, string? 读取整个文件内容
---@field read_lines fun(path: string): string[]?, string? 读取全部行（自动去除行尾换行符）
---@field read_line fun(path: string, line: number): string?, string? 读取第 N 行；越界返回 nil
---@field write fun(path: string, content: string): boolean, string? 覆盖写入整个文件（自动创建目录）
---@field write_lines fun(path: string, lines: string[]): boolean, string? 覆盖写入多行（每行自动补 \n）
---@field write_line fun(path: string, line: number, content: string): boolean, string? 改写第 N 行（不足补空行）
---@field append fun(path: string, content: string): boolean, string? 追加内容到文件末尾（不自动补换行）
---@field append_line fun(path: string, content: string): boolean, string? 追加一行（末尾无换行时自动补）
---@field exists fun(path: string): boolean 判断文件是否存在
---@field remove fun(path: string): boolean, string? 删除文件
M.file = file

-- ====================================================================
-- command 多级命令注册
-- ====================================================================

---@class jn.CommandOpts
---@field description string 命令描述（用于 /help）
---@field usage string 用法示例 (如 "/system provider switch <id>")

---@alias jn.CommandHandler fun(args: string[], event: jn.Event):boolean, string?

---@class jn.Command
-- 注册一条命令。path 可以是字符串 ("foo bar") 或字符串数组 ({"foo", "bar"})。
-- handler 签名: function(args: string[], event: jn.Event): consumed: boolean, reply: string?
--   - args: 命令路径之后的所有空格分隔参数
--   - event: 触发命令的事件上下文
--   - consumed: 是否消费此命令 (true 跳过 Agent 处理)
--   - reply: 若非空，由系统自动回复给用户
---@field register fun(path: string|string[], handler: jn.CommandHandler, opts?: jn.CommandOpts): boolean, string?
M.command = {
    register = function(path, handler, opts)
        if __jn_internal and __jn_internal.register_command then
            return __jn_internal.register_command(path, handler, opts or {})
        end
        return false, "command API not available"
    end,
}

return M
