package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RuntimeProviders 运行时状态回调（main.go 装配时注入，scrape 时实时读取）。
// 所有回调必须线程安全；DB 查询类回调应使用 CachedInt 包装（避免 scrape 打 DB）。
type RuntimeProviders struct {
	// LoopsActive 当前活跃 Agent ReAct 循环数。
	LoopsActive func() int
	// ConcurrencyInUse 全局并发槽占用数。
	ConcurrencyInUse func() int
	// PluginsLoaded 已加载插件数。
	PluginsLoaded func() int
	// Inventory 业务库存（resource → 数量，如 knowledge_items/memory_items/...）。
	Inventory func() map[string]float64
	// ExternalHealth 外部服务健康 0/1（service: rag/t2i/sandbox/redis）。
	ExternalHealth func() map[string]float64
}

var runtimeProviders struct {
	sync.RWMutex
	p RuntimeProviders
}

// SetRuntimeProviders 注入运行时回调（main.go 装配；覆盖旧值）。
func SetRuntimeProviders(p RuntimeProviders) {
	runtimeProviders.Lock()
	runtimeProviders.p = p
	runtimeProviders.Unlock()
}

func getRuntimeProviders() RuntimeProviders {
	runtimeProviders.RLock()
	defer runtimeProviders.RUnlock()
	return runtimeProviders.p
}

// runtimeCollector 自定义 Collector：scrape 时从回调实时输出 gauge。
type runtimeCollector struct{}

var _ prometheus.Collector = (*runtimeCollector)(nil)

func (rc *runtimeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- loopsActiveDesc
	ch <- concurrencyDesc
	ch <- pluginsLoadedDesc
	ch <- inventoryDesc
	ch <- externalHealthDesc
}

func (rc *runtimeCollector) Collect(ch chan<- prometheus.Metric) {
	p := getRuntimeProviders()
	if p.LoopsActive != nil {
		ch <- prometheus.MustNewConstMetric(loopsActiveDesc, prometheus.GaugeValue, float64(p.LoopsActive()))
	}
	if p.ConcurrencyInUse != nil {
		ch <- prometheus.MustNewConstMetric(concurrencyDesc, prometheus.GaugeValue, float64(p.ConcurrencyInUse()))
	}
	if p.PluginsLoaded != nil {
		ch <- prometheus.MustNewConstMetric(pluginsLoadedDesc, prometheus.GaugeValue, float64(p.PluginsLoaded()))
	}
	if p.Inventory != nil {
		for resource, v := range p.Inventory() {
			ch <- prometheus.MustNewConstMetric(inventoryDesc, prometheus.GaugeValue, v, resource)
		}
	}
	if p.ExternalHealth != nil {
		for service, v := range p.ExternalHealth() {
			ch <- prometheus.MustNewConstMetric(externalHealthDesc, prometheus.GaugeValue, v, service)
		}
	}
}

var (
	loopsActiveDesc = prometheus.NewDesc(
		"juanniang_agent_loops_active", "当前活跃 Agent ReAct 循环数", nil, nil)
	concurrencyDesc = prometheus.NewDesc(
		"juanniang_agent_concurrency_in_use", "全局并发槽占用数", nil, nil)
	pluginsLoadedDesc = prometheus.NewDesc(
		"juanniang_plugins_loaded", "已加载插件数", nil, nil)
	inventoryDesc = prometheus.NewDesc(
		"juanniang_inventory", "业务库存（条目数等）",
		[]string{"resource"}, nil)
	externalHealthDesc = prometheus.NewDesc(
		"juanniang_external_health", "外部服务健康（1=健康 0=异常/未配置）",
		[]string{"service"}, nil)
)

// CachedInt 带 TTL 的整数缓存：包装耗时查询（DB count 等），
// scrape 高频拉取时避免每次打库；首个调用同步执行，之后 TTL 内复用。
func CachedInt(ttl time.Duration, fn func() (int64, error)) func() int {
	var (
		mu       sync.Mutex
		cached   int64
		cachedAt time.Time
	)
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(cachedAt) < ttl {
			return int(cached)
		}
		if v, err := fn(); err == nil {
			cached = v
			cachedAt = time.Now()
		}
		return int(cached)
	}
}

// CachedMap 带 TTL 的 map 缓存：包装多值查询（如各外部服务健康探测）。
// 失败时返回零值（服务视为 0），保证 scrape 不被慢服务阻塞。
func CachedMap(ttl time.Duration, fn func() map[string]float64) func() map[string]float64 {
	var (
		mu       sync.Mutex
		cached   map[string]float64
		cachedAt time.Time
	)
	return func() map[string]float64 {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(cachedAt) < ttl {
			return cached
		}
		cached = fn()
		cachedAt = time.Now()
		return cached
	}
}
