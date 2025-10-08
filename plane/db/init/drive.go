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

-- 节点监控配置表
CREATE TABLE IF NOT EXISTS node_monitoring_config (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL UNIQUE,
    monitoring_enabled INTEGER NOT NULL DEFAULT 1,
    report_interval INTEGER NOT NULL DEFAULT 60, -- 上报间隔(秒)
    collect_system_info INTEGER NOT NULL DEFAULT 1, -- 收集系统信息
    collect_network_stats INTEGER NOT NULL DEFAULT 1, -- 收集网络统计
    collect_tunnel_stats INTEGER NOT NULL DEFAULT 1, -- 收集隧道统计
    collect_performance INTEGER NOT NULL DEFAULT 1, -- 收集性能数据
    data_retention_days INTEGER NOT NULL DEFAULT 30, -- 数据保留天数
    alert_cpu_threshold REAL NOT NULL DEFAULT 80.0, -- CPU告警阈值
    alert_memory_threshold REAL NOT NULL DEFAULT 80.0, -- 内存告警阈值
    alert_disk_threshold REAL NOT NULL DEFAULT 80.0, -- 磁盘告警阈值
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_monitoring_config_node_id ON node_monitoring_config(node_id);
CREATE INDEX IF NOT EXISTS idx_monitoring_config_enabled ON node_monitoring_config(monitoring_enabled);

-- 节点实时监控数据表
CREATE TABLE IF NOT EXISTS node_monitoring_data (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    -- 系统基础信息
    system_uptime INTEGER NOT NULL DEFAULT 0, -- 系统运行时间(秒)
    boot_time DATETIME, -- 开机时间
    -- CPU信息
    cpu_usage REAL NOT NULL DEFAULT 0, -- CPU使用率(0-100)
    cpu_load_1m REAL NOT NULL DEFAULT 0, -- 1分钟负载
    cpu_load_5m REAL NOT NULL DEFAULT 0, -- 5分钟负载
    cpu_load_15m REAL NOT NULL DEFAULT 0, -- 15分钟负载
    cpu_cores INTEGER NOT NULL DEFAULT 0, -- CPU核心数
    -- 内存信息
    memory_total INTEGER NOT NULL DEFAULT 0, -- 总内存(bytes)
    memory_used INTEGER NOT NULL DEFAULT 0, -- 已用内存(bytes)
    memory_available INTEGER NOT NULL DEFAULT 0, -- 可用内存(bytes)
    memory_usage_percent REAL NOT NULL DEFAULT 0, -- 内存使用率(0-100)
    -- 磁盘信息
    disk_total INTEGER NOT NULL DEFAULT 0, -- 总磁盘空间(bytes)
    disk_used INTEGER NOT NULL DEFAULT 0, -- 已用磁盘空间(bytes)
    disk_available INTEGER NOT NULL DEFAULT 0, -- 可用磁盘空间(bytes)
    disk_usage_percent REAL NOT NULL DEFAULT 0, -- 磁盘使用率(0-100)
    -- 网络信息
    network_interfaces TEXT, -- 网络接口信息(JSON)
    bandwidth_in INTEGER NOT NULL DEFAULT 0, -- 入站带宽(bps)
    bandwidth_out INTEGER NOT NULL DEFAULT 0, -- 出站带宽(bps)
    -- 连接信息
    tcp_connections INTEGER NOT NULL DEFAULT 0, -- TCP连接数
    udp_connections INTEGER NOT NULL DEFAULT 0, -- UDP连接数
    active_tunnels INTEGER NOT NULL DEFAULT 0, -- 活跃隧道数
    total_connections INTEGER NOT NULL DEFAULT 0, -- 总连接数
    -- 流量统计
    traffic_in_bytes INTEGER NOT NULL DEFAULT 0, -- 入站流量(bytes)
    traffic_out_bytes INTEGER NOT NULL DEFAULT 0, -- 出站流量(bytes)
    packets_in INTEGER NOT NULL DEFAULT 0, -- 入站数据包数
    packets_out INTEGER NOT NULL DEFAULT 0, -- 出站数据包数
    -- 错误统计
    connection_errors INTEGER NOT NULL DEFAULT 0, -- 连接错误数
    tunnel_errors INTEGER NOT NULL DEFAULT 0, -- 隧道错误数
    -- 性能指标
    avg_response_time REAL NOT NULL DEFAULT 0, -- 平均响应时间(ms)
    max_response_time REAL NOT NULL DEFAULT 0, -- 最大响应时间(ms)
    min_response_time REAL NOT NULL DEFAULT 0, -- 最小响应时间(ms)
    -- 应用级信息
    app_version TEXT, -- 应用版本
    go_version TEXT, -- Go版本
    os_info TEXT, -- 操作系统信息
    -- 配置信息
    node_config_version TEXT, -- 节点配置版本
    last_config_update DATETIME, -- 最后配置更新时间
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_monitoring_data_node_id ON node_monitoring_data(node_id);
CREATE INDEX IF NOT EXISTS idx_monitoring_data_timestamp ON node_monitoring_data(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_monitoring_data_node_time ON node_monitoring_data(node_id, timestamp DESC);

-- 节点性能历史数据表（聚合数据）
CREATE TABLE IF NOT EXISTS node_performance_history (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL,
    date DATE NOT NULL, -- 统计日期
    aggregation_type TEXT NOT NULL CHECK(aggregation_type IN ('hourly', 'daily', 'monthly')),
    aggregation_time DATETIME NOT NULL, -- 聚合时间点
    -- 平均值统计
    avg_cpu_usage REAL NOT NULL DEFAULT 0,
    avg_memory_usage REAL NOT NULL DEFAULT 0,
    avg_disk_usage REAL NOT NULL DEFAULT 0,
    avg_bandwidth_in INTEGER NOT NULL DEFAULT 0,
    avg_bandwidth_out INTEGER NOT NULL DEFAULT 0,
    avg_connections INTEGER NOT NULL DEFAULT 0,
    avg_response_time REAL NOT NULL DEFAULT 0,
    -- 最大值统计
    max_cpu_usage REAL NOT NULL DEFAULT 0,
    max_memory_usage REAL NOT NULL DEFAULT 0,
    max_connections INTEGER NOT NULL DEFAULT 0,
    max_response_time REAL NOT NULL DEFAULT 0,
    -- 流量统计
    total_traffic_in INTEGER NOT NULL DEFAULT 0,
    total_traffic_out INTEGER NOT NULL DEFAULT 0,
    total_packets_in INTEGER NOT NULL DEFAULT 0,
    total_packets_out INTEGER NOT NULL DEFAULT 0,
    -- 错误统计
    total_errors INTEGER NOT NULL DEFAULT 0,
    -- 可用性统计
    uptime_seconds INTEGER NOT NULL DEFAULT 0, -- 在线时间(秒)
    downtime_seconds INTEGER NOT NULL DEFAULT 0, -- 离线时间(秒)
    availability_percent REAL NOT NULL DEFAULT 0, -- 可用性百分比
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_performance_history_node_id ON node_performance_history(node_id);
CREATE INDEX IF NOT EXISTS idx_performance_history_date ON node_performance_history(date DESC);
CREATE INDEX IF NOT EXISTS idx_performance_history_type ON node_performance_history(aggregation_type);
CREATE INDEX IF NOT EXISTS idx_performance_history_time ON node_performance_history(aggregation_time DESC);

-- 监控权限配置表
CREATE TABLE IF NOT EXISTS monitoring_permissions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    node_id TEXT, -- NULL表示全局配置
    permission_type TEXT NOT NULL CHECK(permission_type IN ('view_basic', 'view_detailed', 'view_system', 'view_network', 'disabled')),
    enabled INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL, -- 管理员ID
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    description TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE(user_id, node_id)
);

CREATE INDEX IF NOT EXISTS idx_monitoring_permissions_user_id ON monitoring_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_monitoring_permissions_node_id ON monitoring_permissions(node_id);
CREATE INDEX IF NOT EXISTS idx_monitoring_permissions_type ON monitoring_permissions(permission_type);

-- 节点告警规则表
CREATE TABLE IF NOT EXISTS node_alert_rules (
    id TEXT PRIMARY KEY,
    node_id TEXT, -- NULL表示全局规则
    rule_name TEXT NOT NULL,
    metric_type TEXT NOT NULL CHECK(metric_type IN ('cpu', 'memory', 'disk', 'network', 'connections', 'response_time', 'uptime')),
    operator TEXT NOT NULL CHECK(operator IN ('>', '<', '>=', '<=', '=', '!=')),
    threshold_value REAL NOT NULL,
    duration_seconds INTEGER NOT NULL DEFAULT 60, -- 持续时间
    severity TEXT NOT NULL DEFAULT 'warning' CHECK(severity IN ('info', 'warning', 'critical')),
    enabled INTEGER NOT NULL DEFAULT 1,
    notification_channels TEXT, -- 通知渠道(JSON)
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_node_id ON node_alert_rules(node_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON node_alert_rules(enabled);
CREATE INDEX IF NOT EXISTS idx_alert_rules_severity ON node_alert_rules(severity);

-- 节点告警历史表
CREATE TABLE IF NOT EXISTS node_alert_history (
    id TEXT PRIMARY KEY,
    rule_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    alert_type TEXT NOT NULL,
    severity TEXT NOT NULL,
    message TEXT NOT NULL,
    metric_value REAL,
    threshold_value REAL,
    status TEXT NOT NULL DEFAULT 'triggered' CHECK(status IN ('triggered', 'acknowledged', 'resolved')),
    triggered_at DATETIME NOT NULL,
    acknowledged_at DATETIME,
    resolved_at DATETIME,
    acknowledged_by TEXT, -- 用户ID
    details TEXT, -- 详细信息(JSON)
    FOREIGN KEY (rule_id) REFERENCES node_alert_rules(id) ON DELETE CASCADE,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (acknowledged_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_alert_history_node_id ON node_alert_history(node_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_status ON node_alert_history(status);
CREATE INDEX IF NOT EXISTS idx_alert_history_triggered ON node_alert_history(triggered_at DESC);

-- 插入默认套餐
INSERT OR IGNORE INTO plans (id, name, max_rules, max_traffic, max_bandwidth, max_connections, billing_cycle, price, description)
VALUES ('free-plan', '免费套餐', 3, 10737418240, 1048576, 10, 'monthly', 0, '免费用户套餐：3个规则，10GB流量/月，1Mbps带宽');

INSERT OR IGNORE INTO plans (id, name, max_rules, max_traffic, max_bandwidth, max_connections, billing_cycle, price, description)
VALUES ('unlimited-plan', '无限套餐', 0, 0, 0, 0, 'permanent', 0, '管理员无限套餐');

-- 插入默认监控告警规则
INSERT OR IGNORE INTO node_alert_rules (id, node_id, rule_name, metric_type, operator, threshold_value, severity, notification_channels)
VALUES ('global-cpu-warning', NULL, 'CPU使用率告警', 'cpu', '>', 80.0, 'warning', '["system"]');

INSERT OR IGNORE INTO node_alert_rules (id, node_id, rule_name, metric_type, operator, threshold_value, severity, notification_channels)
VALUES ('global-memory-warning', NULL, '内存使用率告警', 'memory', '>', 80.0, 'warning', '["system"]');

INSERT OR IGNORE INTO node_alert_rules (id, node_id, rule_name, metric_type, operator, threshold_value, severity, notification_channels)
VALUES ('global-disk-critical', NULL, '磁盘使用率告警', 'disk', '>', 90.0, 'critical', '["system"]');

INSERT OR IGNORE INTO node_alert_rules (id, node_id, rule_name, metric_type, operator, threshold_value, severity, notification_channels)
VALUES ('global-response-warning', NULL, '响应时间告警', 'response_time', '>', 1000.0, 'warning', '["system"]');
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
