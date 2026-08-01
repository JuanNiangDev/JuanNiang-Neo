package agent

import (
	"context"
	"time"

	"JuanNiang-Neo/internal/logging"
)

var drainLog = logging.NewLogger("drainer")

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

// DrainerAgent 排水 Agent，消费后台任务步骤结果，全部完成后汇报给主 Agent。
// 不再发送消息到 QQ，只产出 DrainerOutput 到 BgTaskResultChan。
type DrainerAgent struct {
	inputChan  <-chan DrainerOutput
	resultChan chan<- DrainerOutput

	// 按任务跟踪
	tasks map[string]*DrainTask
}

// DrainTask 排水任务跟踪。
type DrainTask struct {
	TaskID      string
	ChatAreaID  string
	MessageType string
	TargetID    int64
	UserPrompt  string
	TotalSteps  int
	Steps       map[string]DrainStepResult
	Status      string // pending / running / done / failed
	StartedAt   time.Time
	FinishedAt  time.Time
}

// DrainStepResult 单步执行结果。
type DrainStepResult struct {
	ToolName string
	Status   string // running / done / failed
	Result   string
	Error    string
}

// NewDrainerAgent 创建排水 Agent。
func NewDrainerAgent(inputChan <-chan DrainerOutput, resultChan chan<- DrainerOutput) *DrainerAgent {
	return &DrainerAgent{
		inputChan:  inputChan,
		resultChan: resultChan,
		tasks:      make(map[string]*DrainTask),
	}
}

// Run 启动排水 Agent 事件循环。
func (d *DrainerAgent) Run(ctx context.Context) {
	drainLog.Info("DrainerAgent 已启动 (仅汇报最终结果)")
	for {
		select {
		case <-ctx.Done():
			return
		case output, ok := <-d.inputChan:
			if !ok {
				return
			}
			d.handle(output)
		}
	}
}

func (d *DrainerAgent) handle(output DrainerOutput) {
	task, exists := d.tasks[output.TaskID]
	if !exists {
		task = &DrainTask{
			TaskID:      output.TaskID,
			ChatAreaID:  output.ChatAreaID,
			MessageType: output.MessageType,
			TargetID:    output.TargetID,
			UserPrompt:  output.UserPrompt,
			Steps:       make(map[string]DrainStepResult),
			Status:      "running",
			StartedAt:   time.Now(),
		}
		d.tasks[output.TaskID] = task
	}

	// 记录步骤结果
	if output.StepID != "" {
		task.Steps[output.StepID] = DrainStepResult{
			Status: output.Status,
			Result: output.Result,
			Error:  output.Error,
		}
	}

	// 最终完成信号（step 级输出不转发给主 Agent）
	if (output.Status == "done" || output.Status == "failed") && output.StepID == "" {
		task.Status = output.Status
		task.FinishedAt = time.Now()

		// 构造汇总报告，发一条给主 Agent
		summary := d.buildSummary(task)
		d.resultChan <- DrainerOutput{
			TaskID:      task.TaskID,
			ChatAreaID:  task.ChatAreaID,
			MessageType: task.MessageType,
			TargetID:    task.TargetID,
			Status:      task.Status,
			Result:      summary,
			UserPrompt:  task.UserPrompt,
		}

		drainLog.Info("排水任务完成，汇报给主 Agent", "task_id", task.TaskID, "status", task.Status)
	}
}

func (d *DrainerAgent) buildSummary(task *DrainTask) string {
	summary := "[后台任务已完成]\n"
	doneCount := 0
	failedCount := 0
	for _, step := range task.Steps {
		if step.Status == "failed" {
			summary += "- [失败] " + step.ToolName + ": " + step.Error + "\n"
			failedCount++
		} else if step.Status == "done" {
			summary += "- [完成] " + step.ToolName + ": " + truncate(step.Result, 200) + "\n"
			doneCount++
		}
	}
	summary += "---\n"
	summary += "成功: " + itoa(doneCount) + ", 失败: " + itoa(failedCount)
	return summary
}

// QueryProgress 查询任务进度（主 Agent 可主动调用）。
func (d *DrainerAgent) QueryProgress(taskID string) (*DrainTask, bool) {
	task, ok := d.tasks[taskID]
	return task, ok
}

// CleanTask 清理已完成的任务（释放内存）。
func (d *DrainerAgent) CleanTask(taskID string) {
	delete(d.tasks, taskID)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
