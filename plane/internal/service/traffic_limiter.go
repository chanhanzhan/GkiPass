package service

import (
	"fmt"
	"sync"
	"time"

	"gkipass/plane/db"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// TrafficLimiter 流量和带宽限制器
type TrafficLimiter struct {
	db          *db.Manager
	userQuotas  map[string]*UserQuota
	mu          sync.RWMutex
	planService *PlanService
}

// UserQuota 用户配额
type UserQuota struct {
	UserID         string
	TrafficUsed    int64
	TrafficLimit   int64
	BandwidthLimit int64
	LastUpdate     time.Time
}

// NewTrafficLimiter 创建流量限制器
func NewTrafficLimiter(dbManager *db.Manager) *TrafficLimiter {
	return &TrafficLimiter{
		db:          dbManager,
		userQuotas:  make(map[string]*UserQuota),
		planService: NewPlanService(dbManager),
	}
}

// CheckTrafficQuota 检查流量配额
func (tl *TrafficLimiter) CheckTrafficQuota(userID string, bytes int64) error {
	tl.mu.RLock()
	quota, exists := tl.userQuotas[userID]
	tl.mu.RUnlock()

	if !exists {
		// 加载用户配额
		quota = tl.loadUserQuota(userID)
		if quota == nil {
			return fmt.Errorf("无法获取用户配额")
		}
	}

	// 检查流量限制（0表示无限）
	if quota.TrafficLimit > 0 && quota.TrafficUsed+bytes > quota.TrafficLimit {
		return fmt.Errorf("流量配额不足: 已用 %d/%d 字节", quota.TrafficUsed, quota.TrafficLimit)
	}

	return nil
}

// AddTraffic 增加流量使用
func (tl *TrafficLimiter) AddTraffic(userID string, bytes int64) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	quota, exists := tl.userQuotas[userID]
	if !exists {
		quota = tl.loadUserQuota(userID)
		if quota == nil {
			return fmt.Errorf("无法获取用户配额")
		}
		tl.userQuotas[userID] = quota
	}

	quota.TrafficUsed += bytes
	quota.LastUpdate = time.Now()

	// 异步更新数据库
	go tl.planService.AddTrafficUsage(userID, bytes)

	logger.Debug("流量使用已更新",
		zap.String("userID", userID),
		zap.Int64("added", bytes),
		zap.Int64("total", quota.TrafficUsed))

	return nil
}

// loadUserQuota 加载用户配额
func (tl *TrafficLimiter) loadUserQuota(userID string) *UserQuota {
	sub, plan, err := tl.planService.GetUserSubscription(userID)
	if err != nil {
		return nil
	}

	return &UserQuota{
		UserID:         userID,
		TrafficUsed:    sub.UsedTraffic,
		TrafficLimit:   plan.MaxTraffic,
		BandwidthLimit: plan.MaxBandwidth,
		LastUpdate:     time.Now(),
	}
}

// CheckBandwidth 检查带宽限制
func (tl *TrafficLimiter) CheckBandwidth(userID string, bps int64) error {
	tl.mu.RLock()
	quota, exists := tl.userQuotas[userID]
	tl.mu.RUnlock()

	if !exists {
		quota = tl.loadUserQuota(userID)
	}

	if quota != nil && quota.BandwidthLimit > 0 && bps > quota.BandwidthLimit {
		return fmt.Errorf("带宽超限: %d > %d bps", bps, quota.BandwidthLimit)
	}

	return nil
}

// RefreshQuota 刷新用户配额
func (tl *TrafficLimiter) RefreshQuota(userID string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	delete(tl.userQuotas, userID)
	logger.Info("用户配额已刷新", zap.String("userID", userID))
}
