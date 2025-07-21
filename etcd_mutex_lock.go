package distributedlock

import (
	"context"
	"fmt"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

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

	mutexValue, ok := em.mutexes.Load(mutexKey)
	if !ok {
		return fmt.Errorf("mutex not found for key: %s", key)
	}

	sessionValue, ok := em.sessions.Load(mutexKey)
	if !ok {
		return fmt.Errorf("session not found for key: %s", key)
	}

	mutex := mutexValue.(*concurrency.Mutex)
	session := sessionValue.(*concurrency.Session)

	err := mutex.Unlock(ctx)
	if err != nil {
		return fmt.Errorf("unlock failed: %w", err)
	}

	// 清理存储的数据
	em.mutexes.Delete(mutexKey)
	em.sessions.Delete(mutexKey)

	// 关闭session
	session.Close()

	return nil
}

// Renew 续期锁
func (em *EtcdMutexLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	// Mutex模式下，session会自动续期，这里不需要手动续期
	return nil
}

// IsLocked 检查锁是否被持有
func (em *EtcdMutexLock) IsLocked(ctx context.Context, key string) (bool, error) {
	mutexKey := em.getMutexKey(key)

	resp, err := em.client.Get(ctx, mutexKey, clientv3.WithPrefix())
	if err != nil {
		return false, fmt.Errorf("etcd get failed: %w", err)
	}

	return len(resp.Kvs) > 0, nil
}

// Close 关闭连接
func (em *EtcdMutexLock) Close() error {
	// 清理所有存储的session
	em.sessions.Range(func(key, value interface{}) bool {
		session := value.(*concurrency.Session)
		session.Close()
		return true
	})

	// 清理映射
	em.mutexes = sync.Map{}
	em.sessions = sync.Map{}

	return em.client.Close()
}
