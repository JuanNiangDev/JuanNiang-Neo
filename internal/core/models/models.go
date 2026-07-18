package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// ---------- 通用 JSON 字段类型 ----------

// JSONMap 是 map[string]any 的 GORM 兼容类型。
type JSONMap map[string]any

func (j JSONMap) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONMap) Scan(value any) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, j)
}

// JSONSlice 是 []string 的 GORM 兼容类型。
type JSONSlice []string

func (j JSONSlice) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONSlice) Scan(value any) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, j)
}

// ---------- 用户 ----------

type AdminUser struct {
	gorm.Model
	Username     string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	Role         string `gorm:"default:admin"`
}

// ---------- Onebot11 Adapter ----------

type Onebot11Adapter struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	Addr      string         `gorm:"column:addr;type:varchar(255);not null;comment:连接地址"`
	Port      int            `gorm:"column:port;not null;comment:连接端口"`
	Token     string         `gorm:"column:token;type:varchar(255);comment:访问令牌"`
	Admins    []int64        `gorm:"column:admins;type:json;serializer:json;comment:管理员ID列表"`
}

// ---------- LLM Provider ----------

type ModelType string

const (
	ModelTypeText      ModelType = "text_model"
	ModelTypeImage     ModelType = "image_model"
	ModelTypeEmbedding ModelType = "embedding_model"
)

