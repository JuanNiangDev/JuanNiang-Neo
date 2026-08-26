package groupmgr

import (
	"context"
	"embed"
	"encoding/base64"
	"math/rand"
	"strconv"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/metrics"
	"JuanNiang-Neo/internal/otelx"

	"go.opentelemetry.io/otel/attribute"
)

// 静态图（go:embed，刷屏警告/复读警告配图，原 redrock_group_manager img/ 目录）。
//
//go:embed shuaping.png shuaping_2.png
var warnImages embed.FS

var (
	imgShuapingB64  = loadImageB64("shuaping.png")
	imgShuaping2B64 = loadImageB64("shuaping_2.png")
)

// loadImageB64 读取 go:embed 图片并编码为 base64:// 前缀数据（供群聊回复发送）。
func loadImageB64(name string) string {
	data, err := warnImages.ReadFile(name)
	if err != nil {
		return ""
	}
	return "base64://" + base64.StdEncoding.EncodeToString(data)
}

// 三级惩罚第 2 次违规禁言时长默认值（面板可配，cfg.ViolationMuteSeconds）。
const defaultViolationMuteSeconds = 1800

// 三级惩罚话术（每级多套随机，卷娘语气；广告固定开头「打广告先交广告费」，敏感固定「小鬼不能碰」）。
var tierTemplates = map[string][3][]string{
	"ad": {
		{
			"打广告先交广告费！卷娘记住本本上了，本次违规予以警告。再犯的话可就要禁言 30 分钟啦～",
			"打广告先交广告费！卷娘记住本本上了，本次违规予以警告。下次再发广告就是禁言 30 分钟起步哦",
			"打广告先交广告费！卷娘记住本本上了，本次违规予以警告。广告费什么时候交呀～",
		},
		{
			"打广告先交广告费！卷娘记住本本上了，本次违规予以禁言 30 分钟。再犯的话就只能请你出去啦～",
			"打广告先交广告费！卷娘记住本本上了，本次违规予以禁言 30 分钟。事不过三，第三次就走人了哦",
			"打广告先交广告费！卷娘记住本本上了，本次违规予以禁言 30 分钟。歇会儿冷静一下吧～",
		},
		{
			"打广告先交广告费！卷娘记住本本上了，本次违规予以踢出群聊。广告费没交，江湖再见啦～",
			"打广告先交广告费！卷娘记住本本上了，本次违规予以踢出群聊。想回来记得先把广告费交了哦～",
			"打广告先交广告费！卷娘记住本本上了，本次违规予以踢出群聊。三次广告，本本都写满了～",
		},
	},
	"sensitive": {
		{
			"小鬼不能碰这个话题哦。卷娘记住本本上了，本次违规予以警告。再犯的话可就要禁言 30 分钟啦～",
			"小鬼不能碰这个话题哦。卷娘记住本本上了，本次违规予以警告。下次再聊就是禁言 30 分钟起步",
			"小鬼不能碰这个话题哦。卷娘记住本本上了，本次违规予以警告。这个话题就当没看见吧～",
		},
		{
			"小鬼不能碰这个话题哦。卷娘记住本本上了，本次违规予以禁言 30 分钟。再犯的话就只能请你出去啦～",
			"小鬼不能碰这个话题哦。卷娘记住本本上了，本次违规予以禁言 30 分钟。事不过三哦",
			"小鬼不能碰这个话题哦。卷娘记住本本上了，本次违规予以禁言 30 分钟。冷静一下哦～",
		},
		{
			"小鬼不能碰这个话题哦。卷娘记住本本上了，本次违规予以踢出群聊。江湖再见啦～",
			"小鬼不能碰这个话题哦。卷娘记住本本上了，本次违规予以踢出群聊。换个群好好说话哦～",
			"小鬼不能碰这个话题哦。卷娘记住本本上了，本次违规予以踢出群聊。三次了还聊，本本都写满了～",
		},
	},
}

