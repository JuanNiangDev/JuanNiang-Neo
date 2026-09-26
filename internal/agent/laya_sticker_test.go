package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/layasticker"
	"JuanNiang-Neo/internal/agent/tool"
	"JuanNiang-Neo/internal/core/models"
)

func TestSelectLayaStickerUsesConfiguredTagsAndSkipsPreviousID(t *testing.T) {
	cfg := layasticker.Config{Categories: []layasticker.Category{
		{ID: "HAPPY", Description: "开心", StickerTags: []string{"开心", "庆祝"}, Enabled: true},
	}}
	calls := make([]string, 0, 2)
	list := func(_ context.Context, tag, _ string, _, _ int) ([]models.Sticker, error) {
		calls = append(calls, tag)
		if tag == "开心" {
			return []models.Sticker{{ID: "previous", Name: "旧表情", Tags: models.JSONSlice{"开心"}}}, nil
		}
		return []models.Sticker{{ID: "new-id", Name: "新表情", Tags: models.JSONSlice{"庆祝"}}}, nil
	}
	sticker, err := selectLayaSticker(context.Background(), cfg, "HAPPY", list, "previous")
	if err != nil {
		t.Fatal(err)
	}
	if sticker == nil || sticker.ID != "new-id" {
		t.Fatalf("selected sticker = %+v", sticker)
	}
	if len(calls) != 2 || calls[0] != "开心" || calls[1] != "庆祝" {
		t.Fatalf("configured tags not used in order: %v", calls)
	}
}

