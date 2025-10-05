package sqlite

import (
	"database/sql"

	dbinit "gkipass/plane/db/init"
)

// CreateSubscription 创建订阅
func (s *SQLiteDB) CreateSubscription(sub *dbinit.UserSubscription) error {
	query := `
		INSERT INTO user_subscriptions (id, user_id, plan_id, start_date, end_date, status, 
			used_rules, used_traffic, traffic_reset)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, sub.ID, sub.UserID, sub.PlanID, sub.StartDate, sub.EndDate,
		sub.Status, sub.UsedRules, sub.UsedTraffic, sub.TrafficReset)
	return err
}

// GetSubscription 获取订阅
func (s *SQLiteDB) GetSubscription(id string) (*dbinit.UserSubscription, error) {
	sub := &dbinit.UserSubscription{}
	query := `SELECT * FROM user_subscriptions WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.StartDate, &sub.EndDate, &sub.Status,
		&sub.UsedRules, &sub.UsedTraffic, &sub.TrafficReset, &sub.CreatedAt, &sub.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

// GetActiveSubscriptionByUserID 获取用户的有效订阅
func (s *SQLiteDB) GetActiveSubscriptionByUserID(userID string) (*dbinit.UserSubscription, error) {
	sub := &dbinit.UserSubscription{}
	query := `
		SELECT * FROM user_subscriptions 
		WHERE user_id = ? AND status = 'active' AND end_date > datetime('now')
		ORDER BY end_date DESC LIMIT 1
	`
	err := s.db.QueryRow(query, userID).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.StartDate, &sub.EndDate, &sub.Status,
		&sub.UsedRules, &sub.UsedTraffic, &sub.TrafficReset, &sub.CreatedAt, &sub.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

// ListSubscriptionsByUserID 列出用户的所有订阅
func (s *SQLiteDB) ListSubscriptionsByUserID(userID string) ([]*dbinit.UserSubscription, error) {
	query := `SELECT * FROM user_subscriptions WHERE user_id = ? ORDER BY created_at DESC`
	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subs := []*dbinit.UserSubscription{}
	for rows.Next() {
		sub := &dbinit.UserSubscription{}
		err := rows.Scan(
			&sub.ID, &sub.UserID, &sub.PlanID, &sub.StartDate, &sub.EndDate, &sub.Status,
			&sub.UsedRules, &sub.UsedTraffic, &sub.TrafficReset, &sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}

	return subs, rows.Err()
}

// GetSubscriptionsByPlanID 获取套餐的所有订阅
func (s *SQLiteDB) GetSubscriptionsByPlanID(planID string) ([]*dbinit.UserSubscription, error) {
	query := `SELECT * FROM user_subscriptions WHERE plan_id = ? AND status = 'active'`
	rows, err := s.db.Query(query, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subs := []*dbinit.UserSubscription{}
	for rows.Next() {
		sub := &dbinit.UserSubscription{}
		err := rows.Scan(
			&sub.ID, &sub.UserID, &sub.PlanID, &sub.StartDate, &sub.EndDate, &sub.Status,
			&sub.UsedRules, &sub.UsedTraffic, &sub.TrafficReset, &sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}

	return subs, rows.Err()
}

// UpdateSubscription 更新订阅
func (s *SQLiteDB) UpdateSubscription(sub *dbinit.UserSubscription) error {
	query := `
		UPDATE user_subscriptions 
		SET status=?, used_rules=?, used_traffic=?, traffic_reset=?
		WHERE id=?
	`
	result, err := s.db.Exec(query, sub.Status, sub.UsedRules, sub.UsedTraffic,
		sub.TrafficReset, sub.ID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// CancelSubscription 取消订阅
func (s *SQLiteDB) CancelSubscription(id string) error {
	query := `UPDATE user_subscriptions SET status = 'cancelled' WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

