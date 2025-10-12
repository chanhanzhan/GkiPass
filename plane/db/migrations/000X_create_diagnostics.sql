-- 创建诊断表
CREATE TABLE IF NOT EXISTS diagnostics (
    id VARCHAR(36) PRIMARY KEY,
    source_id VARCHAR(36) NOT NULL,
    target_id VARCHAR(36) NOT NULL,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    message TEXT,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP,
    duration INTEGER,
    results TEXT,
    summary TEXT,
    
    FOREIGN KEY (source_id) REFERENCES nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_diagnostics_source_id ON diagnostics(source_id);
CREATE INDEX IF NOT EXISTS idx_diagnostics_target_id ON diagnostics(target_id);
CREATE INDEX IF NOT EXISTS idx_diagnostics_type ON diagnostics(type);
CREATE INDEX IF NOT EXISTS idx_diagnostics_status ON diagnostics(status);
CREATE INDEX IF NOT EXISTS idx_diagnostics_start_time ON diagnostics(start_time);
