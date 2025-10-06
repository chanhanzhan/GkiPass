-- 部署Token表
CREATE TABLE IF NOT EXISTS deployment_tokens (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    server_name TEXT,
    server_id TEXT,
    status TEXT NOT NULL DEFAULT 'unused' CHECK(status IN ('unused', 'active', 'inactive', 'revoked')),
    first_used_at DATETIME,
    last_seen_at DATETIME,
    server_info TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (group_id) REFERENCES node_groups(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_deployment_tokens_group_id ON deployment_tokens(group_id);
CREATE INDEX IF NOT EXISTS idx_deployment_tokens_token ON deployment_tokens(token);
CREATE INDEX IF NOT EXISTS idx_deployment_tokens_status ON deployment_tokens(status);
CREATE INDEX IF NOT EXISTS idx_deployment_tokens_server_id ON deployment_tokens(server_id);

