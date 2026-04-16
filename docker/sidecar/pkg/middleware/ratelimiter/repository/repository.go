package repository

import (
	"context"
	"time"
)

type Repository interface {
	RemoveOldRecords(ctx context.Context, key string, from, to time.Time) error
	CountRecords(ctx context.Context, key string) (int64, error)
	AddRecord(ctx context.Context, key string, tm time.Time) error
	ExpireKey(ctx context.Context, key string, ttl time.Duration) error
}
