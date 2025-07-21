# Distributed Lock Go

一个基于 Go 的高性能分布式锁实现，支持 Redis 和 Etcd 两种后端存储。该包提供了统一的分布式锁接口，具备完善的锁管理功能，适用于分布式系统中的资源协调和并发控制。

## 🚀 核心特性

### 1. 多后端支持
- **Redis 分布式锁**: 基于 Redis SETNX + Lua 脚本实现，高性能、低延迟
- **Etcd 分布式锁**: 基于 Etcd 事务实现，强一致性保证
- **Etcd Mutex 分布式锁**: 基于 Etcd concurrency 包实现，简化版本

### 2. 完善的锁机制
- **原子性操作**: 使用数据库原生原子操作确保锁的原子性
- **TTL 支持**: 自动过期机制防止死锁
- **锁续期**: 支持长时间任务执行时的锁续期，包括手动续期和自动续期
- **重试机制**: 可配置的重试策略，提高锁获取成功率
- **阻塞/非阻塞**: 支持阻塞式和非阻塞式锁获取

### 3. 便捷的服务层
- **LockService**: 提供高级锁操作接口，简化使用
- **LockManager**: 支持多种锁类型的统一管理
- **上下文隔离**: 不同上下文的锁操作相互隔离
- **自动续期**: 支持长时间任务的自动续期功能
- **锁信息查询**: 提供锁状态和配置信息查询

### 4. 配置兼容性
- **Redis 配置**: 直接嵌入 `redis.Options` 结构体
- **Etcd 配置**: 直接嵌入 `clientv3.Config` 结构体
- 完全兼容标准客户端配置，无需额外学习成本

## 📦 项目结构

```
distributed-lock-go/
├── interface.go                # 接口定义和基础类型
├── redis.go                    # Redis 分布式锁实现
├── etcd.go                     # Etcd 分布式锁实现
├── factory.go                  # 工厂函数和配置
├── service.go                  # 服务包装器和管理器
├── example.go                  # 使用示例
├── example_test.go             # 示例测试
├── distributedlock_test.go     # 单元测试
├── go.mod                      # 模块定义
├── go.sum                      # 依赖校验和
├── README.md                   # 项目文档
├── LICENSE                     # 许可证文件
```

## 🔧 技术实现

### Redis 分布式锁实现
- **加锁**: 使用 `SET key value NX EX ttl` 命令实现原子性加锁
- **解锁**: 使用 Lua 脚本确保只有锁持有者才能解锁
- **续期**: 使用 Lua 脚本实现原子性锁续期
- **唯一标识**: 每个锁都有随机生成的唯一标识符
- **线程安全**: 使用 `sync.Map` 管理锁值，确保并发安全

### Etcd 分布式锁实现
- **加锁**: 使用 Etcd 事务和租约机制实现原子性加锁
- **解锁**: 使用事务确保只有锁持有者才能解锁
- **续期**: 通过续期租约实现锁续期
- **强一致性**: 利用 Etcd 的强一致性特性
- **线程安全**: 使用 `sync.Map` 管理锁值和租约ID

### 安全特性
- **唯一标识**: 每个锁都有唯一的标识符，防止误删其他进程的锁
- **TTL 保护**: 自动过期机制防止死锁
- **原子操作**: 使用数据库的原子操作确保操作的原子性
- **上下文隔离**: 不同上下文的锁操作相互隔离
- **线程安全**: 使用 `sync.Map` 确保并发安全

## 📋 安装

```bash
go get github.com/lascyb/distributed-lock-go
```

## 🚀 快速开始

### 1. 使用 Redis 分布式锁

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/lascyb/distributed-lock-go"
    "github.com/redis/go-redis/v9"
)

func main() {
    // 创建 Redis 配置
    redisConfig := &distributedlock.RedisConfig{
        Options: &redis.Options{
            Addr:     "localhost:6379",
            Password: "",
            DB:       0,
            PoolSize: 10,
        },
        Prefix: "lock:",
    }
    
    // 创建 Redis 分布式锁
    redisLock, err := distributedlock.NewRedisDistributedLock(redisConfig)
    if err != nil {
        log.Fatalf("创建 Redis 锁失败: %v", err)
    }
    defer redisLock.Close()
    
    // 创建锁服务
    lockService := distributedlock.NewLockService(redisLock, &distributedlock.LockOptions{
        TTL:        30 * time.Second,
        RetryCount: 3,
        RetryDelay: 100 * time.Millisecond,
        LockType:   distributedlock.LockTypeExclusive,
    })
    
    // 使用锁执行临界区代码
    ctx := context.Background()
    err = lockService.WithLock(ctx, "my-resource", func() error {
        // 这里是需要保护的临界区代码
        log.Println("获取到锁，执行临界区代码...")
        time.Sleep(2 * time.Second) // 模拟工作
        log.Println("临界区代码执行完成")
        return nil
    })
    
    if err != nil {
        log.Printf("执行锁操作失败: %v", err)
    }
}
```

### 2. 使用 Etcd 分布式锁

```go
package main

