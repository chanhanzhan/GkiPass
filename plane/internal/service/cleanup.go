package service

import (
	"time"

	"gkipass/plane/db"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// CleanupService 清理服务（定时任务）
type CleanupService struct {
	db          *db.Manager
	planService *PlanService
	portManager *PortManager
	stopChan    chan struct{}
}

// NewCleanupService 创建清理服务
func NewCleanupService(dbManager *db.Manager) *CleanupService {
	return &CleanupService{
		db:          dbManager,
		planService: NewPlanService(dbManager),
		portManager: GetPortManager(dbManager),
		stopChan:    make(chan struct{}),
	}
}

// Start 启动清理服务
func (s *CleanupService) Start() {
	s.cleanupExpiredSubscriptions()
	s.cleanupInactiveTunnels()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runCleanup()
		case <-s.stopChan:
			return
		}
	}
}

// Stop 停止清理服务
func (s *CleanupService) Stop() {
	close(s.stopChan)
}

// runCleanup 执行清理
func (s *CleanupService) runCleanup() {
	logger.Debug("执行定时清理任务")

	// 1. 清理过期订阅
	s.cleanupExpiredSubscriptions()

	// 2. 清理无效隧道
	s.cleanupInactiveTunnels()

	// 3. 重置流量（如果到达重置时间）
	s.resetTrafficIfNeeded()
}

// cleanupExpiredSubscriptions 清理过期订阅
func (s *CleanupService) cleanupExpiredSubscriptions() {
	logger.Debug("检查过期订阅...")

	// 查询所有已过期但状态仍为active的订阅
	expiredSubs, err := s.db.DB.SQLite.ListExpiredSubscriptions()
	if err != nil {
		logger.Error("查询过期订阅失败", zap.Error(err))
		return
	}

	if len(expiredSubs) == 0 {
		return
	}

	logger.Info("发现过期订阅", zap.Int("count", len(expiredSubs)))

	// 处理每个过期订阅
	for _, sub := range expiredSubs {
		// 更新订阅状态为过期
		sub.Status = "expired"
		if err := s.db.DB.SQLite.UpdateSubscription(sub); err != nil {
			logger.Error("更新订阅状态失败",
				zap.String("subID", sub.ID),
				zap.Error(err))
			continue
		}

		// 禁用用户的所有隧道
		s.disableUserTunnels(sub.UserID)

		logger.Info("订阅已过期",
			zap.String("userID", sub.UserID),
			zap.String("planID", sub.PlanID))
	}
}

// cleanupInactiveTunnels 清理无效隧道（套餐到期/流量用尽）
func (s *CleanupService) cleanupInactiveTunnels() {
	logger.Debug("检查无效隧道...")

	// 获取所有启用的隧道
	enabled := true
	tunnels, err := s.db.DB.SQLite.ListTunnels("", &enabled)
	if err != nil {
		logger.Error("获取隧道列表失败", zap.Error(err))
		return
	}

	disabledCount := 0
	for _, tunnel := range tunnels {
		// 检查用户订阅状态
		sub, plan, err := s.planService.GetUserSubscription(tunnel.UserID)
		if err != nil {
			// 用户没有有效订阅，禁用隧道
			s.disableTunnel(tunnel.ID, "用户无有效订阅")
			disabledCount++
			continue
		}

		// 检查套餐是否过期
		if time.Now().After(sub.EndDate) {
			s.disableTunnel(tunnel.ID, "套餐已过期")
			disabledCount++
			continue
		}

		// 检查流量是否用尽
		if plan.MaxTraffic > 0 && sub.UsedTraffic >= plan.MaxTraffic {
			s.disableTunnel(tunnel.ID, "流量已用尽")
			disabledCount++
			continue
		}
	}

	if disabledCount > 0 {
		logger.Info("已禁用无效隧道", zap.Int("count", disabledCount))
	}
}

// disableTunnel 禁用隧道并释放端口
func (s *CleanupService) disableTunnel(tunnelID, reason string) {
	tunnel, err := s.db.DB.SQLite.GetTunnel(tunnelID)
	if err != nil || tunnel == nil {
		return
	}

	// 禁用隧道
	tunnel.Enabled = false
	tunnel.UpdatedAt = time.Now()

	if err := s.db.DB.SQLite.UpdateTunnel(tunnel); err != nil {
		logger.Error("禁用隧道失败",
			zap.String("tunnelID", tunnelID),
			zap.Error(err))
		return
	}

	// 释放端口
	s.portManager.ReleasePortInGroup(tunnel.EntryGroupID, tunnel.LocalPort)

	logger.Info("隧道已自动禁用",
		zap.String("tunnelID", tunnelID),
		zap.String("tunnelName", tunnel.Name),
		zap.Int("port", tunnel.LocalPort),
		zap.String("reason", reason))
}

// disableUserTunnels 禁用用户的所有隧道
func (s *CleanupService) disableUserTunnels(userID string) {
	tunnels, err := s.db.DB.SQLite.ListTunnels(userID, nil)
	if err != nil {
		logger.Error("获取用户隧道失败", zap.String("userID", userID), zap.Error(err))
		return
	}

	for _, tunnel := range tunnels {
		if tunnel.Enabled {
			s.disableTunnel(tunnel.ID, "用户套餐到期")
		}
	}
}

// resetTrafficIfNeeded 重置流量（如果到达重置时间）
func (s *CleanupService) resetTrafficIfNeeded() {
	logger.Debug("检查流量重置...")
}
