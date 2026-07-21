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

	// 收集步骤结果
	var stepOutputs []DrainerOutput
	userPrompt := ""
	for _, o := range outputs {
		if o.StepID != "" && o.Result != "" {
			stepOutputs = append(stepOutputs, o)
		}
		if o.UserPrompt != "" {
			userPrompt = o.UserPrompt
		}
	}

	if len(stepOutputs) == 0 {
		d.sendMsg(outputs[0], "后台任务执行完成。")
		return
	}

	// 每个步骤结果作为独立消息发送，而不是合并为一条
	baseOutput := stepOutputs[0]

	// 先发送用户原始请求的引用（如果有）
	if userPrompt != "" {
		d.sendMsg(baseOutput, fmt.Sprintf("关于「%s」的执行结果：", userPrompt))
	}

	for _, o := range stepOutputs {
		d.sendMsg(o, o.Result)
	}

	// 发送完成提示
	d.sendMsg(baseOutput, "所有后台任务已执行完毕。")
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
