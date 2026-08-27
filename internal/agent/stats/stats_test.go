package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestEmitWrite 基础写盘：Emit 事件后 Close，文件应为合法 NDJSON（每行一个 JSON）。
func TestEmitWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chat-events.log")
	w := New(Config{Path: path, QueueSize: 64})
	w.Start()

	w.Emit(Event{Timestamp: time.Now(), GroupID: 100, UserID: 200, MessageID: 1, Direction: DirectionMsg, Text: "早上好"})
	w.Emit(Event{Timestamp: time.Now(), GroupID: 100, UserID: 200, MessageID: 1, Direction: DirectionReply, Source: SourceGroupMgr, Text: "你好呀～", ReplyTo: "早上好"})
	w.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("应有 2 行事件，got %d: %q", len(lines), data)
	}
	var ev Event
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatalf("第 1 行非合法 JSON: %v", err)
	}
	if ev.GroupID != 100 || ev.Direction != DirectionMsg || ev.Text != "早上好" || ev.Source != "" {
		t.Fatalf("第 1 行字段不符: %+v", ev)
	}
	if err := json.Unmarshal([]byte(lines[1]), &ev); err != nil {
		t.Fatalf("第 2 行非合法 JSON: %v", err)
	}
	if ev.Direction != DirectionReply || ev.Source != SourceGroupMgr || ev.ReplyTo != "早上好" {
		t.Fatalf("第 2 行字段不符: %+v", ev)
	}
}

// TestEmitNonBlocking 队列满时不阻塞（丢弃并计数），保证消息主循环不受影响。
func TestEmitNonBlocking(t *testing.T) {
	w := New(Config{QueueSize: 2, Path: filepath.Join(t.TempDir(), "x.log")})
	// 不 Start：无消费者，队列立即满
	for i := 0; i < 10; i++ {
		w.Emit(Event{GroupID: int64(i), Direction: DirectionMsg})
	}
	if got := w.Dropped(); got < 8 {
		t.Fatalf("队列满应丢弃至少 8 条，got %d", got)
	}
	w.Close()
}

// TestCloseIdempotent Close 幂等：重复调用不 panic。
func TestCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	w := New(Config{Path: filepath.Join(dir, "x.log")})
	w.Start()
	w.Close()
	w.Close()
}

// TestDropCounterConcurrent 并发 Emit 安全（-race 下验证无数据竞争）。
func TestDropCounterConcurrent(t *testing.T) {
	w := New(Config{QueueSize: 8})
	var done atomic.Int64
	for i := 0; i < 4; i++ {
		go func() {
			defer done.Add(1)
			for j := 0; j < 50; j++ {
				w.Emit(Event{GroupID: 1, Direction: DirectionMsg})
			}
		}()
	}
	// 等待发射完成
	for done.Load() < 4 {
		time.Sleep(5 * time.Millisecond)
	}
	w.Close()
	_ = w.Dropped()
}

// TestNilSafe nil 接收者调用不 panic（HagoCenter.Stats 未配置时为 nil）。
func TestNilSafe(t *testing.T) {
	var w *Writer
	w.Emit(Event{})
	w.Close()
	if w.Dropped() != 0 {
		t.Fatal("nil 接收者 Dropped 应为 0")
	}
}
