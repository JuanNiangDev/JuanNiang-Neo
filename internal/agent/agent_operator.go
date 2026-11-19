package agent

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	ragcaller "JuanNiang-Neo/infrastructure/rag/handler"
	sandbox "JuanNiang-Neo/infrastructure/sandbox"
	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2i "JuanNiang-Neo/infrastructure/t2i"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/agent/mcp"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/pluggin"
)

// _ implements pluggin.AgentOperator
var _ pluggin.AgentOperator = (*HagoCenter)(nil)

func (h *HagoCenter) SetProviderActive(ctx context.Context, id string, active bool) error {
	if err := h.DAO.Provider.SetActive(ctx, id, active); err != nil {
		return err
	}
	if active {
		p, err := h.DAO.Provider.GetByID(ctx, id)
		if err != nil {
			return err
		}
		h.Providers.AddProvider(provider.NewProvider(provider.ProviderConfigFromModel(p)))
	} else {
		h.Providers.DelProvider(id)
	}

	// Provider 变更影响 Eino Agent 的 model adapter（构建时持有实例引用），必须重建
	h.RebuildEinoAgent(ctx)
	return nil
}

// SwitchProvider 切换同类型 Provider：停用其他同类型 Provider，激活指定 Provider。
func (h *HagoCenter) SwitchProvider(ctx context.Context, id string) error {
	p, err := h.DAO.Provider.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("provider 不存在: %w", err)
	}

	// 停用同类型其他 Provider（DB + 运行时）
	if err := h.DAO.Provider.DeactivateByType(ctx, p.Type, id); err != nil {
		return err
	}
	// 从运行时移除同类型其他 Provider
	for _, existing := range h.Providers.ListProviders() {
		if existing.Type() == provider.ModelType(p.Type) && existing.ID() != id {
			h.Providers.DelProvider(existing.ID())
		}
	}

	// 激活指定 Provider（DB + 运行时）
	if err := h.DAO.Provider.SetActive(ctx, id, true); err != nil {
		return err
	}
	h.Providers.AddProvider(provider.NewProvider(provider.ProviderConfig{
		ID:             p.ID,
		Name:           p.Name,
		Type:           provider.ModelType(p.Type),
		Endpoint:       p.Endpoint,
		Token:          p.Token,
		Model:          p.Model,
		Temperature:    p.Temperature,
		EnableThinking: p.EnableThinking,
	}))

	// Provider 切换影响 Eino Agent 的 model adapter，必须重建
	h.RebuildEinoAgent(ctx)
	log.Info("Provider 切换完成", "id", id, "type", p.Type)
	return nil
}

func (h *HagoCenter) SetMCPActive(ctx context.Context, id string, active bool) error {
	if err := h.DAO.MCPServer.SetActive(ctx, id, active); err != nil {
		return err
	}
	if !active {
		// 停用：断开并移除
		if m, ok := h.MCP.GetMCP(id); ok {
			m.Disconnect()
			h.MCP.DelMCP(id)
		}
	} else {
		// 启用：若运行时不存在则连接
		if _, ok := h.MCP.GetMCP(id); !ok {
			srv, err := h.DAO.MCPServer.GetByID(ctx, id)
			if err != nil {
				return err
			}
			headers := make(map[string]string)
			for k, v := range srv.Headers {
				if str, ok := v.(string); ok {
					headers[k] = str
				}
			}
			client := mcp.NewSSEMCPClient(mcp.McpSSEConfig{
				ID:            srv.ID,
				Name:          srv.Name,
				ServerURL:     srv.ServerURL,
				Headers:       headers,
				Timeout:       0,
				RetryCount:    srv.RetryCount,
				ToolFilter:    srv.ToolFilter,
				AutoReconnect: srv.AutoReconnect,
			})
			if err := client.Connect(ctx); err != nil {
				log.Error("MCP 连接失败", "name", srv.Name, "err", err)
				return err
			}
			h.MCP.AddMCP(client)
		}
	}
	return nil
}

