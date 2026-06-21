# RPA-market Benchmark v1

## 测试结论

本文件为 P0 压测基线报告。每次正式压测后必须固化执行环境、功能开关矩阵、接口目标、吞吐延迟结果以及 PostgreSQL/Redis 资源峰值。

2026-06-21 固化了一轮 `50 VUs / 20s` 固定并发单接口压测。当前 App 为单容器、2 CPU 限额，应用容器在多数读接口压测中接近吃满 2 核；PostgreSQL 容器未设置同等 CPU 限额，峰值可超过 100%。本轮运行暴露了脚本环境变量口径问题：manifest 记录的 `FEATURE_RATE_LIMIT_ENABLED=false` 未等价于 compose 实际使用的 `FEATURES_RATE_LIMIT_ENABLED=false`，导致本轮最终 App 容器实际仍启用限流。脚本已修复为同时支持 `FEATURE_*` 与 `FEATURES_*`，并会在 manifest 中追加 App 容器实际 `FEATURES_*` 环境变量。

本轮可直接参考 0 错误率接口的并发吞吐与延迟；`iam_register`、`iam_login`、Market 公开读接口由于实际限流开启，错误率接近 100%，这些结果反映的是限流/快速拒绝路径，不代表业务成功吞吐上限。

2026-06-21 追加固化 `jwt-casbin-mq-rl-off-50vu-20260621-165020`：关闭限流，开启 JWT、Casbin、请求池与 MQ，执行 `50 VUs / 60s` 固定并发单接口压测。manifest 中两次 App 重启后的容器实际环境均为 `FEATURES_JWT_AUTH=true`、`FEATURES_CASBIN_AUTHZ=true`、`FEATURES_RATE_LIMIT_ENABLED=false`、`FEATURES_REQUEST_POOL_ENABLED=true`。本轮修复了 k6 认证态传递：登录后显式携带 `Authorization: Bearer` 与 `Cookie: auth_token=...; session_id=...`。公开接口、登录/注册、Profile 等接口可作为有效业务压测参考；依赖 Casbin 细粒度权限或 seed 对象创建的 Market/MQ/部分 IAM 接口仍大量失败，应按授权/数据准备失败路径解读。

## 执行器硬件规格

| 项目 | 值 |
| --- | --- |
| 执行日期 | TBD |
| 执行机器 | `fedora` |
| CPU | 20 logical CPUs |
| 内存 | 未采集 |
| 操作系统 | `Linux-7.1.0-0.rc7.260611g9716c086c8e8.50.fc45.x86_64-x86_64-with-glibc2.43.9000` |
| Docker 版本 | `Docker version 29.5.3, build d1c06ef` |
| Docker Compose 版本 | `5.1.4` |
| k6 版本 | 容器镜像 `grafana/k6:latest`，具体版本未采集 |

## 被测环境

| 项目 | 值 |
| --- | --- |
| API Base URL | `http://localhost:12660` |
| PostgreSQL 容器 | `RPA-pgdb` |
| Redis 容器 | `RPA-redis` |
| Go 应用容器 | `RPA-app`，compose 限额 `2 CPU / 2GiB` |
| Go 应用镜像/Commit | `rpa-app:v1.0`，commit 未采集 |
| 初始化脚本 | `script/init_better.sql` |

## 功能开关矩阵

| Case | JWT | Casbin | RateLimit Backend | RequestPool | 备注 |
| --- | --- | --- | --- | --- | --- |
| baseline | on | on | redis | on | 标准生产近似配置 |
| auth-off | off | off | memory | on | 认证开销对照 |
| concurrency-50vu-20260621-155757 | off | off | memory，实际 enabled | on | 本轮脚本参数期望关闭限流，但容器实际 `FEATURES_RATE_LIMIT_ENABLED=true`，结果保留用于分析脚本缺陷与限流路径 |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | on | on | disabled | on | 开启 MQ；manifest 记录容器实际 `FEATURES_RATE_LIMIT_ENABLED=false` |

## 压测目标接口

| 接口 | 方法 | URL | 认证 | 说明 |
| --- | --- | --- | --- | --- |
| 应用列表 | GET | `/api/v1/market/apps?page=1&page_size=20` | 否 | 首页列表 |
| 应用详情 | GET | `/api/v1/market/apps/{app_id}` | 否 | Cache-Aside 命中/未命中分别记录 |
| 应用下载 | GET | `/api/v1/market/apps/{app_id}/download` | 否 | 307 预签名跳转 |
| 登录 | POST | `/api/v1/iam/login` | 否 | JWT + Redis Session |
| 订单支付 | POST | `/api/v1/orders/{order_id}/pay` | 是 | 钱包扣款 + Outbox 发放 |

## 执行参数

