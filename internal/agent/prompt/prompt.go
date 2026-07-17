package prompt

import (
	"bytes"
	"context"
	"strings"
	"text/template"
	"time"

	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
)

// PromptType 提示词类型。
type PromptType string

const (
	PromptTypeSystem      PromptType = "system"
	PromptTypePersonality PromptType = "personality"
	PromptTypeCustom      PromptType = "custom"
)

// PromptManager 提示词管理器。
type PromptManager struct {
	dao *dao.PromptDAO
}

func NewPromptManager(dao *dao.PromptDAO) *PromptManager {
	return &PromptManager{dao: dao}
}

// RenderTemplate 渲染单个模板。
func (pm *PromptManager) RenderTemplate(tmpl string, vars map[string]any) (string, error) {
	t, err := template.New("prompt").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// BuildSystemPrompt 构建系统提示词 (system + personality prompts)。
func (pm *PromptManager) BuildSystemPrompt(ctx context.Context, vars map[string]any) (string, error) {
	var parts []string

	systemPrompts, err := pm.dao.ListByType(ctx, models.PromptTypeSystem)
	if err != nil {
		return "", err
	}
	for _, p := range systemPrompts {
		rendered, err := pm.RenderTemplate(p.Content, vars)
		if err != nil {
			return "", err
		}
		parts = append(parts, rendered)
	}

	personalityPrompts, err := pm.dao.ListByType(ctx, models.PromptTypePersonality)
	if err != nil {
		return "", err
	}
	for _, p := range personalityPrompts {
		rendered, err := pm.RenderTemplate(p.Content, vars)
		if err != nil {
			return "", err
		}
		parts = append(parts, rendered)
	}

	return strings.Join(parts, "\n\n"), nil
}

// BuildFullContext 构建完整上下文 (system prompts + 长期记忆 + 工具/技能描述)。
func (pm *PromptManager) BuildFullContext(ctx context.Context, vars map[string]any, longTermMemories []string, toolDescriptions string) (string, error) {
	systemPrompt, err := pm.BuildSystemPrompt(ctx, vars)
	if err != nil {
		return "", err
	}

	var parts []string
	if systemPrompt != "" {
		parts = append(parts, systemPrompt)
	}

	if len(longTermMemories) > 0 {
		parts = append(parts, "以下是相关的长期记忆：")
		for i, mem := range longTermMemories {
			parts = append(parts, mem)
			_ = i
		}
	}

	if toolDescriptions != "" {
		parts = append(parts, "可用工具：\n"+toolDescriptions)
	}

	return strings.Join(parts, "\n\n"), nil
}

// GetDefaultVars 获取默认模板变量。
func GetDefaultVars(userName, groupName string) map[string]any {
	return map[string]any{
		"Time":      time.Now().Format("2006-01-02 15:04:05"),
		"UserName":  userName,
		"GroupName": groupName,
	}
}

// GetByID 获取指定 Prompt。
func (pm *PromptManager) GetByID(ctx context.Context, id string) (*models.Prompt, error) {
	return pm.dao.GetByID(ctx, id)
}

// List 列出所有 Prompt。
func (pm *PromptManager) List(ctx context.Context) ([]models.Prompt, error) {
	return pm.dao.List(ctx)
}
