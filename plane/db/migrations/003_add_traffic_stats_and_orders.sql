-- 隧道流量统计表
CREATE TABLE IF NOT EXISTS tunnel_traffic_stats (
    id TEXT PRIMARY KEY,
    tunnel_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    entry_group_id TEXT NOT NULL,
    exit_group_id TEXT NOT NULL,
    traffic_in INTEGER NOT NULL DEFAULT 0,
    traffic_out INTEGER NOT NULL DEFAULT 0,
    billed_traffic_in INTEGER NOT NULL DEFAULT 0,
    billed_traffic_out INTEGER NOT NULL DEFAULT 0,
    date DATE NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (tunnel_id) REFERENCES tunnels(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- 支付订单表
CREATE TABLE IF NOT EXISTS payment_orders (
    id TEXT PRIMARY KEY,
    order_no TEXT NOT NULL UNIQUE,
    user_id TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('recharge', 'subscription')),
    amount REAL NOT NULL,
    payment_method TEXT,
    payment_status TEXT NOT NULL DEFAULT 'pending' CHECK(payment_status IN ('pending', 'processing', 'completed', 'failed', 'cancelled', 'refunded')),
    payment_data TEXT,
    related_id TEXT,
    related_type TEXT,
    callback_data TEXT,
    completed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- 节点组配置表（扩展）
CREATE TABLE IF NOT EXISTS node_group_configs (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL UNIQUE,
    port_range TEXT,
    allowed_protocols TEXT,
    allowed_tunnel_types TEXT,
    ip_type TEXT DEFAULT 'auto' CHECK(ip_type IN ('auto', 'ipv4', 'ipv6', 'dual')),
    allow_entry_protocols BOOLEAN DEFAULT 1,
    traffic_multiplier REAL DEFAULT 1.0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (group_id) REFERENCES node_groups(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_tunnel_traffic_stats_tunnel_id ON tunnel_traffic_stats(tunnel_id);
CREATE INDEX IF NOT EXISTS idx_tunnel_traffic_stats_user_id ON tunnel_traffic_stats(user_id);
CREATE INDEX IF NOT EXISTS idx_tunnel_traffic_stats_date ON tunnel_traffic_stats(date DESC);
CREATE INDEX IF NOT EXISTS idx_payment_orders_user_id ON payment_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_payment_orders_order_no ON payment_orders(order_no);
CREATE INDEX IF NOT EXISTS idx_payment_orders_status ON payment_orders(payment_status);
CREATE INDEX IF NOT EXISTS idx_payment_orders_created_at ON payment_orders(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_group_configs_group_id ON node_group_configs(group_id);

