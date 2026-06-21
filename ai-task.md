# AI Task Handoff: RPA-market 项目全量工程上下文

本文档为 `RPA-market` 项目的全局状态机描述。AI 在接手本会话或生成代码时，**必须严格按本文档声明的“现状”作为逻辑基线，严禁在未获明确指令前将“修改方向”中的设想幻觉为已实现功能。**

---

## 一、 项目概览与系统拓扑

`RPA-market` 是基于 Go + Vue 3 的 RPA 应用市场单体仓库（Monorepo）。

* **后端技术栈**：Gin、GORM、PostgreSQL、Redis、MinIO、RocketMQ、Casbin。
* **前端目录**：`frontend/RPA-market/`，基于 Vue 3 + Vite + TypeScript。
* **网络边界**：HTTP（默认端口由 `server.port` 控制，当前配置为 `:12660`）与 gRPC（默认端口 `:12661`，受配置项 `grpc.enabled` 与 `grpc.port` 控制）在同一进程中启动并存。

**启动入口**：

```text
main.go
app.go

```

**启动链路**（严格按以下顺序初始化）：

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

### 目录结构与物理契约

```text
common/config/          配置加载，含 features 开关
common/database/        PostgreSQL、Redis、MinIO 初始化
common/middleware/      JWT、Casbin、CORS、限流、trace、请求池、熔断器
common/response/        统一标准错误响应体
common/audit/           审计日志异步攒批落库
common/metrics/         Prometheus 指标注册与 HTTP/GORM 采集
common/utils/           JWT、Hash、Casbin Manager、锁、令牌桶
common/grpcserver/      gRPC server 启动生命周期封装

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

* **【严禁事项】**：`services/market/respository/` 目录名的拼写错误为历史既定契约，**严禁对其进行重命名或纠正**。
* **【架构约束】**：当前维持单体仓库架构，gRPC 仅作为模块边界及未来拆分准备。**严禁在代码中引入任何形式的微服务动态服务发现机制。**

---

## 二、 领域功能现状核对表 (Ground Truth)

### 1. 认证与会话 (Auth & Session)

* `[现状：已落地]` **JWT (`HS256`) + Redis Session 混合双检机制**。登录操作必须成功将 Session 写入 Redis，否则强行返回 `503`。登出、修改密码操作会同步清除 `auth_token` 与 Redis 中的 `session_id`。
* `[现状：已落地]` `JWTAuth` 支持浏览器 Cookie 模式与纯 API Bearer 模式。Cookie 模式继续校验 Redis 中的 `session_id` 防顶号；纯 API 客户端只要携带有效 `Authorization: Bearer <token>` 即可通过，不再强制要求 `session_id` Cookie。过期 JWT 的自动续签仍必须依赖 Redis Session 与 Cookie 一致。

### 2. 功能开关与限流 (Features & RateLimit)

* `[现状：已落地]` `features` 配置项已支持 JWT、Casbin、CORS、限流、请求池、熔断器、订单超时取消等能力的全局开关或参数配置，并完全支持环境变量覆盖。
* `[现状：已落地]` 限流中间件具备 `memory` 与 `redis` 两种底层驱动；Market 公开接口、登录、注册路由已挂载限流器。全局请求池已接入，并发满载时快速拒绝并返回 `503`。
* `[压测约束]` 执行绕过认证的压测时，配置项 `auth_bypass_user_id` 必须传入数据库中真实存在的 UUID。默认取值需设为初始化脚本生成的管理员 ID：`00000000-0000-0000-0000-000000000001`。
* `[现状：已落地]` 熔断器中间件 `broker.go` 已通过 `ConfiguredCircuitBreaker()` 挂载到全局 Gin 链路，配置项为 `features.circuit_breaker.enabled/max_failures/timeout_seconds`。Redis 滑动窗口限流仍已实现，但系统配置文件默认驱动仍为 `memory`，需要压测 Redis 限流时显式设置 `features.rate_limit.backend=redis`。

### 3. Casbin 权限与 RBAC

* `[现状：已落地]` 配置文件路径：`config/casbin/RBAC.conf`。已接入带 Domain 的 RBAC 模型（群组相关接口通过路由参数 `:id` 识别 Domain；Market 应用相关接口可通过 `:app_id` 识别资源域；无明确 Domain 的接口默认回落至系统全局域 `11111111-1111-1111-1111-111111111111`）。
* `[现状：已落地]` Casbin Manager 采用 `LRU 内存缓存池 + singleflight 并发抑制 + TTL 过期` 机制。首次访问某 Domain 时触发 `LoadFilteredPolicy` 按需加载。角色/权限变更时，清除本地缓存并向 RocketMQ 广播失效消息（支持识别 `invalidate_domain` 与 `purge_all` 指令）。
* `[现状：已落地]` 角色与权限基础管理接口已开通：
```text
GET /api/v1/iam/roles
GET /api/v1/iam/roles/:role_id
GET /api/v1/iam/permissions
PUT /api/v1/iam/roles/:role_id/permissions

