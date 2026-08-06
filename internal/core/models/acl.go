package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- ACL Rule ----------

// ACLScope ACL 管理范围。
type ACLScope string

const (
	ACLScopeChat ACLScope = "chat" // 聊天
	ACLScopeTool ACLScope = "tool" // Agent 工具调用
	ACLScopeMCP  ACLScope = "mcp"  // MCP 调用
)

// ACLPermission 允许或拒绝。
type ACLPermission string

const (
	ACLPermissionAllow ACLPermission = "allow"
	ACLPermissionDeny  ACLPermission = "deny"
)

// ACLTargetType 规则目标类型。
type ACLTargetType string

const (
	ACLTargetAll  ACLTargetType = "all"  // 所有用户
	ACLTargetList ACLTargetType = "list" // 指定用户列表
)

// ACLRule 访问控制规则。
// 当前实际语义为「聊天黑名单」：
//   - 无规则 = 允许所有（默认）
//   - deny all  = 拒绝所有用户
//   - deny list = 拒绝指定用户列表
//   - allow 规则（allow all / allow list）保留在模型中但不参与判定（历史遗留，前端不再创建）；
//   - Scope 目前仅 chat 生效（前端固定创建 chat/deny 规则）；tool / mcp 为预留值，
//     其检查逻辑未接入运行时，配置了也不会生效。
type ACLRule struct {
	ID         int64         `gorm:"primaryKey;autoIncrement"`
	ChatAreaID string        `gorm:"not null;index"`
	Scope      ACLScope      `gorm:"not null;index;comment:管理范围(chat/tool/mcp，仅chat生效)"`
	Permission ACLPermission `gorm:"not null;default:allow;comment:允许或拒绝（仅deny生效）"`
	TargetType ACLTargetType `gorm:"not null;default:all;comment:目标(all/list)"`
	UserIDs    JSONSlice     `gorm:"type:jsonb;default:'[]';comment:目标用户ID列表(TargetType=list时有效)"`
	ToolIDs    JSONSlice     `gorm:"type:jsonb;default:'[]';comment:工具ID列表(Scope=tool时有效，预留未生效)"`
	MCPIDs     JSONSlice     `gorm:"type:jsonb;default:'[]';comment:MCP服务器ID列表(Scope=mcp时有效，预留未生效)"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

func (ACLRule) TableName() string {
	return "acl_rules"
}
