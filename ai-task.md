# AI Task Handoff

本文用于新会话快速接手 `RPA-market` 项目。详细修复复盘和面试八股见 `归档.md`，本文只保留当前状态、关键注意点、待办和常用命令。

## 项目概览

`RPA-market` 是 Go + Vue 的 RPA 应用市场单体仓库。

后端技术栈：Gin、GORM、PostgreSQL、Redis、MinIO、RocketMQ、Casbin。

前端目录：`frontend/RPA-market/`，使用 Vue 3 + Vite + TypeScript。

启动入口：

```text
main.go
app.go
```

启动链路：

```text
config.InitConfig
database.InitGORM
database.InitRedis
database.InitMinio
rocketmq.Init
utils.InitCasbinPool
iam.RegisterHandlers
market.RegisterMarketHandlers
```

## 关键目录

```text
common/config/          配置加载，含 features 开关
common/database/        PostgreSQL、Redis、MinIO 初始化
common/middleware/      JWT、Casbin、CORS、限流、trace、请求池
common/response/        统一错误响应
common/audit/           审计异步批量落库
common/utils/           JWT、Hash、Casbin Manager、锁、令牌桶
common/grpcserver/      gRPC server 启动封装

services/iam/           登录、注册、用户资料、群组、RBAC
services/market/        应用发布、更新、下架、列表、详情、下载、删除
services/wallet/        钱包领域雏形
services/entitlement/   权益领域雏形

script/docker-compose.yaml
script/init_better.sql  当前主数据库初始化脚本
script/init.sql         旧脚本，已归档
script/k6-scripts/      k6 压测脚本

proto/wallet/v1/        wallet gRPC proto
gen/go/                 proto 生成代码
```

注意：`services/market/respository/` 目录名拼写保持现状，不要随手改名。

## 当前已完成

### 认证与会话

- JWT + Redis Session 混合认证已完成。
- 登录必须成功写入 Redis Session，否则返回 `503`。
- 登出、改密会清理 `auth_token` 和 `session_id`。
- JWT 解析限制 `HS256`。
- 当前 JWTAuth 同时依赖 JWT 和 `session_id` Cookie。

注意：纯 API 客户端如果只带 Bearer Token、不带 `session_id` Cookie，可能无法通过认证；如需支持需重新设计兼容策略。

### 功能开关与限流

- `features` 配置已支持 JWT、Casbin、CORS、限流、请求池等开关。
- 环境变量覆盖已支持。
- 限流支持 `memory` 和 `redis` 两种后端。
- Market 公开接口、登录、注册已挂限流。
- 全局请求池已接入，池满返回 `503` 快速失败。

压测绕过认证时注意：`auth_bypass_user_id` 必须是真实存在的 UUID。默认使用初始化脚本里的管理员 `00000000-0000-0000-0000-000000000001`。

### Casbin 权限

- Casbin model 路径配置化：`config/casbin/RBAC.conf`。
- Domain RBAC 已接入。
- Casbin Manager 使用 LRU + singleflight + TTL。
- 权限变更会本地失效并通过 RocketMQ 广播。
- 角色/权限接口已完成：

```text
GET /api/v1/iam/roles
GET /api/v1/iam/roles/:role_id
GET /api/v1/iam/permissions
PUT /api/v1/iam/roles/:role_id/permissions
```

注意：Market app 权限存在全局域 fallback；资源级限制目前主要靠 `developer_id` 所有者校验。

### 数据库

- Compose 已改用 `script/init_better.sql`。
- `script/init.sql` 已标记归档。
- DB 初始化脚本已补索引、分区、trigger、审计表、下载指标表、MinIO 删除补偿表。
- 新建群组创建者角色已从 `superadmin` 修复为 `owner`。
- 当前 DB 不要轻易删除测试数据。

### Market 能力

已有接口：

```text
GET    /api/v1/market/apps
GET    /api/v1/market/apps/:app_id
GET    /api/v1/market/apps/:app_id/download
GET    /api/v1/market/rankings
POST   /api/v1/market/apps
PUT    /api/v1/market/apps/:app_id
PUT    /api/v1/market/apps/:app_id/offshelf
DELETE /api/v1/market/apps/:app_id
```

