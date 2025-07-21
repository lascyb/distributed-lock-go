package distributedlock

import (
	"context"
	"time"
)

// LockType 锁类型枚举
type LockType int

const (
	// LockTypeExclusive 排他锁 {0:排他锁}
	LockTypeExclusive LockType = iota
	// LockTypeShared 共享锁 {1:共享锁}
	LockTypeShared
)

// LockOptions 锁选项
type LockOptions struct {
	// TTL 锁的生存时间
	TTL time.Duration
	// RetryCount 重试次数
	RetryCount int
	// RetryDelay 重试延迟
	RetryDelay time.Duration
	// LockType 锁类型
	LockType LockType
}

// DefaultLockOptions 默认锁选项
func DefaultLockOptions() *LockOptions {
	return &LockOptions{
		TTL:        30 * time.Second,
		RetryCount: 3,
		RetryDelay: 100 * time.Millisecond,
		LockType:   LockTypeExclusive,
	}
}

// DistributedLock 分布式锁接口
type DistributedLock interface {
	// TryLock 尝试获取锁
	TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error)

	// Lock 获取锁（阻塞）
	Lock(ctx context.Context, key string, opts *LockOptions) error

	// Unlock 释放锁
	Unlock(ctx context.Context, key string) error

	// Renew 续期锁
	Renew(ctx context.Context, key string, ttl time.Duration) error

	// IsLocked 检查锁是否被持有
	IsLocked(ctx context.Context, key string) (bool, error)

	// Close 关闭连接
	Close() error
}
