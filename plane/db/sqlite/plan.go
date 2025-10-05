package sqlite

import (
	"database/sql"

	dbinit "gkipass/plane/db/init"
)

// CreatePlan 创建套餐
func (s *SQLiteDB) CreatePlan(plan *dbinit.Plan) error {
	query := `
		INSERT INTO plans (id, name, max_rules, max_traffic, max_bandwidth, max_connections, 
			max_connect_ips, allowed_node_ids, billing_cycle, price, enabled, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, plan.ID, plan.Name, plan.MaxRules, plan.MaxTraffic,
		plan.MaxBandwidth, plan.MaxConnections, plan.MaxConnectIPs, plan.AllowedNodeIDs,
		plan.BillingCycle, plan.Price, plan.Enabled, plan.Description)
	return err
}

// GetPlan 获取套餐
func (s *SQLiteDB) GetPlan(id string) (*dbinit.Plan, error) {
	plan := &dbinit.Plan{}
	query := `SELECT * FROM plans WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&plan.ID, &plan.Name, &plan.MaxRules, &plan.MaxTraffic, &plan.MaxBandwidth,
		&plan.MaxConnections, &plan.MaxConnectIPs, &plan.AllowedNodeIDs, &plan.BillingCycle,
		&plan.Price, &plan.Enabled, &plan.CreatedAt, &plan.UpdatedAt, &plan.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return plan, err
}

// ListPlans 列出套餐
func (s *SQLiteDB) ListPlans(enabled *bool) ([]*dbinit.Plan, error) {
	query := `SELECT * FROM plans WHERE 1=1`
	args := []interface{}{}

	if enabled != nil {
		query += ` AND enabled = ?`
		args = append(args, *enabled)
	}

	query += ` ORDER BY price ASC, created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := []*dbinit.Plan{}
	for rows.Next() {
		plan := &dbinit.Plan{}
		err := rows.Scan(
			&plan.ID, &plan.Name, &plan.MaxRules, &plan.MaxTraffic, &plan.MaxBandwidth,
			&plan.MaxConnections, &plan.MaxConnectIPs, &plan.AllowedNodeIDs, &plan.BillingCycle,
			&plan.Price, &plan.Enabled, &plan.CreatedAt, &plan.UpdatedAt, &plan.Description,
		)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}

	return plans, rows.Err()
}

// UpdatePlan 更新套餐
func (s *SQLiteDB) UpdatePlan(plan *dbinit.Plan) error {
	query := `
		UPDATE plans 
		SET name=?, max_rules=?, max_traffic=?, max_bandwidth=?, max_connections=?, 
			max_connect_ips=?, allowed_node_ids=?, billing_cycle=?, price=?, enabled=?, description=?
		WHERE id=?
	`
	result, err := s.db.Exec(query, plan.Name, plan.MaxRules, plan.MaxTraffic, plan.MaxBandwidth,
		plan.MaxConnections, plan.MaxConnectIPs, plan.AllowedNodeIDs, plan.BillingCycle,
		plan.Price, plan.Enabled, plan.Description, plan.ID)
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

// DeletePlan 删除套餐
func (s *SQLiteDB) DeletePlan(id string) error {
	query := `DELETE FROM plans WHERE id = ?`
	result, err := s.db.Exec(query, id)
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

