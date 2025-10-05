package sqlite

import (
	"database/sql"
	"time"

	dbinit "gkipass/plane/db/init"

	"github.com/google/uuid"
)

// CreateNotification 创建通知
func (s *SQLiteDB) CreateNotification(notification *dbinit.Notification) error {
	if notification.ID == "" {
		notification.ID = uuid.New().String()
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO notifications (id, user_id, type, title, content, link, is_read, priority, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		notification.ID,
		notification.UserID,
		notification.Type,
		notification.Title,
		notification.Content,
		notification.Link,
		notification.IsRead,
		notification.Priority,
		notification.CreatedAt,
	)
	return err
}

// GetNotification 获取通知
func (s *SQLiteDB) GetNotification(id string) (*dbinit.Notification, error) {
	notification := &dbinit.Notification{}
	query := `
		SELECT id, user_id, type, title, content, link, is_read, priority, created_at
		FROM notifications WHERE id = ?
	`
	err := s.db.QueryRow(query, id).Scan(
		&notification.ID,
		&notification.UserID,
		&notification.Type,
		&notification.Title,
		&notification.Content,
		&notification.Link,
		&notification.IsRead,
		&notification.Priority,
		&notification.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return notification, err
}

// ListNotifications 获取用户通知列表
func (s *SQLiteDB) ListNotifications(userID string, page, limit int) ([]*dbinit.Notification, int, error) {
	offset := (page - 1) * limit

	// 获取总数（包括用户特定通知和全局通知）
	var total int
	countQuery := `SELECT COUNT(*) FROM notifications WHERE user_id = ? OR user_id = ''`
	err := s.db.QueryRow(countQuery, userID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取列表
	notifications := []*dbinit.Notification{}
	query := `
		SELECT id, user_id, type, title, content, link, is_read, priority, created_at
		FROM notifications 
		WHERE user_id = ? OR user_id = ''
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		n := &dbinit.Notification{}
		err := rows.Scan(
			&n.ID,
			&n.UserID,
			&n.Type,
			&n.Title,
			&n.Content,
			&n.Link,
			&n.IsRead,
			&n.Priority,
			&n.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		notifications = append(notifications, n)
	}

	return notifications, total, rows.Err()
}

// ListAllNotifications 获取所有通知（管理员）
func (s *SQLiteDB) ListAllNotifications(page, limit int) ([]*dbinit.Notification, int, error) {
	offset := (page - 1) * limit

	// 获取总数
	var total int
	countQuery := `SELECT COUNT(*) FROM notifications`
	err := s.db.QueryRow(countQuery).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取列表
	notifications := []*dbinit.Notification{}
	query := `
		SELECT id, user_id, type, title, content, link, is_read, priority, created_at
		FROM notifications 
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		n := &dbinit.Notification{}
		err := rows.Scan(
			&n.ID,
			&n.UserID,
			&n.Type,
			&n.Title,
			&n.Content,
			&n.Link,
			&n.IsRead,
			&n.Priority,
			&n.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		notifications = append(notifications, n)
	}

	return notifications, total, rows.Err()
}

// MarkNotificationAsRead 标记通知为已读
func (s *SQLiteDB) MarkNotificationAsRead(id string) error {
	query := `UPDATE notifications SET is_read = 1 WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// MarkAllNotificationsAsRead 标记用户所有通知为已读
func (s *SQLiteDB) MarkAllNotificationsAsRead(userID string) error {
	query := `UPDATE notifications SET is_read = 1 WHERE user_id = ? AND is_read = 0`
	_, err := s.db.Exec(query, userID)
	return err
}

// DeleteNotification 删除通知
func (s *SQLiteDB) DeleteNotification(id string) error {
	query := `DELETE FROM notifications WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// GetUnreadNotificationCount 获取未读通知数量
func (s *SQLiteDB) GetUnreadNotificationCount(userID string) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM notifications WHERE (user_id = ? OR user_id = '') AND is_read = 0`
	err := s.db.QueryRow(query, userID).Scan(&count)
	return count, err
}
