package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	dbinit "gkipass/plane/db/init"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteDB SQLite数据库客户端
type SQLiteDB struct {
	db *sql.DB
}

// NewSQLiteDB 创建新的SQLite数据库连接
func NewSQLiteDB(dbPath string) (*SQLiteDB, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	client := &SQLiteDB{db: db}

	// 初始化schema
	if err := dbinit.InitSQLiteSchema(db); err != nil {
		return nil, err
	}

	// 创建触发器
	if err := dbinit.CreateTriggers(db); err != nil {
		return nil, err
	}

	return client, nil
}

// Close 关闭数据库连接
func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

// Get 获取底层的 *sql.DB
func (s *SQLiteDB) Get() *sql.DB {
	return s.db
}

// GetDB 获取原始数据库连接
func (s *SQLiteDB) GetDB() *sql.DB {
	return s.db
}

// === Node 操作 ===

// CreateNode 创建节点
func (s *SQLiteDB) CreateNode(node *dbinit.Node) error {
	query := `
		INSERT INTO nodes (id, name, type, status, ip, port, version, cert_id, api_key, group_id, user_id, last_seen, tags, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, node.ID, node.Name, node.Type, node.Status, node.IP, node.Port,
		node.Version, node.CertID, node.APIKey, node.GroupID, node.UserID, node.LastSeen, node.Tags, node.Description)
	return err
}

// GetNode 获取节点
func (s *SQLiteDB) GetNode(id string) (*dbinit.Node, error) {
	node := &dbinit.Node{}
	query := `SELECT * FROM nodes WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&node.ID, &node.Name, &node.Type, &node.Status, &node.IP, &node.Port,
		&node.Version, &node.CertID, &node.APIKey, &node.GroupID, &node.UserID, &node.LastSeen,
		&node.CreatedAt, &node.UpdatedAt, &node.Tags, &node.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return node, err
}

