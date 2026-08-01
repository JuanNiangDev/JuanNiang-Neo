package planner

import (
	"context"
	"encoding/json"
	"fmt"

	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/logging"
)

var log = logging.NewLogger("planner")

// LLMPlanner 阶段二：LLM 规划器。
type LLMPlanner struct {
	provider provider.Provider
}

// NewLLMPlanner 创建 LLM 规划器。
func NewLLMPlanner(p provider.Provider) *LLMPlanner {
	return &LLMPlanner{provider: p}
}

// Plan 调用 LLM 进行规划。
func (lp *LLMPlanner) Plan(ctx context.Context, msg string, contextInfo string, memories []string) (*PlannerResult, error) {
	systemPrompt := `你是 JuanNiang-Neo 的对话规划器。分析用户消息并制定回复计划。

输出 JSON 格式：
{
  "should_reply": true/false,
  "reply_style": "text" / "image" / "emoji_first" / "cq_mixed",
  "intent": "question" / "chat" / "command" / "task",
  "tool_plan": [{"tool_name": "工具名", "reason": "原因", "priority": 1}],
  "memory_hints": ["需要查询的记忆类型: fact_memory/profile_memory/..."],
  "confidence": 0.0-1.0,
  "summary": "简要规划说明"
}

规划原则：
- 简单闲聊: should_reply=true, intent=chat, 无需工具
- 需要查询信息: intent=question, 标记 memory_hints, 可能调用 search 工具
- 执行任务: intent=task, 列出 tool_plan
- 完全无关/无意义: should_reply=false
- 回复风格: 大部分是 text，涉及图片用 image，表达情绪用 emoji_first`

	messages := []provider.ChatMessage{
		{Role: "system", Content: systemPrompt},
	}

	if contextInfo != "" {
		messages = append(messages, provider.ChatMessage{
			Role: "system", Content: "【当前上下文】\n" + contextInfo,
		})
	}

	if len(memories) > 0 {
		memText := ""
		for i, m := range memories {
			if i > 0 {
				memText += "\n"
			}
			memText += fmt.Sprintf("- %s", m)
		}
		messages = append(messages, provider.ChatMessage{
			Role: "system", Content: "【相关记忆】\n" + memText,
		})
	}

	messages = append(messages, provider.ChatMessage{
		Role: "user", Content: msg,
	})

	req := provider.ChatRequest{
		Messages:    messages,
		Temperature: 0.3, // 规划任务用低温
	}

	resp, err := lp.provider.Chat(ctx, req)
	if err != nil {
		log.Error("Planner LLM 调用失败", "err", err)
		return &PlannerResult{ShouldReply: true, Intent: "chat", ReplyStyle: "text", Confidence: 0.5}, nil
	}

	var result PlannerResult
	if err := json.Unmarshal([]byte(resp.Message.Content), &result); err != nil {
		log.Warn("Planner 输出解析失败，回退到默认", "content", resp.Message.Content, "err", err)
		return &PlannerResult{ShouldReply: true, Intent: "chat", ReplyStyle: "text", Confidence: 0.5}, nil
	}

	return &result, nil
}
