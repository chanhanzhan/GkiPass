package model

import (
	"database/sql"
	"time"
)

// NodeGroup 节点组
type NodeGroup struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Role        NodeRole  `json:"role" db:"role"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`

	// 关联设置
	RequiresEgress  bool   `json:"requires_egress" db:"requires_egress"`     // 是否需要出口
	DefaultEgressID string `json:"default_egress_id" db:"default_egress_id"` // 默认出口组ID

	// 协议和端口设置
	DisabledProtocols []string `json:"disabled_protocols" db:"disabled_protocols"`   // 禁用的协议
	AllowedPortRanges []string `json:"allowed_port_ranges" db:"allowed_port_ranges"` // 允许的端口范围

	// 权限设置
	AllowProbeView bool `json:"allow_probe_view" db:"allow_probe_view"` // 是否允许查看探针

	// 关联节点
	Nodes []string `json:"nodes" db:"-"` // 组内节点ID列表
}

// NodeGroupNode 节点组与节点的关联
type NodeGroupNode struct {
	GroupID string    `json:"group_id" db:"group_id"`
	NodeID  string    `json:"node_id" db:"node_id"`
	AddedAt time.Time `json:"added_at" db:"added_at"`
}

// CreateNodeGroup 创建节点组
func CreateNodeGroup(db *sql.DB, group *NodeGroup) error {
	// 设置创建时间和更新时间
	now := time.Now()
	group.CreatedAt = now
	group.UpdatedAt = now

	// 插入节点组
	_, err := db.Exec(`
		INSERT INTO node_groups (
			id, name, description, role, created_at, updated_at, 
			requires_egress, default_egress_id, disabled_protocols, 
			allowed_port_ranges, allow_probe_view
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		group.ID, group.Name, group.Description, group.Role, group.CreatedAt, group.UpdatedAt,
		group.RequiresEgress, group.DefaultEgressID, group.DisabledProtocols,
		group.AllowedPortRanges, group.AllowProbeView,
	)

	return err
}

// GetNodeGroup 获取节点组
func GetNodeGroup(db *sql.DB, id string) (*NodeGroup, error) {
	var group NodeGroup

	// 查询节点组
	err := db.QueryRow(`
		SELECT 
			id, name, description, role, created_at, updated_at, 
			requires_egress, default_egress_id, disabled_protocols, 
			allowed_port_ranges, allow_probe_view
		FROM node_groups 
		WHERE id = ?
	`, id).Scan(
		&group.ID, &group.Name, &group.Description, &group.Role, &group.CreatedAt, &group.UpdatedAt,
		&group.RequiresEgress, &group.DefaultEgressID, &group.DisabledProtocols,
		&group.AllowedPortRanges, &group.AllowProbeView,
	)

	if err != nil {
		return nil, err
	}

	// 查询关联节点
	rows, err := db.Query(`
		SELECT node_id FROM node_group_nodes WHERE group_id = ?
	`, id)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 收集节点ID
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err != nil {
			return nil, err
		}
		group.Nodes = append(group.Nodes, nodeID)
	}

	return &group, nil
}

// UpdateNodeGroup 更新节点组
func UpdateNodeGroup(db *sql.DB, group *NodeGroup) error {
	// 更新时间
	group.UpdatedAt = time.Now()

	// 更新节点组
	_, err := db.Exec(`
		UPDATE node_groups SET
			name = ?,
			description = ?,
			role = ?,
			updated_at = ?,
			requires_egress = ?,
			default_egress_id = ?,
			disabled_protocols = ?,
			allowed_port_ranges = ?,
			allow_probe_view = ?
		WHERE id = ?
	`,
		group.Name, group.Description, group.Role, group.UpdatedAt,
		group.RequiresEgress, group.DefaultEgressID, group.DisabledProtocols,
		group.AllowedPortRanges, group.AllowProbeView,
		group.ID,
	)

	return err
}

// DeleteNodeGroup 删除节点组
func DeleteNodeGroup(db *sql.DB, id string) error {
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	// 删除节点关联
	_, err = tx.Exec(`DELETE FROM node_group_nodes WHERE group_id = ?`, id)
	if err != nil {
		tx.Rollback()
		return err
	}

	// 删除节点组
	_, err = tx.Exec(`DELETE FROM node_groups WHERE id = ?`, id)
	if err != nil {
		tx.Rollback()
		return err
	}

	// 提交事务
	return tx.Commit()
}

// ListNodeGroups 列出所有节点组
func ListNodeGroups(db *sql.DB) ([]*NodeGroup, error) {
	// 查询所有节点组
	rows, err := db.Query(`
		SELECT 
			id, name, description, role, created_at, updated_at, 
			requires_egress, default_egress_id, disabled_protocols, 
			allowed_port_ranges, allow_probe_view
		FROM node_groups
		ORDER BY name
	`)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 收集节点组
	var groups []*NodeGroup
	for rows.Next() {
		var group NodeGroup
		if err := rows.Scan(
			&group.ID, &group.Name, &group.Description, &group.Role, &group.CreatedAt, &group.UpdatedAt,
			&group.RequiresEgress, &group.DefaultEgressID, &group.DisabledProtocols,
			&group.AllowedPortRanges, &group.AllowProbeView,
		); err != nil {
			return nil, err
		}
		groups = append(groups, &group)
	}

	// 查询每个组的节点
	for _, group := range groups {
		nodeRows, err := db.Query(`
			SELECT node_id FROM node_group_nodes WHERE group_id = ?
		`, group.ID)

		if err != nil {
			return nil, err
		}

		// 收集节点ID
		for nodeRows.Next() {
			var nodeID string
			if err := nodeRows.Scan(&nodeID); err != nil {
				nodeRows.Close()
				return nil, err
			}
			group.Nodes = append(group.Nodes, nodeID)
		}
		nodeRows.Close()
	}

	return groups, nil
}

// AddNodeToGroup 将节点添加到组
func AddNodeToGroup(db *sql.DB, groupID, nodeID string) error {
	// 检查是否已存在
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM node_group_nodes WHERE group_id = ? AND node_id = ?
	`, groupID, nodeID).Scan(&count)

	if err != nil {
		return err
	}

	if count > 0 {
		// 已存在，无需添加
		return nil
	}

	// 添加关联
	_, err = db.Exec(`
		INSERT INTO node_group_nodes (group_id, node_id, added_at)
		VALUES (?, ?, ?)
	`, groupID, nodeID, time.Now())

	return err
}

