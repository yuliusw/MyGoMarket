# Go 并发与工程化拷打点

本文只整理当前项目真实用到的 Go 语言点，重点覆盖 `channel`、`mutex`、`event`、`select`、`context`、`goroutine`、`sync.Once`、`atomic`、`defer`、`panic/recover`、数据库事务和常见面试追问。

## 1. 总览：项目里哪里用了什么

| 能力 | 主要位置 | 用途 |
| --- | --- | --- |
| channel | `main.go`、`common/audit/audit.go`、`common/middleware/request_pool.go` | 服务启动错误传递、优雅退出同步、审计事件队列、请求池 done 同步 |
| select | `main.go`、`common/audit/audit.go`、`common/queue/kafka/kafka.go` | 多路等待、退出信号、ticker 定时任务、非阻塞发送、消费循环取消 |
| mutex | `common/audit/audit.go`、`common/utils/token_bucket.go`、`common/middleware/rate_limit.go`、`common/middleware/broker.go`、`common/queue/*` | 保护共享状态、map、限流桶、熔断状态、MQ client map |
| RWMutex | `common/middleware/rate_limit.go`、`common/middleware/broker.go`、`common/queue/kafka/kafka.go`、`common/queue/rocketmq/rocketmq.go` | 读多写少场景，降低读路径锁竞争 |
| sync.Once | `common/utils/pool/gopool.go`、`common/queue/kafka/kafka.go`、`common/queue/rocketmq/rocketmq.go`、生成的 pb.go | 单例初始化，保证只初始化一次 |
| atomic.Value | `common/utils/lock/optimistic.go` | 无锁读、CAS 乐观更新、版本快照 |
| goroutine | `main.go`、`common/audit/audit.go`、`common/audit/export.go`、`common/middleware/rate_limit.go`、`services/order/repository/order_repository.go` | 并发启动 HTTP/gRPC、后台审计、补偿 worker、CSV pipe 写入、MinIO 导出上传、outbox worker、限流清理 |
| event | `common/audit/audit.go`、`services/market/app/market.go`、`services/iam/app/role_auth.go` | 审计事件、角色权限变更事件、Market 关键操作事件 |
| context | 几乎所有 repository、HTTP handler、gRPC service、Redis/MinIO/MQ 调用 | 取消、超时、请求作用域、数据库/Redis/MinIO/MQ 调用链传递 |
| defer | 各种 `cancel()`、`Close()`、`Unlock()`、临时文件删除、panic recover | 资源释放、锁释放、上下文取消、清理临时文件 |
| panic/recover | `common/middleware/request_pool.go`、`common/utils/pool/gopool.go` | 请求池任务 panic 兜底、协程池 panic handler |

## 2. channel 用法

### 2.1 `main.go`：HTTP/gRPC 启动错误聚合

位置：`main.go:72`

```go
serverErr := make(chan error, 1)
```

调用链：

1. `main()` 创建 `serverErr`，缓冲区大小为 1。
2. `startGRPCServer(grpcServer, serverErr)` 启动 gRPC goroutine。
3. HTTP server 也在 goroutine 中启动。
4. 两个 goroutine 都可能向 `serverErr` 写入 `err` 或 `nil`。
5. 主 goroutine 用 `select` 等待 `serverErr` 或系统退出信号。

相关代码：

```go
serverErr := make(chan error, 1)
startGRPCServer(grpcServer, serverErr)
go func() {
    if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        serverErr <- err
        return
    }
    serverErr <- nil
}()
```

这里 channel 的作用：

- 把子 goroutine 的启动/运行错误传回主 goroutine。
- 避免 HTTP/gRPC 启动失败后主 goroutine 无感知。
- 用 channel 实现 goroutine 生命周期协作。

为什么是 `make(chan error, 1)`：

- 缓冲为 1，至少允许一个 server goroutine 在主 goroutine 尚未接收时完成发送。
- 如果无缓冲，发送方必须等接收方 ready，可能造成某些退出路径阻塞。
- 但由于这里 HTTP 和 gRPC 都可能发送，缓冲 1 只能容纳一个结果，第二个发送可能阻塞。

雷点：

- `serverErr` 同时接收 HTTP 和 gRPC 的正常退出 `nil`，任何一个 server 正常返回 nil 都可能让主 goroutine 从 `select` 返回，导致进程退出逻辑不完整。
- channel 缓冲区为 1，但有两个发送方，若主 goroutine不再接收，第二个发送方可能阻塞。
- 更稳的做法是把“启动失败”和“正常关闭”语义区分开，或者使用 `errgroup.WithContext` 管理多个 server 生命周期。

面试拷打：

- Q：为什么这里用 channel，而不是全局变量？
- A：channel 是 goroutine 间同步和通信原语，能安全传递错误并唤醒主 goroutine；全局变量还需要锁或原子操作，并且无法天然阻塞等待。
- Q：缓冲 channel 和无缓冲 channel 区别？
- A：无缓冲要求发送和接收同步 rendezvous；缓冲 channel 在容量未满时发送不阻塞，容量为空时接收阻塞。
- Q：这里 `chan<- error` 是什么？
- A：发送方向 channel 类型，只能发送不能接收，用于函数签名约束，`startGRPCServer(server *grpc.Server, serverErr chan<- error)` 明确该函数只负责汇报错误。

### 2.2 `main.go`：gRPC 优雅停止同步

位置：`main.go:171`

```go
stopped := make(chan struct{})
go func() {
    server.GracefulStop()
    close(stopped)
}()
select {
case <-stopped:
    log.Println("gRPC server stopped")
case <-ctx.Done():
    server.Stop()
    log.Println("gRPC server force stopped")
}
```

