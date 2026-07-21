package agent

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// cqCodeMatch 匹配 [CQ:type,key=val,...] 格式的 CQ 码。
var cqCodeMatch = regexp.MustCompile(`\[CQ:[^\]]+\]`)

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
	UserPrompt  string `json:"user_prompt,omitempty"` // 用户原始请求

	// MediaPayloads 从步骤结果中提取的 CQ 码（图片等），由主 Agent 直接发送到 QQ。
	MediaPayloads []string `json:"media_payloads,omitempty"`
}

// DrainerAgent 排水 Agent，消费后台任务结果，汇总后发送给主 Agent 处理。
type DrainerAgent struct {
	inputChan  <-chan DrainerOutput
	resultChan chan<- DrainerOutput // 汇总后的最终结果发给主 Agent

	// 按 ChatArea 分组累积结果
	pending map[string][]DrainerOutput
}

func NewDrainerAgent(
	inputChan <-chan DrainerOutput,
	resultChan chan<- DrainerOutput,
) *DrainerAgent {
	return &DrainerAgent{
		inputChan:  inputChan,
		resultChan: resultChan,
		pending:    make(map[string][]DrainerOutput),
	}
}

func (d *DrainerAgent) Run(ctx context.Context) {
	slog.Info("DrainerAgent 已启动 (result → bgTaskResultChan)")
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

	// 最终完成信号（含 failed 状态），整合所有结果发送给主 Agent
	if (output.Status == "done" || output.Status == "failed") && output.StepID == "" {
		d.finalize(ctx, chatAreaID)
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
	var failedSteps []DrainerOutput
	var allMedia []string
	for _, o := range outputs {
		if o.StepID != "" {
			if o.Status == "failed" {
				failedSteps = append(failedSteps, o)
			}
			if o.Result != "" {
				// 从结果中提取 CQ 码，记录到 MediaPayloads
				media := cqCodeMatch.FindAllString(o.Result, -1)
				allMedia = append(allMedia, media...)
				stepOutputs = append(stepOutputs, o)
			}
		}
	}

	// 汇总所有步骤结果 + 错误（CQ 码替换为占位符，不截断）
	var summaryParts []string
	for _, o := range failedSteps {
		summaryParts = append(summaryParts, fmt.Sprintf("[失败] %s: %s", o.StepID, o.Error))
	}
	for _, o := range stepOutputs {
		if o.Status != "failed" {
			// 清除 CQ 码，替换为 [图片/文件] 占位符
			cleanResult := cqCodeMatch.ReplaceAllString(o.Result, "[媒体内容]")
			summaryParts = append(summaryParts, fmt.Sprintf("[完成] %s: %s", o.StepID, truncateClean(cleanResult, 500)))
		}
	}

	summary := fmt.Sprintf("[后台任务已完成]\n%s", strings.Join(summaryParts, "\n"))
	userPrompt := ""
	errMsg := ""
	if len(outputs) > 0 {
		userPrompt = outputs[0].UserPrompt
	}
	if len(failedSteps) > 0 {
		errMsg = fmt.Sprintf("%d 个步骤失败", len(failedSteps))
	}

	finalStatus := "done"
	if len(stepOutputs) == 0 && len(failedSteps) > 0 {
		finalStatus = "failed"
	}

	slog.Info("Drainer 汇总完成，发送给主 Agent", "chat_area_id", chatAreaID,
		"status", finalStatus, "failed", len(failedSteps), "success", len(stepOutputs), "media", len(allMedia))

	// 发送汇总结果给主 Agent
	d.resultChan <- DrainerOutput{
		TaskID:        outputs[0].TaskID,
		ChatAreaID:    chatAreaID,
		MessageType:   outputs[0].MessageType,
		TargetID:      outputs[0].TargetID,
		Status:        finalStatus,
		Result:        summary,
		Error:         errMsg,
		UserPrompt:    userPrompt,
		MediaPayloads: allMedia,
	}
}

// truncateClean 截断已清理的纯文本到指定长度。
func truncateClean(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
