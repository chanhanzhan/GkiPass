package init

import (
	"database/sql"
	"fmt"
)

const (
	// SQLite 初始化脚本
	SQLiteInitSchema = `
-- 节点组表
CREATE TABLE IF NOT EXISTS node_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('entry', 'exit')),
    user_id TEXT NOT NULL,
    node_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    description TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_node_groups_user_id ON node_groups(user_id);
CREATE INDEX IF NOT EXISTS idx_node_groups_type ON node_groups(type);

-- 节点表
CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('client', 'server')),
    status TEXT NOT NULL DEFAULT 'offline' CHECK(status IN ('online', 'offline', 'error')),
    ip TEXT NOT NULL,
    port INTEGER NOT NULL,
    version TEXT,
    cert_id TEXT,
    api_key TEXT UNIQUE NOT NULL,
    group_id TEXT,
    user_id TEXT NOT NULL,
    last_seen DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    tags TEXT,
    description TEXT,
    FOREIGN KEY (group_id) REFERENCES node_groups(id) ON DELETE SET NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);
CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);
CREATE INDEX IF NOT EXISTS idx_nodes_api_key ON nodes(api_key);
CREATE INDEX IF NOT EXISTS idx_nodes_group_id ON nodes(group_id);
CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id);

-- 策略表
CREATE TABLE IF NOT EXISTS policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('protocol', 'acl', 'routing')),
    priority INTEGER NOT NULL DEFAULT 100,
    enabled INTEGER NOT NULL DEFAULT 1,
    config TEXT NOT NULL,
    node_ids TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    description TEXT
);

CREATE INDEX IF NOT EXISTS idx_policies_type ON policies(type);
CREATE INDEX IF NOT EXISTS idx_policies_enabled ON policies(enabled);
CREATE INDEX IF NOT EXISTS idx_policies_priority ON policies(priority);

-- 证书表
CREATE TABLE IF NOT EXISTS certificates (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK(type IN ('ca', 'leaf')),
    name TEXT NOT NULL,
    common_name TEXT NOT NULL,
    public_key TEXT NOT NULL,
    private_key TEXT NOT NULL,
    pin TEXT NOT NULL UNIQUE,
    parent_id TEXT,
    not_before DATETIME NOT NULL,
    not_after DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    revoked INTEGER NOT NULL DEFAULT 0,
    description TEXT,
    FOREIGN KEY (parent_id) REFERENCES certificates(id)
);

CREATE INDEX IF NOT EXISTS idx_certificates_type ON certificates(type);
CREATE INDEX IF NOT EXISTS idx_certificates_pin ON certificates(pin);
CREATE INDEX IF NOT EXISTS idx_certificates_revoked ON certificates(revoked);

-- 统计表
CREATE TABLE IF NOT EXISTS statistics (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    bytes_in INTEGER NOT NULL DEFAULT 0,
    bytes_out INTEGER NOT NULL DEFAULT 0,
    packets_in INTEGER NOT NULL DEFAULT 0,
    packets_out INTEGER NOT NULL DEFAULT 0,
    connections INTEGER NOT NULL DEFAULT 0,
    active_sessions INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0,
    avg_latency REAL NOT NULL DEFAULT 0,
    cpu_usage REAL NOT NULL DEFAULT 0,
    memory_usage INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_statistics_node_id ON statistics(node_id);
CREATE INDEX IF NOT EXISTS idx_statistics_timestamp ON statistics(timestamp);

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT,
    email TEXT UNIQUE,
    avatar TEXT,
    provider TEXT NOT NULL DEFAULT 'local' CHECK(provider IN ('local', 'github')),
    provider_id TEXT,
    role TEXT NOT NULL DEFAULT 'user' CHECK(role IN ('admin', 'user')),
    enabled INTEGER NOT NULL DEFAULT 1,
    last_login_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_enabled ON users(enabled);
CREATE INDEX IF NOT EXISTS idx_users_provider ON users(provider, provider_id);

-- Connection Keys 表
CREATE TABLE IF NOT EXISTS connection_keys (
    id TEXT PRIMARY KEY,
    key TEXT UNIQUE NOT NULL,
    node_id TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('node', 'user')),
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ck_key ON connection_keys(key);
CREATE INDEX IF NOT EXISTS idx_ck_node_id ON connection_keys(node_id);
CREATE INDEX IF NOT EXISTS idx_ck_expires ON connection_keys(expires_at);

-- 套餐表
CREATE TABLE IF NOT EXISTS plans (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    max_rules INTEGER NOT NULL DEFAULT 0,
    max_traffic INTEGER NOT NULL DEFAULT 0,
    max_bandwidth INTEGER NOT NULL DEFAULT 0,
    max_connections INTEGER NOT NULL DEFAULT 0,
    max_connect_ips INTEGER NOT NULL DEFAULT 0,
    allowed_node_ids TEXT,
    billing_cycle TEXT NOT NULL DEFAULT 'monthly' CHECK(billing_cycle IN ('monthly', 'yearly', 'permanent')),
    price REAL NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    description TEXT
);

CREATE INDEX IF NOT EXISTS idx_plans_enabled ON plans(enabled);

-- 用户订阅表
CREATE TABLE IF NOT EXISTS user_subscriptions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    plan_id TEXT NOT NULL,
    start_date DATETIME NOT NULL,
    end_date DATETIME NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'expired', 'cancelled')),
    used_rules INTEGER NOT NULL DEFAULT 0,
    used_traffic INTEGER NOT NULL DEFAULT 0,
    traffic_reset DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_subs_user_id ON user_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subs_status ON user_subscriptions(status);

-- 隧道表
CREATE TABLE IF NOT EXISTS tunnels (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL CHECK(protocol IN ('tcp', 'udp', 'http', 'https')),
    entry_group_id TEXT NOT NULL,
    exit_group_id TEXT NOT NULL,
    local_port INTEGER NOT NULL UNIQUE,
    targets TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    traffic_in INTEGER NOT NULL DEFAULT 0,
    traffic_out INTEGER NOT NULL DEFAULT 0,
    connection_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    description TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (entry_group_id) REFERENCES node_groups(id) ON DELETE RESTRICT,
    FOREIGN KEY (exit_group_id) REFERENCES node_groups(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_tunnels_user_id ON tunnels(user_id);
CREATE INDEX IF NOT EXISTS idx_tunnels_enabled ON tunnels(enabled);
CREATE INDEX IF NOT EXISTS idx_tunnels_entry_group ON tunnels(entry_group_id);
CREATE INDEX IF NOT EXISTS idx_tunnels_exit_group ON tunnels(exit_group_id);
CREATE INDEX IF NOT EXISTS idx_tunnels_local_port ON tunnels(local_port);

-- 钱包表
CREATE TABLE IF NOT EXISTS wallets (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    balance REAL NOT NULL DEFAULT 0,
    frozen REAL NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- 钱包交易记录表
CREATE TABLE IF NOT EXISTS wallet_transactions (
    id TEXT PRIMARY KEY,
    wallet_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('recharge', 'consume', 'refund', 'withdraw')),
    amount REAL NOT NULL,
    balance REAL NOT NULL,
    related_id TEXT,
    related_type TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'completed', 'failed', 'cancelled')),
    payment_method TEXT,
    transaction_no TEXT,
    description TEXT,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- 通知表
CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    user_id TEXT,
    type TEXT NOT NULL CHECK(type IN ('system', 'subscription', 'traffic', 'security')),
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    link TEXT,
    is_read BOOLEAN NOT NULL DEFAULT 0,
    priority TEXT NOT NULL DEFAULT 'normal' CHECK(priority IN ('low', 'normal', 'high', 'urgent')),
    created_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- 公告表
CREATE TABLE IF NOT EXISTS announcements (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('notice', 'maintenance', 'update', 'warning')),
    priority TEXT NOT NULL DEFAULT 'normal' CHECK(priority IN ('low', 'normal', 'high')),
    enabled BOOLEAN NOT NULL DEFAULT 1,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    created_by TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_wallets_user_id ON wallets(user_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_user_id ON wallet_transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_wallet_id ON wallet_transactions(wallet_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_created_at ON wallet_transactions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_is_read ON notifications(is_read);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_announcements_enabled ON announcements(enabled);
CREATE INDEX IF NOT EXISTS idx_announcements_time ON announcements(start_time, end_time);

-- 插入默认套餐
INSERT OR IGNORE INTO plans (id, name, max_rules, max_traffic, max_bandwidth, max_connections, billing_cycle, price, description)
VALUES ('free-plan', '免费套餐', 3, 10737418240, 1048576, 10, 'monthly', 0, '免费用户套餐：3个规则，10GB流量/月，1Mbps带宽');

INSERT OR IGNORE INTO plans (id, name, max_rules, max_traffic, max_bandwidth, max_connections, billing_cycle, price, description)
VALUES ('unlimited-plan', '无限套餐', 0, 0, 0, 0, 'permanent', 0, '管理员无限套餐');
`
)