调用链：

1. 优雅退出时调用 `gracefulShutdown()`。
2. `gracefulShutdown()` 构造带超时的 context。
3. `stopGRPCServer(ctx, grpcServer)` 启动一个 goroutine 执行 `GracefulStop()`。
4. `GracefulStop()` 返回后关闭 `stopped` channel。
5. 主 goroutine 用 `select` 等待 stopped 或 ctx 超时。
6. 超时则调用 `server.Stop()` 强制停止。

为什么用 `chan struct{}`：

- 这里只需要通知“完成”，不需要传递数据。
- `struct{}` 零大小，不携带额外内存语义。
- `close(stopped)` 可以广播完成信号，所有 `<-stopped` 都会立即返回。

知识点：

- 关闭 channel 是一种广播通知方式。
- 从已关闭 channel 接收会立即返回零值。
- 不应该向已关闭 channel 发送，会 panic。
- 一般由发送方关闭 channel，接收方不要关闭。

雷点：

- `GracefulStop()` 如果一直卡住，靠 context 超时走 `server.Stop()`。
- `server.Stop()` 和 `GracefulStop()` 并发调用需要确保 gRPC 实现允许该模式；通常这是官方推荐的超时兜底写法。

### 2.3 `common/audit/audit.go`：审计事件 channel

位置：`common/audit/audit.go:56`

```go
type Writer struct {
    db     *gorm.DB
    ch     chan Event
    done   chan struct{}
    closed bool
    mu     sync.Mutex
}
```

初始化：

```go
writer = &Writer{db: db, ch: make(chan Event, 1024), done: make(chan struct{})}
go writer.run()
```

调用链：

1. `main.go` 启动时调用 `audit.Start(database.DB)`。
2. `audit.Start` 创建全局 `Writer`。
3. `Writer` 内部有 `ch chan Event`，容量 1024。
4. `go writer.run()` 后台消费事件。
5. 业务代码调用 `audit.Emit(event)`。
6. `Emit` 用 `select` 非阻塞写入 `writer.ch`。
7. `writer.run()` 批量收集事件，达到 100 条或每 2 秒 flush 到 DB。
8. 进程退出时调用 `audit.Shutdown(ctx)`，关闭 `writer.ch`。
9. `run()` 发现 channel 关闭后 flush 剩余事件并退出，最后关闭 `done`。

Event 定义：

```go
type Event struct {
    EventType string
    TraceID   string
    ActorID   string
    Resource  string
    Metadata  map[string]interface{}
    Error     string
}
```

channel 设计目的：

- 业务请求不直接同步写审计 DB，降低主链路延迟。
- 批量写 DB，减少 insert 次数。
- channel 做生产者-消费者队列。

为什么容量是 1024：

- 能吸收短时间流量尖峰。
- 避免每次 `Emit` 都阻塞在 DB 写入上。
- 但容量有限，防止无限内存增长。

非阻塞发送：

```go
select {
case writer.ch <- event:
default:
    log.Printf("audit queue full, drop event=%s trace_id=%s", event.EventType, event.TraceID)
}
```

这段的含义：

- 如果 channel 未满，发送成功。
- 如果 channel 已满，走 `default`，直接丢弃审计事件。
- 不阻塞业务主流程。

雷点：

- 审计事件可能丢失，不适合强合规审计。
- 进程崩溃会丢失 channel 中尚未 flush 的事件。
- `Metadata map[string]interface{}` 如果调用方后续继续修改同一个 map，理论上可能造成数据竞争；更稳是 Emit 时深拷贝 metadata。
- 只有一个 writer goroutine，flush 慢时可能堆积。

面试拷打：

- Q：channel 满了为什么丢弃而不是阻塞？
- A：审计是旁路能力，不应拖垮核心业务链路；这是可用性优先的取舍。
- Q：如果审计不能丢，该怎么改？
- A：用 MQ、Outbox Pattern、同步事务内写审计表，或持久化本地 WAL；同时要做限流和告警。
- Q：关闭 channel 后还能读吗？
- A：可以，读出剩余缓冲数据；读完后继续读会返回零值和 `ok=false`。
- Q：关闭 channel 后还能写吗？
- A：不能，会 panic。

### 2.4 `common/audit/audit.go`：done channel

位置：`common/audit/audit.go:57`

```go
done chan struct{}
```

用途：

- 通知 `Shutdown(ctx)`：后台 writer 已经退出。
- `run()` 中 `defer close(w.done)`。
- `Shutdown` 用 `select` 等待 `writer.done` 或 `ctx.Done()`。

相关代码：

```go
select {
case <-writer.done:
case <-ctx.Done():
    log.Printf("audit shutdown timeout: %v", ctx.Err())
}
```

知识点：

- done channel 常用于 goroutine 退出通知。
- 用 `close(done)` 而不是 `done <- struct{}{}` 可以避免接收方数量问题。
- 配合 context 可以实现“等一段时间，等不到就放弃”。

### 2.5 `common/middleware/request_pool.go`：请求池 done channel

位置：`common/middleware/request_pool.go:33`

```go
done := make(chan struct{})
err := requestPool.Submit(func() {
    defer close(done)
    c.Next()
})
...
<-done
```

调用链：

1. HTTP 请求进入 `RequestPoolFastFail` 中间件。
2. 中间件创建 `done` channel。
3. 把实际 `c.Next()` 提交到 ants pool。
4. 池中 worker 执行业务 handler。
5. handler 执行结束后 `defer close(done)`。
6. 外层 goroutine `<-done` 等待处理完成。

设计目的：

