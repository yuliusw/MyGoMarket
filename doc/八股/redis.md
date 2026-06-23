# Redis 八股与项目实战

本文只整理当前项目真实对上代码的 Redis 用法，覆盖连接池、Session、限流、排行榜、发布锁、分布式锁工具、故障降级和面试拷打点。

## 1. 项目里 Redis 用在哪里

| 场景 | 代码位置 | Redis 数据结构/命令 | 是否主链路 |
| --- | --- | --- | --- |
| Redis 初始化与连接池 | `common/database/redis.go` | `redis.NewClient`、`Ping` | 是 |
| JWT + Session 顶号 | `services/iam/repository/user_repository.go`、`common/middleware/auth.go` | `SET`、`GET`、`DEL` | 是 |
| Market 下载排行榜 | `services/market/app/market.go` | `Pipeline`、`ZINCRBY`、`EXPIRE`、`ZREVRANGE WITHSCORES` | 是 |
| Market 发布幂等并发锁 | `services/market/app/market.go` | `SETNX`、`DEL` | 是 |
| Redis 滑动窗口限流 | `common/middleware/redis_sliding_window.go` | Lua、`ZREMRANGEBYSCORE`、`ZCARD`、`ZADD`、`PEXPIRE` | 配置支持，默认未启用 |
| Redis 分布式锁工具 | `common/utils/lock/redis_lock.go` | `SETNX`、Lua compare-and-delete、轮询自旋 | 工具类，当前主链路未直接调用 |
| go-redis 连接池配置 | `config/config.yaml`、`common/config/config.go` | pool size、timeout | 是 |

默认配置里 `features.rate_limit.backend` 是 `memory`，所以公开接口限流默认走内存令牌桶；如果改成 `redis`，会走 Redis Lua 滑动窗口。

## 2. Redis 初始化链路

入口：`main.go`

```go
database.InitRedis()
```

实现：`common/database/redis.go`

```go
RedisClient = redis.NewClient(&redis.Options{
    Addr:         fmt.Sprintf("%s:%d", redisConf.Host, redisConf.Port),
    Password:     redisConf.Password,
    DB:           redisConf.DB,
    PoolSize:     intOrDefault(redisConf.PoolSize, 100),
    MinIdleConns: intOrDefault(redisConf.MinIdleConns, 10),
    DialTimeout:  durationSecondsOrDefault(redisConf.DialTimeoutSeconds, 5*time.Second),
    ReadTimeout:  durationSecondsOrDefault(redisConf.ReadTimeoutSeconds, 3*time.Second),
    WriteTimeout: durationSecondsOrDefault(redisConf.WriteTimeoutSeconds, 3*time.Second),
    PoolTimeout:  durationSecondsOrDefault(redisConf.PoolTimeoutSeconds, 4*time.Second),
})
```

启动校验：

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
_, err := RedisClient.Ping(ctx).Result()
```

项目配置：`config/config.yaml`

```yaml
redis:
  host: "localhost"
  port: 6379
  password: ""
  db: 0
  pool_size: 100
  min_idle_conns: 10
  dial_timeout_seconds: 5
  read_timeout_seconds: 3
  write_timeout_seconds: 3
  pool_timeout_seconds: 4