// RemoveNodeFromGroup 从组中移除节点
func RemoveNodeFromGroup(db *sql.DB, groupID, nodeID string) error {
	_, err := db.Exec(`
		DELETE FROM node_group_nodes WHERE group_id = ? AND node_id = ?
	`, groupID, nodeID)

	return err
}

// GetNodeGroups 获取节点所属的组
func GetNodeGroups(db *sql.DB, nodeID string) ([]*NodeGroup, error) {
	// 查询节点所属的组ID
	rows, err := db.Query(`
		SELECT group_id FROM node_group_nodes WHERE node_id = ?
	`, nodeID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 收集组ID
	var groupIDs []string
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, groupID)
	}

	if len(groupIDs) == 0 {
		return []*NodeGroup{}, nil
	}

	// 构建IN查询
	query := `
		SELECT 
			id, name, description, role, created_at, updated_at, 
			requires_egress, default_egress_id, disabled_protocols, 
			allowed_port_ranges, allow_probe_view
		FROM node_groups
		WHERE id IN (?` + ", ?"[len(groupIDs)-1:] + `)
		ORDER BY name
	`

	// 转换参数为interface{}切片
	args := make([]interface{}, len(groupIDs))
	for i, id := range groupIDs {
		args[i] = id
	}

	// 查询组详情
	groupRows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer groupRows.Close()

	// 收集组
	var groups []*NodeGroup
	for groupRows.Next() {
		var group NodeGroup
		if err := groupRows.Scan(
			&group.ID, &group.Name, &group.Description, &group.Role, &group.CreatedAt, &group.UpdatedAt,
			&group.RequiresEgress, &group.DefaultEgressID, &group.DisabledProtocols,
			&group.AllowedPortRanges, &group.AllowProbeView,
		); err != nil {
			return nil, err
		}

		// 添加当前节点
		group.Nodes = []string{nodeID}

		groups = append(groups, &group)
	}

	return groups, nil
}

// GetIngressGroups 获取所有入口组
func GetIngressGroups(db *sql.DB) ([]*NodeGroup, error) {
	return getGroupsByRole(db, NodeRoleIngress, NodeRoleBoth)
}

// GetEgressGroups 获取所有出口组
func GetEgressGroups(db *sql.DB) ([]*NodeGroup, error) {
	return getGroupsByRole(db, NodeRoleEgress, NodeRoleBoth)
}

// getGroupsByRole 根据角色获取组
func getGroupsByRole(db *sql.DB, roles ...NodeRole) ([]*NodeGroup, error) {
	// 构建IN查询
	query := `
		SELECT 
			id, name, description, role, created_at, updated_at, 
			requires_egress, default_egress_id, disabled_protocols, 
			allowed_port_ranges, allow_probe_view
		FROM node_groups
		WHERE role IN (?` + ", ?"[len(roles)-1:] + `)
		ORDER BY name
	`

	// 转换参数为interface{}切片
	args := make([]interface{}, len(roles))
	for i, role := range roles {
		args[i] = role
	}

	// 查询组
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 收集组
	var groups []*NodeGroup
	for rows.Next() {
		var group NodeGroup
		if err := rows.Scan(
			&group.ID, &group.Name, &group.Description, &group.Role, &group.CreatedAt, &group.UpdatedAt,
			&group.RequiresEgress, &group.DefaultEgressID, &group.DisabledProtocols,
			&group.AllowedPortRanges, &group.AllowProbeView,
		); err != nil {
			return nil, err
		}
		groups = append(groups, &group)
	}

	// 查询每个组的节点
	for _, group := range groups {
		nodeRows, err := db.Query(`
			SELECT node_id FROM node_group_nodes WHERE group_id = ?
		`, group.ID)

		if err != nil {
			return nil, err
		}

		// 收集节点ID
		for nodeRows.Next() {
			var nodeID string
			if err := nodeRows.Scan(&nodeID); err != nil {
				nodeRows.Close()
				return nil, err
			}
			group.Nodes = append(group.Nodes, nodeID)
		}
		nodeRows.Close()
	}

	return groups, nil
}
