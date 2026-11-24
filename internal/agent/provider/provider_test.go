package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ---------- URL 规则（§5.3） ----------

func TestEndpointURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  ProviderConfig
		want string
	}{
		{"base chat_completions", ProviderConfig{Endpoint: "https://api.deepseek.com"}, "https://api.deepseek.com/chat/completions"},
		{"base with /v1", ProviderConfig{Endpoint: "https://api.openai.com/v1"}, "https://api.openai.com/v1/chat/completions"},
		{"full url detected", ProviderConfig{Endpoint: "https://openwebui.local/openai/v1/chat/completions"}, "https://openwebui.local/openai/v1/chat/completions"},
		{"full url case-insensitive suffix", ProviderConfig{Endpoint: "https://x.com/openai/v1/Chat/Completions"}, "https://x.com/openai/v1/Chat/Completions"},
		{"trailing slash trimmed", ProviderConfig{Endpoint: "https://api.deepseek.com/"}, "https://api.deepseek.com/chat/completions"},
		{"anthropic base", ProviderConfig{APIMode: APIModeAnthropicMessages, Endpoint: "https://api.anthropic.com"}, "https://api.anthropic.com/messages"},
		{"anthropic full url", ProviderConfig{APIMode: APIModeAnthropicMessages, Endpoint: "https://api.deepseek.com/anthropic"}, "https://api.deepseek.com/anthropic/messages"},
		{"responses base", ProviderConfig{APIMode: APIModeOpenAIResponses, Endpoint: "https://api.deepseek.com"}, "https://api.deepseek.com/responses"},
		{"gemini model path", ProviderConfig{APIMode: APIModeGeminiNative, Endpoint: "https://generativelanguage.googleapis.com/v1beta", Model: "gemini-3-flash"}, "https://generativelanguage.googleapis.com/v1beta/models/gemini-3-flash:generateContent"},
		{"gemini full url", ProviderConfig{APIMode: APIModeGeminiNative, Endpoint: "https://x.com/models/gemini-3-flash:generateContent", Model: "gemini-3-flash"}, "https://x.com/models/gemini-3-flash:generateContent"},
		{"exact mode raw", ProviderConfig{URLMode: "exact", Endpoint: "https://custom.io/openai/v1/chat/completions"}, "https://custom.io/openai/v1/chat/completions"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := NewProvider(c.cfg).(*openAIProvider)
			if got := p.endpointURL(); got != c.want {
				t.Errorf("endpointURL() = %q, want %q", got, c.want)
			}
		})
	}
}

// ---------- 协议请求体快照（§7.2） ----------

func TestBuildRequestChatCompletions(t *testing.T) {
	p := NewProvider(ProviderConfig{
		Model: "deepseek-v4-pro", Temperature: 0.7,
		ProviderKey: "deepseek", ThinkingEffort: "high",
	}).(*openAIProvider)
	req := ChatRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "你是助手"},
			{Role: "user", Content: "你好"},
		},
		MaxTokens: 1000,
		Tools: []ToolDef{
			{Type: "function", Function: ToolDefFunc{Name: "get_weather", Description: "天气", Parameters: json.RawMessage(`{}`)}},
		},
	}
	body, err := p.buildRequest(req, false)
	if err != nil {
		t.Fatalf("buildRequest err: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal err: %v", err)
	}
	if m["model"] != "deepseek-v4-pro" {
		t.Errorf("model = %v", m["model"])
	}
	if m["max_tokens"] != float64(1000) {
		t.Errorf("max_tokens = %v", m["max_tokens"])
	}
	// deepseek thinking_object + v4 reasoning_effort
	th, ok := m["thinking"].(map[string]any)
	if !ok || th["type"] != "enabled" {
		t.Errorf("thinking = %v", m["thinking"])
	}
	if m["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %v", m["reasoning_effort"])
	}
	if _, ok := m["tools"].([]any); !ok {
		t.Errorf("tools missing")
	}
	if m["stream"] != nil {
		t.Errorf("stream should be absent for non-stream")
	}
}

// ---------- 响应解析（§7.3） ----------