// GetNodeByAPIKey 通过API Key获取节点
func (s *SQLiteDB) GetNodeByAPIKey(apiKey string) (*dbinit.Node, error) {
	node := &dbinit.Node{}
	query := `SELECT * FROM nodes WHERE api_key = ?`
	err := s.db.QueryRow(query, apiKey).Scan(
		&node.ID, &node.Name, &node.Type, &node.Status, &node.IP, &node.Port,
		&node.Version, &node.CertID, &node.APIKey, &node.GroupID, &node.UserID, &node.LastSeen,
		&node.CreatedAt, &node.UpdatedAt, &node.Tags, &node.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return node, err
}

// ListNodes 列出节点
func (s *SQLiteDB) ListNodes(nodeType, status string, limit, offset int) ([]*dbinit.Node, error) {
	query := `SELECT * FROM nodes WHERE 1=1`
	args := []interface{}{}

	if nodeType != "" {
		query += ` AND type = ?`
		args = append(args, nodeType)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := []*dbinit.Node{}
	for rows.Next() {
		node := &dbinit.Node{}
		err := rows.Scan(
			&node.ID, &node.Name, &node.Type, &node.Status, &node.IP, &node.Port,
			&node.Version, &node.CertID, &node.APIKey, &node.GroupID, &node.UserID, &node.LastSeen,
			&node.CreatedAt, &node.UpdatedAt, &node.Tags, &node.Description,
		)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, rows.Err()
}

// UpdateNode 更新节点
func (s *SQLiteDB) UpdateNode(node *dbinit.Node) error {
	query := `
		UPDATE nodes 
		SET name=?, type=?, status=?, ip=?, port=?, version=?, cert_id=?, group_id=?, last_seen=?, tags=?, description=?
		WHERE id=?
	`
	result, err := s.db.Exec(query, node.Name, node.Type, node.Status, node.IP, node.Port,
		node.Version, node.CertID, node.GroupID, node.LastSeen, node.Tags, node.Description, node.ID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("node not found")
	}

	return nil
}

// DeleteNode 删除节点
func (s *SQLiteDB) DeleteNode(id string) error {
	query := `DELETE FROM nodes WHERE id = ?`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("node not found")
	}

	return nil
}

// === Policy 操作 ===

// CreatePolicy 创建策略
func (s *SQLiteDB) CreatePolicy(policy *dbinit.Policy) error {
	query := `
		INSERT INTO policies (id, name, type, priority, enabled, config, node_ids, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, policy.ID, policy.Name, policy.Type, policy.Priority,
		policy.Enabled, policy.Config, policy.NodeIDs, policy.Description)
	return err
}

// GetPolicy 获取策略
func (s *SQLiteDB) GetPolicy(id string) (*dbinit.Policy, error) {
	policy := &dbinit.Policy{}
	query := `SELECT * FROM policies WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&policy.ID, &policy.Name, &policy.Type, &policy.Priority, &policy.Enabled,
		&policy.Config, &policy.NodeIDs, &policy.CreatedAt, &policy.UpdatedAt, &policy.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return policy, err
}

// ListPolicies 列出策略
func (s *SQLiteDB) ListPolicies(policyType string, enabled *bool) ([]*dbinit.Policy, error) {
	query := `SELECT * FROM policies WHERE 1=1`
	args := []interface{}{}

	if policyType != "" {
		query += ` AND type = ?`
		args = append(args, policyType)
	}
	if enabled != nil {
		query += ` AND enabled = ?`
		args = append(args, *enabled)
	}

	query += ` ORDER BY priority ASC, created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	policies := []*dbinit.Policy{}
	for rows.Next() {
		policy := &dbinit.Policy{}
		err := rows.Scan(
			&policy.ID, &policy.Name, &policy.Type, &policy.Priority, &policy.Enabled,
			&policy.Config, &policy.NodeIDs, &policy.CreatedAt, &policy.UpdatedAt, &policy.Description,
		)
		if err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}

	return policies, rows.Err()
}

// UpdatePolicy 更新策略
func (s *SQLiteDB) UpdatePolicy(policy *dbinit.Policy) error {
	query := `
		UPDATE policies 
		SET name=?, type=?, priority=?, enabled=?, config=?, node_ids=?, description=?
		WHERE id=?
	`
	result, err := s.db.Exec(query, policy.Name, policy.Type, policy.Priority, policy.Enabled,
		policy.Config, policy.NodeIDs, policy.Description, policy.ID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("policy not found")
	}

	return nil
}

// DeletePolicy 删除策略
func (s *SQLiteDB) DeletePolicy(id string) error {
	query := `DELETE FROM policies WHERE id = ?`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("policy not found")
	}

	return nil
}

// === Certificate 操作 ===

// CreateCertificate 创建证书
func (s *SQLiteDB) CreateCertificate(cert *dbinit.Certificate) error {
	query := `
		INSERT INTO certificates (id, type, name, common_name, public_key, private_key, pin, parent_id, not_before, not_after, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, cert.ID, cert.Type, cert.Name, cert.CommonName, cert.PublicKey,
		cert.PrivateKey, cert.Pin, cert.ParentID, cert.NotBefore, cert.NotAfter, cert.Description)
	return err
}

// GetCertificate 获取证书
func (s *SQLiteDB) GetCertificate(id string) (*dbinit.Certificate, error) {
	cert := &dbinit.Certificate{}
	query := `SELECT * FROM certificates WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&cert.ID, &cert.Type, &cert.Name, &cert.CommonName, &cert.PublicKey, &cert.PrivateKey,
		&cert.Pin, &cert.ParentID, &cert.NotBefore, &cert.NotAfter, &cert.CreatedAt,
		&cert.Revoked, &cert.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return cert, err
}

// ListCertificates 列出证书
func (s *SQLiteDB) ListCertificates(certType string, revoked *bool) ([]*dbinit.Certificate, error) {
	query := `SELECT * FROM certificates WHERE 1=1`
	args := []interface{}{}

	if certType != "" {
		query += ` AND type = ?`
		args = append(args, certType)
	}
	if revoked != nil {
		query += ` AND revoked = ?`
		args = append(args, *revoked)
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	certs := []*dbinit.Certificate{}
	for rows.Next() {
		cert := &dbinit.Certificate{}
		err := rows.Scan(
			&cert.ID, &cert.Type, &cert.Name, &cert.CommonName, &cert.PublicKey, &cert.PrivateKey,
			&cert.Pin, &cert.ParentID, &cert.NotBefore, &cert.NotAfter, &cert.CreatedAt,
			&cert.Revoked, &cert.Description,
		)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}

	return certs, rows.Err()
}

// RevokeCertificate 吊销证书
func (s *SQLiteDB) RevokeCertificate(id string) error {
	query := `UPDATE certificates SET revoked = 1 WHERE id = ?`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("certificate not found")
	}

	return nil
}

// === Statistics 操作 ===

// CreateStatistics 创建统计记录
func (s *SQLiteDB) CreateStatistics(stats *dbinit.Statistics) error {
	query := `
		INSERT INTO statistics (id, node_id, timestamp, bytes_in, bytes_out, packets_in, packets_out,
			connections, active_sessions, error_count, avg_latency, cpu_usage, memory_usage)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, stats.ID, stats.NodeID, stats.Timestamp, stats.BytesIn, stats.BytesOut,
		stats.PacketsIn, stats.PacketsOut, stats.Connections, stats.ActiveSessions, stats.ErrorCount,
		stats.AvgLatency, stats.CPUUsage, stats.MemoryUsage)
	return err
}

// GetStatistics 获取节点统计（按时间范围）
func (s *SQLiteDB) GetStatistics(nodeID string, from, to time.Time) ([]*dbinit.Statistics, error) {
	query := `
		SELECT * FROM statistics 
		WHERE node_id = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp ASC
	`
	rows, err := s.db.Query(query, nodeID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	statsList := []*dbinit.Statistics{}
	for rows.Next() {
		stats := &dbinit.Statistics{}
		err := rows.Scan(
			&stats.ID, &stats.NodeID, &stats.Timestamp, &stats.BytesIn, &stats.BytesOut,
			&stats.PacketsIn, &stats.PacketsOut, &stats.Connections, &stats.ActiveSessions,
			&stats.ErrorCount, &stats.AvgLatency, &stats.CPUUsage, &stats.MemoryUsage,
		)
		if err != nil {
			return nil, err
		}
		statsList = append(statsList, stats)
	}

	return statsList, rows.Err()
}