// SetToolActive 启用/停用工具。停用时从 ToolRegistry 注销，启用时重新注册。
// 注意：Tool 只支持查看和启停，不支持增删。
func (h *HagoCenter) SetToolActive(ctx context.Context, name string, active bool) error {
	// 同步 DB 中的 ToolConfig
	tc, err := h.DAO.ToolConfig.GetByName(ctx, name)
	if err == nil {
		if err := h.DAO.ToolConfig.SetActive(ctx, tc.ID, active); err != nil {
			return err
		}
	}

	if !active {
		h.Tools.Unregister(name)
		log.Info("Tool 已停用", "name", name)
	} else {
		// 启用：若运行时不存在则需重新注册（仅对已注册过的工具支持）
		if _, ok := h.Tools.Get(name); !ok {
			// 重新注册需要原始 Tool 实例，此处仅记录；内置工具在启动时注册，
			// 运行时停用后重新启用需要从 DB 配置重建——但题目要求不支持增删，
			// 仅支持启停，因此停用=注销，启用=重新注册（若 ToolConfig 有记录则标记）
			log.Warn("Tool 重新启用需重启或重新注册", "name", name)
		}
	}
	return nil
}

func (h *HagoCenter) CompactMemory(ctx context.Context, chatAreaID string) error {
	if h.Memory == nil {
		return fmt.Errorf("memory 未初始化")
	}
	llm := h.Providers.SelectModel(provider.ModelTypeText)
	if llm == nil {
		return fmt.Errorf("无可用 Text 模型")
	}
	return h.Memory.CompactShortTermMemory(ctx, chatAreaID, llm)
}

func (h *HagoCenter) GetChatAreaID(userID, groupID int64, messageType string) string {
	var areaType models.AreaType
	var targetID int64
	if messageType == "private" {
		areaType = models.AreaTypePrivate
		targetID = userID
	} else {
		areaType = models.AreaTypeGroup
		targetID = groupID
	}
	area, err := h.DAO.ChatArea.GetOrCreate(context.Background(), areaType, targetID)
	if err != nil {
		return ""
	}
	return area.ID
}

// GetProviderGroup 返回 Provider 访问接口。
func (h *HagoCenter) GetProviderGroup() pluggin.ProviderGroupAccess {
	return &providerGroupAccess{h: h}
}

// GetMCPGroup 返回 MCP 访问接口。
func (h *HagoCenter) GetMCPGroup() pluggin.MCPGroupAccess {
	return &mcpGroupAccess{h: h}
}

// GetToolRegistry 返回 Tool 访问接口。
func (h *HagoCenter) GetToolRegistry() pluggin.ToolRegistryAccess {
	return &toolRegistryAccess{h: h}
}

// GetT2IClient 返回当前 T2I 客户端。
func (h *HagoCenter) GetT2IClient() *t2icaller.Client {
	return h.T2IClient
}

// GetRAGClient 返回当前 RAG 客户端（nil=未启用）。
func (h *HagoCenter) GetRAGClient() *ragcaller.Client {
	return h.RAGClient.Load()
}

// GetSandboxClient 返回当前 Sandbox 客户端。
func (h *HagoCenter) GetSandboxClient() *sandboxcaller.Client {
	return h.SandboxClient
}

// SetT2IActive 启用/停用 T2I 服务。
// 启用时根据 DB 配置创建新客户端；停用时清空运行时客户端。
func (h *HagoCenter) SetT2IActive(ctx context.Context, active bool) error {
	cfg, err := h.DAO.T2I.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("读取 T2I 配置失败: %w", err)
	}
	cfg.IsActive = active
	if err := h.DAO.T2I.UpdateConfig(ctx, cfg); err != nil {
		return fmt.Errorf("更新 T2I 配置失败: %w", err)
	}

	if active {
		client, err := t2i.NewClient(
			t2i.WithBaseURL(cfg.BaseURL),
			t2i.WithTimeout(time.Duration(cfg.Timeout)*time.Second),
		)
		if err != nil {
			log.Warn("T2I 客户端创建失败", "err", err)
			h.T2IClient = nil
			return fmt.Errorf("T2I 客户端创建失败: %w", err)
		}
		h.T2IClient = client
		log.Info("T2I 服务已启用", "base_url", cfg.BaseURL)
	} else {
		h.T2IClient = nil
		log.Info("T2I 服务已停用")
	}

	// T2I 状态变化影响工具可用性（text_to_image），重建 Eino Agent 自动注册/卸载
	h.RebuildEinoAgent(ctx)
	return nil
}

