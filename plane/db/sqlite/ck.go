package sqlite

import (
	"database/sql"
	"time"

	dbinit "gkipass/plane/db/init"
)

// CreateConnectionKey 创建 CK
func (s *SQLiteDB) CreateConnectionKey(ck *dbinit.ConnectionKey) error {
	query := `
		INSERT INTO connection_keys (id, key, node_id, type, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, ck.ID, ck.Key, ck.NodeID, ck.Type, ck.ExpiresAt, ck.CreatedAt)
	return err
}

// GetConnectionKeyByKey 根据 Key 获取 CK
func (s *SQLiteDB) GetConnectionKeyByKey(key string) (*dbinit.ConnectionKey, error) {
	ck := &dbinit.ConnectionKey{}
	query := `SELECT * FROM connection_keys WHERE key = ?`
	err := s.db.QueryRow(query, key).Scan(
		&ck.ID, &ck.Key, &ck.NodeID, &ck.Type, &ck.ExpiresAt, &ck.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ck, err
}

// GetConnectionKeysByNodeID 获取节点的所有 CK
func (s *SQLiteDB) GetConnectionKeysByNodeID(nodeID string) ([]*dbinit.ConnectionKey, error) {
	query := `SELECT * FROM connection_keys WHERE node_id = ? ORDER BY created_at DESC`
	rows, err := s.db.Query(query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cks := []*dbinit.ConnectionKey{}
	for rows.Next() {
		ck := &dbinit.ConnectionKey{}
		err := rows.Scan(
			&ck.ID, &ck.Key, &ck.NodeID, &ck.Type, &ck.ExpiresAt, &ck.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		cks = append(cks, ck)
	}

	return cks, rows.Err()
}

// DeleteConnectionKey 删除 CK
func (s *SQLiteDB) DeleteConnectionKey(id string) error {
	query := `DELETE FROM connection_keys WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// DeleteExpiredConnectionKeys 删除过期的 CK
func (s *SQLiteDB) DeleteExpiredConnectionKeys() error {
	query := `DELETE FROM connection_keys WHERE expires_at < ?`
	_, err := s.db.Exec(query, time.Now())
	return err
}

// RevokeConnectionKey 撤销 CK
func (s *SQLiteDB) RevokeConnectionKey(key string) error {
	query := `DELETE FROM connection_keys WHERE key = ?`
	_, err := s.db.Exec(query, key)
	return err
}
