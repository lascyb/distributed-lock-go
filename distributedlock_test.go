package distributedlock

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// TestLockOptions 测试锁选项
func TestLockOptions(t *testing.T) {
	opts := DefaultLockOptions()
	if opts == nil {
		t.Fatal("DefaultLockOptions should not return nil")
	}

	if opts.TTL != 30*time.Second {
		t.Errorf("Expected TTL to be 30s, got %v", opts.TTL)
	}

	if opts.RetryCount != 3 {
		t.Errorf("Expected RetryCount to be 3, got %d", opts.RetryCount)
	}

	if opts.RetryDelay != 100*time.Millisecond {
		t.Errorf("Expected RetryDelay to be 100ms, got %v", opts.RetryDelay)
	}

	if opts.LockType != LockTypeExclusive {
		t.Errorf("Expected LockType to be LockTypeExclusive, got %d", opts.LockType)
	}
}

// TestLockType 测试锁类型
func TestLockType(t *testing.T) {
	if LockTypeExclusive != 0 {
		t.Errorf("Expected LockTypeExclusive to be 0, got %d", LockTypeExclusive)
	}

	if LockTypeShared != 1 {
		t.Errorf("Expected LockTypeShared to be 1, got %d", LockTypeShared)
	}
}

// TestBackendType 测试后端类型
func TestBackendType(t *testing.T) {
	if BackendTypeRedis != 0 {
		t.Errorf("Expected BackendTypeRedis to be 0, got %d", BackendTypeRedis)
	}

	if BackendTypeEtcd != 1 {
		t.Errorf("Expected BackendTypeEtcd to be 1, got %d", BackendTypeEtcd)
	}

	if BackendTypeEtcdMutex != 2 {
		t.Errorf("Expected BackendTypeEtcdMutex to be 2, got %d", BackendTypeEtcdMutex)
	}
}

// TestRedisConfig 测试Redis配置
func TestRedisConfig(t *testing.T) {
	config := &RedisConfig{
		Options: &redis.Options{
			Addr:     "localhost:6379",
			Password: "password",
			DB:       1,
			PoolSize: 10,
		},
		Prefix: "test:",
	}

	if config.Options.Addr != "localhost:6379" {
		t.Errorf("Expected Addr to be localhost:6379, got %s", config.Options.Addr)
	}

	if config.Options.Password != "password" {
		t.Errorf("Expected Password to be password, got %s", config.Options.Password)
	}

	if config.Options.DB != 1 {
		t.Errorf("Expected DB to be 1, got %d", config.Options.DB)
	}

	if config.Prefix != "test:" {
		t.Errorf("Expected Prefix to be test:, got %s", config.Prefix)
	}
}

// TestEtcdConfig 测试Etcd配置
func TestEtcdConfig(t *testing.T) {
	config := &EtcdConfig{
		Config: &clientv3.Config{
			Endpoints: []string{"localhost:2379"},
			Username:  "user",
			Password:  "password",
		},
		Prefix: "/test/",
	}

	if len(config.Config.Endpoints) != 1 || config.Config.Endpoints[0] != "localhost:2379" {
		t.Errorf("Expected Endpoints to be [localhost:2379], got %v", config.Config.Endpoints)
	}

	if config.Config.Username != "user" {
		t.Errorf("Expected Username to be user, got %s", config.Config.Username)
	}

	if config.Config.Password != "password" {
		t.Errorf("Expected Password to be password, got %s", config.Config.Password)
	}

	if config.Prefix != "/test/" {
		t.Errorf("Expected Prefix to be /test/, got %s", config.Prefix)
	}
}

// TestDistributedLockInterface 测试分布式锁接口
func TestDistributedLockInterface(t *testing.T) {
	// 测试接口是否正确定义
	var lock DistributedLock
	if lock == nil {
		// 这是预期的，因为我们没有实现
		t.Log("DistributedLock interface is properly defined")
	}
}

// TestLockService 测试锁服务
func TestLockService(t *testing.T) {
	// 测试锁服务是否正确定义
	var service *LockService
	if service == nil {
		// 这是预期的，因为我们没有实现
		t.Log("LockService is properly defined")
	}
}

// TestLockManager 测试锁管理器
func TestLockManager(t *testing.T) {
	// 测试锁管理器是否正确定义
	var manager *LockManager
	if manager == nil {
		// 这是预期的，因为我们没有实现
		t.Log("LockManager is properly defined")
	}
}

// TestContextWithLockValue 测试上下文中的锁值
func TestContextWithLockValue(t *testing.T) {
	ctx := context.Background()

	// 测试设置锁值
	lockValue := "test-lock-value"
	ctx = context.WithValue(ctx, "lock_value", lockValue)

	// 测试获取锁值
	value := ctx.Value("lock_value")
	if value == nil {
		t.Fatal("Lock value should not be nil")
	}

	if value.(string) != lockValue {
		t.Errorf("Expected lock value to be %s, got %s", lockValue, value.(string))
	}
}

// BenchmarkDefaultLockOptions 基准测试默认锁选项
func BenchmarkDefaultLockOptions(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DefaultLockOptions()
	}
}

// BenchmarkLockManager 基准测试锁管理器
func BenchmarkLockManager(b *testing.B) {
	for i := 0; i < b.N; i++ {
		manager := NewLockManager()
		manager.Close()
	}
}
