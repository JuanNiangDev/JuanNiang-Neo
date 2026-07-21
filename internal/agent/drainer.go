package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/memory"
	"JuanNiang-Neo/internal/agent/prompt"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/agent/session"
)

// DrainerOutput 后台任务步骤/整体完成时的输出。
type DrainerOutput struct {
	TaskID      string `json:"task_id"`
	StepID      string `json:"step_id,omitempty"`
	ChatAreaID  string `json:"chat_area_id"`
	MessageType string `json:"message_type"` // "private" / "group"
	TargetID    int64  `json:"target_id"`    // user_id (私聊) / group_id (群聊)
	Status      string `json:"status"`
	Result      string `json:"result,omitempty"`
	Error       string `json:"error,omitempty"`
	UserPrompt  string `json:"user_prompt,omitempty"` // 用户原始请求，供 drainer 上下文
}

// DrainerAgent 排水 Agent，消费后台任务结果缓冲区，整合后发送 QQ 消息。
type DrainerAgent struct {
	inputChan <-chan DrainerOutput
	providers *provider.ProviderGroup
	adapter   *adapter.Adapter
	session   *session.SessionManager
	prompt    *prompt.PromptManager
	memory    *memory.MemoryGroup

	// 按 ChatArea 分组累积结果
	pending map[string][]DrainerOutput
}

func NewDrainerAgent(
	inputChan <-chan DrainerOutput,
	providers *provider.ProviderGroup,
	adapter *adapter.Adapter,
	sessionMgr *session.SessionManager,
	promptMgr *prompt.PromptManager,
	memGrp *memory.MemoryGroup,
) *DrainerAgent {
	return &DrainerAgent{
		inputChan: inputChan,
		providers: providers,
		adapter:   adapter,
		session:   sessionMgr,
		prompt:    promptMgr,
		memory:    memGrp,
		pending:   make(map[string][]DrainerOutput),
	}
}

func (d *DrainerAgent) Run(ctx context.Context) {
	slog.Info("DrainerAgent 已启动")
	for {
		select {
		case <-ctx.Done():
			return
		case output, ok := <-d.inputChan:
			if !ok {
				return
			}
			d.handle(ctx, output)
		}
	}
}

func (d *DrainerAgent) handle(ctx context.Context, output DrainerOutput) {
	chatAreaID := output.ChatAreaID

	d.pending[chatAreaID] = append(d.pending[chatAreaID], output)

	if output.Status == "done" && output.StepID == "" {
		d.finalize(ctx, chatAreaID)
		return
	}

	if output.StepID != "" && output.Status == "done" {
		totalResults := len(d.pending[chatAreaID])
		if totalResults%3 == 0 {
			d.sendProgress(ctx, chatAreaID, output.TaskID)
		}
	}
}

func (d *DrainerAgent) finalize(ctx context.Context, chatAreaID string) {
	outputs := d.pending[chatAreaID]
	if len(outputs) == 0 {
		return
	}
	delete(d.pending, chatAreaID)

	llm := d.providers.SelectModel(provider.ModelTypeText)
	if llm == nil {
		slog.Error("Drainer: 无可用 Text 模型")
		return
	}

	// 收集任务执行结果
	var results []string
	userPrompt := ""
	for _, o := range outputs {
		if o.StepID != "" {
			results = append(results, fmt.Sprintf("[步骤 %s]: %s", o.StepID, o.Result))
		}
		if o.UserPrompt != "" {
			userPrompt = o.UserPrompt
		}
	}

	if len(results) == 0 {
		d.sendMsg(outputs[0], "后台任务执行完成。")
		return
	}

	// 获取系统提示词（人格/行为约束）
	sysPrompt := "你是一个任务报告助手，请用中文生成友好的任务结果总结。"
	if d.prompt != nil {
		if sp, err := d.prompt.BuildSystemPrompt(ctx); err == nil && sp != "" {
			sysPrompt = sp + "\n\n当前角色：后台任务排水 Agent，负责将任务执行结果以友好的方式反馈给用户。"
		}
	}

	// 获取短期记忆上下文（最近对话）
	contextLines := ""
	if d.memory != nil {
		msgs, err := d.memory.GetShortTermMessages(ctx, chatAreaID)
		if err == nil && len(msgs) > 0 {
			contextLines = "背景对话上下文：\n"
			// 取最近 6 条避免过长
			start := len(msgs) - 6
			if start < 0 {
				start = 0
			}
			for i := start; i < len(msgs); i++ {
				role := msgs[i].Role
				if role == "" {
					role = "unknown"
				}
				contextLines += fmt.Sprintf("- [%s]: %s\n", role, msgs[i].Content)
			}
		}
	}

	// 构建最终 prompt
	userReqLine := ""
	if userPrompt != "" {
		userReqLine = fmt.Sprintf("用户的原始请求是：「%s」\n", userPrompt)
	}

	summaryPrompt := fmt.Sprintf(
		"%s\n\n%s后台任务执行结果：\n%s\n\n请用自然语言将结果反馈给用户，注意：\n"+
			"- 直接告诉用户结果是什么，不要说「任务执行完毕」等冗余开场白\n"+
			"- 结合用户的原始请求自然地呈现结果，像正常对话一样\n"+
			"- 如果和对话上下文相关，可以适当引用上文",
		contextLines,
		userReqLine,
		strings.Join(results, "\n"),
	)

	resp, err := llm.Chat(ctx, provider.ChatRequest{
		Messages: []provider.ChatMessage{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: summaryPrompt},
		},
	})
	if err != nil {
		slog.Error("Drainer LLM 调用失败", "err", err)
		d.sendMsg(outputs[0], fmt.Sprintf("任务执行完成:\n%s", strings.Join(results, "\n")))
		return
	}

	d.sendMsg(outputs[0], resp.Message.Content)
}

func (d *DrainerAgent) sendProgress(ctx context.Context, chatAreaID, taskID string) {
	outputs := d.pending[chatAreaID]
	var results []string
	for _, o := range outputs {
		if o.StepID != "" {
			results = append(results, fmt.Sprintf("\u2022 %s", o.Result))
		}
	}
	msg := fmt.Sprintf("任务 %s 进度更新:\n%s", taskID[:8], strings.Join(results, "\n"))

	if len(outputs) > 0 {
		d.sendMsg(outputs[0], msg)
	}
}

// sendMsg 根据 DrainerOutput 的消息类型发送到正确目标。
func (d *DrainerAgent) sendMsg(output DrainerOutput, content string) {
	switch output.MessageType {
	case "private":
		if _, err := d.adapter.SendPrivateMsg(output.TargetID, content); err != nil {
			slog.Error("Drainer 发送私聊失败", "err", err, "user_id", output.TargetID)
		}
	case "group":
		if _, err := d.adapter.SendGroupMsg(output.TargetID, content); err != nil {
			slog.Error("Drainer 发送群聊失败", "err", err, "group_id", output.TargetID)
		}
	default:
		slog.Error("Drainer: 未知消息类型", "type", output.MessageType)
	}
}
