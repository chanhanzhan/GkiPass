package sqlite

import (
	"database/sql"
	"time"
)

// DeploymentToken 部署Token
type DeploymentToken struct {
	ID          string    `json:"id" db:"id"`
	GroupID     string    `json:"group_id" db:"group_id"`
	Token       string    `json:"token" db:"token"`
	ServerName  string    `json:"server_name" db:"server_name"`
	ServerID    string    `json:"server_id" db:"server_id"`
	Status      string    `json:"status" db:"status"` // unused/active/inactive/revoked
	FirstUsedAt time.Time `json:"first_used_at" db:"first_used_at"`
	LastSeenAt  time.Time `json:"last_seen_at" db:"last_seen_at"`
	ServerInfo  string    `json:"server_info" db:"server_info"` // JSON格式的服务器信息
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// CreateDeploymentToken 创建部署Token
func (s *SQLiteDB) CreateDeploymentToken(token *DeploymentToken) error {
	query := `
		INSERT INTO deployment_tokens 
		(id, group_id, token, server_name, server_id, status, server_info, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		token.ID, token.GroupID, token.Token, token.ServerName, token.ServerID,
		token.Status, token.ServerInfo, token.CreatedAt, token.UpdatedAt)
	return err
}

// GetDeploymentToken 获取部署Token
func (s *SQLiteDB) GetDeploymentToken(id string) (*DeploymentToken, error) {
	token := &DeploymentToken{}
	query := `SELECT * FROM deployment_tokens WHERE id = ?`
	
	err := s.db.QueryRow(query, id).Scan(
		&token.ID, &token.GroupID, &token.Token, &token.ServerName, &token.ServerID,
		&token.Status, &token.FirstUsedAt, &token.LastSeenAt, &token.ServerInfo,
		&token.CreatedAt, &token.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return token, err
}

// GetDeploymentTokenByToken 通过Token字符串获取
func (s *SQLiteDB) GetDeploymentTokenByToken(token string) (*DeploymentToken, error) {
	t := &DeploymentToken{}
	query := `SELECT * FROM deployment_tokens WHERE token = ?`
	
	err := s.db.QueryRow(query, token).Scan(
		&t.ID, &t.GroupID, &t.Token, &t.ServerName, &t.ServerID,
		&t.Status, &t.FirstUsedAt, &t.LastSeenAt, &t.ServerInfo,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// ListDeploymentTokens 列出部署Tokens
func (s *SQLiteDB) ListDeploymentTokens(groupID string, status string, limit, offset int) ([]*DeploymentToken, int, error) {
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	
	if groupID != "" {
		whereClause += " AND group_id = ?"
		args = append(args, groupID)
	}
	if status != "" {
		whereClause += " AND status = ?"
		args = append(args, status)
	}

	// 获取总数
	var total int
	countQuery := "SELECT COUNT(*) FROM deployment_tokens " + whereClause
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取数据
	query := `
		SELECT * FROM deployment_tokens ` + whereClause + `
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tokens []*DeploymentToken
	for rows.Next() {
		token := &DeploymentToken{}
		err := rows.Scan(
			&token.ID, &token.GroupID, &token.Token, &token.ServerName, &token.ServerID,
			&token.Status, &token.FirstUsedAt, &token.LastSeenAt, &token.ServerInfo,
			&token.CreatedAt, &token.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		tokens = append(tokens, token)
	}

	return tokens, total, rows.Err()
}

// UpdateDeploymentToken 更新部署Token
func (s *SQLiteDB) UpdateDeploymentToken(token *DeploymentToken) error {
	query := `
		UPDATE deployment_tokens 
		SET server_name = ?, server_id = ?, status = ?, 
		    first_used_at = ?, last_seen_at = ?, server_info = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.Exec(query,
		token.ServerName, token.ServerID, token.Status,
		token.FirstUsedAt, token.LastSeenAt, token.ServerInfo,
		token.UpdatedAt, token.ID)
	return err
}

// DeleteDeploymentToken 删除部署Token
func (s *SQLiteDB) DeleteDeploymentToken(id string) error {
	query := `DELETE FROM deployment_tokens WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// CountDeploymentTokensByStatus 统计Token状态
func (s *SQLiteDB) CountDeploymentTokensByStatus(groupID string) (map[string]int, error) {
	query := `
		SELECT status, COUNT(*) as count
		FROM deployment_tokens
		WHERE group_id = ?
		GROUP BY status
	`
	
	rows, err := s.db.Query(query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}

	return counts, rows.Err()
}

// MarkTokenAsActive 标记Token为活跃状态
func (s *SQLiteDB) MarkTokenAsActive(tokenStr, serverID, serverInfo string) error {
	now := time.Now()
	query := `
		UPDATE deployment_tokens 
		SET status = 'active', 
		    server_id = ?, 
		    first_used_at = CASE WHEN first_used_at IS NULL THEN ? ELSE first_used_at END,
		    last_seen_at = ?,
		    server_info = ?,
		    updated_at = ?
		WHERE token = ? AND status IN ('unused', 'active')
	`
	result, err := s.db.Exec(query, serverID, now, now, serverInfo, now, tokenStr)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows // Token不存在或已被吊销
	}

	return nil
}

// UpdateTokenHeartbeat 更新Token心跳
func (s *SQLiteDB) UpdateTokenHeartbeat(tokenStr string, serverInfo string) error {
	now := time.Now()
	query := `
		UPDATE deployment_tokens 
		SET last_seen_at = ?,
		    server_info = ?,
		    updated_at = ?
		WHERE token = ? AND status = 'active'
	`
	_, err := s.db.Exec(query, now, serverInfo, now, tokenStr)
	return err
}

