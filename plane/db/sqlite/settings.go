package sqlite

import (
	"database/sql"
	"time"

	dbinit "gkipass/plane/db/init"

	"github.com/google/uuid"
)

// CreateSystemSetting 创建系统设置
func (s *SQLiteDB) CreateSystemSetting(setting *dbinit.SystemSettings) error {
	if setting.ID == "" {
		setting.ID = uuid.New().String()
	}
	setting.UpdatedAt = time.Now()

	query := `
		INSERT INTO system_settings (id, key, value, category, updated_by, updated_at, description)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		setting.ID,
		setting.Key,
		setting.Value,
		setting.Category,
		setting.UpdatedBy,
		setting.UpdatedAt,
		setting.Description,
	)
	return err
}

// GetSystemSetting 获取系统设置
func (s *SQLiteDB) GetSystemSetting(key string) (*dbinit.SystemSettings, error) {
	setting := &dbinit.SystemSettings{}
	query := `
		SELECT id, key, value, category, updated_by, updated_at, description
		FROM system_settings WHERE key = ?
	`
	err := s.db.QueryRow(query, key).Scan(
		&setting.ID,
		&setting.Key,
		&setting.Value,
		&setting.Category,
		&setting.UpdatedBy,
		&setting.UpdatedAt,
		&setting.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return setting, err
}

// GetSystemSettingByID 根据ID获取系统设置
func (s *SQLiteDB) GetSystemSettingByID(id string) (*dbinit.SystemSettings, error) {
	setting := &dbinit.SystemSettings{}
	query := `
		SELECT id, key, value, category, updated_by, updated_at, description
		FROM system_settings WHERE id = ?
	`
	err := s.db.QueryRow(query, id).Scan(
		&setting.ID,
		&setting.Key,
		&setting.Value,
		&setting.Category,
		&setting.UpdatedBy,
		&setting.UpdatedAt,
		&setting.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return setting, err
}

// UpdateSystemSetting 更新系统设置
func (s *SQLiteDB) UpdateSystemSetting(setting *dbinit.SystemSettings) error {
	setting.UpdatedAt = time.Now()
	query := `
		UPDATE system_settings 
		SET value = ?, category = ?, updated_by = ?, updated_at = ?, description = ?
		WHERE key = ?
	`
	_, err := s.db.Exec(query,
		setting.Value,
		setting.Category,
		setting.UpdatedBy,
		setting.UpdatedAt,
		setting.Description,
		setting.Key,
	)
	return err
}

// UpsertSystemSetting 创建或更新系统设置
func (s *SQLiteDB) UpsertSystemSetting(setting *dbinit.SystemSettings) error {
	existing, err := s.GetSystemSetting(setting.Key)
	if err != nil {
		return err
	}

	if existing == nil {
		return s.CreateSystemSetting(setting)
	}

	setting.ID = existing.ID
	return s.UpdateSystemSetting(setting)
}

// DeleteSystemSetting 删除系统设置
func (s *SQLiteDB) DeleteSystemSetting(key string) error {
	query := `DELETE FROM system_settings WHERE key = ?`
	_, err := s.db.Exec(query, key)
	return err
}

// ListSystemSettings 获取系统设置列表
func (s *SQLiteDB) ListSystemSettings(category string) ([]*dbinit.SystemSettings, error) {
	settings := []*dbinit.SystemSettings{}
	var query string
	var rows *sql.Rows
	var err error

	if category != "" {
		query = `
			SELECT id, key, value, category, updated_by, updated_at, description
			FROM system_settings 
			WHERE category = ?
			ORDER BY key ASC
		`
		rows, err = s.db.Query(query, category)
	} else {
		query = `
			SELECT id, key, value, category, updated_by, updated_at, description
			FROM system_settings 
			ORDER BY category ASC, key ASC
		`
		rows, err = s.db.Query(query)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		setting := &dbinit.SystemSettings{}
		err := rows.Scan(
			&setting.ID,
			&setting.Key,
			&setting.Value,
			&setting.Category,
			&setting.UpdatedBy,
			&setting.UpdatedAt,
			&setting.Description,
		)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	return settings, rows.Err()
}

// GetSettingsByCategory 根据分类获取设置
func (s *SQLiteDB) GetSettingsByCategory(category string) ([]*dbinit.SystemSettings, error) {
	return s.ListSystemSettings(category)
}
