package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"JuanNiang-Neo/internal/agent/mcp"
	"JuanNiang-Neo/internal/agent/tool"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
	"golang.org/x/sync/errgroup"
)

// TaskStep 后台任务步骤定义。
type TaskStep struct {
	ID       string
	ToolName string
	Args     json.RawMessage
	Depends  []string
}

// BackgroundTaskExecutor 后台任务执行器，支持 DAG + errgroup 并发。
type BackgroundTaskExecutor struct {
	tools      *tool.ToolRegistry
	mcpGroup   *mcp.MCPGroup
	dao        *dao.BackgroundTaskDAO
	outputChan chan<- DrainerOutput
	mu         sync.Mutex
}

func NewBackgroundTaskExecutor(tools *tool.ToolRegistry, mcpGroup *mcp.MCPGroup, dao *dao.BackgroundTaskDAO, outputChan chan<- DrainerOutput) *BackgroundTaskExecutor {
	return &BackgroundTaskExecutor{
		tools:      tools,
		mcpGroup:   mcpGroup,
		dao:        dao,
		outputChan: outputChan,
	}
}

// Submit 提交一个后台任务，返回 taskID。
func (b *BackgroundTaskExecutor) Submit(ctx context.Context, chatAreaID string, msgType string, targetID int64, userPrompt string, steps []TaskStep) (string, error) {
	stepsJSON, _ := json.Marshal(steps)
	stepsMap := make(models.JSONMap)
	_ = json.Unmarshal(stepsJSON, &stepsMap)
	task := &models.BackgroundTask{
		ChatAreaID:  chatAreaID,
		MessageType: msgType,
		TargetID:    targetID,
		UserPrompt:  userPrompt,
		Status:      models.TaskStatusPending,
		Steps:       stepsMap,
		Results:     models.JSONMap{},
	}
	if err := b.dao.Create(ctx, task); err != nil {
		return "", err
	}

	go b.executeAsync(task, msgType, targetID, userPrompt, steps)
	return task.ID, nil
}

// executeTool 统一执行工具：优先查 MCP，再查 ToolRegistry。
func (b *BackgroundTaskExecutor) executeTool(ctx context.Context, toolName string, args json.RawMessage) (string, error) {
	// 优先检查 MCP 工具（避免与内置工具同名时路由错误）
	if b.mcpGroup != nil {
		if b.mcpGroup.HasTool(ctx, toolName) {
			slog.Info("BgTask 执行 MCP 工具", "tool", toolName)
			return b.mcpGroup.CallTool(ctx, toolName, args)
		}
	}
	// 回退到内置工具
	slog.Info("BgTask 执行内置工具", "tool", toolName)
	return b.tools.Execute(ctx, toolName, args)
}

