package sqlite

import (
	"database/sql"
	"time"
)

// PaymentOrder 支付订单
type PaymentOrder struct {
	ID            string    `json:"id" db:"id"`
	OrderNo       string    `json:"order_no" db:"order_no"`
	UserID        string    `json:"user_id" db:"user_id"`
	Type          string    `json:"type" db:"type"`
	Amount        float64   `json:"amount" db:"amount"`
	PaymentMethod string    `json:"payment_method" db:"payment_method"`
	PaymentStatus string    `json:"payment_status" db:"payment_status"`
	PaymentData   string    `json:"payment_data" db:"payment_data"`
	RelatedID     string    `json:"related_id" db:"related_id"`
	RelatedType   string    `json:"related_type" db:"related_type"`
	CallbackData  string    `json:"callback_data" db:"callback_data"`
	CompletedAt   time.Time `json:"completed_at" db:"completed_at"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// CreatePaymentOrder 创建支付订单
func (s *SQLiteDB) CreatePaymentOrder(order *PaymentOrder) error {
	query := `
		INSERT INTO payment_orders 
		(id, order_no, user_id, type, amount, payment_method, payment_status, 
		 payment_data, related_id, related_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		order.ID, order.OrderNo, order.UserID, order.Type, order.Amount,
		order.PaymentMethod, order.PaymentStatus, order.PaymentData,
		order.RelatedID, order.RelatedType, order.CreatedAt, order.UpdatedAt)
	return err
}

// GetPaymentOrder 获取支付订单
func (s *SQLiteDB) GetPaymentOrder(id string) (*PaymentOrder, error) {
	order := &PaymentOrder{}
	query := `SELECT * FROM payment_orders WHERE id = ?`

	err := s.db.QueryRow(query, id).Scan(
		&order.ID, &order.OrderNo, &order.UserID, &order.Type, &order.Amount,
		&order.PaymentMethod, &order.PaymentStatus, &order.PaymentData,
		&order.RelatedID, &order.RelatedType, &order.CallbackData,
		&order.CompletedAt, &order.CreatedAt, &order.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return order, err
}

// GetPaymentOrderByOrderNo 通过订单号获取订单
func (s *SQLiteDB) GetPaymentOrderByOrderNo(orderNo string) (*PaymentOrder, error) {
	order := &PaymentOrder{}
	query := `SELECT * FROM payment_orders WHERE order_no = ?`

	err := s.db.QueryRow(query, orderNo).Scan(
		&order.ID, &order.OrderNo, &order.UserID, &order.Type, &order.Amount,
		&order.PaymentMethod, &order.PaymentStatus, &order.PaymentData,
		&order.RelatedID, &order.RelatedType, &order.CallbackData,
		&order.CompletedAt, &order.CreatedAt, &order.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return order, err
}

// UpdatePaymentOrder 更新支付订单
func (s *SQLiteDB) UpdatePaymentOrder(order *PaymentOrder) error {
	query := `
		UPDATE payment_orders 
		SET payment_method = ?, payment_status = ?, payment_data = ?,
		    callback_data = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.Exec(query,
		order.PaymentMethod, order.PaymentStatus, order.PaymentData,
		order.CallbackData, order.CompletedAt, order.UpdatedAt, order.ID)
	return err
}

// ListPaymentOrders 列出支付订单
func (s *SQLiteDB) ListPaymentOrders(userID string, status string, limit, offset int) ([]*PaymentOrder, int, error) {
	whereClause := "WHERE 1=1"
	args := []interface{}{}

	if userID != "" {
		whereClause += " AND user_id = ?"
		args = append(args, userID)
	}
	if status != "" {
		whereClause += " AND payment_status = ?"
		args = append(args, status)
	}

	// 获取总数
	var total int
	countQuery := "SELECT COUNT(*) FROM payment_orders " + whereClause
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取数据
	query := `
		SELECT * FROM payment_orders ` + whereClause + `
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []*PaymentOrder
	for rows.Next() {
		order := &PaymentOrder{}
		err := rows.Scan(
			&order.ID, &order.OrderNo, &order.UserID, &order.Type, &order.Amount,
			&order.PaymentMethod, &order.PaymentStatus, &order.PaymentData,
			&order.RelatedID, &order.RelatedType, &order.CallbackData,
			&order.CompletedAt, &order.CreatedAt, &order.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		orders = append(orders, order)
	}

	return orders, total, rows.Err()
}
