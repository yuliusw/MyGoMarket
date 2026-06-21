# AI Task Handoff: RPA-market 项目全量工程上下文

本文档为 `RPA-market` 项目的全局状态机描述。AI 在接手本会话或生成代码时，**必须严格按本文档声明的“现状”作为逻辑基线，严禁在未获明确指令前将“修改方向”中的设想幻觉为已实现功能。**

---

## 一、 项目概览与系统拓扑

`RPA-market` 是基于 Go + Vue 3 的 RPA 应用市场单体仓库（Monorepo）。

* **后端技术栈**：Gin、GORM、PostgreSQL、Redis、MinIO、RocketMQ、Casbin。
* **前端目录**：`frontend/RPA-market/`，基于 Vue 3 + Vite + TypeScript。
* **网络边界**：HTTP（默认端口 `:8080`）与 gRPC（默认端口 `:12661`，受配置项 `grpc.enabled` 与 `grpc.port` 控制）在同一进程中启动并存。

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
common/middleware/      JWT、Casbin、CORS、限流、trace、请求池
common/response/        统一标准错误响应体
common/audit/           审计日志异步攒批落库
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
* `[现状：存在局限]` 当前的 `JWTAuth` 中间件逻辑**强行同时绑定**了 JWT Token 与 Cookie 中的 `session_id`。纯 API 客户端若只携带 Bearer Token 而无 Cookie，会被判定为认证失败。生成开放 API 相关代码时需特别注意此兼容性瓶颈。

### 2. 功能开关与限流 (Features & RateLimit)

* `[现状：已落地]` `features` 配置项已支持 JWT、Casbin、CORS、限流、请求池等中间件的全局开关，并完全支持环境变量覆盖。
* `[现状：已落地]` 限流中间件具备 `memory` 与 `redis` 两种底层驱动；Market 公开接口、登录、注册路由已挂载限流器。全局请求池已接入，并发满载时快速拒绝并返回 `503`。
* `[压测约束]` 执行绕过认证的压测时，配置项 `auth_bypass_user_id` 必须传入数据库中真实存在的 UUID。默认取值需设为初始化脚本生成的管理员 ID：`00000000-0000-0000-0000-000000000001`。
* `[现状：存在局限]` 代码中虽实现了 Redis 滑动窗口限流，但**系统配置文件默认开启的驱动为 `memory**`；代码仓库中存在熔断器中间件 `broker.go`，但**该中间件从未在 `main.go` 或任何路由组中被调用挂载**，也未接入压测开关矩阵。

### 3. Casbin 权限与 RBAC

* `[现状：已落地]` 配置文件路径：`config/casbin/RBAC.conf`。已接入带 Domain 的 RBAC 模型（群组相关接口通过路由参数 `:id` 识别 Domain；无明确 Domain 的接口默认回落至系统全局域 `11111111-1111-1111-1111-111111111111`）。
* `[现状：已落地]` Casbin Manager 采用 `LRU 内存缓存池 + singleflight 并发抑制 + TTL 过期` 机制。首次访问某 Domain 时触发 `LoadFilteredPolicy` 按需加载。角色/权限变更时，清除本地缓存并向 RocketMQ 广播失效消息（支持识别 `invalidate_domain` 与 `purge_all` 指令）。
* `[现状：已落地]` 角色与权限基础管理接口已开通：
```text
GET /api/v1/iam/roles
GET /api/v1/iam/roles/:role_id
GET /api/v1/iam/permissions
PUT /api/v1/iam/roles/:role_id/permissions

```


* `[现状：存在局限]` Market 模块路由传参为 `:app_id`，当前 Casbin 中间件无法自动将其解析为资源域，因此 **Market 应用的编辑、下架、删除操作在 Casbin 层均回落为全局域校验**，资源级归属权完全依赖业务 Handler 内部进行 `developer_id` 比对。

### 4. 数据库 (PostgreSQL)

* `[现状：已落地]` 容器编排初始化脚本已变更为 `script/init_better.sql`（原 `script/init.sql` 已作废归档）。
* `[现状：已落地]` 脚本内已补全：业务表索引、分区定义、触发器、审计表、下载指标表、MinIO删除补偿表以及 `orders` 订单表。新建群组的创建者默认角色已由 `superadmin` 修正为 `owner`。
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
* `[现状：已落地·下载与统计]` 下载接口签发有效期为 5 分钟的 MinIO 预签名 URL 并返回 HTTP `307` 重定向；同步记录 `app_download_metrics` 持久化数据；通过 Redis Pipeline 批量累加 ZSET 排行榜：
```text
market:rank:downloads:daily:<yyyymmdd>
market:rank:downloads:weekly:<yyyyww>
market:rank:downloads:total

```