已完成重点：

- 上传支持 `.zip`、`.gz`、`.tgz` 白名单和文件头校验。
- 上传 object name 使用服务端 UUID 路径。
- 原始文件名只进入 metadata，已做路径和 UTF-8 安全处理。
- 支持 `Idempotency-Key` / `idempotency_key`。
- Redis 短锁 + DB 唯一索引兜底上传幂等。
- DB 写入失败会补偿删除 MinIO 对象并写审计。
- 下载使用 5 分钟 MinIO 预签名 URL，返回 `307`。
- 下载写 Redis ZSET 热榜和 `app_download_metrics` 持久化指标。
- 更新/下架/删除已做 `developer_id` 所有者校验。
- 更新已改为字段级 `Updates`，避免 `Save` lost update。
- 删除 MinIO 失败会入 `minio_delete_retries`，后台 worker 重试。
- Market 与鉴权错误已迁移到统一 `code/message/request_id`。

热榜 Redis key：

```text
market:rank:downloads:daily:<yyyymmdd>
market:rank:downloads:weekly:<yyyyww>
market:rank:downloads:total
```

### 统一错误、审计和退出

- `common/response` 已接入，历史 `gin.H{"error":...}` 已清理。
- `X-Trace-ID` 自动生成并回写，错误响应 `request_id` 复用 trace id。
- Market 审计使用 channel + ticker 批量落库，退出时 flush。
- `main.go` 已改为 `http.Server`，支持 `SIGINT/SIGTERM` 优雅退出。
- 优雅退出会关闭 HTTP server、请求池、RocketMQ、Redis、PostgreSQL。

### gRPC 与钱包

- `proto/wallet/v1/wallet.proto` 已新增。
- `buf.yaml` / `buf.gen.yaml` 已新增。
- 生成代码输出到 `gen/go`。
- `common/grpcserver` 已新增。
- `main.go` 同进程启动 HTTP + gRPC，默认 gRPC 端口 `12661`。
- `grpc.enabled` / `grpc.port` 配置和环境变量覆盖已支持。
- `WalletService` 已暴露 `GetWallet` / `GetOrCreateWallet`。
- `WalletRepository` 已支持按 `owner_type + owner_id + currency_code` 查询/创建钱包。
- 钱包 HTTP API 已新增：`/api/v1/wallets/me`、流水列表、充值、扣款、转账。
- 钱包写操作已使用 DB transaction + `SELECT ... FOR UPDATE` 锁钱包行；`idempotency_key` 支持防重复扣款/转账，余额不足不写流水。

### 订单系统

- `orders` 表已加入 `script/init_better.sql`，支持 `pending / paid / cancelled` 状态、支付流水 `tx_id`、创建幂等键。
- 订单 HTTP API 已新增：购买、创建、列表、详情、支付、取消。
- 支付订单会在同一个 DB transaction 内锁订单行，调用钱包 `DebitInTx` 扣款并写 `wallet_transactions.reference_id = order_id`，再发放 `subscriptions` 并把订单置为 `paid`。
- `orders.subscription_id` 与 `subscriptions.source_order_id` 已用于串联订单和发放结果。

### 前端工作台

- 前端路由已从扫描 `components/*.vue` 改为显式菜单：`/market`、`/wallet`、`/orders`、`/login`。
- 新增 `src/api.ts`，集中封装 Market、Wallet、Order API。
- `AppMarket.vue` 已重写为当前购买链路页面，支持应用列表、发布、下架、删除、购买并展示 `order_id / tx_id / subscription_id`。
- 新增 `Wallet.vue`，支持查看当前钱包、测试充值、查看流水。
- 新增 `Orders.vue`，支持查看订单和订阅发放结果。
- 主布局和全局样式已替换为当前工作台风格。

注意：当前还是单体仓库，gRPC 只是模块边界和后续拆分准备，不要过早引入服务发现。