// punish 三级惩罚：第 1 次撤回+警告；第 2 次撤回+禁言 30min；第 3 次撤回+踢出（成功重置次数）。
// 每次处罚 @ 回复违规者 + 私聊通知所有管理员。踢人失败保留次数并通知人工处理；
// 撤回失败不阻断处罚（回复与通知注明「原消息撤回失败」）；踢人失败直接返回
// （仅记 kick_failed，不落公共出口重复记 kick）。
// ctx 透传请求作用域（OneBot11 适配器调用可被取消，避免阻塞消息处理）；
// path 为判定来源（rag / keyword / llm），与 reason（LLM 确认违规时为 LLM 输出的 reason）
// 一并写入违规记录，供面板展示分析类型与 LLM 原因。
func (m *Manager) punish(ctx context.Context, ev adapter.Event, reason, category, path string) {
	groupID := ev.Message.GroupID
	userID := ev.Message.UserID

	// 提前声明供 span defer 闭包引用（词法作用域限制）
	var (
		count  int
		action string
		err    error
	)

	// 链路追踪：处罚 span（记录分类/等级/动作，失败链路一眼可见）
	_, span := otelx.Span(ctx, "groupmgr.punish",
		attribute.String("category", category),
		attribute.String("path", path),
	)
	defer func() {
		span.SetAttributes(attribute.String("action", action), attribute.Int("count", count))
		span.End()
	}()

	meta := dao.ViolationMeta{
		Username:      violationUsername(ev),
		DetectionPath: path,
		LLMReason:     reason,
	}

	// 原子自增违规计数并返回新值：事件循环（关键词直罚）与 Run 循环（LLM 追罚）
	// 双 goroutine 并发时不会 read-modify-write 丢计数（单条 SQL 保证）。
	count, err = m.dao.ViolationIncr(ctx, groupID, userID, meta)
	if err != nil {
		log.Warn("违规计数自增失败", "err", err)
		return
	}
	// 违规禁言时长取面板配置（默认 30 分钟）
	muteSeconds := defaultViolationMuteSeconds
	if cfg := m.getCfg(ctx); cfg != nil && cfg.ViolationMuteSeconds > 0 {
		muteSeconds = cfg.ViolationMuteSeconds
	}

	action = ""
	// 撤回违规消息：失败不阻断后续处罚（警告/禁言/踢人独立生效），记日志并在
	// 回复/管理员通知中注明（QQ 撤回有时限，LLM 异步追罚路径易超时，不可静默）
	recallFailed := false
	if derr := m.adp.DeleteMsg(ev.Message.MessageID); derr != nil {
		log.Warn("撤回违规消息失败", "user", userID, "group", groupID, "err", derr)
		recallFailed = true
	}
	switch {
	case count == 1:
		action = "warn"
	case count == 2:
		action = "mute"
		_ = m.adp.BanGroupMember(groupID, userID, muteSeconds)
		_, _ = m.dao.StatIncr(ctx, gkey(groupID, "stats:mute"))
	default:
		action = "kick"
		if err := m.adp.KickGroupMember(groupID, userID, false); err != nil {
			// 踢人失败：保留违规次数（下次仍按第 3 级），通知管理员人工处理；
			// 直接返回，不再落入公共出口重复计数（kick_failed 已单独上报）
			log.Warn("踢人失败", "user", userID, "group", groupID, "err", err)
			metrics.GroupMgrViolationsTotal.WithLabelValues(category, "kick_failed").Inc()
			note := "踢人失败: " + err.Error()
			if recallFailed {
				note += "（原消息撤回失败）"
			}
			m.notifyAdmins(ev, itoa(userID)+" "+reason+"（第 "+strconv.Itoa(count)+" 次）-> "+note+"，请管理员人工处理")
			return
		}
		_ = m.dao.ViolationSet(ctx, groupID, userID, 0, dao.ViolationMeta{})
		_, _ = m.dao.StatIncr(ctx, gkey(groupID, "stats:kick"))
	}
	metrics.GroupMgrViolationsTotal.WithLabelValues(category, action).Inc()
	_, _ = m.dao.StatIncr(ctx, gkey(groupID, "stats:"+category))

	// 回复话术：非法分类兜底归一为 ad（防意外值索引 nil 切片 panic）
	if _, ok := tierTemplates[category]; !ok {
		category = "ad"
	}
	bucket := tierTemplates[category][min(count-1, 2)]
	reply := bucket[rand.Intn(len(bucket))]
	if recallFailed {
		reply += "（原消息撤回失败）"
	}
	m.replyGroup(ev, reply)

	actionText := map[string]string{"warn": "撤回并警告", "mute": "撤回并禁言30分钟", "kick": "撤回并踢出群聊"}[action]
	notifyText := itoa(userID) + " " + reason + "（第 " + strconv.Itoa(count) + " 次）-> " + actionText
	if recallFailed {
		notifyText += "（原消息撤回失败）"
	}
	m.notifyAdmins(ev, notifyText)
	log.Info("群管理处罚", "user", userID, "group", groupID, "reason", reason, "count", count, "action", actionText)
}