- 控制同时执行的请求数量。
- ants pool 满时快速失败，返回 `503`。
- 外层必须等待业务处理完成，否则 Gin 请求生命周期会提前结束。

雷点：

- Gin 的 `Context` 在另一个 goroutine 里执行，即使外层同步等待，也属于敏感用法。
- 如果提交任务后任务内部永久阻塞，外层 `<-done` 也会一直阻塞，除非请求 context 或下游超时生效。
- 如果 `requestPool.Submit` 成功但任务 panic，代码通过 recover 写 500 并 close(done)，避免死锁。

## 3. select 用法

### 3.1 `main.go`：主协程等待 server 错误或退出信号

位置：`main.go:86`

```go
select {
case err := <-serverErr:
    if err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
case <-shutdownCtx.Done():
    gracefulShutdown(server, grpcServer)
}
```

select 作用：

- 同时等待多个 channel。
- 哪个先 ready 就执行哪个 case。
- 用于“服务异常退出”和“系统信号退出”两个事件竞争。

知识点：

- 如果多个 case 同时 ready，Go 会伪随机选择一个。
- 如果没有 case ready 且没有 default，select 阻塞。
- 如果有 default，select 不阻塞。

雷点：

- 如果收到 `serverErr <- nil`，代码不会 graceful shutdown，而是 `main()` 返回。
- 如果两个 server 都可能写 channel，需要考虑多个结果的收敛策略。

### 3.2 `audit.Emit`：非阻塞发送

位置：`common/audit/audit.go:80`

```go
select {
case writer.ch <- event:
default:
    log.Printf("audit queue full, drop event=%s trace_id=%s", event.EventType, event.TraceID)
}
```

拷打点：

- `default` 会让 select 变成非阻塞。
- 非阻塞发送常用于“尽力而为”的指标、日志、审计旁路。
- 如果是核心业务消息，不应该这么写，否则会静默丢数据。

### 3.3 `audit.run`：消费事件或定时 flush

位置：`common/audit/audit.go:192`

```go
for {
    select {
    case event, ok := <-w.ch:
        if !ok {
            flush()
            return
        }
        batch = append(batch, event)
        if len(batch) >= 100 {
            flush()
        }
    case <-ticker.C:
        flush()
    }
}
```

调用链：

- `w.ch` 有事件时消费。
- `ticker.C` 每 2 秒触发一次 flush。
- `w.ch` 被关闭时 flush 剩余数据并退出。

知识点：

- `ticker.C` 是一个只读 channel。
- `time.NewTicker` 必须 `defer ticker.Stop()`，否则泄露 runtime timer。
- channel 接收的第二个返回值 `ok` 用来判断 channel 是否关闭。

雷点：

- 如果事件持续高频，select 在事件 case 和 ticker case 之间调度不保证严格公平。
- flush 中 DB 写入如果慢，会阻塞整个 audit writer，期间无法消费新事件。

### 3.4 MinIO retry worker：退出或定时执行

位置：`common/audit/audit.go:125`

```go
for {
    select {
    case <-ctx.Done():
        return
    case <-ticker.C:
        runMinioDeleteRetries(ctx, db, minioClient)
    }
}
```

作用：

- 每分钟扫描补偿任务。
- 收到 cancel 后退出，避免 goroutine 泄露。

拷打点：

- 为什么不能 `for { time.Sleep(time.Minute); ... }`？
- 因为 `Sleep` 期间无法及时响应退出，ticker + select 能监听 `ctx.Done()`。

### 3.5 Kafka consumer：监听 ctx.Done

位置：`common/queue/kafka/kafka.go:90`

```go
for {
    select {
    case <-ctx.Done():
        return
    default:
        msg, err := reader.FetchMessage(ctx)
        ...
    }
}
```

作用：

- 外部 cancel 时停止消费，避免 goroutine 泄露。
- `FetchMessage(ctx)` 本身也会感知 context 取消。

雷点：

- `default` 会让 select 不阻塞，随后进入 `FetchMessage(ctx)` 阻塞。
- 如果 `FetchMessage` 因 ctx cancel 返回错误，当前代码会打印错误并 continue；下一轮 select 才会退出。
- 如果不是 context error 的临时错误，当前代码无限 continue，可能刷日志。

## 4. mutex / RWMutex 用法

### 4.1 `audit.Writer.mu`：保护 closed 状态

位置：`common/audit/audit.go:59`

```go
closed bool
mu     sync.Mutex
```

使用位置：

```go
writer.mu.Lock()
closed := writer.closed
writer.mu.Unlock()
if closed { return }
```

关闭位置：

```go
writer.mu.Lock()
if !writer.closed {
    writer.closed = true
    close(writer.ch)
}
writer.mu.Unlock()
```

为什么需要锁：

- `Emit` 可能被多个请求 goroutine 并发调用。
- `Shutdown` 可能同时修改 `closed` 并关闭 channel。
- 如果没有锁，读写 `closed` 是数据竞争。

雷点：

- 当前 `Emit` 读取 closed 后释放锁，再发送 channel。理论上可能出现：`Emit` 释放锁后，`Shutdown` 关闭 channel，然后 `Emit` 向已关闭 channel 发送，导致 panic。
- 更稳的做法是在锁内完成 closed 检查与发送，但发送可能阻塞；或者使用额外 recover；或者用 context/atomic 状态 + 专门关闭流程；或者不关闭 `ch`，用单独 stop channel 通知。

面试拷打：

- Q：锁保护了什么临界区？
- A：保护 `closed` 状态和关闭 channel 的一次性动作。
- Q：为什么 close channel 和 send channel 也要考虑并发？
- A：send on closed channel 会 panic，close 已关闭 channel 也会 panic。

### 4.2 `TokenBucket.mu`：令牌桶并发安全

