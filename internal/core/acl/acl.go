package acl

import (
	"context"
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

// Check 检查 userID 在 chatAreaID 中是否被聊天黑名单禁止使用 Agent 循环。
// 黑名单语义:
//   - 无规则 = 允许所有（默认）
//   - deny all  = 禁止所有用户
//   - deny list = 禁止指定用户列表
//
// 命中黑名单返回 false（禁止），否则 true（允许）。allow/白名单规则不再生效。
// 查询失败时默认放行（fail-open）——与"无规则=允许所有"的默认语义一致，
// 但会记录 Error 日志以便排查 DB 故障。
func (a *ACL) Check(ctx context.Context, userID int64, chatAreaID string, scope models.ACLScope) bool {
	rules, err := a.dao.GetByChatAreaAndScope(ctx, chatAreaID, scope)
	if err != nil {
		log.Error("ACL 查询失败", "user_id", userID, "chat_area_id", chatAreaID, "scope", scope, "err", err)
		return true // 查询失败默认允许（黑名单语义：无规则 = 允许所有）
	}
	if len(rules) == 0 {
		return true // 无规则 = 允许所有
	}

	uidStr := strconv.FormatInt(userID, 10)
	for _, rule := range rules {
		// 仅 deny（黑名单）规则生效
		if rule.Permission != models.ACLPermissionDeny {
			continue
		}
		if rule.TargetType == models.ACLTargetAll {
			return false
		}
		if rule.TargetType == models.ACLTargetList && containsUserID(rule.UserIDs, uidStr) {
			return false
		}
	}
	return true
}

// CheckChat 检查聊天黑名单权限（ACL 现仅管理 Chat 行为）。
func (a *ACL) CheckChat(ctx context.Context, userID int64, chatAreaID string) bool {
	return a.Check(ctx, userID, chatAreaID, models.ACLScopeChat)
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