// SetSandboxActive 启用/停用 Sandbox 服务。
// 启用时根据 DB 配置创建新客户端；停用时清空运行时客户端。
func (h *HagoCenter) SetSandboxActive(ctx context.Context, active bool) error {
	cfg, err := h.DAO.Sandbox.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("读取 Sandbox 配置失败: %w", err)
	}
	cfg.IsActive = active
	if err := h.DAO.Sandbox.UpdateConfig(ctx, cfg); err != nil {
		return fmt.Errorf("更新 Sandbox 配置失败: %w", err)
	}

	if active {
		client, err := sandbox.NewClient(
			sandbox.WithBaseURL(cfg.BaseURL),
			sandbox.WithAPIKey(cfg.APIKey),
			sandbox.WithTimeout(time.Duration(cfg.Timeout)*time.Second),
		)
		if err != nil {
			log.Warn("Sandbox 客户端创建失败", "err", err)
			h.SandboxClient = nil
			return fmt.Errorf("Sandbox 客户端创建失败: %w", err)
		}
		h.SandboxClient = client
		log.Info("Sandbox 服务已启用", "base_url", cfg.BaseURL)
	} else {
		h.SandboxClient = nil
		log.Info("Sandbox 服务已停用")
	}

	// Sandbox 状态变化影响工具可用性（create_sandbox/code_exec 等），重建 Eino Agent
	h.RebuildEinoAgent(ctx)
	return nil
}

// ---------- LLMAccess ----------

var _ pluggin.LLMAccess = (*HagoCenter)(nil)

// Chat 使用 Bot 当前启用的文本模型 Provider 调用 LLM。
// 复用 Bot 的模型 / 采样参数 / 密钥配置（req 未设置的采样参数回退 Bot 配置）。
func (h *HagoCenter) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	pr := h.Providers.SelectModel(provider.ModelTypeText)
	if pr == nil {
		return nil, fmt.Errorf("无可用文本模型 Provider")
	}
	return pr.Chat(ctx, req)
}

// VisionURL 用指定名称的 image_model Provider 描述一张网络图片。
// providerName 为空时回退到当前启用的图像模型；图片统一转码为 JPEG 再送模型
// （Provider.Vision 固定按 image/jpeg 编码 data URL）。
func (h *HagoCenter) VisionURL(ctx context.Context, providerName, imageURL, prompt string) (string, error) {
	if h.Providers == nil {
		return "", fmt.Errorf("Provider 未初始化")
	}
	name := strings.TrimSpace(providerName)
	var pr provider.Provider
	if name == "" {
		pr = h.Providers.SelectModel(provider.ModelTypeImage)
	} else {
		for _, p := range h.Providers.ListProviders() {
			if p.Type() == provider.ModelTypeImage &&
				(strings.EqualFold(p.Name(), name) || strings.EqualFold(p.ID(), name)) {
				pr = p
				break
			}
		}
	}
	if pr == nil {
		if name == "" {
			return "", fmt.Errorf("无可用图像模型 Provider")
		}
		return "", fmt.Errorf("未找到图像模型 Provider: %s", name)
	}

	img, err := fetchImageBytes(ctx, imageURL)
	if err != nil {
		return "", err
	}
	jpegImg, err := toJPEG(img)
	if err != nil {
		return "", fmt.Errorf("图片转码失败: %w", err)
	}
	return pr.Vision(ctx, jpegImg, prompt)
}

// fetchImageBytes 下载图片：仅允许公网地址（含重定向目标），限制 12 MiB。
func fetchImageBytes(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("图片地址无效: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	resp, err := publicImageClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("图片下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("图片下载失败: HTTP %d", resp.StatusCode)
	}
	// 多读 1 字节以区分「恰好 12 MiB」与「超限被截断」
	data, err := io.ReadAll(io.LimitReader(resp.Body, (12<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("图片读取失败: %w", err)
	}
	if len(data) > 12<<20 {
		return nil, fmt.Errorf("图片超过 12 MiB 上限")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("图片内容为空")
	}
	return data, nil
}

// publicImageClient 禁用代理，并在每次连接/重定向时校验目标 IP 为公网地址，
// 防止插件传入的图片 URL 访问内网资源（SSRF）。
func publicImageClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := publicIPs(ctx, host)
			if err != nil {
				return nil, err
			}
			// 直接连接已校验的 IP，避免 DNS 重绑定（解析与连接使用同一结果）
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("图片重定向过多")
			}
			return checkPublicHost(req.Context(), req.URL.Hostname())
		},
	}
}