```


* `[现状：已落地]` Casbin 中间件已支持从 `:app_id` 解析 Market 应用资源域。Market 应用的编辑、下架、删除操作在 Casbin 层可按应用域校验，同时业务 Handler 仍保留 `developer_id` 所有者比对作为最终兜底。

### 4. 数据库 (PostgreSQL)

* `[现状：已落地]` 容器编排初始化脚本已变更为 `script/init_better.sql`（原 `script/init.sql` 已作废归档）。
* `[现状：已落地]` 脚本内已补全：业务表索引、分区定义、触发器、审计表、下载指标表、MinIO删除补偿表、`orders` 订单表以及 `entitlement_outbox` 本地消息表。新建群组的创建者默认角色已由 `superadmin` 修正为 `owner`。
* **【严禁事项】**：**严禁通过生成 DDL/DML 语句对当前数据库内的测试数据进行物理清除。**

### 5. Market 核心能力与底层驱动

* **已开放的标准 API**：
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


* `[现状：已落地·上传链路]` 严格校验 `.zip`、`.gz`、`.tgz` 后缀及对应文件头魔数；MinIO ObjectName 使用服务端生成的纯 UUID 路径；原始文件名仅存入 DB Metadata 并作路径安全与 UTF-8 规范化处理；支持 HTTP 请求头 `Idempotency-Key` / `idempotency_key` 幂等检测；底层由 Redis `SetNX` 短锁与 DB 唯一索引提供双重并发防护；DB 写入失败时同步触发 MinIO 对象删除补偿并记录审计日志。
* `[现状：已落地·变更链路]` 更新、下架、删除操作严格校验 `developer_id`。更新数据库行时强制使用 GORM 的 `Updates` 字段级更新，严禁使用 `Save` 以防并发覆盖。删除 MinIO 对象失败的记录会落入 `minio_delete_retries` 表由后台 Worker 异步重试。
* `[现状：已落地·下载与统计]` 下载接口签发有效期为 5 分钟的 MinIO 预签名 URL 并返回 HTTP `307` 重定向；同步记录 `app_download_metrics` 持久化数据；通过 Redis Pipeline 批量累加 ZSET 排行榜，并在默认热榜缓存键上做短 TTL 失效：
```text
market:rank:downloads:daily:<yyyymmdd>
market:rank:downloads:weekly:<yyyyww>
market:rank:downloads:total

