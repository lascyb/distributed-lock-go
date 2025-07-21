# 分布式锁 Go 实现

一个功能完整的 Go 分布式锁库，支持 Redis 和 etcd 作为后端存储，提供排他锁和共享锁两种锁类型。

## 🚀 特性

### 锁类型支持
- **排他锁 (Exclusive Lock)**: 传统的互斥锁，同一时间只能有一个客户端持有
  - 💡 **推荐场景**: 数据库写操作、文件写入、订单处理、库存扣减、状态机转换
- **共享锁 (Shared Lock)**: 读锁，多个客户端可以同时持有，但与排他锁互斥
  - 💡 **推荐场景**: 数据库读操作、配置文件读取、缓存查询、报表生成、日志分析

### 后端存储支持
- **Redis**: 高性能、低延迟，基于 Lua 脚本确保原子性
  - 💡 **推荐场景**: 高频交易系统、实时游戏、秒杀活动、API 限流、缓存更新协调
- **etcd**: 强一致性、高可用，基于事务和租约机制
  - 💡 **推荐场景**: 微服务配置管理、服务发现、分布式任务调度、集群选主、关键业务流程
- **etcd Mutex**: 基于 etcd concurrency 包的简化实现
  - 💡 **推荐场景**: 简单的资源互斥、快速原型开发、轻量级分布式协调

### 核心功能
- ✅ 非阻塞锁获取 (`TryLock`)
- ✅ 阻塞锁获取 (`Lock`) 
- ✅ 锁续期 (`Renew`)
- ✅ 锁状态检查 (`IsLocked`)
- ✅ 自动过期 (TTL)
- ✅ 重试机制
- ✅ 并发安全
- ✅ 连接池管理

## 📦 安装

```bash
go get github.com/lascyb/distributed-lock-go
```

## 🔧 快速开始

### Redis 分布式锁

```go
package main

import (
    "context"
    "time"
    
    "github.com/redis/go-redis/v9"
    distributedlock "github.com/lascyb/distributed-lock-go"
)

func main() {
    // 创建 Redis 客户端
    client := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })
    defer client.Close()
    
    // 创建分布式锁
    lock := distributedlock.NewRedisCombinedLock(client, "myapp:")
    
    ctx := context.Background()
    
    // 排他锁示例
    exclusiveOpts := &distributedlock.LockOptions{
        TTL:        30 * time.Second,
        RetryCount: 3,
        RetryDelay: 100 * time.Millisecond,
        LockType:   distributedlock.LockTypeExclusive,
    }
    
    acquired, err := lock.TryLock(ctx, "resource1", exclusiveOpts)
    if err != nil {
        panic(err)
    }
    
    if acquired {
        // 执行业务逻辑
        defer lock.Unlock(ctx, "resource1")
        // ... 业务代码
    }
    
    // 共享锁示例
    sharedOpts := &distributedlock.LockOptions{
        TTL:        30 * time.Second,
        RetryCount: 3,
        RetryDelay: 100 * time.Millisecond,
        LockType:   distributedlock.LockTypeShared,
    }
    
    acquired, err = lock.TryLock(ctx, "resource2", sharedOpts)
    if err != nil {
        panic(err)
    }
    
    if acquired {
        // 执行读操作
        defer lock.Unlock(ctx, "resource2")
        // ... 读取数据
    }
}
```

### etcd 分布式锁

```go
package main

import (
    "context"
    "time"
    
    clientv3 "go.etcd.io/etcd/client/v3"
    distributedlock "github.com/lascyb/distributed-lock-go"
)

func main() {
    // 创建 etcd 客户端
    client, err := clientv3.New(clientv3.Config{
        Endpoints:   []string{"localhost:2379"},
        DialTimeout: 5 * time.Second,
    })
    if err != nil {
        panic(err)
    }
    defer client.Close()
    
    // 创建分布式锁
    lock := distributedlock.NewEtcdCombinedLock(client, "/myapp")
    
    ctx := context.Background()
    opts := &distributedlock.LockOptions{
        TTL:        30 * time.Second,
        RetryCount: 3,
        RetryDelay: 100 * time.Millisecond,
        LockType:   distributedlock.LockTypeExclusive,
    }
    
    if err := lock.Lock(ctx, "resource", opts); err != nil {
        panic(err)
    }
    defer lock.Unlock(ctx, "resource")
    
    // 执行业务逻辑
}
```

## 🏗️ 架构设计

### 组件结构