## 当前待办

### P1 压测归档

- Market 多场景压测结果还需要整理归档。
- 建议覆盖：list/ranking、publish、download、请求池过载、Redis 限流、Casbin on/off。

已有脚本：

```text
script/k6-scripts/test.js
script/k6-scripts/market.js
```

### P2 钱包交易

已有领域和表：

```text
services/wallet/domain/wallet.go
services/wallet/domain/transaction.go
wallets
wallet_transactions
```

已推进：

```text
1. TransactionRepository 能力已并入 WalletRepository
2. Credit / Debit / Transfer 应用服务已通过 HTTP handler 暴露
3. 流水列表已支持按钱包倒序分页
```

后续建议实现顺序：

```text
1. 并发与幂等测试
2. 钱包 gRPC 写接口
```

### P2 订单系统

已推进：

```text
services/order/domain/order.go
services/order/repository/order_repository.go
services/order/app/http_service.go
services/order/router.go
orders
subscriptions.source_order_id
```

已支持 API：

```text
POST /api/v1/orders/purchase
POST /api/v1/orders
GET  /api/v1/orders
GET  /api/v1/orders/:order_id
POST /api/v1/orders/:order_id/pay
POST /api/v1/orders/:order_id/cancel
```

后续建议：

```text
1. 商品价格从 app metadata 或独立 sku/price 表派生，避免客户端传 amount
2. 团队购买场景再对接 group_entitlements
3. 补订单并发支付、幂等支付、余额不足不写流水、订阅发放测试
```

### P2 前端更新

已推进：

```text
frontend/RPA-market/src/router/index.ts
frontend/RPA-market/src/api.ts
frontend/RPA-market/src/layout/MainLayout.vue
frontend/RPA-market/src/components/AppMarket.vue
frontend/RPA-market/src/components/Wallet.vue
frontend/RPA-market/src/components/Orders.vue
```

验证：

```bash
npm run build
```

结果：通过。

后续建议：

```text
1. 增加用户态 store，避免页面间重复拉取 profile/wallet
2. 增加购买前余额不足引导充值流程
3. 接入 app.metadata.price 或后端价格接口，移除前端默认 10 COIN
```

关键要求：

- 扣款不能“读余额 -> 判断 -> Save”。
- 必须用事务 + `SELECT ... FOR UPDATE`，或单条 `UPDATE ... WHERE balance >= ?`。
- `idempotency_key` 用于防止重复扣款。
- 余额不足不写流水。
- 流水记录 `balance_after`、`reference_id`、`description`、`idempotency_key`。

建议 API：

```text
GET  /api/v1/wallets/me
GET  /api/v1/wallets/:wallet_id/transactions
POST /api/v1/wallets/:wallet_id/credit
POST /api/v1/wallets/:wallet_id/debit
POST /api/v1/wallets/transfer
```

### P2 gRPC 扩展

- 补 market/v1、iam/v1 proto 包。
- 补 gRPC unary interceptor。
- 统一 trace_id/request_id metadata。
- 钱包 `Credit / Debit / Transr` 完成后再暴露写接口。

## 常用命令

Go 测试：

```bash
go test ./...
```

查看容器：

```bash
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
```

查询 DB 表：

```bash
docker exec "RPA-pgdb" psql -U "rpa_app" -d "RPA" -c "SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY table_name;"
```

重放初始化脚本验证幂等：

```bash
docker exec -i "RPA-pgdb" psql -U "rpa_app" -d "RPA" -v ON_ERROR_STOP=1 < "script/init_better.sql"
```

生成 proto：

```bash
$(go env GOPATH)/bin/buf generate
```

如缺工具：

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
go install github.com/bufbuild/buf/cmd/buf@v1.59.0
```

## 最近验证

```bash
go test ./...
```

结果：通过。

```bash
docker exec -i "RPA-pgdb" psql -U "rpa_app" -d "RPA" -v ON_ERROR_STOP=1 < "script/init_better.sql"
```

结果：通过。
