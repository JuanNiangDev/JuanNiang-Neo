# Changelog 2026-08-26（插件自定义指标 jn.metrics）

## 新功能

### Lua 插件自定义 Prometheus 指标（jn.metrics）
- 新增 `jn.metrics` 全局表（无需权限，默认注入）：`counter(name, help?)` / `gauge(name, help?)` / `histogram(name, help?)`
- 句柄方法：counter `inc()/add(n)`（add 拒绝负数）、gauge `set(n)/inc()/add(n)`、histogram `observe(n)`
- **自动前缀隔离**：指标名 = `juanniang_plugin_<插件名>_<短名>`（插件名非法字符转 `_`），跨插件零冲突，随 `/metrics` 暴露，Grafana 可查
- **幂等注册**：同名同类型返回已有句柄（计数跨插件重载延续，杜绝重复注册 panic）；同名不同类型返回错误
- 短名校验：仅允许 `[a-zA-Z_][a-zA-Z0-9_]*`（Prometheus 命名规范），非法返回错误
- 指标注册后常驻（卸载不注销，避免指标空洞）

### 配套
- `internal/metrics` 新增 `Register(c) error`（非 panic 注册，动态注册场景用）与 `Gatherer()` 导出
- SDK（`data/pluggins/sdk/jn.lua`）新增 `jn.Metrics`/`jn.CounterHandle`/`jn.GaugeHandle`/`jn.HistogramHandle` LuaCATS 注解
- `docs/plugin-development.md` 新增 `jn.metrics` 章节（含示例）
- 测试：幂等/前缀/类型冲突/非法名/值累加（`internal/pluggin/metrics_test.go`）

## 使用示例

```lua
local c = jn.metrics.counter("msg_count", "消息处理数")
c:inc()
local h = jn.metrics.histogram("handle_latency", "耗时")
h:observe(0.42)
```