func TestSelectLayaStickerSkipsUnknownDisabledAndNoSendCategories(t *testing.T) {
	list := func(_ context.Context, tag, _ string, _, _ int) ([]models.Sticker, error) {
		t.Fatalf("category %q should not query sticker DAO", tag)
		return nil, nil
	}
	cases := []string{"UNKNOWN", "DISABLED", "NONE"}
	cfg := layasticker.Config{Categories: []layasticker.Category{
		{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
		{ID: "DISABLED", Description: "禁用", StickerTags: []string{"禁用"}, Enabled: false},
	}}
	for _, category := range cases {
		sticker, err := selectLayaSticker(context.Background(), cfg, category, list, "")
		if err != nil {
			t.Fatalf("category %s returned error: %v", category, err)
		}
		if sticker != nil {
			t.Fatalf("category %s should not select sticker: %+v", category, sticker)
		}
	}
}

func TestBuildLayaAutoSendPreservesOriginalDeliverySemantics(t *testing.T) {
	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
	sticker := &models.Sticker{ID: "short-id"}
	auto := buildLayaAutoSend(msg, sticker)
	if auto.MessageType != "group" || auto.TargetID != 123 {
		t.Fatalf("target not copied: %+v", auto)
	}
	if auto.Origin != "laya_auto" || auto.Delivery || !auto.DropOnReview {
		t.Fatalf("automatic flags are wrong: %+v", auto)
	}
	if !tool.MessageHasExpression(auto.Message) {
		t.Fatalf("automatic message must contain a stk expression: %#v", auto.Message)
	}
}

func TestSendLayaAutoAfterTextPreservesOrderAndRequiresTextSuccess(t *testing.T) {
	recorder := &agentSendRecorder{}
	q := tool.NewDeferredSendQueue()
	auto := tool.DeferredSend{MessageType: "group", TargetID: 123, Message: "[CQ:image,file=stk://auto]", Origin: "laya_auto", DropOnReview: true}
	recorder.events = append(recorder.events, "text")
	if !sendLayaAutoAfterText(context.Background(), q, recorder, &auto, true) {
		t.Fatal("successful text should permit automatic send")
	}
	if fmt.Sprint(recorder.events) != "[text auto]" {
		t.Fatalf("send order = %v, want [text auto]", recorder.events)
	}

	noTextRecorder := &agentSendRecorder{}
	if sendLayaAutoAfterText(context.Background(), tool.NewDeferredSendQueue(), noTextRecorder, &auto, false) {
		t.Fatal("automatic item must not send when normal text failed")
	}
	if len(noTextRecorder.events) != 0 {
		t.Fatalf("unexpected sends after failed text: %v", noTextRecorder.events)
	}
}

func TestSendLayaAutoAfterSuccessfulReplyRunsDecisionUnlockedAndSendsLast(t *testing.T) {
	recorder := &agentSendRecorder{}
	queue := tool.NewDeferredSendQueue()
	var sendMu sync.Mutex
	textSent := func() bool {
		recorder.events = append(recorder.events, "text")
		return true
	}()
	decided := false
	auto := sendLayaAutoAfterReply(context.Background(), queue, recorder, &sendMu, textSent, true, func() *tool.DeferredSend {
		if !sendMu.TryLock() {
			t.Fatal("Laya decision must not hold the global send lock")
		}
		sendMu.Unlock()
		recorder.events = append(recorder.events, "decision")
		decided = true
		return &tool.DeferredSend{
			MessageType: "group", TargetID: 123,
			Message: "[CQ:image,file=stk://auto]", Origin: "laya_auto", DropOnReview: true,
		}
	}, nil)
	if auto == nil || !decided {
		t.Fatalf("successful Laya decision should return the sent item, auto=%+v decided=%v", auto, decided)
	}
	if got := fmt.Sprint(recorder.events); got != "[text decision auto]" {
		t.Fatalf("send sequence = %s, want [text decision auto]", got)
	}
}

func TestSendLayaAutoAfterReplyReviewRejectionSkipsDecisionAndSend(t *testing.T) {
	recorder := &agentSendRecorder{}
	queue := tool.NewDeferredSendQueue()
	var sendMu sync.Mutex
	decisionCalls := 0
	auto := sendLayaAutoAfterReply(context.Background(), queue, recorder, &sendMu, true, false, func() *tool.DeferredSend {
		decisionCalls++
		return &tool.DeferredSend{MessageType: "group", TargetID: 123, Message: "[CQ:image,file=stk://auto]", Origin: "laya_auto", DropOnReview: true}
	}, nil)
	if auto != nil || decisionCalls != 0 || queue.Len() != 0 || len(recorder.events) != 0 {
		t.Fatalf("review rejection must skip automatic item: auto=%+v calls=%d queue=%d events=%v", auto, decisionCalls, queue.Len(), recorder.events)
	}
}

func TestSendLayaAutoAfterReplyDecisionFailureKeepsSentText(t *testing.T) {
	recorder := &agentSendRecorder{}
	queue := tool.NewDeferredSendQueue()
	var sendMu sync.Mutex
	recorder.events = append(recorder.events, "text")
	auto := sendLayaAutoAfterReply(context.Background(), queue, recorder, &sendMu, true, true, func() *tool.DeferredSend {
		recorder.events = append(recorder.events, "decision")
		return nil // timeout, invalid category, or no matching sticker
	}, nil)
	if auto != nil {
		t.Fatalf("failed Laya decision must not return an automatic item: %+v", auto)
	}
	if got := fmt.Sprint(recorder.events); got != "[text decision]" {
		t.Fatalf("fallback should preserve the already-sent text, events=%s", got)
	}
}

func TestMaybeAddLayaStickerSkipsExistingPrimaryDeliveryAndSendFace(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"answers":{"sticker":{"choice":"HAPPY"}}}`))
	}))
	defer srv.Close()
	cfg := layasticker.Config{
		Enabled:         true,
		Endpoint:        srv.URL,
		ProtocolMode:    layasticker.ProtocolJSON,
		HTTPMethod:      http.MethodPost,
		Timeout:         time.Second,
		RequestTemplate: `{"choice":{"criteria":{{criteria}}}}`,
		CategoryPointer: "/answers/sticker/choice",
		Categories:      []layasticker.Category{{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true}, {ID: "HAPPY", Description: "开心", Enabled: true}},
	}
	h := NewHagoCenter()
	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
	q := tool.NewDeferredSendQueue()
	q.Add(tool.DeferredSend{MessageType: "group", TargetID: 123, Message: "普通工具交付", Delivery: true})
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "正常回复", q, cfg); got != nil {
		t.Fatal("existing primary delivery should suppress Laya")
	}
	q = tool.NewDeferredSendQueue()
	q.Add(tool.DeferredSend{MessageType: "group", TargetID: 123, Message: "[CQ:face,id=14]", Delivery: false})
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "正常回复", q, cfg); got != nil {
		t.Fatal("send_face should suppress duplicate Laya sticker")
	}
	q = tool.NewDeferredSendQueue()
	q.Add(tool.DeferredSend{MessageType: "group", TargetID: 123, Message: "[CQ:image,file=stk://tool-sent,subType=1]", Delivery: false})
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "正常回复", q, cfg); got != nil {
		t.Fatal("gallery sticker already sent by an expression tool must suppress duplicate Laya sticker")
	}
	if calls != 0 {
		t.Fatalf("Laya should not be called for existing queue semantics, calls=%d", calls)
	}
	// The inbound user's face is not an outbound bot message and must not
	// suppress the optional decision request.
	msg.RawMessage = "[CQ:face,id=14]"
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户表情", "正常回复", tool.NewDeferredSendQueue(), cfg); got != nil {
		t.Fatal("without a sticker DAO the decision cannot select an item")
	}
	if calls != 1 {
		t.Fatalf("inbound user expression must not count as bot output, calls=%d", calls)
	}
}

func TestMaybeAddLayaStickerDisabledDoesNotCallLaya(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"answers":{"sticker":{"choice":"HAPPY"}}}`))
	}))
	defer srv.Close()
	h := NewHagoCenter()
	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
	cfg := layasticker.Config{Endpoint: srv.URL, ProtocolMode: layasticker.ProtocolJSON}
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "正常回复", tool.NewDeferredSendQueue(), cfg); got != nil {
		t.Fatal("disabled Laya must not produce an automatic item")
	}
	if calls != 0 {
		t.Fatalf("disabled Laya must not call the endpoint, calls=%d", calls)
	}
}

