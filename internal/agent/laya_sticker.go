package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/layasticker"
	"JuanNiang-Neo/internal/agent/tool"
	"JuanNiang-Neo/internal/core/models"
)

// layaRecentWindow 去重窗口：记住最近发过的几张表情 ID，优先换新的。
const layaRecentWindow = 3

type stickerListFunc func(ctx context.Context, tag, keyword string, limit, offset int) ([]models.Sticker, error)

func layaConfigFromModel(cfg *models.ReplyStrategyConfig) (layasticker.Config, error) {
	if cfg == nil {
		return layasticker.Config{}, nil
	}
	categories := []layasticker.Category(nil)
	if strings.TrimSpace(cfg.LayaStickerCategories) != "" {
		if err := json.Unmarshal([]byte(cfg.LayaStickerCategories), &categories); err != nil {
			return layasticker.Config{}, fmt.Errorf("Laya categories JSON 无效: %w", err)
		}
	}
	timeout := cfg.LayaStickerTimeout
	if timeout <= 0 {
		timeout = 10
	}
	// 自动表情有效期：默认 30s，上限与超时一致避免表情迟到太久。
	taskTTL := cfg.LayaStickerTaskTTL
	if taskTTL <= 0 {
		taskTTL = int(layaTaskTTL / time.Second)
	}
	protocolMode := strings.TrimSpace(cfg.LayaStickerProtocolMode)
	if protocolMode == "" {
		protocolMode = layasticker.ProtocolJSON
	}
	httpMethod := strings.TrimSpace(cfg.LayaStickerHTTPMethod)
	if httpMethod == "" {
		httpMethod = "POST"
	}
	categoryPath := strings.TrimSpace(cfg.LayaStickerCategoryPath)
	if categoryPath == "" {
		categoryPath = "/answers/sticker/choice"
	}
	confidencePath := strings.TrimSpace(cfg.LayaStickerConfidencePath)
	if confidencePath == "" {
		confidencePath = "/answers/sticker/answer_confidence"
	}
	result := layasticker.Config{
		Enabled:              cfg.LayaStickerEnabled,
		Endpoint:             cfg.LayaStickerEndpoint,
		CapabilitiesEndpoint: cfg.LayaCapabilitiesEndpoint,
		APIKey:               cfg.LayaStickerAPIKey,
		Model:                cfg.LayaStickerModel,
		Timeout:              time.Duration(timeout) * time.Second,
		TaskTTL:              time.Duration(taskTTL) * time.Second,
		ProtocolMode:         protocolMode,
		HTTPMethod:           httpMethod,
		RequestTemplate:      cfg.LayaStickerRequestTemplate,
		CategoryPointer:      categoryPath,
		ConfidencePointer:    confidencePath,
		Categories:           categories,
		MinConfidence:        cfg.LayaStickerMinConfidence,
	}
	if !result.Enabled {
		return result, nil
	}
	if err := layasticker.ValidateConfig(result); err != nil {
		return layasticker.Config{}, err
	}
	return result, nil
}

func layaTargetKey(msg *adapter.MessageEvent) string {
	if msg == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", msg.MessageType, getTargetID(msg))
}

// recentLayaAutoStickers 返回该目标最近发送过的表情 ID（新的在前）。
// 读取与写入都在 layaAutoMu 下完成；真正的"选择→发送→记录"原子性由后处理器的
// per-target 分片保证，这里只维护去重窗口。
func (h *HagoCenter) recentLayaAutoStickers(msg *adapter.MessageEvent) []string {
	if h == nil {
		return nil
	}
	key := layaTargetKey(msg)
	h.layaAutoMu.Lock()
	defer h.layaAutoMu.Unlock()
	history := h.layaLastAuto[key]
	return append([]string(nil), history...)
}

// rememberLayaAutoSticker 记录该目标刚发送成功的表情 ID，仅保留最近 layaRecentWindow 个。
func (h *HagoCenter) rememberLayaAutoSticker(msg *adapter.MessageEvent, stickerID string) {
	if h == nil || strings.TrimSpace(stickerID) == "" {
		return
	}
	key := layaTargetKey(msg)
	h.layaAutoMu.Lock()
	defer h.layaAutoMu.Unlock()
	if h.layaLastAuto == nil {
		h.layaLastAuto = make(map[string][]string)
	}
	if len(h.layaLastAuto) >= 2048 {
		h.layaLastAuto = make(map[string][]string)
	}
	history := append([]string{stickerID}, h.layaLastAuto[key]...)
	if len(history) > layaRecentWindow {
		history = history[:layaRecentWindow]
	}
	h.layaLastAuto[key] = history
}

