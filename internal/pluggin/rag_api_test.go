package pluggin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	caller "JuanNiang-Neo/infrastructure/rag/handler"
	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"

	"github.com/google/uuid"
	lua "github.com/yuin/gopher-lua"
)

// stubAgentOp 实现 AgentOperator（jn.rag 注入只依赖 GetRAGClient，其余零值）。
type stubAgentOp struct{ rag *caller.Client }

func (s *stubAgentOp) SetProviderActive(context.Context, string, bool) error { return nil }
func (s *stubAgentOp) SetMCPActive(context.Context, string, bool) error      { return nil }
func (s *stubAgentOp) SetToolActive(context.Context, string, bool) error     { return nil }
func (s *stubAgentOp) SwitchProvider(context.Context, string) error          { return nil }
func (s *stubAgentOp) SetT2IActive(context.Context, bool) error              { return nil }
func (s *stubAgentOp) SetSandboxActive(context.Context, bool) error          { return nil }
func (s *stubAgentOp) CompactMemory(context.Context, string) error           { return nil }
func (s *stubAgentOp) GetChatAreaID(userID, groupID int64, messageType string) string {
	return messageType
}
func (s *stubAgentOp) GetProviderGroup() ProviderGroupAccess { return nil }
func (s *stubAgentOp) GetMCPGroup() MCPGroupAccess           { return nil }
func (s *stubAgentOp) GetToolRegistry() ToolRegistryAccess   { return nil }
func (s *stubAgentOp) GetT2IClient() *t2icaller.Client       { return nil }
func (s *stubAgentOp) GetSandboxClient() *sandboxcaller.Client {
	return nil
}
func (s *stubAgentOp) GetRAGClient() *caller.Client { return s.rag }

// loadRagTestPlugin 加载带 rag 权限的迷你插件（jn.rag 测试专用）。
func loadRagTestPlugin(t *testing.T, src string) (*PluginEngine, *lua.LState, *stubAgentOp) {
	t.Helper()
	ragSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/tags/search"):
			// 返回两条命中
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"tag": uuid.NewString(), "score": 0.9},
					{"tag": uuid.NewString(), "score": 0.5},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/tags/"):
			// PUT upsert / DELETE
			_ = json.NewEncoder(w).Encode(map[string]any{"tag": uuid.NewString(), "chunk_count": 1, "truncated": false})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ragSrv.Close)

	ragCli := &caller.Client{
		Config:     caller.Config{BaseURL: ragSrv.URL, Timeout: 5 * time.Second},
		HttpClient: &http.Client{Timeout: 5 * time.Second},
	}
	op := &stubAgentOp{rag: ragCli}
	pe := NewPluginEngine(t.TempDir(), &fakeAdapter{}, nil, nil, nil, nil, nil, op, nil)

	pluginDir := filepath.Join(pe.basePath, "ragtest")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sdkDir := filepath.Join(pe.basePath, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdkDir, "jn.lua"), []byte(jnSDKSource), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "name: ragtest\nentry: main.lua\npermissions:\n  - onebot11\n  - rag\nenabled: true\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "pluggin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "main.lua"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	L := lua.NewState()
	pe.injectSDK(L, "ragtest")
	pe.injectBaseAPI(L, "ragtest", []string{"onebot11", "rag"})
	pe.injectCommandAPI(L, "ragtest")
	if err := L.DoString(`package.path = ` + strconvQuote(pluginDir) + ` .. ";" .. package.path`); err != nil {
		t.Fatal(err)
	}
	if err := L.DoFile(filepath.Join(pluginDir, "main.lua")); err != nil {
		t.Fatalf("加载 rag 插件失败: %v", err)
	}
	pe.mu.Lock()
	p := &LoadedPlugin{Manifest: Manifest{Name: "ragtest"}, State: L, Dir: pluginDir}
	attachPluginRef(L, p)
	pe.plugins["ragtest"] = p
	pe.mu.Unlock()
	t.Cleanup(func() {
		pe.mu.Lock()
		p, ok := pe.plugins["ragtest"]
		delete(pe.plugins, "ragtest")
		pe.mu.Unlock()
		if ok && p != nil {
			p.close()
		}
	})
	return pe, L, op
}

func strconvQuote(s string) string { return `"` + s + `/?.lua"` }

// TestPluginRagAPI 插件 jn.rag 写入/查询（feat-rag-service 新增 API）：
// 同步 add / search 走 mock RAG-Service，结果正确透传。
func TestPluginRagAPI(t *testing.T) {
	pe, L, _ := loadRagTestPlugin(t, `
	local jn = require("jn")
add_result = nil
search_hits = nil
search_err = nil
function on_message(event)
    local ok = jn.rag.add("3af2b489-b13a-42e4-af98-fe89d0e6b00e", "测试文本")
    add_result = ok
    local hits, err = jn.rag.search("查询", 5)
    search_hits = hits
    search_err = err
    return false, nil
end
`)

	// on_message 内 add/search 是同步 API（阻塞返回）
	runOnMessage(t, pe, L, 0, 0, "trigger", "90003")

	// add 契约：成功返回 (true)（SDK: boolean, string?，第二值可选为 nil）
	addRes := luaGetGlobal(pe, L, "add_result")
	if addRes.Type() != lua.LTBool || !bool(addRes.(lua.LBool)) {
		t.Fatalf("rag.add 应成功返回 true，got %v", addRes)
	}
	searchHits := luaGetGlobal(pe, L, "search_hits")
	if searchHits.Type() != lua.LTTable {
		t.Fatalf("rag.search 应返回表，got %v", searchHits)
	}
	tbl := searchHits.(*lua.LTable)
	if tbl.Len() != 2 {
		t.Fatalf("rag.search 应返回 2 条命中，got %d", tbl.Len())
	}
	searchErr := luaGetGlobal(pe, L, "search_err")
	if searchErr.Type() != lua.LTNil {
		t.Fatalf("rag.search 不应有错误，got %v", searchErr)
	}
}