```

环境变量覆盖：`common/config/config.go` 绑定了 `redis.host`、`redis.port`、`redis.password`、`redis.db`、`redis.pool_size`、`redis.min_idle_conns`、`redis.dial_timeout_seconds`、`redis.read_timeout_seconds`、`redis.write_timeout_seconds`、`redis.pool_timeout_seconds`。

容器部署：`script/docker-compose.yaml`

- 镜像：`redis:8.4.0-alpine`
- 容器名：`RPA-redis`
- 配置文件：`./redis.conf:/usr/local/etc/redis/redis.conf`
- 数据卷：`redisdata:/data`
- 端口：`6379:6379`
- healthcheck：`redis-cli ping`

面试拷打：

- `PoolSize` 控制 go-redis 最大连接数，不是 Redis 服务端最大连接数。
- `MinIdleConns` 用于维持空闲连接，减少突发请求建连成本。
- `PoolTimeout` 是连接池拿连接等待时间，超过会报错。
- `ReadTimeout` / `WriteTimeout` 是单次 socket 读写超时。
- 初始化阶段 `Ping` 用 `WithTimeout`，避免 Redis 异常导致启动无限卡住。

雷点：

- 当前 `InitRedis` 失败直接 `log.Fatalf`，Redis 是强依赖。因为 JWT Session、Market 热榜、发布锁等都依赖 Redis。
- `RedisClient` 是全局变量，初始化顺序必须在依赖它的 repository/middleware 前完成。
- 容器内配置通过环境变量覆盖，不能只看 `config/config.yaml` 的 localhost。

## 3. Redis Session：JWT + 长会话 + 顶号

相关代码：

- `common/middleware/auth.go`
- `services/iam/repository/user_repository.go`
- `common/utils/jwt.go`
- `app.go`

依赖注入：`app.go`

```go
middleware.InitJWTAuth(repository.NewUserRepository(database.DB, database.RedisClient))
```

Session key：`services/iam/repository/user_repository.go`

```go
const UserSessionPrefix = "user:session:"
```

写 Session：

```go
func (r *userRepository) SetSession(ctx context.Context, userID string, sessionID string, expiration time.Duration) error {
    key := UserSessionPrefix + userID
    return r.redis.Set(ctx, key, sessionID, expiration).Err()
}
```

读 Session：

```go
func (r *userRepository) GetSession(ctx context.Context, userID string) (string, error) {
    key := UserSessionPrefix + userID
    return r.redis.Get(ctx, key).Result()
}
```

删 Session：

```go
func (r *userRepository) DeleteSession(ctx context.Context, userID string) error {
    key := UserSessionPrefix + userID
    return r.redis.Del(ctx, key).Err()
}
```

认证链路：`common/middleware/auth.go`

1. 从 `Authorization: Bearer` 或 `auth_token` Cookie 取 JWT。
2. 严格校验 JWT。
3. JWT 有效时，如果是浏览器 Cookie 模式，则查 Redis：`user:session:{user_id}`。
4. Cookie 模式下 Redis 中 session 必须等于 Cookie `session_id`。
5. JWT 有效且是纯 API Bearer 模式时，只要 token 校验通过即可放行，不再强制要求 `session_id` Cookie。
6. JWT 过期时，用 `ParseTokenIgnoreExpiry` 提取 user_id。
7. Redis Session 仍有效且与 Cookie 一致时重新签 JWT，并刷新 Redis TTL。
8. Redis Session 不存在、不一致或查错，则返回 `401`。

为什么这么设计：

- JWT 短期有效，减少服务端状态压力。
- Redis Session 长期有效，支持自动登录和续签。
- Redis 保存单个当前 session，实现顶号。新登录覆盖旧 session 后，旧设备携带的 `session_id` 与 Redis 不一致，会被踢下线。
- 改密码或登出删除 Redis Session，强制重新登录。

Redis 命令语义：

- `SET key value expiration`：保存 session 并设置 TTL。
- `GET key`：读取当前有效 session。
- `DEL key`：注销或改密时清理。

面试拷打：

- 为什么 JWT 有效还要查 Redis？
- 为了服务端可控：顶号、登出、改密强制失效。如果只用纯 JWT，签发后直到过期前无法主动失效。
- Redis 宕机会怎样？
- Cookie 会话模式会因为 Session 校验失败而不可用；纯 Bearer 模式下有效 JWT 可降低对 `session_id` Cookie 的耦合，但自动续签仍依赖 Redis Session。
- 非浏览器客户端只带 Bearer token 行不行？
- 可以。当前纯 API 客户端只携带有效 `Authorization: Bearer <token>` 即可通过；只有过期 token 自动续签需要 Redis Session 与 Cookie 一致。

雷点：

- Redis Session 仍是浏览器会话、防顶号和自动续签的强依赖。
- `session_id` 是 Cookie，不在 Authorization header 中；移动端/脚本调用如果只使用有效 Bearer token，可以不携带 `session_id`。
- Session key 没有租户前缀，当前单系统没问题，多环境共用 Redis 时要加 namespace。

## 4. Redis 限流：滑动窗口 Lua

配置入口：`common/middleware/rate_limit_config.go`

```go
if strings.EqualFold(rateLimit.Backend, "redis") {
    return RedisSlidingWindowMiddleware(
        database.RedisClient,
        durationFromSeconds(rateLimit.WindowSeconds, time.Second),
        intFromConfig(rateLimit.Limit, 100),
    )
}
```

默认配置：`config/config.yaml`

```yaml
features:
  rate_limit:
    enabled: true
    backend: "memory"
    rate: 5
    capacity: 10
    cleanup_seconds: 300
    ttl_seconds: 600
    window_seconds: 1
    limit: 100