import (
    "context"
    "log"
    "github.com/lascyb/distributed-lock-go"
    clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
    // 创建 Etcd 配置
    etcdConfig := &distributedlock.EtcdConfig{
        Config: &clientv3.Config{
            Endpoints: []string{"localhost:2379"},
        },
        Prefix: "/locks/",
    }
    
    // 创建 Etcd 分布式锁
    etcdLock, err := distributedlock.NewEtcdDistributedLock(etcdConfig)
    if err != nil {
        log.Fatalf("创建 Etcd 锁失败: %v", err)
    }
    defer etcdLock.Close()
    
    // 创建锁服务
    lockService := distributedlock.NewLockService(etcdLock, nil) // 使用默认选项
    
    ctx := context.Background()
    
    // 尝试获取锁（非阻塞）
    acquired, err := lockService.TryWithLock(ctx, "my-resource", func() error {
        log.Println("成功获取到锁，执行任务...")
        return nil
    })
    
    if err != nil {
        log.Printf("尝试锁操作失败: %v", err)
        return
    }
    
    if !acquired {
        log.Println("锁已被其他进程持有，无法获取")
    } else {
        log.Println("任务执行完成")
    }
}
```

### 3. 使用锁续期功能

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/lascyb/distributed-lock-go"
)

func main() {
    // 创建锁服务
    lockService := distributedlock.NewLockService(redisLock, &distributedlock.LockOptions{
        TTL:        60 * time.Second,
        RetryCount: 3,
        RetryDelay: 100 * time.Millisecond,
    })
    
    ctx := context.Background()
    
    // 获取锁
    err := lockService.WithLock(ctx, "long-running-task", func() error {
        log.Println("开始长时间任务...")
        
        // 手动续期
        go func() {
            ticker := time.NewTicker(20 * time.Second)
            defer ticker.Stop()
            
            for {
                select {
                case <-ctx.Done():
                    return
                case <-ticker.C:
                    if err := lockService.Renew(ctx, "long-running-task", 60*time.Second); err != nil {
                        log.Printf("续期失败: %v", err)
                        return
                    }
                    log.Println("锁续期成功")
                }
            }
        }()
        
        // 执行长时间任务
        time.Sleep(5 * time.Minute)
        log.Println("长时间任务完成")
        return nil
    })
    
    if err != nil {
        log.Printf("任务执行失败: %v", err)
    }
}
```

### 4. 使用自动续期功能

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/lascyb/distributed-lock-go"
)

func main() {
    lockService := distributedlock.NewLockService(redisLock, &distributedlock.LockOptions{
        TTL:        60 * time.Second,
        RetryCount: 3,
        RetryDelay: 100 * time.Millisecond,
    })
    
    ctx := context.Background()
    
    // 使用自动续期
    err := lockService.WithLock(ctx, "auto-renew-task", func() error {
        log.Println("开始自动续期任务...")
        
        // 启动自动续期（每20秒续期一次）
        stopChan := make(chan struct{})
        go func() {
            err := lockService.AutoRenew(ctx, "auto-renew-task", 20*time.Second, stopChan)
            if err != nil {
                log.Printf("自动续期失败: %v", err)
            }
        }()
        
        // 执行任务
        time.Sleep(3 * time.Minute)
        
        // 停止自动续期
        close(stopChan)
        log.Println("任务完成")
        return nil
    })
    
    if err != nil {
        log.Printf("任务执行失败: %v", err)
    }
}
```

### 5. 使用锁管理器

```go
package main

import (
    "context"
    "log"
    "github.com/lascyb/distributed-lock-go"
)

