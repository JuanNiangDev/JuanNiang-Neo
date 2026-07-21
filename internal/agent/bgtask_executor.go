package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

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
	dao        *dao.BackgroundTaskDAO
	outputChan chan<- DrainerOutput
	mu         sync.Mutex
}

func NewBackgroundTaskExecutor(tools *tool.ToolRegistry, dao *dao.BackgroundTaskDAO, outputChan chan<- DrainerOutput) *BackgroundTaskExecutor {
	return &BackgroundTaskExecutor{
		tools:      tools,
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
		ChatAreaID: chatAreaID,
		Status:     models.TaskStatusPending,
		Steps:      stepsMap,
		Results:    models.JSONMap{},
	}
	if err := b.dao.Create(ctx, task); err != nil {
		return "", err
	}

	go b.executeAsync(task, msgType, targetID, userPrompt, steps)
	return task.ID, nil
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
	var mu sync.Mutex

	slog.Info("后台任务执行开始", "task_id", task.ID, "steps", len(steps))

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
				result, err := b.tools.Execute(gCtx, step.ToolName, step.Args)
				mu.Lock()
				defer mu.Unlock()

				if err != nil {
					results[step.ID] = "error: " + err.Error()
				} else {
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
	b.dao.Update(ctx, &models.BackgroundTask{
		ID:      task.ID,
		Status:  models.TaskStatusDone,
		Results: resultsMap,
	})

	b.outputChan <- DrainerOutput{
		TaskID:      task.ID,
		ChatAreaID:  task.ChatAreaID,
		MessageType: msgType,
		TargetID:    targetID,
		Status:      "done",
		Result:      "所有步骤已完成",
		UserPrompt:  userPrompt,
	}

	slog.Info("后台任务执行完成", "task_id", task.ID)
}

func (b *BackgroundTaskExecutor) Run(ctx context.Context) {
	slog.Info("BackgroundTaskExecutor 已启动")
	<-ctx.Done()
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
