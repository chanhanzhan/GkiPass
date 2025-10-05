package sqlite

import (
	"database/sql"

	dbinit "gkipass/plane/db/init"
)

// CreateNodeGroup 创建节点组
func (s *SQLiteDB) CreateNodeGroup(group *dbinit.NodeGroup) error {
	query := `
		INSERT INTO node_groups (id, name, type, user_id, description)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, group.ID, group.Name, group.Type, group.UserID, group.Description)
	return err
}

// GetNodeGroup 获取节点组
func (s *SQLiteDB) GetNodeGroup(id string) (*dbinit.NodeGroup, error) {
	group := &dbinit.NodeGroup{}
	query := `SELECT * FROM node_groups WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&group.ID, &group.Name, &group.Type, &group.UserID, &group.NodeCount,
		&group.CreatedAt, &group.UpdatedAt, &group.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return group, err
}

// ListNodeGroups 列出节点组
func (s *SQLiteDB) ListNodeGroups(userID, groupType string) ([]*dbinit.NodeGroup, error) {
	query := `SELECT * FROM node_groups WHERE 1=1`
	args := []interface{}{}

	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	if groupType != "" {
		query += ` AND type = ?`
		args = append(args, groupType)
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := []*dbinit.NodeGroup{}
	for rows.Next() {
		group := &dbinit.NodeGroup{}
		err := rows.Scan(
			&group.ID, &group.Name, &group.Type, &group.UserID, &group.NodeCount,
			&group.CreatedAt, &group.UpdatedAt, &group.Description,
		)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// UpdateNodeGroup 更新节点组
func (s *SQLiteDB) UpdateNodeGroup(group *dbinit.NodeGroup) error {
	query := `
		UPDATE node_groups 
		SET name=?, type=?, description=?
		WHERE id=?
	`
	result, err := s.db.Exec(query, group.Name, group.Type, group.Description, group.ID)
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

// DeleteNodeGroup 删除节点组
func (s *SQLiteDB) DeleteNodeGroup(id string) error {
	query := `DELETE FROM node_groups WHERE id = ?`
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

// GetNodesInGroup 获取组内所有节点
func (s *SQLiteDB) GetNodesInGroup(groupID string) ([]*dbinit.Node, error) {
	query := `SELECT * FROM nodes WHERE group_id = ? ORDER BY created_at DESC`
	rows, err := s.db.Query(query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := []*dbinit.Node{}
	for rows.Next() {
		node := &dbinit.Node{}
		err := rows.Scan(
			&node.ID, &node.Name, &node.Type, &node.Status, &node.IP, &node.Port,
			&node.Version, &node.CertID, &node.APIKey, &node.GroupID, &node.UserID,
			&node.LastSeen, &node.CreatedAt, &node.UpdatedAt, &node.Tags, &node.Description,
		)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, rows.Err()
}