type Provider struct {
	ID          string    `gorm:"primaryKey;type:uuid"`
	Name        string    `gorm:"not null"`
	Type        ModelType `gorm:"not null;index"`
	Endpoint    string    `gorm:"not null"`
	Token       string    `gorm:"not null"`
	Model       string    `gorm:"not null"`
	Temperature float32   `gorm:"default:0.7"`
	IsActive    bool      `gorm:"default:true;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// ---------- MCP Server ----------

type MCPServer struct {
	ID            string    `gorm:"primaryKey;type:uuid"`
	Name          string    `gorm:"not null"`
	ServerURL     string    `gorm:"not null"`
	Headers       JSONMap   `gorm:"type:jsonb;default:'{}'"`
	Timeout       int       `gorm:"default:30000"`
	RetryCount    int       `gorm:"default:3"`
	ToolFilter    JSONSlice `gorm:"type:jsonb;default:'[]'"`
	AutoReconnect bool      `gorm:"default:true"`
	IsActive      bool      `gorm:"default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

// ---------- Skill ----------

type Skill struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	Name         string `gorm:"not null"`
	Description  string
	Keywords     JSONSlice `gorm:"type:jsonb;default:'[]'"`
	RegexPattern string
	PromptRef    string
	ToolRefs     JSONSlice `gorm:"type:jsonb;default:'[]'"`
	McpRefs      JSONSlice `gorm:"type:jsonb;default:'[]'"`
	IsActive     bool      `gorm:"default:true"`
	IsSystem     bool      `gorm:"default:false"`
	Priority     int       `gorm:"default:0"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// ---------- Tool Config ----------

type ToolConfig struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	Name        string  `gorm:"uniqueIndex;not null"`
	Description string  `gorm:"not null"`
	Parameters  JSONMap `gorm:"type:jsonb;default:'{}'"`
	Timeout     int     `gorm:"default:30000"`
	IsActive    bool    `gorm:"default:true"`
	IsBuiltin   bool    `gorm:"default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// ---------- Prompt ----------

type PromptType string

const (
	PromptTypeSystem      PromptType = "system"
	PromptTypePersonality PromptType = "personality"
	PromptTypeCustom      PromptType = "custom"
)

type Prompt struct {
	ID        string     `gorm:"primaryKey;type:uuid"`
	Name      string     `gorm:"not null"`
	Content   string     `gorm:"type:text;not null"`
	Type      PromptType `gorm:"not null;index"`
	IsActive  bool       `gorm:"default:true"`
	Variables JSONSlice  `gorm:"type:jsonb;default:'[]'"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// ---------- Chat Area ----------

type AreaType string

const (
	AreaTypePrivate AreaType = "private"
	AreaTypeGroup   AreaType = "group"
)

type ChatArea struct {
	ID        string   `gorm:"primaryKey;type:uuid"`
	AreaType  AreaType `gorm:"not null;index"`
	TargetID  int64    `gorm:"not null;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// ---------- Session ----------

type Session struct {
	ID         string   `gorm:"primaryKey;type:uuid"`
	ChatAreaID string   `gorm:"not null;index"`
	ChatArea   ChatArea `gorm:"foreignKey:ChatAreaID"`
	Model      string   `gorm:"default:''"`
	TokenUsage int64    `gorm:"default:0"`
	MetaData   JSONMap  `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// ---------- Memory ----------

type ShortTermMemory struct {
	ID          string   `gorm:"primaryKey;type:uuid"`
	ChatAreaID  string   `gorm:"uniqueIndex;not null"`
	ChatArea    ChatArea `gorm:"foreignKey:ChatAreaID"`
	WindowSize  int      `gorm:"default:20"`
	AutoCompact bool     `gorm:"default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

type LongTermMemory struct {
	ID           string   `gorm:"primaryKey;type:uuid"`
	ChatAreaID   string   `gorm:"uniqueIndex;not null"`
	ChatArea     ChatArea `gorm:"foreignKey:ChatAreaID"`
	HotAreaSize  int      `gorm:"default:10"`
	HotMemoryTTL int      `gorm:"default:86400"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// ---------- Background Task ----------

type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusFailed  TaskStatus = "failed"
)

type BackgroundTask struct {
	ID         string     `gorm:"primaryKey;type:uuid"`
	ChatAreaID string     `gorm:"not null;index"`
	ChatArea   ChatArea   `gorm:"foreignKey:ChatAreaID"`
	Status     TaskStatus `gorm:"default:pending;index"`
	Steps      JSONMap    `gorm:"type:jsonb;default:'[]'"`
	Results    JSONMap    `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// ---------- Chat Record ----------

type ChatRecord struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	ChatAreaID string    `gorm:"not null;index"`
	ChatArea   ChatArea  `gorm:"foreignKey:ChatAreaID"`
	UserID     int64     `gorm:"not null;index"`
	Role       string    `gorm:"not null"`
	Content    string    `gorm:"type:text"`
	TokenCount int       `gorm:"default:0"`
	ToolCalls  JSONMap   `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time `gorm:"index"`
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// ---------- Plugin ----------

type Plugin struct {
	ID        string  `gorm:"primaryKey;type:uuid"`
	Name      string  `gorm:"uniqueIndex;not null"`
	Version   string  `gorm:"default:'1.0.0'"`
	Path      string  `gorm:"not null"`
	Config    JSONMap `gorm:"type:jsonb;default:'{}'"`
	IsActive  bool    `gorm:"default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// ---------- ACL Rule ----------

type ACLPermission string

const (
	ACLPermissionAllowed ACLPermission = "allowed"
	ACLPermissionDenied  ACLPermission = "denied"
)

type ACLRule struct {
	ID         int64         `gorm:"primaryKey;autoIncrement"`
	UserID     int64         `gorm:"not null;index"`
	ChatAreaID string        `gorm:"not null;index"`
	Permission ACLPermission `gorm:"not null;default:allowed"`
	Actions    JSONSlice     `gorm:"type:jsonb;default:'[]'"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// ---------- 长期记忆条目 ----------

type LongTermMemoryItem struct {
	ID         string    `gorm:"primaryKey;type:uuid"`
	ChatAreaID string    `gorm:"not null;index"`
	Content    string    `gorm:"type:text;not null"`
	Embedding  []byte    `gorm:"type:bytea"`
	Metadata   JSONMap   `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time `gorm:"index"`
}

// ---------- 管理员 QQ ----------

type AdminQQ struct {
	ID        int64 `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// ---------- 后台任务步骤结果 ----------

type TaskStepResult struct {
	TaskID     string
	StepID     string
	ChatAreaID string
	Status     TaskStatus
	Result     string
	Error      string
}
