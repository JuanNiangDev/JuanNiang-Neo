package memory

import "time"

type ShortTermMemoryConfig struct {
	WindowSize int64
}

type LongTermMemoryConfig struct {
	HotAreaSize  int64
	HotMemoryTTL time.Duration
}

type Memory interface {
}

type MemoryGroup struct {
	LongTerm             Memory
	ShortTerm            Memory
	BackGroundTaskMemory Memory
}

func NewMemoryGroup(longTermMem Memory, shortTermMem Memory, backGroundTaskMemory Memory) *MemoryGroup {
	return &MemoryGroup{
		LongTerm:             longTermMem,
		ShortTerm:            shortTermMem,
		BackGroundTaskMemory: backGroundTaskMemory,
	}
}
