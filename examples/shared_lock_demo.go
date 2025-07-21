package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	distributedlock "github.com/lascyb/distributed-lock-go"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 创建Redis客户端
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	defer client.Close()

	// 直接创建组合锁实例
	lock := distributedlock.NewRedisCombinedLock(client, "example:")

	// 演示排他锁
	fmt.Println("=== 排他锁演示 ===")
	demonstrateExclusiveLock(lock)

	// 等待一段时间，确保锁被释放
	time.Sleep(2 * time.Second)

	// 演示共享锁
	fmt.Println("\n=== 共享锁演示 ===")
	demonstrateSharedLock(lock)

	// 等待一段时间，确保锁被释放
	time.Sleep(2 * time.Second)

	// 演示排他锁与共享锁的互斥
	fmt.Println("\n=== 排他锁与共享锁互斥演示 ===")
	demonstrateExclusiveVsShared(lock)

	// 演示获取锁详细信息
	fmt.Println("\n=== 锁信息演示 ===")
	demonstrateLockInfo(lock)
}

// 演示排他锁
func demonstrateExclusiveLock(lock distributedlock.DistributedLock) {
	ctx := context.Background()
	key := "exclusive_test"

	var wg sync.WaitGroup
	wg.Add(3)

	// 启动3个协程尝试获取排他锁
	for i := 0; i < 3; i++ {
		go func(id int) {
			defer wg.Done()

			opts := &distributedlock.LockOptions{
				TTL:        10 * time.Second,
				RetryCount: 3,
				RetryDelay: 100 * time.Millisecond,
				LockType:   distributedlock.LockTypeExclusive,
			}

			fmt.Printf("协程 %d: 尝试获取排他锁...\n", id)
			start := time.Now()

			acquired, err := lock.TryLock(ctx, key, opts)
			if err != nil {
				fmt.Printf("协程 %d: 获取锁失败: %v\n", id, err)
				return
			}

			if acquired {
				fmt.Printf("协程 %d: 成功获取排他锁 (耗时: %v)\n", id, time.Since(start))
				// 模拟工作
				time.Sleep(2 * time.Second)

				// 释放锁
				if err := lock.Unlock(ctx, key); err != nil {
					fmt.Printf("协程 %d: 释放锁失败: %v\n", id, err)
				} else {
					fmt.Printf("协程 %d: 成功释放排他锁\n", id)
				}
			} else {
				fmt.Printf("协程 %d: 无法获取排他锁 (已被其他协程持有)\n", id)
			}
		}(i)
	}

	wg.Wait()
}

// 演示共享锁
func demonstrateSharedLock(lock distributedlock.DistributedLock) {
	ctx := context.Background()
	key := "shared_test"

	var wg sync.WaitGroup
	wg.Add(5)

	// 启动5个协程尝试获取共享锁
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer wg.Done()

			opts := &distributedlock.LockOptions{
				TTL:        10 * time.Second,
				RetryCount: 3,
				RetryDelay: 100 * time.Millisecond,
				LockType:   distributedlock.LockTypeShared,
			}

			fmt.Printf("协程 %d: 尝试获取共享锁...\n", id)
			start := time.Now()

			acquired, err := lock.TryLock(ctx, key, opts)
			if err != nil {
				fmt.Printf("协程 %d: 获取锁失败: %v\n", id, err)
				return
			}

			if acquired {
				fmt.Printf("协程 %d: 成功获取共享锁 (耗时: %v)\n", id, time.Since(start))

				// 模拟读取操作
				time.Sleep(3 * time.Second)

				// 释放锁
				if err := lock.Unlock(ctx, key); err != nil {
					fmt.Printf("协程 %d: 释放锁失败: %v\n", id, err)
				} else {
					fmt.Printf("协程 %d: 成功释放共享锁\n", id)
				}
			} else {
				fmt.Printf("协程 %d: 无法获取共享锁\n", id)
			}
		}(i)
	}

	wg.Wait()
}

