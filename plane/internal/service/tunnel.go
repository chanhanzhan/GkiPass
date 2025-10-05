package service

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"gkipass/plane/db"
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TunnelService 隧道服务
type TunnelService struct {
	db          *db.Manager
	planService *PlanService
	portManager *PortManager
}

// NewTunnelService 创建隧道服务
func NewTunnelService(dbManager *db.Manager, planService *PlanService) *TunnelService {
	return &TunnelService{
		db:          dbManager,
		planService: planService,
		portManager: GetPortManager(dbManager),
	}
}

// ValidateTunnelTargets 验证隧道目标
func ValidateTunnelTargets(targets []dbinit.TunnelTarget) error {
	if len(targets) == 0 {
		return fmt.Errorf("至少需要一个目标")
	}

	if len(targets) > 10 {
		return fmt.Errorf("最多支持10个目标")
	}

	for i, target := range targets {
		// 验证主机
		if target.Host == "" {
			return fmt.Errorf("目标 %d: 主机不能为空", i+1)
		}

		// 验证端口
		if target.Port < 1 || target.Port > 65535 {
			return fmt.Errorf("目标 %d: 端口必须在 1-65535 范围内", i+1)
		}

		// 验证主机格式（域名或IP）
		if net.ParseIP(target.Host) == nil {
			// 不是IP，检查是否是有效域名
			if !isValidDomain(target.Host) {
				return fmt.Errorf("目标 %d: 无效的域名或IP: %s", i+1, target.Host)
			}
		}

		// 权重默认为1
		if target.Weight <= 0 {
			target.Weight = 1
		}
	}

	return nil
}

// isValidDomain 验证域名格式
func isValidDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}

	// 简单的域名验证
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 || len(part) > 63 {
			return false
		}
	}

	return true
}

// ParseTargetsFromString 从字符串解析目标列表
func ParseTargetsFromString(targetsStr string) ([]dbinit.TunnelTarget, error) {
	if targetsStr == "" {
		return nil, fmt.Errorf("目标列表为空")
	}

	// 尝试JSON解析
	var targets []dbinit.TunnelTarget
	if err := json.Unmarshal([]byte(targetsStr), &targets); err == nil {
		return targets, nil
	}

	// 兼容旧格式：host:port
	parts := strings.SplitN(targetsStr, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("无效的目标格式")
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("无效的端口: %s", parts[1])
	}

	return []dbinit.TunnelTarget{{
		Host:   parts[0],
		Port:   port,
		Weight: 1,
	}}, nil
}