func TestLayaConfigSnapshotIsCopiedForOneTurn(t *testing.T) {
	cfg := &models.ReplyStrategyConfig{
		LayaStickerEnabled:         true,
		LayaStickerEndpoint:        "https://laya.example/decision",
		LayaStickerProtocolMode:    "json",
		LayaStickerTimeout:         10,
		LayaStickerRequestTemplate: `{"choice":{"criteria":{{criteria}}}}`,
		LayaStickerCategories:      `[{"id":"NONE","description":"不发送","no_send":true,"enabled":true}]`,
	}
	snapshot, err := layaConfigFromModel(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.LayaStickerCategories = `[{"id":"CHANGED","description":"changed","enabled":true}]`
	if snapshot.Categories[0].ID != "NONE" {
		t.Fatalf("snapshot changed after DB config mutation: %+v", snapshot.Categories)
	}
}

// TestSubmitLayaTaskQueueFullDropsWithoutAdvancingWatermark 队列满时必须丢弃任务
// 且不得推进水位：否则该目标后续到达的新任务会因水位已被"未入队的旧任务"
// 抬高而被误判为过期。
func TestSubmitLayaTaskQueueFullDropsWithoutAdvancingWatermark(t *testing.T) {
	// 仅测试 submit 的满队列行为，不启动会并发排空队列的 worker。
	p := &layaPostProcessor{
		queues: []chan *layaTask{make(chan *layaTask, layaQueuePerWorker)},
		latest: make(map[string]uint64),
	}

	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
	key := layaTargetKey(msg)
	// 填满该目标所在分片，使后续提交落入 default 分支。
	for i := 0; i < layaQueuePerWorker; i++ {
		if !p.submit(&layaTask{msg: msg}) {
			t.Fatalf("filling queue at %d should succeed", i)
		}
	}
	watermark := p.latest[key]

	overflow := &layaTask{msg: msg}
	if p.submit(overflow) {
		t.Fatal("submit must fail when the shard queue is full")
	}
	if got := atomic.LoadInt64(&p.dropped); got == 0 {
		t.Fatal("dropped counter must record the discarded task")
	}
	if p.latest[key] != watermark {
		t.Fatalf("watermark moved for a task that never entered the queue: %d -> %d", watermark, p.latest[key])
	}
}

// TestLayaTaskTTLDropsExpiredTask 入队已过期的任务必须在发起 HTTP 前丢弃，
// 避免旧上下文的表情挂到新对话上。
func TestLayaTaskTTLDropsExpiredTask(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"answers":{"sticker":{"choice":"HAPPY"}}}`))
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := newLayaPostProcessor(ctx, NewHagoCenter(), 1)
	defer p.Stop()

	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
	cfg := layasticker.Config{
		Enabled:         true,
		Endpoint:        srv.URL,
		ProtocolMode:    layasticker.ProtocolJSON,
		HTTPMethod:      http.MethodPost,
		Timeout:         time.Second,
		TaskTTL:         time.Millisecond,
		RequestTemplate: `{"choice":{"criteria":{{criteria}}}}`,
		CategoryPointer: "/answers/sticker/choice",
		Categories: []layasticker.Category{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心", Enabled: true},
		},
	}
	expired := &layaTask{
		msg:              msg,
		assistantContent: "正常回复",
		cfg:              cfg,
		queuedAt:         time.Now().Add(-time.Minute),
		sequence:         1,
	}
	p.handle(expired)
	if calls != 0 {
		t.Fatalf("expired task must not reach Laya, calls=%d", calls)
	}
	if got := atomic.LoadInt64(&p.dropped); got == 0 {
		t.Fatal("expired task must be counted as dropped")
	}
}

type agentSendRecorder struct {
	events []string
}

func (r *agentSendRecorder) SendPrivateMsg(_ int64, message any) (int64, error) {
	return r.SendGroupMsg(0, message)
}
func (r *agentSendRecorder) SendGroupMsg(_ int64, message any) (int64, error) {
	if tool.MessageHasExpression(message) {
		r.events = append(r.events, "auto")
	}
	return 1, nil
}
func (r *agentSendRecorder) DeleteMsg(_ int64) error                          { return nil }
func (r *agentSendRecorder) GetMsg(_ int64) (*adapter.MessageEvent, error)    { return nil, nil }
func (r *agentSendRecorder) GetGroupInfo(_ int64) (*adapter.GroupInfo, error) { return nil, nil }
func (r *agentSendRecorder) GetGroupMemberList(_ int64) ([]adapter.GroupMemberInfo, error) {
	return nil, nil
}
func (r *agentSendRecorder) KickGroupMember(_, _ int64, _ bool) error               { return nil }
func (r *agentSendRecorder) BanGroupMember(_, _ int64, _ int) error                 { return nil }
func (r *agentSendRecorder) SetGroupWholeBan(_ int64, _ bool) error                 { return nil }
func (r *agentSendRecorder) SetGroupCard(_, _ int64, _ string) error                { return nil }
func (r *agentSendRecorder) HandleFriendRequest(_ string, _ bool, _ string) error   { return nil }
func (r *agentSendRecorder) HandleGroupRequest(_, _ string, _ bool, _ string) error { return nil }

// TestLayaConfidenceOKRequiresFieldWhenThresholdSet 设置了最低置信度时，
// 响应缺少置信度字段必须视为不达标（宁可不发）；未设下限则不过滤。
func TestLayaConfidenceOKRequiresFieldWhenThresholdSet(t *testing.T) {
	low, high := 0.3, 0.9
	cases := []struct {
		name       string
		min        float64
		confidence *float64
		want       bool
	}{
		{"未设下限-缺字段", 0, nil, true},
		{"未设下限-低置信度", 0, &low, true},
		{"设下限-缺字段", 0.5, nil, false},
		{"设下限-低于下限", 0.5, &low, false},
		{"设下限-高于下限", 0.5, &high, true},
	}
	for _, tc := range cases {
		if got := layaConfidenceOK(tc.min, tc.confidence); got != tc.want {
			t.Fatalf("%s: layaConfidenceOK(%v, %v) = %v, want %v", tc.name, tc.min, tc.confidence, got, tc.want)
		}
	}
}

// TestLayaTaskTTLRecheckedAfterSlowDecision Laya 响应很慢时，即使期间没有新回合
// 推进水位，决策返回后也必须复查有效期并丢弃过期任务。
func TestLayaTaskTTLRecheckedAfterSlowDecision(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write([]byte(`{"answers":{"sticker":{"choice":"HAPPY"}}}`))
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := newLayaPostProcessor(ctx, NewHagoCenter(), 1)
	defer p.Stop()

	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
	cfg := layasticker.Config{
		Enabled:         true,
		Endpoint:        srv.URL,
		ProtocolMode:    layasticker.ProtocolJSON,
		HTTPMethod:      http.MethodPost,
		Timeout:         time.Second,
		TaskTTL:         20 * time.Millisecond,
		RequestTemplate: `{"choice":{"criteria":{{criteria}}}}`,
		CategoryPointer: "/answers/sticker/choice",
		Categories: []layasticker.Category{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心", Enabled: true},
		},
	}
	// 入队时未过期（HTTP 前检查通过），但 80ms 的响应超过 20ms 有效期。
	task := &layaTask{
		msg:              msg,
		assistantContent: "正常回复",
		cfg:              cfg,
		queuedAt:         time.Now(),
		sequence:         1,
	}
	p.handle(task)
	if calls != 1 {
		t.Fatalf("fresh task must reach Laya once, calls=%d", calls)
	}
	if got := atomic.LoadInt64(&p.dropped); got == 0 {
		t.Fatal("task expired during slow decision must be counted as dropped")
	}
}

func TestSendLayaAutoRechecksReviewBeforeSending(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		messageType                string
		blocked, pending, wantSend bool
	}{
		{"late rejection", "group", true, false, false},
		{"still pending after text timeout", "group", false, true, false},
		{"allowed or unreviewed", "group", false, false, true},
		{"private message unaffected", "private", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &agentSendRecorder{events: []string{"text"}}
			msg := &adapter.MessageEvent{MessageType: tc.messageType, GroupID: 123, UserID: 456, MessageID: 999}
			task := &layaTask{msg: msg, triggerMessageID: -789}
			auto := buildLayaAutoSend(msg, &models.Sticker{ID: "auto"})
			calls := 0
			gate := func(_ context.Context, groupID, userID, messageID int64) (bool, bool) {
				calls++
				if groupID != 123 || userID != 456 || messageID != -789 {
					t.Errorf("wrong trigger identity: %d/%d/%d", groupID, userID, messageID)
				}
				return tc.blocked, tc.pending
			}
			got := sendLayaAutoAfterReview(context.Background(), task, recorder, &auto, gate)
			if got != tc.wantSend {
				t.Errorf("sent=%v, want %v", got, tc.wantSend)
			}
			want := "[text]"
			if tc.wantSend {
				want = "[text auto]"
			}
			if fmt.Sprint(recorder.events) != want {
				t.Errorf("sends=%v, want %s", recorder.events, want)
			}
			if tc.messageType == "group" && calls != 1 {
				t.Errorf("review must be checked once, calls=%d", calls)
			}
			if tc.messageType == "private" && calls != 0 {
				t.Error("private message should not consult group review")
			}
		})
	}
	t.Run("no manager", func(t *testing.T) {
		recorder := &agentSendRecorder{}
		msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
		auto := buildLayaAutoSend(msg, &models.Sticker{ID: "auto"})
		if !sendLayaAutoAfterReview(context.Background(), &layaTask{msg: msg}, recorder, &auto, nil) || fmt.Sprint(recorder.events) != "[auto]" {
			t.Fatal("missing manager must preserve send behavior")
		}
	})
}

// TestMaybeAddLayaStickerSystemOneNativeDecisions 原生协议下：no_send 决策、
// 未知类别与服务端错误都必须回退原版文字回复（返回 nil，不产生发送项）。
func TestMaybeAddLayaStickerSystemOneNativeDecisions(t *testing.T) {
	choice := "NONE"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if choice == "ERROR" {
			http.Error(w, "bad", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"answers":{"sticker":{"choice":"` + choice + `"}}}`))
	}))
	defer srv.Close()
	cfg := layasticker.Config{
		Enabled:      true,
		Endpoint:     srv.URL + "/v1/systemone",
		Model:        "multilingual",
		ProtocolMode: layasticker.ProtocolSystemOne,
		HTTPMethod:   http.MethodPost,
		Timeout:      time.Second,
		Categories: []layasticker.Category{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心", Enabled: true},
		},
	}
	h := NewHagoCenter()
	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}

	// no_send 类别参与决策但不生成发送项。
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "正常回复", nil, cfg); got != nil {
		t.Fatal("NONE decision must not produce a send item")
	}
	// 未知类别回退。
	choice = "UNKNOWN"
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "正常回复", nil, cfg); got != nil {
		t.Fatal("unknown category must fall back to plain text")
	}
	// 服务端错误回退。
	choice = "ERROR"
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "正常回复", nil, cfg); got != nil {
		t.Fatal("Laya error must fall back to plain text")
	}
}

