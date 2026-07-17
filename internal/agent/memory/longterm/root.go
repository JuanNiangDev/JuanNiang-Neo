package longterm

import (
	"JuanNiang-Neo/internal/agent/memory"
)

type LongTermMemory struct {
	conf    memory.LongTermMemoryConfig
	HotArea map[string]any
	// dao 这里的数据库操作全部由core的dao进行
}