```

所以当前默认不是 Redis 限流，而是内存令牌桶。如果 `backend` 改成 `redis`，使用 `common/middleware/redis_sliding_window.go`。

Lua 脚本逻辑：

```lua
redis.call('ZREMRANGEBYSCORE', key, '-inf', clearBefore)
local count = redis.call('ZCARD', key)
if count < limit then
    redis.call('ZADD', key, now, member)
    redis.call('PEXPIRE', key, window)
    return 1
else
    return 0
end
```

key 设计：

```go
key := fmt.Sprintf("rpa:ratelimit:ip:%s", ip)
```

member 设计：

```go
member := uuid.New().String()
```

为什么用 UUID 做 member：

- ZSET member 必须唯一。
- 如果用毫秒时间戳做 member，同一毫秒并发请求会互相覆盖，导致统计偏小。

为什么用 Lua：

- 删除过期请求、计数、判断、写入、设置 TTL 必须原子完成。
- 多条命令分开执行会有并发窗口。
- Redis 单线程执行 Lua，脚本执行期间不会被其他命令打断。

故障策略：

```go
if err != nil {
    c.Next()
    return
}
```

Redis 限流异常时项目选择降级放行，优先保证核心业务可用。

雷点：

- 降级放行会在 Redis 故障时失去限流保护。
- 滑动窗口 ZSET 会对每个请求写一个 member，高 QPS 下 Redis 写放大明显。
- `PEXPIRE key window` 只保留一个窗口期，适合短窗口限流；如果要审计请求历史，这个设计不保留。
- 当前默认内存限流是单实例维度，多实例部署时每个实例各限各的；Redis 限流才是全局维度。

面试拷打：

- 滑动窗口比固定窗口好在哪里？
- 固定窗口在边界处可能双倍突刺；滑动窗口按当前时间向前滚动统计，更平滑。
- 滑动窗口比令牌桶差在哪里？
- 滑动窗口每次都写 ZSET 并清理，成本更高；令牌桶成本低但允许一定突发。
- Redis Lua 会阻塞 Redis 吗？
- 会。Lua 是原子执行，脚本太慢会阻塞其他命令，所以脚本必须短小。

## 5. Market 排行榜：Redis ZSET + Pipeline

相关代码：`services/market/app/market.go`

下载链路：

```go
recordDownloadRank(ctx, appID.String())
```

写入逻辑：

```go
pipe := redisClient.Pipeline()
pipe.ZIncrBy(ctx, key, 1, appID)
pipe.Expire(ctx, key, 48*time.Hour)
...
pipe.Exec(ctx)
```

key 设计：

```go
daily  = market:rank:downloads:daily:YYYYMMDD
weekly = market:rank:downloads:weekly:YYYYWW
total  = market:rank:downloads:total
```

TTL 策略：

- daily：48 小时。
- weekly：15 天。
- total：不设置 TTL。

查询逻辑：

```go
items, err := redisClient.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
```

然后批量查 PostgreSQL：

```go
apps, err := appRepo.GetByIDs(ctx, ids)
```

再过滤：只返回 `published` 状态 app。

为什么用 ZSET：

- member 是 app_id。
- score 是下载次数。
- `ZINCRBY` 天然适合计数累加。
- `ZREVRANGE WITHSCORES` 天然适合 TopN 排行榜。

为什么用 Pipeline：

- 一次下载要更新 daily、weekly、total，多次 Redis 往返会放大延迟。
- Pipeline 合并网络往返，但不保证事务原子性。
- 这里允许排行榜短暂不一致，所以 Pipeline 足够。

为什么还写 PostgreSQL：

```go
appRepo.IncrementDownloadMetric(ctx, parsedAppID, now)
```

- Redis 用于实时榜单。
- PostgreSQL `app_download_metrics` 用于持久化统计。
- Redis 丢数据或过期后，DB 仍保留按天指标。

雷点：

- `recordDownloadRank` 先写 DB 再写 Redis；Redis 写失败只打日志，不影响下载。
- `total` 不过期，长期 app_id 会越来越多，需要考虑清理已删除 app 的 member。
- 查询热榜时 Redis 返回 app_id 后还要查 DB，Redis 中可能有已下架/删除 app，代码做了过滤。
- Redis Pipeline 不是事务；如果 daily 成功、weekly 失败，可能出现短期不一致。

面试拷打：

- Redis ZSET 排行榜如何分页？
- `ZREVRANGE key start stop WITHSCORES`，但深分页成本高。大榜单可考虑分片或异步物化。
- 如何处理同分排序？
- Redis ZSET score 相同按 member 字典序排序。如果业务需要时间优先，需要把 score 设计成复合分数或额外存时间。
- Redis 榜单和 DB 统计不一致怎么办？
- 明确 Redis 是实时缓存，DB 是持久事实源；可定时从 DB 重建 Redis 榜单。

## 6. Market 发布锁：Redis SetNX

位置：`services/market/app/market.go:628`

```go
key := fmt.Sprintf("market:publish:lock:%s:%s", developerID.String(), idempotencyKey)
locked, err := redisClient.SetNX(ctx, key, "1", 2*time.Minute).Result()
return func() { _ = redisClient.Del(context.Background(), key).Err() }, locked, nil
```

调用链：

1. 发布应用时读取 `Idempotency-Key`。
2. 先查 DB 是否已有同开发者同幂等键记录。
3. 没有记录则用 Redis `SETNX` 抢锁。
4. 抢锁失败返回 `409 DUPLICATE_UPLOAD_IN_PROGRESS`。
5. 抢锁成功后继续保存临时文件、上传 MinIO、写 DB。
6. handler 结束 `defer unlock()` 删除锁。

为什么需要 Redis 锁：

- 同一个开发者同一个幂等键可能并发提交。
- DB 最终有唯一索引兜底，但 Redis 锁可以提前快速拒绝重复上传，避免重复上传大文件到 MinIO。

为什么不能只靠 Redis 锁：

- Redis 锁 TTL 到期后可能失效。
- Redis 宕机或锁删除失败会影响互斥。
- 最终幂等仍要靠 DB 唯一索引和查询兜底。

项目里的 DB 兜底：`script/init_better.sql`

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_apps_developer_idempotency_key
  ON apps(developer_id, (metadata ->> 'idempotency_key'))
  WHERE COALESCE(metadata ->> 'idempotency_key', '') <> '';
```