// TestMaybeAddLayaStickerAllowsInlineFaceBeforeHTTP 最终文字仅含普通 QQ 内联
// 小表情（[CQ:face,id=14]）时不得阻止 Laya 决策——小表情是文字语气的一部分，
// 与图库表情不冲突（诊断 reason=inline_face_allowed）。
func TestMaybeAddLayaStickerAllowsInlineFaceBeforeHTTP(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"answers":{"sticker":{"choice":"HAPPY"}}}`))
	}))
	defer srv.Close()
	cfg := layasticker.Config{
		Enabled:         true,
		Endpoint:        srv.URL,
		ProtocolMode:    layasticker.ProtocolJSON,
		HTTPMethod:      http.MethodPost,
		Timeout:         time.Second,
		RequestTemplate: `{"choice":{"criteria":{{criteria}}}}`,
		CategoryPointer: "/answers/sticker/choice",
		Categories: []layasticker.Category{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心", Enabled: true},
		},
	}
	h := NewHagoCenter()
	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
	// 无 Sticker DAO 时决策完成后返回 nil，但 HTTP 决策必须已经发起。
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "好的，没问题 [CQ:face,id=14]", nil, cfg); got != nil {
		t.Fatal("without a sticker DAO the decision cannot select an item")
	}
	if calls != 1 {
		t.Fatalf("inline QQ face must not block the Laya decision, calls=%d", calls)
	}
}

// TestMaybeAddLayaStickerSkipsGalleryStickerBeforeHTTP 最终文字已包含图库
// 表情包（stk://）时在 HTTP 前跳过，避免重复追加
// （诊断 reason=existing_sticker_blocked）。
func TestMaybeAddLayaStickerSkipsGalleryStickerBeforeHTTP(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"answers":{"sticker":{"choice":"HAPPY"}}}`))
	}))
	defer srv.Close()
	cfg := layasticker.Config{
		Enabled:         true,
		Endpoint:        srv.URL,
		ProtocolMode:    layasticker.ProtocolJSON,
		HTTPMethod:      http.MethodPost,
		Timeout:         time.Second,
		RequestTemplate: `{"choice":{"criteria":{{criteria}}}}`,
		CategoryPointer: "/answers/sticker/choice",
		Categories: []layasticker.Category{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心", Enabled: true},
		},
	}
	h := NewHagoCenter()
	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "看这里 [CQ:image,file=stk://abc123,subType=1]", nil, cfg); got != nil {
		t.Fatal("existing gallery sticker in final text must suppress the automatic sticker")
	}
	if calls != 0 {
		t.Fatalf("existing gallery sticker must skip before HTTP, calls=%d", calls)
	}
}

