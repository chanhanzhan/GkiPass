-- 创建节点组表
CREATE TABLE IF NOT EXISTS node_groups (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    role VARCHAR(20) NOT NULL, -- ingress, egress, both
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    requires_egress BOOLEAN NOT NULL DEFAULT FALSE,
    default_egress_id VARCHAR(36),
    disabled_protocols TEXT, -- 逗号分隔的协议列表
    allowed_port_ranges TEXT, -- 逗号分隔的端口范围
    allow_probe_view BOOLEAN NOT NULL DEFAULT TRUE
);

-- 创建节点组与节点的关联表
CREATE TABLE IF NOT EXISTS node_group_nodes (
    group_id VARCHAR(36) NOT NULL,
    node_id VARCHAR(36) NOT NULL,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id, node_id),
    FOREIGN KEY (group_id) REFERENCES node_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_node_groups_role ON node_groups(role);
CREATE INDEX IF NOT EXISTS idx_node_group_nodes_node_id ON node_group_nodes(node_id);
CREATE INDEX IF NOT EXISTS idx_node_group_nodes_group_id ON node_group_nodes(group_id);

-- 添加默认组
INSERT INTO node_groups (id, name, description, role, requires_egress, allow_probe_view)
VALUES 
    ('default-ingress', '默认入口组', '系统默认的入口节点组', 'ingress', true, true),
    ('default-egress', '默认出口组', '系统默认的出口节点组', 'egress', false, true),
    ('default-both', '默认双向组', '系统默认的双向节点组', 'both', false, true);
