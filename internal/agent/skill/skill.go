package skill

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// ---------- 配置 ----------

type SkillConfig struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Keywords     []string `json:"keywords"`
	RegexPattern string   `json:"regex_pattern,omitempty"`
	PromptRefs   []string `json:"prompt_refs"`
	ToolRefs     []string `json:"tool_refs,omitempty"`
	McpRefs      []string `json:"mcp_refs,omitempty"`
	IsActive     bool     `json:"is_active"`
	IsSystem     bool     `json:"is_system"`
	Priority     int      `json:"priority"`
}

// ---------- 引擎 ----------

type SkillEngine struct {
	mu     sync.RWMutex
	skills map[string]*SkillConfig // id -> skill
	sorted []*SkillConfig          // 按优先级排序
}

func NewSkillEngine() *SkillEngine {
	return &SkillEngine{
		skills: make(map[string]*SkillConfig),
	}
}

func (se *SkillEngine) AddSkill(s *SkillConfig) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.skills[s.ID] = s
	se.resort()
}

func (se *SkillEngine) DeleteSkill(id string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	delete(se.skills, id)
	se.resort()
}

func (se *SkillEngine) GetSkill(id string) (*SkillConfig, bool) {
	se.mu.RLock()
	defer se.mu.RUnlock()
	s, ok := se.skills[id]
	return s, ok
}

func (se *SkillEngine) ListSkills() []*SkillConfig {
	se.mu.RLock()
	defer se.mu.RUnlock()
	out := make([]*SkillConfig, len(se.sorted))
	copy(out, se.sorted)
	return out
}

// GetSystemSkills 返回所有激活的系统技能。
func (se *SkillEngine) GetSystemSkills() []*SkillConfig {
	se.mu.RLock()
	defer se.mu.RUnlock()
	var list []*SkillConfig
	for _, s := range se.sorted {
		if s.IsActive && s.IsSystem {
			list = append(list, s)
		}
	}
	return list
}

// Match 根据用户输入匹配技能。按优先级返回第一个匹配的。
func (se *SkillEngine) Match(input string) (*SkillConfig, bool) {
	se.mu.RLock()
	defer se.mu.RUnlock()

	input = strings.TrimSpace(input)

	for _, s := range se.sorted {
		if !s.IsActive {
			continue
		}

		// 关键词匹配
		for _, kw := range s.Keywords {
			if strings.Contains(input, kw) {
				return s, true
			}
		}

		// 正则匹配
		if s.RegexPattern != "" {
			re, err := regexp.Compile(s.RegexPattern)
			if err == nil && re.MatchString(input) {
				return s, true
			}
		}
	}

	return nil, false
}

// Activate 激活技能 (返回该技能引用的工具/提示词配置)。
func (se *SkillEngine) Activate(_ context.Context, s *SkillConfig) *SkillConfig {
	return s
}

func (se *SkillEngine) resort() {
	se.sorted = make([]*SkillConfig, 0, len(se.skills))
	for _, s := range se.skills {
		se.sorted = append(se.sorted, s)
	}
	sort.Slice(se.sorted, func(i, j int) bool {
		return se.sorted[i].Priority > se.sorted[j].Priority
	})
}