雷点：

- lock value 是固定 `1`，unlock 直接 `DEL`，如果锁过期后被另一个请求获取，旧请求结束时可能误删新锁。
- 当前发布锁 TTL 是 2 分钟，如果上传大文件或 DB 慢超过 2 分钟，会出现锁过期并发窗口。
- 更严谨做法是 value 用 UUID，删除时用 Lua compare-and-delete。
- 项目已经有 `common/utils/lock/redis_lock.go` 提供 UUID value + Lua 解锁工具，但 Market 发布当前没有用该工具。

## 7. RedisLock 工具类

位置：`common/utils/lock/redis_lock.go`

加锁：

```go
success, err := l.client.SetNX(ctx, l.key, l.value, l.expiration).Result()
```

解锁 Lua：

```lua
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
```

自旋：

```go
for time.Now().Before(deadline) {
    success, err := l.Lock(ctx)
    if err == nil && success { return nil }
    time.Sleep(100 * time.Millisecond)
}
```

可讲亮点：

- value 用 UUID，避免误删别人的锁。
- 解锁用 Lua 保证 get + del 原子性。
- SpinLock 有超时时间，避免无限等待。

雷点：

- 没有锁续期，看门狗能力缺失。
- 自旋固定 100ms，缺少随机抖动，高并发可能形成惊群。
- 单 Redis 实例分布式锁不是强一致锁。Redis 主从故障切换场景可能丢锁。
- 当前工具类存在，但主链路未看到直接调用，讲项目时不要说所有锁都用了这个工具。

## 8. Redis 和 PostgreSQL 的边界

本项目边界划分：

- Redis：短生命周期、实时性、可重建或可降级的数据。
- PostgreSQL：核心事实数据、交易数据、审计落库、订阅、钱包、订单。

