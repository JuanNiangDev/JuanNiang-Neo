package acl

import (
	"context"
	"fmt"
	"strconv"

	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"
)

var log = logging.NewModule("acl")

// ACL 访问控制列表，决定用户是否可在指定 ChatArea 执行特定操作。
type ACL struct {
	dao *dao.ACLDAO
}

func NewACL(dao *dao.ACLDAO) *ACL {
	return &ACL{dao: dao}
}

// Check 检查 userID 在 chatAreaID 中是否可执行 scope 对应的操作。
// 逻辑:
//   - 无规则 = 允许所有（默认）
//   - deny all  = 拒绝所有用户
//   - deny list = 拒绝指定用户
//   - allow all = 允许所有用户
//   - allow list = 仅允许指定用户（白名单）
//   - 优先级: deny > allow，有 allow 规则时未命中则拒绝。
func (a *ACL) Check(ctx context.Context, userID int64, chatAreaID string, scope models.ACLScope) bool {
	rules, err := a.dao.GetByChatAreaAndScope(ctx, chatAreaID, scope)
	if err != nil {
		log.Error("ACL 查询失败", "user_id", userID, "chat_area_id", chatAreaID, "scope", scope, "err", err)
		return true // 查询失败默认允许
	}
	if len(rules) == 0 {
		return true // 无规则 = 允许所有
	}

	uidStr := strconv.FormatInt(userID, 10)
	hasAllowRules := false
	allowHit := false

	for _, rule := range rules {
		switch rule.Permission {
		case models.ACLPermissionDeny:
			if rule.TargetType == models.ACLTargetAll {
				return false
			}
			if rule.TargetType == models.ACLTargetList && containsUserID(rule.UserIDs, uidStr) {
				return false
			}
		case models.ACLPermissionAllow:
			hasAllowRules = true
			if rule.TargetType == models.ACLTargetAll {
				allowHit = true
			}
			if rule.TargetType == models.ACLTargetList && containsUserID(rule.UserIDs, uidStr) {
				allowHit = true
			}
		}
	}

	// 有 allow 规则时，未命中则拒绝；无 allow 规则则允许
	return !hasAllowRules || allowHit
}

// CheckChat 检查聊天权限。
func (a *ACL) CheckChat(ctx context.Context, userID int64, chatAreaID string) bool {
	return a.Check(ctx, userID, chatAreaID, models.ACLScopeChat)
}

// CheckTool 检查工具调用权限。返回 (allowed, denialMessage)。
func (a *ACL) CheckTool(ctx context.Context, userID int64, chatAreaID, toolName string) (bool, string) {
	if a.Check(ctx, userID, chatAreaID, models.ACLScopeTool) {
		return true, ""
	}
	return false, fmt.Sprintf("用户：%d的工具调用(%s)被禁止", userID, toolName)
}

// CheckMCP 检查 MCP 调用权限。返回 (allowed, denialMessage)。
func (a *ACL) CheckMCP(ctx context.Context, userID int64, chatAreaID, toolName string) (bool, string) {
	if a.Check(ctx, userID, chatAreaID, models.ACLScopeMCP) {
		return true, ""
	}
	return false, fmt.Sprintf("用户：%d的MCP调用(%s)被禁止", userID, toolName)
}

// AddRule 添加 ACL 规则。
func (a *ACL) AddRule(ctx context.Context, rule *models.ACLRule) error {
	return a.dao.Create(ctx, rule)
}

// RemoveRule 删除 ACL 规则。
func (a *ACL) RemoveRule(ctx context.Context, id int64) error {
	return a.dao.Delete(ctx, id)
}

// ListRules 列出所有 ACL 规则。
func (a *ACL) ListRules(ctx context.Context) ([]models.ACLRule, error) {
	return a.dao.List(ctx)
}

// containsUserID 检查 JSONSlice 中是否包含指定的用户 ID（字符串形式）。
func containsUserID(ids models.JSONSlice, uidStr string) bool {
	for _, id := range ids {
		if id == uidStr {
			return true
		}
	}
	return false
}
