package sqlite

import (
	"database/sql"
	"time"

	dbinit "gkipass/plane/db/init"

	"github.com/google/uuid"
)

// CreateAnnouncement 创建公告
func (s *SQLiteDB) CreateAnnouncement(announcement *dbinit.Announcement) error {
	if announcement.ID == "" {
		announcement.ID = uuid.New().String()
	}
	if announcement.CreatedAt.IsZero() {
		announcement.CreatedAt = time.Now()
	}
	announcement.UpdatedAt = time.Now()

	query := `
		INSERT INTO announcements 
		(id, title, content, type, priority, enabled, start_time, end_time, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		announcement.ID,
		announcement.Title,
		announcement.Content,
		announcement.Type,
		announcement.Priority,
		announcement.Enabled,
		announcement.StartTime,
		announcement.EndTime,
		announcement.CreatedBy,
		announcement.CreatedAt,
		announcement.UpdatedAt,
	)
	return err
}

// GetAnnouncement 获取公告
func (s *SQLiteDB) GetAnnouncement(id string) (*dbinit.Announcement, error) {
	announcement := &dbinit.Announcement{}
	query := `
		SELECT id, title, content, type, priority, enabled, start_time, end_time, created_by, created_at, updated_at
		FROM announcements WHERE id = ?
	`
	err := s.db.QueryRow(query, id).Scan(
		&announcement.ID,
		&announcement.Title,
		&announcement.Content,
		&announcement.Type,
		&announcement.Priority,
		&announcement.Enabled,
		&announcement.StartTime,
		&announcement.EndTime,
		&announcement.CreatedBy,
		&announcement.CreatedAt,
		&announcement.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return announcement, err
}

// UpdateAnnouncement 更新公告
func (s *SQLiteDB) UpdateAnnouncement(announcement *dbinit.Announcement) error {
	announcement.UpdatedAt = time.Now()
	query := `
		UPDATE announcements 
		SET title = ?, content = ?, type = ?, priority = ?, enabled = ?, start_time = ?, end_time = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.Exec(query,
		announcement.Title,
		announcement.Content,
		announcement.Type,
		announcement.Priority,
		announcement.Enabled,
		announcement.StartTime,
		announcement.EndTime,
		announcement.UpdatedAt,
		announcement.ID,
	)
	return err
}

// DeleteAnnouncement 删除公告
func (s *SQLiteDB) DeleteAnnouncement(id string) error {
	query := `DELETE FROM announcements WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// ListAnnouncements 获取公告列表（管理员）
func (s *SQLiteDB) ListAnnouncements(page, limit int) ([]*dbinit.Announcement, int, error) {
	offset := (page - 1) * limit

	// 获取总数
	var total int
	countQuery := `SELECT COUNT(*) FROM announcements`
	err := s.db.QueryRow(countQuery).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取列表
	announcements := []*dbinit.Announcement{}
	query := `
		SELECT id, title, content, type, priority, enabled, start_time, end_time, created_by, created_at, updated_at
		FROM announcements 
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		a := &dbinit.Announcement{}
		err := rows.Scan(
			&a.ID,
			&a.Title,
			&a.Content,
			&a.Type,
			&a.Priority,
			&a.Enabled,
			&a.StartTime,
			&a.EndTime,
			&a.CreatedBy,
			&a.CreatedAt,
			&a.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		announcements = append(announcements, a)
	}

	return announcements, total, rows.Err()
}

// ListActiveAnnouncements 获取有效的公告列表（用户）
func (s *SQLiteDB) ListActiveAnnouncements() ([]*dbinit.Announcement, error) {
	now := time.Now()
	announcements := []*dbinit.Announcement{}
	query := `
		SELECT id, title, content, type, priority, enabled, start_time, end_time, created_by, created_at, updated_at
		FROM announcements 
		WHERE enabled = 1 AND start_time <= ? AND end_time >= ?
		ORDER BY priority DESC, created_at DESC
	`
	rows, err := s.db.Query(query, now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		a := &dbinit.Announcement{}
		err := rows.Scan(
			&a.ID,
			&a.Title,
			&a.Content,
			&a.Type,
			&a.Priority,
			&a.Enabled,
			&a.StartTime,
			&a.EndTime,
			&a.CreatedBy,
			&a.CreatedAt,
			&a.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		announcements = append(announcements, a)
	}

	return announcements, rows.Err()
}

// ToggleAnnouncementStatus 切换公告状态
func (s *SQLiteDB) ToggleAnnouncementStatus(id string, enabled bool) error {
	query := `UPDATE announcements SET enabled = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, enabled, time.Now(), id)
	return err
}