// TestMaybeAddLayaStickerSkipsSilenceAndEmptyBeforeHTTP 空回复与静默回复同样
// 在 HTTP 前跳过（诊断 reason=empty_or_silence）。
func TestMaybeAddLayaStickerSkipsSilenceAndEmptyBeforeHTTP(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"answers":{"sticker":{"choice":"HAPPY"}}}`))
	}))
	defer srv.Close()
	cfg := layasticker.Config{
		Enabled:         true,
		Endpoint:        srv.URL,
		ProtocolMode:    layasticker.ProtocolJSON,
		HTTPMethod:      http.MethodPost,
		Timeout:         time.Second,
		RequestTemplate: `{"choice":{"criteria":{{criteria}}}}`,
		CategoryPointer: "/answers/sticker/choice",
		Categories: []layasticker.Category{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心", Enabled: true},
		},
	}
	h := NewHagoCenter()
	msg := &adapter.MessageEvent{MessageType: "group", GroupID: 123}
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", "", nil, cfg); got != nil {
		t.Fatal("empty assistant content must skip")
	}
	if got := h.maybeAddLayaSticker(context.Background(), msg, "用户", SilenceToken, nil, cfg); got != nil {
		t.Fatal("silence response must skip")
	}
	if calls != 0 {
		t.Fatalf("empty/silence must skip before HTTP, calls=%d", calls)
	}
}
