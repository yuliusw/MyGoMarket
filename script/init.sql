/*
    用户表：用户ID, 用户名, 邮箱, 密码哈希值, 头像链接, 创建时间, 修改时间
*/
CREATE TABLE IF NOT EXISTS users (
    user_id UUID PRIMARY KEY DEFAULT uuidv7(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    avatar_url TEXT,
    create_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT TRUE
);

/*
    角色表：角色ID, 角色名称(管理员/开发者/普通用户), 描述
*/
CREATE TABLE IF NOT EXISTS roles (
    role_id SERIAL PRIMARY KEY,
    role_name VARCHAR(50) UNIQUE NOT NULL,
    description TEXT
);

/*
    权限定义表：权限ID, 权限标识符(如'app:publish', 'project:create'), 功能描述, 创建时间
*/
CREATE TABLE IF NOT EXISTS permissions (
    permission_id SERIAL PRIMARY KEY,
    permission_code VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

/*
    角色-权限映射表：角色ID, 权限ID
*/
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id INTEGER REFERENCES roles(role_id) ON DELETE CASCADE,
    permission_id INTEGER REFERENCES permissions(permission_id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

/*
    团体表：团体ID, 团体名称, 所有者ID, 团体类型, 创建时间
*/
CREATE TABLE IF NOT EXISTS groups (
    group_id UUID PRIMARY KEY DEFAULT uuidv7(),
    group_name VARCHAR(100) NOT NULL,
    owner_id UUID NOT NULL REFERENCES users(user_id),
    group_type VARCHAR(20) DEFAULT 'standard',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

/*
    成员-团体-角色映射表：团体ID, 用户ID, 角色ID
*/
CREATE TABLE IF NOT EXISTS group_members (
    group_id UUID REFERENCES groups(group_id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(user_id) ON DELETE CASCADE,
    role_id INTEGER REFERENCES roles(role_id),
    PRIMARY KEY (group_id, user_id)
);

/*
    应用市场表：应用ID, 应用名, 开发者ID, 分类, 标签, 元数据, 状态, 创建时间, 修改时间
    （市场里供用户挑选的基础组件/模块）
*/
CREATE TABLE IF NOT EXISTS apps (
    app_id UUID PRIMARY KEY DEFAULT uuidv7(),
    name VARCHAR(255) NOT NULL,
    developer_id UUID NOT NULL REFERENCES users(user_id),
    category VARCHAR(50),
    tags TEXT[],
    metadata JSONB DEFAULT '{}', -- 应用接口定义、输入输出Schema等
    status VARCHAR(20) DEFAULT 'published', 
    create_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

/*
    用户项目表：项目ID, 项目名称, 所有者ID, 描述, 创建时间, 修改时间
    （用户的工作空间，用于承载编排流程和个人资产）
*/
CREATE TABLE IF NOT EXISTS projects (
    project_id UUID PRIMARY KEY DEFAULT uuidv7(),
    name VARCHAR(255) NOT NULL,
    owner_id UUID NOT NULL REFERENCES users(user_id),
    description TEXT,
    create_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

/*
    项目编排流程表：流程ID, 项目ID, 流程名称, 编排定义(JSONB), 修改时间
    （JSONB中记录DAG图：节点引用 apps 表的 app_id，输入输出关联 assets 表的 asset_id）
*/
CREATE TABLE IF NOT EXISTS project_workflows (
    workflow_id UUID PRIMARY KEY DEFAULT uuidv7(),
    project_id UUID REFERENCES projects(project_id) ON DELETE CASCADE,
    workflow_name VARCHAR(100) NOT NULL,
    definition JSONB NOT NULL, -- 示例: {"nodes": [{"app_id": "xxx", "inputs": ["asset_id_1"]}], "edges": [...]}
    is_active BOOLEAN DEFAULT TRUE,
    update_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

/*
    项目资产表(用户文件)：资产ID, 项目ID, 资产名称, 资产类型, 存储URL, 校验和, 创建时间
    （用户上传的自有文件，隶属于项目，在编排流程中被当作输入）
*/
CREATE TABLE IF NOT EXISTS project_assets (
    asset_id UUID PRIMARY KEY DEFAULT uuidv7(),
    project_id UUID REFERENCES projects(project_id) ON DELETE CASCADE,
    asset_name VARCHAR(255) NOT NULL,
    asset_type VARCHAR(50), -- 如 'csv', 'image', 'model_weights'
    storage_url TEXT NOT NULL,
    checksum VARCHAR(64),
    file_size BIGINT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

/*
    订阅授权表：订阅ID, 应用ID, 用户ID, 计划类型, 授权截止时间, 状态
    （用户在市场订阅/购买应用的记录）
*/
CREATE TABLE IF NOT EXISTS subscriptions (
    sub_id UUID DEFAULT uuidv7(),
    app_id UUID REFERENCES apps(app_id),
    user_id UUID REFERENCES users(user_id),
    plan_type VARCHAR(20),
    expired_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(10) DEFAULT 'active',
    PRIMARY KEY (sub_id, expired_at)
) PARTITION BY RANGE (expired_at);

/* =========================================================
   虚拟钱包表：钱包ID, 所有者ID(用户或团队), 余额, 币种
   (支持用户个人钱包，也支持团队公共钱包)
   ========================================================= */
CREATE TABLE IF NOT EXISTS wallets (
    wallet_id UUID PRIMARY KEY DEFAULT uuidv7(),
    owner_id UUID NOT NULL, -- 关联 users(user_id) 或 groups(group_id)
    owner_type VARCHAR(20) NOT NULL CHECK (owner_type IN ('user', 'group')),
    balance DECIMAL(18, 4) DEFAULT 0.0000, -- 使用 Decimal 保证精度
    currency_code VARCHAR(10) DEFAULT 'COIN',
    status VARCHAR(20) DEFAULT 'active',
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (owner_id, currency_code)
);

/* =========================================================
   虚拟货币流水表：流水ID, 钱包ID, 交易类型, 金额, 关联业务ID, 描述
   (满足情形 3：虚拟货币流水)
   ========================================================= */
CREATE TABLE IF NOT EXISTS wallet_transactions (
    tx_id UUID PRIMARY KEY DEFAULT uuidv7(),
    wallet_id UUID REFERENCES wallets(wallet_id),
    tx_type VARCHAR(50) NOT NULL, -- 如 'recharge'(充值), 'consume'(消费), 'refund'(退款)
    amount DECIMAL(18, 4) NOT NULL, -- 正数表示收入，负数表示支出
    balance_after DECIMAL(18, 4) NOT NULL, -- 记录交易后的快照余额，方便对账
    reference_id UUID, -- 关联业务单据号 (如 订阅ID, 订单ID)
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

/* =========================================================
   优惠券定义表：记录优惠券的模板规则
   ========================================================= */
CREATE TABLE IF NOT EXISTS coupon_templates (
    template_id UUID PRIMARY KEY DEFAULT uuidv7(),
    name VARCHAR(100) NOT NULL,
    discount_type VARCHAR(20) NOT NULL CHECK (discount_type IN ('fixed', 'percent')),
    discount_value DECIMAL(18, 4) NOT NULL,
    min_spend DECIMAL(18, 4) DEFAULT 0,
    valid_days INTEGER, -- 领取后有效天数
    target_app_id UUID, -- 如果仅限特定应用可用，则关联 apps(app_id)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

/* =========================================================
   用户/团队优惠券表：具体下发到账户的优惠券凭证
   (满足情形 1：下发用于此的优惠券的能力)
   ========================================================= */
CREATE TABLE IF NOT EXISTS coupons (
    coupon_id UUID PRIMARY KEY DEFAULT uuidv7(),
    template_id UUID REFERENCES coupon_templates(template_id),
    owner_id UUID NOT NULL,
    owner_type VARCHAR(20) NOT NULL CHECK (owner_type IN ('user', 'group')),
    status VARCHAR(20) DEFAULT 'unused' CHECK (status IN ('unused', 'used', 'expired')),
    expired_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE,
    used_tx_id UUID, -- 关联 wallet_transactions(tx_id)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

/* =========================================================
   团队权益实例表 (Team Entitlements)
   (满足情形 2：购买团队权益)
   ========================================================= */
CREATE TABLE IF NOT EXISTS group_entitlements (
    entitlement_id UUID PRIMARY KEY DEFAULT uuidv7(),
    group_id UUID NOT NULL, -- 关联核心库 groups(group_id)
    benefit_code VARCHAR(100) NOT NULL, -- 权益标识符 (如 'app:premium_feature', 'max_projects:50')
    source_type VARCHAR(50), -- 来源 (如 'subscription', 'system_grant')
    source_id UUID, -- 关联 subscriptions(sub_id)
    expired_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(20) DEFAULT 'active'
);

-- 索引优化
CREATE INDEX IF NOT EXISTS idx_workflow_def ON project_workflows USING GIN (definition);
CREATE INDEX IF NOT EXISTS idx_project_owner ON projects(owner_id);
CREATE INDEX IF NOT EXISTS idx_assets_project ON project_assets(project_id);


/* =========================================================
   1. 初始化基础角色 (Roles)
   ========================================================= */
INSERT INTO roles (role_name, description) VALUES
    ('superadmin', '系统超级管理员，拥有全局最高权限'),
    ('owner', '团体/项目所有者，拥有当前域内的所有权限'),
    ('developer', '开发者，可以发布应用和创建项目编排'),
    ('member', '普通成员，仅具备基本访问和运行权限')
ON CONFLICT (role_name) DO NOTHING;

/* =========================================================
   2. 初始化基础权限 (Permissions)
   ========================================================= */
INSERT INTO permissions (permission_code, description) VALUES
    ('app:publish', '发布应用到市场'),
    ('app:delete', '删除市场应用'),
    ('app:subscribe', '订阅/购买应用'),
    ('project:create', '创建新项目'),
    ('project:edit', '编辑项目及编排'),
    ('project:delete', '删除项目'),
    ('group:invite', '邀请成员加入团体'),
    ('group:kick', '将成员移出团体'),
    ('group:view', '查看团体成员'),
    ('group:edit', '邀请成员加入团体'),
    ('group:delete', '删除团体')
ON CONFLICT (permission_code) DO NOTHING;

/* =========================================================
   3. 绑定角色-权限映射 (Role-Permissions)
   ========================================================= */
-- 给 superadmin 绑定所有权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id
FROM roles r, permissions p
WHERE r.role_name = 'superadmin'
ON CONFLICT DO NOTHING;

-- 给 owner 绑定项目级和群组级权限 (排除掉全局的 app:publish)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id
FROM roles r, permissions p
WHERE r.role_name = 'owner'
  AND p.permission_code IN ('project:create', 'project:edit', 'project:delete', 'group:invite', 'app:subscribe')
ON CONFLICT DO NOTHING;

-- 给 developer 绑定特定的开发权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id
FROM roles r, permissions p
WHERE r.role_name = 'developer'
  AND p.permission_code IN ('app:publish', 'project:create', 'project:edit', 'app:subscribe')
ON CONFLICT DO NOTHING;

/* =========================================================
   4. 初始化默认管理员账户 (Users)
   注意: password_hash 这里填入的是 '123456' 的 bcrypt 密文示例
   ========================================================= */
INSERT INTO users (user_id, username, email, password_hash, is_active)
VALUES (
    '00000000-0000-0000-0000-000000000001', 
    'admin', 
    'admin@example.com', 
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 
    TRUE
)
ON CONFLICT (username) DO NOTHING;
-- 如果邮箱也有 UNIQUE 约束，它也会被 ON CONFLICT 保护（Postgres 15+ 支持直接忽略冲突）

/* =========================================================
   5. 初始化默认系统团体/全局域 (Groups)
   ========================================================= */
INSERT INTO groups (group_id, group_name, owner_id, group_type)
VALUES (
    '11111111-1111-1111-1111-111111111111', 
    'System Global Group', 
    '00000000-0000-0000-0000-000000000001', 
    'system'
)
ON CONFLICT (group_id) DO NOTHING;

/* =========================================================
   6. 绑定管理员到系统团体，并赋予超级管理员角色 (Group Members)
   配合 Casbin 的 g 规则: g(admin_user_id, superadmin, system_group_id)
   ========================================================= */
INSERT INTO group_members (group_id, user_id, role_id)
SELECT 
    '11111111-1111-1111-1111-111111111111', -- group_id
    '00000000-0000-0000-0000-000000000001', -- user_id (admin)
    r.role_id                               -- role_id (superadmin)
FROM roles r
WHERE r.role_name = 'superadmin'
ON CONFLICT DO NOTHING;
-- ARCHIVED: legacy initialization script retained for historical reference only.
-- Use script/init_better.sql for local development and deployments.
