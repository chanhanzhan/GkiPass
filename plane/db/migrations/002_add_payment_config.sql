-- 支付配置表
CREATE TABLE IF NOT EXISTS payment_config (
    id TEXT PRIMARY KEY,
    payment_type TEXT NOT NULL CHECK(payment_type IN ('epay', 'crypto')),
    enabled INTEGER NOT NULL DEFAULT 0,
    config TEXT NOT NULL,  -- JSON配置
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 插入默认配置
INSERT OR IGNORE INTO payment_config (id, payment_type, enabled, config) VALUES
('epay_default', 'epay', 0, '{"api_url":"","merchant_id":"","merchant_key":"","notify_url":"","return_url":""}'),
('crypto_usdt', 'crypto', 0, '{"network":"TRC20","address":"","api_key":"","check_interval":30}');

-- 支付订单监听记录表
CREATE TABLE IF NOT EXISTS payment_monitors (
    id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL,
    payment_type TEXT NOT NULL,
    payment_address TEXT,
    expected_amount REAL NOT NULL,
    status TEXT NOT NULL DEFAULT 'monitoring' CHECK(status IN ('monitoring', 'confirmed', 'timeout', 'failed')),
    confirm_count INTEGER DEFAULT 0,
    last_check_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    FOREIGN KEY (transaction_id) REFERENCES wallet_transactions(id)
);

CREATE INDEX IF NOT EXISTS idx_payment_monitors_status ON payment_monitors(status);
CREATE INDEX IF NOT EXISTS idx_payment_monitors_expires ON payment_monitors(expires_at);


