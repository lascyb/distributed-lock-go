package distributedlock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSharedLock Redis共享锁实现
type RedisSharedLock struct {
	client *redis.Client
	prefix string
	// 存储锁值的映射，用于线程安全的锁值管理
	lockValues sync.Map
}

// NewRedisSharedLock 创建Redis共享锁实例
func NewRedisSharedLock(client *redis.Client, prefix string) *RedisSharedLock {
	if prefix == "" {
		prefix = "shared_lock:"
	}
	return &RedisSharedLock{
		client: client,
		prefix: prefix,
	}
}

// generateLockValue 生成锁值（用于标识锁的持有者）
func (r *RedisSharedLock) generateLockValue() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// getLockKey 获取完整的锁键名
func (r *RedisSharedLock) getLockKey(key string) string {
	return r.prefix + key
}

// getCounterKey 获取共享锁计数器键名
func (r *RedisSharedLock) getCounterKey(key string) string {
	return r.prefix + key + ":counter"
}

// getHolderKey 获取锁持有者集合键名
func (r *RedisSharedLock) getHolderKey(key string) string {
	return r.prefix + key + ":holders"
}

// getLockValue 获取锁值，如果不存在则生成新的
func (r *RedisSharedLock) getLockValue(key string) string {
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

// TryLock 尝试获取共享锁
func (r *RedisSharedLock) TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error) {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	lockKey := r.getLockKey(key)
	counterKey := r.getCounterKey(key)
	holderKey := r.getHolderKey(key)
	lockValue := r.getLockValue(key)
	ttlSeconds := int64(opts.TTL.Seconds())

	// 使用Lua脚本实现原子性的共享锁获取
	script := `
		-- 检查是否存在排他锁（以"exclusive:"开头的键）
		local exclusive_key = "exclusive:" .. KEYS[1]
		if redis.call("exists", exclusive_key) == 1 then
			return 0  -- 排他锁存在，无法获取共享锁
		end
		
		-- 增加共享锁计数器
		local count = redis.call("incr", KEYS[2])
		
		-- 将当前锁持有者添加到集合中
		redis.call("sadd", KEYS[3], ARGV[1])
		
		-- 设置TTL
		redis.call("expire", KEYS[1], ARGV[2])
		redis.call("expire", KEYS[2], ARGV[2])
		redis.call("expire", KEYS[3], ARGV[2])
		
		return 1  -- 成功获取共享锁
	`

	result, err := r.client.Eval(ctx, script, []string{lockKey, counterKey, holderKey}, lockValue, ttlSeconds).Result()
	if err != nil {
		return false, fmt.Errorf("redis eval failed: %w", err)
	}

	return result.(int64) == 1, nil
}

// Lock 获取共享锁（阻塞）
func (r *RedisSharedLock) Lock(ctx context.Context, key string, opts *LockOptions) error {
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

	return fmt.Errorf("failed to acquire shared lock after %d retries", opts.RetryCount)
}

// Unlock 释放共享锁
func (r *RedisSharedLock) Unlock(ctx context.Context, key string) error {
	lockKey := r.getLockKey(key)
	counterKey := r.getCounterKey(key)
	holderKey := r.getHolderKey(key)
	lockValue := r.getLockValue(key)

	// 使用Lua脚本确保原子性解锁
	script := `
		-- 检查当前客户端是否持有锁
		if redis.call("sismember", KEYS[3], ARGV[1]) == 0 then
			return 0  -- 当前客户端未持有锁
		end
		
		-- 从持有者集合中移除当前客户端
		redis.call("srem", KEYS[3], ARGV[1])
		
		-- 减少计数器
		local count = redis.call("decr", KEYS[2])
		
		-- 如果计数器为0，删除所有相关键
		if count <= 0 then
			redis.call("del", KEYS[1])
			redis.call("del", KEYS[2])
			redis.call("del", KEYS[3])
		end
		
		return 1  -- 成功释放锁
	`

	result, err := r.client.Eval(ctx, script, []string{lockKey, counterKey, holderKey}, lockValue).Result()
	if err != nil {
		return fmt.Errorf("redis eval failed: %w", err)
	}

	if result.(int64) == 0 {
		return fmt.Errorf("shared lock not held by this client")
	}

	// 解锁成功后，从映射中删除锁值
	r.lockValues.Delete(lockKey)

	return nil
}

// Renew 续期共享锁
func (r *RedisSharedLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	lockKey := r.getLockKey(key)
	counterKey := r.getCounterKey(key)
	holderKey := r.getHolderKey(key)
	lockValue := r.getLockValue(key)
	ttlSeconds := int64(ttl.Seconds())

	// 使用Lua脚本确保原子性续期
	script := `
		-- 检查当前客户端是否持有锁
		if redis.call("sismember", KEYS[3], ARGV[1]) == 0 then
			return 0  -- 当前客户端未持有锁
		end
		
		-- 续期所有相关键
		redis.call("expire", KEYS[1], ARGV[2])
		redis.call("expire", KEYS[2], ARGV[2])
		redis.call("expire", KEYS[3], ARGV[2])
		
		return 1  -- 成功续期
	`

	result, err := r.client.Eval(ctx, script, []string{lockKey, counterKey, holderKey}, lockValue, ttlSeconds).Result()
	if err != nil {
		return fmt.Errorf("redis eval failed: %w", err)
	}

	if result.(int64) == 0 {
		return fmt.Errorf("shared lock not held by this client")
	}

	return nil
}

// IsLocked 检查共享锁是否被持有
func (r *RedisSharedLock) IsLocked(ctx context.Context, key string) (bool, error) {
	counterKey := r.getCounterKey(key)

	count, err := r.client.Get(ctx, counterKey).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, fmt.Errorf("redis get failed: %w", err)
	}

	countValue, err := strconv.Atoi(count)
	if err != nil {
		return false, fmt.Errorf("invalid counter value: %w", err)
	}

	return countValue > 0, nil
}

// GetLockCount 获取共享锁的持有者数量
func (r *RedisSharedLock) GetLockCount(ctx context.Context, key string) (int, error) {
	counterKey := r.getCounterKey(key)

	count, err := r.client.Get(ctx, counterKey).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, fmt.Errorf("redis get failed: %w", err)
	}

	countValue, err := strconv.Atoi(count)
	if err != nil {
		return 0, fmt.Errorf("invalid counter value: %w", err)
	}

	return countValue, nil
}

// GetLockHolders 获取共享锁的所有持有者
func (r *RedisSharedLock) GetLockHolders(ctx context.Context, key string) ([]string, error) {
	holderKey := r.getHolderKey(key)

	holders, err := r.client.SMembers(ctx, holderKey).Result()
	if err != nil {
		return nil, fmt.Errorf("redis smembers failed: %w", err)
	}

	return holders, nil
}

// Close 关闭连接
func (r *RedisSharedLock) Close() error {
	// Redis客户端的关闭由外部管理
	return nil
}
