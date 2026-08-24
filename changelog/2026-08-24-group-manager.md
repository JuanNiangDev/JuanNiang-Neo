# 2026-08-24 群管理系统功能（feat-groupmgr）

## 概述

把 `redrock_group_manager` Lua 插件升级为与定时任务/摸鱼人日历同级的 **Go 原生系统功能**（`internal/agent/groupmgr`），并将违禁言论检测从"关键词 + LLM 复核"升级为 **RAG 语义核实（首选）+ 关键词降级 + LLM 审核兜底 + 学习闭环**。

## 检测链路（Phase 0.5 闸门）

检测闸门位于事件循环 **Phase 0（幂等去重）之后、Phase 1（Lua 插件派发）之前**，系统级优先于所有插件：

```
群聊消息 → 白名单/管理员/排除群豁免
  │
  ├─ 推荐卡片文本化 → 与普通文本统一处理（不再直罚）
  ├─ RAG 语义核实（第一核实人，同步 ≤1s，三档阈值）
  │    ├─ 高置信 ≥ high_score        → 直接处罚（无词命中同样直罚）
  │    ├─ 模棱两可 (low, high)       → LLM 审核（LLM 异常按 fallback_score 分数兜底）
  │    └─ 低置信 ≤ low_score
  │         ├─ 有词/卡片命中 → LLM 终审（词是硬信号）
  │         └─ 无硬信号     → 放行
  ├─ RAG 不可用 → 降级关键词路径（= 旧插件行为：敏感/黑词高危复核、灰词常规审查）
  └─ LLM 判定违规 → 三级惩罚 + 学习闭环（样本入库 + RAG Upsert，越用越准）
```

- 三档阈值（`high_score` 0.75 / `low_score` 0.5 / `fallback_score` 0.6）面板可调
- 推荐卡片不再直罚，走 RAG 核实 / LLM 复核（RAG 降级时 LLM 挂才回退直罚）

## 新增内容

### 数据表（全部新表，零迁移）
`group_mgr_configs`（含排除群 ID 列表 + 三份 LLM 提示词）、`group_mgr_words`（黑/灰/敏感三分类词条）、`group_mgr_samples`（RAG 违规样本 + 命中计数）、`group_mgr_violations`、`group_mgr_whitelist`、`group_mgr_admins`、`group_mgr_stats`（kv）。

### 功能平移（与旧插件对齐）
- 三级惩罚（撤回+警告 → 禁言 30min → 踢出，失败保留并通知管理员）
- 图片刷屏（窗口/阈值/警告→禁言）、+1 复读、入群统计
- 白名单豁免 / 管理员豁免（系统/手动/群角色 owner+admin）
- 管理员通知队列（随机延迟 5~30s 防风控 + 群名缓存）
- 词库 `go:embed` 种子导入（首次启动，之后热更新走 DB）

### RAG 集成
- `ragtag.Word/Sample` v5 派生 tag（`w:`/`s:` 前缀，与知识 `k:`/记忆 `m:` 隔离）
- 词条 txt 导入/新增时 RAG 可用则同步写向量库，不可用静默跳过
- 「同步向量库」按钮：词条 + 样本全量批量 upsert（幂等）
- 样本删除双删 RAG；学习闭环 LLM 确认违规自动入库

### 系统命令（后注册覆盖插件同名命令）
`/groupstats`、`/白名单`、`/豁免`、`/解除豁免`、`/取消豁免`（均仅管理员，话术与旧插件一致）。

### REST API + Web
- `/group-mgr/*`：config / words（增删查 + txt 导入）/ sync-rag / samples / violations / whitelist / admins / stats / test（链路测试不处罚不写库）
- `/memory/sync-rag`：长期记忆手动全量同步向量库（补齐 Compact 双写前的历史记忆）
- `GroupMgrPage.vue`：阈值/开关/排除群、提示词编辑（可恢复默认）、词库三分类 tab + 导入 + 同步、白名单/管理员/违规记录、统计面板、样本库、链路测试流水、旧插件启用警告横幅
- `MemoryPage.vue`：「同步向量库」按钮
- RAGPage 的 `/info` 服务信息展示已在 RAG-Service 集成时实现（本次验证确认）

## 迁移注意事项

1. **部署时需停用旧 Lua 插件 `redrock_group_manager`**（Plugin 管理页），否则双重检测重复处罚；GroupMgrPage 顶部会显示警告横幅
2. RAG-Service 未配置/不可达时全部静默降级为关键词路径（= 接入前行为），不报错
3. 旧插件的 SQLite kv 与 config.yaml 不迁移（统计/刷屏状态为一次性数据；违规记录与白名单需重新录入）

## 验证

- `go build/vet ./...` + `go test ./...` 全通过
- 单元测试：词库解析/卡片检测/参数解析纯函数；TestViolation 链路（sqlite + mock RAG server 覆盖高分直罚/中分审核/低分放行/降级关键词四档）；DAO 幂等
- `cd web && npm run typecheck` 通过
