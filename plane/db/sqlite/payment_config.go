package sqlite

import (
	"database/sql"
	"time"
)

// PaymentConfig 支付配置
type PaymentConfig struct {
	ID          string    `json:"id" db:"id"`
	PaymentType string    `json:"payment_type" db:"payment_type"` // epay/crypto
	Enabled     bool      `json:"enabled" db:"enabled"`
	Config      string    `json:"config" db:"config"` // JSON配置
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// PaymentMonitor 支付监听记录
type PaymentMonitor struct {
	ID             string     `json:"id" db:"id"`
	TransactionID  string     `json:"transaction_id" db:"transaction_id"`
	PaymentType    string     `json:"payment_type" db:"payment_type"`
	PaymentAddress string     `json:"payment_address" db:"payment_address"`
	ExpectedAmount float64    `json:"expected_amount" db:"expected_amount"`
	Status         string     `json:"status" db:"status"` // monitoring/confirmed/timeout/failed
	ConfirmCount   int        `json:"confirm_count" db:"confirm_count"`
	LastCheckAt    *time.Time `json:"last_check_at" db:"last_check_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt      time.Time  `json:"expires_at" db:"expires_at"`
}

// GetPaymentConfig 获取支付配置
func (s *SQLiteDB) GetPaymentConfig(id string) (*PaymentConfig, error) {
	config := &PaymentConfig{}
	query := `SELECT id, payment_type, enabled, config, created_at, updated_at 
		FROM payment_config WHERE id = ?`

	var enabled int
	err := s.db.QueryRow(query, id).Scan(
		&config.ID, &config.PaymentType, &enabled, &config.Config,
		&config.CreatedAt, &config.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	config.Enabled = enabled == 1
	return config, err
}

// ListPaymentConfigs 列出所有支付配置
func (s *SQLiteDB) ListPaymentConfigs() ([]*PaymentConfig, error) {
	query := `SELECT id, payment_type, enabled, config, created_at, updated_at 
		FROM payment_config ORDER BY payment_type`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*PaymentConfig
	for rows.Next() {
		config := &PaymentConfig{}
		var enabled int
		if err := rows.Scan(
			&config.ID, &config.PaymentType, &enabled, &config.Config,
			&config.CreatedAt, &config.UpdatedAt,
		); err != nil {
			return nil, err
		}
		config.Enabled = enabled == 1
		configs = append(configs, config)
	}
	return configs, rows.Err()
}

// UpdatePaymentConfig 更新支付配置
func (s *SQLiteDB) UpdatePaymentConfig(id string, enabled bool, config string) error {
	query := `UPDATE payment_config SET enabled = ?, config = ?, updated_at = ? WHERE id = ?`
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := s.db.Exec(query, enabledInt, config, time.Now(), id)
	return err
}

// CreatePaymentMonitor 创建支付监听记录
func (s *SQLiteDB) CreatePaymentMonitor(monitor *PaymentMonitor) error {
	query := `INSERT INTO payment_monitors 
		(id, transaction_id, payment_type, payment_address, expected_amount, status, confirm_count, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query,
		monitor.ID, monitor.TransactionID, monitor.PaymentType, monitor.PaymentAddress,
		monitor.ExpectedAmount, monitor.Status, monitor.ConfirmCount,
		monitor.CreatedAt, monitor.ExpiresAt,
	)
	return err
}

// GetPaymentMonitor 获取支付监听记录
func (s *SQLiteDB) GetPaymentMonitor(transactionID string) (*PaymentMonitor, error) {
	monitor := &PaymentMonitor{}
	query := `SELECT id, transaction_id, payment_type, payment_address, expected_amount, 
		status, confirm_count, last_check_at, created_at, expires_at
		FROM payment_monitors WHERE transaction_id = ?`

	var lastCheckAt sql.NullTime
	err := s.db.QueryRow(query, transactionID).Scan(
		&monitor.ID, &monitor.TransactionID, &monitor.PaymentType, &monitor.PaymentAddress,
		&monitor.ExpectedAmount, &monitor.Status, &monitor.ConfirmCount,
		&lastCheckAt, &monitor.CreatedAt, &monitor.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if lastCheckAt.Valid {
		monitor.LastCheckAt = &lastCheckAt.Time
	}
	return monitor, err
}

// UpdatePaymentMonitor 更新支付监听状态
func (s *SQLiteDB) UpdatePaymentMonitor(id string, status string, confirmCount int) error {
	query := `UPDATE payment_monitors 
		SET status = ?, confirm_count = ?, last_check_at = ? 
		WHERE id = ?`
	_, err := s.db.Exec(query, status, confirmCount, time.Now(), id)
	return err
}

// ListPendingMonitors 列出待监听的订单
func (s *SQLiteDB) ListPendingMonitors() ([]*PaymentMonitor, error) {
	query := `SELECT id, transaction_id, payment_type, payment_address, expected_amount, 
		status, confirm_count, last_check_at, created_at, expires_at
		FROM payment_monitors 
		WHERE status = 'monitoring' AND expires_at > datetime('now')
		ORDER BY created_at ASC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []*PaymentMonitor
	for rows.Next() {
		monitor := &PaymentMonitor{}
		var lastCheckAt sql.NullTime
		if err := rows.Scan(
			&monitor.ID, &monitor.TransactionID, &monitor.PaymentType, &monitor.PaymentAddress,
			&monitor.ExpectedAmount, &monitor.Status, &monitor.ConfirmCount,
			&lastCheckAt, &monitor.CreatedAt, &monitor.ExpiresAt,
		); err != nil {
			return nil, err
		}
		if lastCheckAt.Valid {
			monitor.LastCheckAt = &lastCheckAt.Time
		}
		monitors = append(monitors, monitor)
	}
	return monitors, rows.Err()
}
