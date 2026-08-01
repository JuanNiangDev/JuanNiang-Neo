package logging

import (
	"context"
	"io"
)

// WriterHandler 是一个 io.Writer 适配器，用于兼容需要 slog.Handler 的旧代码。
// 迁移完成后可移除。
type WriterHandler struct {
	w io.Writer
}

// Write 实现 io.Writer，逐行输出到 w。
func (h *WriterHandler) Write(p []byte) (n int, err error) {
	return h.w.Write(p)
}

// LogToWriter 是一个辅助 builder，返回一个用于 slog.TextHandler 兼容的 WriterHandler。
// 仅用于迁移过渡。
func LogToWriter(w io.Writer) *WriterHandler {
	return &WriterHandler{w: w}
}

// 保留 handler.go 中与 slog 兼容的最小接口。
// 旧 Handler 类型现在不实现 slog.Handler，仅作文档保留。
type _HandlerDoc struct {
	_ [0]byte // 占位
}

// 确保 context 被使用（避免未使用导入错误）
var _ = context.Background
