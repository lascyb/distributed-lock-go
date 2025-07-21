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

// EtcdSharedLock Etcd共享锁实现
type EtcdSharedLock struct {
	client *clientv3.Client
	prefix string
	// 存储锁值和租约ID的映射，用于线程安全的锁值管理
	lockValues sync.Map
	leaseIDs   sync.Map
}

// NewEtcdSharedLock 创建Etcd共享锁实例
func NewEtcdSharedLock(client *clientv3.Client, prefix string) *EtcdSharedLock {
	if prefix == "" {
		prefix = "/shared_locks/"
	}
	return &EtcdSharedLock{
		client: client,
		prefix: prefix,
	}
}

// generateLockValue 生成锁值（用于标识锁的持有者）
func (e *EtcdSharedLock) generateLockValue() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// getLockKey 获取完整的锁键名
func (e *EtcdSharedLock) getLockKey(key string) string {
	return e.prefix + key
}

// getCounterKey 获取共享锁计数器键名
func (e *EtcdSharedLock) getCounterKey(key string) string {
	return e.prefix + key + "/counter"
}

// getHolderKeyPrefix 获取锁持有者键前缀
func (e *EtcdSharedLock) getHolderKeyPrefix(key string) string {
	return e.prefix + key + "/holders/"
}

// getHolderKey 获取特定持有者的键名
func (e *EtcdSharedLock) getHolderKey(key, holder string) string {
	return e.getHolderKeyPrefix(key) + holder
}

// getExclusiveLockKey 获取排他锁键名
func (e *EtcdSharedLock) getExclusiveLockKey(key string) string {
	return "/exclusive_locks/" + key
}

// getLockValue 获取锁值，如果不存在则生成新的
func (e *EtcdSharedLock) getLockValue(key string) string {
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
func (e *EtcdSharedLock) getLeaseID(key string) (clientv3.LeaseID, bool) {
	lockKey := e.getLockKey(key)
	if leaseID, ok := e.leaseIDs.Load(lockKey); ok {
		return leaseID.(clientv3.LeaseID), true
	}
	return 0, false
}

// setLeaseID 设置租约ID
func (e *EtcdSharedLock) setLeaseID(key string, leaseID clientv3.LeaseID) {
	lockKey := e.getLockKey(key)
	e.leaseIDs.Store(lockKey, leaseID)
}

// TryLock 尝试获取共享锁
func (e *EtcdSharedLock) TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error) {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	lockKey := e.getLockKey(key)
	counterKey := e.getCounterKey(key)
	exclusiveKey := e.getExclusiveLockKey(key)
	lockValue := e.getLockValue(key)
	holderKey := e.getHolderKey(key, lockValue)

	// 创建租约
	lease, err := e.client.Grant(ctx, int64(opts.TTL.Seconds()))
	if err != nil {
		return false, fmt.Errorf("etcd grant lease failed: %w", err)
	}

	// 使用事务确保原子性
	txn := e.client.Txn(ctx)

	// 条件：排他锁不存在
	txn.If(clientv3.Compare(clientv3.CreateRevision(exclusiveKey), "=", 0))

	// 如果排他锁不存在，则执行以下操作：
	then := []clientv3.Op{
		// 1. 获取当前计数器值
		clientv3.OpGet(counterKey),
		// 2. 创建或更新锁标记
		clientv3.OpPut(lockKey, "shared", clientv3.WithLease(lease.ID)),
		// 3. 添加持有者
		clientv3.OpPut(holderKey, lockValue, clientv3.WithLease(lease.ID)),
	}

	// 如果排他锁存在，则获取排他锁信息
	els := []clientv3.Op{
		clientv3.OpGet(exclusiveKey),
	}

	resp, err := txn.Then(then...).Else(els...).Commit()
	if err != nil {
		return false, fmt.Errorf("etcd txn failed: %w", err)
	}

	if !resp.Succeeded {
		// 排他锁存在，无法获取共享锁
		return false, nil
	}

	// 获取当前计数器值并递增
	var currentCount int64 = 0
	if len(resp.Responses) > 0 && len(resp.Responses[0].GetResponseRange().Kvs) > 0 {
		countStr := string(resp.Responses[0].GetResponseRange().Kvs[0].Value)
		if count, err := strconv.ParseInt(countStr, 10, 64); err == nil {
			currentCount = count
		}
	}

	newCount := currentCount + 1

	// 更新计数器
	_, err = e.client.Put(ctx, counterKey, strconv.FormatInt(newCount, 10), clientv3.WithLease(lease.ID))
	if err != nil {
		return false, fmt.Errorf("failed to update counter: %w", err)
	}

	// 存储租约ID
	e.setLeaseID(key, lease.ID)

	return true, nil
}