func TestParseAnthropic(t *testing.T) {
	p := NewProvider(ProviderConfig{APIMode: APIModeAnthropicMessages}).(*openAIProvider)
	body := `{"content":[{"type":"text","text":"hello "},{"type":"text","text":"world"}],"usage":{"input_tokens":10,"output_tokens":5},"stop_reason":"end_turn"}`
	resp, err := p.parseResponse([]byte(body))
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if resp.Message.Content != "hello world" {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.TokenUsage != 15 {
		t.Errorf("token = %d", resp.TokenUsage)
	}
}

func TestParseGemini(t *testing.T) {
	p := NewProvider(ProviderConfig{APIMode: APIModeGeminiNative}).(*openAIProvider)
	body := `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"totalTokenCount":7}}`
	resp, err := p.parseResponse([]byte(body))
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if resp.Message.Content != "hi" {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.TokenUsage != 7 {
		t.Errorf("token = %d", resp.TokenUsage)
	}
}

func TestParseResponses(t *testing.T) {
	p := NewProvider(ProviderConfig{APIMode: APIModeOpenAIResponses}).(*openAIProvider)
	body := `{"output":[{"content":[{"type":"output_text","text":"ok"}]}],"usage":{"total_tokens":3},"status":"completed"}`
	resp, err := p.parseResponse([]byte(body))
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Errorf("content = %q", resp.Message.Content)
	}
}

// ---------- thinking 矩阵（§5.5） ----------

func TestThinkingMatrix(t *testing.T) {
	cases := []struct {
		name  string
		cfg   ProviderConfig
		check func(t *testing.T, m map[string]any)
	}{
		{"deepseek off", ProviderConfig{ProviderKey: "deepseek", ThinkingEffort: "off"}, func(t *testing.T, m map[string]any) {
			if th := m["thinking"]; th == nil || th.(map[string]any)["type"] != "disabled" {
				t.Errorf("thinking = %v", m["thinking"])
			}
		}},
		{"alibaba enabled", ProviderConfig{ProviderKey: "alibaba", ThinkingEffort: "high"}, func(t *testing.T, m map[string]any) {
			if m["enable_thinking"] != true {
				t.Errorf("enable_thinking = %v", m["enable_thinking"])
			}
		}},
		{"default reasoning_effort", ProviderConfig{ThinkingEffort: "medium"}, func(t *testing.T, m map[string]any) {
			if m["reasoning_effort"] != "medium" {
				t.Errorf("reasoning_effort = %v", m["reasoning_effort"])
			}
		}},
		{"enable_thinking legacy maps to medium", ProviderConfig{EnableThinking: true}, func(t *testing.T, m map[string]any) {
			if m["reasoning_effort"] != "medium" {
				t.Errorf("reasoning_effort = %v", m["reasoning_effort"])
			}
		}},
		{"anthropic adaptive", ProviderConfig{APIMode: APIModeAnthropicMessages, Model: "claude-opus-4-7", ThinkingEffort: "high"}, func(t *testing.T, m map[string]any) {
			if th := m["thinking"]; th == nil || th.(map[string]any)["type"] != "adaptive" {
				t.Errorf("thinking = %v", m["thinking"])
			}
		}},
		{"anthropic budget", ProviderConfig{APIMode: APIModeAnthropicMessages, Model: "claude-haiku-4-5", ThinkingEffort: "high", ThinkingBudget: 5000}, func(t *testing.T, m map[string]any) {
			th := m["thinking"].(map[string]any)
			if th["type"] != "enabled" || th["budget_tokens"] != float64(5000) {
				t.Errorf("thinking = %v", th)
			}
		}},
		{"gemini 3 level", ProviderConfig{APIMode: APIModeGeminiNative, Model: "gemini-3-flash", ThinkingEffort: "high"}, func(t *testing.T, m map[string]any) {
			tc := m["thinkingConfig"].(map[string]any)
			if tc["includeThoughts"] != true || tc["thinkingLevel"] != "high" {
				t.Errorf("thinkingConfig = %v", tc)
			}
		}},
		{"responses reasoning", ProviderConfig{APIMode: APIModeOpenAIResponses, ThinkingEffort: "high"}, func(t *testing.T, m map[string]any) {
			r := m["reasoning"].(map[string]any)
			if r["effort"] != "high" {
				t.Errorf("reasoning = %v", r)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := NewProvider(c.cfg).(*openAIProvider)
			body, err := p.buildRequest(ChatRequest{}, false)
			if err != nil {
				t.Fatalf("build err: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(body, &m); err != nil {
				t.Fatalf("unmarshal err: %v", err)
			}
			c.check(t, m)
		})
	}
}

// ---------- 参数映射优先级（§5.4） ----------

func TestParseChatCompletions(t *testing.T) {
	p := NewProvider(ProviderConfig{}).(*openAIProvider)
	body := `{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"total_tokens":4}}`
	resp, err := p.parseChatCompletions([]byte(body))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Message.Content != "hi" || resp.TokenUsage != 4 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestResolveMaxTokens(t *testing.T) {
	p := NewProvider(ProviderConfig{MaxTokens: 2000}).(*openAIProvider)
	if got := p.resolveMaxTokens(ChatRequest{}, 4096); got != 2000 {
		t.Errorf("cfg priority got %d", got)
	}
	if got := p.resolveMaxTokens(ChatRequest{MaxTokens: 3000}, 4096); got != 3000 {
		t.Errorf("req priority got %d", got)
	}
	if got := p.resolveMaxTokens(ChatRequest{}, 4096); got != 2000 {
		t.Errorf("default got %d", got)
	}
}

// ---------- 兼容性（§7.6） ----------

func TestEmptyAPIModeIsChatCompletions(t *testing.T) {
	p := NewProvider(ProviderConfig{Endpoint: "https://api.deepseek.com"}).(*openAIProvider)
	if p.APIMode() != APIModeChatCompletions {
		t.Errorf("api mode = %v", p.APIMode())
	}
	if got := p.endpointURL(); got != "https://api.deepseek.com/chat/completions" {
		t.Errorf("endpoint = %q", got)
	}
}

// ---------- 认证头（§5.2） ----------

func TestSetHeaders(t *testing.T) {
	p := NewProvider(ProviderConfig{APIMode: APIModeAnthropicMessages, Token: "tok", AuthHeader: "x-api-key"}).(*openAIProvider)
	req := mustNewRequest(t, "https://x/messages")
	p.setHeaders(req)
	if req.Header.Get("x-api-key") != "tok" {
		t.Errorf("x-api-key = %q", req.Header.Get("x-api-key"))
	}
	if req.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("version = %q", req.Header.Get("anthropic-version"))
	}
}

func TestSetHeadersGeminiDefault(t *testing.T) {
	p := NewProvider(ProviderConfig{APIMode: APIModeGeminiNative, Token: "tok"}).(*openAIProvider)
	req := mustNewRequest(t, "https://x/messages")
	p.setHeaders(req)
	if req.Header.Get("x-goog-api-key") != "tok" {
		t.Errorf("gemini key = %q", req.Header.Get("x-goog-api-key"))
	}
}

// ---------- 厂商预设 ----------

func TestProviderPresets(t *testing.T) {
	preset, ok := GetProviderPreset("deepseek")
	if !ok {
		t.Fatal("deepseek preset missing")
	}
	if len(preset.Protocols) != 3 {
		t.Errorf("deepseek protocols = %d, want 3", len(preset.Protocols))
	}
	if preset.Protocols[1].APIMode != APIModeAnthropicMessages {
		t.Errorf("proto[1] mode = %v", preset.Protocols[1].APIMode)
	}
}

// ---------- 模型能力元数据（§5.7） ----------

func TestGetModelCapability(t *testing.T) {
	if c, ok := GetModelCapability("deepseek-v4-pro"); !ok || c.ContextWindow != 1000000 {
		t.Errorf("deepseek-v4-pro capability = %+v, ok=%v", c, ok)
	}
	// 前缀匹配：快照版本回落基础规格
	if c, ok := GetModelCapability("deepseek-v4-flash-0731"); !ok || c.ThinkingPattern != ThinkingPatternThinkingObject {
		t.Errorf("deepseek-v4-flash-0731 capability = %+v, ok=%v", c, ok)
	}
	if _, ok := GetModelCapability("unknown-model-xyz"); ok {
		t.Errorf("unknown model should not be found")
	}
}

// ---------- helpers ----------

func mustNewRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("request err: %v", err)
	}
	return r
}

// ---------- Responses 协议 system→instructions 适配（§7.3） ----------

func TestBuildResponsesSystemToInstructions(t *testing.T) {
	p := NewProvider(ProviderConfig{
		APIMode: APIModeOpenAIResponses,
		Model:   "deepseek-v4-pro",
	}).(*openAIProvider)
	req := ChatRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "系统提示词A"},
			{Role: "system", Content: "系统提示词B"},
			{Role: "user", Content: "你好"},
		},
	}
	body, err := p.buildRequest(req, false)
	if err != nil {
		t.Fatalf("buildRequest err: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal err: %v", err)
	}
	// system 消息合并进 instructions，不进 input
	if got := m["instructions"]; got != "系统提示词A\n系统提示词B" {
		t.Errorf("instructions = %q, want 合并后的系统提示词", got)
	}
	input, ok := m["input"].([]any)
	if !ok {
		t.Fatalf("input = %v (%T), want []any", m["input"], m["input"])
	}
	if len(input) != 1 {
		t.Fatalf("input 长度 = %d, want 1（仅 user）", len(input))
	}
	first := input[0].(map[string]any)
	if first["role"] != "user" {
		t.Errorf("input[0].role = %v, want user（system 不得出现在 input）", first["role"])
	}
}

