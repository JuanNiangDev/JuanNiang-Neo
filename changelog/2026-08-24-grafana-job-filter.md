# 2026-08-24 Grafana 面板入库 + job 过滤修复

## 背景

Grafana 面板此前以 JSON 片段形式直接提供给用户复制，未入库管理；且所有 PromQL 查询**未按 `job` 过滤**，导致在 Prometheus 采集了其他 Go 服务（`go_*`/`process_*` 为任何 Go 进程的标准指标）或其他卷娘实例时，面板串入非本实例数据（如「未连接服务」时 Goroutine/CPU/RSS/堆内存/RAG 分数分布仍有数据）。

## 变更

- 新增 `deployments/grafana/juanniang-dashboard.json`：完整 Grafana 面板（13 行 / 32 面板，覆盖全部指标）
- 所有查询加 `{job=~"$job"}` 标签过滤，新增模板变量 `job`（`label_values(up, job)`，默认 `juanniang`）——面板只展示指定采集 job 的卷娘实例数据
- 常量参考线（`vector(0.75)`/`vector(0.5)`）不受 job 过滤影响

## 使用

1. Prometheus `scrape_configs` 中卷娘 job 命名建议固定为 `juanniang`（`job_name: juanniang`）
2. Grafana 导入 `deployments/grafana/juanniang-dashboard.json`，选择 Prometheus 数据源与 `job=juanniang`
3. 确认数据来源：Prometheus 查询 `up{job="juanniang"}` 为 1 才代表卷娘 `/metrics` 正在被采集；`juanniang_groupmgr_rag_score` 仅在群管理 RAG 核实成功时写入（无假数据）
