package skill

import (
	"context"
	"testing"
)

func TestSkillEngine_MatchKeyword(t *testing.T) {
	se := NewSkillEngine()
	se.AddSkill(&SkillConfig{
		ID:       "1",
		Name:     "ping",
		Keywords: []string{"/ping", "ping"},
		IsActive: true,
		Priority: 10,
	})

	s, ok := se.Match("/ping")
	if !ok || s.Name != "ping" {
		t.Errorf("expected ping skill, got %v", s)
	}

	_, ok = se.Match("hello")
	if ok {
		t.Error("should not match")
	}
}

func TestSkillEngine_MatchRegex(t *testing.T) {
	se := NewSkillEngine()
	se.AddSkill(&SkillConfig{
		ID:           "2",
		Name:         "weather",
		RegexPattern: `天气|weather`,
		IsActive:     true,
		Priority:     5,
	})

	s, ok := se.Match("今天天气怎么样")
	if !ok || s.Name != "weather" {
		t.Errorf("expected weather skill, got %v", s)
	}

	s, ok = se.Match("what is the weather")
	if !ok || s.Name != "weather" {
		t.Errorf("expected weather skill, got %v", s)
	}
}

func TestSkillEngine_Priority(t *testing.T) {
	se := NewSkillEngine()
	se.AddSkill(&SkillConfig{ID: "a", Name: "low", Keywords: []string{"test"}, IsActive: true, Priority: 1})
	se.AddSkill(&SkillConfig{ID: "b", Name: "high", Keywords: []string{"test"}, IsActive: true, Priority: 10})

	s, ok := se.Match("test message")
	if !ok || s.Name != "high" {
		t.Errorf("expected high priority skill, got %v", s)
	}
}

func TestSkillEngine_Inactive(t *testing.T) {
	se := NewSkillEngine()
	se.AddSkill(&SkillConfig{ID: "x", Name: "disabled", Keywords: []string{"foo"}, IsActive: false, Priority: 10})

	_, ok := se.Match("foo")
	if ok {
		t.Error("inactive skill should not match")
	}
}

func TestSkillEngine_GetSystemSkills(t *testing.T) {
	se := NewSkillEngine()
	se.AddSkill(&SkillConfig{ID: "s1", Name: "sys1", IsActive: true, IsSystem: true, Priority: 5})
	se.AddSkill(&SkillConfig{ID: "s2", Name: "usr1", IsActive: true, IsSystem: false, Priority: 5})

	list := se.GetSystemSkills()
	if len(list) != 1 || list[0].Name != "sys1" {
		t.Errorf("expected 1 system skill, got %d", len(list))
	}
}

func TestSkillEngine_Delete(t *testing.T) {
	se := NewSkillEngine()
	se.AddSkill(&SkillConfig{ID: "d", Name: "del", Keywords: []string{"bye"}, IsActive: true, Priority: 1})

	_, ok := se.Match("bye")
	if !ok {
		t.Error("should match before delete")
	}

	se.DeleteSkill("d")

	_, ok = se.Match("bye")
	if ok {
		t.Error("should not match after delete")
	}

	_, ok = se.GetSkill("d")
	if ok {
		t.Error("should not get after delete")
	}
}

func TestSkillEngine_List(t *testing.T) {
	se := NewSkillEngine()
	se.AddSkill(&SkillConfig{ID: "1", Name: "a", IsActive: true, Priority: 1})
	se.AddSkill(&SkillConfig{ID: "2", Name: "b", IsActive: true, Priority: 2})

	list := se.ListSkills()
	if len(list) != 2 {
		t.Errorf("expected 2 skills, got %d", len(list))
	}
	if list[0].Name != "b" {
		t.Error("higher priority should come first")
	}
}

func TestSkillEngine_Activate(t *testing.T) {
	se := NewSkillEngine()
	cfg := &SkillConfig{ID: "act", Name: "active-test", IsActive: true}

	result := se.Activate(context.Background(), cfg)
	if result != cfg {
		t.Error("Activate should return the config")
	}
}