func (b *BackgroundTaskExecutor) executeAsync(task *models.BackgroundTask, msgType string, targetID int64, userPrompt string, steps []TaskStep) {
	ctx := context.Background()

	b.dao.UpdateStatus(ctx, task.ID, models.TaskStatusRunning)

	deps := make(map[string][]string)
	for _, s := range steps {
		deps[s.ID] = s.Depends
	}

	completed := make(map[string]bool)
	results := make(map[string]string)
	stepErrors := make(map[string]string) // 记录每步错误
	hasFailed := false
	var mu sync.Mutex

	slog.Info("后台任务执行开始", "task_id", task.ID, "steps", len(steps))
	for _, s := range steps {
		slog.Info("后台任务步骤", "task_id", task.ID, "step_id", s.ID, "tool", s.ToolName)
	}

	for len(completed) < len(steps) {
		g, gCtx := errgroup.WithContext(ctx)

		for _, s := range steps {
			mu.Lock()
			isCompleted := completed[s.ID]
			depsMet := true
			for _, d := range s.Depends {
				if !completed[d] {
					depsMet = false
					break
				}
			}
			mu.Unlock()

			if isCompleted || !depsMet {
				continue
			}

			step := s
			g.Go(func() error {
				slog.Info("后台任务步骤执行中", "task_id", task.ID, "step_id", step.ID, "tool", step.ToolName)
				result, err := b.executeTool(gCtx, step.ToolName, step.Args)
				mu.Lock()
				defer mu.Unlock()

				if err != nil {
					slog.Error("后台任务步骤执行失败", "task_id", task.ID, "step_id", step.ID, "tool", step.ToolName, "err", err)
					results[step.ID] = "error: " + err.Error()
					stepErrors[step.ID] = err.Error()
					hasFailed = true
				} else {
					slog.Info("后台任务步骤执行完成", "task_id", task.ID, "step_id", step.ID, "tool", step.ToolName, "result_len", len(result))
					results[step.ID] = result
				}
				completed[step.ID] = true

				b.outputChan <- DrainerOutput{
					TaskID:      task.ID,
					StepID:      step.ID,
					ChatAreaID:  task.ChatAreaID,
					MessageType: msgType,
					TargetID:    targetID,
					Status:      stepStatus(err),
					Result:      result,
					Error:       errMsg(err),
					UserPrompt:  userPrompt,
				}

				return nil
			})
		}

		_ = g.Wait()
	}

	resultsJSON, _ := json.Marshal(results)
	resultsMap := make(models.JSONMap)
	_ = json.Unmarshal(resultsJSON, &resultsMap)

	// 根据是否有失败步骤来设置最终状态
	finalStatus := models.TaskStatusDone
	if hasFailed {
		finalStatus = models.TaskStatusFailed
		// 将错误信息存入结果，方便前端展示
		errorsJSON, _ := json.Marshal(stepErrors)
		resultsMap["_errors"] = string(errorsJSON)
	}

	b.dao.Update(ctx, &models.BackgroundTask{
		ID:      task.ID,
		Status:  finalStatus,
		Results: resultsMap,
	})

	// 根据是否有错误，发送不同的最终通知
	var finalResult string
	if hasFailed {
		var failedTools []string
		for stepID, errMsg := range stepErrors {
			failedTools = append(failedTools, fmt.Sprintf("%s: %s", stepID, errMsg))
		}
		finalResult = fmt.Sprintf("后台任务执行失败 (%d/%d 步骤出错): %s", len(stepErrors), len(steps), strings.Join(failedTools, "; "))
	} else {
		finalResult = "所有步骤已完成"
	}

	b.outputChan <- DrainerOutput{
		TaskID:      task.ID,
		ChatAreaID:  task.ChatAreaID,
		MessageType: msgType,
		TargetID:    targetID,
		Status:      string(finalStatus),
		Result:      finalResult,
		Error:       finalResult,
		UserPrompt:  userPrompt,
	}

	slog.Info("后台任务执行完成", "task_id", task.ID, "status", finalStatus, "failed", hasFailed)
}

// Run 启动后台任务执行器，并恢复 DB 中未完成的任务。
func (b *BackgroundTaskExecutor) Run(ctx context.Context) {
	slog.Info("BackgroundTaskExecutor 已启动")

	// 恢复重启前未完成的任务
	b.recoverTasks(ctx)

	<-ctx.Done()
}

// recoverTasks 从 DB 恢复所有 pending/running 状态的任务并重新执行。
func (b *BackgroundTaskExecutor) recoverTasks(ctx context.Context) {
	tasks, err := b.dao.ListPending(ctx)
	if err != nil {
		slog.Error("恢复后台任务失败: 查询 DB 出错", "err", err)
		return
	}
	if len(tasks) == 0 {
		return
	}

	slog.Info("正在恢复未完成的后台任务", "count", len(tasks))
	for _, t := range tasks {
		// 解析 Steps JSON → []TaskStep
		stepsJSON, _ := json.Marshal(t.Steps)
		var steps []TaskStep
		if err := json.Unmarshal(stepsJSON, &steps); err != nil {
			slog.Error("恢复任务失败: 解析步骤出错", "task_id", t.ID, "err", err)
			b.dao.UpdateStatus(ctx, t.ID, models.TaskStatusFailed)
			continue
		}

		// 将 running 状态视为需要重新执行（可能是上次崩溃导致）
		// 跳过缺少发送目标信息的旧任务（target_id=0 表示升级前创建的任务）
		if t.TargetID == 0 || t.MessageType == "" {
			slog.Warn("跳过无法恢复的旧任务（缺少发送目标）", "task_id", t.ID)
			b.dao.UpdateStatus(ctx, t.ID, models.TaskStatusFailed)
			continue
		}
		slog.Info("恢复执行后台任务", "task_id", t.ID, "status", t.Status)
		go b.executeAsync(&t, t.MessageType, t.TargetID, t.UserPrompt, steps)
	}
}

func stepStatus(err error) string {
	if err != nil {
		return "failed"
	}
	return "done"
}

func errMsg(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
