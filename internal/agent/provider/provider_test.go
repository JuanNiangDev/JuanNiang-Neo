package provider

import (
	"encoding/json"
	"net/http"
	"testing"
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