具体例子：

- Session 存 Redis，但用户账号在 PostgreSQL。
- 热榜实时分数在 Redis，但下载指标持久化在 PostgreSQL。
- 上传并发锁在 Redis，但应用发布幂等唯一性最终靠 PostgreSQL 唯一索引。
- 限流可以 Redis 全局化，也可以内存单实例化。

面试总结：

- Redis 不应该承载不可丢的核心交易事实。
- Redis 很适合做缓存、计数器、排行榜、锁、限流、Session。
- 真正一致性要靠 DB 事务/唯一约束/行锁兜底。

## 9. Redis 常见拷打题

### 9.1 缓存穿透、击穿、雪崩

项目已有应用详情 Cache-Aside 缓存，仍要能把通用缓存问题和代码实现对应起来。

- 穿透：查不存在 key，穿透到 DB。解决：空值缓存、布隆过滤器、参数校验。
- 击穿：热点 key 过期瞬间大量请求打 DB。解决：互斥锁、singleflight、逻辑过期、热点不过期。
- 雪崩：大量 key 同时过期或 Redis 故障。解决：TTL 加随机、限流降级、多级缓存、高可用。

和项目对应：

- 应用详情缓存 key 是 `market:app:detail:{app_id}`，查无实体写 30 秒空值标记，属于防穿透。
- 应用详情 Cache Miss 用 `singleflight` 合并回源，属于防击穿。
- 应用详情正常 TTL 是 `5m + rand(0~60s)`，属于防雪崩。
- 热榜结构体缓存 key 是 `market:rank:cache:{type}:{limit}`，默认 TTL 10 秒。
- Casbin 用的是本地 LRU + singleflight，不是 Redis 缓存。
- Redis 限流失败时项目选择降级放行。
- Session 不适合空值缓存，认证失败直接 401。

### 9.2 Redis 分布式锁正确性

必答点：

- 加锁：`SET key value NX PX ttl`。
- value 必须唯一，通常 UUID。
- 解锁必须 Lua 判断 value 后删除。
- 业务执行时间可能超过 TTL，需要续期或合理 TTL。
- 单 Redis 锁不能解决所有分布式一致性问题。

和项目对应：

- `RedisLock` 工具类是比较标准的 UUID + Lua 解锁。
- Market 发布锁当前是简化版 `SetNX + Del`，存在误删风险，但 DB 唯一索引兜底。

### 9.3 Pipeline、事务、Lua 区别

- Pipeline：合并网络往返，不保证原子性。
- MULTI/EXEC：事务队列，执行时连续执行，但不能基于中间结果做复杂判断。
- Lua：服务端原子执行，可以读结果后分支判断。

和项目对应：

- 排行榜用 Pipeline，因为允许短期不一致。
- 滑动窗口限流用 Lua，因为必须原子判断 count 并写入。

### 9.4 Redis ZSET 排行榜为什么合适

- `ZINCRBY` 更新分数。
- `ZREVRANGE WITHSCORES` 获取 TopN。
- 可按时间维度拆 key，方便 TTL。
- 缺点是大 key、深分页和内存占用。

### 9.5 Session 放 Redis 的优缺点

优点：

- 服务端可主动失效。
- 多实例共享登录态。
- TTL 自动过期。

缺点：

- Redis 成为认证强依赖。
- 每次私有请求多一次 Redis 查询。
- Cookie、跨域、安全属性要处理好。

## 10. 当前 Redis 风险和可优化点

- Market 发布锁建议改用 `RedisLock` 工具类的 UUID value + Lua 解锁。
- Redis key 建议统一加系统和环境前缀，例如 `rpa:{env}:user:session:{userID}`。
- Cookie 会话模式下 Session 查 Redis 是私有接口关键路径，高并发要关注 Redis QPS 和连接池耗尽。
- 应用详情缓存要关注缓存击穿、穿透和主动失效是否覆盖发布、更新、下架、删除全链路。
- Redis 限流降级放行要配合告警，否则 Redis 故障时系统会失去保护。
- Redis 热榜 total key 长期增长，需要定期清理删除/下架 app 的 member。
- RedisLock SpinLock 建议加入 context 检查和 jitter，避免固定间隔惊群。
- 如果 Redis 用主从或哨兵，需要重新评估分布式锁语义。
