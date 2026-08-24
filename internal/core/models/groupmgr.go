package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- 群管理（系统级功能，替代 redrock_group_manager Lua 插件） ----------

// GroupMgrConfig 群管理配置（单行表）。
// 阈值语义：RAG 语义核实 score ≥ HighScore 直接处罚；
// LowScore < score < HighScore 模棱两可送 LLM；
// score ≤ LowScore 时仅"关键词/卡片命中"送 LLM（词是硬信号），否则放行。
type GroupMgrConfig struct {
	ID            uint    `gorm:"primarykey"`
	Enabled       bool    `gorm:"not null;default:false"` // 总开关
	LLMReview     bool    `gorm:"not null;default:true"`  // LLM 审核开关（关闭后 RAG 高置信直罚、其余放行）
	HighScore     float64 `gorm:"not null;default:0.75"`  // RAG 高置信阈值（≥ 直接处罚）
	LowScore      float64 `gorm:"not null;default:0.5"`   // RAG 模棱两可下限（≤ 视为不违规）
	FallbackScore float64 `gorm:"not null;default:0.6"`   // LLM 异常 + RAG 模棱两可时的分数兜底（≥ 直罚）

	// ExcludeGroups 排除检测的群 ID 列表（这些群不跑任何检测/惩罚）。
	ExcludeGroups JSONSlice `gorm:"type:jsonb;default:'[]'"`

	// LLM 审核提示词（默认值平移自 redrock_group_manager，面板可编辑）。
	LLMCriteria       string `gorm:"type:text"` // 公共判定标准（拼接进两套提示词）
	LLMGrayPrompt     string `gorm:"type:text"` // 常规审查提示词（灰色词 / 低置信有词）
	LLMHighRiskPrompt string `gorm:"type:text"` // 高危复核提示词（敏感/黑名单/卡片/RAG 高置信复核）

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (GroupMgrConfig) TableName() string { return "group_mgr_configs" }

// GroupMgrWord 违规关键词条（黑色/灰色/敏感三分类，Web 可增删/txt 导入）。
// Word 统一小写去重；Source 区分系统种子（go:embed 首次导入）与用户导入。
// RAGSynced 标记该词条是否已同步到 RAG 向量库（同步/导入成功时置 true，删除/新增时置 false）。
// RAGTag 是该词条派生的 RAG-Service tag UUID（ragtag.Word(id)），供面板展示与对账。
type GroupMgrWord struct {
	ID        uint   `gorm:"primarykey"`
	Word      string `gorm:"not null;uniqueIndex"`
	Category  string `gorm:"not null;index"` // black / gray / sensitive
	Source    string `gorm:"not null;default:'system'"`
	RAGSynced bool   `gorm:"not null;default:false"` // 是否已同步到 RAG 向量库
	RAGTag    string `gorm:"type:varchar(64)"`       // 派生的 RAG tag UUID（展示用）
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (GroupMgrWord) TableName() string { return "group_mgr_words" }

// GroupMgrSample RAG 违规样本（向量库本体，tag = ragtag.Sample(id)）。
// 来源：seed（词条/关键词导入种子）、learn（LLM 确认违规自动入库）、import（txt 导入）。
type GroupMgrSample struct {
	ID        uint   `gorm:"primarykey"`
	Text      string `gorm:"type:text;not null"`
	Category  string `gorm:"not null;index"` // ad / sensitive
	Source    string `gorm:"not null;default:'seed'"`
	HitCount  int    `gorm:"not null;default:0"` // RAG 高置信命中次数（面板展示）
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (GroupMgrSample) TableName() string { return "group_mgr_samples" }

// GroupMgrViolation 违规记录（群+用户唯一；count = 当前违规等级 1/2/3）。
// Username 记录处罚时该用户的群名片/昵称（面板展示用，可能过期）。
// DetectionPath 记录判定来源：rag / keyword / llm（LLM 确认违规后追罚）。
// LLMReason 记录 LLM 审核返回的 reason（detection_path=llm 时有值，面板可查看）。
type GroupMgrViolation struct {
	ID            uint   `gorm:"primarykey"`
	GroupID       int64  `gorm:"not null;uniqueIndex:idx_gm_viol_g_u"`
	UserID        int64  `gorm:"not null;uniqueIndex:idx_gm_viol_g_u"`
	Count         int    `gorm:"not null;default:1"`
	Username      string `gorm:"type:varchar(128)"` // 处罚时群名片/昵称
	DetectionPath string `gorm:"type:varchar(16)"`  // rag / keyword / llm
	LLMReason     string `gorm:"type:text"`         // LLM 审核返回的 reason
	UpdatedAt     time.Time
}

func (GroupMgrViolation) TableName() string { return "group_mgr_violations" }

// GroupMgrWhitelist 白名单 QQ（不参与任何检测；加入时清违规记录并解禁言）。
type GroupMgrWhitelist struct {
	ID        uint  `gorm:"primarykey"`
	QQ        int64 `gorm:"not null;uniqueIndex"`
	CreatedAt time.Time
}

func (GroupMgrWhitelist) TableName() string { return "group_mgr_whitelist" }

// GroupMgrAdmin 手动管理员 QQ（群角色无法识别时的手动指定）。
type GroupMgrAdmin struct {
	ID        uint  `gorm:"primarykey"`
	QQ        int64 `gorm:"not null;uniqueIndex"`
	CreatedAt time.Time
}

func (GroupMgrAdmin) TableName() string { return "group_mgr_admins" }

// GroupMgrStat 群管理统计/状态 kv（value 为字符串，兼容时间戳列表等非数值态）。
// key 约定："{group_id}:stats:{metric}" 统计计数；"{group_id}:ims:{user}" 图片刷屏时间戳等。
type GroupMgrStat struct {
	Key   string `gorm:"primaryKey;type:varchar(192)"`
	Value string `gorm:"type:text;not null;default:''"`
}

func (GroupMgrStat) TableName() string { return "group_mgr_stats" }
