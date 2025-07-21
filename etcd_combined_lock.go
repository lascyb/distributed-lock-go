package distributedlock

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdCombinedLock Etcd组合锁，根据锁类型选择相应的实现
type EtcdCombinedLock struct {
	exclusiveLock *EtcdExclusiveLock
	sharedLock    *EtcdSharedLock
}

// NewEtcdCombinedLock 创建Etcd组合锁实例
func NewEtcdCombinedLock(client *clientv3.Client, prefix string) *EtcdCombinedLock {
	return &EtcdCombinedLock{
		exclusiveLock: NewEtcdExclusiveLock(client, prefix+"/exclusive_locks/"),
		sharedLock:    NewEtcdSharedLock(client, prefix+"/shared_locks/"),
	}
}

// TryLock 尝试获取锁（根据锁类型选择实现）
func (e *EtcdCombinedLock) TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error) {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	switch opts.LockType {
	case LockTypeExclusive:
		return e.exclusiveLock.TryLock(ctx, key, opts)
	case LockTypeShared:
		return e.sharedLock.TryLock(ctx, key, opts)
	default:
		return false, fmt.Errorf("unsupported lock type: %d", opts.LockType)
	}
}

// Lock 获取锁（阻塞，根据锁类型选择实现）
func (e *EtcdCombinedLock) Lock(ctx context.Context, key string, opts *LockOptions) error {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	switch opts.LockType {
	case LockTypeExclusive:
		return e.exclusiveLock.Lock(ctx, key, opts)
	case LockTypeShared:
		return e.sharedLock.Lock(ctx, key, opts)
	default:
		return fmt.Errorf("unsupported lock type: %d", opts.LockType)
	}
}

// Unlock 释放锁（需要根据锁类型选择实现）
func (e *EtcdCombinedLock) Unlock(ctx context.Context, key string) error {
	// 首先尝试释放排他锁
	err := e.exclusiveLock.Unlock(ctx, key)
	if err == nil {
		return nil
	}

	// 如果排他锁释放失败，尝试释放共享锁
	err = e.sharedLock.Unlock(ctx, key)
	if err == nil {
		return nil
	}

	return fmt.Errorf("lock not held by this client")
}

// Renew 续期锁（需要根据锁类型选择实现）
func (e *EtcdCombinedLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	// 首先尝试续期排他锁
	err := e.exclusiveLock.Renew(ctx, key, ttl)
	if err == nil {
		return nil
	}

	// 如果排他锁续期失败，尝试续期共享锁
	err = e.sharedLock.Renew(ctx, key, ttl)
	if err == nil {
		return nil
	}

	return fmt.Errorf("lock not held by this client")
}

// IsLocked 检查锁是否被持有（检查两种锁类型）
func (e *EtcdCombinedLock) IsLocked(ctx context.Context, key string) (bool, error) {
	// 检查排他锁
	exclusiveLocked, err := e.exclusiveLock.IsLocked(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check exclusive lock: %w", err)
	}
	if exclusiveLocked {
		return true, nil
	}

	// 检查共享锁
	sharedLocked, err := e.sharedLock.IsLocked(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check shared lock: %w", err)
	}

	return sharedLocked, nil
}

// GetLockInfo 获取锁的详细信息
func (e *EtcdCombinedLock) GetLockInfo(ctx context.Context, key string) (map[string]interface{}, error) {
	info := make(map[string]interface{})

	// 检查排他锁
	exclusiveLocked, err := e.exclusiveLock.IsLocked(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to check exclusive lock: %w", err)
	}

	if exclusiveLocked {
		info["lock_type"] = "exclusive"
		info["is_locked"] = true
		return info, nil
	}

	// 检查共享锁
	sharedLocked, err := e.sharedLock.IsLocked(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to check shared lock: %w", err)
	}

	if sharedLocked {
		info["lock_type"] = "shared"
		info["is_locked"] = true

		// 获取共享锁的持有者数量
		count, err := e.sharedLock.GetLockCount(ctx, key)
		if err == nil {
			info["holder_count"] = count
		}

		// 获取共享锁的持有者列表
		holders, err := e.sharedLock.GetLockHolders(ctx, key)
		if err == nil {
			info["holders"] = holders
		}

		return info, nil
	}

	info["is_locked"] = false
	return info, nil
}

// Close 关闭连接
func (e *EtcdCombinedLock) Close() error {
	// 连接由外部管理，这里不需要关闭
	return nil
}