func main() {
    // 创建锁管理器
    manager := distributedlock.NewLockManager()
    defer manager.Close()
    
    // 注册 Redis 锁服务
    redisConfig := &distributedlock.RedisConfig{
        Options: &redis.Options{
            Addr:     "localhost:6379",
            Password: "",
            DB:       0,
            PoolSize: 10,
        },
        Prefix: "redis-lock:",
    }
    
    redisLock, err := distributedlock.NewRedisDistributedLock(redisConfig)
    if err != nil {
        log.Fatalf("创建 Redis 锁失败: %v", err)
    }
    
    redisService := distributedlock.NewLockService(redisLock, &distributedlock.LockOptions{
        TTL:        60 * time.Second,
        RetryCount: 5,
        RetryDelay: 200 * time.Millisecond,
    })
    
    manager.RegisterService("redis", redisService)
    
    // 注册 Etcd 锁服务
    etcdConfig := &distributedlock.EtcdConfig{
        Config: &clientv3.Config{
            Endpoints: []string{"localhost:2379"},
        },
        Prefix: "/etcd-locks/",
    }
    
    etcdLock, err := distributedlock.NewEtcdDistributedLock(etcdConfig)
    if err != nil {
        log.Fatalf("创建 Etcd 锁失败: %v", err)
    }
    
    etcdService := distributedlock.NewLockService(etcdLock, &distributedlock.LockOptions{
        TTL:        120 * time.Second,
        RetryCount: 3,
        RetryDelay: 500 * time.Millisecond,
    })
    
    manager.RegisterService("etcd", etcdService)
    
    // 使用不同的锁服务
    ctx := context.Background()
    
    // 使用 Redis 锁
    err = manager.WithLock("redis", ctx, "resource-1", func() error {
        log.Println("使用 Redis 锁保护资源1")
        return nil
    })
    
    if err != nil {
        log.Printf("Redis 锁操作失败: %v", err)
    }
    
    // 使用 Etcd 锁
    err = manager.WithLock("etcd", ctx, "resource-2", func() error {
        log.Println("使用 Etcd 锁保护资源2")
        return nil
    })
    
    if err != nil {
        log.Printf("Etcd 锁操作失败: %v", err)
    }
    
    // 检查锁状态
    isLocked, err := manager.IsLocked("redis", ctx, "resource-1")
    if err != nil {
        log.Printf("检查锁状态失败: %v", err)
    } else {
        log.Printf("Redis 锁状态: %v", isLocked)
    }
}
```

### 6. 获取锁信息

```go
package main

import (
    "context"
    "log"
    "github.com/lascyb/distributed-lock-go"
)

func main() {
    lockService := distributedlock.NewLockService(redisLock, &distributedlock.LockOptions{
        TTL:        30 * time.Second,
        RetryCount: 3,
        RetryDelay: 100 * time.Millisecond,
    })
    
    ctx := context.Background()
    
    // 获取锁信息
    info, err := lockService.GetLockInfo(ctx, "my-resource")
    if err != nil {
        log.Printf("获取锁信息失败: %v", err)
    } else {
        log.Printf("锁信息: %+v", info)
        // 输出示例:
        // 锁信息: map[is_locked:true key:my-resource lock_type:0 retry_count:3 retry_delay:100ms ttl:30s]
    }
}
```

## 🧪 运行测试

```bash
go test -v
```

## 📚 API 文档

### 核心接口

#### DistributedLock

```go
type DistributedLock interface {
    // TryLock 尝试获取锁（非阻塞）
    TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error)
    
    // Lock 获取锁（阻塞）
    Lock(ctx context.Context, key string, opts *LockOptions) error
    
    // Unlock 释放锁
    Unlock(ctx context.Context, key string) error
    
    // Renew 续期锁
    Renew(ctx context.Context, key string, ttl time.Duration) error
    
    // IsLocked 检查锁是否被持有
    IsLocked(ctx context.Context, key string) (bool, error)
    
    // Close 关闭连接
    Close() error
}
```

#### LockOptions

```go
type LockOptions struct {
    TTL         time.Duration // 锁的生存时间
    RetryCount  int           // 重试次数
    RetryDelay  time.Duration // 重试延迟
    LockType    LockType      // 锁类型
}
```

#### LockType

```go
type LockType int

const (
    LockTypeExclusive LockType = iota // 排他锁 {0:排他锁}
    LockTypeShared                    // 共享锁 {1:共享锁}
)
```

### 服务层接口

#### LockService

```go
type LockService struct {
    // 内部字段
}

// WithLock 使用锁执行函数
func (s *LockService) WithLock(ctx context.Context, key string, fn func() error) error

// TryWithLock 尝试使用锁执行函数
func (s *LockService) TryWithLock(ctx context.Context, key string, fn func() error) (bool, error)

// Renew 续期锁
func (s *LockService) Renew(ctx context.Context, key string, ttl time.Duration) error

// IsLocked 检查锁是否被持有
func (s *LockService) IsLocked(ctx context.Context, key string) (bool, error)

// GetLockInfo 获取锁信息
func (s *LockService) GetLockInfo(ctx context.Context, key string) (map[string]interface{}, error)

// AutoRenew 自动续期锁
func (s *LockService) AutoRenew(ctx context.Context, key string, interval time.Duration, stopChan <-chan struct{}) error

// Close 关闭服务
func (s *LockService) Close() error
```

#### LockManager

```go
type LockManager struct {
    // 内部字段
}

