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
// - 无规则 = 允许所有（默认）
// - deny all  = 拒绝所有用户
// - deny list = 拒绝指定用户
// - allow all = 允许所有用户
// - allow list = 仅允许指定用户（白名单）
// 检查优先级: deny > allow，有 allow 规则时未命中则拒绝。
type ACLRule struct {
	ID         int64         `gorm:"primaryKey;autoIncrement"`
	ChatAreaID string        `gorm:"not null;index"`
	Scope      ACLScope      `gorm:"not null;index;comment:管理范围(chat/tool/mcp)"`
	Permission ACLPermission `gorm:"not null;default:allow;comment:允许或拒绝"`
	TargetType ACLTargetType `gorm:"not null;default:all;comment:目标(all/list)"`
	UserIDs    JSONSlice     `gorm:"type:jsonb;default:'[]';comment:目标用户ID列表(TargetType=list时有效)"`
	ToolIDs    JSONSlice     `gorm:"type:jsonb;default:'[]';comment:工具ID列表(Scope=tool时有效)"`
	MCPIDs     JSONSlice     `gorm:"type:jsonb;default:'[]';comment:MCP服务器ID列表(Scope=mcp时有效)"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

func (ACLRule) TableName() string {
	return "acl_rules"
}
