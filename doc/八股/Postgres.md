# PostgreSQL / GORM 八股与项目实战

本文基于当前项目真实代码和 `script/init_better.sql` 整理 PostgreSQL、GORM、事务、行级锁、索引、约束、分区、JSONB、Outbox、流式导出和面试拷打点。讲法按“基础概念 -> 项目表结构 -> 查询/事务链路 -> 优化追问”展开，避免只背术语。

## 1. 项目里 PostgreSQL 用在哪里

| 场景 | 代码/脚本位置 | 关键能力 |
| --- | --- | --- |
| 连接初始化 | `common/database/gorm.go` | GORM、PostgreSQL driver、连接池 |
| 表结构初始化 | `script/init_better.sql` | extensions、表、索引、触发器、种子数据 |
| IAM 用户/角色/群组 | `services/iam/*` | 唯一约束、外键、RBAC 表 |
| Market 应用 | `services/market/respository/app_respository.go` | JSONB metadata、数组 tags、ILIKE、GIN/trgm 索引、upsert 指标 |
| Wallet 钱包 | `services/wallet/repository/wallet_repository.go` | 事务、`SELECT FOR UPDATE`、唯一约束、decimal |
| Order 订单 | `services/order/repository/order_repository.go` | 事务、行级锁、订单状态机、订阅发放 |
| Audit 审计 | `common/audit/audit.go`、`common/audit/export.go` | 批量插入、游标导出、JSONB metadata |
| Admin 查询 | `services/admin/app/*` | 多条件过滤、分页、HTTP/gRPC 对齐 |

## 2. PostgreSQL 初始化链路

入口：`main.go`

```go
database.InitGORM()
```

实现：`common/database/gorm.go`

```go
dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=Asia/Shanghai",
    dbConf.Host, dbConf.User, dbConf.Password, dbConf.DBName, dbConf.Port)
DB, err = gorm.Open(postgres.Open(dsn), gormConfig)
```

连接池配置：

```go
sqlDB.SetMaxIdleConns(intOrDefault(dbConf.MaxIdleConns, 10))
sqlDB.SetMaxOpenConns(intOrDefault(dbConf.MaxOpenConns, 100))
sqlDB.SetConnMaxLifetime(durationSecondsOrDefault(dbConf.ConnMaxLifetimeSeconds, time.Hour))
sqlDB.SetConnMaxIdleTime(durationSecondsOrDefault(dbConf.ConnMaxIdleTimeSeconds, 10*time.Minute))
```

GORM 日志级别：

```go
logger.Default.LogMode(gormLogLevel(dbConf.LogLevel))
```

支持：`silent`、`error`、`info`，默认 `warn`。

容器配置：`script/docker-compose.yaml`

- 镜像：`postgres:18-alpine3.22`
- 容器名：`RPA-pgdb`
- 初始化 SQL：`./init_better.sql:/docker-entrypoint-initdb.d/init.sql:ro`
- 数据卷：`pgdata:/var/lib/postgresql/data`
- healthcheck：`pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"`

配置：`config/config.yaml`

```yaml
database:
  host: "localhost"
  port: 5432
  user: "rpa_app"
  password: "change-me-db-password"
  dbname: "RPA"
  max_idle_conns: 10
  max_open_conns: 100
  conn_max_lifetime_seconds: 3600
  conn_max_idle_time_seconds: 600
  log_level: "warn"
```

雷点：

- 容器里 DB 用户、密码、库名由 compose 环境变量注入，不能只看本地 config。
- `sslmode=disable` 适合本地/内网，生产环境需要 TLS 策略。
- `TimeZone=Asia/Shanghai` 影响连接会话时区，但业务里有些地方显式 UTC，例如下载指标按 UTC 日期归档。

## 3. 初始化 SQL 的扩展能力

