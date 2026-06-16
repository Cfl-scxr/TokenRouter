package service

import (
	"context"
	"time"
)

type SchedulerOutboxEvent struct {
	ID        int64
	EventType string
	AccountID *int64
	GroupID   *int64
	Payload   map[string]any
	CreatedAt time.Time
}

// SchedulerOutboxRepository 提供调度 outbox 的读取接口。
type SchedulerOutboxRepository interface {
	ListAfter(ctx context.Context, afterID int64, limit int) ([]SchedulerOutboxEvent, error)
	MaxID(ctx context.Context) (int64, error)
	// MarkProcessed 清理已处理事件的去重键，允许后续同类型事件重新入队。
	MarkProcessed(ctx context.Context, eventIDs []int64) error
}
