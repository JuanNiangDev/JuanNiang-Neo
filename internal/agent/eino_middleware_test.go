package agent

import "testing"

// TestAdminOnlyToolAllowed 验证工具管理员校验判定：
// adminOnly=true 的工具仅 Admins 列表内用户可调用，其余一律拒绝；
// adminOnly=false 的工具不受限制。
func TestAdminOnlyToolAllowed(t *testing.T) {
	cases := []struct {
		name      string
		adminOnly bool
		userID    int64
		admins    []string
		expected  bool
	}{
		// admin_only 开启：非管理员 / 无管理员配置 → 拒绝
		{"仅管理员-踢人-非管理员", true, 10001, []string{"10002"}, false},
		{"仅管理员-禁言-非管理员", true, 10001, []string{"10002"}, false},
		{"仅管理员-全员禁言-无admin配置", true, 10001, nil, false},
		{"仅管理员-群名片-非管理员", true, 10001, []string{}, false},
		{"仅管理员-好友请求-非管理员", true, 10001, []string{"10002"}, false},
		{"仅管理员-群请求-非管理员", true, 10001, []string{"10002"}, false},
		{"仅管理员-撤回-非管理员", true, 10001, []string{"10002"}, false},

		// admin_only 开启：管理员 → 放行
		{"仅管理员-踢人-管理员", true, 10002, []string{"10002"}, true},
		{"仅管理员-禁言-管理员", true, 10002, []string{"10002", "10003"}, true},
		{"仅管理员-撤回-管理员", true, 10003, []string{"10002", "10003"}, true},

		// admin_only 关闭：不受限制
		{"非仅管理员-发群消息", false, 99999, nil, true},
		{"非仅管理员-发私聊", false, 99999, nil, true},
		{"非仅管理员-查群信息", false, 99999, nil, true},
		{"非仅管理员-沙箱", false, 99999, nil, true},
		{"非仅管理员-未知工具", false, 99999, nil, true},
	}

	for _, c := range cases {
		got := isAdminOnlyToolAllowed(c.adminOnly, c.userID, c.admins)
		if got != c.expected {
			t.Errorf("%s: isAdminOnlyToolAllowed(%v, %d, %v) = %v, want %v",
				c.name, c.adminOnly, c.userID, c.admins, got, c.expected)
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
