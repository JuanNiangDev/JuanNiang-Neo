package agent

import "testing"

// TestAdminOnlyToolAllowed 验证工具管理员校验判定：
// adminOnly=true 的工具仅 Admins 列表内用户可调用；非管理员仅当工具目标是
// 调用者本人（自防御/自处理）时放行，其余一律拒绝；
// adminOnly=false 的工具不受限制。
func TestAdminOnlyToolAllowed(t *testing.T) {
	const (
		banSelf  = `{"group_id":10001,"user_id":10001,"duration":60}` // 禁言目标=本人
		banOther = `{"group_id":10001,"user_id":10002,"duration":60}` // 禁言目标=第三方
		banZero  = `{"group_id":10001,"duration":60}`                 // 无 user_id 参数
		wholeBan = `{"group_id":10001,"enable":true}`                 // 全员禁言（无用户目标）
	)
	cases := []struct {
		name      string
		adminOnly bool
		toolName  string
		argsJSON  string
		userID    int64
		admins    []string
		expected  bool
	}{
		// admin_only 开启：非管理员 → 拒绝
		{"仅管理员-禁言第三方-非管理员", true, "ban_group_member", banOther, 10001, []string{"10002"}, false},
		{"仅管理员-禁言-无admin配置", true, "ban_group_member", banOther, 10001, nil, false},
		{"仅管理员-全员禁言-非管理员", true, "set_group_whole_ban", wholeBan, 10001, []string{"10002"}, false},
		{"仅管理员-好友请求-非管理员", true, "handle_friend_request", `{"flag":"f1","approve":true}`, 10001, []string{"10002"}, false},
		{"仅管理员-撤回-非管理员", true, "delete_msg", `{"message_id":123}`, 10001, []string{"10002"}, false},
		{"仅管理员-禁言-参数缺user_id", true, "ban_group_member", banZero, 10001, []string{"10002"}, false},

		// admin_only 开启：非管理员但目标=本人（自防御/自处理）→ 放行
		{"仅管理员-禁言本人-非管理员自防御", true, "ban_group_member", banSelf, 10001, []string{"10002"}, true},
		{"仅管理员-踢出本人-非管理员自处理", true, "kick_group_member", `{"group_id":10001,"user_id":10001}`, 10001, nil, true},
		{"仅管理员-改本人名片-非管理员", true, "set_group_card", `{"group_id":10001,"user_id":10001,"card":"x"}`, 10001, []string{"10002"}, true},

		// admin_only 开启：管理员 → 放行
		{"仅管理员-禁言-管理员", true, "ban_group_member", banOther, 10002, []string{"10002"}, true},
		{"仅管理员-撤回-管理员", true, "delete_msg", `{"message_id":123}`, 10003, []string{"10002", "10003"}, true},

		// admin_only 关闭：不受限制
		{"非仅管理员-发群消息", false, "send_group_msg", "{}", 99999, nil, true},
		{"非仅管理员-查群信息", false, "get_group_info", "{}", 99999, nil, true},
		{"非仅管理员-未知工具", false, "unknown_tool", "{}", 99999, nil, true},
	}

	for _, c := range cases {
		got := isAdminToolCallAllowed(c.adminOnly, c.toolName, c.argsJSON, c.userID, c.admins)
		if got != c.expected {
			t.Errorf("%s: isAdminToolCallAllowed(%v, %q, %q, %d, %v) = %v, want %v",
				c.name, c.adminOnly, c.toolName, c.argsJSON, c.userID, c.admins, got, c.expected)
		}
	}
}

// TestAdminToolSelfTarget 验证自目标解析：仅用户定向的高危工具返回 user_id，
// 无用户目标的工具（全员禁言/处理请求/撤回）返回 ok=false。
func TestAdminToolSelfTarget(t *testing.T) {
	cases := []struct {
		name     string
		toolName string
		argsJSON string
		wantID   int64
		wantOK   bool
	}{
		{"禁言-正常参数", "ban_group_member", `{"group_id":1,"user_id":10086,"duration":60}`, 10086, true},
		{"踢人-正常参数", "kick_group_member", `{"group_id":1,"user_id":10010}`, 10010, true},
		{"改群名片-正常参数", "set_group_card", `{"group_id":1,"user_id":10020,"card":"x"}`, 10020, true},
		{"禁言-缺user_id", "ban_group_member", `{"group_id":1}`, 0, false},
		{"禁言-非法JSON", "ban_group_member", `not-json`, 0, false},
		{"全员禁言-无用户目标", "set_group_whole_ban", `{"group_id":1,"enable":true}`, 0, false},
		{"好友请求-无用户目标", "handle_friend_request", `{"flag":"f1","approve":true}`, 0, false},
		{"撤回-无用户目标", "delete_msg", `{"message_id":1}`, 0, false},
		{"未知工具", "unknown_tool", `{}`, 0, false},
	}
	for _, c := range cases {
		gotID, gotOK := adminToolSelfTarget(c.toolName, c.argsJSON)
		if gotID != c.wantID || gotOK != c.wantOK {
			t.Errorf("%s: adminToolSelfTarget(%q, %q) = (%d, %v), want (%d, %v)",
				c.name, c.toolName, c.argsJSON, gotID, gotOK, c.wantID, c.wantOK)
		}
	}
}

// TestAdminOnlyToolNamesCoverOneBot11AdminAPIs 验证高危名单覆盖了所有
// OneBot11 群管理/请求处理/撤回类内置工具，防止未来新增工具漏加防护。
func TestAdminOnlyToolNamesCoverOneBot11AdminAPIs(t *testing.T) {
	// OneBot11 敏感 API 对应的内置工具名（与 tool/builtin.go 注册保持一致）
	sensitiveTools := []string{
		"kick_group_member",     // 踢出群成员
		"ban_group_member",      // 禁言
		"set_group_whole_ban",   // 全员禁言
		"set_group_card",        // 群名片
		"handle_friend_request", // 好友申请
		"handle_group_request",  // 加群/邀请
		"delete_msg",            // 撤回
	}
	for _, name := range sensitiveTools {
		if !adminOnlyToolNames[name] {
			t.Errorf("高危工具 %q 未加入 adminOnlyToolNames，存在提示词注入风险", name)
		}
	}
}