// InitSQLiteSchema 初始化 SQLite 数据库schema
func InitSQLiteSchema(db *sql.DB) error {
	_, err := db.Exec(SQLiteInitSchema)
	if err != nil {
		return fmt.Errorf("failed to initialize SQLite schema: %w", err)
	}
	return nil
}

// CreateTriggers 创建触发器
func CreateTriggers(db *sql.DB) error {
	triggers := []string{
		// 节点更新时间触发器
		`CREATE TRIGGER IF NOT EXISTS update_nodes_timestamp 
		 AFTER UPDATE ON nodes 
		 BEGIN 
		     UPDATE nodes SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
		 END;`,

		// 节点组更新时间触发器
		`CREATE TRIGGER IF NOT EXISTS update_node_groups_timestamp 
		 AFTER UPDATE ON node_groups 
		 BEGIN 
		     UPDATE node_groups SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
		 END;`,

		// 策略更新时间触发器
		`CREATE TRIGGER IF NOT EXISTS update_policies_timestamp 
		 AFTER UPDATE ON policies 
		 BEGIN 
		     UPDATE policies SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
		 END;`,

		// 用户更新时间触发器
		`CREATE TRIGGER IF NOT EXISTS update_users_timestamp 
		 AFTER UPDATE ON users 
		 BEGIN 
		     UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
		 END;`,

		// 节点组节点数量自动更新
		`CREATE TRIGGER IF NOT EXISTS update_node_group_count_insert 
		 AFTER INSERT ON nodes 
		 WHEN NEW.group_id IS NOT NULL
		 BEGIN 
		     UPDATE node_groups SET node_count = (
		         SELECT COUNT(*) FROM nodes WHERE group_id = NEW.group_id
		     ) WHERE id = NEW.group_id;
		 END;`,

		`CREATE TRIGGER IF NOT EXISTS update_node_group_count_delete 
		 AFTER DELETE ON nodes 
		 WHEN OLD.group_id IS NOT NULL
		 BEGIN 
		     UPDATE node_groups SET node_count = (
		         SELECT COUNT(*) FROM nodes WHERE group_id = OLD.group_id
		     ) WHERE id = OLD.group_id;
		 END;`,

		`CREATE TRIGGER IF NOT EXISTS update_node_group_count_update 
		 AFTER UPDATE ON nodes 
		 WHEN OLD.group_id IS NOT NEW.group_id
		 BEGIN 
		     UPDATE node_groups SET node_count = (
		         SELECT COUNT(*) FROM nodes WHERE group_id = OLD.group_id
		     ) WHERE id = OLD.group_id;
		     UPDATE node_groups SET node_count = (
		         SELECT COUNT(*) FROM nodes WHERE group_id = NEW.group_id
		     ) WHERE id = NEW.group_id AND NEW.group_id IS NOT NULL;
		 END;`,
	}

	for _, trigger := range triggers {
		if _, err := db.Exec(trigger); err != nil {
			return fmt.Errorf("failed to create trigger: %w", err)
		}
	}

	return nil
}