位置：`common/utils/token_bucket.go:14`

```go
type TokenBucket struct {
    rate         float64
    capacity     float64
    tokens       float64
    lastActivity time.Time
    mu           sync.Mutex
}
```

Allow 调用链：

1. HTTP 限流中间件按 IP 获取 TokenBucket。
2. 每个请求调用 `bucket.Allow()`。
3. `Allow()` 加互斥锁。
4. 根据距离上次访问的时间补充令牌。
5. 如果令牌数 >= 1，扣 1 个令牌并放行。
6. 否则拒绝。

为什么用 Mutex：

- `tokens` 和 `lastActivity` 都会被多个请求并发读写。
- 补充令牌和扣令牌必须是一个原子临界区。
- 如果不加锁，可能并发扣出负数或覆盖 lastActivity。

雷点：

- `GetLastActivity()` 用 `mu.Lock()` 而不是 `RLock()`，因为 `TokenBucket` 只有 Mutex 没有 RWMutex。
- `Allow()` 中不要在持锁期间做慢 IO，否则会降低并发性能；当前只做内存计算，合理。

### 4.3 `IPRateLimiter.mu`：保护 IP -> TokenBucket map

位置：`common/middleware/rate_limit.go:15`

```go
ips map[string]*utils.TokenBucket
mu  sync.RWMutex
```

GetLimiter 双重检查：

```go
limiter, exists := i.ips[ip]
i.mu.RUnlock()

if !exists {
    i.mu.Lock()
    defer i.mu.Unlock()
    limiter, exists = i.ips[ip]
    if !exists {
        limiter = utils.NewTokenBucket(i.rate, i.capacity)
        i.ips[ip] = limiter
    }
}
```

为什么用 RWMutex：

- 大多数请求只是读取已有 IP 的 bucket。
- 只有新 IP 或清理时需要写锁。
- 读多写少场景 RWMutex 比 Mutex 更合适。

为什么要双重检查：

- 两个 goroutine 同时发现 IP 不存在。
- 第一个拿到写锁创建 bucket。
- 第二个拿到写锁后必须再检查一次，避免重复创建覆盖。

后台清理：

- `go limiter.cleanupStaleBuckets(cleanupInterval, ttl)`。
- 每个 ticker 周期加写锁遍历 map。
- 删除长时间未访问的 IP。

雷点：

- cleanup goroutine 没有 context 或 stop channel，创建后无法停止，测试或热重载场景可能泄露。
- 清理时持写锁遍历整个 map，IP 很多时会阻塞请求读取。

### 4.4 `CircuitBreaker.mu`：熔断状态机

位置：`common/middleware/broker.go:21`

共享状态：

- `state`
- `failureCount`
- `lastFailure`

Allow 读路径：

- 先 RLock 读取 `state` 和 `lastFailure`。
- 如果 open 且冷却时间到了，再 Lock 进入 half-open。

ReportResult 写路径：

- Lock。
- 失败时 failureCount++，达到阈值进入 open。
- 成功时重置 failureCount，状态回 closed。

雷点：

- HalfOpen 当前实现允许多个请求通过，注释也说“简化处理”。严格熔断通常只允许一个或少量探针请求。
- ReportResult 根据 HTTP 5xx 或 Gin errors 判断失败，不能覆盖所有业务失败语义。

### 4.5 MQ client 的 RWMutex + Once

位置：`common/queue/kafka/kafka.go`、`common/queue/rocketmq/rocketmq.go`

Kafka：

- `once sync.Once`：保证 `Init` 只初始化一次单例。
- `writers map[string]*kafka.Writer`：每个 topic 一个 writer。
- `mu sync.RWMutex`：保护 writers map。
- `getWriter` 用读锁查 map，不存在再写锁创建。

RocketMQ：

- `once sync.Once`：保证 MQClient 单例只初始化一次。
- `consumers map[string]rocketmq.PushConsumer`：保存消费者。
- `mu sync.RWMutex`：注册和关闭时保护 map。

雷点：

- RocketMQ 的 `producer` 字段写入没有加锁，如果 StartProducer 和 Send 并发调用可能有数据竞争；实际启动链路中是先启动 producer 再对外服务，风险较低。
- `sync.Once` 一旦执行过，即使初始化失败，也不会再次执行。Go 里 `Once` 不知道你的初始化是否成功。

## 5. sync.Once 用法

### 5.1 协程池单例

位置：`common/utils/pool/gopool.go`

```go
var (
    defaultPool *ants.Pool
    ioPool      *ants.Pool
    requestPool *ants.Pool
    defaultOnce sync.Once
    ioOnce      sync.Once
    requestOnce sync.Once
)
```

调用链：

- `InitGetDefaultPool()` 创建普通任务池。
- `InitGetIOPool()` 创建高并发 IO 池。
- `InitGetRequestPool(capacity)` 创建 HTTP 请求池。
- 每个池只初始化一次。

为什么用 Once：

- 多个 goroutine 同时获取池时，只允许创建一个池。
- 避免重复创建资源、重复 goroutine、重复配置。

拷打点：

- `sync.Once` 是并发安全的。
- `Do(f)` 中的 `f` 只会执行一次。
- 如果 `f` panic，Go 里该 Once 会被认为已经执行过，后续不会再执行。
- 如果初始化参数第一次传错，例如 request pool capacity 第一次传 100，后续传 1000 不会生效。

## 6. atomic.Value / CAS 用法

位置：`common/utils/lock/optimistic.go`

核心结构：

```go
type snapshot[T any] struct {
    data    T
    version int64
}

type Versioned[T any] struct {
    value atomic.Value
}
```

