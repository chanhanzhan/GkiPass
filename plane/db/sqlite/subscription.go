package sqlite

import (
	"database/sql"
	"fmt"

	dbinit "gkipass/plane/db/init"
)

// CreateSubscription 创建订阅
func (s *SQLiteDB) CreateSubscription(sub *dbinit.Subscription) error {
	query := `
		INSERT INTO user_subscriptions 
		(id, user_id, plan_id, status, start_at, expires_at, traffic, used_traffic, 
		 max_tunnels, max_bandwidth, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		sub.ID, sub.UserID, sub.PlanID, sub.Status, sub.StartAt, sub.ExpiresAt,
		sub.Traffic, sub.UsedTraffic, sub.MaxTunnels, sub.MaxBandwidth,
		sub.CreatedAt, sub.UpdatedAt)
	return err
}

// GetUserSubscription 获取用户订阅
func (s *SQLiteDB) GetUserSubscription(userID string) (*dbinit.Subscription, error) {
	sub := &dbinit.Subscription{}
	query := `
		SELECT id, user_id, plan_id, status, start_at, expires_at, 
		       traffic, used_traffic, max_tunnels, max_bandwidth, created_at, updated_at
		FROM user_subscriptions 
		WHERE user_id = ? AND status = 'active'
		ORDER BY created_at DESC 
		LIMIT 1
	`
	err := s.db.QueryRow(query, userID).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.Status, &sub.StartAt, &sub.ExpiresAt,
		&sub.Traffic, &sub.UsedTraffic, &sub.MaxTunnels, &sub.MaxBandwidth,
		&sub.CreatedAt, &sub.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

// UpdateSubscription 更新订阅
func (s *SQLiteDB) UpdateSubscription(sub interface{}) error {
	query := `
		UPDATE user_subscriptions 
		SET status = ?, end_date = ?, used_traffic = ?, updated_at = ?
		WHERE id = ?
	`

	switch v := sub.(type) {
	case *dbinit.Subscription:
		_, err := s.db.Exec(query, v.Status, v.ExpiresAt, v.UsedTraffic, v.UpdatedAt, v.ID)
		return err
	case *dbinit.UserSubscription:
		_, err := s.db.Exec(query, v.Status, v.EndDate, v.UsedTraffic, v.UpdatedAt, v.ID)
		return err
	default:
		return fmt.Errorf("unsupported subscription type")
	}
}

// GetActiveSubscriptionByUserID 获取用户的活跃订阅（返回UserSubscription类型）
func (s *SQLiteDB) GetActiveSubscriptionByUserID(userID string) (*dbinit.UserSubscription, error) {
	sub := &dbinit.UserSubscription{}
	query := `
		SELECT id, user_id, plan_id, status, start_date, end_date, 
		       used_rules, used_traffic, traffic_reset, created_at, updated_at
		FROM user_subscriptions 
		WHERE user_id = ? AND status = 'active'
		ORDER BY created_at DESC 
		LIMIT 1
	`
	err := s.db.QueryRow(query, userID).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.Status, &sub.StartDate, &sub.EndDate,
		&sub.UsedRules, &sub.UsedTraffic, &sub.TrafficReset,
		&sub.CreatedAt, &sub.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

// CreateSubscriptionFromUserSubscription 从UserSubscription创建订阅
func (s *SQLiteDB) CreateSubscriptionFromUserSubscription(sub *dbinit.UserSubscription) error {
	query := `
		INSERT INTO user_subscriptions 
		(id, user_id, plan_id, status, start_date, end_date, 
		 used_rules, used_traffic, traffic_reset, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		sub.ID, sub.UserID, sub.PlanID, sub.Status, sub.StartDate, sub.EndDate,
		sub.UsedRules, sub.UsedTraffic, sub.TrafficReset,
		sub.CreatedAt, sub.UpdatedAt)
	return err
}

// GetSubscriptionsByPlanID 根据套餐ID获取订阅列表
func (s *SQLiteDB) GetSubscriptionsByPlanID(planID string) ([]*dbinit.Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, start_at, expires_at, 
		       traffic, used_traffic, max_tunnels, max_bandwidth, created_at, updated_at
		FROM user_subscriptions 
		WHERE plan_id = ?
		ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subscriptions []*dbinit.Subscription
	for rows.Next() {
		sub := &dbinit.Subscription{}
		err := rows.Scan(
			&sub.ID, &sub.UserID, &sub.PlanID, &sub.Status, &sub.StartAt, &sub.ExpiresAt,
			&sub.Traffic, &sub.UsedTraffic, &sub.MaxTunnels, &sub.MaxBandwidth,
			&sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, sub)
	}

	return subscriptions, rows.Err()
}

// ListSubscriptions 列出所有订阅（管理员）
func (s *SQLiteDB) ListSubscriptions(limit, offset int) ([]*dbinit.Subscription, int, error) {
	// 获取总数
	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM user_subscriptions").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取数据
	query := `
		SELECT id, user_id, plan_id, status, start_at, expires_at, 
		       traffic, used_traffic, max_tunnels, max_bandwidth, created_at, updated_at
		FROM user_subscriptions 
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var subscriptions []*dbinit.Subscription
	for rows.Next() {
		sub := &dbinit.Subscription{}
		err := rows.Scan(
			&sub.ID, &sub.UserID, &sub.PlanID, &sub.Status, &sub.StartAt, &sub.ExpiresAt,
			&sub.Traffic, &sub.UsedTraffic, &sub.MaxTunnels, &sub.MaxBandwidth,
			&sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		subscriptions = append(subscriptions, sub)
	}

	return subscriptions, total, rows.Err()
}