// violationUsername 处罚现场用户名：群名片优先，其次昵称。
func violationUsername(ev adapter.Event) string {
	if ev.Message == nil {
		return ""
	}
	if ev.Message.Sender.Card != "" {
		return ev.Message.Sender.Card
	}
	return ev.Message.Sender.Nickname
}

// replyGroup 群聊 @ 发信人回复。
func (m *Manager) replyGroup(ev adapter.Event, text string) {
	_, _ = m.adp.SendGroupMsg(ev.Message.GroupID, []adapter.Segment{
		{Type: "at", Data: map[string]any{"qq": itoa(ev.Message.UserID)}},
		{Type: "text", Data: map[string]any{"text": " " + text}},
	})
}

// replyGroupImage 群聊 @ 发信人回复 + 图片。
func (m *Manager) replyGroupImage(ev adapter.Event, text, b64 string) {
	segments := []adapter.Segment{
		{Type: "at", Data: map[string]any{"qq": itoa(ev.Message.UserID)}},
		{Type: "text", Data: map[string]any{"text": " " + text}},
	}
	if b64 != "" {
		segments = append(segments, adapter.Segment{Type: "image", Data: map[string]any{"file": b64}})
	}
	_, _ = m.adp.SendGroupMsg(ev.Message.GroupID, segments)
}

// ---------- 管理员通知队列（异步 pump，随机延迟 5~30s 防风控） ----------

const (
	notifyDelayMin = 5
	notifyDelayMax = 30
	groupNameTTL   = 3600
	groupNameFail  = 60
)

// notifyAdmins 私聊通知所有管理员（异步入队）。
func (m *Manager) notifyAdmins(ev adapter.Event, text string) {
	if len(ev.Admins) == 0 {
		return
	}
	groupName := m.groupName(ev.Message.GroupID)
	msg := "【群管理通知】\n群: " + groupName + "\n" + text

	m.notifyMu.Lock()
	for _, a := range ev.Admins {
		if qq, err := strconv.ParseInt(a, 10, 64); err == nil {
			m.notifyQueue = append(m.notifyQueue, notifyItem{qq: qq, msg: msg})
		}
	}
	if !m.notifyRunning {
		m.notifyRunning = true
		go m.notifyPump()
	}
	m.notifyMu.Unlock()
}

// notifyPump 队列头部一条发送后，随机延迟再发下一条（goroutine 内）。
func (m *Manager) notifyPump() {
	for {
		m.notifyMu.Lock()
		if len(m.notifyQueue) == 0 {
			m.notifyRunning = false
			m.notifyMu.Unlock()
			return
		}
		item := m.notifyQueue[0]
		m.notifyQueue = m.notifyQueue[1:]
		more := len(m.notifyQueue) > 0
		m.notifyMu.Unlock()

		_, _ = m.adp.SendPrivateMsg(item.qq, item.msg)

		if !more {
			continue
		}
		time.Sleep(time.Duration(notifyDelayMin+rand.Intn(notifyDelayMax-notifyDelayMin+1)) * time.Second)
	}
}

// groupName 群名（缓存：成功 1h / 失败 60s 重试窗口防同步查询阻塞）。
func (m *Manager) groupName(groupID int64) string {
	now := time.Now()
	m.nameMu.Lock()
	if c, ok := m.nameCache[groupID]; ok && now.Sub(c.ts) < c.ttl {
		name := c.name
		m.nameMu.Unlock()
		return name
	}
	m.nameMu.Unlock()

	name := itoa(groupID)
	ttl := time.Duration(groupNameFail) * time.Second
	if info, err := m.adp.GetGroupInfo(groupID); err == nil && info != nil {
		name = info.GroupName
		ttl = time.Duration(groupNameTTL) * time.Second
	} else {
		log.Warn("查询群名失败，使用群号兜底", "group", groupID, "err", err)
	}
	m.nameMu.Lock()
	m.nameCache[groupID] = nameEntry{name: name, ts: time.Now(), ttl: ttl}
	m.nameMu.Unlock()
	return name
}

// gkey 群级统计/状态 kv key 拼装（{group_id}:{suffix}）。
func gkey(groupID int64, suffix string) string {
	return itoa(groupID) + ":" + suffix
}
