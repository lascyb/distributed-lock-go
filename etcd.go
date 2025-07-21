package distributedlock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// EtcdLock Etcd分布式锁实现
type EtcdLock struct {
	client *clientv3.Client
	prefix string
	// 存储锁值和租约ID的映射，用于线程安全的锁值管理
	lockValues sync.Map
	leaseIDs   sync.Map
}

// NewEtcdLock 创建Etcd分布式锁实例
func NewEtcdLock(client *clientv3.Client, prefix string) *EtcdLock {
	if prefix == "" {
		prefix = "/locks/"
	}
	return &EtcdLock{
		client: client,
		prefix: prefix,
	}
}

// generateLockValue 生成锁值（用于标识锁的持有者）
func (e *EtcdLock) generateLockValue() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// getLockKey 获取完整的锁键名
func (e *EtcdLock) getLockKey(key string) string {
	return e.prefix + key
}

// getLockValue 获取锁值，如果不存在则生成新的
func (e *EtcdLock) getLockValue(key string) string {
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
func (e *EtcdLock) getLeaseID(key string) (clientv3.LeaseID, bool) {
	lockKey := e.getLockKey(key)
	if leaseID, ok := e.leaseIDs.Load(lockKey); ok {
		return leaseID.(clientv3.LeaseID), true
	}
	return 0, false
}

// setLeaseID 设置租约ID
func (e *EtcdLock) setLeaseID(key string, leaseID clientv3.LeaseID) {
	lockKey := e.getLockKey(key)
	e.leaseIDs.Store(lockKey, leaseID)
}

// TryLock 尝试获取锁
func (e *EtcdLock) TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error) {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	lockKey := e.getLockKey(key)
	lockValue := e.getLockValue(key)

	// 使用Etcd的租约机制实现TTL
	lease, err := e.client.Grant(ctx, int64(opts.TTL.Seconds()))
	if err != nil {
		return false, fmt.Errorf("etcd grant lease failed: %w", err)
	}

	// 尝试创建键值对，如果键不存在则创建成功
	txn := e.client.Txn(ctx)
	txn.If(clientv3.Compare(clientv3.CreateRevision(lockKey), "=", 0)).
		Then(clientv3.OpPut(lockKey, lockValue, clientv3.WithLease(lease.ID))).
		Else(clientv3.OpGet(lockKey))

	resp, err := txn.Commit()
	if err != nil {
		return false, fmt.Errorf("etcd txn failed: %w", err)
	}

	if resp.Succeeded {
		// 存储租约ID用于后续续期
		e.setLeaseID(key, lease.ID)
		return true, nil
	}

	return false, nil
}

// Lock 获取锁（阻塞）
func (e *EtcdLock) Lock(ctx context.Context, key string, opts *LockOptions) error {
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

	return fmt.Errorf("failed to acquire lock after %d retries", opts.RetryCount)
}

// Unlock 释放锁
func (e *EtcdLock) Unlock(ctx context.Context, key string) error {
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
		return fmt.Errorf("lock not held by this client")
	}

	// 解锁成功后，清理存储的数据
	e.lockValues.Delete(lockKey)
	e.leaseIDs.Delete(lockKey)

	return nil
}

