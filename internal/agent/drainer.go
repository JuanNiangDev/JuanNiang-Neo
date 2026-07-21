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

	// 最终完成信号（含 failed 状态），整合所有结果发送
	if (output.Status == "done" || output.Status == "failed") && output.StepID == "" {
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

	// 收集步骤结果（含失败步骤）
	var stepOutputs []DrainerOutput
	var failedSteps []DrainerOutput
	for _, o := range outputs {
		if o.StepID != "" {
			if o.Status == "failed" {
				failedSteps = append(failedSteps, o)
			}
			if o.Result != "" {
				stepOutputs = append(stepOutputs, o)
			}
		}
	}

	// 先发送失败的步骤错误信息
	for _, o := range failedSteps {
		d.sendMsg(o, fmt.Sprintf("步骤 %s 执行失败: %s", o.StepID, o.Error))
	}

	if len(stepOutputs) == 0 {
		// 检查是否有最终错误消息
		for _, o := range outputs {
			if o.StepID == "" && o.Error != "" {
				d.sendMsg(o, o.Error)
				return
			}
		}
		d.sendMsg(outputs[0], "后台任务执行完成。")
		return
	}

	// 直接发送每个步骤的结果，不加任何包装文字
	for _, o := range stepOutputs {
		if o.Status != "failed" {
			d.sendMsg(o, o.Result)
		}
	}
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

// sendMsg 根据 DrainerOutput 的消息类型发送到正确目标。支持 <|msg|> 及 \n\n 多消息拆分。
func (d *DrainerAgent) sendMsg(output DrainerOutput, content string) {
	for _, part := range splitMessages(content) {
		switch output.MessageType {
		case "private":
			if _, err := d.adapter.SendPrivateMsg(output.TargetID, part); err != nil {
				slog.Error("Drainer 发送私聊失败", "err", err, "user_id", output.TargetID)
			}
		case "group":
			if _, err := d.adapter.SendGroupMsg(output.TargetID, part); err != nil {
				slog.Error("Drainer 发送群聊失败", "err", err, "group_id", output.TargetID)
			}
		default:
			slog.Error("Drainer: 未知消息类型", "type", output.MessageType)
		}
	}
}
