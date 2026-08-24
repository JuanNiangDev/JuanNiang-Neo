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
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	caller "JuanNiang-Neo/infrastructure/rag/handler"

	"github.com/google/uuid"
)

// 配置内存缓存 TTL：Web 面板保存后调用 Reload 立即失效，TTL 仅兜底。
const cfgCacheTTL = 30 * time.Second

// Manager 群管理系统功能。
type Manager struct {
	dao       *dao.GroupMgrDAO
	adp       *adapter.Adapter
	getRAG    func() *caller.Client // RAG 客户端（热更新，nil = 未配置 → 降级）
	providers *provider.ProviderGroup

	// 配置/词库/白名单/管理员 内存缓存（Reload 立即失效，TTL 兜底）
	mu        sync.RWMutex
	cfg       *models.GroupMgrConfig
	cfgAt     time.Time
	words     map[string]map[string]bool // category(black/gray/sensitive) → word set
	wordsAt   time.Time
	whitelist map[int64]bool
	admins    map[int64]bool
	listAt    time.Time

	// RAG 样本候选集（本地 id → tag 映射，供检索命中过滤；变更即重建）
	sampleMu    sync.Mutex
	sampleSet   map[uuid.UUID]sampleInfo // tag → 样本信息
	sampleSetAt time.Time

	// 异步 LLM 审查去重/在途
	llmMu       sync.Mutex
	llmPending  map[string]bool // "群:QQ" 在途审查
	llmReviewed map[int64]int64 // message_id → 审查时间（10min 去重）
	llmResults  chan reviewOutcome

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
		dao:         d,
		adp:         adp,
		getRAG:      getRAG,
		providers:   pg,
		words:       map[string]map[string]bool{},
		whitelist:   map[int64]bool{},
		admins:      map[int64]bool{},
		llmPending:  map[string]bool{},
		llmReviewed: map[int64]int64{},
		llmResults:  make(chan reviewOutcome, llmQueueSize),
		imgState:    map[string][]int64{},
		imgWarn:     map[string]bool{},
		cpState:     map[int64]*copyState{},
		nameCache:   map[int64]nameEntry{},
	}
	return m
}

// Init 初始化：建默认配置 + 空词库时种子导入 + 载入内存缓存。
func (m *Manager) Init(ctx context.Context) error {
	if err := m.dao.InitConfig(ctx); err != nil {
		return err
	}
	// 种子导入：词条表为空时从 go:embed 词库导入（source=system）
	if n, err := m.dao.WordCount(ctx); err == nil && n == 0 {
		seeds := loadSeedWords()
		imported := 0
		for category, ws := range seeds {
			for _, w := range ws {
				if _, err := m.dao.WordUpsert(ctx, w, category, "system"); err != nil {
					log.Warn("种子词条导入失败", "word", w, "err", err)
					continue
				}
				imported++
			}
		}
		log.Info("群管理词库种子导入完成", "imported", imported)
	}
	// 图片刷屏窗口恢复（重启不丢窗口；顺带清理过期 ims: kv 行）
	cfg, _ := m.dao.GetConfig(ctx)
	if cfg != nil {
		m.restoreImgState(ctx, cfg.ImgSpamWindow)
	}
	return m.Reload(ctx)
}

// Reload 重载配置/词库/白名单/管理员（Web 面板保存后调用；TTL 兜底）。
func (m *Manager) Reload(ctx context.Context) error {
	cfg, err := m.dao.GetConfig(ctx)
	if err != nil {
		return err
	}
	wordList, err := m.dao.WordListAll(ctx)
	if err != nil {
		return err
	}
	words := map[string]map[string]bool{}
	for _, w := range wordList {
		if words[w.Category] == nil {
			words[w.Category] = map[string]bool{}
		}
		words[w.Category][w.Word] = true
	}
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
	log.Info("群管理配置已重载", "enabled", cfg.Enabled, "words", len(wordList))
	return nil
}

// InvalidateSampleSet 样本候选集失效（样本增删改后调用，供 Web API 触发）。
func (m *Manager) InvalidateSampleSet() { m.invalidateSampleSet() }

func (m *Manager) invalidateSampleSet() {
	m.sampleMu.Lock()
	m.sampleSet = nil
	m.sampleMu.Unlock()
}

// ---------- 内存缓存读取 ----------

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

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