调用链：

1. `Pack(data)` 存入初始 snapshot，version = 1。
2. `Get()` 通过 `atomic.Value.Load()` 无锁读取当前快照。
3. `TryUpdate(oldVersion, fn)` 读取当前快照。
4. 校验版本是否等于调用方传入的 oldVersion。
5. 通过 fn 计算新数据。
6. 构建新 snapshot。
7. `CompareAndSwap(current, newSnapshot)`，只有当前指针没变才替换。
8. `UpdateWithRetry` 失败后重新读取版本并重试。

知识点：

- atomic.Value 适合读多写少的配置/快照类数据。
- 通过不可变 snapshot 避免读写同一对象。
- CAS 是 compare-and-swap，典型乐观锁。
- 乐观锁适合冲突少的场景；冲突多时自旋重试成本高。

雷点：

- `data T` 如果是 map/slice/pointer，虽然 snapshot 指针不可变，但内部数据仍可能被外部修改。要做到真正 immutable，需要深拷贝。
- atomic.Value 要求存储的具体类型一致。
- CAS 比 mutex 难读，不要为了炫技滥用。

面试拷打：

- Q：atomic 和 mutex 怎么选？
- A：简单计数/状态可用 atomic；复杂临界区、多字段一致性优先 mutex；读多写少快照可 atomic.Value；不要用 atomic 拼复杂业务事务。

## 7. event 事件模型

### 7.1 审计事件 Event

位置：`common/audit/audit.go`

Event 字段含义：

- `EventType`：事件类型，如 `publish_succeeded`、`role_permissions_updated`。
- `TraceID`：链路追踪 ID。
- `ActorID`：操作者用户 ID。
- `Resource`：资源 ID，如 app_id、role_id。
- `Metadata`：事件扩展信息。
- `Error`：失败原因。

落库表：`audit_events`。

### 7.2 Market 事件

位置：`services/market/app/market.go:675`

事件包括：

- `publish_idempotent_returned`
- `publish_lock_failed`
- `publish_temp_save_failed`
- `publish_file_type_rejected`
- `publish_checksum_failed`
- `publish_minio_upload_failed`
- `publish_db_failed_compensation_failed`
- `publish_db_create_failed`
- `publish_succeeded`
- `delete_minio_remove_failed`

调用链：

1. Market handler 处理发布/删除等操作。
2. 调用 `logMarketAudit(...)`。
3. 构造 `audit.Event`。
4. 调用 `audit.Emit(auditEvent)`。
5. audit writer 异步批量落库。

### 7.3 IAM 角色权限事件

位置：`services/iam/app/role_auth.go:94`

事件包括：

- `role_permissions_update_failed`
- `role_permissions_updated`

调用链：

1. 管理员调用 `PUT /api/v1/iam/roles/:role_id/permissions`。
2. `ReplaceRolePermissions` 读取 before。
3. 替换角色权限。
4. 成功后 `purgeCasbinAndBroadcast()`。
5. 调用 `emitRolePermissionAudit`。
6. 审计事件写入异步 channel。

### 7.4 Event 和 MQ 消息的区别

- `audit.Event` 是审计事件，目的是记录发生了什么。
- `CasbinSyncMessage` 是业务同步消息，目的是通知其他实例清缓存。
- 审计事件当前是进程内 channel，可能丢。
- RocketMQ 消息是外部消息队列，跨实例分发。

面试拷打：

- Q：事件、消息、日志有什么区别？
- A：日志面向排障，人读为主；事件是结构化事实，可查询和审计；消息面向系统间通信，有消费语义和可靠性要求。

## 8. context 用法

### 8.1 项目里 context 的主要来源

- HTTP 请求：`c.Request.Context()`。
- gRPC 请求：方法参数 `ctx context.Context`。
- 系统退出：`signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)`。
- 超时控制：`context.WithTimeout(...)`。
- worker 停止：`context.WithCancel(...)`。
- 后台补偿/清理：部分使用 `context.Background()`。

### 8.2 HTTP handler 到 repository 的传递

典型链路：Market 列表。

```go
apps, total, err := appRepo.List(c.Request.Context(), req.Page, req.PageSize, req.Keyword, req.Category, req.Status)
```

意义：

- 客户端断开连接后，请求 context 会取消。
- DB 查询 `db.WithContext(ctx)` 能感知取消。
- 防止请求已经没意义但 DB 还继续跑。

### 8.3 gRPC context

位置：`services/admin/app/grpc_service.go`、`services/wallet/app/grpc_service.go`

gRPC 方法天然带 context：

```go
func (s *WalletService) GetWallet(ctx context.Context, req *walletv1.GetWalletRequest) (...)
```

意义：

- gRPC deadline、cancel、metadata 都通过 ctx 传递。
- 服务端 DB 查询应该使用 `db.WithContext(ctx)`。

### 8.4 初始化依赖时的超时 context

位置：`common/database/redis.go`、`common/database/minio.go`

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
```

用途：

- Redis Ping 最多等待 5 秒。
- MinIO BucketExists/MakeBucket 最多等待 5 秒。
- 避免启动阶段无限卡死。

### 8.5 Market 发布 DB 超时

位置：`services/market/app/market.go:229`

```go
dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
if err := appRepo.Create(dbCtx, newApp); err != nil { ... }
```

意义：

- 即使 HTTP 请求 context 没取消，DB 创建也最多 5 秒。
- 超时后进入补偿逻辑，删除已上传的 MinIO 对象。

雷点：

- `lookupCtx := context.WithTimeout(context.Background(), 5*time.Second)` 使用 Background 而非请求 ctx，是为了补偿/幂等查询不受客户端断开影响。
- Background 会脱离请求生命周期，必须自己设置 timeout，否则可能泄露。

### 8.6 RocketMQ 发送超时

位置：`common/queue/rocketmq/casbin.go:86`

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()
res, err := client.SendSync(ctx, CasbinSyncTopic, body)
```

