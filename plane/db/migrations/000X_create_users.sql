-- 创建用户表
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login TIMESTAMP
);

-- 创建权限表
CREATE TABLE IF NOT EXISTS permissions (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 创建用户权限表
CREATE TABLE IF NOT EXISTS user_permissions (
    user_id VARCHAR(36) NOT NULL,
    permission_id VARCHAR(36) NOT NULL,
    granted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    granted_by VARCHAR(36) NOT NULL,
    PRIMARY KEY (user_id, permission_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
    FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE CASCADE
);

-- 创建角色权限表
CREATE TABLE IF NOT EXISTS role_permissions (
    role VARCHAR(20) NOT NULL,
    permission_id VARCHAR(36) NOT NULL,
    granted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (role, permission_id),
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);

-- 创建探测权限表
CREATE TABLE IF NOT EXISTS probe_permissions (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    node_id VARCHAR(36),
    group_id VARCHAR(36),
    granted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    granted_by VARCHAR(36) NOT NULL,
    expiration TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (group_id) REFERENCES node_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE CASCADE,
    CHECK ((node_id IS NULL AND group_id IS NOT NULL) OR (node_id IS NOT NULL AND group_id IS NULL))
);

-- 创建API密钥表
CREATE TABLE IF NOT EXISTS api_keys (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    name VARCHAR(50) NOT NULL,
    key VARCHAR(64) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_enabled ON users(enabled);
CREATE INDEX IF NOT EXISTS idx_user_permissions_user_id ON user_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_permissions_permission_id ON user_permissions(permission_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_role ON role_permissions(role);
CREATE INDEX IF NOT EXISTS idx_probe_permissions_user_id ON probe_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_probe_permissions_node_id ON probe_permissions(node_id);
CREATE INDEX IF NOT EXISTS idx_probe_permissions_group_id ON probe_permissions(group_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_enabled ON api_keys(enabled);

-- 插入默认权限
INSERT INTO permissions (id, name, description) VALUES
    ('perm_admin', 'admin', '管理员权限'),
    ('perm_user_manage', 'user_manage', '用户管理权限'),
    ('perm_node_manage', 'node_manage', '节点管理权限'),
    ('perm_group_manage', 'group_manage', '节点组管理权限'),
    ('perm_rule_manage', 'rule_manage', '规则管理权限'),
    ('perm_tunnel_manage', 'tunnel_manage', '隧道管理权限'),
    ('perm_probe', 'probe', '探测权限'),
    ('perm_view_metrics', 'view_metrics', '查看指标权限'),
    ('perm_view_logs', 'view_logs', '查看日志权限');

-- 插入角色权限
INSERT INTO role_permissions (role, permission_id) VALUES
    ('admin', 'perm_admin'),
    ('admin', 'perm_user_manage'),
    ('admin', 'perm_node_manage'),
    ('admin', 'perm_group_manage'),
    ('admin', 'perm_rule_manage'),
    ('admin', 'perm_tunnel_manage'),
    ('admin', 'perm_probe'),
    ('admin', 'perm_view_metrics'),
    ('admin', 'perm_view_logs'),
    
    ('user', 'perm_node_manage'),
    ('user', 'perm_tunnel_manage'),
    ('user', 'perm_probe'),
    ('user', 'perm_view_metrics'),
    ('user', 'perm_view_logs'),
    
    ('guest', 'perm_view_metrics'),
    ('guest', 'perm_view_logs');

-- 插入默认管理员用户
INSERT INTO users (id, username, email, password, role, enabled, created_at, updated_at)
VALUES ('admin', 'admin', 'admin@example.com', '$2a$10$EqKCjUQBqsKE9A/QZQiVwOLqtQ.hcjAITJKXK4MCjCGQmVwQOp1Oe', 'admin', true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
-- 密码为 'admin'，使用bcrypt加密
