package pluggin

import (
	"os"
	"path/filepath"
	"testing"

	"JuanNiang-Neo/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestPluginMetrics 插件自定义指标（jn.metrics）：
// 幂等注册 / 自动前缀 / 类型校验 / 非法名拒绝 / 计数跨句柄累加。
func TestPluginMetrics(t *testing.T) {
	pe := NewPluginEngine(t.TempDir(), &fakeAdapter{}, nil, nil, nil, nil, nil, nil, nil)
	// SDK 落盘：测试环境手动补齐（生产由 ensureEmbeddedAssets 写入）
	sdkDir := filepath.Join(pe.basePath, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdkDir, "jn.lua"), []byte(jnSDKSource), 0o644); err != nil {
		t.Fatal(err)
	}

	pluginDir := filepath.Join(pe.basePath, "metricstest")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "pluggin.yaml"),
		[]byte("name: metricstest\nversion: \"1.0.0\"\nentry: main.lua\nenabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 入口脚本：创建三种指标并操作；同名幂等；非法名返回错误
	mainLua := `local jn = require("jn")
c = jn.metrics.counter("msg_count", "消息计数")
c:inc()
c:add(2)
c2 = jn.metrics.counter("msg_count", "忽略help")
c2:add(3)
g = jn.metrics.gauge("online", "在线数")
g:set(5)
h = jn.metrics.histogram("latency", "耗时")
h:observe(0.5)
bad, bad_err = jn.metrics.counter("非法名", "")
assert(bad == nil and bad_err ~= nil, "非法名应返回错误")
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.lua"), []byte(mainLua), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := pe.Load("metricstest"); err != nil {
		t.Fatalf("加载插件失败: %v", err)
	}

	// gather 验证：指标名带前缀、值符合预期
	mfs, err := metrics.Gatherer().Gather()
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]float64{}
	for _, mf := range mfs {
		if len(mf.GetMetric()) == 0 {
			continue
		}
		m := mf.GetMetric()[0]
		switch mf.GetType() {
		case dto.MetricType_COUNTER:
			values[mf.GetName()] = m.GetCounter().GetValue()
		case dto.MetricType_GAUGE:
			values[mf.GetName()] = m.GetGauge().GetValue()
		case dto.MetricType_HISTOGRAM:
			values[mf.GetName()] = float64(m.GetHistogram().GetSampleCount())
		}
	}
	if v := values["juanniang_plugin_metricstest_msg_count"]; v != 6 { // inc(1)+add(2)+add(3)
		t.Fatalf("counter 应累计 6，got %v", v)
	}
	if v := values["juanniang_plugin_metricstest_online"]; v != 5 {
		t.Fatalf("gauge 应为 5，got %v", v)
	}
	if v := values["juanniang_plugin_metricstest_latency"]; v != 1 {
		t.Fatalf("histogram sample 应为 1，got %v", v)
	}
}

// TestGetOrCreateAdoptTypeCheck 回归：收编已注册 collector 时须校验实际类型。
// 旧实现用 prometheus 接口断言（Gauge 接口方法集是 Counter 的超集，gauge 会被
// 误判为 counter），收编后句柄类型混淆；现在以 dto 探测类型，不符直接拒绝。
func TestGetOrCreateAdoptTypeCheck(t *testing.T) {
	// 1. 其他路径先注册 gauge，同名前缀被要求当 counter → 拒绝
	full := "juanniang_plugin_adopttest_gauge_as_counter"
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: full, Help: "h"})
	if err := metrics.Register(g); err != nil {
		t.Fatalf("预注册 gauge 失败: %v", err)
	}
	if c, err := getOrCreate(full, "h", "counter"); err == nil {
		t.Fatalf("gauge 被要求当 counter 应拒绝，got %v", c)
	}
	// 2. 同类型收编 → 成功且返回已有实例
	c, err := getOrCreate(full, "h", "gauge")
	if err != nil {
		t.Fatalf("同类型收编应成功: %v", err)
	}
	if c != prometheus.Collector(g) {
		t.Fatal("应返回已注册的同一实例")
	}
	// 3. 先注册 counter，再要求 histogram → 拒绝（防 handle 类型断言 panic）
	full2 := "juanniang_plugin_adopttest_counter_as_histogram"
	ctr := prometheus.NewCounter(prometheus.CounterOpts{Name: full2, Help: "h"})
	if err := metrics.Register(ctr); err != nil {
		t.Fatalf("预注册 counter 失败: %v", err)
	}
	if c, err := getOrCreate(full2, "h", "histogram"); err == nil {
		t.Fatalf("counter 被要求当 histogram 应拒绝，got %v", c)
	}
}

// TestPluginMetricsTypeConflict 同名不同类型注册应报错（Lua 侧返回 nil + err）。
func TestPluginMetricsTypeConflict(t *testing.T) {
	pe := NewPluginEngine(t.TempDir(), &fakeAdapter{}, nil, nil, nil, nil, nil, nil, nil)
	sdkDir := filepath.Join(pe.basePath, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdkDir, "jn.lua"), []byte(jnSDKSource), 0o644); err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(pe.basePath, "metricconflict")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "pluggin.yaml"),
		[]byte("name: metricconflict\nversion: \"1.0.0\"\nentry: main.lua\nenabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainLua := `local jn = require("jn")
c, err = jn.metrics.counter("same", "x")
assert(c ~= nil and err == nil, "counter 创建应成功")
g, gerr = jn.metrics.gauge("same", "y")
assert(g == nil and gerr ~= nil, "同名不同类型应返回错误")
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.lua"), []byte(mainLua), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pe.Load("metricconflict"); err != nil {
		t.Fatalf("加载插件失败: %v", err)
	}
}
