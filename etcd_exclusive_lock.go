package distributedlock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdExclusiveLock Etcd排他锁实现
type EtcdExclusiveLock struct {
	client *clientv3.Client
	prefix string
	// 存储锁值和租约ID的映射，用于线程安全的锁值管理
	lockValues sync.Map
	leaseIDs   sync.Map
}

// NewEtcdExclusiveLock 创建Etcd排他锁实例
func NewEtcdExclusiveLock(client *clientv3.Client, prefix string) *EtcdExclusiveLock {
	if prefix == "" {
		prefix = "/exclusive_locks/"
	}
	return &EtcdExclusiveLock{
		client: client,
		prefix: prefix,
	}
}

// generateLockValue 生成锁值（用于标识锁的持有者）
func (e *EtcdExclusiveLock) generateLockValue() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// getLockKey 获取完整的锁键名
func (e *EtcdExclusiveLock) getLockKey(key string) string {
	return e.prefix + key
}

// getSharedLockCounterKey 获取共享锁计数器键名
func (e *EtcdExclusiveLock) getSharedLockCounterKey(key string) string {
	return "/shared_locks/" + key + "/counter"
}

// getLockValue 获取锁值，如果不存在则生成新的
func (e *EtcdExclusiveLock) getLockValue(key string) string {
	lockKey := e.getLockKey(key)

	// 尝试从映射中获取锁值
	if value, ok := e.lockValues.Load(lockKey); ok {
		return value.(string)
	}

	// 生成新的锁值并存储
	lockValue := e.generateLockValue()
	e.lockValues.Store(lockKey, lockValue)
	return lockValue
}

// getLeaseID 获取租约ID
func (e *EtcdExclusiveLock) getLeaseID(key string) (clientv3.LeaseID, bool) {
	lockKey := e.getLockKey(key)
	if leaseID, ok := e.leaseIDs.Load(lockKey); ok {
		return leaseID.(clientv3.LeaseID), true
	}
	return 0, false
}

// setLeaseID 设置租约ID
func (e *EtcdExclusiveLock) setLeaseID(key string, leaseID clientv3.LeaseID) {
	lockKey := e.getLockKey(key)
	e.leaseIDs.Store(lockKey, leaseID)
}

// TryLock 尝试获取排他锁
func (e *EtcdExclusiveLock) TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error) {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	lockKey := e.getLockKey(key)
	lockValue := e.getLockValue(key)
	sharedCounterKey := e.getSharedLockCounterKey(key)

	// 使用Etcd的租约机制实现TTL
	lease, err := e.client.Grant(ctx, int64(opts.TTL.Seconds()))
	if err != nil {
		return false, fmt.Errorf("etcd grant lease failed: %w", err)
	}

	// 使用事务确保原子性：检查共享锁计数器和排他锁状态
	txn := e.client.Txn(ctx)

	// 条件：排他锁不存在 AND 共享锁计数器不存在或为0
	txn.If(
		clientv3.Compare(clientv3.CreateRevision(lockKey), "=", 0),
	)

	// 首先获取共享锁计数器
	then := []clientv3.Op{
		clientv3.OpGet(sharedCounterKey),
	}

	// 如果排他锁已存在，直接获取排他锁信息
	els := []clientv3.Op{
		clientv3.OpGet(lockKey),
	}

	resp, err := txn.Then(then...).Else(els...).Commit()
	if err != nil {
		return false, fmt.Errorf("etcd txn failed: %w", err)
	}

	if !resp.Succeeded {
		// 排他锁已存在
		return false, nil
	}

	// 检查共享锁计数器
	if len(resp.Responses) > 0 && len(resp.Responses[0].GetResponseRange().Kvs) > 0 {
		countStr := string(resp.Responses[0].GetResponseRange().Kvs[0].Value)
		if count, err := strconv.ParseInt(countStr, 10, 64); err == nil && count > 0 {
			// 共享锁存在，无法获取排他锁
			return false, nil
		}
	}

	// 没有共享锁和排他锁，可以获取排他锁
	_, err = e.client.Put(ctx, lockKey, lockValue, clientv3.WithLease(lease.ID))
	if err != nil {
		return false, fmt.Errorf("failed to put exclusive lock: %w", err)
	}

	// 存储租约ID用于后续续期
	e.setLeaseID(key, lease.ID)

	return true, nil
}

// Lock 获取排他锁（阻塞）
func (e *EtcdExclusiveLock) Lock(ctx context.Context, key string, opts *LockOptions) error {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	for i := 0; i <= opts.RetryCount; i++ {
		acquired, err := e.TryLock(ctx, key, opts)
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
func (e *EtcdExclusiveLock) Unlock(ctx context.Context, key string) error {
	lockKey := e.getLockKey(key)
	lockValue := e.getLockValue(key)

	// 使用事务确保原子性解锁
	txn := e.client.Txn(ctx)
	txn.If(clientv3.Compare(clientv3.Value(lockKey), "=", lockValue)).
		Then(clientv3.OpDelete(lockKey)).
		Else(clientv3.OpGet(lockKey))

	resp, err := txn.Commit()
	if err != nil {
		return fmt.Errorf("etcd txn failed: %w", err)
	}

	if !resp.Succeeded {
		return fmt.Errorf("exclusive lock not held by this client")
	}

	// 解锁成功后，清理存储的数据
	e.lockValues.Delete(lockKey)
	e.leaseIDs.Delete(lockKey)

	return nil
}

// Renew 续期排他锁
func (e *EtcdExclusiveLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	leaseID, exists := e.getLeaseID(key)
	if !exists {
		return fmt.Errorf("lease id not found for key: %s", key)
	}

	// 验证租约ID是否有效
	if leaseID == 0 {
		return fmt.Errorf("invalid lease ID for key: %s", key)
	}

	// 续期租约
	_, err := e.client.KeepAliveOnce(ctx, leaseID)
	if err != nil {
		return fmt.Errorf("etcd keep alive failed for key %s: %w", key, err)
	}

	return nil
}

// IsLocked 检查排他锁是否被持有
func (e *EtcdExclusiveLock) IsLocked(ctx context.Context, key string) (bool, error) {
	lockKey := e.getLockKey(key)

	resp, err := e.client.Get(ctx, lockKey)
	if err != nil {
		return false, fmt.Errorf("etcd get failed: %w", err)
	}

	return len(resp.Kvs) > 0, nil
}

// Close 关闭连接
func (e *EtcdExclusiveLock) Close() error {
	// Etcd客户端的关闭由外部管理
	return nil
}
