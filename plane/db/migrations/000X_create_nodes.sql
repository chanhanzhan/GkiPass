-- 创建节点表
CREATE TABLE IF NOT EXISTS nodes (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'offline',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_online TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    hardware_id VARCHAR(255),
    system_info TEXT,
    ip_address VARCHAR(45),
    version VARCHAR(50),
    
    role VARCHAR(20) NOT NULL
);

-- 创建节点指标表
CREATE TABLE IF NOT EXISTS node_metrics (
    id VARCHAR(36) PRIMARY KEY,
    node_id VARCHAR(36) NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cpu_usage REAL NOT NULL,
    memory_usage REAL NOT NULL,
    disk_usage REAL NOT NULL,
    network_in BIGINT NOT NULL,
    network_out BIGINT NOT NULL,
    connections INTEGER NOT NULL,
    goroutines INTEGER NOT NULL,
    gc_pause BIGINT NOT NULL,
    
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- 创建节点日志表
CREATE TABLE IF NOT EXISTS node_logs (
    id VARCHAR(36) PRIMARY KEY,
    node_id VARCHAR(36) NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    level VARCHAR(10) NOT NULL,
    message TEXT NOT NULL,
    source VARCHAR(50),
    details TEXT,
    
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);
CREATE INDEX IF NOT EXISTS idx_nodes_role ON nodes(role);
CREATE INDEX IF NOT EXISTS idx_node_metrics_node_id ON node_metrics(node_id);
CREATE INDEX IF NOT EXISTS idx_node_metrics_timestamp ON node_metrics(timestamp);
CREATE INDEX IF NOT EXISTS idx_node_logs_node_id ON node_logs(node_id);
CREATE INDEX IF NOT EXISTS idx_node_logs_timestamp ON node_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_node_logs_level ON node_logs(level);
