# RPA Market API 文档

本文档面向前端开发，覆盖当前后端已注册的 IAM、Market、Wallet 与 Order API。

## 通用约定

- 服务地址：按环境配置，例如 `http://localhost:12660`
- IAM 基础路径：`/api/v1/iam`
- Market 基础路径：`/api/v1/market`
- Wallet 基础路径：`/api/v1/wallets`
- Order 基础路径：`/api/v1/orders`
- JSON 请求头：`Content-Type: application/json`
- 文件上传请求头：`multipart/form-data`
- 建议所有请求携带 `X-Trace-ID`，未携带时后端会自动生成并回写响应头。错误响应中的 `request_id` 与该值一致。
- 登录成功后后端会返回 `token`，并写入 HttpOnly Cookie：`auth_token`、`session_id`
- 私有接口需要登录态。浏览器端建议使用 Cookie；非浏览器客户端当前仍需要同时携带 `session_id` Cookie
- 常见错误响应：`{"code":"错误码","message":"错误信息","request_id":"trace-id"}`
- 当前 HTTP 服务已接入请求池快速失败：服务过载时会直接返回 `503`，不会无限堆积请求。

统一错误响应示例：

```json
{
  "code": "APP_NOT_FOUND",
  "message": "App not found",
  "request_id": "trace-id"
}
```

## 接口总览

公开接口：

- `POST /api/v1/iam/register`：注册
- `POST /api/v1/iam/login`：登录
- `GET /api/v1/market/apps`：应用列表
- `GET /api/v1/market/apps/:app_id`：应用详情
- `GET /api/v1/market/apps/:app_id/download`：下载应用，返回 `307`
- `GET /api/v1/market/rankings`：下载热榜

登录接口：

- `GET /api/v1/iam/profile`：当前用户资料
- `PUT /api/v1/iam/profile`：更新资料
- `POST /api/v1/iam/profile/avatar`：上传头像
- `PUT /api/v1/iam/profile/password`：修改密码
- `GET /api/v1/wallets/me`：当前用户钱包
- `GET /api/v1/wallets/:wallet_id/transactions`：钱包流水
- `POST /api/v1/wallets/:wallet_id/credit`：充值钱包
- `POST /api/v1/wallets/:wallet_id/debit`：扣款钱包
- `POST /api/v1/wallets/transfer`：钱包转账
- `POST /api/v1/orders/purchase`：一步购买应用
- `POST /api/v1/orders`：创建订单
- `GET /api/v1/orders`：订单列表
- `GET /api/v1/orders/:order_id`：订单详情
- `POST /api/v1/orders/:order_id/pay`：支付订单
- `POST /api/v1/orders/:order_id/cancel`：取消订单

管理/开发者接口：

- `POST /api/v1/market/apps`：发布应用
- `PUT /api/v1/market/apps/:app_id`：更新应用
- `PUT /api/v1/market/apps/:app_id/offshelf`：下架应用
- `DELETE /api/v1/market/apps/:app_id`：删除应用
- `GET /api/v1/iam/roles`、`GET /api/v1/iam/permissions`、`PUT /api/v1/iam/roles/:role_id/permissions`：RBAC 管理

## 认证与会话

### 注册

`POST /api/v1/iam/register`

请求体：

```json
{
  "username": "demo_user",
  "email": "demo@example.com",
  "password": "123456"
}
```

成功响应：`201`

```json
{
  "message": "User registered successfully",
  "user_id": "uuid"
}
```

### 登录

`POST /api/v1/iam/login`

请求体：

```json
{
  "email": "demo@example.com",
  "password": "123456"
}
```

成功响应：`200`

```json
{
  "message": "Login successful",
  "token": "jwt-token",
  "user_id": "uuid"
}
```

说明：登录成功后会写 Cookie：`auth_token` 有效期约 30 分钟，`session_id` 有效期约 7 天。

## 用户资料

以下接口均需要登录。

### 获取当前用户资料

`GET /api/v1/iam/profile`

成功响应：

```json
{
  "user_id": "uuid",
  "username": "demo_user",
  "email": "demo@example.com",
  "avatar_url": "minio-presigned-url",
  "created_at": "2026-06-19T00:00:00Z"
}
```

### 更新当前用户资料