意义：

- 权限缓存同步消息最多发送 3 秒。
- 避免 MQ 异常拖死业务请求。

### 8.7 context 拷打点

- context 是并发安全的，可以多个 goroutine 同时读取。
- context 应作为函数第一个参数，命名为 `ctx`。
- 不要把 context 存进 struct 里长期持有。
- 不要传 nil context，不知道传什么时用 `context.TODO()`。
- 请求链路优先传 `c.Request.Context()`。
- 后台任务用 `context.Background()` 派生，并设置 cancel 或 timeout。
- `WithTimeout` / `WithCancel` 返回的 cancel 必须调用，通常 `defer cancel()`。
- context 只放请求作用域数据，不放可选业务参数。
- `ctx.Done()` 是一个 channel，取消时会关闭。
- `ctx.Err()` 返回 `context.Canceled` 或 `context.DeadlineExceeded`。

## 9. goroutine 用法

### 9.1 启动 HTTP server

位置：`main.go:74`

HTTP server 在 goroutine 中启动，主 goroutine 留下来监听错误或信号。

### 9.2 启动 gRPC server

位置：`main.go:128`

gRPC server 在 goroutine 中 `Serve(listener)`。

### 9.3 gRPC graceful stop goroutine

位置：`main.go:172`

`GracefulStop()` 可能阻塞，所以放到 goroutine 中，再用 stopped channel 和 context 超时竞争。

### 9.4 审计 writer goroutine

位置：`common/audit/audit.go:67`

后台消费审计事件，批量写 DB。

### 9.5 MinIO retry worker goroutine

位置：`common/audit/audit.go:121`

后台定时扫描删除补偿任务。

### 9.6 CSV 导出 pipe goroutine

位置：`common/audit/export.go:113`

```go
httpReader, httpWriter := io.Pipe()
minioReader, minioWriter := io.Pipe()
writer := io.MultiWriter(httpWriter, minioWriter)
go func() {
    err := ExportCSV(c.Request.Context(), db, writer, filter)
    _ = httpWriter.CloseWithError(err)
    _ = minioWriter.CloseWithError(err)
}()
c.DataFromReader(http.StatusOK, -1, "text/csv; charset=utf-8", httpReader, headers)
```

当前项目实际有两条 pipe：

- `httpReader/httpWriter`：Gin 从 reader 读，导出 goroutine 往 writer 写，形成 HTTP 流式响应。
- `minioReader/minioWriter`：如果 MinIO 可用，导出内容通过 `io.MultiWriter` 同时写入 MinIO 上传 goroutine。

调用链：

1. HTTP 请求进入 CSV 导出 handler。
2. 创建 HTTP pipe 和 MinIO pipe。
3. goroutine 往 pipe writer 写 CSV。
4. Gin 从 pipe reader 流式返回给客户端。
5. MinIO goroutine 从另一条 pipe reader 读取并上传对象。
6. 写入结束或出错时 `CloseWithError(err)`。

可讲亮点：

- 不需要把整个 CSV 全部加载到内存。
- 支持边查边写边返回。
- 适合大文件导出。
- 结合 keyset cursor，每批只查 500 条，控制应用内存。
- 同一份 CSV 可同时给 HTTP 客户端和 MinIO 归档。

雷点：

- 如果客户端断开，`c.Request.Context()` 取消，DB 查询应停止。
- Pipe 读写双方必须正确关闭，否则可能 goroutine 泄露。
- `CloseWithError` 能把导出错误传递给 reader。
- `io.MultiWriter` 是同步写，任一 writer 慢都会拖慢导出；MinIO 上传慢可能影响 HTTP 响应速度。
- 当前是应用层 keyset cursor，不是 PostgreSQL `DECLARE CURSOR`。

面试追问：为什么不会 OOM？

- DB 查询每批 500 条，不会一次性加载全量数据。
- CSV writer 写入 pipe，pipe 没有无限缓存；下游读得慢时上游会阻塞，形成天然背压。
- HTTP 响应通过 reader 流式发送，不需要先生成完整文件。

面试追问：背压的代价是什么？

- 客户端慢或 MinIO 慢会让导出 goroutine 阻塞，单次导出占用连接和 goroutine 时间变长。
- 生产场景如果导出非常大，更适合做异步导出任务表，前端轮询状态，完成后下载 MinIO 文件。

### 9.7 Outbox worker goroutine

位置：`services/order/repository/order_repository.go`

并发控制层次：

1. DB 事务中查询 outbox：`FOR UPDATE SKIP LOCKED`。
2. 抢到的事件先更新为 `processing`。
3. 处理单个 order 前加 RedisLock：`entitlement:order:{order_id}`。
4. 发放订阅仍在 DB transaction 内锁订单行。
5. subscriptions 分区上的 `(user_id, app_id, source_order_id)` 唯一索引做最终防重。

为什么需要多层：

- `SKIP LOCKED` 解决多 worker 扫同一 outbox 表时的任务抢占。
- RedisLock 解决同一 order 的跨进程互斥，尤其是异常重试或重复事件场景。
- 唯一索引解决最终幂等，防止锁失效或重复处理导致重复发放权益。

面试追问：`SKIP LOCKED` 有什么风险？

- 它会跳过被其他事务锁住的行，吞吐更好，但不是严格公平队列。
- 如果某些行长期 processing 且没有恢复机制，会被正常扫描绕开，所以需要状态超时重置或告警。

