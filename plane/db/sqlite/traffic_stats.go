package sqlite

import (
	"database/sql"
	"time"
)

// TunnelTrafficStat 隧道流量统计
type TunnelTrafficStat struct {
	ID               string    `json:"id" db:"id"`
	TunnelID         string    `json:"tunnel_id" db:"tunnel_id"`
	UserID           string    `json:"user_id" db:"user_id"`
	EntryGroupID     string    `json:"entry_group_id" db:"entry_group_id"`
	ExitGroupID      string    `json:"exit_group_id" db:"exit_group_id"`
	TrafficIn        int64     `json:"traffic_in" db:"traffic_in"`
	TrafficOut       int64     `json:"traffic_out" db:"traffic_out"`
	BilledTrafficIn  int64     `json:"billed_traffic_in" db:"billed_traffic_in"`
	BilledTrafficOut int64     `json:"billed_traffic_out" db:"billed_traffic_out"`
	Date             time.Time `json:"date" db:"date"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// CreateTunnelTrafficStat 创建隧道流量统计
func (s *SQLiteDB) CreateTunnelTrafficStat(stat *TunnelTrafficStat) error {
	query := `
		INSERT INTO tunnel_traffic_stats 
		(id, tunnel_id, user_id, entry_group_id, exit_group_id, traffic_in, traffic_out, 
		 billed_traffic_in, billed_traffic_out, date, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		stat.ID, stat.TunnelID, stat.UserID, stat.EntryGroupID, stat.ExitGroupID,
		stat.TrafficIn, stat.TrafficOut, stat.BilledTrafficIn, stat.BilledTrafficOut,
		stat.Date, stat.CreatedAt)
	return err
}

// GetTunnelTrafficStatsByDateRange 按日期范围查询流量统计
func (s *SQLiteDB) GetTunnelTrafficStatsByDateRange(userID string, startDate, endDate time.Time) ([]*TunnelTrafficStat, error) {
	query := `
		SELECT * FROM tunnel_traffic_stats
		WHERE user_id = ? AND date >= ? AND date <= ?
		ORDER BY date DESC
	`

	rows, err := s.db.Query(query, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []*TunnelTrafficStat
	for rows.Next() {
		stat := &TunnelTrafficStat{}
		err := rows.Scan(
			&stat.ID, &stat.TunnelID, &stat.UserID, &stat.EntryGroupID, &stat.ExitGroupID,
			&stat.TrafficIn, &stat.TrafficOut, &stat.BilledTrafficIn, &stat.BilledTrafficOut,
			&stat.Date, &stat.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// ListTunnelTrafficStats 列出隧道流量统计（支持筛选）
func (s *SQLiteDB) ListTunnelTrafficStats(userID, tunnelID string, limit, offset int) ([]*TunnelTrafficStat, int, error) {
	whereClause := "WHERE 1=1"
	args := []interface{}{}

	if userID != "" {
		whereClause += " AND user_id = ?"
		args = append(args, userID)
	}
	if tunnelID != "" {
		whereClause += " AND tunnel_id = ?"
		args = append(args, tunnelID)
	}

	// 获取总数
	var total int
	countQuery := "SELECT COUNT(*) FROM tunnel_traffic_stats " + whereClause
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取数据
	query := `
		SELECT * FROM tunnel_traffic_stats ` + whereClause + `
		ORDER BY date DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var stats []*TunnelTrafficStat
	for rows.Next() {
		stat := &TunnelTrafficStat{}
		err := rows.Scan(
			&stat.ID, &stat.TunnelID, &stat.UserID, &stat.EntryGroupID, &stat.ExitGroupID,
			&stat.TrafficIn, &stat.TrafficOut, &stat.BilledTrafficIn, &stat.BilledTrafficOut,
			&stat.Date, &stat.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		stats = append(stats, stat)
	}

	return stats, total, rows.Err()
}

// GetTunnelTrafficSummary 获取隧道流量汇总
func (s *SQLiteDB) GetTunnelTrafficSummary(userID, tunnelID string, startDate, endDate time.Time) (trafficIn, trafficOut, billedIn, billedOut int64, err error) {
	query := `
		SELECT 
			COALESCE(SUM(traffic_in), 0),
			COALESCE(SUM(traffic_out), 0),
			COALESCE(SUM(billed_traffic_in), 0),
			COALESCE(SUM(billed_traffic_out), 0)
		FROM tunnel_traffic_stats
		WHERE user_id = ? AND date >= ? AND date <= ?
	`

	args := []interface{}{userID, startDate, endDate}
	if tunnelID != "" {
		query += " AND tunnel_id = ?"
		args = append(args, tunnelID)
	}

	err = s.db.QueryRow(query, args...).Scan(&trafficIn, &trafficOut, &billedIn, &billedOut)
	if err == sql.ErrNoRows {
		return 0, 0, 0, 0, nil
	}

	return
}
