package acl

import (
	"context"
	"errors"
	"log/slog"

	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

// ACL 访问控制列表，决定用户是否可在指定 ChatArea 执行特定操作。
type ACL struct {
	dao *dao.ACLDAO
}

func NewACL(dao *dao.ACLDAO) *ACL {
	return &ACL{dao: dao}
}

// Check 检查 userID 在 chatAreaID 中是否可执行 action。
// 默认: 无规则 = 允许所有。有 deny 规则 = 拒绝；有 allow 规则 = 仅允许指定 actions。
func (a *ACL) Check(ctx context.Context, userID int64, chatAreaID string, action string) bool {
	rule, err := a.dao.GetByUserAndChatArea(ctx, userID, chatAreaID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true
		}
		slog.Error("ACL 查询失败", "user_id", userID, "chat_area_id", chatAreaID, "err", err)
		return true
	}

	switch rule.Permission {
	case models.ACLPermissionDenied:
		return false
	case models.ACLPermissionAllowed:
		if len(rule.Actions) == 0 {
			return true
		}
		for _, a := range rule.Actions {
			if a == action || a == "*" {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// AddRule 添加或更新 ACL 规则。
func (a *ACL) AddRule(ctx context.Context, rule *models.ACLRule) error {
	existing, err := a.dao.GetByUserAndChatArea(ctx, rule.UserID, rule.ChatAreaID)
	if err == nil {
		rule.ID = existing.ID
	}
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