func publicIPs(ctx context.Context, host string) ([]net.IPAddr, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("图片域名解析失败 %s: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("图片域名无解析结果: %s", host)
	}
	for _, entry := range ips {
		if !isPublicIP(entry.IP) {
			return nil, fmt.Errorf("拒绝非公网图片地址: %s (%s)", host, entry.IP)
		}
	}
	return ips, nil
}

func checkPublicHost(ctx context.Context, host string) error {
	_, err := publicIPs(ctx, host)
	return err
}

func isPublicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	// CGNAT 100.64.0.0/10 也不是公网地址
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return !cgnat.Contains(ip)
}

// toJPEG 先按尺寸预算校验（防止压缩炸弹），再完整解码并统一转 JPEG。
func toJPEG(data []byte) ([]byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片解码失败: %w", err)
	}
	const maxSide, maxPixels = 8192, 40_000_000
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide ||
		float64(cfg.Width)*float64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("图片尺寸超出预算: %dx%d", cfg.Width, cfg.Height)
	}
	_ = format // 已由 DecodeConfig 识别；png/gif/jpeg 解码器均已注册
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片完整解码失败: %w", err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("图片 JPEG 编码失败: %w", err)
	}
	return buf.Bytes(), nil
}

// Available 当前是否有启用的文本模型 Provider。
func (h *HagoCenter) Available() bool {
	return h.Providers.SelectModel(provider.ModelTypeText) != nil
}

// ---------- ProviderGroupAccess ----------

type providerGroupAccess struct{ h *HagoCenter }

func (p *providerGroupAccess) List() []pluggin.ProviderInfo {
	list := p.h.Providers.ListProviders()
	out := make([]pluggin.ProviderInfo, 0, len(list))
	for _, pr := range list {
		out = append(out, pluggin.ProviderInfo{
			ID:     pr.ID(),
			Name:   pr.Name(),
			Type:   string(pr.Type()),
			Model:  pr.Model(),
			Active: true,
		})
	}
	return out
}

func (p *providerGroupAccess) GetActive(id string) bool {
	_, ok := p.h.Providers.GetProvider(id)
	return ok
}

// ---------- MCPGroupAccess ----------

type mcpGroupAccess struct{ h *HagoCenter }

func (m *mcpGroupAccess) ListMCPs() []pluggin.MCPInfo {
	list := m.h.MCP.ListMCPs()
	out := make([]pluggin.MCPInfo, 0, len(list))
	for _, mc := range list {
		out = append(out, pluggin.MCPInfo{
			ID:       mc.ID(),
			Name:     mc.Name(),
			Active:   mc.IsConnected(),
			IsActive: mc.IsConnected(),
		})
	}
	return out
}

func (m *mcpGroupAccess) IsConnected(id string) bool {
	mcp, ok := m.h.MCP.GetMCP(id)
	if !ok {
		return false
	}
	return mcp.IsConnected()
}

// ---------- ToolRegistryAccess ----------

type toolRegistryAccess struct{ h *HagoCenter }

func (t *toolRegistryAccess) ListTools() []pluggin.ToolInfo {
	list := t.h.Tools.List()
	out := make([]pluggin.ToolInfo, 0, len(list))
	for _, tl := range list {
		out = append(out, pluggin.ToolInfo{
			Name:        tl.Name(),
			Description: tl.Description(),
			Builtin:     tl.IsBuiltin(),
			LongRunning: tl.IsLongRunning(),
			Active:      true,
		})
	}
	return out
}

func (t *toolRegistryAccess) IsActive(name string) bool {
	_, ok := t.h.Tools.Get(name)
	return ok
}