// lastLayaAutoSticker 返回该目标最近一次发送的表情 ID（无记录则空串）。
func (h *HagoCenter) lastLayaAutoSticker(msg *adapter.MessageEvent) string {
	if h == nil {
		return ""
	}
	key := layaTargetKey(msg)
	h.layaAutoMu.Lock()
	defer h.layaAutoMu.Unlock()
	if history := h.layaLastAuto[key]; len(history) > 0 {
		return history[0]
	}
	return ""
}

func selectLayaSticker(ctx context.Context, cfg layasticker.Config, categoryID string, list stickerListFunc, previousID string) (*models.Sticker, error) {
	return selectLayaStickerRecent(ctx, cfg, categoryID, list, []string{previousID})
}

func selectLayaStickerRecent(ctx context.Context, cfg layasticker.Config, categoryID string, list stickerListFunc, recentIDs []string) (*models.Sticker, error) {
	var category *layasticker.Category
	for i := range cfg.Categories {
		if cfg.Categories[i].ID == categoryID {
			category = &cfg.Categories[i]
			break
		}
	}
	if category == nil || !category.Enabled || category.NoSend || list == nil {
		return nil, nil
	}
	recent := make(map[string]struct{}, len(recentIDs))
	for _, id := range recentIDs {
		if id != "" {
			recent[id] = struct{}{}
		}
	}
	seen := make(map[string]struct{})
	var pool []models.Sticker
	for _, configuredTag := range category.StickerTags {
		tag := strings.TrimSpace(configuredTag)
		if tag == "" {
			continue
		}
		items, err := list(ctx, tag, "", 50, 0)
		if err != nil {
			return nil, err
		}
		for i := range items {
			item := items[i]
			if item.ID == "" {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			pool = append(pool, item)
		}
	}
	if len(pool) == 0 {
		return nil, nil
	}
	// 优先候选：不在最近发送窗口内。
	var fresh []models.Sticker
	for _, item := range pool {
		if _, ok := recent[item.ID]; !ok {
			fresh = append(fresh, item)
		}
	}
	// 只有一张（或全在窗口内）时允许重复，避免"该类目只有一张就再也不发"。
	if len(fresh) == 0 {
		fresh = pool
	}
	return &fresh[rand.Intn(len(fresh))], nil
}

func buildLayaAutoSend(msg *adapter.MessageEvent, sticker *models.Sticker) tool.DeferredSend {
	targetID := getTargetID(msg)
	return tool.DeferredSend{
		MessageType:  msg.MessageType,
		TargetID:     targetID,
		Message:      fmt.Sprintf("[CQ:image,file=stk://%s,subType=1]", sticker.ID),
		Origin:       "laya_auto",
		Delivery:     false,
		DropOnReview: true,
	}
}

func stickerIDFromMessage(message any) string {
	find := func(raw string) string {
		index := strings.Index(raw, "stk://")
		if index < 0 {
			return ""
		}
		value := raw[index+len("stk://"):]
		if end := strings.IndexAny(value, ",] \t\r\n"); end >= 0 {
			value = value[:end]
		}
		return strings.TrimSpace(value)
	}
	switch value := message.(type) {
	case string:
		return find(value)
	case adapter.Segment:
		if file, ok := value.Data["file"].(string); ok {
			return find(file)
		}
	case []adapter.Segment:
		for _, segment := range value {
			if id := stickerIDFromMessage(segment); id != "" {
				return id
			}
		}
	}
	return ""
}

// maybeAddLayaSticker decides one automatic item but intentionally does not add
// it to the original queue. The caller sends it only after the normal text reply
// succeeds, preserving the official queue FIFO and delivery rules.
func (h *HagoCenter) maybeAddLayaSticker(ctx context.Context, msg *adapter.MessageEvent, userMessage, assistantContent string, q *tool.DeferredSendQueue, cfg layasticker.Config) *tool.DeferredSend {
	// 诊断日志（第 7 版）：worker 执行后、发起 HTTP 前的每个提前返回分支
	// 都记录明确跳过原因，用于区分"任务未提交 / 入队被丢弃 / worker 跳过
	// HTTP"三种情况。只记录原因与目标标识，不记录聊天内容。
	if h == nil || msg == nil {
		return nil
	}
	if !cfg.Enabled {
		log.Info("Laya 自动表情跳过：配置未启用", "stage", "pre_http", "reason", "disabled")
		return nil
	}
	if strings.TrimSpace(assistantContent) == "" || isSilenceResponse(assistantContent) {
		log.Info("Laya 自动表情跳过：回复为空或静默", "stage", "pre_http", "reason", "empty_or_silence",
			"message_type", msg.MessageType, "target", getTargetID(msg))
		return nil
	}
	targetID := getTargetID(msg)
	if targetID <= 0 {
		log.Info("Laya 自动表情跳过：目标无效", "stage", "pre_http", "reason", "invalid_target", "message_type", msg.MessageType)
		return nil
	}
	// 只阻止"已有图库表情包"的重复追加；普通 QQ 内联小表情（face 段）
	// 不阻止 Laya 决策——小表情是文字语气的一部分，与图库表情不冲突。
	if tool.MessageHasGallerySticker(assistantContent) {
		log.Info("Laya 自动表情跳过：回复已含图库表情", "stage", "pre_http", "reason", "existing_sticker_blocked",
			"message_type", msg.MessageType, "target", targetID)
		return nil
	}
	if tool.MessageHasExpression(assistantContent) {
		log.Info("Laya 自动表情：内联 QQ 小表情不阻止决策", "stage", "pre_http", "reason", "inline_face_allowed",
			"message_type", msg.MessageType, "target", targetID)
	}
	if q != nil && (q.DeliveredTo(msg.MessageType, targetID) || q.HasExpressionTo(msg.MessageType, targetID)) {
		log.Info("Laya 自动表情跳过：队列已有交付或表情", "stage", "pre_http", "reason", "queue_semantics",
			"message_type", msg.MessageType, "target", targetID)
		return nil
	}
	client, err := layasticker.NewClient(cfg)
	if err != nil {
		log.Warn("Laya 表情决策配置无效，回退原版回复", "stage", "pre_http", "reason", "invalid_config", "err", err)
		return nil
	}
	request := layasticker.DecisionRequest{
		UserMessage:    userMessage,
		AssistantReply: assistantContent,
		MessageType:    msg.MessageType,
		Model:          cfg.Model,
		Categories:     cfg.Categories,
	}
	for _, category := range cfg.Categories {
		if category.Enabled {
			request.EnabledCategories = append(request.EnabledCategories, category)
		}
	}
	decision, err := client.Decide(ctx, request)
	if err != nil {
		log.Warn("Laya 表情决策失败，回退原版回复", "stage", "http", "reason", "decide_error", "err", err)
		return nil
	}
	// 置信度闸门：MinConfidence>0 时响应必须携带有效置信度——字段缺失或低于
	// 下限都不发送（缺失时无从判断是否达标，宁可不发）。MinConfidence<=0 不过滤。
	// confidence 的具体含义由 Laya 服务定义，这里只做下限拦截，不当作校准概率使用。
	if !layaConfidenceOK(cfg.MinConfidence, decision.Confidence) {
		if decision.Confidence == nil {
			log.Info("Laya 响应缺少置信度字段，跳过自动表情",
				"min", cfg.MinConfidence, "category", decision.Category)
		} else {
			log.Info("Laya 置信度低于阈值，跳过自动表情",
				"confidence", *decision.Confidence, "min", cfg.MinConfidence, "category", decision.Category)
		}
		return nil
	}
	if h.DAO == nil || h.DAO.Sticker == nil {
		log.Info("Laya 自动表情跳过：Sticker DAO 未就绪", "stage", "post_http", "reason", "sticker_dao_unavailable",
			"category", decision.Category, "message_type", msg.MessageType, "target", targetID)
		return nil
	}
	sticker, err := selectLayaStickerRecent(ctx, cfg, decision.Category, h.DAO.Sticker.List, h.recentLayaAutoStickers(msg))
	if err != nil {
		log.Warn("Laya 类别匹配原版表情失败，回退原版回复", "stage", "post_http", "reason", "select_error", "category", decision.Category, "err", err)
		return nil
	}
	if sticker == nil {
		// no_send 决策、未知/禁用类别或类别下无可用表情都走这里。
		log.Info("Laya 自动表情跳过：类别无可发表情", "stage", "post_http", "reason", "no_sticker_for_category",
			"category", decision.Category, "message_type", msg.MessageType, "target", targetID)
		return nil
	}
	auto := buildLayaAutoSend(msg, sticker)
	log.Info("Laya 自动表情已选中", "category", decision.Category, "sticker_id", sticker.ID, "message_type", msg.MessageType, "target", targetID)
	return &auto
}

// submitLayaTask 把自动表情任务交给后台后处理器；后处理器未初始化时直接丢弃
// （装饰性表情不阻塞正常回复链路）。
func (h *HagoCenter) submitLayaTask(task *layaTask) bool {
	if h == nil || h.layaPost == nil {
		return false
	}
	return h.layaPost.submit(task)
}

func (h *HagoCenter) advanceLayaTarget(msg *adapter.MessageEvent) uint64 {
	if h == nil || h.layaPost == nil {
		return 0
	}
	return h.layaPost.advanceTarget(msg)
}

// ---------- 自动表情后处理器 ----------
//
// 决策与发送都必须离开主发送链路：Laya 是外部 HTTP 请求（最长 LayaStickerTimeout，
// 配置上限 120s），若在 orderedReplier 的顺序锁内执行，同批次后续回复会被拖住；
// 若在 handleMessage 内同步执行，还会一直占着 ConcurrencyManager 的并发令牌。
// 因此这里用独立 worker 池承接：
//   - 固定 worker 数 + 固定容量队列：提交非阻塞，队列满直接丢弃装饰性任务，
//     不会因为阻塞在信号量上堆积 goroutine；
//   - 按 target 哈希分片：同一个群/私聊恒定落到同一个 worker，天然 per-target FIFO，
//     同时解决"并发回合选中同一张表情"；
//   - 触发消息 ID + 有效期：过期或已被同目标更新回合超越的表情直接丢弃，
//     避免旧上下文的表情挂到新对话上。

const (
	layaWorkerCount    = 4
	layaQueuePerWorker = 32
	layaTaskTTL        = 30 * time.Second
)

// layaTask 一个待处理的自动表情任务。
type layaTask struct {
	msg              *adapter.MessageEvent
	userMessage      string
	assistantContent string
	cfg              layasticker.Config
	triggerMessageID int64
	queuedAt         time.Time
	sequence         uint64
}

// layaPostProcessor 自动表情后处理器。
type layaPostProcessor struct {
	center  *HagoCenter
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	queues  []chan *layaTask
	seqMu   sync.Mutex
	latest  map[string]uint64 // target key → 已受理的最新任务序号（单调水位）
	seq     uint64
	stopped bool
	dropped int64
}

func newLayaPostProcessor(parent context.Context, h *HagoCenter, workerCount int) *layaPostProcessor {
	if workerCount <= 0 {
		workerCount = layaWorkerCount
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	p := &layaPostProcessor{
		center: h,
		ctx:    ctx,
		cancel: cancel,
		queues: make([]chan *layaTask, workerCount),
		latest: make(map[string]uint64),
	}
	for i := range p.queues {
		p.queues[i] = make(chan *layaTask, layaQueuePerWorker)
	}
	for i := range p.queues {
		p.wg.Add(1)
		go func(ch chan *layaTask) {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case task, ok := <-ch:
					if !ok {
						return
					}
					safeCall(func() { p.handle(task) })
				}
			}
		}(p.queues[i])
	}
	return p
}

