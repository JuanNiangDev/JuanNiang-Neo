// Package groupmgr 群管理：系统级群违规检测与处罚（替代 redrock_group_manager Lua 插件）。
//
// 检测闸门位于事件循环 Phase 0（幂等去重）之后、Phase 1（Lua 插件派发）之前，
// 优先级高于所有插件。判定链路（违禁言论）：
//
//	卡片文本化 → RAG 语义核实（第一核实人）→ 高置信直罚 / 模棱两可送 LLM /
//	低置信有词送 LLM / 低置信无词放行；RAG 不可用降级关键词路径（=旧插件行为）；
//	LLM 判定违规 → 三级惩罚 + 样本入库（学习闭环）。
//
// 图片刷屏 / +1 复读 / 入群统计 / 白名单豁免 / 管理员豁免等全部能力与旧插件对齐。
package groupmgr

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/agent/stats"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	caller "JuanNiang-Neo/infrastructure/rag/handler"
)

// 配置内存缓存 TTL：Web 面板保存后调用 Reload 立即失效，TTL 仅兜底。
const cfgCacheTTL = 30 * time.Second

// Manager 群管理系统功能。
type Manager struct {
	dao       *dao.GroupMgrDAO
	adp       *adapter.Adapter
	getRAG    func() *caller.Client // RAG 客户端（热更新，nil = 未配置 → 降级）
	providers *provider.ProviderGroup

	// stats 群消息/回复统计写入器（Loki+Promtail 通道；nil = 未启用）。
	// 处罚/刷屏/复读回复的埋点出口，由 main 注入（SetStats）。
	stats *stats.Writer

	// 配置/词库/白名单/管理员 内存缓存（Reload 立即失效，TTL 兜底）
	mu        sync.RWMutex
	cfg       *models.GroupMgrConfig
	cfgAt     time.Time
	words     map[string]map[string]bool // category(black/gray/sensitive) → word set
	wordsAt   time.Time
	whitelist map[int64]bool
	admins    map[int64]bool
	listAt    time.Time

	// RAG 语录候选集（本地 语录ID → 派生 tag，黑白双集合；变更即重建）
	sampleMu    sync.Mutex
	sampleSet   *phraseSet
	sampleSetAt time.Time

	// 异步学习闭环（黑/白语录写入）串行化：并发双插会绕过幂等去重（Text 无唯一索引）
	learnMu sync.Mutex
	// 异步 LLM 审查去重/在途 + 批窗口（3s 凑批统一提交，逐条独立判定）
	llmMu       sync.Mutex
	llmPending  map[string]bool // "群:QQ" 在途审查
	llmReviewed map[int64]int64 // message_id → 审查时间（10min 去重）
	// reviewVerdict 审核终态（message_id → black/white/none）：
	// Agent 回复发送前闸门（ReviewGate）查询用；TTL 与 llmReviewed 对齐（10min），
	// 重启丢失 → 查不到按放行处理（撤回兜底仍在）。
	reviewVerdict map[int64]string
	llmResults    chan reviewOutcome
	llmBatchMu    sync.Mutex
	llmBatchItems []reviewItem // 批队列（到点/满批统一提交）

	// 图片刷屏状态（内存态 + kv 持久化兜底）
	imgMu    sync.Mutex
	imgState map[string][]int64 // "群:QQ" → 时间戳列表
	imgWarn  map[string]bool    // "群:QQ" → 已警告

	// +1 复读状态（内存态）
	cpMu    sync.Mutex
	cpState map[int64]*copyState // group_id → 复读窗口状态

	// 管理员通知队列（异步 pump，随机延迟防风控）
	notifyMu      sync.Mutex
	notifyQueue   []notifyItem
	notifyRunning bool

	// 群名缓存（get_group_info 最坏 10s 超时，成功 1h / 失败 60s 重试窗口）
	nameMu    sync.Mutex
	nameCache map[int64]nameEntry
}

type copyState struct {
	lastMsg string
	count   int
	users   map[int64]bool
	trig    bool
}

type notifyItem struct {
	qq  int64
	msg string
}

type nameEntry struct {
	name string
	ts   time.Time
	ttl  time.Duration
}

// New 创建群管理 Manager（不启动 goroutine，Run 负责消费回调）。
func New(d *dao.GroupMgrDAO, adp *adapter.Adapter, getRAG func() *caller.Client, pg *provider.ProviderGroup) *Manager {
	m := &Manager{
		dao:           d,
		adp:           adp,
		getRAG:        getRAG,
		providers:     pg,
		words:         map[string]map[string]bool{},
		whitelist:     map[int64]bool{},
		admins:        map[int64]bool{},
		llmPending:    map[string]bool{},
		llmReviewed:   map[int64]int64{},
		reviewVerdict: map[int64]string{},
		llmResults:    make(chan reviewOutcome, llmQueueSize),
		imgState:      map[string][]int64{},
		imgWarn:       map[string]bool{},
		cpState:       map[int64]*copyState{},
		nameCache:     map[int64]nameEntry{},
	}
	return m
}

