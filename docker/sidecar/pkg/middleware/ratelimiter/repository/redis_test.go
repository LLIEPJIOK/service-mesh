package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LLIEPJIOK/sidecar/pkg/middleware/ratelimiter/repository"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func setupRedisContainer(t *testing.T) (rdb *redis.Client, cleanup func()) {
	t.Helper()

	ctx := context.Background()

	cont, err := testredis.Run(ctx, "redis:7.0")
	require.NoError(t, err, "failed to start redis container")

	dsn, err := cont.ConnectionString(ctx)
	require.NoError(t, err, "failed to get connection string")

	addr := strings.TrimPrefix(dsn, "redis://")

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()

	_, err = client.Ping(pingCtx).Result()
	require.NoError(t, err, "failed to ping redis")

	return client, func() {
		err := client.Close()
		assert.NoError(t, err, "failed to close redis client")

		termCtx, termCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer termCancel()

		err = cont.Terminate(termCtx)
		assert.NoError(t, err, "failed to terminate redis container")
	}
}

func TestRedisRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	rdb, cleanup := setupRedisContainer(t)
	defer cleanup()

	repo := repository.NewRedis(rdb)
	ctx := context.Background()
	testKey := "test:ratelimit:zset"

	t.Run("AddAndCountRecords", func(t *testing.T) {
		err := rdb.Del(ctx, testKey).Err()
		require.NoError(t, err)

		now := time.Now()
		times := []time.Time{
			now.Add(-2 * time.Second),
			now.Add(-1 * time.Second),
			now,
		}

		for i, tm := range times {
			err := repo.AddRecord(ctx, testKey, tm)
			require.NoError(t, err, "failed to add record %d", i+1)
		}

		count, err := repo.CountRecords(ctx, testKey)
		require.NoError(t, err, "failed to count records")
		assert.Equal(t, int64(len(times)), count, "record count mismatch")

		members, err := rdb.ZRange(ctx, testKey, 0, -1).Result()
		require.NoError(t, err, "failed to get members")
		assert.Len(t, members, len(times), "member count mismatch")

		// Clean up the test key
		err = rdb.Del(ctx, testKey).Err()
		require.NoError(t, err, "failed to delete test key")
	})

	t.Run("RemoveOldRecords", func(t *testing.T) {
		now := time.Now()
		times := []time.Time{
			now.Add(-3 * time.Second),
			now.Add(-2 * time.Second),
			now.Add(-1 * time.Second),
		}

		for _, tm := range times {
			err := repo.AddRecord(ctx, testKey, tm)
			require.NoError(t, err, "failed to add record %v", tm)
		}

		startWindow := now.Add(-1500 * time.Millisecond)

		err := repo.RemoveOldRecords(ctx, testKey, time.Time{}, startWindow)
		require.NoError(t, err, "failed to remove old records")

		count, err := repo.CountRecords(ctx, testKey)
		require.NoError(t, err, "failed to count records after removal")
		assert.Equal(t, int64(1), count, "should only have 1 record left")

		members, err := rdb.ZRange(ctx, testKey, 0, -1).Result()
		require.NoError(t, err, "failed to get members after removal")
		require.Len(t, members, 1, "should only have 1 member left")

		// Clean up the test key
		err = rdb.Del(ctx, testKey).Err()
		require.NoError(t, err, "failed to delete test key")
	})

	t.Run("ExpireKey", func(t *testing.T) {
		err := repo.AddRecord(ctx, testKey, time.Now())
		require.NoError(t, err, "failed to add record for expiration test")

		ttl := 1 * time.Second
		err = repo.ExpireKey(ctx, testKey, ttl)
		require.NoError(t, err, "failed to set expiration")

		actualTTL, err := rdb.PTTL(ctx, testKey).Result()
		require.NoError(t, err, "failed to get TTL")
		assert.True(
			t,
			actualTTL > 0 && actualTTL <= ttl,
			"TTL %v is not within expected range (0, %v]",
			actualTTL,
			ttl,
		)

		time.Sleep(ttl + 200*time.Millisecond) // Wait a bit longer than TTL

		exists, err := rdb.Exists(ctx, testKey).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists, "key should not exist after expiration")

		err = repo.ExpireKey(ctx, "nonexistent:key", 1*time.Minute)
		assert.NoError(t, err, "expiring a non-existent key should not return an error")

		// Clean up the test key
		err = rdb.Del(ctx, testKey).Err()
		require.NoError(t, err, "failed to delete test key")
	})

	t.Run("OperationsOnNonExistentKey", func(t *testing.T) {
		nonExistentKey := "test:ratelimit:nonexistent"
		err := rdb.Del(ctx, nonExistentKey).Err()
		require.NoError(t, err, "failed to delete non-existent key")

		// Count on non-existent key
		count, err := repo.CountRecords(ctx, nonExistentKey)
		assert.NoError(t, err, "counting records on non-existent key should not return an error")
		assert.Equal(t, int64(0), count, "count should be 0 for non-existent key")

		// Remove on non-existent key
		err = repo.RemoveOldRecords(
			ctx,
			nonExistentKey,
			time.Time{},
			time.Now(),
		)
		assert.NoError(
			t,
			err,
			"removing old records on non-existent key should not return an error",
		)

		// Expire on non-existent key
		err = repo.ExpireKey(ctx, nonExistentKey, 1*time.Minute)
		assert.NoError(t, err, "expiring a non-existent key should not return an error")
	})
}
