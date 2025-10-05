package sqlite

import (
	"database/sql"

	dbinit "gkipass/plane/db/init"
)

// CreateTunnel 创建隧道
func (s *SQLiteDB) CreateTunnel(tunnel *dbinit.Tunnel) error {
	query := `
		INSERT INTO tunnels (id, user_id, name, protocol, entry_group_id, exit_group_id, 
			local_port, targets, enabled, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, tunnel.ID, tunnel.UserID, tunnel.Name, tunnel.Protocol,
		tunnel.EntryGroupID, tunnel.ExitGroupID, tunnel.LocalPort, tunnel.Targets,
		tunnel.Enabled, tunnel.Description)
	return err
}

// GetTunnel 获取隧道
func (s *SQLiteDB) GetTunnel(id string) (*dbinit.Tunnel, error) {
	tunnel := &dbinit.Tunnel{}
	query := `SELECT * FROM tunnels WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&tunnel.ID, &tunnel.UserID, &tunnel.Name, &tunnel.Protocol, &tunnel.EntryGroupID,
		&tunnel.ExitGroupID, &tunnel.LocalPort, &tunnel.Targets,
		&tunnel.Enabled, &tunnel.TrafficIn, &tunnel.TrafficOut, &tunnel.ConnectionCount,
		&tunnel.CreatedAt, &tunnel.UpdatedAt, &tunnel.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return tunnel, err
}

// ListTunnels 列出隧道
func (s *SQLiteDB) ListTunnels(userID string, enabled *bool) ([]*dbinit.Tunnel, error) {
	query := `SELECT * FROM tunnels WHERE 1=1`
	args := []interface{}{}

	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}

	if enabled != nil {
		query += ` AND enabled = ?`
		args = append(args, *enabled)
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tunnels := []*dbinit.Tunnel{}
	for rows.Next() {
		tunnel := &dbinit.Tunnel{}
		err := rows.Scan(
			&tunnel.ID, &tunnel.UserID, &tunnel.Name, &tunnel.Protocol, &tunnel.EntryGroupID,
			&tunnel.ExitGroupID, &tunnel.LocalPort, &tunnel.Targets,
			&tunnel.Enabled, &tunnel.TrafficIn, &tunnel.TrafficOut, &tunnel.ConnectionCount,
			&tunnel.CreatedAt, &tunnel.UpdatedAt, &tunnel.Description,
		)
		if err != nil {
			return nil, err
		}
		tunnels = append(tunnels, tunnel)
	}

	return tunnels, rows.Err()
}

// UpdateTunnel 更新隧道
func (s *SQLiteDB) UpdateTunnel(tunnel *dbinit.Tunnel) error {
	query := `
		UPDATE tunnels 
		SET name=?, protocol=?, entry_group_id=?, exit_group_id=?, local_port=?, 
			targets=?, enabled=?, traffic_in=?, traffic_out=?, 
			connection_count=?, description=?
		WHERE id=?
	`
	result, err := s.db.Exec(query, tunnel.Name, tunnel.Protocol, tunnel.EntryGroupID,
		tunnel.ExitGroupID, tunnel.LocalPort, tunnel.Targets,
		tunnel.Enabled, tunnel.TrafficIn, tunnel.TrafficOut, tunnel.ConnectionCount,
		tunnel.Description, tunnel.ID)
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

// DeleteTunnel 删除隧道
func (s *SQLiteDB) DeleteTunnel(id string) error {
	query := `DELETE FROM tunnels WHERE id = ?`
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

// GetTunnelsByGroupID 根据节点组ID获取隧道
func (s *SQLiteDB) GetTunnelsByGroupID(groupID string, groupType string) ([]*dbinit.Tunnel, error) {
	var query string
	if groupType == "entry" {
		query = `SELECT * FROM tunnels WHERE entry_group_id = ? AND enabled = 1`
	} else {
		query = `SELECT * FROM tunnels WHERE exit_group_id = ? AND enabled = 1`
	}

	rows, err := s.db.Query(query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tunnels := []*dbinit.Tunnel{}
	for rows.Next() {
		tunnel := &dbinit.Tunnel{}
		err := rows.Scan(
			&tunnel.ID, &tunnel.UserID, &tunnel.Name, &tunnel.Protocol, &tunnel.EntryGroupID,
			&tunnel.ExitGroupID, &tunnel.LocalPort, &tunnel.Targets,
			&tunnel.Enabled, &tunnel.TrafficIn, &tunnel.TrafficOut, &tunnel.ConnectionCount,
			&tunnel.CreatedAt, &tunnel.UpdatedAt, &tunnel.Description,
		)
		if err != nil {
			return nil, err
		}
		tunnels = append(tunnels, tunnel)
	}

	return tunnels, rows.Err()
}