// SetStats 注入群消息/回复统计写入器（Loki+Promtail 通道；nil = 不埋点）。
// 处罚/刷屏/复读回复发送时写入 direction=reply 事件，来源标记为 groupmgr。
func (m *Manager) SetStats(w *stats.Writer) { m.stats = w }

// Init 初始化：建默认配置 + 载入内存缓存（词库仅从 go:embed txt 加载到内存，不入 DB）。
func (m *Manager) Init(ctx context.Context) error {
	if err := m.dao.InitConfig(ctx); err != nil {
		return err
	}
	// 关键词词库仅从 go:embed txt 加载到内存（兜底用，不入 DB/RAG/samples）。
	// 旧 GroupMgrWord 表残留数据视为废弃，不再读取。
	// 图片刷屏窗口恢复（重启不丢窗口；顺带清理过期 ims: kv 行）
	cfg, _ := m.dao.GetConfig(ctx)
	if cfg != nil {
		m.restoreImgState(ctx, cfg.ImgSpamWindow)
		// 提示词迁移：三套提示词合并为一份 LLMPrompt（空则以 LLMGrayPrompt 为默认）
		if cfg.LLMPrompt == "" && cfg.LLMGrayPrompt != "" {
			cfg.LLMPrompt = cfg.LLMGrayPrompt
			_ = m.dao.UpdateConfig(ctx, cfg)
		}
	}
	return m.Reload(ctx)
}

// Reload 重载配置/词库/白名单/管理员（Web 面板保存后调用；TTL 兜底）。
// 词库仅从 go:embed txt 加载到内存（不入 DB，不可 Web 修改）；配置/白名单/管理员仍走 DB。
func (m *Manager) Reload(ctx context.Context) error {
	cfg, err := m.dao.GetConfig(ctx)
	if err != nil {
		return err
	}
	// 关键词词库：从 go:embed txt 加载到内存（兜底专用，不入 DB/RAG/samples）
	words := loadSeedWordsMap()
	wl, err := m.dao.WlList(ctx)
	if err != nil {
		return err
	}
	al, err := m.dao.AdminList(ctx)
	if err != nil {
		return err
	}
	whitelist := map[int64]bool{}
	for _, w := range wl {
		whitelist[w.QQ] = true
	}
	admins := map[int64]bool{}
	for _, a := range al {
		admins[a.QQ] = true
	}

	m.mu.Lock()
	m.cfg = cfg
	m.cfgAt = time.Now()
	m.words = words
	m.wordsAt = time.Now()
	m.whitelist = whitelist
	m.admins = admins
	m.listAt = time.Now()
	m.mu.Unlock()

	// 样本候选集失效重建
	m.invalidateSampleSet()
	log.Info("群管理配置已重载", "enabled", cfg.Enabled, "words", len(words["black"])+len(words["gray"])+len(words["sensitive"]))
	return nil
}

// InvalidateSampleSet 样本候选集失效（样本增删改后调用，供 Web API 触发）。
func (m *Manager) InvalidateSampleSet() { m.invalidateSampleSet() }

// invalidateSampleSet 置空语录候选集缓存（下次 buildPhraseSet 重建）。
func (m *Manager) invalidateSampleSet() {
	m.sampleMu.Lock()
	m.sampleSet = nil
	m.sampleMu.Unlock()
}

// ---------- 内存缓存读取 ----------

