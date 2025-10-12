-- 创建隧道表
CREATE TABLE IF NOT EXISTS tunnels (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(36) NOT NULL,
    
    ingress_node_id VARCHAR(36),
    egress_node_id VARCHAR(36),
    ingress_group_id VARCHAR(36),
    egress_group_id VARCHAR(36),
    ingress_protocol VARCHAR(20),
    egress_protocol VARCHAR(20),
    
    listen_port INT NOT NULL,
    target_address VARCHAR(255) NOT NULL,
    target_port INT NOT NULL,
    enable_encryption BOOLEAN NOT NULL DEFAULT FALSE,
    
    rate_limit_bps BIGINT NOT NULL DEFAULT 0,
    max_connections INT NOT NULL DEFAULT 0,
    idle_timeout INT NOT NULL DEFAULT 0,
    
    FOREIGN KEY (ingress_node_id) REFERENCES nodes(id) ON DELETE SET NULL,
    FOREIGN KEY (egress_node_id) REFERENCES nodes(id) ON DELETE SET NULL,
    FOREIGN KEY (ingress_group_id) REFERENCES node_groups(id) ON DELETE SET NULL,
    FOREIGN KEY (egress_group_id) REFERENCES node_groups(id) ON DELETE SET NULL,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE
);

-- 创建隧道规则关联表
CREATE TABLE IF NOT EXISTS tunnel_rules (
    tunnel_id VARCHAR(36) NOT NULL,
    rule_id VARCHAR(36) NOT NULL,
    PRIMARY KEY (tunnel_id, rule_id),
    FOREIGN KEY (tunnel_id) REFERENCES tunnels(id) ON DELETE CASCADE,
    FOREIGN KEY (rule_id) REFERENCES rules(id) ON DELETE CASCADE
);

-- 创建隧道探测结果表
CREATE TABLE IF NOT EXISTS tunnel_probes (
    id VARCHAR(36) PRIMARY KEY,
    tunnel_id VARCHAR(36) NOT NULL,
    success BOOLEAN NOT NULL,
    message TEXT,
    response_time INT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    details TEXT,
    
    FOREIGN KEY (tunnel_id) REFERENCES tunnels(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_tunnels_enabled ON tunnels(enabled);
CREATE INDEX IF NOT EXISTS idx_tunnels_ingress_node ON tunnels(ingress_node_id);
CREATE INDEX IF NOT EXISTS idx_tunnels_egress_node ON tunnels(egress_node_id);
CREATE INDEX IF NOT EXISTS idx_tunnels_ingress_group ON tunnels(ingress_group_id);
CREATE INDEX IF NOT EXISTS idx_tunnels_egress_group ON tunnels(egress_group_id);
CREATE INDEX IF NOT EXISTS idx_tunnel_rules_tunnel_id ON tunnel_rules(tunnel_id);
CREATE INDEX IF NOT EXISTS idx_tunnel_rules_rule_id ON tunnel_rules(rule_id);
CREATE INDEX IF NOT EXISTS idx_tunnel_probes_tunnel_id ON tunnel_probes(tunnel_id);
