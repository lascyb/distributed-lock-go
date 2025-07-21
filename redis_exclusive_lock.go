package distributedlock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisExclusiveLock Redis排他锁实现
type RedisExclusiveLock struct {
	client *redis.Client
	prefix string
	// 存储锁值的映射，用于线程安全的锁值管理
	lockValues sync.Map
}

// NewRedisExclusiveLock 创建Redis排他锁实例
func NewRedisExclusiveLock(client *redis.Client, prefix string) *RedisExclusiveLock {
	if prefix == "" {
		prefix = "exclusive:"
	}
	return &RedisExclusiveLock{
		client: client,
		prefix: prefix,
	}
}

// generateLockValue 生成锁值（用于标识锁的持有者）
func (r *RedisExclusiveLock) generateLockValue() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// getLockKey 获取完整的锁键名
func (r *RedisExclusiveLock) getLockKey(key string) string {
	return r.prefix + key
}

// getLockValue 获取锁值，如果不存在则生成新的
func (r *RedisExclusiveLock) getLockValue(key string) string {
	lockKey := r.getLockKey(key)

	// 尝试从映射中获取锁值
	if value, ok := r.lockValues.Load(lockKey); ok {
		return value.(string)
	}

	// 生成新的锁值并存储
	lockValue := r.generateLockValue()
	r.lockValues.Store(lockKey, lockValue)
	return lockValue
}

// TryLock 尝试获取排他锁
func (r *RedisExclusiveLock) TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error) {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	lockKey := r.getLockKey(key)
	lockValue := r.getLockValue(key)

	// 使用Lua脚本检查共享锁并原子性地获取排他锁
	script := `
		-- 检查是否存在共享锁
		local shared_counter = "shared_lock:" .. KEYS[1] .. ":counter"
		if redis.call("exists", shared_counter) == 1 then
			local count = redis.call("get", shared_counter)
			if tonumber(count) > 0 then
				return 0  -- 共享锁存在，无法获取排他锁
			end
		end
		
		-- 尝试获取排他锁
		if redis.call("setnx", KEYS[2], ARGV[1]) == 1 then
			redis.call("expire", KEYS[2], ARGV[2])
			return 1  -- 成功获取排他锁
		else
			return 0  -- 排他锁已被其他客户端持有
		end
	`

	result, err := r.client.Eval(ctx, script, []string{key, lockKey}, lockValue, int(opts.TTL.Seconds())).Result()
	if err != nil {
		return false, fmt.Errorf("redis eval failed: %w", err)
	}

	return result.(int64) == 1, nil
}

// Lock 获取排他锁（阻塞）
func (r *RedisExclusiveLock) Lock(ctx context.Context, key string, opts *LockOptions) error {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	for i := 0; i <= opts.RetryCount; i++ {
		acquired, err := r.TryLock(ctx, key, opts)
		if err != nil {
			return err
		}

		if acquired {
			return nil
		}

		// 如果不是最后一次重试，则等待
		if i < opts.RetryCount {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(opts.RetryDelay):
				continue
			}
		}
	}

	return fmt.Errorf("failed to acquire exclusive lock after %d retries", opts.RetryCount)
}

// Unlock 释放排他锁
func (r *RedisExclusiveLock) Unlock(ctx context.Context, key string) error {
	lockKey := r.getLockKey(key)
	lockValue := r.getLockValue(key)

	// 使用Lua脚本确保原子性解锁
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	result, err := r.client.Eval(ctx, script, []string{lockKey}, lockValue).Result()
	if err != nil {
		return fmt.Errorf("redis eval failed: %w", err)
	}

	if result.(int64) == 0 {
		return fmt.Errorf("exclusive lock not held by this client")
	}

	// 解锁成功后，从映射中删除锁值
	r.lockValues.Delete(lockKey)

	return nil
}

// Renew 续期排他锁
func (r *RedisExclusiveLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	lockKey := r.getLockKey(key)
	lockValue := r.getLockValue(key)

	// 验证TTL参数
	if ttl <= 0 {
		return fmt.Errorf("invalid TTL duration for key %s: %v", key, ttl)
	}

	// 使用Lua脚本确保原子性续期
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("expire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	result, err := r.client.Eval(ctx, script, []string{lockKey}, lockValue, int(ttl.Seconds())).Result()
	if err != nil {
		return fmt.Errorf("redis eval failed for key %s: %w", key, err)
	}

	if result.(int64) == 0 {
		return fmt.Errorf("exclusive lock not held by this client for key: %s", key)
	}

	return nil
}

// IsLocked 检查排他锁是否被持有
func (r *RedisExclusiveLock) IsLocked(ctx context.Context, key string) (bool, error) {
	lockKey := r.getLockKey(key)

	exists, err := r.client.Exists(ctx, lockKey).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists failed: %w", err)
	}

	return exists > 0, nil
}

// Close 关闭连接
func (r *RedisExclusiveLock) Close() error {
	// Redis客户端的关闭由外部管理
	return nil
}