// getCfg 读取配置缓存（TTL 内直接返回；过期重载，失败回退缓存/默认）。
func (m *Manager) getCfg(ctx context.Context) *models.GroupMgrConfig {
	m.mu.RLock()
	cfg, at := m.cfg, m.cfgAt
	m.mu.RUnlock()
	if cfg != nil && time.Since(at) < cfgCacheTTL {
		return cfg
	}
	if err := m.Reload(ctx); err != nil {
		log.Warn("群管理配置读取失败，使用缓存/默认", "err", err)
		m.mu.RLock()
		defer m.mu.RUnlock()
		if m.cfg != nil {
			return m.cfg
		}
		return &models.GroupMgrConfig{HighScore: 0.75, LowScore: 0.5, FallbackScore: 0.6}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// wordHit 命中关键词（优先黑/敏感/灰顺序返回命中词与类别）。
func (m *Manager) wordHit(ctx context.Context, text string) (string, string) {
	m.mu.RLock()
	words, at := m.words, m.wordsAt
	m.mu.RUnlock()
	if time.Since(at) > cfgCacheTTL {
		_ = m.Reload(ctx)
		m.mu.RLock()
		words = m.words
		m.mu.RUnlock()
	}
	lower := strings.ToLower(text)
	for _, cat := range []string{"sensitive", "black", "gray"} {
		for w := range words[cat] {
			if strings.Contains(lower, w) {
				return w, cat
			}
		}
	}
	return "", ""
}

// isWhitelisted 是否白名单（不参与任何检测）。
func (m *Manager) isWhitelisted(ctx context.Context, userID int64) bool {
	m.mu.RLock()
	wl, at := m.whitelist, m.listAt
	m.mu.RUnlock()
	if time.Since(at) > cfgCacheTTL {
		_ = m.Reload(ctx)
		m.mu.RLock()
		wl = m.whitelist
		m.mu.RUnlock()
	}
	return wl[userID]
}

// isManualAdmin 是否手动管理员（高频路径用，不查群角色）。
func (m *Manager) isManualAdmin(ctx context.Context, userID int64) bool {
	m.mu.RLock()
	al, at := m.admins, m.listAt
	m.mu.RUnlock()
	if time.Since(at) > cfgCacheTTL {
		_ = m.Reload(ctx)
		m.mu.RLock()
		al = m.admins
		m.mu.RUnlock()
	}
	return al[userID]
}

// isSystemAdmin 是否系统管理员（Adapter.Admins 列表）。
func (m *Manager) isSystemAdmin(userID int64, admins []string) bool {
	uid := itoa(userID)
	for _, a := range admins {
		if a == uid {
			return true
		}
	}
	return false
}

// isGroupAdmin 是否管理员/群主（处罚豁免用）：系统/手动管理员直接放行；
// 群角色可识别时 owner/admin 放行；识别失败退回手动管理员判断。
// 成员信息走 Adapter 层带缓存查询（正缓存 10min + 负缓存 60s），
// 避免每条非白名单群消息都同步调 OneBot11 get_group_member_info。
func (m *Manager) isGroupAdmin(userID int64, admins []string, groupID int64) bool {
	if m.isSystemAdmin(userID, admins) {
		return true
	}
	m.mu.RLock()
	manual := m.admins[userID]
	m.mu.RUnlock()
	if manual {
		return true
	}
	info, err := m.adp.GetGroupMemberInfoCached(groupID, userID)
	if err != nil || info == nil {
		return false
	}
	return info.Role == "owner" || info.Role == "admin"
}

// excludedGroup 是否排除检测的群。
func (m *Manager) excludedGroup(ctx context.Context, groupID int64) bool {
	cfg := m.getCfg(ctx)
	for _, g := range cfg.ExcludeGroups {
		if g == itoa(groupID) {
			return true
		}
	}
	return false
}

// ---------- 事件入口（Phase 0.5） ----------

// Process 处理 OneBot11 事件，返回 consumed（true = 消息被本模块消费，不进 Agent）。
// message 事件：排除群/白名单完全豁免；管理员豁免违禁言论，但**图片刷屏 / +1 复读仍检测**
// （与旧插件一致，管理员刷屏同样警告/禁言）；其余成员跑完整检测。
// notice 事件：入群统计。
func (m *Manager) Process(ctx context.Context, ev adapter.Event) bool {
	cfg := m.getCfg(ctx)
	if !cfg.Enabled {
		return false
	}
	switch {
	case ev.PostType == "message" && ev.Message != nil:
		msg := ev.Message
		if msg.MessageType != "group" {
			return false
		}
		if m.excludedGroup(ctx, msg.GroupID) {
			return false
		}
		if m.isWhitelisted(ctx, msg.UserID) {
			return false // 白名单完全豁免（含刷屏/复读）
		}
		if m.isGroupAdmin(msg.UserID, ev.Admins, msg.GroupID) {
			// 管理员豁免违禁言论检测，但刷屏/复读不豁免
			if m.checkImageSpam(ctx, ev, cfg) {
				return true
			}
			if m.checkCopySpam(ctx, ev, cfg) {
				return true
			}
			return false
		}
		return m.detectMessage(ctx, ev, cfg)
	case ev.PostType == "notice" && ev.Notice != nil && ev.Notice.NoticeType == "group_increase":
		m.recordJoin(ctx, ev.Notice.GroupID)
	}
	return false
}

// itoa int64 → 十进制字符串。
func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
