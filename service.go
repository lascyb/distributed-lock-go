package distributedlock

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LockService 分布式锁服务
type LockService struct {
	lock    DistributedLock
	mu      sync.RWMutex
	options *LockOptions
}

// NewLockService 创建分布式锁服务
func NewLockService(lock DistributedLock, options *LockOptions) *LockService {
	if options == nil {
		options = DefaultLockOptions()
	}
	return &LockService{
		lock:    lock,
		options: options,
	}
}

// WithLock 使用锁执行函数
func (s *LockService) WithLock(ctx context.Context, key string, fn func() error) error {
	return s.WithLockOptions(ctx, key, s.options, fn)
}

// WithLockOptions 使用自定义选项的锁执行函数
func (s *LockService) WithLockOptions(ctx context.Context, key string, opts *LockOptions, fn func() error) error {
	if err := s.lock.Lock(ctx, key, opts); err != nil {
		return fmt.Errorf("acquire lock failed: %w", err)
	}

	defer func() {
		if err := s.lock.Unlock(ctx, key); err != nil {
			// 记录解锁失败的错误，但不影响主流程
			fmt.Printf("unlock failed for key %s: %v\n", key, err)
		}
	}()

	return fn()
}

// TryWithLock 尝试使用锁执行函数
func (s *LockService) TryWithLock(ctx context.Context, key string, fn func() error) (bool, error) {
	return s.TryWithLockOptions(ctx, key, s.options, fn)
}

// TryWithLockOptions 尝试使用自定义选项的锁执行函数
func (s *LockService) TryWithLockOptions(ctx context.Context, key string, opts *LockOptions, fn func() error) (bool, error) {
	acquired, err := s.lock.TryLock(ctx, key, opts)
	if err != nil {
		return false, fmt.Errorf("try acquire lock failed: %w", err)
	}

	if !acquired {
		return false, nil
	}

	defer func() {
		if err := s.lock.Unlock(ctx, key); err != nil {
			// 记录解锁失败的错误，但不影响主流程
			fmt.Printf("unlock failed for key %s: %v\n", key, err)
		}
	}()

	return true, fn()
}

// IsLocked 检查锁是否被持有
func (s *LockService) IsLocked(ctx context.Context, key string) (bool, error) {
	return s.lock.IsLocked(ctx, key)
}

// Renew 续期锁
func (s *LockService) Renew(ctx context.Context, key string, ttl time.Duration) error {
	return s.lock.Renew(ctx, key, ttl)
}

// GetLockInfo 获取锁信息（如果支持的话）
func (s *LockService) GetLockInfo(ctx context.Context, key string) (map[string]interface{}, error) {
	isLocked, err := s.IsLocked(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get lock status: %w", err)
	}

	info := map[string]interface{}{
		"key":         key,
		"is_locked":   isLocked,
		"ttl":         s.options.TTL.String(),
		"retry_count": s.options.RetryCount,
		"retry_delay": s.options.RetryDelay.String(),
		"lock_type":   s.options.LockType,
	}

	return info, nil
}

// AutoRenew 自动续期锁（在后台定期续期）
func (s *LockService) AutoRenew(ctx context.Context, key string, interval time.Duration, stopChan <-chan struct{}) error {
	if interval <= 0 {
		interval = s.options.TTL / 3 // 默认在TTL的1/3时续期
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stopChan:
			return nil
		case <-ticker.C:
			if err := s.Renew(ctx, key, s.options.TTL); err != nil {
				return fmt.Errorf("auto renew failed: %w", err)
			}
		}
	}
}

// Close 关闭服务
func (s *LockService) Close() error {
	return s.lock.Close()
}

// LockManager 锁管理器（支持多种锁类型）
type LockManager struct {
	services map[string]*LockService
	mu       sync.RWMutex
}

// NewLockManager 创建锁管理器
func NewLockManager() *LockManager {
	return &LockManager{
		services: make(map[string]*LockService),
	}
}

// RegisterService 注册锁服务
func (lm *LockManager) RegisterService(name string, service *LockService) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.services[name] = service
}

// GetService 获取锁服务
func (lm *LockManager) GetService(name string) (*LockService, bool) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	service, exists := lm.services[name]
	return service, exists
}

// WithLock 使用指定服务执行锁操作
func (lm *LockManager) WithLock(serviceName string, ctx context.Context, key string, fn func() error) error {
	service, exists := lm.GetService(serviceName)
	if !exists {
		return fmt.Errorf("lock service '%s' not found", serviceName)
	}
	return service.WithLock(ctx, key, fn)
}

// TryWithLock 尝试使用指定服务执行锁操作
func (lm *LockManager) TryWithLock(serviceName string, ctx context.Context, key string, fn func() error) (bool, error) {
	service, exists := lm.GetService(serviceName)
	if !exists {
		return false, fmt.Errorf("lock service '%s' not found", serviceName)
	}
	return service.TryWithLock(ctx, key, fn)
}

// Renew 续期指定服务的锁
func (lm *LockManager) Renew(serviceName string, ctx context.Context, key string, ttl time.Duration) error {
	service, exists := lm.GetService(serviceName)
	if !exists {
		return fmt.Errorf("lock service '%s' not found", serviceName)
	}
	return service.Renew(ctx, key, ttl)
}

// IsLocked 检查指定服务的锁是否被持有
func (lm *LockManager) IsLocked(serviceName string, ctx context.Context, key string) (bool, error) {
	service, exists := lm.GetService(serviceName)
	if !exists {
		return false, fmt.Errorf("lock service '%s' not found", serviceName)
	}
	return service.IsLocked(ctx, key)
}

// Close 关闭所有服务
func (lm *LockManager) Close() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	var lastErr error
	for name, service := range lm.services {
		if err := service.Close(); err != nil {
			lastErr = fmt.Errorf("close service '%s' failed: %w", name, err)
		}
	}

	return lastErr
}
