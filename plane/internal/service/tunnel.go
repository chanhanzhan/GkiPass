package service

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gkipass/plane/internal/model"
)

// TunnelService 隧道服务
type TunnelService struct {
	db               *sql.DB
	logger           *zap.Logger
	ruleService      *RuleService
	nodeService      *NodeService
	nodeGroupService *NodeGroupService
}

// NewTunnelService 创建隧道服务
func NewTunnelService(db *sql.DB, ruleService *RuleService, nodeService *NodeService, nodeGroupService *NodeGroupService) *TunnelService {
	return &TunnelService{
		db:               db,
		logger:           zap.L().Named("tunnel-service"),
		ruleService:      ruleService,
		nodeService:      nodeService,
		nodeGroupService: nodeGroupService,
	}
}

// UpdateTraffic 更新隧道流量统计
func (s *TunnelService) UpdateTraffic(tunnelID string, bytesIn, bytesOut int64) error {
	// TODO: Implement traffic update
	s.logger.Debug("更新流量统计",
		zap.String("tunnel_id", tunnelID),
		zap.Int64("bytes_in", bytesIn),
		zap.Int64("bytes_out", bytesOut))
	return nil
}

// GetTunnelsByGroupID 根据组ID获取隧道列表
func (s *TunnelService) GetTunnelsByGroupID(groupID string) ([]*model.Tunnel, error) {
	// TODO: Implement group-based tunnel retrieval
	s.logger.Debug("获取组隧道列表", zap.String("group_id", groupID))
	return []*model.Tunnel{}, nil
}