// Renew 续期锁
func (e *EtcdLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
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

// IsLocked 检查锁是否被持有
func (e *EtcdLock) IsLocked(ctx context.Context, key string) (bool, error) {
	lockKey := e.getLockKey(key)

	resp, err := e.client.Get(ctx, lockKey)
	if err != nil {
		return false, fmt.Errorf("etcd get failed: %w", err)
	}

	return len(resp.Kvs) > 0, nil
}

// Close 关闭连接
func (e *EtcdLock) Close() error {
	return e.client.Close()
}

// EtcdMutexLock 使用Etcd的Mutex实现的分布式锁（更简单的实现）
type EtcdMutexLock struct {
	client *clientv3.Client
	prefix string
	// 存储mutex和session的映射
	mutexes  sync.Map
	sessions sync.Map
}

// NewEtcdMutexLock 创建基于Mutex的Etcd分布式锁实例
func NewEtcdMutexLock(client *clientv3.Client, prefix string) *EtcdMutexLock {
	if prefix == "" {
		prefix = "/mutex/"
	}
	return &EtcdMutexLock{
		client: client,
		prefix: prefix,
	}
}

// getMutexKey 获取mutex的键
func (em *EtcdMutexLock) getMutexKey(key string) string {
	return em.prefix + key
}

// TryLock 尝试获取锁
func (em *EtcdMutexLock) TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error) {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	mutexKey := em.getMutexKey(key)

	session, err := concurrency.NewSession(em.client, concurrency.WithTTL(int(opts.TTL.Seconds())))
	if err != nil {
		return false, fmt.Errorf("create session failed: %w", err)
	}

	mutex := concurrency.NewMutex(session, mutexKey)

	err = mutex.TryLock(ctx)
	if err != nil {
		session.Close()
		return false, fmt.Errorf("try lock failed: %w", err)
	}

	// 存储mutex和session
	em.mutexes.Store(mutexKey, mutex)
	em.sessions.Store(mutexKey, session)

	return true, nil
}

// Lock 获取锁（阻塞）
func (em *EtcdMutexLock) Lock(ctx context.Context, key string, opts *LockOptions) error {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	mutexKey := em.getMutexKey(key)

	session, err := concurrency.NewSession(em.client, concurrency.WithTTL(int(opts.TTL.Seconds())))
	if err != nil {
		return fmt.Errorf("create session failed: %w", err)
	}

	mutex := concurrency.NewMutex(session, mutexKey)

	err = mutex.Lock(ctx)
	if err != nil {
		session.Close()
		return fmt.Errorf("lock failed: %w", err)
	}

	// 存储mutex和session
	em.mutexes.Store(mutexKey, mutex)
	em.sessions.Store(mutexKey, session)

	return nil
}

// Unlock 释放锁
func (em *EtcdMutexLock) Unlock(ctx context.Context, key string) error {
	mutexKey := em.getMutexKey(key)

	mutexValue, mutexExists := em.mutexes.Load(mutexKey)
	sessionValue, sessionExists := em.sessions.Load(mutexKey)

	if !mutexExists || !sessionExists {
		return fmt.Errorf("mutex or session not found for key: %s", key)
	}

	mutex := mutexValue.(*concurrency.Mutex)
	session := sessionValue.(*concurrency.Session)

	err := mutex.Unlock(ctx)
	if err != nil {
		return fmt.Errorf("unlock failed: %w", err)
	}

	session.Close()

	// 清理存储的数据
	em.mutexes.Delete(mutexKey)
	em.sessions.Delete(mutexKey)

	return nil
}

// Renew 续期锁（对于Mutex实现，通过续期session来实现）
func (em *EtcdMutexLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	mutexKey := em.getMutexKey(key)

	sessionValue, exists := em.sessions.Load(mutexKey)
	if !exists {
		return fmt.Errorf("session not found for key: %s", key)
	}

	session, ok := sessionValue.(*concurrency.Session)
	if !ok {
		return fmt.Errorf("invalid session type for key: %s", key)
	}

	// 检查session是否已关闭
	if session == nil {
		return fmt.Errorf("session is nil for key: %s", key)
	}

	// 对于Mutex实现，续期通过session的TTL机制自动处理
	// 但是我们可以通过重新创建session来延长TTL
	// 这里我们使用KeepAliveOnce来续期session的租约
	leaseID := session.Lease()
	if leaseID == 0 {
		return fmt.Errorf("invalid lease ID for session, key: %s", key)
	}

	// 续期租约
	_, err := em.client.KeepAliveOnce(ctx, leaseID)
	if err != nil {
		return fmt.Errorf("etcd keep alive failed for key %s: %w", key, err)
	}

	return nil
}

// IsLocked 检查锁是否被持有
func (em *EtcdMutexLock) IsLocked(ctx context.Context, key string) (bool, error) {
	resp, err := em.client.Get(ctx, em.prefix+key)
	if err != nil {
		return false, fmt.Errorf("etcd get failed: %w", err)
	}

	return len(resp.Kvs) > 0, nil
}

// Close 关闭连接
func (em *EtcdMutexLock) Close() error {
	return em.client.Close()
}