`PUT /api/v1/iam/profile`

请求体：

```json
{
  "username": "new-name",
  "email": "new@example.com"
}
```

成功响应：

```json
{
  "message": "Profile updated successfully",
  "user": {
    "username": "new-name",
    "email": "new@example.com"
  }
}
```

### 上传头像

`POST /api/v1/iam/profile/avatar`

请求类型：`multipart/form-data`

字段：

- `avatar`：文件，必填，最大 5MB

上传安全约束：仅支持 `.jpg`、`.jpeg`、`.png`、`.webp`，后端会校验文件名、扩展名和文件头内容类型。

成功响应：

```json
{
  "message": "Avatar uploaded successfully",
  "avatar_url": "minio-presigned-url"
}
```

### 修改密码

`PUT /api/v1/iam/profile/password`

请求体：

```json
{
  "old_password": "old-password",
  "new_password": "new-password"
}
```

成功响应：

```json
{
  "message": "Password updated successfully. Please log in again."
}
```

说明：修改密码后会清理当前 Redis Session 和 Cookie，需要重新登录。

## 群组

以下接口均需要登录。部分接口还需要 Casbin 权限。

### 创建群组

`POST /api/v1/iam/groups`

请求体：

```json
{
  "name": "团队名称"
}
```

成功响应：`201`

```json
{
  "GroupID": "uuid",
  "GroupName": "团队名称",
  "OwnerID": "uuid",
  "Type": "standard",
  "CreatedAt": "2026-06-19T00:00:00Z"
}
```

### 获取我的群组

`GET /api/v1/iam/groups`

成功响应：

```json
[
  {
    "GroupID": "uuid",
    "GroupName": "团队名称",
    "OwnerID": "uuid",
    "Type": "standard",
    "CreatedAt": "2026-06-19T00:00:00Z"
  }
]
```

### 获取群组详情

`GET /api/v1/iam/groups/:id`

权限：`group:view`

成功响应：

```json
{
  "Group": {
    "GroupID": "uuid",
    "GroupName": "团队名称",
    "OwnerID": "uuid",
    "Type": "standard",
    "CreatedAt": "2026-06-19T00:00:00Z"
  },
  "Members": [
    {
      "GroupID": "uuid",
      "UserID": "uuid",
      "RoleID": 2,
      "Role": {
        "ID": 2,
        "Name": "owner",
        "Description": "团体/项目所有者，拥有当前域内的所有权限"
      }
    }
  ]
}
```

### 更新群组名称

`PUT /api/v1/iam/groups/:id`

权限：`group:edit`

请求体：

```json
{
  "name": "新团队名称"
}
```

成功响应：

```json
{
  "message": "更新成功"
}
```

### 解散群组

`DELETE /api/v1/iam/groups/:id`

权限：`group:delete`

成功响应：

```json
{
  "message": "团体已解散"
}
```

### 邀请成员

`POST /api/v1/iam/groups/:id/members`

权限：`group:invite`

请求体：

```json
{
  "user_id": "uuid",
  "role_id": 4
}
```

成功响应：

```json
{
  "message": "邀请成功"
}
```

### 移除成员

`DELETE /api/v1/iam/groups/:id/members/:user_id`

权限：`group:kick`

成功响应：

```json
{
  "message": "移除成功"
}
```

### 修改成员角色

`PUT /api/v1/iam/groups/:id/members/:user_id/role`

权限：`group:edit`

请求体：

```json
{
  "role_id": 3
}
```

成功响应：

```json
{
  "message": "成员角色已更新"
}
```

说明：不能修改群 owner 的角色。修改成功后会触发 Casbin domain 缓存热更新。

### 退出群组

`POST /api/v1/iam/groups/:id/leave`

成功响应：

```json
{
  "message": "已退出团体"
}
```

说明：owner 不能直接退出，需要先转让或解散。

## 角色与权限管理

以下接口均需要登录，并要求当前用户在系统全局域拥有 `role:manage` 权限。

系统全局域 ID：`11111111-1111-1111-1111-111111111111`

### 获取角色列表

`GET /api/v1/iam/roles`

成功响应：

