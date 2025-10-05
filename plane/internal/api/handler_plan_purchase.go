package api

import (
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PurchasePlanRequest 购买套餐请求
type PurchasePlanRequest struct {
	PlanID string `json:"plan_id" binding:"required"`
}

// PurchasePlan 购买套餐
func (h *PlanHandler) PurchasePlan(c *gin.Context) {
	var req PurchasePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")

	// 1. 获取套餐信息
	plan, err := h.app.DB.DB.SQLite.GetPlan(req.PlanID)
	if err != nil || plan == nil {
		response.BadRequest(c, "Plan not found")
		return
	}

	if !plan.Enabled {
		response.BadRequest(c, "Plan is not available")
		return
	}

	// 2. 检查用户余额
	wallet, err := h.app.DB.DB.SQLite.GetWalletByUserID(userID.(string))
	if err != nil || wallet == nil {
		response.BadRequest(c, "Wallet not found")
		return
	}

	if wallet.Balance < plan.Price {
		response.BadRequest(c, "Insufficient balance")
		return
	}

	// 3. 扣除余额
	newBalance := wallet.Balance - plan.Price
	if err := h.app.DB.DB.SQLite.UpdateWalletBalance(wallet.ID, newBalance, wallet.Frozen); err != nil {
		logger.Error("更新钱包余额失败", zap.Error(err))
		response.InternalError(c, "Failed to update wallet balance")
		return
	}

	// 4. 记录交易
	transaction := &dbinit.WalletTransaction{
		ID:          uuid.New().String(),
		WalletID:    wallet.ID,
		Type:        "purchase",
		Amount:      -plan.Price,
		Balance:     newBalance,
		Description: "购买套餐: " + plan.Name,
		CreatedAt:   time.Now(),
	}
	if err := h.app.DB.DB.SQLite.CreateWalletTransaction(transaction); err != nil {
		logger.Warn("记录交易失败", zap.Error(err))
	}

	// 5. 创建订阅记录
	startDate := time.Now()
	var endDate time.Time
	switch plan.BillingCycle {
	case "monthly":
		endDate = startDate.AddDate(0, 1, 0)
	case "yearly":
		endDate = startDate.AddDate(1, 0, 0)
	case "permanent":
		endDate = startDate.AddDate(100, 0, 0) // 100年后
	default:
		endDate = startDate.AddDate(0, 1, 0)
	}

	subscription := &dbinit.UserSubscription{
		ID:           uuid.New().String(),
		UserID:       userID.(string),
		PlanID:       plan.ID,
		StartDate:    startDate,
		EndDate:      endDate,
		Status:       "active",
		UsedRules:    0,
		UsedTraffic:  0,
		TrafficReset: startDate,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := h.app.DB.DB.SQLite.CreateSubscription(subscription); err != nil {
		logger.Error("创建订阅失败", zap.Error(err))
		response.InternalError(c, "Failed to create subscription")
		return
	}

	logger.Info("用户购买套餐成功",
		zap.String("userID", userID.(string)),
		zap.String("planID", plan.ID),
		zap.String("planName", plan.Name),
		zap.Float64("price", plan.Price))

	response.Success(c, gin.H{
		"subscription": subscription,
		"plan":         plan,
		"new_balance":  newBalance,
	})
}
