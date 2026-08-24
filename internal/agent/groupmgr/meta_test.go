package groupmgr

import (
	"context"
	"strings"
	"testing"
)

// TestPunishRecordsMeta 处罚记录现场信息：用户名/判定来源/LLM reason。
func TestPunishRecordsMeta(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()
	ev := groupEv(100, 200, "广告")
	ev.Message.Sender.Card = "张三"
	ev.Message.Sender.Nickname = "nick张三"

	// RAG 路径处罚
	m.punish(ev, "RAG语义核实(校园卡)", "ad", "rag")
	list, err := gmdao.ViolationList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("应 1 条违规记录，got %d", len(list))
	}
	v := list[0]
	if v.Username != "张三" {
		t.Errorf("用户名应为群名片 张三，got %q", v.Username)
	}
	if v.DetectionPath != "rag" {
		t.Errorf("判定来源应为 rag，got %q", v.DetectionPath)
	}

	// LLM 路径覆盖（reason 为 LLM 输出）
	m.punish(ev, "明确广告引流：低价流量卡", "ad", "llm")
	list, _ = gmdao.ViolationList(ctx)
	v = list[0]
	if v.Count != 2 {
		t.Errorf("第 2 次违规 count = %d", v.Count)
	}
	if v.DetectionPath != "llm" || !strings.Contains(v.LLMReason, "广告引流") {
		t.Errorf("LLM 路径应记录 reason，path=%q reason=%q", v.DetectionPath, v.LLMReason)
	}
}

// TestSyncAdminsFromAdapter 从 Adapter.Admins 合并管理员（去重）。
func TestSyncAdminsFromAdapter(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()
	_ = gmdao.AdminAdd(ctx, 100) // 已有管理员

	added, err := m.SyncAdminsFromAdapter(ctx, []string{"100", "200", "abc", "300"})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 { // 200、300 新增；100 已存在；abc 非法跳过
		t.Fatalf("应新增 2 个管理员，got %d", added)
	}
	list, _ := gmdao.AdminList(ctx)
	if len(list) != 3 {
		t.Fatalf("管理员总数应 3，got %d", len(list))
	}
	if !m.IsCommandAdmin(0, 200, nil) {
		t.Error("同步后 200 应视为管理员")
	}
}