```json
{
  "data": [
    {
      "ID": 1,
      "Name": "superadmin",
      "Description": "系统超级管理员，拥有全局最高权限",
      "Permissions": [
        {
          "ID": 1,
          "Code": "app:create",
          "Description": "发布应用到市场",
          "CreatedAt": "2026-06-19T00:00:00Z"
        }
      ]
    }
  ]
}
```

### 获取单个角色

`GET /api/v1/iam/roles/:role_id`

成功响应：

```json
{
  "data": {
    "ID": 2,
    "Name": "owner",
    "Description": "团体/项目所有者，拥有当前域内的所有权限",
    "Permissions": []
  }
}
```

### 获取权限列表

`GET /api/v1/iam/permissions`

成功响应：

```json
{
  "data": [
    {
      "ID": 1,
      "Code": "app:create",
      "Description": "发布应用到市场",
      "CreatedAt": "2026-06-19T00:00:00Z"
    }
  ]
}
```

### 全量替换角色权限

`PUT /api/v1/iam/roles/:role_id/permissions`

请求体：

```json
{
  "permission_ids": [1, 2, 3, 15]
}
```

成功响应：

```json
{
  "message": "角色权限已更新",
  "data": {
    "ID": 3,
    "Name": "developer",
    "Description": "开发者，可以发布应用和创建项目编排",
    "Permissions": []
  }
}
```

说明：

- 这是全量替换，不是增量追加。
- 如果传空数组，表示清空该角色权限。
- `superadmin` 不能移除 `role:manage`，避免锁死权限管理入口。
- 更新成功后会本地 `PurgeAll` 并广播 `purge_all`，让所有实例重新加载 Casbin 策略。

## Market 应用市场

### 应用列表

`GET /api/v1/market/apps?page=1&page_size=10&keyword=&category=&status=published`

公开接口，已挂限流。

查询参数：

- `page`：页码，默认 1
- `page_size`：每页数量，默认 10，最大 100
- `keyword`：按名称模糊搜索，可选
- `category`：分类过滤，可选
- `status`：状态过滤，可选，默认 `published`

成功响应：

```json
{
  "data": [
    {
      "app_id": "uuid",
      "name": "应用名",
      "developer_id": "uuid",
      "category": "工具",
      "tags": ["rpa", "tool"],
      "metadata": {},
      "status": "published",
      "create_at": "2026-06-19T00:00:00Z",
      "update_at": "2026-06-19T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 10
}
```

### 应用详情

`GET /api/v1/market/apps/:app_id`

公开接口，已挂限流。

成功响应：

```json
{
  "data": {
    "app_id": "uuid",
    "name": "应用名",
    "developer_id": "uuid",
    "category": "工具",
    "tags": [],
    "metadata": {},
    "status": "published",
    "create_at": "2026-06-19T00:00:00Z",
    "update_at": "2026-06-19T00:00:00Z"
  }
}
```

### 下载应用

`GET /api/v1/market/apps/:app_id/download`

公开接口，已挂限流。

响应：`307 Temporary Redirect`

说明：

- 后端会生成 5 分钟有效的 MinIO 预签名 URL 并重定向。
- 仅 `published` 状态应用可下载。
- 下载成功生成链接后会写入 Redis 热榜计数，并同步累加 `app_download_metrics` 持久化指标。
- 如果应用 metadata 中存在完整性信息，响应头会包含 `X-App-SHA256` 和 `X-App-ETag`。

### 发布应用

`POST /api/v1/market/apps`

权限：登录 + `app:create`

请求类型：`multipart/form-data`

可选请求头：

- `Idempotency-Key`：发布幂等键。推荐客户端每次“业务发布动作”生成一个稳定唯一值；网络重试时复用同一个值。

字段：

- `name`：应用名，必填
- `category`：分类，可选
- `tags`：标签，可传多个同名字段，例如 `tags=rpa&tags=tool`
- `status`：状态，可选，默认 `published`
- `app_file`：应用文件，必填，最大 100MB，仅支持 `.zip`、`.gz`、`.tgz`
- `idempotency_key`：发布幂等键，可选；如果请求头 `Idempotency-Key` 已传，则优先使用请求头。

上传安全约束：