* `[现状：已落地·锁封装]` 工具包内已提供标准的分布式锁实现 `RedisLock`（基于 UUID Value + Lua Compare-And-Delete 脚本原子解锁）。
* `[现状：未落地·缓存空洞]` **应用详情接口 `GET /apps/:app_id` 当前为纯数据库裸查，未接入任何 Redis 缓存**。首页排行榜数据全量实时读 Redis ZSET，无 Cache-Aside 静态结构体缓存；详情回源无 singleflight 保护；无随机 TTL 防雪崩机制；无空值占位缓存或布隆过滤器防穿透机制。

### 6. 统一错误、审计与进程生命周期

* `[现状：已落地]` 统一使用 `common/response` 封装体响应，代码库中无裸露的 `gin.H{"error":...}`。
* `[现状：已落地]` 请求全链路自动注入 `X-Trace-ID`，发生异常时错误体的 `request_id` 强行复用 TraceID。
* `[现状：已落地]` Market 审计日志采用 `Channel + Ticker` 异步内存攒批落库，进程接收退出信号时触发 Flush 保证数据不丢。
* `[现状：已落地]` `main.go` 采用标准的 `http.Server` 实现 `SIGINT/SIGTERM` 优雅停机。资源释放链路严格遵循：关闭 HTTP $\rightarrow$ 停止请求池 $\rightarrow$ 断开 RocketMQ $\rightarrow$ 关闭 Redis 客户端 $\rightarrow$ 关闭 PostgreSQL 连接池。

### 7. gRPC、钱包与订单交易链路

* `[现状：已落地·契约与服务]` `proto/wallet/v1/wallet.proto` 及 Buf 构建配置已就绪，存根代码输出至 `gen/go`。`WalletService` 对外暴露 `GetWallet` / `GetOrCreateWallet` 存根接口；`WalletRepository` 支持按 `owner_type + owner_id + currency_code` 联合键查询或初始化钱包。
* `[现状：已落地·HTTP端点]` 暴露 `/api/v1/wallets/me`、流水列表（支持按钱包 ID 倒序分页）、充值、扣款、转账 API。订单系统暴露 purchase、创建、列表、详情、支付、取消 API。订单表具备 `pending` / `paid` / `cancelled` 状态枚举。
* `[现状：已落地·事务底层驱动]` 钱包扣款与订单支付在同一个 DB Transaction 内部强制执行 `SELECT ... FOR UPDATE` 锁住钱包行；调用 `DebitInTx` 完成扣款后记录流水（`reference_id` 存入 `order_id`），随后生成 `subscriptions` 订阅表记录并将订单状态流转为 `paid`。转账接口在事务加锁前，对参与转账的两个 `wallet_id` 字符串进行字典序排序后依次加锁，绝对规避死锁。具备幂等键防重扣机制，余额不足时直接回滚事务，不产生失败流水。数据串联外键为：`orders.subscription_id` $\leftrightarrow$ `subscriptions.source_order_id`。
* `[现状：未落地·高级交易特性]` **权益（订阅记录）的发放动作是在订单支付的 DB 事务内同步阻塞执行的，代码中不存在 RocketMQ 异步发放链路**；发券消费端未加装 Redis 分布式锁防重发；系统域内**仅存在订单（Order）概念，完全不存在“工单（Work Order）”概念**；未实现超时未支付订单自动取消机制；无 Redis Lua 库存预扣减机制。

### 8. 大数据量导出与流式处理

* `[现状：已落地]` 审计日志 CSV 导出服务（`common/audit/export.go`）基于 `io.Pipe` 实现了流式边查边写边透传 HTTP 响应。底层采用 Keyset 游标分页（游标组合键为 `created_at, event_id`），单次攒批查询 500 条。
* `[现状：未落地]` 该流式驱动为**纯应用层 Keyset 内存游标，并非 PostgreSQL 服务端游标 `DECLARE CURSOR**`；导出的流数据直接推给 HTTP 响应体，未实现同时双写转存至 MinIO；支持导出的底层表仅限 `audit_events`，不包含钱包或订单流水。

### 9. 可观测性与前端工作台物料

