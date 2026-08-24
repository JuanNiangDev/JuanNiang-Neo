# Changelog 2026-08-23

## 新功能

### 回复策略收敛为仅按相关性回复
- 删除 `never_reply` / `at_only` / `always` 三种策略，仅保留 `relevance`：@/命令/提及名字必回（规则快路径，零 LLM 调用），其余群聊消息由 LLM 按相关性判断（批量合并判断 + Redis 缓存/冷却 + 刷屏降级）
- `ReplyStrategyConfig` 默认值改为 `relevance`，启动时幂等迁移存量行（表结构不变，`strategy` 字段保留供兼容）
- API `PUT /reply-strategy` 不再接受 `strategy` 字段（阈值/超时/失败策略等参数校验保留）；Web 面板移除策略单选，直接展示相关性配置
- 移除 always 专属的 `skipSilenceCheck`：群聊静默响应（`__NO_REPLY__` / 静默短语）始终检测丢弃

### 聊天黑名单移除管理员豁免
- `filterBlockedEvents` 不再对 Admins 列表豁免：被 ban 的 QQ 号（含管理员）一律无法使用 Agent 循环（插件拦截阶段不受影响）
- `isAdmin` 保留供 admin_only 工具权限校验（`eino_middleware`）、插件与 API 层使用

### 插件发消息支持合并转发
- 插件新增 `onebot11.send_group_forward_msg`（异步）/ `send_group_forward_msg_sync`（同步，返回 message_id）
- 节点格式：构造节点 `{user_id=…, nickname=…, content=“文本或消息段数组”}`；引用节点 `{id=群内已有消息ID}`
- `adapter.SendGroupForwardMsg` 修正为标准 node 段序列化（`{type:"node",data:{…}}`），内容自动解析 CQ 码与图床引用

### 插件发消息支持引用回复
- `send_private_msg` / `send_group_msg` 及两个 sync 版本新增可选第 3 参数 `reply_to`（被引用消息 ID）：消息前自动插入 `reply` 段
- 字符串含 CQ 码时先解析为段数组再前插；参数非法时忽略引用并告警，对存量插件完全兼容

### 插件 HTTP 请求 API 支持代理（http/socks4/socks5）
- `http.get` / `http.post` / `http.get_async` / `http.post_async` 全部支持可选 `proxy` 参数（向后兼容，非法协议返回明确错误）
- 代理协议：`http(s)://host:port`（标准 HTTP 代理）、`socks5://[user:pass@]host:port`（`golang.org/x/net/proxy`，提升为直接依赖）、`socks4://`/`socks4a://`（自实现握手，域名目标走 socks4a）
- 按代理地址缓存 `http.Client` 复用连接池；socks 拨号时清空环境 HTTP 代理避免双代理冲突
- 异步签名兼容：`get_async(url, ctx?, headers?, proxy?)` 或 opts 表 `{proxy=…, headers=…, ctx=…}`；`post_async(url, ct?, body?, proxy?, ctx?)`

### 长期记忆对话语义召回（pg_trgm 倒排索引）
- **索引**：启动时幂等创建 `pg_trgm` 扩展 + `long_term_memory_items.content` GIN trgm 倒排索引（仅 PostgreSQL 方言；首次建索引量级为秒级-分钟级）
- **召回链路**：对话主链路由"最近 N 条"改为**按消息语义召回**——消息清洗 → 3-gram（不足退 2-gram，去停用字，限 10 个）→ gram 常量模式 LIKE OR 展开走 GIN 倒排候选（BitmapOr）→ 候选内 `similarity(content, query)` 降序取 5 条
- **保底**：gram 为空（纯表情/短消息）、候选为空（新话题）、检索异常时自动回退最近条目，召回质量不劣于旧行为
- **开关**：环境变量 `LTM_RECALL_MODE=recent` 可整体回退旧行为（灰度/故障逃生）
- 兼容性：DAO 检索谓词按方言选择（PG 用 `ILIKE`，SQLite 测试环境用 `LIKE`），单元测试覆盖回退路径

## 修复

### SQLite 测试环境不支持 ILIKE
- **问题**：长期记忆关键词检索的 `ILIKE` 是 PostgreSQL 专有语法，SQLite 内存库执行报 `near "ILIKE": syntax error`，新增测试路径一跑即崩
- **修复**：`LongTermMemoryItemDAO` 检索谓词按 `db.Dialector.Name()` 选择（`ILIKE` / `LIKE`）

### 回复策略收敛后的去重测试适配
- **问题**：去重测试依赖旧默认策略 `always`（消息必进 Agent）；策略收敛为 relevance 后，无 LLM Provider 的测试环境里"你好"类候选消息被相关性过滤丢弃，测试断言消费次数失败
- **修复**：测试消息改含"机器人"关键词，命中 `isDefinitelyRelevant` 必回快路径（与相关性判断解耦）