package sqlite

import (
	dbinit "gkipass/plane/db/init"
)

// ListAllActiveSubscriptions 列出所有活跃订阅
func (s *SQLiteDB) ListAllActiveSubscriptions() ([]*dbinit.UserSubscription, error) {
	query := `
		SELECT * FROM user_subscriptions 
		WHERE status = 'active' 
		ORDER BY end_date ASC
	`
	
	rows, err := s.db.Query(query)
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

// ListExpiredSubscriptions 列出已过期但状态仍为active的订阅
func (s *SQLiteDB) ListExpiredSubscriptions() ([]*dbinit.UserSubscription, error) {
	query := `
		SELECT * FROM user_subscriptions 
		WHERE status = 'active' AND end_date < datetime('now')
		ORDER BY end_date ASC
	`
	
	rows, err := s.db.Query(query)
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