func TestProviderEffectiveTimeout(t *testing.T) {
	SetDefaultTimeout(0)
	t.Cleanup(func() { SetDefaultTimeout(DefaultTimeoutSec) })

	pFollow := NewProvider(ProviderConfig{}).(*openAIProvider)
	if got := pFollow.effectiveTimeout(); got != 150*time.Second {
		t.Fatalf("default effectiveTimeout = %v, want 150s", got)
	}

	SetDefaultTimeout(240)
	if got := pFollow.effectiveTimeout(); got != 240*time.Second {
		t.Fatalf("updated global effectiveTimeout = %v, want 240s", got)
	}

	pOverride := NewProvider(ProviderConfig{Timeout: 420}).(*openAIProvider)
	if got := pOverride.effectiveTimeout(); got != 420*time.Second {
		t.Fatalf("override effectiveTimeout = %v, want 420s", got)
	}
}

func TestProviderExplicitTimeoutPrecedence(t *testing.T) {
	SetDefaultTimeout(DefaultTimeoutSec)
	t.Cleanup(func() { SetDefaultTimeout(DefaultTimeoutSec) })

	// 1. 显式超时 300s 优先于较小的全球默认 (150s) 与 Provider 自身配置 (120s)
	pSmall := NewProvider(ProviderConfig{Timeout: 120}).(*openAIProvider)
	explicitCtx, cancelExplicit := WithExplicitTimeout(context.Background(), 300*time.Second)
	defer cancelExplicit()

	reqCtx, cancelReq := pSmall.withEffectiveTimeout(explicitCtx)
	defer cancelReq()
	dl, ok := reqCtx.Deadline()
	if !ok {
		t.Fatal("expected deadline on reqCtx")
	}
	if rem := time.Until(dl); rem < 250*time.Second || rem > 305*time.Second {
		t.Fatalf("explicit timeout truncated/expanded: remaining = %v, want ~300s", rem)
	}

	// 2. 未设置显式超时时，Provider 大超时 (420s) 生效
	pLarge := NewProvider(ProviderConfig{Timeout: 420}).(*openAIProvider)
	plainCtx, cancelPlain := WithExplicitTimeout(context.Background(), 0)
	defer cancelPlain()
	reqCtx2, cancelReq2 := pLarge.withEffectiveTimeout(plainCtx)
	defer cancelReq2()
	dl2, ok := reqCtx2.Deadline()
	if !ok {
		t.Fatal("expected deadline on reqCtx2")
	}
	if rem := time.Until(dl2); rem < 380*time.Second || rem > 425*time.Second {
		t.Fatalf("provider large timeout not applied: remaining = %v, want ~420s", rem)
	}
}