// shardFor 按 target 哈希选取分片，保证同一目标恒定同一 worker（FIFO）。
func (p *layaPostProcessor) shardFor(key string) int {
	if p == nil || len(p.queues) == 0 {
		return 0
	}
	var hash uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	return int(hash % uint32(len(p.queues)))
}

// submit 非阻塞提交任务。队列满或已关停时丢弃并计数——自动表情是装饰性内容，
// 宁可少发一条也不阻塞正常回复链路。
func (p *layaPostProcessor) submit(task *layaTask) bool {
	if p == nil || task == nil || task.msg == nil {
		return false
	}
	if len(p.queues) == 0 {
		return false
	}
	key := layaTargetKey(task.msg)
	shard := p.shardFor(key)
	if task.queuedAt.IsZero() {
		task.queuedAt = time.Now()
	}
	// 序号分配与入队在同一把锁内完成。这样多个回合并发提交到同一目标时，
	// channel 的接收顺序与序号顺序一致，不会出现“后提交的任务先入队”。
	p.seqMu.Lock()
	defer p.seqMu.Unlock()
	if p.stopped {
		return false
	}
	newSequence := false
	if task.sequence == 0 {
		p.seq++
		task.sequence = p.seq
		newSequence = true
	}
	select {
	case p.queues[shard] <- task:
		if newSequence {
			p.latest[key] = task.sequence
		}
		return true
	default:
		atomic.AddInt64(&p.dropped, 1)
		log.Warn("Laya 自动表情队列已满，丢弃装饰性任务", "key", key)
		return false
	}
}