* `[现状：已落地·可观测性]` 基础环境通过 Prometheus 抓取 `cAdvisor` 容器级硬件指标；压测容器配置 `K6_PROMETHEUS_RW_SERVER_URL` 将 k6 内部测试指标 Remote Write 推送至 Prometheus；`run-load-tests.sh` 脚本对每个 Case 抽样录制 `docker stats` 并输出 CSV 文件；由 Alloy 容器抓取应用 stdout 原始日志推入 Loki；Grafana 已预置对应的数据源与基础仪表盘。
* `[现状：已落地·前端工程]` 路由完全由显式配置表驱动（包含 `/market`, `/wallet`, `/orders`, `/login`）。后端请求层统一封装于 `src/api.ts`。`AppMarket.vue` 实现了当前完整的购买链路交互（成功后展示 `order_id`, `tx_id`, `subscription_id`）。具备独立的 `Wallet.vue` 与 `Orders.vue` 面板。
* `[现状：未落地]` k6 的压测指标只推给 Prometheus，Loki 内无指标数据；**Go 业务应用本身未注册暴露 `/metrics` 埋点端点**；链路追踪仅停留在日志 TraceID 串联阶段，未生成 OpenTelemetry 标准的 Span 数据；**代码库中尚无官方出具的标准压测性能报告文档**。

---

## 三、 核心模块演进方向（P0级需求映射）

AI 在生成代码时，默认必须按照“现状描述”中的方式编写逻辑；只有在用户明确下达“优化/重构某模块”的指令时，方可按以下“修改方向”作为目标进行代码输出。

### 1. 应用详情缓存模块 (App Detail Cache)

* **现状描述**：`GET /api/v1/market/apps/:app_id` 直接请求 PostgreSQL 数据库，无任何缓存介入。
* **修改方向**：引入标准的 Cache-Aside 模式。在 Redis 中建立 `market:app:detail:{app_id}` 缓存结构体；当 Cache Miss 触发 DB 回源时，必须外挂 `golang.org/x/sync/singleflight` 抑制并发回源请求；写入 Redis 的 TTL 必须采用“基础时间 + 随机扰动”算法（如 `5m + rand(0~60s)`）防止缓存雪崩；对数据库中查无实体的 `app_id`，强制写入短 TTL（如 `30s`）的空值标记，防御穿透攻击。

### 2. 业务指标埋点模块 (Application Metrics Scrape)

* **现状描述**：监控系统仅能通过 cAdvisor 获知容器级别的 CPU/内存占用，无法探知 Go 应用内部的运行状态。
* **修改方向**：引入 `github.com/prometheus/client_golang/prometheus` 库。在 HTTP 路由层注册单独的 `/metrics` 端点；加装全局 Prometheus Middleware，实时采集并上报：HTTP 请求总数（Counter）、接口响应耗时分布（Histogram，按照 Path/Status 聚合）、当前活跃 Goroutine 数量、GORM 连接池状态（`sql.DB.Stats()`）、请求池拒绝任务总数。

### 3. 交易链路解耦模块 (Asynchronous Entitlement Issuance)

* **现状描述**：扣减钱包余额、修改订单状态、向用户发放 `subscriptions` 权益在同一个 PostgreSQL 长事务中同步阻塞执行。
* **修改方向**：引入本地消息表（Outbox Pattern）或 RocketMQ 事务消息。将支付事务缩减为“扣款 + 订单改状态 + 记录 Outbox”，事务提交后异步触发权益发放消费端。消费端处理逻辑必须加装基于 RedisLock 的分布式互斥锁，并在数据库层面以 `(user_id, app_id, source_order_id)` 唯一复合索引作为最终防重发兜底。

### 4. 压测基线沉淀模块 (Load Test Benchmarking)

* **现状描述**：备有 k6 压测夹具脚本，但缺乏可信赖的、量化的工程压测结论文件。
* **修改方向**：在根目录建立 `docs/benchmark_v1.md` 标准压测报告。报告模板内容必须强制包含：测试执行器硬件规格、压测目标接口 URL、功能开关状态组合矩阵（如 `JWT=on, Casbin=on, RateLimit=redis`）、并发线程数（VUs）、稳定持续时长、最终产出的 QPS 均值、P50/P95/P99 耗时水位线、以及压测期间 PostgreSQL 与 Redis 连接池的资源消耗极值峰值。

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
# 输出结果：PASS

docker exec -i "RPA-pgdb" psql -U "rpa_app" -d "RPA" -v ON_ERROR_STOP=1 < "script/init_better.sql"
# 输出结果：PASS (无任何 DDL 报错或主键冲突)

npm --prefix frontend/RPA-market run build
# 输出结果：dist 物料打包成功，TypeScript 静态类型检查无遗漏警告

```

*(注：AI 在本会话中生成任何带有破坏性的重构代码前，建议提示开发者优先执行上述三步快照命令，以验明本地工作区处于纯净无污染状态。)*