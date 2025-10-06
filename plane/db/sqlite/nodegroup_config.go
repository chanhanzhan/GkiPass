package sqlite

import (
	"database/sql"
	"time"
)

// NodeGroupConfig 节点组配置
type NodeGroupConfig struct {
	ID                 string    `json:"id" db:"id"`
	GroupID            string    `json:"group_id" db:"group_id"`
	PortRange          string    `json:"port_range" db:"port_range"`
	AllowedProtocols   string    `json:"allowed_protocols" db:"allowed_protocols"`     // JSON数组
	BlockedProtocols   string    `json:"blocked_protocols" db:"blocked_protocols"`     // JSON数组
	AllowedTunnelTypes string    `json:"allowed_tunnel_types" db:"allowed_tunnel_types"` // JSON数组
	RequireExitGroup   bool      `json:"require_exit_group" db:"require_exit_group"`
	AllowEntryProxy    bool      `json:"allow_entry_proxy" db:"allow_entry_proxy"`
	AllowedExitGroups  string    `json:"allowed_exit_groups" db:"allowed_exit_groups"` // JSON数组
	IPType             string    `json:"ip_type" db:"ip_type"`
	TrafficMultiplier  float64   `json:"traffic_multiplier" db:"traffic_multiplier"`
	ConnectionDomain   string    `json:"connection_domain" db:"connection_domain"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// CreateNodeGroupConfig 创建节点组配置
func (s *SQLiteDB) CreateNodeGroupConfig(config *NodeGroupConfig) error {
	query := `
		INSERT INTO node_group_configs 
		(id, group_id, port_range, allowed_protocols, blocked_protocols, allowed_tunnel_types,
		 require_exit_group, allow_entry_proxy, allowed_exit_groups, ip_type, traffic_multiplier,
		 connection_domain, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		config.ID, config.GroupID, config.PortRange, config.AllowedProtocols,
		config.BlockedProtocols, config.AllowedTunnelTypes, config.RequireExitGroup,
		config.AllowEntryProxy, config.AllowedExitGroups, config.IPType,
		config.TrafficMultiplier, config.ConnectionDomain, config.CreatedAt, config.UpdatedAt)
	return err
}

// GetNodeGroupConfig 获取节点组配置
func (s *SQLiteDB) GetNodeGroupConfig(groupID string) (*NodeGroupConfig, error) {
	config := &NodeGroupConfig{}
	query := `SELECT * FROM node_group_configs WHERE group_id = ?`
	
	err := s.db.QueryRow(query, groupID).Scan(
		&config.ID, &config.GroupID, &config.PortRange, &config.AllowedProtocols,
		&config.BlockedProtocols, &config.AllowedTunnelTypes, &config.RequireExitGroup,
		&config.AllowEntryProxy, &config.AllowedExitGroups, &config.IPType,
		&config.TrafficMultiplier, &config.ConnectionDomain, &config.CreatedAt, &config.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return config, err
}

// UpdateNodeGroupConfig 更新节点组配置
func (s *SQLiteDB) UpdateNodeGroupConfig(config *NodeGroupConfig) error {
	query := `
		UPDATE node_group_configs 
		SET port_range = ?, allowed_protocols = ?, blocked_protocols = ?,
		    allowed_tunnel_types = ?, require_exit_group = ?, allow_entry_proxy = ?,
		    allowed_exit_groups = ?, ip_type = ?, traffic_multiplier = ?,
		    connection_domain = ?, updated_at = ?
		WHERE group_id = ?
	`
	_, err := s.db.Exec(query,
		config.PortRange, config.AllowedProtocols, config.BlockedProtocols,
		config.AllowedTunnelTypes, config.RequireExitGroup, config.AllowEntryProxy,
		config.AllowedExitGroups, config.IPType, config.TrafficMultiplier,
		config.ConnectionDomain, config.UpdatedAt, config.GroupID)
	return err
}

// DeleteNodeGroupConfig 删除节点组配置
func (s *SQLiteDB) DeleteNodeGroupConfig(groupID string) error {
	query := `DELETE FROM node_group_configs WHERE group_id = ?`
	_, err := s.db.Exec(query, groupID)
	return err
}

// UpsertNodeGroupConfig 创建或更新节点组配置
func (s *SQLiteDB) UpsertNodeGroupConfig(config *NodeGroupConfig) error {
	existing, err := s.GetNodeGroupConfig(config.GroupID)
	if err != nil {
		return err
	}

	if existing == nil {
		return s.CreateNodeGroupConfig(config)
	}

	config.ID = existing.ID
	config.CreatedAt = existing.CreatedAt
	return s.UpdateNodeGroupConfig(config)
}