### 9.8 限流清理 goroutine

位置：`common/middleware/rate_limit.go:55`

后台清理过期 IP token bucket。

雷点：

- 没有 stop 机制。
- 如果创建多个 limiter，就会启动多个清理 goroutine。

## 10. defer 用法

项目中典型 defer：

- `defer cancel()`：释放 context timer 资源。
- `defer logs.Log.Sync()`：进程退出前 flush zap 日志。
- `defer stop()`：停止 signal.NotifyContext。
- `defer ticker.Stop()`：释放 ticker。
- `defer close(w.done)`：通知 goroutine 退出。
- `defer os.Remove(tempFilePath)`：清理上传临时文件。
- `defer f.Close()`：关闭文件。
- `defer tb.mu.Unlock()`：保证锁释放。
- `defer unlock()`：释放 Redis 上传锁。
- `defer recover()`：请求池任务 panic 兜底。

拷打点：

- defer 是函数返回前执行，不是代码块结束执行。
- 多个 defer 后进先出。
- defer 参数会在注册 defer 时求值。
- 循环里大量 defer 可能导致资源延迟释放和额外开销。
- 加锁后紧跟 `defer Unlock()` 是常见安全写法，但长函数会扩大持锁时间，需要注意。

## 11. panic / recover 用法

### 11.1 请求池 recover

位置：`common/middleware/request_pool.go:36`

```go
defer func() {
    if recovered := recover(); recovered != nil {
        log.Printf("request pool task panic: %v", recovered)
        if !c.Writer.Written() {
            response.Abort(c, http.StatusInternalServerError, "REQUEST_POOL_PANIC", "internal server error")
        }
    }
}()
```

作用：

- 防止业务 handler panic 导致 worker goroutine 异常退出。
- 尝试返回 500。
- 因为 `defer close(done)` 在前面注册，panic 也能 close done，避免外层永久阻塞。

拷打点：

- recover 只有在 defer 中直接调用才有效。
- recover 只能恢复当前 goroutine 的 panic，不能跨 goroutine recover。
- panic 不应用于普通业务错误，应返回 error。

### 11.2 ants panic handler

位置：`common/utils/pool/gopool.go`

ants pool 初始化时配置 `WithPanicHandler`，用于捕获池内任务 panic 并记录日志。

雷点：

- request_pool 中既有任务内部 recover，也有 ants panic handler，实际 handler panic 会先被内部 recover 捕获。
- panic 后是否能安全继续处理请求，取决于 panic 发生时响应是否已经写出。

## 12. 数据库事务与锁也是 Go 面试拷打点

虽然这不是 Go 语法原语，但在 Go 服务端面试中经常和并发一起问。

### 12.1 钱包行级锁

位置：`services/wallet/repository/wallet_repository.go:278`

```go
tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("wallet_id = ?", walletID).First(&model)
```

作用：

- 在 DB transaction 内锁住钱包行。
- 防止并发扣款时两个事务读到同一个旧余额。

拷打点：

- Go 的 Mutex 只能保护单进程内并发，保护不了多实例部署。
- 钱包余额这种跨实例共享数据必须靠数据库锁或乐观锁。
- `SELECT FOR UPDATE` 必须在 transaction 内才有意义。

### 12.2 转账固定锁顺序

位置：`services/wallet/repository/wallet_repository.go:130`

```go
firstID, secondID := fromWalletID, toWalletID
if strings.Compare(firstID.String(), secondID.String()) > 0 {
    firstID, secondID = secondID, firstID
}
```

作用：

- A 转 B 和 B 转 A 并发时，都按同样顺序锁钱包。
- 避免事务 1 锁 A 等 B，事务 2 锁 B 等 A 的死锁。

### 12.3 订单行级锁

位置：`services/order/repository/order_repository.go:217`

支付订单时锁订单行，避免重复支付。

## 13. 项目里的 Go 常见拷打题与答案

### 13.1 channel 相关

Q：channel 底层是什么？

A：channel 是 Go runtime 提供的并发通信结构，内部维护缓冲队列、发送等待队列、接收等待队列和互斥锁。发送和接收会根据缓冲区状态、等待队列状态决定是否阻塞或直接唤醒对方 goroutine。

Q：无缓冲 channel 和有缓冲 channel 区别？

A：无缓冲 channel 发送和接收必须同时准备好，是同步交接；有缓冲 channel 可以在缓冲未满时发送不阻塞，在缓冲非空时接收不阻塞。

Q：关闭 channel 的语义？

A：关闭表示不会再有新值发送。接收方仍可读完缓冲数据，之后读到零值且 `ok=false`。向已关闭 channel 发送会 panic，重复 close 也会 panic。

Q：nil channel 会怎样？

A：对 nil channel 发送和接收会永久阻塞；close nil channel 会 panic。select 中 nil channel 对应 case 永远不 ready，可以用来动态开关 case。

Q：怎么避免 goroutine 泄露？

A：所有长期 goroutine 都应有退出条件，常见方式是监听 `ctx.Done()`、done channel、关闭任务 channel；阻塞 IO 要支持 context 或 timeout。

### 13.2 select 相关

Q：select 多个 case 同时 ready 怎么选？

A：Go 会伪随机选择一个 ready case，不能依赖顺序。

Q：select 加 default 的效果？

A：变成非阻塞 select。如果没有其他 case ready，会立即执行 default。

Q：项目哪里用了非阻塞 select？

A：`audit.Emit`，审计队列满时走 default 丢弃事件。

Q：select 监听 ticker 时要注意什么？

A：用 `time.NewTicker` 后要 `Stop()`；否则 ticker 关联的 runtime timer 不释放。worker 要同时监听退出信号，避免无限跑。