// advanceTarget 标记目标进入了新的 Agent 回合。即使这一回合因为原版工具
// 表情、审核、发送失败或其他原因没有提交 Laya 任务，也必须推进水位，
// 让之前尚未完成的 Laya 决策失效。
func (p *layaPostProcessor) advanceTarget(msg *adapter.MessageEvent) uint64 {
	if p == nil || msg == nil {
		return 0
	}
	key := layaTargetKey(msg)
	p.seqMu.Lock()
	defer p.seqMu.Unlock()
	if p.stopped {
		return 0
	}
	p.seq++
	p.latest[key] = p.seq
	return p.seq
}

// layaConfidenceOK 判断决策置信度是否满足下限：未设下限（<=0）恒通过；
// 设了下限则响应必须携带置信度且不低于下限。
func layaConfidenceOK(minConfidence float64, confidence *float64) bool {
	if minConfidence <= 0 {
		return true
	}
	return confidence != nil && *confidence >= minConfidence
}

// layaTaskExpired 判断任务是否已超出有效期（入队时间 + TaskTTL）。
func layaTaskExpired(task *layaTask) bool {
	ttl := task.cfg.TaskTTL
	if ttl <= 0 {
		ttl = layaTaskTTL
	}
	return time.Since(task.queuedAt) > ttl
}