// 演示排他锁与共享锁的互斥
func demonstrateExclusiveVsShared(lock distributedlock.DistributedLock) {
	ctx := context.Background()
	key := "exclusive_vs_shared_test"

	// 首先获取一个排他锁
	exclusiveOpts := &distributedlock.LockOptions{
		TTL:        5 * time.Second,
		RetryCount: 3,
		RetryDelay: 100 * time.Millisecond,
		LockType:   distributedlock.LockTypeExclusive,
	}

	fmt.Println("主协程: 获取排他锁...")
	acquired, err := lock.TryLock(ctx, key, exclusiveOpts)
	if err != nil {
		log.Fatalf("获取排他锁失败: %v", err)
	}

	if !acquired {
		fmt.Println("主协程: 无法获取排他锁")
		return
	}

	fmt.Println("主协程: 成功获取排他锁")

	// 启动多个协程尝试获取共享锁
	var wg sync.WaitGroup
	wg.Add(3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			defer wg.Done()

			sharedOpts := &distributedlock.LockOptions{
				TTL:        10 * time.Second,
				RetryCount: 1,
				RetryDelay: 100 * time.Millisecond,
				LockType:   distributedlock.LockTypeShared,
			}

			fmt.Printf("协程 %d: 尝试获取共享锁（排他锁存在时）...\n", id)
			acquired, err := lock.TryLock(ctx, key, sharedOpts)
			if err != nil {
				fmt.Printf("协程 %d: 获取共享锁失败: %v\n", id, err)
				return
			}

			if acquired {
				fmt.Printf("协程 %d: 意外获取了共享锁（这不应该发生）\n", id)
				lock.Unlock(ctx, key)
			} else {
				fmt.Printf("协程 %d: 正确地无法获取共享锁（排他锁存在）\n", id)
			}
		}(i)
	}

	// 等待一段时间再释放排他锁
	time.Sleep(2 * time.Second)
	fmt.Println("主协程: 释放排他锁")
	lock.Unlock(ctx, key)

	wg.Wait()

	// 现在尝试获取多个共享锁
	fmt.Println("\n排他锁释放后，尝试获取多个共享锁:")

	var wg2 sync.WaitGroup
	wg2.Add(3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			defer wg2.Done()

			sharedOpts := &distributedlock.LockOptions{
				TTL:        5 * time.Second,
				RetryCount: 3,
				RetryDelay: 100 * time.Millisecond,
				LockType:   distributedlock.LockTypeShared,
			}

			fmt.Printf("协程 %d: 尝试获取共享锁（排他锁已释放）...\n", id)
			acquired, err := lock.TryLock(ctx, key, sharedOpts)
			if err != nil {
				fmt.Printf("协程 %d: 获取共享锁失败: %v\n", id, err)
				return
			}

			if acquired {
				fmt.Printf("协程 %d: 成功获取共享锁\n", id)
				time.Sleep(2 * time.Second)
				lock.Unlock(ctx, key)
				fmt.Printf("协程 %d: 释放共享锁\n", id)
			} else {
				fmt.Printf("协程 %d: 无法获取共享锁\n", id)
			}
		}(i)
	}

	wg2.Wait()
}

// 演示获取锁详细信息
func demonstrateLockInfo(lock distributedlock.DistributedLock) {
	ctx := context.Background()
	key := "info_test"

	// 将lock转换为组合锁类型以访问GetLockInfo方法
	combinedLock, ok := lock.(*distributedlock.RedisCombinedLock)
	if !ok {
		fmt.Println("Lock is not a RedisCombinedLock")
		return
	}

	// 获取多个共享锁
	sharedOpts := &distributedlock.LockOptions{
		TTL:        10 * time.Second,
		RetryCount: 3,
		RetryDelay: 100 * time.Millisecond,
		LockType:   distributedlock.LockTypeShared,
	}

	fmt.Println("获取3个共享锁...")
	for i := 0; i < 3; i++ {
		acquired, err := lock.TryLock(ctx, key, sharedOpts)
		if err != nil || !acquired {
			fmt.Printf("获取共享锁 %d 失败\n", i+1)
			continue
		}
		fmt.Printf("成功获取共享锁 %d\n", i+1)
	}

	// 获取锁信息
	info, err := combinedLock.GetLockInfo(ctx, key)
	if err != nil {
		fmt.Printf("获取锁信息失败: %v\n", err)
	} else {
		fmt.Printf("锁信息: %+v\n", info)
	}

	// 清理锁
	for i := 0; i < 3; i++ {
		lock.Unlock(ctx, key)
	}
}