### 13.3 mutex 相关

Q：Mutex 和 RWMutex 怎么选？

A：临界区简单且读写比例不明显，用 Mutex；读多写少且读临界区耗时明显时用 RWMutex。RWMutex 不是一定更快，有额外开销。

Q：Go map 并发读写安全吗？

A：不安全。并发读写 map 会 data race，严重时 panic：`fatal error: concurrent map read and map write`。项目中 IP limiter map、Kafka writers map、RocketMQ consumers map 都用锁保护。

Q：锁粒度怎么判断？

A：锁粒度越大越简单但并发差；粒度越小并发好但复杂且易死锁。项目中 TokenBucket 每个桶一把锁，IPRateLimiter map 一把锁，是两级锁设计。

Q：defer Unlock 有什么问题？

A：安全但可能扩大锁持有时间。短函数适合 defer，长函数或循环里需要手动尽早 unlock。

### 13.4 context 相关

Q：context 解决什么问题？

A：跨 API 边界传递取消信号、截止时间和请求作用域值。项目中用于 HTTP/gRPC 请求取消、DB/Redis/MinIO/MQ 超时、worker 停止。

Q：为什么 `defer cancel()`？

A：释放 timer 和相关资源；如果不调用 cancel，超时前 timer 会一直存在。

Q：什么时候用 Background，什么时候用请求 context？

A：请求链路用请求 context；后台任务或补偿任务可从 Background 派生，但必须设置取消或超时。

Q：context.Value 能放什么？

A：只放请求作用域、跨 API 边界的数据，如 trace id、认证信息；不要放业务可选参数，不要滥用为全局变量。

### 13.5 goroutine 相关

Q：goroutine 泄露常见原因？

A：channel 永远没人读/写、没有退出条件、阻塞 IO 无超时、ticker 不 stop、context 不 cancel、pipe 不 close。

Q：项目哪些 goroutine 需要重点关注泄露？

A：rate limiter cleanup 没有 stop；audit writer 有 shutdown；MinIO retry worker 有 cancel；CSV export pipe 依赖 request context 和 pipe close。

Q：recover 能捕获其他 goroutine 的 panic 吗？

A：不能。recover 只能在同一个 goroutine 的 defer 中捕获 panic。

### 13.6 sync.Once 相关

Q：sync.Once 如果初始化失败怎么办？

A：Once 不知道失败，函数只执行一次。若要失败后可重试，需要自己封装状态，不要直接用 Once。

Q：sync.Once 的典型场景？

A：单例初始化、全局连接池、全局配置 lazy init、生成代码 descriptor init。

### 13.7 atomic 相关

Q：atomic.Value 适合什么？

A：读多写少、整体替换的不可变快照，如动态配置、路由表、规则快照。

Q：atomic.Value 和 mutex 的区别？

A：atomic.Value 读路径无锁但只适合整体替换；mutex 可保护复杂临界区和多步骤更新。

## 14. 当前项目并发设计风险清单

- `serverErr` 有两个发送方但缓冲为 1，且正常退出 nil 也会让 main 返回，生命周期管理可以优化为 errgroup。
- `audit.Emit` 的 closed 检查和发送不在同一临界区，存在 shutdown 并发 send on closed channel 的理论风险。
- `audit.Event.Metadata` 是 map，异步消费前如果被调用方修改可能 data race 或审计内容变化。
- `rate_limit.cleanupStaleBuckets` 没有退出机制，不适合动态创建/销毁 limiter 的场景。
- `request_pool` 在 ants goroutine 中执行 Gin `Context`，属于敏感用法，需要 race test 和压测确认。
- RocketMQ `producer` 字段没有锁保护，依赖启动顺序避免并发读写。
- MinIO retry worker 多实例部署时缺少任务抢占锁，可能重复处理同一补偿任务。
- Kafka consumer 遇到持续错误会 tight loop 打日志，最好对非 ctx 错误做退避。

## 15. 如果面试官要求你现场改进

### 15.1 用 errgroup 管理 HTTP/gRPC 生命周期

可回答思路：

- 使用 `errgroup.WithContext`。
- 一个 goroutine 启 HTTP。
- 一个 goroutine 启 gRPC。
- 一个 goroutine 等信号并触发 shutdown。
- 区分 `http.ErrServerClosed`、`grpc.ErrServerStopped` 和真实错误。

### 15.2 审计强一致改造

可回答思路：

- 如果审计必须不丢，不能用内存 channel 尽力而为。
- 可用 outbox 表，把审计事件和业务事务一起写 DB。
- 后台 worker 扫描 outbox 投递 MQ 或落审计表。
- 使用唯一键和状态机保证幂等。

### 15.3 rate limiter 可停止改造

可回答思路：

- `NewIPRateLimiter` 接收 context。
- cleanup goroutine select 监听 `ctx.Done()` 和 `ticker.C`。
- 应用 shutdown 时 cancel。

### 15.4 请求池改造

可回答思路：

- 更推荐把请求池放在业务重任务处，而不是把整个 Gin handler 放入 goroutine。
- 如果要做全局并发限制，可以用 semaphore middleware：进入请求时 acquire，退出时 release。
- `golang.org/x/sync/semaphore` 或 buffered channel 都可以实现。

## 16. 一句话总结

本项目的 Go 并发核心不是“炫技”，而是服务端工程里的典型问题：多 server 生命周期管理、异步审计削峰、限流状态并发安全、请求过载保护、DB 行级锁保证跨实例一致性、context 控制超时取消。面试时要把语法点和业务场景绑定起来讲，同时主动指出现有实现的取舍和风险。
