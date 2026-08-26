package pluggin

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"JuanNiang-Neo/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	lua "github.com/yuin/gopher-lua"
)

// 插件自定义指标（jn.metrics，无需权限默认注入）：
//
//   - 指标名自动加前缀 `juanniang_plugin_<插件名>_`（插件内只写短名），
//     插件名非法字符（如连字符/中文）自动转 `_`，杜绝跨插件命名冲突
//   - 幂等注册：同名同类型返回已有句柄（计数跨插件重载延续，避免重复注册 panic）；
//     同名不同类型返回错误
//   - 注册后常驻（不随插件卸载注销）：/metrics 暴露指标不出现空洞
//
// 支持类型：counter（inc/add）/ gauge（set/inc/add）/ histogram（observe）。
// 指标名仅允许 [a-zA-Z_][a-zA-Z0-9_]*（Prometheus 命名规范）。

var (
	pluginMetricMu sync.Mutex
	// pluginMetrics 进程级注册表：全量指标名（含前缀）→ collector。
	// 跨插件 LState 共享：同名指标幂等返回同一实例，计数延续。
	pluginMetrics = map[string]prometheus.Collector{}
)

// metricNameRe 插件侧指标名（短名）校验：Prometheus 命名规范。
var metricNameRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// sanitizeMetricSegment 插件名 → 指标名安全段（非法字符转 _）。
func sanitizeMetricSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// pluginMetricName 全量指标名：juanniang_plugin_<插件>_<短名>。
func pluginMetricName(pluginName, name string) string {
	return "juanniang_plugin_" + sanitizeMetricSegment(pluginName) + "_" + name
}

// metricKindMatches 校验已注册 collector 的类型与请求一致。
func metricKindMatches(c prometheus.Collector, kind string) bool {
	switch kind {
	case "counter":
		_, ok := c.(prometheus.Counter)
		return ok
	case "gauge":
		_, ok := c.(prometheus.Gauge)
		return ok
	case "histogram":
		_, ok := c.(prometheus.Histogram)
		return ok
	}
	return false
}

// getOrCreate 幂等注册：已存在同类型返回已有；类型冲突/注册失败返回错误。
func getOrCreate(fullName, help, kind string) (prometheus.Collector, error) {
	pluginMetricMu.Lock()
	defer pluginMetricMu.Unlock()

	if c, ok := pluginMetrics[fullName]; ok {
		if !metricKindMatches(c, kind) {
			return nil, fmt.Errorf("指标 %q 已以其他类型注册", fullName)
		}
		return c, nil
	}

	var c prometheus.Collector
	switch kind {
	case "counter":
		c = prometheus.NewCounter(prometheus.CounterOpts{Name: fullName, Help: help})
	case "gauge":
		c = prometheus.NewGauge(prometheus.GaugeOpts{Name: fullName, Help: help})
	case "histogram":
		c = prometheus.NewHistogram(prometheus.HistogramOpts{Name: fullName, Help: help})
	}
	if err := metrics.Register(c); err != nil {
		// 并发竞态下另一 goroutine 刚注册：收编已有实例（幂等语义）
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing := already.ExistingCollector
			pluginMetrics[fullName] = existing
			return existing, nil
		}
		return nil, err
	}
	pluginMetrics[fullName] = c
	return c, nil
}

// injectMetrics 注入 jn.metrics 全局表（无需权限，默认所有插件可用）。
// SDK（sdk/jn.lua）通过 `local metrics = metrics` 捕获为模块字段。
func (pe *PluginEngine) injectMetrics(L *lua.LState, pluginName string) {
	create := func(kind string) lua.LGFunction {
		return func(L *lua.LState) int {
			name := L.CheckString(1)
			if !metricNameRe.MatchString(name) {
				L.Push(lua.LNil)
				L.Push(lua.LString("指标名非法：仅允许字母/数字/下划线，且以字母或下划线开头（如 msg_count）"))
				return 2
			}
			help := ""
			if L.GetTop() >= 2 && L.Get(2).Type() == lua.LTString {
				help = L.CheckString(2)
			}
			full := pluginMetricName(pluginName, name)
			c, err := getOrCreate(full, help, kind)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(newMetricHandle(L, c, kind))
			return 1
		}
	}

	mt := L.NewTable()
	L.SetFuncs(mt, map[string]lua.LGFunction{
		"counter":   create("counter"),
		"gauge":     create("gauge"),
		"histogram": create("histogram"),
	})
	L.SetGlobal("metrics", mt)
}

// newMetricHandle 指标句柄：table + 方法（闭包捕获指标对象）。
func newMetricHandle(L *lua.LState, c prometheus.Collector, kind string) *lua.LTable {
	h := L.NewTable()
	switch kind {
	case "counter":
		ctr := c.(prometheus.Counter)
		L.SetFuncs(h, map[string]lua.LGFunction{
			"inc": func(L *lua.LState) int {
				ctr.Inc()
				return 0
			},
			"add": func(L *lua.LState) int {
				v := float64(L.CheckNumber(2)) // 冒号调用 :add(n)：第 1 位是 self
				if v < 0 {
					L.Push(lua.LNil)
					L.Push(lua.LString("counter 不能加负数"))
					return 2
				}
				ctr.Add(v)
				return 0
			},
		})
	case "gauge":
		g := c.(prometheus.Gauge)
		L.SetFuncs(h, map[string]lua.LGFunction{
			"set": func(L *lua.LState) int {
				g.Set(float64(L.CheckNumber(2))) // 冒号调用 :set(n)
				return 0
			},
			"inc": func(L *lua.LState) int {
				g.Inc()
				return 0
			},
			"add": func(L *lua.LState) int {
				g.Add(float64(L.CheckNumber(2))) // 冒号调用 :add(n)
				return 0
			},
		})
	case "histogram":
		ht := c.(prometheus.Histogram)
		L.SetFuncs(h, map[string]lua.LGFunction{
			"observe": func(L *lua.LState) int {
				ht.Observe(float64(L.CheckNumber(2))) // 冒号调用 :observe(n)
				return 0
			},
		})
	}
	return h
}