```
分布式锁库
├── 核心接口层
│   ├── DistributedLock      # 统一锁接口
│   ├── LockOptions          # 锁配置选项
│   └── LockType            # 锁类型枚举
│
├── Redis 实现层
│   ├── RedisExclusiveLock  # Redis 排他锁
│   ├── RedisSharedLock     # Redis 共享锁
│   └── RedisCombinedLock   # Redis 组合锁（主入口）
│
├── etcd 实现层
│   ├── EtcdExclusiveLock   # etcd 排他锁
│   ├── EtcdSharedLock      # etcd 共享锁
│   ├── EtcdCombinedLock    # etcd 组合锁（主入口）
│   └── EtcdMutexLock       # etcd Mutex 锁
│
└── 工厂层
    └── Factory Methods     # 统一创建接口
```

### 锁类型说明

#### 排他锁 (Exclusive Lock)
- 同一时间只能有一个客户端持有
- 适用于写操作、资源独占访问
- 与其他所有锁类型互斥
- 💡 **典型应用**: 
  - 电商库存扣减（防止超卖）
  - 银行账户转账（确保余额一致性）
  - 分布式ID生成器
  - 单例服务实例控制

#### 共享锁 (Shared Lock)  
- 多个客户端可以同时持有
- 适用于读操作、数据查询
- 共享锁之间不互斥，但与排他锁互斥
- 💡 **典型应用**:
  - 多个服务同时读取配置文件
  - 并发查询用户信息
  - 多个实例同时访问只读缓存
  - 报表系统并发数据分析

### 互斥规则

| 当前锁类型 | 请求锁类型 | 结果 |
|-----------|-----------|------|
| 无锁      | 排他锁    | ✅ 允许 |
| 无锁      | 共享锁    | ✅ 允许 |
| 排他锁    | 排他锁    | ❌ 拒绝 |
| 排他锁    | 共享锁    | ❌ 拒绝 |
| 共享锁    | 排他锁    | ❌ 拒绝 |
| 共享锁    | 共享锁    | ✅ 允许 |

## 📖 详细 API

### 核心接口

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

### 锁配置选项

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

### 创建锁实例

#### 直接创建

```go
// Redis 组合锁
redisLock := distributedlock.NewRedisCombinedLock(client, "myapp:")

// etcd 组合锁
etcdLock := distributedlock.NewEtcdCombinedLock(client, "/myapp")

// etcd Mutex 锁（仅支持排他锁）
mutexLock := distributedlock.NewEtcdMutexLock(client, "/mutex")
```

#### 工厂方法创建

```go
// Redis
redisConfig := &distributedlock.RedisConfig{
    Options: &redis.Options{Addr: "localhost:6379"},
    Prefix:  "myapp:",
}
lock, err := distributedlock.NewDistributedLock(
    distributedlock.BackendTypeRedis, 
    redisConfig,
)

// etcd
etcdConfig := &distributedlock.EtcdConfig{
    Config: &clientv3.Config{
        Endpoints: []string{"localhost:2379"},
    },
    Prefix: "/myapp",
}
lock, err := distributedlock.NewDistributedLock(
    distributedlock.BackendTypeEtcd,
    etcdConfig,
)

// etcd Mutex
lock, err := distributedlock.NewDistributedLock(
    distributedlock.BackendTypeEtcdMutex,
    etcdConfig,
)
```

### 获取锁信息

```go
// 获取详细锁信息（Redis/etcd 组合锁支持）
if combinedLock, ok := lock.(*distributedlock.RedisCombinedLock); ok {
    info, err := combinedLock.GetLockInfo(ctx, "resource")
    if err == nil {
        fmt.Printf("锁类型: %v\n", info["lock_type"])
        fmt.Printf("是否被持有: %v\n", info["is_locked"])
        if info["lock_type"] == "shared" {
            fmt.Printf("持有者数量: %v\n", info["holder_count"])
            fmt.Printf("持有者列表: %v\n", info["holders"])
        }
    }
}
```

## 🏆 技术实现

### Redis 实现原理

#### 排他锁
- 使用 `SETNX` 命令原子性设置锁
- 使用 Lua 脚本确保解锁原子性
- 支持 TTL 自动过期

#### 共享锁
- 使用计数器追踪持有者数量
- 使用 Redis Set 存储持有者 ID
- 使用 Lua 脚本确保操作原子性
- 通过键名前缀实现与排他锁的互斥检查

### etcd 实现原理

#### 排他锁
- 使用事务 (Transaction) 确保原子性
- 使用租约 (Lease) 实现 TTL
- 基于键的创建版本判断锁状态

#### 共享锁
- 使用事务确保操作原子性
- 使用租约实现 TTL
- 通过键值对存储计数器和持有者信息
- 通过键名空间隔离实现与排他锁的互斥

## 🎯 使用场景

### 排他锁适用场景
- **数据写入**: 确保同一时间只有一个进程修改数据
  - 💼 用户余额变更、订单状态更新、商品库存调整
- **资源独占**: 文件处理、设备访问等需要独占的场景
  - 💼 日志文件轮转、硬件设备控制、临时文件生成
