package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"JuanNiang-Neo/internal/core/dao"
)

// TestIsDeadlockErr 结构化 SQLSTATE 判定：40P01/40001 命中，
// 普通错误（含文本含 deadlock 字样）不命中，避免误触发事务重试。
func TestIsDeadlockErr(t *testing.T) {
	deadlocks := []*pgconn.PgError{
		{Code: "40P01", Message: "deadlock detected"},
		{Code: "40001", Message: "could not serialize access due to concurrent update"},
	}
	for _, pe := range deadlocks {
		if !isDeadlockErr(pe) {
			t.Errorf("SQLSTATE %s 应判定为可重试错误", pe.Code)
		}
	}
	nonDeadlocks := []error{
		&pgconn.PgError{Code: "23505", Message: "duplicate key"},
		errors.New("deadlock detected in the text"),
		errors.New("connection reset by peer"),
		nil,
	}
	for _, err := range nonDeadlocks {
		if isDeadlockErr(err) {
			t.Errorf("错误不应判定为死锁: %v", err)
		}
	}
}

// newTxTestDB 构造 sqlite 内存库 + Service（仅 providerTx 使用 DAO.DB）。
func newTxTestDB(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return &Service{DAO: dao.NewBundle(db)}
}

// TestProviderTxCtxCancel ctx 取消后不应继续发起新事务
// （gorm 可能在事务开始前即拦截，fn 调用 0 或 1 次均可，但不得重试）。
func TestProviderTxCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	s := newTxTestDB(t)
	err := s.providerTx(ctx, func(tx *gorm.DB) error {
		calls++
		return &pgconn.PgError{Code: "40P01"}
	})
	if err == nil {
		t.Fatal("ctx 已取消应返回错误")
	}
	if calls > 1 {
		t.Fatalf("ctx 取消后不应重试, calls=%d", calls)
	}
}

// TestProviderTxRetryBudget 死锁重试上限 3 次并带退避，不无限重试。
func TestProviderTxRetryBudget(t *testing.T) {
	start := time.Now()
	calls := 0
	s := newTxTestDB(t)
	err := s.providerTx(context.Background(), func(tx *gorm.DB) error {
		calls++
		return &pgconn.PgError{Code: "40P01"}
	})
	if err == nil {
		t.Fatal("应返回错误")
	}
	if calls != 3 {
		t.Fatalf("重试次数应为 3, got %d", calls)
	}
	if time.Since(start) < 100*time.Millisecond {
		t.Fatalf("重试间应有退避间隔, elapsed=%v", time.Since(start))
	}
}

// TestProviderTxNonDeadlockNoRetry 非死锁错误立即返回，不重试。
func TestProviderTxNonDeadlockNoRetry(t *testing.T) {
	calls := 0
	s := newTxTestDB(t)
	err := s.providerTx(context.Background(), func(tx *gorm.DB) error {
		calls++
		return errors.New("connection reset by peer")
	})
	if err == nil || calls != 1 {
		t.Fatalf("非死锁错误不应重试, calls=%d err=%v", calls, err)
	}
}
