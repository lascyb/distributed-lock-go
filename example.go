package distributedlock

import (
	"context"
	"fmt"

	"log"
	"time"

	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ExampleUsage 使用示例
func ExampleUsage() {
	// 创建Redis分布式锁
	redisConfig := &RedisConfig{
		Options: &redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			PoolSize: 10,
		},
		Prefix: "lock:",
	}

	redisLock, err := NewRedisDistributedLock(redisConfig)
	if err != nil {
		log.Fatalf("创建Redis锁失败: %v", err)
	}
	defer redisLock.Close()

	// 创建锁服务
	lockService := NewLockService(redisLock, &LockOptions{
		TTL:        30 * time.Second,
		RetryCount: 3,
		RetryDelay: 100 * time.Millisecond,
		LockType:   LockTypeExclusive,
	})

	// 使用锁执行临界区代码
	ctx := context.Background()
	err = lockService.WithLock(ctx, "my-resource", func() error {
		fmt.Println("获取到锁，执行临界区代码...")
		time.Sleep(2 * time.Second) // 模拟工作
		fmt.Println("临界区代码执行完成")
		return nil
	})
	if err != nil {
		log.Printf("执行锁操作失败: %v", err)
	}
}

// ExampleTryLock 尝试锁示例
func ExampleTryLock() {
	// 创建Etcd分布式锁
	etcdConfig := &EtcdConfig{
		Config: &clientv3.Config{
			Endpoints: []string{"localhost:2379"},
		},
		Prefix: "/locks/",
	}

	etcdLock, err := NewEtcdDistributedLock(etcdConfig)
	if err != nil {
		log.Fatalf("创建Etcd锁失败: %v", err)
	}
	defer etcdLock.Close()

	// 创建锁服务
	lockService := NewLockService(etcdLock, nil) // 使用默认选项

	ctx := context.Background()

	// 尝试获取锁
	acquired, err := lockService.TryWithLock(ctx, "my-resource", func() error {
		fmt.Println("成功获取到锁，执行任务...")
		time.Sleep(1 * time.Second)
		return nil
	})
	if err != nil {
		log.Printf("尝试锁操作失败: %v", err)
		return
	}

	if !acquired {
		fmt.Println("锁已被其他进程持有，无法获取")
	} else {
		fmt.Println("任务执行完成")
	}
}

// ExampleLockManager 锁管理器示例
func ExampleLockManager() {
	// 创建锁管理器
	manager := NewLockManager()
	defer manager.Close()

	// 注册Redis锁服务
	redisConfig := &RedisConfig{
		Options: &redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			PoolSize: 10,
		},
		Prefix: "redis-lock:",
	}

	redisLock, err := NewRedisDistributedLock(redisConfig)
	if err != nil {
		log.Fatalf("创建Redis锁失败: %v", err)
	}

	redisService := NewLockService(redisLock, &LockOptions{
		TTL:        60 * time.Second,
		RetryCount: 5,
		RetryDelay: 200 * time.Millisecond,
	})

	manager.RegisterService("redis", redisService)

	// 注册Etcd锁服务
	etcdConfig := &EtcdConfig{
		Config: &clientv3.Config{
			Endpoints: []string{"localhost:2379"},
		},
		Prefix: "/etcd-locks/",
	}

	etcdLock, err := NewEtcdDistributedLock(etcdConfig)
	if err != nil {
		log.Fatalf("创建Etcd锁失败: %v", err)
	}

	etcdService := NewLockService(etcdLock, &LockOptions{
		TTL:        120 * time.Second,
		RetryCount: 3,
		RetryDelay: 500 * time.Millisecond,
	})

	manager.RegisterService("etcd", etcdService)

	// 使用不同的锁服务
	ctx := context.Background()

	// 使用Redis锁
	err = manager.WithLock("redis", ctx, "resource-1", func() error {
		fmt.Println("使用Redis锁保护资源1")
		time.Sleep(1 * time.Second)
		return nil
	})
	if err != nil {
		log.Printf("Redis锁操作失败: %v", err)
	}

	// 使用Etcd锁
	err = manager.WithLock("etcd", ctx, "resource-2", func() error {
		fmt.Println("使用Etcd锁保护资源2")
		time.Sleep(1 * time.Second)
		return nil
	})
	if err != nil {
		log.Printf("Etcd锁操作失败: %v", err)
	}
}

// ExampleConcurrentAccess 并发访问示例
func ExampleConcurrentAccess() {
	// 创建Redis锁
	redisConfig := &RedisConfig{
		Options: &redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			PoolSize: 10,
		},
		Prefix: "concurrent:",
	}

	redisLock, err := NewRedisDistributedLock(redisConfig)
	if err != nil {
		log.Fatalf("创建Redis锁失败: %v", err)
	}
	defer redisLock.Close()

	lockService := NewLockService(redisLock, &LockOptions{
		TTL:        10 * time.Second,
		RetryCount: 10,
		RetryDelay: 100 * time.Millisecond,
	})

	// 模拟多个goroutine并发访问
	for i := 0; i < 5; i++ {
		go func(id int) {
			ctx := context.Background()
			err := lockService.WithLock(ctx, "shared-resource", func() error {
				fmt.Printf("Goroutine %d 获取到锁\n", id)
				time.Sleep(2 * time.Second) // 模拟工作
				fmt.Printf("Goroutine %d 释放锁\n", id)
				return nil
			})
			if err != nil {
				log.Printf("Goroutine %d 锁操作失败: %v", id, err)
			}
		}(i)
	}

	// 等待所有goroutine完成
	time.Sleep(15 * time.Second)
	fmt.Println("所有并发操作完成")
}
