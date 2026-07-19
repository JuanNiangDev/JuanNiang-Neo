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
	TaskID     string `json:"task_id"`
	StepID     string `json:"step_id,omitempty"`
	ChatAreaID string `json:"chat_area_id"`
	Status     string `json:"status"`
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
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

	var lines []string
	var results []string
	for _, o := range outputs {
		if o.StepID != "" {
			results = append(results, fmt.Sprintf("[%s]: %s", o.StepID, o.Result))
		}
	}

	var areaID int64
	if len(outputs) > 0 {
		lines = append(lines, "后台任务执行结果汇总：")
		for _, r := range results {
			lines = append(lines, r)
		}
	}

	summaryPrompt := fmt.Sprintf(
		"请将以下后台任务的执行结果整理为自然语言总结，告诉用户任务完成情况：\n\n%s",
		strings.Join(lines, "\n"),
	)

	resp, err := llm.Chat(ctx, provider.ChatRequest{
		Messages: []provider.ChatMessage{
			{Role: "system", Content: "你是一个任务报告助手，请用中文生成友好的任务结果总结。"},
			{Role: "user", Content: summaryPrompt},
		},
	})
	if err != nil {
		slog.Error("Drainer LLM 调用失败", "err", err)
		d.adapter.SendGroupMsg(areaID, fmt.Sprintf("任务执行完成:\n%s", strings.Join(results, "\n")))
		return
	}

	d.adapter.SendGroupMsg(areaID, resp.Message.Content)
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

	var areaID int64
	d.adapter.SendGroupMsg(areaID, msg)
}