// handle 单个任务的决策 + 发送（同目标串行执行）。
func (p *layaPostProcessor) handle(task *layaTask) {
	if task == nil || task.msg == nil {
		return
	}
	key := layaTargetKey(task.msg)
	// 有效期：入队已久的表情不再符合当前聊天上下文。
	if layaTaskExpired(task) {
		atomic.AddInt64(&p.dropped, 1)
		log.Info("Laya 自动表情已过期，丢弃", "drop_reason", "expired_pre_http", "key", key, "age_ms", time.Since(task.queuedAt).Milliseconds())
		return
	}
	// 新鲜度：同目标已有更新的回合通过水位时，这条旧表情不再发送。
	p.seqMu.Lock()
	stale := task.sequence > 0 && p.latest[key] > task.sequence
	p.seqMu.Unlock()
	if stale {
		atomic.AddInt64(&p.dropped, 1)
		log.Info("Laya 自动表情已被更新回合超越，丢弃", "drop_reason", "stale_pre_http", "key", key)
		return
	}
	p.process(task)
}

// process 执行一次自动表情的"决策 → 选择 → 发送"。同一目标由同一 worker
// 串行调用，因此 select 与 record 之间天然原子（不再依赖两把非原子锁的读改写）。
func (p *layaPostProcessor) process(task *layaTask) {
	h := p.center
	if h == nil {
		return
	}
	cfg := task.cfg
	// 决策请求超时独立于触发器上下文：本任务在回合返回后才执行。
	baseCtx := context.Background()
	if p.ctx != nil {
		baseCtx = p.ctx
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = layasticker.DefaultTimeout
	}
	decideCtx, cancel := context.WithTimeout(baseCtx, timeout)
	defer cancel()
	auto := h.maybeAddLayaSticker(decideCtx, task.msg, task.userMessage, task.assistantContent, nil, cfg)
	// Laya 响应可能很慢：决策返回后必须复查有效期，过期表情不再发送，
	// 即使期间群里没有新回合（水位未推进）也一样。
	if layaTaskExpired(task) {
		atomic.AddInt64(&p.dropped, 1)
		log.Info("Laya 自动表情在决策期间过期，丢弃", "drop_reason", "expired_post_http", "key", layaTargetKey(task.msg), "age_ms", time.Since(task.queuedAt).Milliseconds())
		return
	}
	if auto == nil {
		return
	}
	// 新回合可能在 Laya HTTP 请求期间提交；决策返回后再次检查新鲜度，
	// 防止旧请求完成后把过期表情发到最新对话里。
	key := layaTargetKey(task.msg)
	p.seqMu.Lock()
	stale := task.sequence > 0 && p.latest[key] > task.sequence
	stopped := p.stopped
	p.seqMu.Unlock()
	ctxDone := p.ctx != nil && p.ctx.Err() != nil
	if stale || stopped || ctxDone {
		atomic.AddInt64(&p.dropped, 1)
		log.Info("Laya 自动表情决策后被丢弃", "drop_reason", "stale_or_stopped",
			"key", key, "stale", stale, "stopped", stopped, "ctx_done", ctxDone)
		return
	}
	if h.Adapter == nil {
		return
	}
	// 全局发送锁只在真正发送的瞬间持有。
	sent := false
	func() {
		h.sendMu.Lock()
		defer h.sendMu.Unlock()
		// 新回合的正常文字发送与任务提交都在 sendMu 内完成。再次检查有效期
		// 与水位必须位于真正发送前，避免旧 Laya 请求刚返回时把已过期表情
		// 插入新回合文字之后。
		p.seqMu.Lock()
		stale := task.sequence > 0 && p.latest[key] > task.sequence
		stopped := p.stopped
		p.seqMu.Unlock()
		if stale || stopped || layaTaskExpired(task) {
			atomic.AddInt64(&p.dropped, 1)
			log.Info("Laya 自动表情发送前被丢弃", "drop_reason", "expired_or_stale_at_send",
				"key", key, "stale", stale, "stopped", stopped, "expired", layaTaskExpired(task))
			return
		}
		var reviewGate func(context.Context, int64, int64, int64) (bool, bool)
		if h.GroupMgr != nil {
			reviewGate = h.GroupMgr.ReviewGate
		}
		sent = sendLayaAutoAfterReview(decideCtx, task, h.Adapter, auto, reviewGate)
	}()
	if sent {
		h.rememberLayaAutoSticker(task.msg, stickerIDFromMessage(auto.Message))
	}
}

