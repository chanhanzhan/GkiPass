package service

import (
	"fmt"
	"time"

	"gkipass/plane/db"
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PlanService 套餐服务
type PlanService struct {
	db *db.Manager
}

// NewPlanService 创建套餐服务
func NewPlanService(dbManager *db.Manager) *PlanService {
	return &PlanService{db: dbManager}
}

// CreatePlan 创建套餐
func (s *PlanService) CreatePlan(plan *dbinit.Plan) error {
	plan.ID = uuid.New().String()
	plan.CreatedAt = time.Now()
	plan.UpdatedAt = time.Now()

	if err := s.db.DB.SQLite.CreatePlan(plan); err != nil {
		logger.Error("创建套餐失败",
			zap.String("name", plan.Name),
			zap.Error(err))
		return err
	}

	logger.Info("套餐已创建",
		zap.String("id", plan.ID),
		zap.String("name", plan.Name))

	return nil
}

// GetPlan 获取套餐
func (s *PlanService) GetPlan(id string) (*dbinit.Plan, error) {
	plan, err := s.db.DB.SQLite.GetPlan(id)
	if err != nil {
		logger.Error("获取套餐失败",
			zap.String("id", id),
			zap.Error(err))
		return nil, err
	}
	return plan, nil
}

// ListPlans 列出套餐
func (s *PlanService) ListPlans(enabled *bool) ([]*dbinit.Plan, error) {
	plans, err := s.db.DB.SQLite.ListPlans(enabled)
	if err != nil {
		logger.Error("列出套餐失败", zap.Error(err))
		return nil, err
	}
	return plans, nil
}

// UpdatePlan 更新套餐
func (s *PlanService) UpdatePlan(plan *dbinit.Plan) error {
	plan.UpdatedAt = time.Now()

	if err := s.db.DB.SQLite.UpdatePlan(plan); err != nil {
		logger.Error("更新套餐失败",
			zap.String("id", plan.ID),
			zap.Error(err))
		return err
	}

	logger.Info("套餐已更新",
		zap.String("id", plan.ID),
		zap.String("name", plan.Name))

	return nil
}

// DeletePlan 删除套餐
func (s *PlanService) DeletePlan(id string) error {
	// 检查是否有用户正在使用
	subs, err := s.db.DB.SQLite.GetSubscriptionsByPlanID(id)
	if err != nil {
		return err
	}

	if len(subs) > 0 {
		return fmt.Errorf("套餐正在被 %d 个用户使用，无法删除", len(subs))
	}

	if err := s.db.DB.SQLite.DeletePlan(id); err != nil {
		logger.Error("删除套餐失败",
			zap.String("id", id),
			zap.Error(err))
		return err
	}

	logger.Info("套餐已删除", zap.String("id", id))
	return nil
}

// SubscribeUserToPlan 用户订阅套餐
func (s *PlanService) SubscribeUserToPlan(userID, planID string, months int) (*dbinit.UserSubscription, error) {
	// 获取套餐信息
	plan, err := s.GetPlan(planID)
	if err != nil {
		return nil, err
	}
	if !plan.Enabled {
		return nil, fmt.Errorf("套餐未启用")
	}

	// 检查用户是否已有有效订阅
	existingSub, _ := s.db.DB.SQLite.GetActiveSubscriptionByUserID(userID)
	if existingSub != nil {
		return nil, fmt.Errorf("用户已有有效订阅")
	}

	// 创建订阅
	startDate := time.Now()
	var endDate time.Time
	var trafficReset time.Time

	switch plan.BillingCycle {
	case "monthly":
		endDate = startDate.AddDate(0, months, 0)
		trafficReset = startDate.AddDate(0, 1, 0)
	case "yearly":
		endDate = startDate.AddDate(months, 0, 0)
		trafficReset = startDate.AddDate(1, 0, 0)
	case "permanent":
		endDate = startDate.AddDate(100, 0, 0) // 100年
		trafficReset = startDate.AddDate(0, 1, 0)
	}

	sub := &dbinit.UserSubscription{
		ID:           uuid.New().String(),
		UserID:       userID,
		PlanID:       planID,
		StartDate:    startDate,
		EndDate:      endDate,
		Status:       "active",
		UsedRules:    0,
		UsedTraffic:  0,
		TrafficReset: trafficReset,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.DB.SQLite.CreateSubscriptionFromUserSubscription(sub); err != nil {
		logger.Error("创建订阅失败",
			zap.String("userID", userID),
			zap.String("planID", planID),
			zap.Error(err))
		return nil, err
	}

	logger.Info("用户已订阅套餐",
		zap.String("userID", userID),
		zap.String("planID", planID),
		zap.String("planName", plan.Name))

	return sub, nil
}

func (s *PlanService) GetUserSubscription(userID string) (*dbinit.UserSubscription, *dbinit.Plan, error) {
	sub, err := s.db.DB.SQLite.GetActiveSubscriptionByUserID(userID)
	if err != nil {
		return nil, nil, err
	}
	if sub == nil {
		// 用户无订阅，返回nil而不是错误
		return nil, nil, nil
	}

	// 检查是否过期
	if time.Now().After(sub.EndDate) {
		sub.Status = "expired"
		_ = s.db.DB.SQLite.UpdateSubscription(sub)
		// 订阅过期，返回nil
		return nil, nil, nil
	}

	// 检查流量是否需要重置
	if time.Now().After(sub.TrafficReset) {
		sub.UsedTraffic = 0
		sub.TrafficReset = sub.TrafficReset.AddDate(0, 1, 0) // 下个月
		_ = s.db.DB.SQLite.UpdateSubscription(sub)
	}

	plan, err := s.GetPlan(sub.PlanID)
	if err != nil {
		return sub, nil, err
	}

	return sub, plan, nil
}

// CheckQuota 检查配额
func (s *PlanService) CheckQuota(userID string, checkType string) error {
	sub, plan, err := s.GetUserSubscription(userID)
	if err != nil {
		return err
	}

	switch checkType {
	case "rules":
		if plan.MaxRules > 0 && sub.UsedRules >= plan.MaxRules {
			return fmt.Errorf("规则数已达上限 (%d/%d)", sub.UsedRules, plan.MaxRules)
		}
	case "traffic":
		if plan.MaxTraffic > 0 && sub.UsedTraffic >= plan.MaxTraffic {
			return fmt.Errorf("流量已用完 (%d/%d)", sub.UsedTraffic, plan.MaxTraffic)
		}
	}

	return nil
}

// IncrementRuleCount 增加规则数
func (s *PlanService) IncrementRuleCount(userID string) error {
	sub, err := s.db.DB.SQLite.GetActiveSubscriptionByUserID(userID)
	if err != nil || sub == nil {
		return fmt.Errorf("未找到有效订阅")
	}

	sub.UsedRules++
	sub.UpdatedAt = time.Now()

	return s.db.DB.SQLite.UpdateSubscription(sub)
}

// DecrementRuleCount 减少规则数
func (s *PlanService) DecrementRuleCount(userID string) error {
	sub, err := s.db.DB.SQLite.GetActiveSubscriptionByUserID(userID)
	if err != nil || sub == nil {
		return nil // 忽略错误
	}

	if sub.UsedRules > 0 {
		sub.UsedRules--
		sub.UpdatedAt = time.Now()
		return s.db.DB.SQLite.UpdateSubscription(sub)
	}

	return nil
}

// AddTrafficUsage 添加流量使用
func (s *PlanService) AddTrafficUsage(userID string, bytes int64) error {
	sub, err := s.db.DB.SQLite.GetActiveSubscriptionByUserID(userID)
	if err != nil || sub == nil {
		return nil // 忽略错误
	}

	sub.UsedTraffic += bytes
	sub.UpdatedAt = time.Now()

	return s.db.DB.SQLite.UpdateSubscription(sub)
}