type closeSignalingBody struct {
	io.ReadCloser
	closed chan struct{}
	once   sync.Once
}

func (b *closeSignalingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return b.ReadCloser.Close()
}

type closeSignalingTransport struct {
	base   http.RoundTripper
	closed chan struct{}
}

func (t *closeSignalingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &closeSignalingBody{ReadCloser: resp.Body, closed: t.closed}
	return resp, nil
}

func TestStreamRequestExitsOnReqCtxTimeout(t *testing.T) {
	// 服务端快速推送 40 条 SSE chunk 后保持连接打开（无 [DONE]），使 buffer=32 的 ch 阻塞第 33 条发送；
	// 在完全不读取 ch 的前提下，先等待 worker 执行 defer resp.Body.Close() 退出信号，确认无 goroutine 泄漏后再排空 ch。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 40; i++ {
			_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"chunk-%d\"}}]}\n\n", i)
		}
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	bodyClosed := make(chan struct{})
	p := NewProvider(ProviderConfig{
		Endpoint: srv.URL,
		URLMode:  "exact",
		Token:    "sk-test",
		Model:    "gpt-test",
		Timeout:  1,
	}).(*openAIProvider)
	p.cli.Transport = &closeSignalingTransport{base: http.DefaultTransport, closed: bodyClosed}

	ch, err := p.streamRequest(context.Background(), srv.URL, []byte(`{"stream":true}`))
	if err != nil {
		t.Fatalf("streamRequest err: %v", err)
	}

	// 不读取 ch，直接等待 worker 因 reqCtx 超时退出并关闭 resp.Body
	select {
	case <-bodyClosed:
	case <-time.After(3 * time.Second):
		t.Fatal("streamRequest worker leaked: resp.Body was not closed after reqCtx expired")
	}

	// worker 已确认退出后再排空缓冲区并验证 ch 正常关闭
	drainDeadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-drainDeadline:
			t.Fatal("ch was not closed after worker closed resp.Body")
		}
	}
}