```


* `[现状：已落地·锁封装]` 工具包内已提供标准的分布式锁实现 `RedisLock`（基于 UUID Value + Lua Compare-And-Delete 脚本原子解锁）。
* `[现状：已落地·缓存治理]` 应用详情接口 `GET /apps/:app_id` 已采用 Cache-Aside：Redis Key 为 `market:app:detail:{app_id}`，回源使用 `golang.org/x/sync/singleflight` 抑制并发击穿，正常 TTL 为 `5m + rand(0~60s)`，查无实体会写入 `30s` 空值标记防穿透；发布、更新、下架、删除后会主动删除详情缓存。首页排行榜接口已增加 `market:rank:cache:{type}:{limit}` 短 TTL 结构体缓存，默认 TTL 10 秒。

### 6. 统一错误、审计与进程生命周期

* `[现状：已落地]` 统一使用 `common/response` 封装体响应，代码库中无裸露的 `gin.H{"error":...}`。
* `[现状：已落地]` 请求全链路自动注入 `X-Trace-ID`，发生异常时错误体的 `request_id` 强行复用 TraceID。
* `[现状：已落地]` Market 审计日志采用 `Channel + Ticker` 异步内存攒批落库，进程接收退出信号时触发 Flush 保证数据不丢。
* `[现状：已落地]` `main.go` 采用标准的 `http.Server` 实现 `SIGINT/SIGTERM` 优雅停机。资源释放链路严格遵循：关闭 HTTP $\rightarrow$ 停止请求池 $\rightarrow$ 停止 MinIO 删除重试 Worker 与订单异步 Worker $\rightarrow$ Flush 审计日志 $\rightarrow$ 断开 RocketMQ $\rightarrow$ 关闭 Redis 客户端 $\rightarrow$ 关闭 PostgreSQL 连接池。

### 7. gRPC、钱包与订单交易链路

* `[现状：已落地·契约与服务]` `proto/wallet/v1/wallet.proto` 及 Buf 构建配置已就绪，存根代码输出至 `gen/go`。`WalletService` 对外暴露 `GetWallet` / `GetOrCreateWallet` 存根接口；`WalletRepository` 支持按 `owner_type + owner_id + currency_code` 联合键查询或初始化钱包。
* `[现状：已落地·HTTP端点]` 暴露 `/api/v1/wallets/me`、流水列表（支持按钱包 ID 倒序分页）、充值、扣款、转账 API。订单系统暴露 purchase、创建、列表、详情、支付、取消 API。订单表具备 `pending` / `paid` / `cancelled` 状态枚举。
* `[现状：已落地·事务底层驱动]` 钱包扣款与订单支付在同一个 DB Transaction 内部强制执行 `SELECT ... FOR UPDATE` 锁住钱包行；调用 `DebitInTx` 完成扣款后记录流水（`reference_id` 存入 `order_id`），订单状态流转为 `paid` 并写入 `entitlement_outbox`，事务提交后由后台 Worker 异步发放 `subscriptions` 权益并回填 `orders.subscription_id`。转账接口在事务加锁前，对参与转账的两个 `wallet_id` 字符串进行字典序排序后依次加锁，绝对规避死锁。具备幂等键防重扣机制，余额不足时直接回滚事务，不产生失败流水。数据串联外键为：`orders.subscription_id` $\leftrightarrow$ `subscriptions.source_order_id`。
* `[现状：已落地·高级交易特性]` 权益发放采用本地消息表 Outbox Pattern，后台消费者通过 RedisLock 按 `order_id` 加分布式互斥锁，并在订阅分区表上以 `(user_id, app_id, source_order_id)` 唯一索引兜底防重发。pending 订单超时自动取消 Worker 已落地，配置项为 `features.order.pending_timeout_seconds` 与 `features.order.cancel_scan_seconds`。系统域内仍仅存在订单（Order）概念，完全不存在“工单（Work Order）”概念；因当前无库存模型，未实现 Redis Lua 库存预扣减机制。

### 8. 大数据量导出与流式处理

* `[现状：已落地]` 审计日志 CSV 导出服务（`common/audit/export.go`）基于 `io.Pipe` 实现了流式边查边写边透传 HTTP 响应。底层采用 Keyset 游标分页（游标组合键为 `created_at, event_id`），单次攒批查询 500 条。
* `[现状：已落地]` 审计导出已支持 HTTP 流式响应与 MinIO 双写转存，MinIO ObjectName 前缀为 `audit-exports/`，响应头返回 `X-MinIO-Object`。该流式驱动仍为应用层 Keyset 内存游标，并非 PostgreSQL 服务端游标 `DECLARE CURSOR`；支持导出的底层表仍限 `audit_events`，钱包和订单流水通过 Admin 查询接口读取，尚未提供 CSV 导出。

### 9. 可观测性与前端工作台物料

* `[现状：已落地·可观测性]` 基础环境通过 Prometheus 抓取 `cAdvisor` 容器级硬件指标；压测容器配置 `K6_PROMETHEUS_RW_SERVER_URL` 将 k6 内部测试指标 Remote Write 推送至 Prometheus；`run-load-tests.sh` 脚本对每个 Case 抽样录制 `docker stats` 并输出 CSV 文件；由 Alloy 容器抓取应用 stdout 原始日志推入 Loki；Grafana 已预置对应的数据源与基础仪表盘。
* `[现状：已落地·前端工程]` 路由完全由显式配置表驱动（包含 `/market`, `/wallet`, `/orders`, `/login`）。后端请求层统一封装于 `src/api.ts`。`AppMarket.vue` 实现了当前完整的购买链路交互（成功后展示 `order_id`, `tx_id`, `subscription_id`）。具备独立的 `Wallet.vue` 与 `Orders.vue` 面板。
* `[现状：已落地]` Go 业务应用已注册 `/metrics` 端点并接入 Prometheus client。当前采集 HTTP 请求总数、HTTP 响应耗时 Histogram、GORM 连接池状态与请求池拒绝总数；Go runtime 默认 collector 可提供 goroutine 等运行时指标。根目录已新增 `docs/benchmark_v1.md` 标准压测报告模板。k6 指标仍只推给 Prometheus，Loki 内无指标数据；链路追踪仍停留在日志 TraceID 串联阶段，未生成 OpenTelemetry 标准 Span 数据。

---

## 三、 核心模块演进方向（P0级需求映射）

AI 在生成代码时，默认必须按照“现状描述”中的方式编写逻辑；只有在用户明确下达“优化/重构某模块”的指令时，方可按以下“修改方向”作为目标进行代码输出。

### 1. 应用详情缓存模块 (App Detail Cache)

* **落地状态**：已完成。`GET /api/v1/market/apps/:app_id` 已采用 Cache-Aside 模式，Redis Key 为 `market:app:detail:{app_id}`；Cache Miss 回源通过 `singleflight` 合并并发请求；TTL 使用 `5m + rand(0~60s)`；查无实体会写入 `30s` 空值标记。

### 2. 业务指标埋点模块 (Application Metrics Scrape)

* **落地状态**：已完成。已引入 `github.com/prometheus/client_golang/prometheus` 并注册 `/metrics`；全局 Prometheus Middleware 采集 HTTP 请求总数与响应耗时；GORM 连接池状态通过 `sql.DB.Stats()` 周期采集；请求池拒绝任务总数通过 `rpa_request_pool_rejected_total` 暴露；goroutine 等运行时指标由 Prometheus Go runtime collector 暴露。

### 3. 交易链路解耦模块 (Asynchronous Entitlement Issuance)

* **落地状态**：已完成。支付事务已缩减为“扣款 + 订单改状态 + 记录 `entitlement_outbox`”；事务提交后由后台 Worker 异步触发权益发放；消费端基于 RedisLock 做分布式互斥，并以订阅分区表上的 `(user_id, app_id, source_order_id)` 唯一索引作为最终防重发兜底。

### 4. 压测基线沉淀模块 (Load Test Benchmarking)

* **落地状态**：已完成。根目录已建立 `docs/benchmark_v1.md` 标准压测报告模板，包含测试执行器硬件规格、压测目标接口 URL、功能开关状态组合矩阵、VUs、持续时长、QPS、P50/P95/P99、PostgreSQL 与 Redis 资源峰值等字段。

---

## 四、 常用命令与最近基线验证快照

### 常用命令速查

```bash
# 1. 运行全量后端单元测试基线校验
go test ./...

