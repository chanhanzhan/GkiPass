-- 创建规则表
CREATE TABLE IF NOT EXISTS rules (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(36) NOT NULL,
    
    protocol VARCHAR(20) NOT NULL,
    listen_port INT NOT NULL,
    target_address VARCHAR(255) NOT NULL,
    target_port INT NOT NULL,
    ingress_node_id VARCHAR(36),
    egress_node_id VARCHAR(36),
    ingress_group_id VARCHAR(36),
    egress_group_id VARCHAR(36),
    ingress_protocol VARCHAR(20),
    egress_protocol VARCHAR(20),
    
    enable_encryption BOOLEAN NOT NULL DEFAULT FALSE,
    rate_limit_bps BIGINT NOT NULL DEFAULT 0,
    max_connections INT NOT NULL DEFAULT 0,
    idle_timeout INT NOT NULL DEFAULT 0,
    
    connection_count BIGINT NOT NULL DEFAULT 0,
    bytes_in BIGINT NOT NULL DEFAULT 0,
    bytes_out BIGINT NOT NULL DEFAULT 0,
    last_active TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    FOREIGN KEY (ingress_node_id) REFERENCES nodes(id) ON DELETE SET NULL,
    FOREIGN KEY (egress_node_id) REFERENCES nodes(id) ON DELETE SET NULL,
    FOREIGN KEY (ingress_group_id) REFERENCES node_groups(id) ON DELETE SET NULL,
    FOREIGN KEY (egress_group_id) REFERENCES node_groups(id) ON DELETE SET NULL,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE
);

-- 创建ACL规则表
CREATE TABLE IF NOT EXISTS rule_acls (
    id VARCHAR(36) PRIMARY KEY,
    rule_id VARCHAR(36) NOT NULL,
    action VARCHAR(10) NOT NULL, -- allow, deny
    priority INT NOT NULL DEFAULT 0,
    source_ip VARCHAR(50),
    dest_ip VARCHAR(50),
    protocol VARCHAR(20),
    port_range VARCHAR(20),
    
    FOREIGN KEY (rule_id) REFERENCES rules(id) ON DELETE CASCADE
);

-- 创建规则选项表
CREATE TABLE IF NOT EXISTS rule_options (
    rule_id VARCHAR(36) PRIMARY KEY,
    options TEXT NOT NULL,
    
    FOREIGN KEY (rule_id) REFERENCES rules(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_rules_enabled ON rules(enabled);
CREATE INDEX IF NOT EXISTS idx_rules_protocol ON rules(protocol);
CREATE INDEX IF NOT EXISTS idx_rules_listen_port ON rules(listen_port);
CREATE INDEX IF NOT EXISTS idx_rules_ingress_node ON rules(ingress_node_id);
CREATE INDEX IF NOT EXISTS idx_rules_egress_node ON rules(egress_node_id);
CREATE INDEX IF NOT EXISTS idx_rules_ingress_group ON rules(ingress_group_id);
CREATE INDEX IF NOT EXISTS idx_rules_egress_group ON rules(egress_group_id);
CREATE INDEX IF NOT EXISTS idx_rule_acls_rule_id ON rule_acls(rule_id);
