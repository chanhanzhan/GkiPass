package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Rule 隧道规则
type Rule struct {
	// 基本信息
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Enabled     bool      `json:"enabled" db:"enabled"`
	Priority    int       `json:"priority" db:"priority"`
	Version     int64     `json:"version" db:"version"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	CreatedBy   string    `json:"created_by" db:"created_by"`

	// 隧道配置
	Protocol        string `json:"protocol" db:"protocol"`
	ListenPort      int    `json:"listen_port" db:"listen_port"`
	TargetAddress   string `json:"target_address" db:"target_address"`
	TargetPort      int    `json:"target_port" db:"target_port"`
	IngressNodeID   string `json:"ingress_node_id" db:"ingress_node_id"`
	EgressNodeID    string `json:"egress_node_id" db:"egress_node_id"`
	IngressGroupID  string `json:"ingress_group_id" db:"ingress_group_id"`
	EgressGroupID   string `json:"egress_group_id" db:"egress_group_id"`
	IngressProtocol string `json:"ingress_protocol" db:"ingress_protocol"`
	EgressProtocol  string `json:"egress_protocol" db:"egress_protocol"`

	// 高级选项
	EnableEncryption bool              `json:"enable_encryption" db:"enable_encryption"`
	RateLimitBPS     int64             `json:"rate_limit_bps" db:"rate_limit_bps"`
	MaxConnections   int               `json:"max_connections" db:"max_connections"`
	IdleTimeout      int               `json:"idle_timeout" db:"idle_timeout"`
	ACLRules         []ACLRule         `json:"acl_rules" db:"-"`
	Options          map[string]string `json:"options" db:"-"`

	// 统计信息
	ConnectionCount int64     `json:"connection_count" db:"connection_count"`
	BytesIn         int64     `json:"bytes_in" db:"bytes_in"`
	BytesOut        int64     `json:"bytes_out" db:"bytes_out"`
	LastActive      time.Time `json:"last_active" db:"last_active"`
}

// ACLRule 访问控制规则
type ACLRule struct {
	ID        string `json:"id" db:"id"`
	RuleID    string `json:"rule_id" db:"rule_id"`
	Action    string `json:"action" db:"action"`
	Priority  int    `json:"priority" db:"priority"`
	SourceIP  string `json:"source_ip" db:"source_ip"`
	DestIP    string `json:"dest_ip" db:"dest_ip"`
	Protocol  string `json:"protocol" db:"protocol"`
	PortRange string `json:"port_range" db:"port_range"`
}

// CreateRule 创建规则
func CreateRule(db *sql.DB, rule *Rule) error {
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	// 设置创建时间和更新时间
	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	rule.Version = 1

	// 插入规则
	_, err = tx.Exec(`
		INSERT INTO rules (
			id, name, description, enabled, priority, version, created_at, updated_at, created_by,
			protocol, listen_port, target_address, target_port, ingress_node_id, egress_node_id,
			ingress_group_id, egress_group_id, ingress_protocol, egress_protocol,
			enable_encryption, rate_limit_bps, max_connections, idle_timeout,
			connection_count, bytes_in, bytes_out, last_active
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rule.ID, rule.Name, rule.Description, rule.Enabled, rule.Priority, rule.Version,
		rule.CreatedAt, rule.UpdatedAt, rule.CreatedBy, rule.Protocol, rule.ListenPort,
		rule.TargetAddress, rule.TargetPort, rule.IngressNodeID, rule.EgressNodeID,
		rule.IngressGroupID, rule.EgressGroupID, rule.IngressProtocol, rule.EgressProtocol,
		rule.EnableEncryption, rule.RateLimitBPS, rule.MaxConnections, rule.IdleTimeout,
		0, 0, 0, now,
	)

	if err != nil {
		tx.Rollback()
		return err
	}

	// 插入ACL规则
	for _, acl := range rule.ACLRules {
		acl.RuleID = rule.ID
		_, err = tx.Exec(`
			INSERT INTO rule_acls (
				id, rule_id, action, priority, source_ip, dest_ip, protocol, port_range
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			acl.ID, acl.RuleID, acl.Action, acl.Priority, acl.SourceIP, acl.DestIP, acl.Protocol, acl.PortRange,
		)

		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// 插入选项
	if len(rule.Options) > 0 {
		optionsJSON, err := json.Marshal(rule.Options)
		if err != nil {
			tx.Rollback()
			return err
		}

		_, err = tx.Exec(`
			INSERT INTO rule_options (rule_id, options) VALUES (?, ?)
		`, rule.ID, optionsJSON)

		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// 提交事务
	return tx.Commit()
}

// GetRule 获取规则
func GetRule(db *sql.DB, id string) (*Rule, error) {
	var rule Rule

	// 查询规则
	err := db.QueryRow(`
		SELECT 
			id, name, description, enabled, priority, version, created_at, updated_at, created_by,
			protocol, listen_port, target_address, target_port, ingress_node_id, egress_node_id,
			ingress_group_id, egress_group_id, ingress_protocol, egress_protocol,
			enable_encryption, rate_limit_bps, max_connections, idle_timeout,
			connection_count, bytes_in, bytes_out, last_active
		FROM rules 
		WHERE id = ?
	`, id).Scan(
		&rule.ID, &rule.Name, &rule.Description, &rule.Enabled, &rule.Priority, &rule.Version,
		&rule.CreatedAt, &rule.UpdatedAt, &rule.CreatedBy, &rule.Protocol, &rule.ListenPort,
		&rule.TargetAddress, &rule.TargetPort, &rule.IngressNodeID, &rule.EgressNodeID,
		&rule.IngressGroupID, &rule.EgressGroupID, &rule.IngressProtocol, &rule.EgressProtocol,
		&rule.EnableEncryption, &rule.RateLimitBPS, &rule.MaxConnections, &rule.IdleTimeout,
		&rule.ConnectionCount, &rule.BytesIn, &rule.BytesOut, &rule.LastActive,
	)

	if err != nil {
		return nil, err
	}

	// 查询ACL规则
	rows, err := db.Query(`
		SELECT id, action, priority, source_ip, dest_ip, protocol, port_range
		FROM rule_acls
		WHERE rule_id = ?
		ORDER BY priority DESC
	`, id)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 收集ACL规则
	for rows.Next() {
		var acl ACLRule
		acl.RuleID = id

		if err := rows.Scan(
			&acl.ID, &acl.Action, &acl.Priority, &acl.SourceIP, &acl.DestIP, &acl.Protocol, &acl.PortRange,
		); err != nil {
			return nil, err
		}

		rule.ACLRules = append(rule.ACLRules, acl)
	}

	// 查询选项
	var optionsJSON []byte
	err = db.QueryRow(`
		SELECT options FROM rule_options WHERE rule_id = ?
	`, id).Scan(&optionsJSON)

	if err == nil && len(optionsJSON) > 0 {
		// 解析选项
		if err := json.Unmarshal(optionsJSON, &rule.Options); err != nil {
			return nil, err
		}
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	// 如果选项为nil，初始化为空map
	if rule.Options == nil {
		rule.Options = make(map[string]string)
	}

	return &rule, nil
}

// UpdateRule 更新规则
func UpdateRule(db *sql.DB, rule *Rule) error {
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	// 设置更新时间和版本
	rule.UpdatedAt = time.Now()
	rule.Version++

	// 更新规则
	_, err = tx.Exec(`
		UPDATE rules SET
			name = ?, description = ?, enabled = ?, priority = ?, version = ?, updated_at = ?,
			protocol = ?, listen_port = ?, target_address = ?, target_port = ?, 
			ingress_node_id = ?, egress_node_id = ?, ingress_group_id = ?, egress_group_id = ?,
			ingress_protocol = ?, egress_protocol = ?, enable_encryption = ?, 
			rate_limit_bps = ?, max_connections = ?, idle_timeout = ?
		WHERE id = ?
	`,
		rule.Name, rule.Description, rule.Enabled, rule.Priority, rule.Version, rule.UpdatedAt,
		rule.Protocol, rule.ListenPort, rule.TargetAddress, rule.TargetPort,
		rule.IngressNodeID, rule.EgressNodeID, rule.IngressGroupID, rule.EgressGroupID,
		rule.IngressProtocol, rule.EgressProtocol, rule.EnableEncryption,
		rule.RateLimitBPS, rule.MaxConnections, rule.IdleTimeout,
		rule.ID,
	)

	if err != nil {
		tx.Rollback()
		return err
	}

	// 删除旧的ACL规则
	_, err = tx.Exec(`DELETE FROM rule_acls WHERE rule_id = ?`, rule.ID)
	if err != nil {
		tx.Rollback()
		return err
	}

	// 插入新的ACL规则
	for _, acl := range rule.ACLRules {
		acl.RuleID = rule.ID
		_, err = tx.Exec(`
			INSERT INTO rule_acls (
				id, rule_id, action, priority, source_ip, dest_ip, protocol, port_range
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			acl.ID, acl.RuleID, acl.Action, acl.Priority, acl.SourceIP, acl.DestIP, acl.Protocol, acl.PortRange,
		)

		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// 更新选项
	if len(rule.Options) > 0 {
		optionsJSON, err := json.Marshal(rule.Options)
		if err != nil {
			tx.Rollback()
			return err
		}

		// 删除旧选项
		_, err = tx.Exec(`DELETE FROM rule_options WHERE rule_id = ?`, rule.ID)
		if err != nil {
			tx.Rollback()
			return err
		}

		// 插入新选项
		_, err = tx.Exec(`
			INSERT INTO rule_options (rule_id, options) VALUES (?, ?)
		`, rule.ID, optionsJSON)

		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// 提交事务
	return tx.Commit()
}

// DeleteRule 删除规则
func DeleteRule(db *sql.DB, id string) error {
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	// 删除ACL规则
	_, err = tx.Exec(`DELETE FROM rule_acls WHERE rule_id = ?`, id)
	if err != nil {
		tx.Rollback()
		return err
	}

	// 删除选项
	_, err = tx.Exec(`DELETE FROM rule_options WHERE rule_id = ?`, id)
	if err != nil {
		tx.Rollback()
		return err
	}

	// 删除规则
	_, err = tx.Exec(`DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		tx.Rollback()
		return err
	}

	// 提交事务
	return tx.Commit()
}

// ListRules 列出所有规则
func ListRules(db *sql.DB) ([]*Rule, error) {
	// 查询规则
	rows, err := db.Query(`
		SELECT 
			id, name, description, enabled, priority, version, created_at, updated_at, created_by,
			protocol, listen_port, target_address, target_port, ingress_node_id, egress_node_id,
			ingress_group_id, egress_group_id, ingress_protocol, egress_protocol,
			enable_encryption, rate_limit_bps, max_connections, idle_timeout,
			connection_count, bytes_in, bytes_out, last_active
		FROM rules
		ORDER BY priority DESC, name ASC
	`)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 收集规则
	var rules []*Rule
	for rows.Next() {
		var rule Rule

		if err := rows.Scan(
			&rule.ID, &rule.Name, &rule.Description, &rule.Enabled, &rule.Priority, &rule.Version,
			&rule.CreatedAt, &rule.UpdatedAt, &rule.CreatedBy, &rule.Protocol, &rule.ListenPort,
			&rule.TargetAddress, &rule.TargetPort, &rule.IngressNodeID, &rule.EgressNodeID,
			&rule.IngressGroupID, &rule.EgressGroupID, &rule.IngressProtocol, &rule.EgressProtocol,
			&rule.EnableEncryption, &rule.RateLimitBPS, &rule.MaxConnections, &rule.IdleTimeout,
			&rule.ConnectionCount, &rule.BytesIn, &rule.BytesOut, &rule.LastActive,
		); err != nil {
			return nil, err
		}

		rules = append(rules, &rule)
	}

	// 查询每个规则的ACL规则
	for _, rule := range rules {
		aclRows, err := db.Query(`
			SELECT id, action, priority, source_ip, dest_ip, protocol, port_range
			FROM rule_acls
			WHERE rule_id = ?
			ORDER BY priority DESC
		`, rule.ID)

		if err != nil {
			return nil, err
		}

		// 收集ACL规则
		for aclRows.Next() {
			var acl ACLRule
			acl.RuleID = rule.ID

			if err := aclRows.Scan(
				&acl.ID, &acl.Action, &acl.Priority, &acl.SourceIP, &acl.DestIP, &acl.Protocol, &acl.PortRange,
			); err != nil {
				aclRows.Close()
				return nil, err
			}

			rule.ACLRules = append(rule.ACLRules, acl)
		}
		aclRows.Close()

		// 查询选项
		var optionsJSON []byte
		err = db.QueryRow(`
			SELECT options FROM rule_options WHERE rule_id = ?
		`, rule.ID).Scan(&optionsJSON)

		if err == nil && len(optionsJSON) > 0 {
			// 解析选项
			rule.Options = make(map[string]string)
			if err := json.Unmarshal(optionsJSON, &rule.Options); err != nil {
				return nil, err
			}
		} else if err != sql.ErrNoRows {
			return nil, err
		}

		// 如果选项为nil，初始化为空map
		if rule.Options == nil {
			rule.Options = make(map[string]string)
		}
	}

	return rules, nil
}

// UpdateRuleStats 更新规则统计信息
func UpdateRuleStats(db *sql.DB, id string, connectionCount, bytesIn, bytesOut int64) error {
	_, err := db.Exec(`
		UPDATE rules SET
			connection_count = connection_count + ?,
			bytes_in = bytes_in + ?,
			bytes_out = bytes_out + ?,
			last_active = ?
		WHERE id = ?
	`,
		connectionCount, bytesIn, bytesOut, time.Now(), id,
	)

	return err
}

// ResetRuleStats 重置规则统计信息
func ResetRuleStats(db *sql.DB, id string) error {
	_, err := db.Exec(`
		UPDATE rules SET
			connection_count = 0,
			bytes_in = 0,
			bytes_out = 0
		WHERE id = ?
	`, id)

	return err
}
