package distributedlock

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// TestExample 测试示例
func TestExample(t *testing.T) {
	fmt.Println("=== 分布式锁测试示例 ===")

	// 注意：这个示例需要本地运行 Redis 和 Etcd 服务
	// 如果没有运行这些服务，请先启动它们

	// 测试 Redis 锁
	testRedisLock()

	// 测试 Etcd 锁
	testEtcdLock()

	fmt.Println("=== 测试完成 ===")
}

// testRedisLock 测试 Redis 锁
func testRedisLock() {
	fmt.Println("\n--- 测试 Redis 锁 ---")

	// 创建 Redis 配置
	redisConfig := &RedisConfig{
		Options: &redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			PoolSize: 10,
		},
		Prefix: "test:lock:",
	}

	// 创建 Redis 分布式锁
	redisLock, err := NewRedisDistributedLock(redisConfig)
	if err != nil {
		log.Printf("创建 Redis 锁失败: %v", err)
		return
	}
	defer redisLock.Close()

	// 创建锁服务
	lockService := NewLockService(redisLock, &LockOptions{
		TTL:        10 * time.Second,
		RetryCount: 3,
		RetryDelay: 100 * time.Millisecond,
		LockType:   LockTypeExclusive,
	})

	ctx := context.Background()

	// 测试基本锁操作
	fmt.Println("1. 测试基本锁操作")
	err = lockService.WithLock(ctx, "test-resource", func() error {
		fmt.Println("   - 获取到锁，执行临界区代码...")
		time.Sleep(1 * time.Second)
		fmt.Println("   - 临界区代码执行完成")
		return nil
	})

	if err != nil {
		log.Printf("   - 锁操作失败: %v", err)
	} else {
		fmt.Println("   - 锁操作成功")
	}

	// 测试尝试锁操作
	fmt.Println("\n2. 测试尝试锁操作")
	acquired, err := lockService.TryWithLock(ctx, "test-resource", func() error {
		fmt.Println("   - 成功获取到锁，执行任务...")
		time.Sleep(500 * time.Millisecond)
		return nil
	})

	if err != nil {
		log.Printf("   - 尝试锁操作失败: %v", err)
	} else if !acquired {
		fmt.Println("   - 锁已被其他进程持有")
	} else {
		fmt.Println("   - 尝试锁操作成功")
	}

	// 测试锁状态检查
	fmt.Println("\n3. 测试锁状态检查")
	isLocked, err := lockService.IsLocked(ctx, "test-resource")
	if err != nil {
		log.Printf("   - 检查锁状态失败: %v", err)
	} else {
		fmt.Printf("   - 锁状态: %v\n", isLocked)
	}
}

// testEtcdLock 测试 Etcd 锁
func testEtcdLock() {
	fmt.Println("\n--- 测试 Etcd 锁 ---")

	// 创建 Etcd 配置
	etcdConfig := &EtcdConfig{
		Config: &clientv3.Config{
			Endpoints: []string{"localhost:2379"},
		},
		Prefix: "/test-locks/",
	}

	// 创建 Etcd 分布式锁
	etcdLock, err := NewEtcdDistributedLock(etcdConfig)
	if err != nil {
		log.Printf("创建 Etcd 锁失败: %v", err)
		return
	}
	defer etcdLock.Close()

	// 创建锁服务
	lockService := NewLockService(etcdLock, &LockOptions{
		TTL:        10 * time.Second,
		RetryCount: 3,
		RetryDelay: 100 * time.Millisecond,
		LockType:   LockTypeExclusive,
	})

	ctx := context.Background()

	// 测试基本锁操作
	fmt.Println("1. 测试基本锁操作")
	err = lockService.WithLock(ctx, "test-resource", func() error {
		fmt.Println("   - 获取到锁，执行临界区代码...")
		time.Sleep(1 * time.Second)
		fmt.Println("   - 临界区代码执行完成")
		return nil
	})

	if err != nil {
		log.Printf("   - 锁操作失败: %v", err)
	} else {
		fmt.Println("   - 锁操作成功")
	}

	// 测试尝试锁操作
	fmt.Println("\n2. 测试尝试锁操作")
	acquired, err := lockService.TryWithLock(ctx, "test-resource", func() error {
		fmt.Println("   - 成功获取到锁，执行任务...")
		time.Sleep(500 * time.Millisecond)
		return nil
	})

	if err != nil {
		log.Printf("   - 尝试锁操作失败: %v", err)
	} else if !acquired {
		fmt.Println("   - 锁已被其他进程持有")
	} else {
		fmt.Println("   - 尝试锁操作成功")
	}

	// 测试锁状态检查
	fmt.Println("\n3. 测试锁状态检查")
	isLocked, err := lockService.IsLocked(ctx, "test-resource")
	if err != nil {
		log.Printf("   - 检查锁状态失败: %v", err)
	} else {
		fmt.Printf("   - 锁状态: %v\n", isLocked)
	}
}

// TestConcurrentAccess 测试并发访问
func TestConcurrentAccess(t *testing.T) {
	fmt.Println("\n=== 测试并发访问 ===")

	// 创建 Redis 锁
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
		log.Printf("创建 Redis 锁失败: %v", err)
		return
	}
	defer redisLock.Close()

	lockService := NewLockService(redisLock, &LockOptions{
		TTL:        5 * time.Second,
		RetryCount: 10,
		RetryDelay: 100 * time.Millisecond,
	})

	// 模拟多个 goroutine 并发访问
	fmt.Println("启动 5 个并发 goroutine...")
	for i := 0; i < 5; i++ {
		go func(id int) {
			ctx := context.Background()
			err := lockService.WithLock(ctx, "shared-resource", func() error {
				fmt.Printf("   Goroutine %d 获取到锁\n", id)
				time.Sleep(1 * time.Second) // 模拟工作
				fmt.Printf("   Goroutine %d 释放锁\n", id)
				return nil
			})
			if err != nil {
				log.Printf("   Goroutine %d 锁操作失败: %v", id, err)
			}
		}(i)
	}

	// 等待所有 goroutine 完成
	time.Sleep(10 * time.Second)
	fmt.Println("所有并发操作完成")
}