- **关键区域**: 保护不可重入的代码段
  - 💼 单例模式初始化、全局计数器更新、系统配置修改
- **状态变更**: 确保状态转换的原子性
  - 💼 工作流状态机、订单支付流程、用户认证状态

### 共享锁适用场景
- **数据读取**: 多个进程同时读取数据，提高并发性
  - 💼 用户信息查询、商品目录浏览、历史订单查看
- **配置获取**: 多个服务同时读取配置信息
  - 💼 系统配置热加载、服务发现信息获取、特性开关查询
- **缓存访问**: 并发读取缓存数据
  - 💼 热点数据缓存、用户会话信息、静态资源访问
- **只读资源**: 访问不会被修改的资源
  - 💼 数据报表生成、日志分析处理、监控指标收集

## 📊 性能特性

### Redis 后端
- **优势**: 极高性能，亚毫秒级延迟，丰富的数据结构
- **适用**: 高并发场景，对性能要求极高的应用
- **限制**: 需要 Redis 服务支持
- 💡 **推荐场景**:
  - 电商秒杀系统锁定商品
  - 游戏服务器资源竞争
  - 高频API接口限流
  - 实时数据处理管道

### etcd 后端
- **优势**: 强一致性，高可用性，分布式友好
- **适用**: 微服务架构，对一致性要求高的场景
- **限制**: 延迟相对较高（毫秒级）
- 💡 **推荐场景**:
  - Kubernetes 集群资源协调
  - 微服务配置变更同步
  - 分布式定时任务调度
  - 服务注册与发现协调

### etcd Mutex 后端
- **优势**: 实现简单，基于成熟的 concurrency 包
- **适用**: 简单的排他锁场景，快速集成
- **限制**: 仅支持排他锁
- 💡 **推荐场景**:
  - 简单的分布式任务互斥
  - 快速原型验证
  - 小规模服务协调
  - 学习和测试环境

## 🛠️ 最佳实践

### 1. 选择合适的锁类型
```go
// 💰 电商库存扣减 - 使用排他锁防止超卖
writeOpts := &LockOptions{LockType: LockTypeExclusive}
lock.Lock(ctx, "product:123:stock", writeOpts)

// 📊 用户信息查询 - 使用共享锁提高并发
readOpts := &LockOptions{LockType: LockTypeShared}
lock.Lock(ctx, "user:123:profile", readOpts)

// 🏦 银行转账 - 排他锁确保资金安全
transferOpts := &LockOptions{LockType: LockTypeExclusive}
lock.Lock(ctx, "account:transfer:456", transferOpts)
```

### 2. 设置合理的 TTL
```go
// ⚡ 秒杀活动 - 短TTL快速释放
seckillOpts := &LockOptions{
    TTL: 5 * time.Second,  // 秒杀场景需要快速处理
}

// 📈 报表生成 - 长TTL支持复杂计算
reportOpts := &LockOptions{
    TTL: 10 * time.Minute,  // 报表生成可能需要较长时间
}

// 🔄 常规业务 - 标准TTL平衡性能与安全
normalOpts := &LockOptions{
    TTL: 30 * time.Second,  // 大多数业务场景的合理时间
}
```

### 3. 处理锁获取失败
```go
// 🛒 商品库存检查
acquired, err := lock.TryLock(ctx, "product:123:stock", opts)
if err != nil {
    return fmt.Errorf("lock system error: %w", err)
}
if !acquired {
    // 库存正被其他用户操作，返回友好提示
    return errors.New("商品库存正在更新中，请稍后重试")
}

// 🎫 限量活动参与
acquired, err = lock.TryLock(ctx, "activity:limited:join", opts)
if !acquired {
    // 活动锁定中，执行降级逻辑
    return errors.New("活动火爆，请稍后再试或选择其他商品")
}
```

### 4. 使用 defer 确保锁释放
```go
if err := lock.Lock(ctx, key, opts); err != nil {
    return err
}
defer func() {
    if err := lock.Unlock(ctx, key); err != nil {
        log.Printf("unlock failed: %v", err)
    }
}()
```

### 5. 锁续期处理
```go
// 长时间运行的任务需要续期
go func() {
    ticker := time.NewTicker(opts.TTL / 3)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            if err := lock.Renew(ctx, key, opts.TTL); err != nil {
                log.Printf("renew failed: %v", err)
                return
            }
        case <-done:
            return
        }
    }
}()
```

## 🔍 示例程序

查看 `examples/shared_lock_demo.go` 获取完整的使用示例，包括：
- 排他锁的互斥演示
- 共享锁的并发演示  
- 排他锁与共享锁的互斥演示
- 锁信息获取演示

运行示例：
```bash
go run examples/shared_lock_demo.go
```

## 📄 许可证

本项目基于 BSD 3-Clause License 许可证开源，详见 [LICENSE](LICENSE) 文件。