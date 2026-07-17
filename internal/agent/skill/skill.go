package skill

func (s *SkillGroup) AddSkill(conf *SkillConfig) error {}

func (s *SkillGroup) DeleteSkill(skillID string) error {}

func (s *SkillGroup) GetSkill(skillID string) (Skill, error) {}

func (s *SkillGroup) ListSkills() ([]Skill, error) {}