- 原始文件名允许中文，但不能包含 `/`、`\`、NUL 字符或路径片段。
- MinIO 对象名由后端生成，不直接使用原始文件名，格式为 `apps/{developer_id}/{app_id}/app{ext}`。
- 后端会同时校验文件扩展名和文件头内容类型，不信任前端传入的 `Content-Type`。
- 后端会计算上传文件 `sha256`，并保存 MinIO 返回的 `etag` 到 `metadata`。
- DB 写入失败或超时时，后端会尝试补偿删除已上传的 MinIO 对象，并记录审计日志。
- 使用相同幂等键重复提交时，若已有发布记录，会返回原 `app_id`，响应中带 `idempotent: true`。
- 同一开发者同一幂等键并发上传时，正在处理中的重复请求会返回 `409`。

成功响应：

```json
{
  "message": "App published successfully",
  "app_id": "uuid"
}
```

幂等命中响应：`200`

```json
{
  "message": "App published successfully",
  "app_id": "uuid",
  "idempotent": true
}
```

并发冲突响应：`409`

```json
{
  "code": "DUPLICATE_UPLOAD_IN_PROGRESS",
  "message": "Duplicate upload is in progress",
  "request_id": "trace-id"
}
```

### 更新应用元数据

`PUT /api/v1/market/apps/:app_id`

权限：登录 + `app:edit` + 应用所有者校验。

请求体：

```json
{
  "name": "新应用名",
  "category": "工具",
  "tags": ["rpa", "tool"],
  "metadata": {
    "version": "1.0.1"
  },
  "status": "published"
}
```

成功响应：

```json
{
  "message": "App updated successfully",
  "data": {}
}
```

说明：后端按字段更新，未传字段不会覆盖旧值。非应用开发者本人会返回 `403`。

### 下架应用

`PUT /api/v1/market/apps/:app_id/offshelf`

权限：登录 + `app:offshelf` + 应用所有者校验。

成功响应：

```json
{
  "message": "App is now off the shelf"
}
```

### 删除应用

`DELETE /api/v1/market/apps/:app_id`

权限：登录 + `app:delete` + 应用所有者校验。

成功响应：

```json
{
  "message": "App and files deleted successfully"
}
```

说明：当前实现先删除 DB 记录，再尝试删除 MinIO 文件；非应用开发者本人会返回 `403`。

## Market 热榜

### 获取下载热榜

`GET /api/v1/market/rankings?type=daily&limit=20`

公开接口，已挂限流。

查询参数：

- `type`：`daily`、`weekly`、`total`，默认 `daily`
- `limit`：返回数量，默认 20，范围 1-100

成功响应：

```json
{
  "type": "daily",
  "limit": 20,
  "data": [
    {
      "app": {
        "app_id": "uuid",
        "name": "应用名",
        "developer_id": "uuid",
        "category": "工具",
        "tags": [],
        "metadata": {},
        "status": "published",
        "create_at": "2026-06-19T00:00:00Z",
        "update_at": "2026-06-19T00:00:00Z"
      },
      "score": 10,
      "rank": 1
    }
  ]
}
```

说明：热榜实时查询数据来自 Redis ZSET，下载接口会更新 daily、weekly、total 三个榜单，并按日持久化到 `app_download_metrics`。

## Wallet 钱包

以下接口均需要登录。金额字段使用字符串传输，精度为 4 位小数，例如 `10.0000`。

领域约束：

- 钱包所有权当前只支持用户钱包，接口会校验 `owner_type=user` 且 `owner_id=当前用户`。
- 写操作支持 `idempotency_key`，同一个键重复提交会返回同一笔业务结果或返回 `409 IDEMPOTENCY_CONFLICT`。
- 扣款、转账、订单支付都在数据库事务内锁钱包行，避免并发超卖。
- 余额不足返回 `409 INSUFFICIENT_BALANCE`，不会写入 `wallet_transactions`。

### 获取当前用户钱包

`GET /api/v1/wallets/me?currency_code=COIN`

成功响应：

```json
{
  "wallet_id": "uuid",
  "owner_id": "uuid",
  "owner_type": "user",
  "balance": "100.0000",
  "currency_code": "COIN",
  "status": "active",
  "updated_at": "2026-06-21T00:00:00Z"
}
```

### 查询钱包流水

`GET /api/v1/wallets/:wallet_id/transactions?page=1&page_size=20`

成功响应：

```json
{
  "items": [
    {
      "tx_id": "uuid",
      "wallet_id": "uuid",
      "tx_type": "consume",
      "amount": "-10.0000",
      "balance_after": "90.0000",
      "reference_id": "order-id",
      "idempotency_key": "order:order-id:pay:key",
      "description": "order payment",
      "created_at": "2026-06-21T00:00:00Z"
    }
  ],
  "page": 1,
  "page_size": 20
}
```

### 充值钱包

`POST /api/v1/wallets/:wallet_id/credit`

请求体：

```json
{
  "amount": "100.0000",
  "reference_id": "uuid，可选",
  "description": "manual topup",
  "idempotency_key": "topup-unique-key"
}
```

成功响应：

```json
{
  "wallet": {
    "wallet_id": "uuid",
    "owner_id": "uuid",
    "owner_type": "user",
    "balance": "100.0000",
    "currency_code": "COIN",
    "status": "active",
    "updated_at": "2026-06-21T00:00:00Z"
  },
  "transaction": {
    "tx_id": "uuid",
    "wallet_id": "uuid",
    "tx_type": "recharge",
    "amount": "100.0000",
    "balance_after": "100.0000",
    "reference_id": "",
    "idempotency_key": "topup-unique-key",
    "description": "manual topup",
    "created_at": "2026-06-21T00:00:00Z"
  }
}
```

### 扣款钱包

`POST /api/v1/wallets/:wallet_id/debit`

请求体同充值。余额不足返回 `409 INSUFFICIENT_BALANCE`，且不会写流水。

### 钱包转账

`POST /api/v1/wallets/transfer`

请求体：

```json
{
  "from_wallet_id": "uuid",
  "to_wallet_id": "uuid",
  "amount": "10.0000",
  "reference_id": "uuid，可选",
  "description": "transfer",
  "idempotency_key": "transfer-unique-key"
}
```

成功响应：

```json
{
  "out": {
    "tx_type": "transfer_out",
    "amount": "-10.0000"
  },
  "in": {
    "tx_type": "transfer_in",
    "amount": "10.0000"
  }
}
```

## Order 订单与购买

以下接口均需要登录。当前购买发放目标为用户订阅 `subscriptions`。

领域约束：

- 当前订单金额由前端传 `amount`，后续应迁移为后端从 `app.metadata.price` 或 SKU/Price 表派生。
- `idempotency_key` 用于防止重复创建订单或重复支付。
- 一步购买接口适合前端直接调用；拆分式创建/支付接口适合需要先确认订单再支付的流程。
- 支付成功会发放用户订阅，当前默认有效期为 1 年，`subscriptions.plan_type=purchase`。
- 订单状态：`pending`、`paid`、`cancelled`。

### 一步购买应用

`POST /api/v1/orders/purchase`

说明：该接口会一次完成创建订单、支付扣款、写钱包流水、发放订阅。支付成功后返回的订单中会带 `tx_id` 与 `subscription_id`。

请求体：

```json
{
  "app_id": "uuid",
  "amount": "10.0000",
  "currency_code": "COIN",
  "idempotency_key": "purchase-unique-key",
  "description": "buy app"
}
```

成功响应：`200`

```json
{
  "order_id": "uuid",
  "user_id": "uuid",
  "app_id": "uuid",
  "wallet_id": "uuid",
  "amount": "10.0000",
  "currency_code": "COIN",
  "status": "paid",
  "tx_id": "wallet-transaction-id",
  "subscription_id": "subscription-id",
  "idempotency_key": "purchase-unique-key",
  "description": "buy app",
  "created_at": "2026-06-21T00:00:00Z",
  "paid_at": "2026-06-21T00:00:00Z",
  "updated_at": "2026-06-21T00:00:00Z"
}
```

落库关联：

- `orders.status = paid`
- `orders.tx_id = wallet_transactions.tx_id`
- `orders.subscription_id = subscriptions.sub_id`
- `wallet_transactions.reference_id = orders.order_id`
- `subscriptions.source_order_id = orders.order_id`

### 创建订单

`POST /api/v1/orders`

请求体同“一步购买应用”。成功响应：`201`，订单状态为 `pending`，`tx_id` 和 `subscription_id` 为空。

成功响应示例：

```json
{
  "order_id": "uuid",
  "user_id": "uuid",
  "app_id": "uuid",
  "wallet_id": "uuid",
  "amount": "10.0000",
  "currency_code": "COIN",
  "status": "pending",
  "tx_id": "",
  "subscription_id": "",
  "idempotency_key": "create-order-key",
  "description": "buy app",
  "created_at": "2026-06-21T00:00:00Z",
  "paid_at": "",
  "updated_at": "2026-06-21T00:00:00Z"
}
```

### 查询订单列表

`GET /api/v1/orders?page=1&page_size=20`

成功响应：

```json
{
  "items": [],
  "page": 1,
  "page_size": 20
}
```

### 查询订单详情

`GET /api/v1/orders/:order_id`

成功响应：订单对象。

### 支付订单

`POST /api/v1/orders/:order_id/pay`

请求体：

```json
{
  "idempotency_key": "pay-unique-key"
}
```

说明：支付会在 DB transaction 内锁订单行，扣钱包并发放订阅。重复支付已支付订单会直接返回已支付订单。

### 取消订单

`POST /api/v1/orders/:order_id/cancel`

说明：仅 `pending` 订单可取消。

## Admin 管理查询

以下 HTTP 接口均需要登录，并要求具备 `role:manage` 权限。分页参数统一为 `page`、`page_size`，`page_size` 最大为 `100`。

### 查询虚拟账户

`GET /api/v1/admin/virtual-orders?page=1&page_size=20&owner_id=uuid&owner_type=user&currency_code=COIN&status=active`

说明：当前数据库没有独立“虚拟订单”表，此接口按现有 `wallets` 虚拟账户/钱包表查询。

成功响应：

```json
{
  "items": [
    {
      "wallet_id": "uuid",
      "owner_id": "uuid",
      "owner_type": "user",
      "balance": "100.0000",
      "currency_code": "COIN",
      "status": "active",
      "updated_at": "2026-06-21T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

### 查询钱包流水

`GET /api/v1/admin/wallet-transactions?page=1&page_size=20&wallet_id=uuid&reference_id=uuid&tx_type=consume&from=2026-06-21&to=2026-06-22`

支持按钱包、关联单据、流水类型和创建时间范围过滤。

成功响应：

```json
{
  "items": [
    {
      "tx_id": "uuid",
      "wallet_id": "uuid",
      "tx_type": "consume",
      "amount": "-10.0000",
      "balance_after": "90.0000",
      "reference_id": "order-id",
      "idempotency_key": "order:order-id:pay:key",
      "description": "order payment",
      "created_at": "2026-06-21T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

### 查询全局订单

`GET /api/v1/admin/orders?page=1&page_size=20&user_id=uuid&app_id=uuid&wallet_id=uuid&status=paid&currency_code=COIN&from=2026-06-21&to=2026-06-22`

支持按用户、应用、钱包、状态、币种和创建时间范围过滤。

成功响应：

```json
{
  "items": [
    {
      "order_id": "uuid",
      "user_id": "uuid",
      "app_id": "uuid",
      "wallet_id": "uuid",
      "amount": "10.0000",
      "currency_code": "COIN",
      "status": "paid",
      "tx_id": "wallet-transaction-id",
      "subscription_id": "subscription-id",
      "idempotency_key": "purchase-unique-key",
      "description": "buy app",
      "created_at": "2026-06-21T00:00:00Z",
      "paid_at": "2026-06-21T00:00:00Z",
      "updated_at": "2026-06-21T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

### 查询变更日志

`GET /api/v1/admin/change-logs?page=1&page_size=20&event_type=role_permissions_updated&actor_id=uuid&resource=1&trace_id=trace&from=2026-06-21&to=2026-06-22`

说明：变更日志来源于 `audit_events`，包括权限变更、应用发布、补偿失败等审计事件。

成功响应：

```json
{
  "items": [
    {
      "event_id": "uuid",
      "event_type": "role_permissions_updated",
      "trace_id": "trace-id",
      "actor_id": "user-id",
      "resource": "role-id",
      "metadata": "{}",
      "error": "",
      "created_at": "2026-06-21T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

## gRPC 接口

gRPC 默认与 HTTP 同进程启动，端口由 `grpc.port` 配置，默认 `12661`。proto 文件位于 `proto/*/v1`，生成代码位于 `gen/go`。

### wallet.v1.WalletService

- `GetWallet(GetWalletRequest) returns (Wallet)`：按 `owner_id + owner_type + currency_code` 查询钱包。
- `GetOrCreateWallet(GetOrCreateWalletRequest) returns (Wallet)`：按 `owner_id + owner_type + currency_code` 查询或创建钱包。

### admin.v1.AdminQueryService

- `ListVirtualOrders(ListVirtualOrdersRequest) returns (ListVirtualOrdersResponse)`：查询虚拟账户，字段与 HTTP `/api/v1/admin/virtual-orders` 对齐。
- `ListWalletTransactions(ListWalletTransactionsRequest) returns (ListWalletTransactionsResponse)`：查询钱包流水，字段与 HTTP `/api/v1/admin/wallet-transactions` 对齐。
- `ListOrders(ListOrdersRequest) returns (ListOrdersResponse)`：查询全局订单，字段与 HTTP `/api/v1/admin/orders` 对齐。
- `ListChangeLogs(ListChangeLogsRequest) returns (ListChangeLogsResponse)`：查询变更日志，字段与 HTTP `/api/v1/admin/change-logs` 对齐。

通用约束：

- `page` 小于 `1` 时按 `1` 处理。
- `page_size` 小于 `1` 时按 `20` 处理，大于 `100` 时按 `100` 处理。
- UUID 过滤字段格式非法时返回 `InvalidArgument`。
- 时间过滤字段 `from/to` 支持 `RFC3339Nano` 或 `YYYY-MM-DD`，格式非法时返回 `InvalidArgument`。

### 常见业务错误

- `APP_NOT_FOUND`：应用不存在或不是 `published` 状态。
- `INVALID_ORDER_AMOUNT`：订单金额小于等于 0 或格式非法。
- `INSUFFICIENT_BALANCE`：钱包余额不足，订单仍为 `pending`，不会发放订阅。
- `ORDER_NOT_PAYABLE`：订单不是 `pending` 状态，不能支付。
- `ORDER_NOT_CANCELLABLE`：订单不是 `pending` 状态，不能取消。
- `IDEMPOTENCY_CONFLICT`：幂等键已用于其他参数不同的请求。

## 状态码参考

- `200`：成功
- `201`：创建成功
- `307`：下载重定向
- `400`：请求参数错误
- `401`：未登录、Token 无效、Session 失效
- `403`：权限不足
- `404`：资源不存在
- `409`：注册用户名或邮箱冲突、上传幂等并发冲突、余额不足、订单状态冲突、幂等键冲突
- `429`：限流
- `500`：服务内部错误
- `503`：服务过载快速失败或依赖不可用，例如请求池已满、Redis Session 写入失败

## 运行与治理说明

- HTTP 服务支持优雅退出：收到 `SIGINT` / `SIGTERM` 后停止接收新请求，并在配置的超时时间内等待正在处理的请求结束。
- 退出时会关闭请求池、RocketMQ producer/consumer、Redis 连接和 PostgreSQL 连接。
- 请求池配置位于 `features.request_pool`，默认启用，容量为 `1000`。
- 优雅退出超时配置位于 `server.shutdown_timeout_seconds`，默认 `15` 秒。
- 当前 Market 发布、补偿、删除等关键事件已接入异步审计批量落库，表为 `audit_events`。
- MinIO 删除失败会写入 `minio_delete_retries` 补偿队列表，后台 worker 会按 `next_run_at` 重试，最多 5 次。
- Market 压测脚本位于 `script/k6-scripts/market.js`，支持公开列表/热榜、发布、下载场景；发布场景需通过 `APP_FILE` 指定本地 zip/gz/tgz 测试文件。

## 权限码参考

- `app:create`：发布应用
- `app:edit`：编辑应用
- `app:offshelf`：下架应用
- `app:delete`：删除应用
- `app:subscribe`：订阅/购买应用
- `project:create`：创建项目
- `project:edit`：编辑项目
- `project:delete`：删除项目
- `group:view`：查看群组
- `group:edit`：编辑群组或成员角色
- `group:delete`：解散群组
- `group:invite`：邀请成员
- `group:kick`：移除成员
- `role:manage`：管理角色与权限
