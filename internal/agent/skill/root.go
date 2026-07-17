package skill

import "sync"

type SkillConfig struct {
	ID           string
	Name         string
	Description  string
	Keywords     []string // 关键词触发条件
	RegexPattern string   // 正则表达式触发条件
	PromptRef    string
	ToolRefs     []string
	McpRefs      []string
	IsActive     bool
	Priority     int // 优先级（当多个技能同时匹配时，优先级高的生效）
}

type Skill struct {
}

type SkillGroup struct {
	mu     sync.Mutex
	Skills []Skill
}

func NewSkillGroup() *SkillGroup {
	return &SkillGroup{
		mu:     sync.Mutex{},
		Skills: make([]Skill, 0),
	}
}