// RegisterService 注册锁服务
func (lm *LockManager) RegisterService(name string, service *LockService)

// WithLock 使用指定服务执行锁操作
func (lm *LockManager) WithLock(serviceName string, ctx context.Context, key string, fn func() error) error

// TryWithLock 尝试使用指定服务执行锁操作
func (lm *LockManager) TryWithLock(serviceName string, ctx context.Context, key string, fn func() error) (bool, error)

// Renew 续期指定服务的锁
func (lm *LockManager) Renew(serviceName string, ctx context.Context, key string, ttl time.Duration) error

// IsLocked 检查指定服务的锁是否被持有
func (lm *LockManager) IsLocked(serviceName string, ctx context.Context, key string) (bool, error)

// Close 关闭所有服务
func (lm *LockManager) Close() error
```

### 配置结构

#### RedisConfig

```go
type RedisConfig struct {
    // 嵌入 Redis 客户端配置
    *redis.Options
    
    // 锁配置
    Prefix string // 锁键前缀
}
```

#### EtcdConfig

```go
type EtcdConfig struct {
    // 嵌入 Etcd 客户端配置
    *clientv3.Config
    
    // 锁配置
    Prefix string // 锁键前缀
}
```

### 配置示例

```go
// Redis 配置示例
redisConfig := &distributedlock.RedisConfig{
    Options: &redis.Options{
        Addr:         "localhost:6379",
        Password:     "password",
        DB:           0,
        PoolSize:     10,
        MinIdleConns: 5,
        MaxRetries:   3,
        DialTimeout:  5 * time.Second,
        ReadTimeout:  3 * time.Second,
        WriteTimeout: 3 * time.Second,
        PoolTimeout:  5 * time.Minute,
    },
    Prefix: "lock:",
}

// Etcd 配置示例
etcdConfig := &distributedlock.EtcdConfig{
    Config: &clientv3.Config{
        Endpoints:            []string{"localhost:2379"},
        Username:             "user",
        Password:             "password",
        DialTimeout:          5 * time.Second,
        MaxCallSendMsgSize:   2 * 1024 * 1024, // 2MB
        MaxCallRecvMsgSize:   2 * 1024 * 1024, // 2MB
    },
    Prefix: "/locks/",
}
```

## 📊 特性对比

| 特性     | Redis 实现      | Etcd 实现   | Etcd Mutex 实现 |
|--------|---------------|-----------|---------------|
| 原子性    | ✅ SETNX + Lua | ✅ 事务      | ✅ Mutex       |
| TTL 支持 | ✅             | ✅         | ✅             |
| 锁续期    | ✅             | ✅         | ✅             |
| 重试机制   | ✅             | ✅         | ✅             |
| 性能     | 高             | 中         | 中             |
| 一致性    | 最终一致          | 强一致       | 强一致           |
| 配置兼容   | ✅ 标准 Redis    | ✅ 标准 Etcd | ✅ 标准 Etcd     |
| 唯一标识   | ✅             | ✅         | ✅             |
| 上下文隔离  | ✅             | ✅         | ✅             |
| 线程安全   | ✅             | ✅         | ✅             |
| 自动续期   | ✅             | ✅         | ✅             |
| 锁信息查询 | ✅             | ✅         | ✅             |

## 🔒 安全特性

1. **唯一标识**: 每个锁都有随机生成的唯一标识符，防止误删其他进程的锁
2. **TTL 保护**: 自动过期机制防止死锁
3. **原子操作**: 使用数据库的原子操作确保操作的原子性
4. **上下文隔离**: 不同上下文的锁操作相互隔离
5. **连接池**: 复用数据库连接，提高性能
6. **线程安全**: 使用 `sync.Map` 确保并发安全

## 📈 性能考虑

1. **连接池**: 复用数据库连接，减少连接开销
2. **Lua 脚本**: Redis 实现使用 Lua 脚本减少网络往返
3. **批量操作**: 优化数据库操作，提高吞吐量
4. **配置优化**: 可调整的重试策略和超时设置
5. **自动续期**: 支持长时间任务的自动续期，避免锁过期

## ⚠️ 注意事项

1. **锁的释放**: 使用 `WithLock` 或 `TryWithLock` 方法确保锁自动释放
2. **TTL 设置**: 根据业务逻辑合理设置 TTL，避免锁过早过期
3. **重试策略**: 根据网络环境和业务需求调整重试次数和延迟
4. **连接管理**: 及时关闭锁连接，避免资源泄漏
5. **自动续期**: 对于长时间任务，建议使用自动续期功能
6. **错误处理**: 始终检查锁操作的错误，特别是在生产环境中

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

本项目采用 `BSD 3-Clause License` 许可证 - 详见 [LICENSE](LICENSE) 文件