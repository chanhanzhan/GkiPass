package service

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gkipass/plane/internal/model"
)

// ACLService ACL服务
type ACLService struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewACLService 创建ACL服务
func NewACLService(db *sql.DB) *ACLService {
	return &ACLService{
		db:     db,
		logger: zap.L().Named("acl-service"),
	}
}

// CreateACLRule 创建ACL规则
func (s *ACLService) CreateACLRule(acl *model.ACLRule) error {
	// 验证ACL规则
	if err := s.validateACLRule(acl); err != nil {
		return err
	}

	// 生成ID
	if acl.ID == "" {
		acl.ID = uuid.New().String()
	}

	// 插入ACL规则
	_, err := s.db.Exec(`
		INSERT INTO rule_acls (id, rule_id, action, priority, source_ip, dest_ip, protocol, port_range)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		acl.ID, acl.RuleID, acl.Action, acl.Priority, acl.SourceIP, acl.DestIP, acl.Protocol, acl.PortRange,
	)

	if err != nil {
		s.logger.Error("创建ACL规则失败",
			zap.String("rule_id", acl.RuleID),
			zap.Error(err))
		return fmt.Errorf("创建ACL规则失败: %w", err)
	}

	s.logger.Info("创建ACL规则成功",
		zap.String("id", acl.ID),
		zap.String("rule_id", acl.RuleID),
		zap.String("action", acl.Action))

	return nil
}

// UpdateACLRule 更新ACL规则
func (s *ACLService) UpdateACLRule(acl *model.ACLRule) error {
	// 验证ACL规则
	if err := s.validateACLRule(acl); err != nil {
		return err
	}

	// 更新ACL规则
	_, err := s.db.Exec(`
		UPDATE rule_acls SET
			action = ?,
			priority = ?,
			source_ip = ?,
			dest_ip = ?,
			protocol = ?,
			port_range = ?
		WHERE id = ? AND rule_id = ?
	`,
		acl.Action, acl.Priority, acl.SourceIP, acl.DestIP, acl.Protocol, acl.PortRange,
		acl.ID, acl.RuleID,
	)

	if err != nil {
		s.logger.Error("更新ACL规则失败",
			zap.String("id", acl.ID),
			zap.Error(err))
		return fmt.Errorf("更新ACL规则失败: %w", err)
	}

	s.logger.Info("更新ACL规则成功",
		zap.String("id", acl.ID),
		zap.String("rule_id", acl.RuleID))

	return nil
}

// DeleteACLRule 删除ACL规则
func (s *ACLService) DeleteACLRule(id, ruleID string) error {
	// 删除ACL规则
	_, err := s.db.Exec(`DELETE FROM rule_acls WHERE id = ? AND rule_id = ?`, id, ruleID)
	if err != nil {
		s.logger.Error("删除ACL规则失败",
			zap.String("id", id),
			zap.String("rule_id", ruleID),
			zap.Error(err))
		return fmt.Errorf("删除ACL规则失败: %w", err)
	}

	s.logger.Info("删除ACL规则成功",
		zap.String("id", id),
		zap.String("rule_id", ruleID))

	return nil
}

// GetACLRules 获取规则的ACL规则
func (s *ACLService) GetACLRules(ruleID string) ([]model.ACLRule, error) {
	// 查询ACL规则
	rows, err := s.db.Query(`
		SELECT id, rule_id, action, priority, source_ip, dest_ip, protocol, port_range
		FROM rule_acls
		WHERE rule_id = ?
		ORDER BY priority DESC
	`, ruleID)

	if err != nil {
		s.logger.Error("查询ACL规则失败",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		return nil, fmt.Errorf("查询ACL规则失败: %w", err)
	}
	defer rows.Close()

	// 收集ACL规则
	var rules []model.ACLRule
	for rows.Next() {
		var rule model.ACLRule

		if err := rows.Scan(
			&rule.ID, &rule.RuleID, &rule.Action, &rule.Priority,
			&rule.SourceIP, &rule.DestIP, &rule.Protocol, &rule.PortRange,
		); err != nil {
			s.logger.Error("扫描ACL规则失败",
				zap.Error(err))
			return nil, fmt.Errorf("扫描ACL规则失败: %w", err)
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// validateACLRule 验证ACL规则
func (s *ACLService) validateACLRule(acl *model.ACLRule) error {
	if acl.RuleID == "" {
		return errors.New("规则ID不能为空")
	}

	if acl.Action != "allow" && acl.Action != "deny" {
		return errors.New("动作必须为allow或deny")
	}

	// 验证源IP
	if acl.SourceIP != "" {
		if err := validateCIDR(acl.SourceIP); err != nil {
			return fmt.Errorf("无效的源IP: %w", err)
		}
	}

	// 验证目标IP
	if acl.DestIP != "" {
		if err := validateCIDR(acl.DestIP); err != nil {
			return fmt.Errorf("无效的目标IP: %w", err)
		}
	}

	// 验证协议
	if acl.Protocol != "" && !isValidProtocol(acl.Protocol) {
		return errors.New("无效的协议")
	}

	// 验证端口范围
	if acl.PortRange != "" {
		if err := validatePortRange(acl.PortRange); err != nil {
			return fmt.Errorf("无效的端口范围: %w", err)
		}
	}

	return nil
}

// validateCIDR 验证CIDR
func validateCIDR(cidr string) error {
	// 支持单个IP
	if strings.IndexByte(cidr, '/') == -1 {
		ip := net.ParseIP(cidr)
		if ip == nil {
			return errors.New("无效的IP地址")
		}
		return nil
	}

	// 验证CIDR
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	return nil
}

// isValidProtocol 验证协议
func isValidProtocol(protocol string) bool {
	validProtocols := map[string]bool{
		"tcp":  true,
		"udp":  true,
		"icmp": true,
		"any":  true,
	}

	return validProtocols[strings.ToLower(protocol)]
}

// validatePortRange 验证端口范围
func validatePortRange(portRange string) error {
	// 单个端口
	if strings.IndexByte(portRange, '-') == -1 {
		port, err := strconv.Atoi(portRange)
		if err != nil {
			return errors.New("无效的端口")
		}

		if port < 1 || port > 65535 {
			return errors.New("端口必须在1-65535之间")
		}

		return nil
	}

	// 端口范围
	parts := strings.Split(portRange, "-")
	if len(parts) != 2 {
		return errors.New("无效的端口范围格式")
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return errors.New("无效的起始端口")
	}

	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return errors.New("无效的结束端口")
	}

	if start < 1 || start > 65535 || end < 1 || end > 65535 || start > end {
		return errors.New("端口范围必须在1-65535之间，且起始端口不能大于结束端口")
	}

	return nil
}

// CheckAccess 检查访问权限
func (s *ACLService) CheckAccess(ruleID, sourceIP, destIP, protocol string, port int) (bool, error) {
	// 获取规则的ACL规则
	rules, err := s.GetACLRules(ruleID)
	if err != nil {
		return false, err
	}

	// 如果没有ACL规则，默认允许
	if len(rules) == 0 {
		return true, nil
	}

	// 检查每个规则
	for _, rule := range rules {
		// 检查源IP
		if rule.SourceIP != "" && !matchIP(sourceIP, rule.SourceIP) {
			continue
		}

		// 检查目标IP
		if rule.DestIP != "" && !matchIP(destIP, rule.DestIP) {
			continue
		}

		// 检查协议
		if rule.Protocol != "" && rule.Protocol != "any" && !matchProtocol(protocol, rule.Protocol) {
			continue
		}

		// 检查端口
		if rule.PortRange != "" && !matchPort(port, rule.PortRange) {
			continue
		}

		// 规则匹配，返回动作
		return rule.Action == "allow", nil
	}

	// 默认允许
	return true, nil
}

// matchIP 检查IP是否匹配
func matchIP(ip, cidr string) bool {
	// 如果CIDR是单个IP
	if strings.IndexByte(cidr, '/') == -1 {
		return ip == cidr
	}

	// 检查IP是否在CIDR范围内
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	return ipNet.Contains(net.ParseIP(ip))
}

// matchProtocol 检查协议是否匹配
func matchProtocol(protocol, ruleProtocol string) bool {
	return strings.ToLower(protocol) == strings.ToLower(ruleProtocol)
}

// matchPort 检查端口是否匹配
func matchPort(port int, portRange string) bool {
	// 单个端口
	if strings.IndexByte(portRange, '-') == -1 {
		rulePort, err := strconv.Atoi(portRange)
		if err != nil {
			return false
		}

		return port == rulePort
	}

	// 端口范围
	parts := strings.Split(portRange, "-")
	if len(parts) != 2 {
		return false
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}

	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}

	return port >= start && port <= end
}