// CreateTunnel 创建隧道
func (s *TunnelService) CreateTunnel(tunnel *model.Tunnel, userID string) error {
	// 验证隧道
	if err := s.validateTunnel(tunnel); err != nil {
		return err
	}

	// 生成ID
	if tunnel.ID == "" {
		tunnel.ID = uuid.New().String()
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 设置创建时间和更新时间
	now := time.Now()
	tunnel.CreatedAt = now
	tunnel.UpdatedAt = now
	tunnel.CreatedBy = userID

	// 插入隧道
	_, err = tx.Exec(`
		INSERT INTO tunnels (
			id, name, description, enabled, created_at, updated_at, created_by,
			ingress_node_id, egress_node_id, ingress_group_id, egress_group_id,
			ingress_protocol, egress_protocol, listen_port, target_address, target_port,
			enable_encryption, rate_limit_bps, max_connections, idle_timeout
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		tunnel.ID, tunnel.Name, tunnel.Description, tunnel.Enabled, tunnel.CreatedAt, tunnel.UpdatedAt, tunnel.CreatedBy,
		tunnel.IngressNodeID, tunnel.EgressNodeID, tunnel.IngressGroupID, tunnel.EgressGroupID,
		tunnel.IngressProtocol, tunnel.EgressProtocol, tunnel.ListenPort, tunnel.TargetAddress, tunnel.TargetPort,
		tunnel.EnableEncryption, tunnel.RateLimitBPS, tunnel.MaxConnections, tunnel.IdleTimeout,
	)

	if err != nil {
		tx.Rollback()
		s.logger.Error("插入隧道失败",
			zap.String("name", tunnel.Name),
			zap.Error(err))
		return fmt.Errorf("插入隧道失败: %w", err)
	}

	// 创建对应的规则
	rule := &model.Rule{
		Name:             tunnel.Name,
		Description:      tunnel.Description,
		Enabled:          tunnel.Enabled,
		Protocol:         "tcp", // 默认使用TCP协议
		ListenPort:       tunnel.ListenPort,
		TargetAddress:    tunnel.TargetAddress,
		TargetPort:       tunnel.TargetPort,
		IngressNodeID:    tunnel.IngressNodeID,
		EgressNodeID:     tunnel.EgressNodeID,
		IngressGroupID:   tunnel.IngressGroupID,
		EgressGroupID:    tunnel.EgressGroupID,
		IngressProtocol:  tunnel.IngressProtocol,
		EgressProtocol:   tunnel.EgressProtocol,
		EnableEncryption: tunnel.EnableEncryption,
		RateLimitBPS:     tunnel.RateLimitBPS,
		MaxConnections:   tunnel.MaxConnections,
		IdleTimeout:      tunnel.IdleTimeout,
		CreatedBy:        userID,
	}

	// 生成规则ID
	rule.ID = uuid.New().String()

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
		rule.ID, rule.Name, rule.Description, rule.Enabled, 0, 1, now, now, rule.CreatedBy,
		rule.Protocol, rule.ListenPort, rule.TargetAddress, rule.TargetPort,
		rule.IngressNodeID, rule.EgressNodeID, rule.IngressGroupID, rule.EgressGroupID,
		rule.IngressProtocol, rule.EgressProtocol, rule.EnableEncryption,
		rule.RateLimitBPS, rule.MaxConnections, rule.IdleTimeout,
		0, 0, 0, now,
	)

	if err != nil {
		tx.Rollback()
		s.logger.Error("插入规则失败",
			zap.String("name", rule.Name),
			zap.Error(err))
		return fmt.Errorf("插入规则失败: %w", err)
	}

	// 关联隧道和规则
	_, err = tx.Exec(`
		INSERT INTO tunnel_rules (tunnel_id, rule_id) VALUES (?, ?)
	`, tunnel.ID, rule.ID)

	if err != nil {
		tx.Rollback()
		s.logger.Error("关联隧道和规则失败",
			zap.String("tunnel_id", tunnel.ID),
			zap.String("rule_id", rule.ID),
			zap.Error(err))
		return fmt.Errorf("关联隧道和规则失败: %w", err)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		s.logger.Error("提交事务失败",
			zap.Error(err))
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.logger.Info("创建隧道成功",
		zap.String("id", tunnel.ID),
		zap.String("name", tunnel.Name),
		zap.String("created_by", userID))

	return nil
}

// GetTunnel 获取隧道
func (s *TunnelService) GetTunnel(id string) (*model.Tunnel, error) {
	var tunnel model.Tunnel

	// 查询隧道
	err := s.db.QueryRow(`
		SELECT 
			id, name, description, enabled, created_at, updated_at, created_by,
			ingress_node_id, egress_node_id, ingress_group_id, egress_group_id,
			ingress_protocol, egress_protocol, listen_port, target_address, target_port,
			enable_encryption, rate_limit_bps, max_connections, idle_timeout
		FROM tunnels 
		WHERE id = ?
	`, id).Scan(
		&tunnel.ID, &tunnel.Name, &tunnel.Description, &tunnel.Enabled, &tunnel.CreatedAt, &tunnel.UpdatedAt, &tunnel.CreatedBy,
		&tunnel.IngressNodeID, &tunnel.EgressNodeID, &tunnel.IngressGroupID, &tunnel.EgressGroupID,
		&tunnel.IngressProtocol, &tunnel.EgressProtocol, &tunnel.ListenPort, &tunnel.TargetAddress, &tunnel.TargetPort,
		&tunnel.EnableEncryption, &tunnel.RateLimitBPS, &tunnel.MaxConnections, &tunnel.IdleTimeout,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("隧道不存在: %s", id)
		}
		s.logger.Error("获取隧道失败",
			zap.String("id", id),
			zap.Error(err))
		return nil, fmt.Errorf("获取隧道失败: %w", err)
	}

	// 查询关联的规则
	rows, err := s.db.Query(`
		SELECT rule_id FROM tunnel_rules WHERE tunnel_id = ?
	`, id)

	if err != nil {
		s.logger.Error("查询隧道规则失败",
			zap.String("id", id),
			zap.Error(err))
		return nil, fmt.Errorf("查询隧道规则失败: %w", err)
	}
	defer rows.Close()

	// 收集规则ID
	for rows.Next() {
		var ruleID string
		if err := rows.Scan(&ruleID); err != nil {
			s.logger.Error("扫描规则ID失败",
				zap.Error(err))
			return nil, fmt.Errorf("扫描规则ID失败: %w", err)
		}

		tunnel.RuleIDs = append(tunnel.RuleIDs, ruleID)
	}

	// 查询统计信息
	if len(tunnel.RuleIDs) > 0 {
		var connectionCount, bytesIn, bytesOut int64
		var lastActive time.Time

		for _, ruleID := range tunnel.RuleIDs {
			var rule model.Rule
			err := s.db.QueryRow(`
				SELECT connection_count, bytes_in, bytes_out, last_active
				FROM rules
				WHERE id = ?
			`, ruleID).Scan(&rule.ConnectionCount, &rule.BytesIn, &rule.BytesOut, &rule.LastActive)

			if err != nil {
				s.logger.Error("查询规则统计信息失败",
					zap.String("rule_id", ruleID),
					zap.Error(err))
				continue
			}

			connectionCount += rule.ConnectionCount
			bytesIn += rule.BytesIn
			bytesOut += rule.BytesOut

			if lastActive.Before(rule.LastActive) {
				lastActive = rule.LastActive
			}
		}

		tunnel.ConnectionCount = connectionCount
		tunnel.BytesIn = bytesIn
		tunnel.BytesOut = bytesOut
		tunnel.LastActive = lastActive
	}

	return &tunnel, nil
}

// UpdateTunnel 更新隧道
func (s *TunnelService) UpdateTunnel(tunnel *model.Tunnel) error {
	// 验证隧道
	if err := s.validateTunnel(tunnel); err != nil {
		return err
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 设置更新时间
	tunnel.UpdatedAt = time.Now()

	// 更新隧道
	_, err = tx.Exec(`
		UPDATE tunnels SET
			name = ?, description = ?, enabled = ?, updated_at = ?,
			ingress_node_id = ?, egress_node_id = ?, ingress_group_id = ?, egress_group_id = ?,
			ingress_protocol = ?, egress_protocol = ?, listen_port = ?, target_address = ?, target_port = ?,
			enable_encryption = ?, rate_limit_bps = ?, max_connections = ?, idle_timeout = ?
		WHERE id = ?
	`,
		tunnel.Name, tunnel.Description, tunnel.Enabled, tunnel.UpdatedAt,
		tunnel.IngressNodeID, tunnel.EgressNodeID, tunnel.IngressGroupID, tunnel.EgressGroupID,
		tunnel.IngressProtocol, tunnel.EgressProtocol, tunnel.ListenPort, tunnel.TargetAddress, tunnel.TargetPort,
		tunnel.EnableEncryption, tunnel.RateLimitBPS, tunnel.MaxConnections, tunnel.IdleTimeout,
		tunnel.ID,
	)

	if err != nil {
		tx.Rollback()
		s.logger.Error("更新隧道失败",
			zap.String("id", tunnel.ID),
			zap.Error(err))
		return fmt.Errorf("更新隧道失败: %w", err)
	}

	// 查询关联的规则
	rows, err := tx.Query(`
		SELECT rule_id FROM tunnel_rules WHERE tunnel_id = ?
	`, tunnel.ID)

	if err != nil {
		tx.Rollback()
		s.logger.Error("查询隧道规则失败",
			zap.String("id", tunnel.ID),
			zap.Error(err))
		return fmt.Errorf("查询隧道规则失败: %w", err)
	}

	// 收集规则ID
	var ruleIDs []string
	for rows.Next() {
		var ruleID string
		if err := rows.Scan(&ruleID); err != nil {
			rows.Close()
			tx.Rollback()
			s.logger.Error("扫描规则ID失败",
				zap.Error(err))
			return fmt.Errorf("扫描规则ID失败: %w", err)
		}

		ruleIDs = append(ruleIDs, ruleID)
	}
	rows.Close()

	// 更新关联的规则
	for _, ruleID := range ruleIDs {
		_, err = tx.Exec(`
			UPDATE rules SET
				name = ?, description = ?, enabled = ?, updated_at = ?,
				protocol = ?, listen_port = ?, target_address = ?, target_port = ?,
				ingress_node_id = ?, egress_node_id = ?, ingress_group_id = ?, egress_group_id = ?,
				ingress_protocol = ?, egress_protocol = ?, enable_encryption = ?,
				rate_limit_bps = ?, max_connections = ?, idle_timeout = ?
			WHERE id = ?
		`,
			tunnel.Name, tunnel.Description, tunnel.Enabled, tunnel.UpdatedAt,
			"tcp", tunnel.ListenPort, tunnel.TargetAddress, tunnel.TargetPort,
			tunnel.IngressNodeID, tunnel.EgressNodeID, tunnel.IngressGroupID, tunnel.EgressGroupID,
			tunnel.IngressProtocol, tunnel.EgressProtocol, tunnel.EnableEncryption,
			tunnel.RateLimitBPS, tunnel.MaxConnections, tunnel.IdleTimeout,
			ruleID,
		)

		if err != nil {
			tx.Rollback()
			s.logger.Error("更新规则失败",
				zap.String("rule_id", ruleID),
				zap.Error(err))
			return fmt.Errorf("更新规则失败: %w", err)
		}
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		s.logger.Error("提交事务失败",
			zap.Error(err))
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.logger.Info("更新隧道成功",
		zap.String("id", tunnel.ID),
		zap.String("name", tunnel.Name))

	return nil
}

// DeleteTunnel 删除隧道
func (s *TunnelService) DeleteTunnel(id string) error {
	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 查询关联的规则
	rows, err := tx.Query(`
		SELECT rule_id FROM tunnel_rules WHERE tunnel_id = ?
	`, id)

	if err != nil {
		tx.Rollback()
		s.logger.Error("查询隧道规则失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("查询隧道规则失败: %w", err)
	}

	// 收集规则ID
	var ruleIDs []string
	for rows.Next() {
		var ruleID string
		if err := rows.Scan(&ruleID); err != nil {
			rows.Close()
			tx.Rollback()
			s.logger.Error("扫描规则ID失败",
				zap.Error(err))
			return fmt.Errorf("扫描规则ID失败: %w", err)
		}

		ruleIDs = append(ruleIDs, ruleID)
	}
	rows.Close()

	// 删除关联的规则
	for _, ruleID := range ruleIDs {
		// 删除ACL规则
		_, err = tx.Exec(`DELETE FROM rule_acls WHERE rule_id = ?`, ruleID)
		if err != nil {
			tx.Rollback()
			s.logger.Error("删除ACL规则失败",
				zap.String("rule_id", ruleID),
				zap.Error(err))
			return fmt.Errorf("删除ACL规则失败: %w", err)
		}

		// 删除选项
		_, err = tx.Exec(`DELETE FROM rule_options WHERE rule_id = ?`, ruleID)
		if err != nil {
			tx.Rollback()
			s.logger.Error("删除规则选项失败",
				zap.String("rule_id", ruleID),
				zap.Error(err))
			return fmt.Errorf("删除规则选项失败: %w", err)
		}

		// 删除规则
		_, err = tx.Exec(`DELETE FROM rules WHERE id = ?`, ruleID)
		if err != nil {
			tx.Rollback()
			s.logger.Error("删除规则失败",
				zap.String("rule_id", ruleID),
				zap.Error(err))
			return fmt.Errorf("删除规则失败: %w", err)
		}
	}

	// 删除隧道规则关联
	_, err = tx.Exec(`DELETE FROM tunnel_rules WHERE tunnel_id = ?`, id)
	if err != nil {
		tx.Rollback()
		s.logger.Error("删除隧道规则关联失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("删除隧道规则关联失败: %w", err)
	}

	// 删除隧道
	_, err = tx.Exec(`DELETE FROM tunnels WHERE id = ?`, id)
	if err != nil {
		tx.Rollback()
		s.logger.Error("删除隧道失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("删除隧道失败: %w", err)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		s.logger.Error("提交事务失败",
			zap.Error(err))
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.logger.Info("删除隧道成功",
		zap.String("id", id))

	return nil
}

// ListTunnels 列出所有隧道
func (s *TunnelService) ListTunnels() ([]*model.Tunnel, error) {
	// 查询隧道
	rows, err := s.db.Query(`
		SELECT 
			id, name, description, enabled, created_at, updated_at, created_by,
			ingress_node_id, egress_node_id, ingress_group_id, egress_group_id,
			ingress_protocol, egress_protocol, listen_port, target_address, target_port,
			enable_encryption, rate_limit_bps, max_connections, idle_timeout
		FROM tunnels
		ORDER BY name ASC
	`)

	if err != nil {
		s.logger.Error("查询隧道失败",
			zap.Error(err))
		return nil, fmt.Errorf("查询隧道失败: %w", err)
	}
	defer rows.Close()

	// 收集隧道
	var tunnels []*model.Tunnel
	for rows.Next() {
		var tunnel model.Tunnel

		if err := rows.Scan(
			&tunnel.ID, &tunnel.Name, &tunnel.Description, &tunnel.Enabled, &tunnel.CreatedAt, &tunnel.UpdatedAt, &tunnel.CreatedBy,
			&tunnel.IngressNodeID, &tunnel.EgressNodeID, &tunnel.IngressGroupID, &tunnel.EgressGroupID,
			&tunnel.IngressProtocol, &tunnel.EgressProtocol, &tunnel.ListenPort, &tunnel.TargetAddress, &tunnel.TargetPort,
			&tunnel.EnableEncryption, &tunnel.RateLimitBPS, &tunnel.MaxConnections, &tunnel.IdleTimeout,
		); err != nil {
			s.logger.Error("扫描隧道失败",
				zap.Error(err))
			return nil, fmt.Errorf("扫描隧道失败: %w", err)
		}

		tunnels = append(tunnels, &tunnel)
	}

	// 查询每个隧道的规则ID和统计信息
	for _, tunnel := range tunnels {
		// 查询规则ID
		ruleRows, err := s.db.Query(`
			SELECT rule_id FROM tunnel_rules WHERE tunnel_id = ?
		`, tunnel.ID)

		if err != nil {
			s.logger.Error("查询隧道规则失败",
				zap.String("id", tunnel.ID),
				zap.Error(err))
			continue
		}

		// 收集规则ID
		for ruleRows.Next() {
			var ruleID string
			if err := ruleRows.Scan(&ruleID); err != nil {
				ruleRows.Close()
				s.logger.Error("扫描规则ID失败",
					zap.Error(err))
				continue
			}

			tunnel.RuleIDs = append(tunnel.RuleIDs, ruleID)
		}
		ruleRows.Close()

		// 查询统计信息
		if len(tunnel.RuleIDs) > 0 {
			var connectionCount, bytesIn, bytesOut int64
			var lastActive time.Time

			for _, ruleID := range tunnel.RuleIDs {
				var rule model.Rule
				err := s.db.QueryRow(`
					SELECT connection_count, bytes_in, bytes_out, last_active
					FROM rules
					WHERE id = ?
				`, ruleID).Scan(&rule.ConnectionCount, &rule.BytesIn, &rule.BytesOut, &rule.LastActive)

				if err != nil {
					s.logger.Error("查询规则统计信息失败",
						zap.String("rule_id", ruleID),
						zap.Error(err))
					continue
				}

				connectionCount += rule.ConnectionCount
				bytesIn += rule.BytesIn
				bytesOut += rule.BytesOut

				if lastActive.Before(rule.LastActive) {
					lastActive = rule.LastActive
				}
			}

			tunnel.ConnectionCount = connectionCount
			tunnel.BytesIn = bytesIn
			tunnel.BytesOut = bytesOut
			tunnel.LastActive = lastActive
		}
	}

	return tunnels, nil
}

// validateTunnel 验证隧道
func (s *TunnelService) validateTunnel(tunnel *model.Tunnel) error {
	if tunnel.Name == "" {
		return errors.New("隧道名称不能为空")
	}

	if tunnel.ListenPort <= 0 || tunnel.ListenPort > 65535 {
		return errors.New("监听端口必须在1-65535之间")
	}

	if tunnel.TargetAddress == "" {
		return errors.New("目标地址不能为空")
	}

	if tunnel.TargetPort <= 0 || tunnel.TargetPort > 65535 {
		return errors.New("目标端口必须在1-65535之间")
	}

	// 入口节点或入口组必须指定一个
	if tunnel.IngressNodeID == "" && tunnel.IngressGroupID == "" {
		return errors.New("必须指定入口节点或入口组")
	}

	// 如果指定了入口组，检查是否存在
	if tunnel.IngressGroupID != "" {
		_, err := s.nodeGroupService.GetNodeGroup(tunnel.IngressGroupID)
		if err != nil {
			return fmt.Errorf("入口组不存在: %w", err)
		}
	}

	// 如果指定了出口组，检查是否存在
	if tunnel.EgressGroupID != "" {
		_, err := s.nodeGroupService.GetNodeGroup(tunnel.EgressGroupID)
		if err != nil {
			return fmt.Errorf("出口组不存在: %w", err)
		}
	}

	// 如果指定了入口节点，检查是否存在
	if tunnel.IngressNodeID != "" {
		_, err := s.nodeService.GetNode(tunnel.IngressNodeID)
		if err != nil {
			return fmt.Errorf("入口节点不存在: %w", err)
		}
	}

	// 如果指定了出口节点，检查是否存在
	if tunnel.EgressNodeID != "" {
		_, err := s.nodeService.GetNode(tunnel.EgressNodeID)
		if err != nil {
			return fmt.Errorf("出口节点不存在: %w", err)
		}
	}

	return nil
}

// ProbeTunnel 探测隧道
func (s *TunnelService) ProbeTunnel(tunnelID string) (*model.ProbeResult, error) {
	// 获取隧道
	_, err := s.GetTunnel(tunnelID)
	if err != nil {
		return nil, err
	}

	// TODO: 实现探测功能

	// 返回模拟结果
	result := &model.ProbeResult{
		TunnelID:     tunnelID,
		Success:      true,
		Message:      "探测成功",
		ResponseTime: 100, // 毫秒
		Timestamp:    time.Now(),
	}

	return result, nil
}
