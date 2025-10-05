package sqlite

import (
	"database/sql"

	dbinit "gkipass/plane/db/init"
)

// CreateUser 创建用户
func (s *SQLiteDB) CreateUser(user *dbinit.User) error {
	query := `
		INSERT INTO users (id, username, password_hash, email, avatar, provider, provider_id, role, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, user.ID, user.Username, user.PasswordHash, user.Email,
		user.Avatar, user.Provider, user.ProviderID, user.Role, user.Enabled)
	return err
}

// GetUser 获取用户（通过ID）
func (s *SQLiteDB) GetUser(id string) (*dbinit.User, error) {
	user := &dbinit.User{}
	query := `
		SELECT id, username, password_hash, email, avatar, provider, provider_id, 
		       role, enabled, last_login_at, created_at, updated_at 
		FROM users WHERE id = ?
	`
	err := s.db.QueryRow(query, id).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Avatar,
		&user.Provider, &user.ProviderID, &user.Role, &user.Enabled,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

// GetUserByUsername 获取用户（通过用户名）
func (s *SQLiteDB) GetUserByUsername(username string) (*dbinit.User, error) {
	user := &dbinit.User{}

	// 使用明确的字段顺序，与数据库表结构完全一致
	query := `
		SELECT 
			id, 
			username, 
			password_hash, 
			email, 
			avatar, 
			provider, 
			provider_id, 
			role, 
			enabled, 
			last_login_at, 
			created_at, 
			updated_at 
		FROM users 
		WHERE username = ? AND enabled = 1
	`

	err := s.db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Email,
		&user.Avatar,
		&user.Provider,
		&user.ProviderID,
		&user.Role,
		&user.Enabled,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		// 返回详细错误以便调试
		return nil, err
	}

	return user, nil
}

// GetUserByProvider 获取用户（通过OAuth提供商）
func (s *SQLiteDB) GetUserByProvider(provider, providerID string) (*dbinit.User, error) {
	user := &dbinit.User{}
	query := `
		SELECT id, username, password_hash, email, avatar, provider, provider_id, 
		       role, enabled, last_login_at, created_at, updated_at 
		FROM users WHERE provider = ? AND provider_id = ?
	`
	err := s.db.QueryRow(query, provider, providerID).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Avatar,
		&user.Provider, &user.ProviderID, &user.Role, &user.Enabled,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

// GetUserCount 获取用户总数
func (s *SQLiteDB) GetUserCount() (int, error) {
	query := `SELECT COUNT(*) FROM users`
	var count int
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}

// GetUserByEmail 通过邮箱获取用户
func (s *SQLiteDB) GetUserByEmail(email string) (*dbinit.User, error) {
	user := &dbinit.User{}
	query := `
		SELECT id, username, password_hash, email, avatar, provider, provider_id, 
		       role, enabled, last_login_at, created_at, updated_at 
		FROM users WHERE email = ?
	`
	err := s.db.QueryRow(query, email).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Avatar,
		&user.Provider, &user.ProviderID, &user.Role, &user.Enabled,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

// UpdateUserPassword 更新用户密码
func (s *SQLiteDB) UpdateUserPassword(userID, passwordHash string) error {
	query := `UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, passwordHash, userID)
	return err
}

// UpdateUserStatus 更新用户状态
func (s *SQLiteDB) UpdateUserStatus(userID string, enabled bool) error {
	query := `UPDATE users SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, enabled, userID)
	return err
}

// ListUsers 列出用户
func (s *SQLiteDB) ListUsers(role string, enabled *bool) ([]*dbinit.User, error) {
	query := `
		SELECT id, username, password_hash, email, avatar, provider, provider_id, 
		       role, enabled, last_login_at, created_at, updated_at 
		FROM users WHERE 1=1
	`
	args := []interface{}{}

	if role != "" {
		query += ` AND role = ?`
		args = append(args, role)
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

	users := []*dbinit.User{}
	for rows.Next() {
		user := &dbinit.User{}
		err := rows.Scan(
			&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Avatar,
			&user.Provider, &user.ProviderID, &user.Role, &user.Enabled,
			&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

// UpdateUser 更新用户
func (s *SQLiteDB) UpdateUser(user *dbinit.User) error {
	query := `
		UPDATE users 
		SET username=?, email=?, avatar=?, role=?, enabled=?
		WHERE id=?
	`
	result, err := s.db.Exec(query, user.Username, user.Email, user.Avatar,
		user.Role, user.Enabled, user.ID)
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

// UpdateUserLastLogin 更新用户最后登录时间
func (s *SQLiteDB) UpdateUserLastLogin(id string) error {
	query := `UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// DeleteUser 删除用户
func (s *SQLiteDB) DeleteUser(id string) error {
	query := `DELETE FROM users WHERE id = ?`
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
