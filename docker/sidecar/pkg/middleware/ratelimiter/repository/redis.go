package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	rdb *redis.Client
}

func NewRedis(rdb *redis.Client) Repository {
	return &Redis{
		rdb: rdb,
	}
}

func (r *Redis) RemoveOldRecords(ctx context.Context, key string, from, to time.Time) error {
	minStr := strconv.FormatInt(from.UnixNano(), 10)
	maxStr := strconv.FormatInt(to.UnixNano(), 10)

	_, err := r.rdb.ZRemRangeByScore(ctx, key, minStr, maxStr).Result()

	return err
}

func (r *Redis) CountRecords(ctx context.Context, key string) (int64, error) {
	return r.rdb.ZCard(ctx, key).Result()
}

func (r *Redis) AddRecord(ctx context.Context, key string, tm time.Time) error {
	unix := tm.UnixNano()

	_, err := r.rdb.ZAdd(ctx, key, redis.Z{
		Score:  float64(unix),
		Member: strconv.FormatInt(unix, 10),
	}).Result()

	return err
}

func (r *Redis) ExpireKey(ctx context.Context, key string, ttl time.Duration) error {
	_, err := r.rdb.Expire(ctx, key, ttl).Result()

	return err
}
