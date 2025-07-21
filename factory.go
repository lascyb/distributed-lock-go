package distributedlock

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.etcd.io/etcd/client/v3"
)

// BackendType 后端类型枚举
type BackendType int

const (
	// BackendTypeRedis Redis后端 {0:Redis后端}
	BackendTypeRedis BackendType = iota
	// BackendTypeEtcd Etcd后端 {1:Etcd后端}
	BackendTypeEtcd
	// BackendTypeEtcdMutex Etcd Mutex后端 {2:Etcd Mutex后端}
	BackendTypeEtcdMutex
)

// RedisConfig Redis配置
type RedisConfig struct {
	// 嵌入 Redis 客户端配置
	*redis.Options

	// 锁配置
	Prefix string // 锁键前缀
}

// EtcdConfig Etcd配置
type EtcdConfig struct {
	// 嵌入 Etcd 客户端配置
	*clientv3.Config

	// 锁配置
	Prefix string // 锁键前缀
}

// NewDistributedLock 创建分布式锁实例
func NewDistributedLock(backendType BackendType, config interface{}) (DistributedLock, error) {
	switch backendType {
	case BackendTypeRedis:
		redisConfig, ok := config.(*RedisConfig)
		if !ok {
			return nil, fmt.Errorf("invalid redis config type")
		}
		return NewRedisDistributedLock(redisConfig)
	case BackendTypeEtcd:
		etcdConfig, ok := config.(*EtcdConfig)
		if !ok {
			return nil, fmt.Errorf("invalid etcd config type")
		}
		return NewEtcdDistributedLock(etcdConfig)
	case BackendTypeEtcdMutex:
		etcdConfig, ok := config.(*EtcdConfig)
		if !ok {
			return nil, fmt.Errorf("invalid etcd config type")
		}
		return NewEtcdMutexDistributedLock(etcdConfig)
	default:
		return nil, fmt.Errorf("unsupported backend type: %d", backendType)
	}
}

// NewRedisDistributedLock 创建Redis分布式锁
func NewRedisDistributedLock(config *RedisConfig) (DistributedLock, error) {
	if config == nil {
		return nil, fmt.Errorf("redis config cannot be nil")
	}

	// 设置默认值
	if config.Options == nil {
		config.Options = &redis.Options{}
	}
	if config.Options.PoolSize == 0 {
		config.Options.PoolSize = 10
	}
	if config.Options.MinIdleConns == 0 {
		config.Options.MinIdleConns = 5
	}
	if config.Options.MaxRetries == 0 {
		config.Options.MaxRetries = 3
	}
	if config.Options.DialTimeout == 0 {
		config.Options.DialTimeout = 5 * time.Second
	}
	if config.Options.ReadTimeout == 0 {
		config.Options.ReadTimeout = 3 * time.Second
	}
	if config.Options.WriteTimeout == 0 {
		config.Options.WriteTimeout = 3 * time.Second
	}
	if config.Options.PoolTimeout == 0 {
		config.Options.PoolTimeout = 5 * time.Minute
	}

	client := redis.NewClient(config.Options)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return NewRedisLock(client, config.Prefix), nil
}

// NewEtcdDistributedLock 创建Etcd分布式锁
func NewEtcdDistributedLock(config *EtcdConfig) (DistributedLock, error) {
	if config == nil {
		return nil, fmt.Errorf("etcd config cannot be nil")
	}

	// 设置默认值
	if config.Config == nil {
		config.Config = &clientv3.Config{}
	}
	if config.Config.DialTimeout == 0 {
		config.Config.DialTimeout = 5 * time.Second
	}
	if config.Config.MaxCallSendMsgSize == 0 {
		config.Config.MaxCallSendMsgSize = 2 * 1024 * 1024 // 2MB
	}
	if config.Config.MaxCallRecvMsgSize == 0 {
		config.Config.MaxCallRecvMsgSize = 2 * 1024 * 1024 // 2MB
	}

	client, err := clientv3.New(*config.Config)
	if err != nil {
		return nil, fmt.Errorf("create etcd client failed: %w", err)
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if len(config.Config.Endpoints) > 0 {
		if _, err := client.Status(ctx, config.Config.Endpoints[0]); err != nil {
			client.Close()
			return nil, fmt.Errorf("etcd status check failed: %w", err)
		}
	}

	return NewEtcdLock(client, config.Prefix), nil
}

// NewEtcdMutexDistributedLock 创建基于Mutex的Etcd分布式锁
func NewEtcdMutexDistributedLock(config *EtcdConfig) (DistributedLock, error) {
	if config == nil {
		return nil, fmt.Errorf("etcd config cannot be nil")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   config.Endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("create etcd client failed: %w", err)
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Status(ctx, config.Endpoints[0]); err != nil {
		client.Close()
		return nil, fmt.Errorf("etcd status check failed: %w", err)
	}

	return NewEtcdMutexLock(client, config.Prefix), nil
}