// sendLayaAutoAfterReview 仅用于 Laya 的最终发送边界（调用方持有 sendMu）。
// 自动表情不沿用文字回复的审核超时放行策略：终态拒绝或仍在途都丢弃。
func sendLayaAutoAfterReview(ctx context.Context, task *layaTask, sender tool.AdapterProvider, auto *tool.DeferredSend, reviewGate func(context.Context, int64, int64, int64) (bool, bool)) bool {
	if task == nil || task.msg == nil {
		return false
	}
	if task.msg.MessageType == "group" && reviewGate != nil {
		messageID := task.triggerMessageID
		if messageID == 0 {
			messageID = task.msg.MessageID
		}
		blocked, pending := reviewGate(ctx, task.msg.GroupID, task.msg.UserID, messageID)
		if blocked || pending {
			log.Info("群审核拒绝或仍在途，丢弃 Laya 自动表情", "message_id", messageID, "group_id", task.msg.GroupID, "blocked", blocked, "pending", pending)
			return false
		}
	}
	return sendLayaAutoAfterText(ctx, tool.NewDeferredSendQueue(), sender, auto, true)
}

// Stop 停止后处理器并等待在途任务结束。
func (p *layaPostProcessor) Stop() {
	if p == nil || p.cancel == nil {
		return
	}
	p.seqMu.Lock()
	if p.stopped {
		p.seqMu.Unlock()
		return
	}
	p.stopped = true
	p.seqMu.Unlock()
	p.cancel()
	p.wg.Wait()
}

// ---------- 旧同步接口（保留给测试与显式调用） ----------

// sendLayaAutoAfterText sends a previously selected automatic item through a
// DeferredSendQueue only after the normal text path has reported full success.
func sendLayaAutoAfterText(ctx context.Context, q *tool.DeferredSendQueue, adapter tool.AdapterProvider, auto *tool.DeferredSend, textSent bool) bool {
	if q == nil || adapter == nil || auto == nil || !textSent {
		return false
	}
	return q.SendNow(ctx, adapter, *auto)
}

// sendLayaAutoAfterReply 保留给已有包内调用方与测试的同步辅助函数。
// 生产发送链路使用 layaPostProcessor，不在 orderedReplier 中调用此函数。
func sendLayaAutoAfterReply(ctx context.Context, q *tool.DeferredSendQueue, adapter tool.AdapterProvider, sendMu *sync.Mutex, textSent, reviewAllowed bool, decide func() *tool.DeferredSend, onSent func(*tool.DeferredSend)) *tool.DeferredSend {
	if !textSent || !reviewAllowed || decide == nil {
		return nil
	}
	auto := decide()
	if auto == nil || q == nil || adapter == nil {
		return nil
	}
	if sendMu != nil {
		sendMu.Lock()
		defer sendMu.Unlock()
	}
	if !sendLayaAutoAfterText(ctx, q, adapter, auto, true) {
		return nil
	}
	if onSent != nil {
		onSent(auto)
	}
	return auto
}