位置：`script/init_better.sql`

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gist;
```

项目实际用途：

- `pgcrypto`：提供 `gen_random_uuid()`，作为 UUID fallback。
- `pg_trgm`：支持应用名称模糊搜索 trigram GIN 索引。
- `btree_gist`：当前脚本创建了扩展，但从已读 SQL 看主链路暂未明显使用 GiST 排他约束。

UUID fallback：

```sql
CREATE OR REPLACE FUNCTION gen_uuid() RETURNS uuid AS $$
DECLARE v uuid;
DECLARE has_v7 boolean;
BEGIN
  SELECT EXISTS(SELECT 1 FROM pg_proc WHERE proname='uuidv7') INTO has_v7;
  IF has_v7 THEN
    EXECUTE 'SELECT uuidv7()' INTO v;
  ELSE
    v := gen_random_uuid();
  END IF;
  RETURN v;
END;
$$ LANGUAGE plpgsql VOLATILE;
```

亮点：

- 如果环境提供 `uuidv7()`，优先使用时间有序 UUID。
- 否则使用 `gen_random_uuid()`，保证初始化脚本能跑。

面试拷打：

- UUIDv7 相比随机 UUIDv4 对索引更友好，因为大致按时间递增，B-tree 页分裂更少。
- PostgreSQL 原生并不一定带 `uuidv7()`，所以项目做了 fallback。

## 4. 核心表结构

### 4.1 IAM 表

表：

- `users`：用户。
- `roles`：角色。
- `permissions`：权限。
- `role_permissions`：角色权限多对多。
- `groups`：群组，也是 Casbin domain 的业务来源。
- `group_members`：用户在群组中的角色。

关键约束：

- `users.username UNIQUE`
- `users.email UNIQUE`
- `roles.role_name UNIQUE`
- `permissions.permission_code UNIQUE`
- `role_permissions PRIMARY KEY (role_id, permission_id)`
- `group_members PRIMARY KEY (group_id, user_id)`

全局域种子：

```sql
INSERT INTO groups (group_id, group_name, owner_id, group_type)
VALUES ('11111111-1111-1111-1111-111111111111', 'System Global Group', ...)
```

默认管理员：

```sql
INSERT INTO users (user_id, username, email, password_hash, is_active)
VALUES ('00000000-0000-0000-0000-000000000001', 'admin', 'admin@example.com', ...)
```

### 4.2 Market 表

`apps`：

```sql
CREATE TABLE IF NOT EXISTS apps (
    app_id UUID PRIMARY KEY DEFAULT gen_uuid(),
    name VARCHAR(255) NOT NULL,
    developer_id UUID NOT NULL REFERENCES users(user_id),
    category VARCHAR(50),
    tags TEXT[],
    metadata JSONB DEFAULT '{}'::jsonb,
    status VARCHAR(20) DEFAULT 'published',
    create_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

项目实际用法：

- `tags TEXT[]` 对应 Go 里 `pq.StringArray`。
- `metadata JSONB` 保存 MinIO object name、sha256、etag、content_type、idempotency_key 等。
- `status` 用于 `published` / `off_shelved` 等状态过滤。

`app_download_metrics`：

```sql
PRIMARY KEY (app_id, metric_date)
```

用于按天持久化下载次数，配合 Redis 实时热榜。

`minio_delete_retries`：

用于 MinIO 删除失败补偿队列，字段包括 `attempts`、`status`、`next_run_at`。

### 4.3 Wallet 表

`wallets`：

```sql
wallet_id UUID PRIMARY KEY DEFAULT gen_uuid(),
owner_id UUID NOT NULL,
owner_type VARCHAR(20) NOT NULL CHECK (owner_type IN ('user', 'group')),
balance DECIMAL(18, 4) DEFAULT 0.0000,
currency_code VARCHAR(10) DEFAULT 'COIN',
status VARCHAR(20) DEFAULT 'active',
UNIQUE (owner_id, currency_code)
```

项目实际用法：

- `GetOrCreateByOwner` 使用 `ON CONFLICT DO NOTHING` 创建钱包。
- 钱包写操作使用 `SELECT FOR UPDATE` 锁行。
- 金额用 `shopspring/decimal` 对应 DB `DECIMAL(18,4)`。

雷点：

- 表里唯一约束是 `(owner_id, currency_code)`，没有包含 `owner_type`。如果同一个 UUID 同时作为 user 和 group owner，理论上会冲突。当前业务主要用户钱包，问题不明显。

`wallet_transactions`：

```sql
idempotency_key TEXT UNIQUE
```

用于充值、扣款、转账、订单支付幂等。

### 4.4 Order 表

`orders`：

```sql
amount DECIMAL(18, 4) NOT NULL CHECK (amount > 0),
status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'cancelled')),
tx_id UUID REFERENCES wallet_transactions(tx_id),
subscription_id UUID,
idempotency_key TEXT
```

唯一索引：

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_idempotency_key
  ON orders(idempotency_key)
  WHERE COALESCE(idempotency_key, '') <> '';
```

作用：

- 空幂等键不参与唯一约束。
- 非空幂等键全局唯一。

### 4.5 Subscriptions 分区表

```sql
CREATE TABLE IF NOT EXISTS subscriptions (...) PARTITION BY RANGE (expired_at);
```

分区：

- `subscriptions_default`
- `subscriptions_2025`
- `subscriptions_2026`
- `subscriptions_2027`
- `subscriptions_2028`

作用：

- 订阅按过期时间范围分区。
- 查询和清理过期订阅时更容易做分区裁剪或分区维护。

雷点：

- 当前代码创建订阅默认有效期 1 年，如果超过现有分区范围，会落到 default 分区。
- 分区表主键包含 `expired_at`，这是 PostgreSQL 分区表唯一约束要求分区键参与唯一性的典型体现。

## 5. 索引设计与代码对应

索引基础先讲清楚：

- B-tree 适合等值、范围、排序和最左前缀匹配，是 PostgreSQL 默认索引类型。
- GIN 适合数组、JSONB、全文/倒排类检索，本项目用于 `tags TEXT[]` 和 trigram 模糊搜索。
- Partial Index 只索引满足条件的行，适合“非空幂等键”“某些状态”这类稀疏数据。
- Expression Index 索引表达式结果，本项目用 `(metadata ->> 'idempotency_key')` 给 JSONB 内字段加唯一约束。
- 复合索引要按查询谓词和排序设计，不能只看单列是否出现。

项目库表优化的主线：读路径用复合索引贴合 `WHERE + ORDER BY`，写路径用唯一索引兜底幂等，增长型表用分区或游标分页控制长期退化。

### 5.1 App 模糊搜索

代码：`services/market/respository/app_respository.go`

```go
query = query.Where("name ILIKE ?", "%"+keyword+"%")
```

索引：

```sql
CREATE INDEX IF NOT EXISTS idx_apps_name_trgm ON apps USING GIN (name gin_trgm_ops);
```

为什么需要 `pg_trgm`：

- 普通 B-tree 对 `ILIKE '%keyword%'` 基本无效。
- trigram GIN 可以加速包含式模糊查询。

### 5.2 App 列表过滤排序

代码：

```go
WHERE status = ?
WHERE category = ?
ORDER BY create_at DESC
```

索引：

```sql
CREATE INDEX IF NOT EXISTS idx_apps_status_create ON apps(status, create_at DESC);
CREATE INDEX IF NOT EXISTS idx_apps_status_category_create ON apps(status, category, create_at DESC);
```

### 5.3 App tags

字段：`tags TEXT[]`

索引：

```sql
CREATE INDEX IF NOT EXISTS idx_apps_tags ON apps USING GIN (tags);
```

当前代码已保存 tags，但列表查询没有按 tags 过滤，索引属于预留能力。

### 5.4 App 幂等唯一索引

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_apps_developer_idempotency_key
  ON apps(developer_id, (metadata ->> 'idempotency_key'))
  WHERE COALESCE(metadata ->> 'idempotency_key', '') <> '';
```

代码查询：

```go
Where("developer_id = ? AND metadata ->> 'idempotency_key' = ?", developerID, idempotencyKey)
```

作用：

- 同一开发者同一幂等键只能发布一次。
- Redis 锁只是并发优化，DB 唯一索引是最终兜底。

### 5.5 Audit 查询与导出

索引：

```sql
CREATE INDEX IF NOT EXISTS idx_audit_events_trace ON audit_events(trace_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_type_time ON audit_events(event_type, created_at DESC);
```

代码：

- Admin 查询支持 `event_type`、`actor_id`、`resource`、`trace_id`、时间范围。
- CSV 导出按 `created_at ASC, event_id ASC` 游标遍历。

雷点：

- CSV 导出按 `created_at,event_id` 排序，但 SQL 中没有专门 `(created_at, event_id)` 组合索引。大量审计数据时可以补。

### 5.6 Wallet / Order 索引

钱包：

```sql
CREATE INDEX IF NOT EXISTS idx_wallet_owner ON wallets(owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_wallet_tx_wallet_time ON wallet_transactions(wallet_id, created_at DESC);
```

订单：

```sql
CREATE INDEX IF NOT EXISTS idx_orders_user_time ON orders(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_orders_app_status ON orders(app_id, status);
CREATE INDEX IF NOT EXISTS idx_orders_status_time ON orders(status, created_at DESC);
```

对应接口：

- 用户订单列表：`WHERE user_id = ? ORDER BY created_at DESC`。
- 钱包流水：`WHERE wallet_id = ? ORDER BY created_at DESC`。
- Admin 全局订单：按 user/app/wallet/status/currency/time 过滤。

为什么这样建：

- 用户订单列表是典型 `WHERE user_id = ? ORDER BY created_at DESC`，`idx_orders_user_time` 可以同时服务过滤和排序。
- 钱包流水是 `WHERE wallet_id = ? ORDER BY created_at DESC`，`idx_wallet_tx_wallet_time` 避免按钱包过滤后再大范围排序。
- pending 超时取消 worker 会按 `status + created_at` 找待取消订单，`idx_orders_status_time` 支撑扫描待处理集合。
- Admin 全局查询条件更复杂，当前先保留通用复合索引，后续应按真实慢查询继续补，而不是盲目给每列都建索引。

### 5.7 Outbox 与订阅防重索引

`entitlement_outbox`：

```sql
CREATE INDEX IF NOT EXISTS idx_entitlement_outbox_status_next ON entitlement_outbox(status, next_run_at);
```

对应代码：

```go
Where("status IN ? AND next_run_at <= ?", []string{"pending", "failed"}, time.Now()).
    Order("next_run_at ASC").
    Limit(limit)
```

作用：

- worker 能快速找到到期的 pending/failed 消息。
- `next_run_at` 支撑按重试时间顺序扫描。
- 处理批次时使用 `FOR UPDATE SKIP LOCKED`，多 worker 可并发抢不同 outbox 行。

订阅发放防重：

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_2026_order_once
  ON subscriptions_2026(user_id, app_id, source_order_id)
  WHERE source_order_id IS NOT NULL;
```

项目因为 `subscriptions` 是分区表，所以唯一索引分别建在各个分区上。它解决的是“同一订单重复发放订阅”的最终兜底问题，RedisLock 和 outbox 状态机失败时，DB 唯一约束仍能防重复权益。

### 5.8 审计导出索引缺口

当前导出 SQL 是：

```go
Order("created_at ASC, event_id ASC")
Where("created_at > ? OR (created_at = ? AND event_id > ?)")
```

已有索引：

```sql
idx_audit_events_trace
idx_audit_events_type_time(event_type, created_at DESC)
```

缺口：

- 如果不按 `event_type` 过滤，导出大量审计数据时缺少 `(created_at, event_id)` 组合索引。
- 这会让 keyset cursor 的优势打折，因为排序和游标推进仍可能扫描较多数据。
- 可优化为补 `CREATE INDEX idx_audit_events_created_event ON audit_events(created_at ASC, event_id ASC);`。

这不是当前已落地能力，面试中要说“现状能流式导出，但大数据量下索引还可继续补”。

## 6. GORM 使用模式

### 6.1 `WithContext`

项目大量 repository 使用：

```go
r.db.WithContext(ctx).Where(...).First(&model)
```

意义：

- HTTP 客户端断开或超时后，DB 查询能被取消。
- gRPC deadline/cancel 能传到底层 DB。

### 6.2 `Create`

Market：

```go
r.db.WithContext(ctx).Create(app).Error
```

Order：

```go
r.db.WithContext(ctx).Create(newOrderModel(order)).Error
```

Wallet transaction：

```go
tx.WithContext(ctx).Create(newTransactionModel(transaction)).Error
```

### 6.3 `Updates`

Market 按字段更新：

```go
Model(&domain.App{}).Where("app_id = ?", id).Updates(updates)
```

Wallet 更新余额：

```go
Model(&walletModel{}).Where("wallet_id = ?", wallet.ID).Updates(map[string]any{...})
```

Order 更新状态：

```go
Model(&orderModel{}).Where("order_id = ?", order.OrderID).Updates(map[string]any{...})
```

注意：

- GORM `Updates` 使用 struct 时默认忽略零值；项目这里多用 map，能明确写入零值/null 指针。
- Market `Save` 注释里也写了会更新所有字段，包括零值。

### 6.4 `OnConflict`

Wallet 创建：

```go
r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model)
```

作用：

- 并发首次创建同一个用户钱包时，只会插入一条。
- 插入后再查询返回最终钱包。

### 6.5 Raw SQL / Exec Upsert

下载指标：

```go
INSERT INTO app_download_metrics (app_id, metric_date, download_count, updated_at)
VALUES (?, ?, 1, CURRENT_TIMESTAMP)
ON CONFLICT (app_id, metric_date)
DO UPDATE SET download_count = app_download_metrics.download_count + 1, updated_at = CURRENT_TIMESTAMP
```

为什么不用先查再更新：

- 先查再更新有并发丢失更新问题。
- `ON CONFLICT DO UPDATE` 在 DB 端原子完成插入或累加。

## 7. 事务与行级锁

### 7.1 Wallet 充值/扣款事务

位置：`services/wallet/repository/wallet_repository.go`

```go
err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    wallet, transaction, err = r.applyWalletChangeInTx(...)
    return err
})
```

核心步骤：

1. 检查幂等键是否已有流水。
2. 对钱包行加 `FOR UPDATE`。
3. 再次检查幂等键，避免锁等待期间其他事务已插入。
4. 领域对象计算新余额。
5. 更新 wallets。
6. 插入 wallet_transactions。

行锁：

```go
tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("wallet_id = ?", walletID).First(&model)
```

为什么必须 DB 锁：

- Go Mutex 只管单进程，Docker/K8s 多实例时无效。
- 钱包余额是共享持久数据，必须用 DB 事务隔离。

### 7.2 Wallet 转账固定锁顺序

```go
firstID, secondID := fromWalletID, toWalletID
if strings.Compare(firstID.String(), secondID.String()) > 0 {
    firstID, secondID = secondID, firstID
}
```

作用：

- 避免 A->B 与 B->A 并发时互相等待死锁。
- 两个事务都按同一顺序锁钱包行。

### 7.3 Order 支付事务

位置：`services/order/repository/order_repository.go`

支付链路：

1. `Transaction` 开启事务。
2. `lockOrder` 对订单行 `FOR UPDATE`。
3. 校验订单 owner。
4. 如果已支付，返回已有支付结果。
5. pending 订单调用 `wallet.DebitInTx`，在同一事务内扣钱包。
6. 写入 `entitlement_outbox` 本地消息表。
7. 标记订单 paid，写 `tx_id`；事务提交后由后台 Worker 异步创建 subscription 并回填 `subscription_id`。

亮点：

- 订单状态、钱包余额、钱包流水和权益 outbox 同一事务提交。
- 避免扣款成功但订单没支付，或订单 paid 但没有可重试的权益发放消息。
- 订阅发放由 outbox Worker 异步完成，并通过 `(user_id, app_id, source_order_id)` 唯一索引防重发。

雷点：

- `Purchase` = `Create` + `Pay`，创建和支付不是同一个外层事务。支付失败会留下 pending 订单，这是业务设计要解释清楚。
- 当前没有库存模型，不要把订单支付链路讲成 Redis Lua 库存预扣减。

## 8. 审计与批量写

位置：`common/audit/audit.go`

审计 writer 将 channel 中事件聚合为 `[]auditEventRow`：

```go
rows := make([]auditEventRow, 0, len(batch))
...
w.db.WithContext(ctx).Create(&rows).Error
```

特点：

- 每 100 条或每 2 秒批量插入。
- metadata 存 JSONB。
- 插入失败只打日志，不阻塞业务。

CSV 导出：`common/audit/export.go`

- 每批 500 条。
- `created_at,event_id` 作为稳定游标。
- `io.Pipe` 流式返回 CSV。
- 如果 MinIO 可用，使用第二个 `io.Pipe` 和 `io.MultiWriter` 同时写 HTTP 响应和 MinIO 对象，object 前缀为 `audit-exports/`。
- 这是应用层 keyset cursor，不是 PostgreSQL 服务端游标 `DECLARE CURSOR/FETCH`。

游标条件：

```go
query = query.Where("created_at > ? OR (created_at = ? AND event_id > ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.EventID)
```

面试拷打：

- 为什么不用 offset 分页导出？
- offset 深分页会越来越慢，并且并发插入时容易重复/跳过；游标分页更稳定。
- 为什么用 `io.Pipe`？
- 让 CSV 生成 goroutine 边查 DB 边写，HTTP reader 边读边返回，避免把百万行结果一次性放进内存。
- 为什么不是 PostgreSQL server-side cursor？
- 当前实现更贴合 GORM 和 HTTP streaming，复杂度低；缺点是每批都是独立查询，极大数据量下仍依赖合适索引。若要进一步降低 DB executor 内存和事务快照控制，可评估 `DECLARE CURSOR`。

流式导出链路：

1. Handler 创建 `httpReader/httpWriter := io.Pipe()`。
2. 如果 MinIO 可用，再创建 `minioReader/minioWriter := io.Pipe()`。
3. `io.MultiWriter(httpWriter, minioWriter)` 同时写两端。
4. goroutine 调用 `ExportCSV(c.Request.Context(), db, writer, filter)`。
5. `ExportCSV` 每 500 条查一批，写 CSV 并 flush。
6. Gin 通过 `DataFromReader` 把 `httpReader` 直接透传给客户端。
7. MinIO goroutine 从 `minioReader` 读取并上传对象。

雷点：

- `io.MultiWriter` 任一 writer 阻塞都会拖慢整体导出，MinIO 上传慢可能反向影响 HTTP 导出。
- 客户端断开会取消 request context，DB 查询应停止，但 MinIO 上传 goroutine 的生命周期也要靠 pipe 关闭收敛。
- 当前只支持 `audit_events` 导出；钱包流水和订单流水还没有 CSV 导出接口。

## 9. 触发器与更新时间

SQL 中定义：

- `set_update_at()` 更新 `update_at`。
- `set_updated_at()` 更新 `updated_at`。

触发表：

- `users`
- `apps`
- `projects`
- `project_workflows`
- `wallets`
- `orders`
- `minio_delete_retries`

作用：

- 应用层忘记写更新时间时，DB 层兜底。

雷点：

- 应用层有些更新也手动设置 `update_at` / `updated_at`，和触发器会重复，但结果通常一致。
- 触发器隐藏副作用，排查更新时间变化时要知道 DB 自动改了。

## 10. PostgreSQL 常见拷打题

### 10.1 事务 ACID

- Atomicity：事务内操作要么全部成功，要么全部回滚。
- Consistency：约束、外键、check、业务规则保持一致。
- Isolation：并发事务互相隔离。
- Durability：提交后持久化。

项目对应：订单支付事务保证扣款、流水、订阅、订单状态一致。

### 10.2 事务隔离级别

PostgreSQL 常见：

- Read Committed：默认，每条语句看到提交前最新快照。
- Repeatable Read：事务内快照一致。
- Serializable：最强隔离，冲突时可能回滚。

项目没有显式设置隔离级别，默认使用 PostgreSQL Read Committed。钱包/订单关键并发靠 `SELECT FOR UPDATE` 行锁补强。

### 10.3 `SELECT FOR UPDATE` 解决什么

- 锁住选中的行，其他事务更新同一行会等待。
- 用于余额扣减、库存扣减、订单状态流转。
- 必须放在事务内。

### 10.4 唯一索引和幂等

项目对应：

- `wallet_transactions.idempotency_key TEXT UNIQUE`
- `idx_orders_idempotency_key` partial unique index
- `idx_apps_developer_idempotency_key` expression partial unique index

为什么不仅靠代码先查：

- 先查再插在并发下有竞态。
- 唯一索引是最终防线。

### 10.5 JSONB 的优缺点

优点：

- schema 灵活。
- 可表达扩展 metadata。
- 支持表达式索引、GIN 索引。

缺点：

- 类型约束弱。
- 查询写法更复杂。
- 过度使用会让关系模型退化。

项目对应：`apps.metadata` 存文件信息和幂等键。幂等键还建立了表达式唯一索引。

### 10.6 分区表为什么用

项目对应：`subscriptions` 按 `expired_at` RANGE 分区。

适合：

- 按时间清理历史数据。
- 按时间查询有分区裁剪。
- 单表持续增长时降低维护成本。

雷点：

- 分区要持续维护未来分区。
- 唯一约束通常要包含分区键。

### 10.7 库表优化从哪里入手

回答顺序：

1. 先拿慢查询和真实接口，不凭感觉建索引。
2. 看 `WHERE`、`JOIN`、`ORDER BY`、`LIMIT`，设计能同时过滤和排序的复合索引。
3. 用 `EXPLAIN (ANALYZE, BUFFERS)` 验证是否命中索引、扫描行数、回表/排序成本。
4. 对高写入表评估索引写放大，索引不是越多越好。
5. 对长期增长表评估归档、分区、冷热数据和批处理游标。

项目例子：

- Market 列表：`status/category/create_at` 复合索引贴合筛选和倒序分页。
- Wallet 流水：`wallet_id, created_at DESC` 避免钱包内流水列表深排序。
- Order 超时取消：`status, created_at DESC` 支撑扫描 pending 订单，但如果取消 worker 只查 `pending + created_at < deadline`，后续可评估 `(status, created_at ASC)` 是否更贴合。
- Audit 导出：当前应用层 keyset cursor 已避免 offset 深分页，但还缺 `(created_at, event_id)` 索引。
- Subscriptions：按 `expired_at` 分区方便过期数据维护，但未来分区要持续创建，避免大量数据落 default。

### 10.8 Outbox Pattern 解决什么

基础问题：支付成功后发放权益，如果直接在支付事务里同步做所有事情，事务会变长，且外部依赖失败会影响扣款主链路；如果事务提交后再发消息，进程崩溃可能丢消息。

项目做法：

- 支付事务内完成扣款、订单 paid、钱包流水和 `entitlement_outbox` 插入。
- 事务提交后后台 worker 扫描 outbox。
- worker 用 `FOR UPDATE SKIP LOCKED` 抢任务，用 RedisLock 按 `order_id` 互斥。
- 发放订阅时用 `(user_id, app_id, source_order_id)` 唯一索引防重。

顶层结论：Outbox 把“支付事实”和“待发放事件”放进同一个数据库事务，解决本地事务和异步处理之间的可靠衔接。

项目里的 outbox 状态模型：

```text
pending -> processing -> done
pending -> processing -> failed -> processing -> done
```

字段语义：

- `event_id`：outbox 主键，标识一条待处理事件。
- `order_id`：关联订单，也是 RedisLock 的业务互斥粒度。
- `status`：`pending`、`processing`、`failed`、`done`。
- `retry_count`：失败重试次数。
- `next_run_at`：下一次可被 worker 扫描的时间。
- `locked_at`：被 worker 标记 processing 的时间。
- `last_error`：最近一次失败原因。

支付事务内发生什么：

1. `lockOrder` 对订单行 `FOR UPDATE`，防止重复支付并发竞争。
2. `DebitInTx` 在同一个事务内锁钱包行、扣余额、写钱包流水。
3. 插入 `entitlement_outbox(order_id, status='pending', next_run_at=now)`。
4. 更新订单为 `paid`，写入 `tx_id`。
5. 事务提交后，支付事实和待发放事件同时持久化。

worker 处理流程：

1. 周期扫描 `status IN ('pending','failed') AND next_run_at <= now()`。
2. 在 DB transaction 内用 `FOR UPDATE SKIP LOCKED` 锁住一批 outbox 行。
3. 将抢到的事件更新为 `processing`，避免其他 worker 重复处理。
4. 对单个 order 加 RedisLock：`entitlement:order:{order_id}`。
5. 开启 DB transaction，重新锁订单行并校验订单仍为 `paid`。
6. 创建 subscription，并回填 `orders.subscription_id`。
7. 更新 outbox 状态为 `done`。
8. 如果失败，更新为 `failed`，`retry_count + 1`，`next_run_at = now + retry_count^2 秒`。

为什么需要 `FOR UPDATE SKIP LOCKED`：

- 多 worker 可以并行扫描同一张 outbox 表。
- 普通 `FOR UPDATE` 遇到已锁行会等待，吞吐差。
- `SKIP LOCKED` 会跳过别人正在处理的行，直接抢下一批可处理任务。
- 它适合任务队列表，但不是严格公平队列，需要配合失败恢复和积压监控。

为什么还要 RedisLock：

- `SKIP LOCKED` 保护的是 outbox 行级抢占。
- RedisLock 保护的是业务维度的同一 `order_id`，防止重复事件、异常重试、多实例边界下同一订单被并发发放。
- RedisLock 不是最终一致性保证，只是降低重复处理概率和并发冲突。

为什么还要唯一索引：

- 锁都可能失效：Redis 锁可能过期，worker 可能崩溃，重复事件可能进入系统。
- 真正不可突破的防线应该在事实表上。
- 项目在 subscriptions 各分区建立 `(user_id, app_id, source_order_id)` partial unique index，保证同一订单只能发放一次权益。

Q：Outbox 和直接发 MQ 有什么区别？

A：直接发 MQ 有“本地事务提交成功但发消息失败”或“发消息成功但本地事务回滚”的一致性窗口。Outbox 把消息先写入同一个业务数据库事务，事务提交后再由 worker 异步处理，至少保证“业务事实存在时，待处理事件也存在”。后续投递或处理失败可以通过 outbox 状态重试。

Q：Outbox 会不会重复消费？

A：会，Outbox 通常保证的是 at-least-once，而不是 exactly-once。worker 失败重试、锁超时、进程重启都可能导致重复处理，所以消费逻辑必须幂等。本项目用 RedisLock 减少并发重复，用订单行锁校验状态，用 subscriptions 唯一索引做最终防重。

Q：processing 状态的任务如果 worker 崩溃怎么办？

A：这是当前实现需要继续强化的点。理想做法是增加 processing 超时恢复机制，例如扫描 `status='processing' AND locked_at < now()-timeout` 的任务，把它们重置为 failed 或 pending，并记录告警。否则 processing 任务可能长期卡住。

Q：Outbox 的缺点是什么？

A：它引入了额外表、worker、状态机、重试和监控复杂度；事件处理不是实时强同步，会有最终一致延迟；表会增长，需要归档或清理 done 历史；如果没有 backlog、失败率和处理耗时指标，线上排障会比较被动。

## 11. 当前 PostgreSQL 风险和优化点

- `wallets` 唯一约束建议改为 `(owner_type, owner_id, currency_code)`，更符合 owner 模型。
- `audit_events` 游标导出建议补 `(created_at, event_id)` 索引。
- Admin 多维过滤较多，可根据真实查询频率补复合索引。
- Market metadata 里存幂等键可用，但如果幂等成为核心字段，可以考虑独立列，查询和约束更直观。
- 订单金额当前由前端传入，数据库只校验 `amount > 0`，价格权威应迁移到后端价格表/SKU。
- `subscriptions` 未来分区需要自动创建策略，否则长期运行会大量落 default 分区。
- GORM 全局 DB 是单例，测试隔离和多租户场景需要注意。
