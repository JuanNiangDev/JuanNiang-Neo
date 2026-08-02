package dao

import (
	"crypto/rand"
	"fmt"

	"gorm.io/gorm"
)

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// ---------- DAO Bundle ----------

// Bundle 汇聚所有 DAO，方便注入。
type Bundle struct {
	User            *UserDAO
	Provider        *ProviderDAO
	MCPServer       *MCPServerDAO
	Skill           *SkillDAO
	ToolConfig      *ToolConfigDAO
	Prompt          *PromptDAO
	ChatArea        *ChatAreaDAO
	Session         *SessionDAO
	ShortTermMemory *ShortTermMemoryDAO
	LongTermMemory  *LongTermMemoryDAO
	LongTermMemItem *LongTermMemoryItemDAO
	BackgroundTask  *BackgroundTaskDAO
	ChatRecord      *ChatRecordDAO
	Plugin          *PluginDAO
	ACL             *ACLDAO
	Onebot11Adapter *Onebot11AdapterDao
	Sandbox         *SandboxConfigDAO
	T2I             *T2IConfigDAO
	Webhook         *WebhookConfigDAO
	CronJob         *CronJobDAO
	ReplyStrategy   *ReplyStrategyDAO
	SkillMemory     *SkillMemoryDAO
}

func NewBundle(db *gorm.DB) *Bundle {
	return &Bundle{
		User:            NewUserDAO(db),
		Provider:        NewProviderDAO(db),
		MCPServer:       NewMCPServerDAO(db),
		Skill:           NewSkillDAO(db),
		ToolConfig:      NewToolConfigDAO(db),
		Prompt:          NewPromptDAO(db),
		ChatArea:        NewChatAreaDAO(db),
		Session:         NewSessionDAO(db),
		ShortTermMemory: NewShortTermMemoryDAO(db),
		LongTermMemory:  NewLongTermMemoryDAO(db),
		LongTermMemItem: NewLongTermMemoryItemDAO(db),
		BackgroundTask:  NewBackgroundTaskDAO(db),
		ChatRecord:      NewChatRecordDAO(db),
		Plugin:          NewPluginDAO(db),
		ACL:             NewACLDAO(db),
		Onebot11Adapter: NewOnebot11AdapterDao(db),
		Sandbox:         NewSandboxConfigDAO(db),
		T2I:             NewT2IConfigDAO(db),
		Webhook:         NewWebhookConfigDAO(db),
		CronJob:         NewCronJobDAO(db),
		ReplyStrategy:   NewReplyStrategyDAO(db),
		SkillMemory:     NewSkillMemoryDAO(db),
	}
}
