package shortterm

import (
	"JuanNiang-Neo/internal/agent/memory"
)

type ShortTermMemory struct {
	conf memory.ShortTermMemoryConfig
	// cache 这里的数据库操作全部由core的cache进行
}