# 2. 编译构建前端工作台生产物料
npm --prefix frontend/RPA-market run build

# 3. 打印本地 Docker 容器矩阵运行拓扑
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'

# 4. 探查 PostgreSQL 当前 Public 模式下所有物理表名
docker exec "RPA-pgdb" psql -U "rpa_app" -d "RPA" -c "SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY table_name;"

# 5. 纯净重放主数据库初始化脚本（用于严格检验DDL幂等性）
docker exec -i "RPA-pgdb" psql -U "rpa_app" -d "RPA" -v ON_ERROR_STOP=1 < "script/init_better.sql"

# 6. 基于 Protobuf 契约全量重新生成 Go 语言存根代码
$(go env GOPATH)/bin/buf generate

```

*(附：Buf 构建工具链环境缺失时的补全安装命令)*

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
go install github.com/bufbuild/buf/cmd/buf@v1.59.0

```

### 最近一次本地代码盘面验证快照（执行于交接前夕）

```bash
go test ./...
# 输出结果：PASS (本轮后端变更后已复核通过)

docker exec -i "RPA-pgdb" psql -U "rpa_app" -d "RPA" -v ON_ERROR_STOP=1 < "script/init_better.sql"
# 输出结果：PASS (本轮新增 entitlement_outbox、订阅唯一索引等 DDL 后已复核通过，无任何 DDL 报错或主键冲突)

npm --prefix frontend/RPA-market run build
# 输出结果：dist 物料打包成功，TypeScript 静态类型检查无遗漏警告

```

*(注：AI 在本会话中生成任何带有破坏性的重构代码前，建议提示开发者优先执行上述三步快照命令，以验明本地工作区处于纯净无污染状态。)*