| Case | VUs | 持续时长 | Ramp Up | 数据集 | 命令 |
| --- | ---: | --- | --- | --- | --- |
| baseline | TBD | TBD | TBD | TBD | `script/run-load-tests.sh` |
| concurrency-50vu-20260621-155757 | 50 | 20s / 接口 | 无 | k6 动态注册用户、创建测试 group/app；历史测试库，未重建 | `EXECUTOR=constant-vus DEFAULT_DURATION=20s DEFAULT_VUS=50 RUN_HTTP_APIS=true RUN_MQ=false ./run-load-tests.sh` |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | 50 | 60s / 接口 | 无 | k6 动态注册用户、创建测试 group/app；历史测试库，未重建 | `EXECUTOR=constant-vus DEFAULT_DURATION=60s DEFAULT_VUS=50 RUN_HTTP_APIS=true RUN_MQ=true FEATURES_JWT_AUTH=true FEATURES_CASBIN_AUTHZ=true FEATURES_RATE_LIMIT_ENABLED=false FEATURES_REQUEST_POOL_ENABLED=true ./run-load-tests.sh` |

## 吞吐与延迟结果

| Case | 接口 | QPS 均值 | P50 | P95 | P99 | 错误率 |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| baseline | TBD | TBD | TBD | TBD | TBD | TBD |
| concurrency-50vu-20260621-155757 | `iam_group_create` | 4218.77 | 10.593ms | 21.116ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `iam_group_detail` | 5801.72 | 6.220ms | 25.638ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `iam_group_update` | 488.55 | 68.234ms | 309.206ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `iam_groups_list` | 1496.22 | 27.192ms | 91.757ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `iam_login` | 54853.08 | 0.649ms | 2.151ms | 未采集 | 99.99% |
| concurrency-50vu-20260621-155757 | `iam_permissions_list` | 5428.72 | 5.536ms | 33.914ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `iam_profile_avatar` | 287.59 | 117.283ms | 526.552ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `iam_profile_get` | 9390.38 | 3.709ms | 19.513ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `iam_profile_update` | 448.84 | 75.937ms | 335.341ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `iam_register` | 54746.20 | 0.640ms | 2.159ms | 未采集 | 99.99% |
| concurrency-50vu-20260621-155757 | `iam_role_get` | 6583.57 | 6.030ms | 19.519ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `iam_roles_list` | 6516.41 | 6.604ms | 17.303ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `market_app_detail` | 44787.97 | 0.691ms | 2.567ms | 未采集 | 99.99% |
| concurrency-50vu-20260621-155757 | `market_app_download` | 44534.16 | 0.696ms | 2.594ms | 未采集 | 99.99% |
| concurrency-50vu-20260621-155757 | `market_app_publish` | 683.59 | 71.663ms | 124.732ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `market_app_update` | 412.49 | 81.942ms | 360.010ms | 未采集 | 0.00% |
| concurrency-50vu-20260621-155757 | `market_apps_list` | 45606.71 | 0.672ms | 2.666ms | 未采集 | 99.99% |
| concurrency-50vu-20260621-155757 | `market_apps_search` | 45808.39 | 0.653ms | 2.681ms | 未采集 | 99.99% |
| concurrency-50vu-20260621-155757 | `market_rank_daily` | 44952.02 | 0.680ms | 2.666ms | 未采集 | 99.99% |
| concurrency-50vu-20260621-155757 | `market_rank_total` | 45722.37 | 0.663ms | 2.629ms | 未采集 | 99.99% |
| concurrency-50vu-20260621-155757 | `market_rank_weekly` | 44536.06 | 0.683ms | 2.725ms | 未采集 | 99.99% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_group_create` | 2524.09 | 16.786ms | 38.839ms | 未采集 | 0.11% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_group_detail` | 13848.35 | 3.087ms | 7.403ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_group_update` | 13305.47 | 3.204ms | 7.585ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_groups_list` | 4837.31 | 6.419ms | 32.385ms | 未采集 | 0.62% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_login` | 100.85 | 434.191ms | 989.991ms | 未采集 | 0.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_permissions_list` | 14036.35 | 3.039ms | 7.360ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_profile_avatar` | 269.69 | 126.650ms | 556.703ms | 未采集 | 0.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_profile_get` | 6214.16 | 5.121ms | 24.775ms | 未采集 | 0.27% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_profile_update` | 445.22 | 75.149ms | 345.585ms | 未采集 | 0.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_register` | 96.62 | 442.821ms | 901.529ms | 未采集 | 0.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_role_get` | 13625.94 | 3.131ms | 7.573ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `iam_roles_list` | 14016.83 | 3.041ms | 7.376ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `market_app_detail` | 39513.69 | 0.796ms | 3.160ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `market_app_download` | 39085.14 | 0.801ms | 3.181ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `market_app_publish` | 12835.42 | 3.303ms | 7.719ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `market_app_update` | 13431.56 | 3.146ms | 7.589ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `market_apps_list` | 2283.78 | 17.945ms | 61.599ms | 未采集 | 0.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `market_apps_search` | 476.91 | 91.360ms | 208.292ms | 未采集 | 0.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `market_rank_daily` | 4524.57 | 8.188ms | 32.483ms | 未采集 | 0.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `market_rank_total` | 4704.18 | 7.985ms | 31.525ms | 未采集 | 0.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `market_rank_weekly` | 4330.87 | 8.724ms | 34.241ms | 未采集 | 0.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `mq_group_invalidate_broadcast` | 13440.48 | 3.195ms | 7.386ms | 未采集 | 100.00% |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | `mq_role_purge_broadcast` | 14286.63 | 2.949ms | 7.258ms | 未采集 | 100.00% |

## 资源消耗峰值

| Case | 指标 | 峰值 | 数据来源 |
| --- | --- | ---: | --- |
| baseline | PostgreSQL open connections | TBD | `/metrics` `rpa_gorm_db_open_connections` |
| baseline | PostgreSQL in-use connections | TBD | `/metrics` `rpa_gorm_db_in_use_connections` |
| baseline | PostgreSQL wait count | TBD | `/metrics` `rpa_gorm_db_wait_count` |
| baseline | Redis connected clients | TBD | Redis INFO |
| baseline | Redis used memory | TBD | Redis INFO |
| baseline | RequestPool rejected total | TBD | `/metrics` `rpa_request_pool_rejected_total` |
| concurrency-50vu-20260621-155757 | App CPU peak | 196.30% | `docker stats` CSV，接口 `iam_groups_list` |
| concurrency-50vu-20260621-155757 | App memory peak | 402.1MiB | `docker stats` CSV，接口 `iam_register` |
| concurrency-50vu-20260621-155757 | PostgreSQL CPU peak | 757.75% | `docker stats` CSV，接口 `iam_group_create` |
| concurrency-50vu-20260621-155757 | PostgreSQL memory peak | 245.9MiB | `docker stats` CSV，接口 `market_app_update` |
| concurrency-50vu-20260621-155757 | Redis CPU peak | 6.09% | `docker stats` CSV，接口 `market_app_publish` |
| concurrency-50vu-20260621-155757 | Redis memory peak | 25.8MiB | `docker stats` CSV，接口 `iam_login` |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | App CPU peak | 200.79% | `docker stats` CSV，接口 `iam_groups_list` |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | App memory peak | 1.734GiB | `docker stats` CSV，接口 `iam_register` |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | PostgreSQL CPU peak | 1574.01% | `docker stats` CSV，接口 `market_apps_search` |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | PostgreSQL memory peak | 264.7MiB | `docker stats` CSV，接口 `market_apps_search` |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | Redis CPU peak | 28.03% | `docker stats` CSV，接口 `mq_role_purge_broadcast` |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | Redis memory peak | 12.53MiB | `docker stats` CSV，接口 `mq_role_purge_broadcast` |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | RocketMQ broker CPU peak | 11.69% | `docker stats` CSV，接口 `market_apps_search` |
| jwt-casbin-mq-rl-off-50vu-20260621-165020 | RocketMQ broker memory peak | 1.204GiB | `docker stats` CSV，接口 `iam_group_detail` |

## 原始产物

| 类型 | 路径 |
| --- | --- |
| k6 summary JSON | `script/k6-scripts/results/concurrency-50vu-20260621-155757/*.summary.json` |
| docker stats CSV | `script/k6-scripts/results/concurrency-50vu-20260621-155757/*.docker-stats.csv` |
| k6 summary JSON | `script/k6-scripts/results/jwt-casbin-mq-rl-off-50vu-20260621-165020/*.summary.json` |
| docker stats CSV | `script/k6-scripts/results/jwt-casbin-mq-rl-off-50vu-20260621-165020/*.docker-stats.csv` |
| Grafana 截图 | TBD |
| 应用日志 | TBD |

## 备注

正式报告不得删除失败 Case。若发生限流、鉴权失败、Redis 连接池耗尽或 PostgreSQL 等待连接，应保留错误率和资源峰值，并补充根因分析。

本轮压测后已修复 `script/run-load-tests.sh` 的环境变量读取：脚本现在接受 `FEATURE_*` 与 `FEATURES_*` 两套变量名，重启 App 后会把容器内实际 `FEATURES_*` 写入 manifest。后续若要测关闭限流的真实业务并发，应重新执行并确认 manifest 中 `[app_container_env]` 包含 `FEATURES_RATE_LIMIT_ENABLED=false`。

`jwt-casbin-mq-rl-off-50vu-20260621-165020` 执行前还修复了 `script/k6-scripts/lib/common.js` 的认证态传递：JWT 模式下登录后会向后续请求同时传递 Bearer token 与显式 Cookie header。该修复解决了 k6 setup 登录成功但受保护接口被判定为 `Account logged in from another device` 的问题。本轮 summary 仍未包含 P99；后续脚本已补充 `K6_SUMMARY_TREND_STATS` 透传，下一轮正式压测可采集 `p(99)`。
