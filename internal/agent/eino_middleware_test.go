package agent

import "testing"

// TestAdminOnlyToolAllowed 验证高危工具权限判定：
// 群管理/请求处理/撤回类工具仅 Admins 列表内用户可调用，其余一律拒绝；
// 非高危工具不受限制。
func TestAdminOnlyToolAllowed(t *testing.T) {
	cases := []struct {
		name     string
		tool     string
		userID   int64
		admins   []string
		expected bool
	}{
		// 高危工具：非管理员 / 无管理员配置 → 拒绝
		{"高危-踢人-非管理员", "kick_group_member", 10001, []string{"10002"}, false},
		{"高危-禁言-非管理员", "ban_group_member", 10001, []string{"10002"}, false},
		{"高危-全员禁言-无admin配置", "set_group_whole_ban", 10001, nil, false},
		{"高危-群名片-非管理员", "set_group_card", 10001, []string{}, false},
		{"高危-好友请求-非管理员", "handle_friend_request", 10001, []string{"10002"}, false},
		{"高危-群请求-非管理员", "handle_group_request", 10001, []string{"10002"}, false},
		{"高危-撤回-非管理员", "delete_msg", 10001, []string{"10002"}, false},

		// 高危工具：管理员 → 放行
		{"高危-踢人-管理员", "kick_group_member", 10002, []string{"10002"}, true},
		{"高危-禁言-管理员", "ban_group_member", 10002, []string{"10002", "10003"}, true},
		{"高危-撤回-管理员", "delete_msg", 10003, []string{"10002", "10003"}, true},

		// 非高危工具：不受限制
		{"低危-发群消息", "send_group_msg", 99999, nil, true},
		{"低危-发私聊", "send_private_msg", 99999, nil, true},
		{"低危-查群信息", "get_group_info", 99999, nil, true},
		{"低危-沙箱", "code_exec", 99999, nil, true},
		{"低危-未知工具", "some_future_tool", 99999, nil, true},
	}

	for _, c := range cases {
		got := isAdminOnlyToolAllowed(c.tool, c.userID, c.admins)
		if got != c.expected {
			t.Errorf("%s: isAdminOnlyToolAllowed(%q, %d, %v) = %v, want %v",
				c.name, c.tool, c.userID, c.admins, got, c.expected)
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