// TargetsToString 将目标列表转换为字符串
func TargetsToString(targets []dbinit.TunnelTarget) (string, error) {
	data, err := json.Marshal(targets)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CreateTunnel 创建隧道
func (s *TunnelService) CreateTunnel(tunnel *dbinit.Tunnel, targets []dbinit.TunnelTarget) error {
	// 1. 检查规则数配额
	if err := s.planService.CheckQuota(tunnel.UserID, "rules"); err != nil {
		return fmt.Errorf("配额不足: %w", err)
	}

	// 2. 验证端口
	if err := ValidatePort(tunnel.LocalPort); err != nil {
		return err
	}

	// 3. 检查端口在入口组内是否可用
	if !s.portManager.IsPortAvailableInGroup(tunnel.EntryGroupID, tunnel.LocalPort, "") {
		return fmt.Errorf("端口 %d 已被同组内其他隧道占用", tunnel.LocalPort)
	}

	// 4. 验证目标列表
	if err := ValidateTunnelTargets(targets); err != nil {
		return err
	}

	// 5. 验证入口组和出口组
	entryGroup, err := s.db.DB.SQLite.GetNodeGroup(tunnel.EntryGroupID)
	if err != nil || entryGroup == nil {
		return fmt.Errorf("入口组不存在")
	}
	if entryGroup.Type != "entry" {
		return fmt.Errorf("入口组类型错误")
	}

	exitGroup, err := s.db.DB.SQLite.GetNodeGroup(tunnel.ExitGroupID)
	if err != nil || exitGroup == nil {
		return fmt.Errorf("出口组不存在")
	}
	if exitGroup.Type != "exit" {
		return fmt.Errorf("出口组类型错误")
	}

	// 6. 转换目标为JSON
	targetsStr, err := TargetsToString(targets)
	if err != nil {
		return fmt.Errorf("目标序列化失败: %w", err)
	}

	// 7. 创建隧道
	tunnel.ID = uuid.New().String()
	tunnel.Targets = targetsStr
	tunnel.CreatedAt = time.Now()
	tunnel.UpdatedAt = time.Now()
	tunnel.TrafficIn = 0
	tunnel.TrafficOut = 0
	tunnel.ConnectionCount = 0

	if err := s.db.DB.SQLite.CreateTunnel(tunnel); err != nil {
		logger.Error("创建隧道失败",
			zap.String("name", tunnel.Name),
			zap.Error(err))
		return err
	}

	// 8. 占用端口（在入口组内）
	if err := s.portManager.OccupyPortInGroup(tunnel.EntryGroupID, tunnel.LocalPort, tunnel.ID); err != nil {
		// 回滚：删除已创建的隧道
		_ = s.db.DB.SQLite.DeleteTunnel(tunnel.ID)
		return err
	}

	// 9. 增加规则计数
	if err := s.planService.IncrementRuleCount(tunnel.UserID); err != nil {
		logger.Warn("增加规则计数失败", zap.Error(err))
	}

	logger.Info("隧道已创建",
		zap.String("id", tunnel.ID),
		zap.String("name", tunnel.Name),
		zap.Int("port", tunnel.LocalPort),
		zap.String("userID", tunnel.UserID),
		zap.Int("targetCount", len(targets)))

	return nil
}

// GetTunnel 获取隧道
func (s *TunnelService) GetTunnel(id string) (*dbinit.Tunnel, error) {
	tunnel, err := s.db.DB.SQLite.GetTunnel(id)
	if err != nil {
		logger.Error("获取隧道失败",
			zap.String("id", id),
			zap.Error(err))
		return nil, err
	}
	return tunnel, nil
}

// ListTunnels 列出隧道
func (s *TunnelService) ListTunnels(userID string, enabled *bool) ([]*dbinit.Tunnel, error) {
	tunnels, err := s.db.DB.SQLite.ListTunnels(userID, enabled)
	if err != nil {
		logger.Error("列出隧道失败",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, err
	}
	return tunnels, nil
}

// UpdateTunnel 更新隧道
func (s *TunnelService) UpdateTunnel(tunnel *dbinit.Tunnel, targets []dbinit.TunnelTarget) error {
	// 获取原隧道信息
	oldTunnel, err := s.GetTunnel(tunnel.ID)
	if err != nil {
		return err
	}

	// 如果端口改变了，需要检查新端口是否可用
	if tunnel.LocalPort != oldTunnel.LocalPort {
		if err := ValidatePort(tunnel.LocalPort); err != nil {
			return err
		}

		// 检查新端口在组内是否可用（排除当前隧道）
		if !s.portManager.IsPortAvailableInGroup(tunnel.EntryGroupID, tunnel.LocalPort, tunnel.ID) {
			return fmt.Errorf("端口 %d 已被同组内其他隧道占用", tunnel.LocalPort)
		}
	}

	// 验证目标列表
	if err := ValidateTunnelTargets(targets); err != nil {
		return err
	}

	// 转换目标为JSON
	targetsStr, err := TargetsToString(targets)
	if err != nil {
		return fmt.Errorf("目标序列化失败: %w", err)
	}

	tunnel.Targets = targetsStr
	tunnel.UpdatedAt = time.Now()

	if err := s.db.DB.SQLite.UpdateTunnel(tunnel); err != nil {
		logger.Error("更新隧道失败",
			zap.String("id", tunnel.ID),
			zap.Error(err))
		return err
	}

	// 如果端口改变了，更新端口占用
	if tunnel.LocalPort != oldTunnel.LocalPort {
		s.portManager.ReleasePortInGroup(oldTunnel.EntryGroupID, oldTunnel.LocalPort)
		if err := s.portManager.OccupyPortInGroup(tunnel.EntryGroupID, tunnel.LocalPort, tunnel.ID); err != nil {
			logger.Error("更新端口占用失败", zap.Error(err))
		}
	}

	logger.Info("隧道已更新",
		zap.String("id", tunnel.ID),
		zap.String("name", tunnel.Name),
		zap.Int("port", tunnel.LocalPort))

	return nil
}

// DeleteTunnel 删除隧道
func (s *TunnelService) DeleteTunnel(id, userID string) error {
	tunnel, err := s.GetTunnel(id)
	if err != nil {
		return err
	}

	if err := s.db.DB.SQLite.DeleteTunnel(id); err != nil {
		logger.Error("删除隧道失败",
			zap.String("id", id),
			zap.Error(err))
		return err
	}

	// 释放端口
	s.portManager.ReleasePortInGroup(tunnel.EntryGroupID, tunnel.LocalPort)

	// 减少规则计数
	if err := s.planService.DecrementRuleCount(userID); err != nil {
		logger.Warn("减少规则计数失败", zap.Error(err))
	}

	logger.Info("隧道已删除",
		zap.String("id", id),
		zap.String("name", tunnel.Name),
		zap.Int("port", tunnel.LocalPort))

	return nil
}

// ToggleTunnel 切换隧道状态
func (s *TunnelService) ToggleTunnel(id string, enabled bool) error {
	tunnel, err := s.GetTunnel(id)
	if err != nil {
		return err
	}

	// 如果要启用隧道，需要检查配额和端口
	if enabled && !tunnel.Enabled {
		// 检查用户订阅是否有效
		if _, _, err := s.planService.GetUserSubscription(tunnel.UserID); err != nil {
			return fmt.Errorf("无法启用隧道: %w", err)
		}

		// 检查端口在组内是否仍然可用
		if !s.portManager.IsPortAvailableInGroup(tunnel.EntryGroupID, tunnel.LocalPort, tunnel.ID) {
			return fmt.Errorf("端口 %d 已被同组内其他隧道占用", tunnel.LocalPort)
		}

		// 占用端口
		if err := s.portManager.OccupyPortInGroup(tunnel.EntryGroupID, tunnel.LocalPort, tunnel.ID); err != nil {
			return err
		}
	}

	// 如果要禁用隧道，释放端口
	if !enabled && tunnel.Enabled {
		s.portManager.ReleasePortInGroup(tunnel.EntryGroupID, tunnel.LocalPort)
	}

	tunnel.Enabled = enabled
	tunnel.UpdatedAt = time.Now()

	if err := s.db.DB.SQLite.UpdateTunnel(tunnel); err != nil {
		return err
	}

	status := "禁用"
	if enabled {
		status = "启用"
	}

	logger.Info("隧道状态已切换",
		zap.String("id", id),
		zap.Int("port", tunnel.LocalPort),
		zap.String("status", status))

	return nil
}

// UpdateTraffic 更新流量统计
func (s *TunnelService) UpdateTraffic(id string, trafficIn, trafficOut int64) error {
	tunnel, err := s.GetTunnel(id)
	if err != nil {
		return err
	}

	tunnel.TrafficIn += trafficIn
	tunnel.TrafficOut += trafficOut
	tunnel.UpdatedAt = time.Now()

	if err := s.db.DB.SQLite.UpdateTunnel(tunnel); err != nil {
		return err
	}

	// 更新用户流量统计
	totalTraffic := trafficIn + trafficOut
	if err := s.planService.AddTrafficUsage(tunnel.UserID, totalTraffic); err != nil {
		logger.Warn("更新用户流量失败", zap.Error(err))
	}

	return nil
}

// GetTunnelsByGroupID 获取节点组的所有隧道
func (s *TunnelService) GetTunnelsByGroupID(groupID string, groupType string) ([]*dbinit.Tunnel, error) {
	tunnels, err := s.db.DB.SQLite.GetTunnelsByGroupID(groupID, groupType)
	if err != nil {
		logger.Error("获取节点组隧道失败",
			zap.String("groupID", groupID),
			zap.Error(err))
		return nil, err
	}
	return tunnels, nil
}