// Lock 获取共享锁（阻塞）
func (e *EtcdSharedLock) Lock(ctx context.Context, key string, opts *LockOptions) error {
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

	return fmt.Errorf("failed to acquire shared lock after %d retries", opts.RetryCount)
}

// Unlock 释放共享锁
func (e *EtcdSharedLock) Unlock(ctx context.Context, key string) error {
	lockKey := e.getLockKey(key)
	counterKey := e.getCounterKey(key)
	lockValue := e.getLockValue(key)
	holderKey := e.getHolderKey(key, lockValue)

	// 检查当前客户端是否持有锁
	resp, err := e.client.Get(ctx, holderKey)
	if err != nil {
		return fmt.Errorf("failed to check lock holder: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return fmt.Errorf("shared lock not held by this client")
	}

	// 获取当前计数器值
	counterResp, err := e.client.Get(ctx, counterKey)
	if err != nil {
		return fmt.Errorf("failed to get counter: %w", err)
	}

	var currentCount int64 = 0
	if len(counterResp.Kvs) > 0 {
		countStr := string(counterResp.Kvs[0].Value)
		if count, err := strconv.ParseInt(countStr, 10, 64); err == nil {
			currentCount = count
		}
	}

	// 使用事务原子性地执行解锁操作
	txn := e.client.Txn(ctx)

	// 条件：持有者键存在
	txn.If(clientv3.Compare(clientv3.CreateRevision(holderKey), ">", 0))

	newCount := currentCount - 1
	var then []clientv3.Op

	// 删除持有者键
	then = append(then, clientv3.OpDelete(holderKey))

	if newCount <= 0 {
		// 如果计数器归零，删除所有相关键
		then = append(then,
			clientv3.OpDelete(lockKey),
			clientv3.OpDelete(counterKey),
		)
		// 删除所有持有者键
		holderPrefix := e.getHolderKeyPrefix(key)
		then = append(then, clientv3.OpDelete(holderPrefix, clientv3.WithPrefix()))
	} else {
		// 更新计数器
		then = append(then, clientv3.OpPut(counterKey, strconv.FormatInt(newCount, 10)))
	}

	res, err := txn.Then(then...).Commit()
	if err != nil {
		return fmt.Errorf("etcd txn failed: %w", err)
	}

	if !res.Succeeded {
		return fmt.Errorf("shared lock not held by this client")
	}

	// 清理本地状态
	e.lockValues.Delete(lockKey)
	e.leaseIDs.Delete(lockKey)

	return nil
}

// Renew 续期共享锁
func (e *EtcdSharedLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	lockValue := e.getLockValue(key)
	holderKey := e.getHolderKey(key, lockValue)

	// 检查当前客户端是否持有锁
	resp, err := e.client.Get(ctx, holderKey)
	if err != nil {
		return fmt.Errorf("failed to check lock holder: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return fmt.Errorf("shared lock not held by this client")
	}

	// 获取租约ID
	leaseID, exists := e.getLeaseID(key)
	if !exists {
		return fmt.Errorf("lease ID not found")
	}

	// 续期租约
	_, err = e.client.KeepAliveOnce(ctx, leaseID)
	if err != nil {
		return fmt.Errorf("failed to renew lease: %w", err)
	}

	return nil
}

// IsLocked 检查共享锁是否被持有
func (e *EtcdSharedLock) IsLocked(ctx context.Context, key string) (bool, error) {
	counterKey := e.getCounterKey(key)

	resp, err := e.client.Get(ctx, counterKey)
	if err != nil {
		return false, fmt.Errorf("etcd get failed: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return false, nil
	}

	countStr := string(resp.Kvs[0].Value)
	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid counter value: %w", err)
	}

	return count > 0, nil
}

// GetLockCount 获取共享锁的持有者数量
func (e *EtcdSharedLock) GetLockCount(ctx context.Context, key string) (int, error) {
	counterKey := e.getCounterKey(key)

	resp, err := e.client.Get(ctx, counterKey)
	if err != nil {
		return 0, fmt.Errorf("etcd get failed: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return 0, nil
	}

	countStr := string(resp.Kvs[0].Value)
	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid counter value: %w", err)
	}

	return int(count), nil
}

// GetLockHolders 获取共享锁的所有持有者
func (e *EtcdSharedLock) GetLockHolders(ctx context.Context, key string) ([]string, error) {
	holderPrefix := e.getHolderKeyPrefix(key)

	resp, err := e.client.Get(ctx, holderPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd get failed: %w", err)
	}

	var holders []string
	for _, kv := range resp.Kvs {
		holders = append(holders, string(kv.Value))
	}

	return holders, nil
}

// Close 关闭连接
func (e *EtcdSharedLock) Close() error {
	// Etcd客户端的关闭由外部管理
	return nil
}
