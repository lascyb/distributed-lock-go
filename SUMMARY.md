# 分布式锁实现总结

## 概述

在根目录下实现了完整的分布式锁，包括：

1. **接口定义** (`interface.go`) - 统一的分布式锁接口和基础类型
2. **Redis 实现** (`redis.go`) - 基于 Redis 的分布式锁实现
3. **Etcd 实现** (`etcd.go`) - 基于 Etcd 的分布式锁实现（两种方式）
4. **工厂函数** (`factory.go`) - 工厂函数和配置管理
5. **服务包装器** (`service.go`) - 服务包装器和管理器
6. **测试代码** (`example_test.go`, `distributedlock_test.go`) - 完整的测试用例和示例
7. **文档** (`README.md`, `PACKAGE_SUMMARY.md`) - 详细文档

## 文件结构

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
├── PACKAGE_SUMMARY.md          # 包发布总结
├── SUMMARY.md                  # 本总结文档
├── LICENSE                     # 许可证文件
```

## 核心功能

### 1. 统一的分布式锁接口

```go
type DistributedLock interface {
    TryLock(ctx context.Context, key string, opts *LockOptions) (bool, error)
    Lock(ctx context.Context, key string, opts *LockOptions) error
    Unlock(ctx context.Context, key string) error
    Renew(ctx context.Context, key string, ttl time.Duration) error
    IsLocked(ctx context.Context, key string) (bool, error)
    Close() error
}
```

### 2. 支持的后端存储

- **Redis**: 基于 Redis 的分布式锁实现
  - 使用 `SET key value NX EX ttl` 命令实现原子性加锁
  - 使用 Lua 脚本确保只有锁持有者才能解锁
  - 使用 Lua 脚本实现原子性锁续期
  - 每个锁都有随机生成的唯一标识符
  - 使用 `sync.Map` 管理锁值，确保并发安全

- **Etcd**: 基于 Etcd 的分布式锁实现（两种实现方式）
  - **基于事务的实现**: 使用 Etcd 事务和租约机制
  - **基于 Mutex 的实现**: 使用 Etcd concurrency 包实现
  - 强一致性保证，适合对一致性要求高的场景
  - 使用 `sync.Map` 管理锁值和租约ID

### 3. 锁选项配置

```go
type LockOptions struct {
    TTL         time.Duration // 锁的生存时间
    RetryCount  int           // 重试次数
    RetryDelay  time.Duration // 重试延迟
    LockType    LockType      // 锁类型
}

type LockType int

const (
    LockTypeExclusive LockType = iota // 排他锁 {0:排他锁}
    LockTypeShared                    // 共享锁 {1:共享锁}
)
```

### 4. 便捷的服务包装器

- `LockService`: 提供便捷的锁操作接口
  - `WithLock`: 阻塞式锁操作
  - `TryWithLock`: 非阻塞式锁操作
  - `IsLocked`: 检查锁状态
  - `Renew`: 锁续期
  - `GetLockInfo`: 获取锁信息
  - `AutoRenew`: 自动续期功能

- `LockManager`: 支持多种锁类型的统一管理器
  - 支持注册多个锁服务
  - 统一的管理接口
  - 自动资源清理
  - 支持通过服务名续期锁
  - 支持检查指定服务的锁状态

## 技术实现细节

### Redis 分布式锁实现

```go
// 加锁：使用 SETNX 命令
result, err := r.client.SetNX(ctx, lockKey, lockValue, opts.TTL).Result()

// 解锁：使用 Lua 脚本确保原子性
script := `
    if redis.call("get", KEYS[1]) == ARGV[1] then
        return redis.call("del", KEYS[1])
    else
        return 0
    end
`

// 续期：使用 Lua 脚本
script := `
    if redis.call("get", KEYS[1]) == ARGV[1] then
        return redis.call("expire", KEYS[1], ARGV[2])
    else
        return 0
    end
`
```

### Etcd 分布式锁实现

```go
// 加锁：使用事务和租约
lease, err := e.client.Grant(ctx, int64(opts.TTL.Seconds()))
txn := e.client.Txn(ctx)
txn.If(clientv3.Compare(clientv3.CreateRevision(lockKey), "=", 0)).
    Then(clientv3.OpPut(lockKey, lockValue, clientv3.WithLease(lease.ID))).
    Else(clientv3.OpGet(lockKey))

// 解锁：使用事务
txn := e.client.Txn(ctx)
txn.If(clientv3.Compare(clientv3.Value(lockKey), "=", lockValue)).
    Then(clientv3.OpDelete(lockKey)).
    Else(clientv3.OpGet(lockKey))

// 续期：使用 KeepAliveOnce
_, err := e.client.KeepAliveOnce(ctx, leaseID)
```

## 使用方式

### 基本使用

```go
// 创建 Redis 锁
redisConfig := &distributedlock.RedisConfig{
    Options: &redis.Options{
        Addr:     "localhost:6379",
        Password: "",
        DB:       0,
        PoolSize: 10,
    },
    Prefix: "lock:",
}

redisLock, err := distributedlock.NewRedisDistributedLock(redisConfig)
if err != nil {
    log.Fatal(err)
}
defer redisLock.Close()

// 创建锁服务
lockService := distributedlock.NewLockService(redisLock, nil)

