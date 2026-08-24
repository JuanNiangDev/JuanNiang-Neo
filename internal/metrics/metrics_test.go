package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHandlerExposesMetrics /metrics 输出包含注册的指标与计数器增量。
func TestHandlerExposesMetrics(t *testing.T) {
	// 埋点若干指标（与生产调用路径一致的 API）
	EventsTotal.WithLabelValues("message").Inc()
	MessagesTotal.WithLabelValues("group").Inc()
	DedupDroppedTotal.Inc()
	BlockedTotal.WithLabelValues("blacklist").Inc()
	AgentLoopsTotal.WithLabelValues("ok").Inc()
	AgentLoopDuration.Observe(3.2)
	LLMRequestsTotal.WithLabelValues("p1", "ok").Inc()
	LLMTokensTotal.WithLabelValues("agent").Add(123)
	GroupMgrViolationsTotal.WithLabelValues("ad", "warn").Inc()
	GroupMgrDetectionsTotal.WithLabelValues("rag", "punish").Inc()
	GroupMgrRAGScore.Observe(0.8)
	PluginHookErrorsTotal.WithLabelValues("demo", "on_message").Inc()
	PluginHookDuration.WithLabelValues("demo", "on_message").Observe(0.05)
	HTTPRequestsTotal.WithLabelValues("GET", "/metrics", "200").Inc()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		`juanniang_events_total{post_type="message"} 1`,
		`juanniang_messages_total{message_type="group"} 1`,
		"juanniang_message_dedup_dropped_total 1",
		`juanniang_message_blocked_total{reason="blacklist"} 1`,
		`juanniang_agent_loops_total{outcome="ok"} 1`,
		`juanniang_llm_tokens_total{phase="agent"} 123`,
		`juanniang_groupmgr_violations_total{action="warn",category="ad"} 1`,
		`juanniang_groupmgr_detections_total{path="rag",verdict="punish"} 1`,
		`juanniang_plugin_hook_errors_total{hook="on_message",plugin="demo"} 1`,
		`juanniang_http_requests_total{method="GET",path="/metrics",status="200"} 1`,
		"go_goroutines", // Go runtime collector 存在
	} {
		if !strings.Contains(text, want) {
			t.Errorf("/metrics 输出缺少 %q", want)
		}
	}
}

// TestRuntimeCollector 运行时回调注入后 gauge 正确输出。
func TestRuntimeCollector(t *testing.T) {
	SetRuntimeProviders(RuntimeProviders{
		LoopsActive:      func() int { return 3 },
		ConcurrencyInUse: func() int { return 5 },
		PluginsLoaded:    func() int { return 7 },
		Inventory: func() map[string]float64 {
			return map[string]float64{"knowledge_items": 42}
		},
		ExternalHealth: func() map[string]float64 {
			return map[string]float64{"rag": 1, "redis": 0}
		},
	})
	t.Cleanup(func() { SetRuntimeProviders(RuntimeProviders{}) })

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	text := mustRead(t, rec)

	for _, want := range []string{
		"juanniang_agent_loops_active 3",
		"juanniang_agent_concurrency_in_use 5",
		"juanniang_plugins_loaded 7",
		`juanniang_inventory{resource="knowledge_items"} 42`,
		`juanniang_external_health{service="rag"} 1`,
		`juanniang_external_health{service="redis"} 0`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("/metrics 输出缺少 %q", want)
		}
	}
}

// TestCachedInt TTL 缓存：TTL 内复用、过期后重查。
func TestCachedInt(t *testing.T) {
	calls := 0
	fn := CachedInt(50*time.Millisecond, func() (int64, error) {
		calls++
		return int64(calls) * 10, nil
	})
	if got := fn(); got != 10 {
		t.Fatalf("首次 = %d", got)
	}
	if got := fn(); got != 10 {
		t.Fatalf("TTL 内应复用，got %d", got)
	}
	time.Sleep(60 * time.Millisecond)
	if got := fn(); got != 20 {
		t.Fatalf("TTL 过期应重查，got %d", got)
	}
}

func mustRead(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	b, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
