# Changelog 2026-08-23（RAG 集成）

## 新功能

### RAG-Service 基础设施（仿 Sandbox/T2I 可插拔模式）
- 新增 `infrastructure/rag` 客户端：功能选项构造 + handler 封装（Upsert / BatchUpsert / Search / Delete / HealthCheck / Info），对齐 JuanNiang-RAG-Service API（tag ↔ 全文，长文服务端透明分块）
- `RAGConfig` 单行配置表（`rag_configs`，**默认未启用**）+ DAO + AutoMigrate；`loadRAGFromDB` 装配，`svc.OnUpdateRAG` 热更新回调（置 nil = 降级开关）
- Web API：`GET/PUT /rag/config`、`GET /rag/health`、`GET /rag/info`（模型状态/内存/向量规模）
- 前端新增「RAG 向量」配置页（RAGPage）：配置表 + 健康检查 + Info 展示 + RAG-Service 部署提示

### 知识库接入 RAG（语义检索首选，降级 SQL 匹配）
- **tag 隔离**：知识与记忆共用同一 RAG 实例，用 UUID v5 派生 tag（`internal/core/ragtag`：`k:<id>` / `m:<id>`）避免检索互相污染，表结构零改动
- **召回升级**：对话前首选 RAG 向量检索（命中按分数排序注入 ≤5 条），未配置/失败/无命中降级现有关键词 + ILIKE 匹配（LRU 保留）；知识候选集（全量 id → v5 tag）懒构建缓存，随知识变更失效
- **双写双删**：新增/编辑知识同步 Upsert 向量（RAG 未配置静默跳过、失败仅告警），删除同步删向量
- **手动同步**：`POST /knowledge/vector-sync` 全量同步（50 条一批 BatchUpsert），知识库页面新增「同步向量库」按钮

### 长期记忆接入 RAG（语义检索首选，三级降级链）
- Compact 落库后同步 Upsert 记忆向量（`LongTermMemory.Add` 返回含 ID 条目供双写；RAG 未配置静默跳过）
- 召回降级链：**RAG 向量语义 → pg_trgm gram → 最近条目**；记忆候选集 TTL 5 分钟缓存
- 删除不做双删：`DeleteOldest` 当前无调用方（记忆只增不减），预留说明

### 插件新增 `jn.rag` API（权限 `rag`）
- `rag.add` / `rag.add_async`（写入，幂等 upsert）、`rag.search` / `rag.search_async`（查询，返回 `[{tag, score}]`）
- 异步回调 `on_rag_response(req_id, ctx, result, err)`；tag 强制校验 UUID；客户端经运行时动态获取（配置热更新即时生效）
- 契约：插件面向原始 RAG-Service，不要与知识/记忆集合的派生 tag 混用

## 说明

- RAG-Service 本体（Rust）需独立部署：`make download && cargo run --release`（默认 `127.0.0.1:3000`）
- 任何 RAG 故障都不影响主流程：最坏情况回退到接入前行为（记忆 pg_trgm / 知识 SQL 匹配）