// 使用锁
ctx := context.Background()
err = lockService.WithLock(ctx, "my-resource", func() error {
    // 临界区代码
    return nil
})
```

### 高级功能使用

```go
// 自动续期
stopChan := make(chan struct{})
go func() {
    err := lockService.AutoRenew(ctx, "my-resource", 20*time.Second, stopChan)
    if err != nil {
        log.Printf("自动续期失败: %v", err)
    }
}()

// 获取锁信息
info, err := lockService.GetLockInfo(ctx, "my-resource")
if err != nil {
    log.Printf("获取锁信息失败: %v", err)
} else {
    log.Printf("锁信息: %+v", info)
}

// 使用锁管理器
manager := distributedlock.NewLockManager()
manager.RegisterService("redis", redisService)
manager.RegisterService("etcd", etcdService)

err = manager.WithLock("redis", ctx, "resource-1", func() error {
    // 使用 Redis 锁保护的代码
    return nil
})

// 检查锁状态
isLocked, err := manager.IsLocked("redis", ctx, "resource-1")
if err != nil {
    log.Printf("检查锁状态失败: %v", err)
} else {
    log.Printf("锁状态: %v", isLocked)
}
```

### 使用锁管理器

```go
// 创建锁管理器
manager := distributedlock.NewLockManager()
defer manager.Close()

// 注册多种锁服务
manager.RegisterService("redis", redisService)
manager.RegisterService("etcd", etcdService)

// 使用不同的锁服务
err = manager.WithLock("redis", ctx, "resource-1", func() error {
    // 使用 Redis 锁保护的代码
    return nil
})
```

## 特性

1. **原子性**: 使用 Redis 的 SETNX 和 Lua 脚本，Etcd 的事务机制确保操作的原子性
2. **TTL 支持**: 自动过期机制，防止死锁
3. **重试机制**: 可配置的重试次数和延迟
4. **锁续期**: 支持长时间任务的锁续期，包括手动续期和自动续期
5. **上下文传递**: 锁信息存储在上下文中，确保正确的解锁
6. **错误处理**: 完善的错误处理和日志记录
7. **连接管理**: 自动连接测试和资源清理
8. **配置兼容**: 直接嵌入标准客户端配置，无需额外学习成本
9. **线程安全**: 使用 `sync.Map` 确保并发安全
10. **自动续期**: 支持长时间任务的自动续期功能
11. **锁信息查询**: 提供锁状态和配置信息查询

## 安全特性

1. **唯一标识**: 每个锁都有随机生成的唯一标识符，防止误删其他进程的锁
2. **TTL 保护**: 自动过期机制，即使进程崩溃也能自动释放锁
3. **原子操作**: 使用数据库的原子操作确保锁的正确性
4. **上下文隔离**: 不同上下文的锁操作相互隔离
5. **连接池**: 复用数据库连接，提高性能
6. **线程安全**: 使用 `sync.Map` 确保并发安全

## 性能考虑

1. **连接池**: 复用数据库连接，减少连接开销
2. **Lua 脚本**: Redis 实现使用 Lua 脚本减少网络往返
3. **批量操作**: 优化数据库操作，提高吞吐量
4. **配置优化**: 可调整的重试策略和超时设置
5. **自动续期**: 支持长时间任务的自动续期，避免锁过期

## 特性对比

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

## 依赖

- `github.com/redis/go-redis/v9` - Redis 客户端
- `go.etcd.io/etcd/client/v3` - Etcd 客户端

这些依赖已经在项目的 `go.mod` 文件中存在。

## 使用建议

1. **选择合适的后端**: 
   - Redis: 适合高并发、低延迟场景
   - Etcd: 适合需要强一致性的场景

2. **合理设置 TTL**: 
   - 根据任务执行时间设置合适的 TTL
   - 对于长时间任务，使用锁续期机制

3. **错误处理**: 
   - 始终检查锁操作的错误
   - 在生产环境中添加适当的日志记录

4. **资源管理**: 
   - 记得调用 `Close()` 方法释放资源
   - 使用 `defer` 确保资源正确释放

5. **自动续期**: 
   - 对于长时间任务，建议使用自动续期功能
   - 合理设置续期间隔，避免频繁续期

6. **线程安全**: 
   - 在并发环境下使用，确保线程安全
   - 使用 `sync.Map` 管理锁值

## 扩展性

这个实现具有良好的扩展性：

1. **接口设计**: 可以轻松添加新的后端实现
2. **配置灵活**: 支持多种配置选项
3. **服务抽象**: 通过服务包装器提供统一的接口
4. **管理器模式**: 支持多种锁类型的统一管理
5. **自动续期**: 支持长时间任务的自动续期
6. **锁信息查询**: 提供锁状态和配置信息查询

## 技术亮点

1. **统一接口**: 提供统一的分布式锁接口，支持多种后端
2. **配置兼容**: 直接嵌入标准客户端配置，无需额外学习
3. **安全可靠**: 使用唯一标识和原子操作确保安全性
4. **性能优化**: 连接池、Lua 脚本等优化手段
5. **易用性**: 提供便捷的服务包装器和管理器
6. **自动续期**: 支持长时间任务的自动续期功能
7. **线程安全**: 使用 `sync.Map` 确保并发安全

您可以在其他代码中直接使用这个分布式锁包，它提供了完整的分布式锁功能，支持 Redis 和 Etcd 两种后端，并且具有良好的易用性和扩展性。 