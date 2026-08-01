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

你的首要任务是判断是否应该回复这条消息。

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

回复决策原则（按优先级）：
1. 如果消息是明确对机器人说的（@名字、直接提问、请求帮助）→ should_reply=true
2. 如果消息是群聊闲聊且与机器人无关 → should_reply=false
3. 如果消息包含问题但规则打分低（可能是间接提问）→ 结合上下文判断
4. 纯表情/图片/无意义内容 → should_reply=false
5. 私聊消息 → 总是 should_reply=true（除非用户要求不回复）

规划原则：
- 简单闲聊: intent=chat, 无需工具
- 需要查询信息: intent=question, 标记 memory_hints
- 执行任务: intent=task, 列出 tool_plan
- 回复风格: 大部分是 text，涉及图片用 image，表达情绪用 emoji_first

上下文中的"规则打分"是系统计算的参考分，你可以参考但不应盲从：
- 高分(>0.6)且你判断应该回复 → should_reply=true
- 低分(<0.2)但你判断应该回复（如私聊）→ 仍然 should_reply=true
- 中等分数 → 结合完整上下文判断`

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
