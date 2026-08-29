-- ziway-ambs initial schema v2
-- 知味生态 AMBS 系统/鉴权服务
-- 对齐 12U 角色系统 LOCKED 定义：12基础角色 + 2帽子 + NHI

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    user_id       VARCHAR(32) UNIQUE NOT NULL,
    identity_type VARCHAR(10) DEFAULT 'human',   -- human / nhi
    phone         VARCHAR(20) UNIQUE,
    email         VARCHAR(100) UNIQUE,
    password_hash VARCHAR(255),
    nickname      VARCHAR(50),
    avatar_url    VARCHAR(500),
    agent_service VARCHAR(50),                   -- NHI: Agent服务名
    delegated_by  VARCHAR(32),                   -- NHI: 委托方UserID
    status        SMALLINT DEFAULT 1,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_users_phone ON users(phone);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_status ON users(status);
CREATE INDEX idx_users_identity_type ON users(identity_type);

-- 角色表（12U基础角色 + 2帽子角色 + NHI）
CREATE TABLE IF NOT EXISTS roles (
    id          BIGSERIAL PRIMARY KEY,
    role_code   VARCHAR(20) UNIQUE NOT NULL,
    role_name   VARCHAR(50) NOT NULL,
    role_type   VARCHAR(10) DEFAULT 'base',      -- base / hat / nhi
    domain      VARCHAR(20),
    parent_role VARCHAR(20),                      -- 帽子归属：CX→HU, FX→HU/FU
    description VARCHAR(200),
    status      SMALLINT DEFAULT 1,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_roles_domain ON roles(domain);
CREATE INDEX idx_roles_type ON roles(role_type);

-- 权限表
CREATE TABLE IF NOT EXISTS permissions (
    id         BIGSERIAL PRIMARY KEY,
    perm_code  VARCHAR(80) UNIQUE NOT NULL,
    resource   VARCHAR(200),
    action     VARCHAR(10),
    domain     VARCHAR(20),
    status     SMALLINT DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_permissions_domain ON permissions(domain);

-- 用户-角色关联（含domain，支持戴帽子）
CREATE TABLE IF NOT EXISTS user_roles (
    user_id    BIGINT NOT NULL REFERENCES users(id),
    role_id    BIGINT NOT NULL REFERENCES roles(id),
    domain     VARCHAR(20) NOT NULL,
    granted_by BIGINT,
    granted_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, role_id, domain)
);

-- 角色-权限关联
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id    BIGINT NOT NULL REFERENCES roles(id),
    perm_id    BIGINT NOT NULL REFERENCES permissions(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (role_id, perm_id)
);

-- Casbin策略表（由gorm-adapter自动管理）
CREATE TABLE IF NOT EXISTS casbin_rule (
    id    BIGSERIAL PRIMARY KEY,
    ptype VARCHAR(12),
    v0    VARCHAR(128),
    v1    VARCHAR(128),
    v2    VARCHAR(128),
    v3    VARCHAR(128),
    v4    VARCHAR(128),
    v5    VARCHAR(128)
);

CREATE INDEX idx_casbin_rule_ptype ON casbin_rule(ptype);
CREATE INDEX idx_casbin_rule_v0 ON casbin_rule(v0);

-- ============================================================
-- 插入12U基础角色 + 2帽子 + NHI（对齐LOCKED定义）
-- ============================================================
INSERT INTO roles (role_code, role_name, role_type, domain, parent_role, description) VALUES
    -- 12U基础角色
    ('CU', '消费者',           'base', 'mall',   NULL,  '浏览、下单、支付、评价、退款申请'),
    ('DU', '门店经营者',       'base', 'shop',   NULL,  '商品管理、接单、库存、经营看板、经营级VCASE'),
    ('PU', '创研者',           'base', 'lab',    NULL,  'NPI项目、产品定义、原型研发、知识产权'),
    ('EU', '供应商/资源管理者', 'base', 'market', NULL,  '货源发布、采购、账期、仓储、产能、设备调度'),
    ('HU', '工作者',           'base', 'mate',   NULL,  '接单、排班、工单执行、打卡、提现、技能认证'),
    ('OU', '治理者/董事长',     'base', 'owner',  NULL,  'VCASE/FCASE/ICASE/XCASE审批、多重签名、战略决策（主入口：ziway-Owner）'),
    ('GU', '监管者',           'base', 'case',   NULL,  '合规审计、神案执行监控、异常告警（只读）'),
    ('AU', '运维/系统管理员',   'base', 'admin',  NULL,  '服务启停、配置变更、API网关、消息队列、Agent熔断（主入口：ziway-Admin）'),
    ('FU', '财务',             'base', 'case',   NULL,  '财务报表、对账、月结、税务、GP分配执行'),
    ('IU', '投资人',           'base', 'case',   NULL,  '投资专案查看、尽调、投资决议投票、分红查看'),
    ('VU', '运营执行官',       'base', 'vmbs',   NULL,  'VCASE预算编制、资源调度执行、运营监控、报告生成（主入口：VMBS）'),
    ('SU', '孵化创业者',       'base', 'lab',    NULL,  '孵化项目创建、资源申请、路演、接受投资、毕业'),
    -- 帽子角色
    ('CX', '客户体验专员',     'hat',  'mall',   'HU',  '客诉处理、工单跟进、客户回访、满意度调查'),
    ('FX', '退款专员',         'hat',  'case',   'HU',  '退款审核、执行、争议处理、退款报表'),
    -- 非人类身份
    ('NHI','非人类身份(Agent)','nhi',  'ambs',   NULL,  'ziway-Agent运行时身份，继承调用用户权限边界')
ON CONFLICT (role_code) DO UPDATE SET
    role_name   = EXCLUDED.role_name,
    role_type   = EXCLUDED.role_type,
    domain      = EXCLUDED.domain,
    parent_role = EXCLUDED.parent_role,
    description = EXCLUDED.description;

-- ============================================================
-- Casbin基础策略
-- ============================================================
-- 人类用户基础权限（各角色在自己domain下可访问自身信息）
INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4) VALUES
    -- 所有已认证用户通用权限
    ('p', 'CU', 'mall', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'CU', 'mall', '/api/v1/auth/me', 'GET', 'allow'),
    ('p', 'CU', 'mall', '/api/v1/auth/switch-hat', 'POST', 'allow'),
    ('p', 'CU', 'mall', '/api/v1/auth/logout', 'POST', 'allow'),
    -- DU在shop域
    ('p', 'DU', 'shop', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'DU', 'shop', '/api/v1/auth/me', 'GET', 'allow'),
    ('p', 'DU', 'shop', '/api/v1/auth/switch-hat', 'POST', 'allow'),
    -- PU在lab域
    ('p', 'PU', 'lab', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'PU', 'lab', '/api/v1/auth/me', 'GET', 'allow'),
    -- EU在market/dyard域
    ('p', 'EU', 'market', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'EU', 'dyard', '/api/v1/users/me/profile', 'GET', 'allow'),
    -- HU在mate域 + CX帽子在mall域
    ('p', 'HU', 'mate', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'CX', 'mall', '/api/v1/users/me/profile', 'GET', 'allow'),
    -- OU/GU/FU/IU/VU在各自domain下
    ('p', 'OU', 'owner', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'GU', 'case', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'FU', 'case', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'IU', 'case', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'VU', 'vmbs', '/api/v1/users/me/profile', 'GET', 'allow'),
    -- FX帽子在case+mall域
    ('p', 'FX', 'case', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'FX', 'mall', '/api/v1/users/me/profile', 'GET', 'allow'),
    -- SU在lab+market+case域（孵化跨域）
    ('p', 'SU', 'lab', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'SU', 'market', '/api/v1/users/me/profile', 'GET', 'allow'),
    ('p', 'SU', 'case', '/api/v1/users/me/profile', 'GET', 'allow'),
    -- AU管理员权限（admin域）
    ('p', 'AU', 'admin', '/api/v1/users', 'GET', 'allow'),
    ('p', 'AU', 'admin', '/api/v1/users/:id', 'GET', 'allow'),
    ('p', 'AU', 'admin', '/api/v1/auth/me', 'GET', 'allow'),
    -- NHI: Agent本身无独立权限，通过委托继承，此处只给基础认证
    ('p', 'NHI', 'ambs', '/api/v1/auth/me', 'GET', 'allow')
ON CONFLICT DO NOTHING;

-- 帽子继承关系（g = role inheritance）
-- CX继承HU在mall域的权限子集, FX可由HU或FU穿戴
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
    ('g', 'CX', 'HU', 'mall'),
    ('g', 'FX', 'HU', 'case'),
    ('g', 'FX', 'FU', 'case')
ON CONFLICT DO NOTHING;
