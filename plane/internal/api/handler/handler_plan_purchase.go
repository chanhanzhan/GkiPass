package handler

import (
	"fmt"
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
	PlanID        string `json:"plan_id" binding:"required"`
	PaymentMethod string `json:"payment_method" binding:"required"`
}

// PurchasePlan 购买套餐
func (h *PlanHandler) PurchasePlan(c *gin.Context) {
	var req PurchasePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")

	// 获取套餐信息
	plan, err := h.app.DB.DB.SQLite.GetPlan(req.PlanID)
	if err != nil || plan == nil {
		response.NotFound(c, "Plan not found")
		return
	}

	// 创建支付订单
	orderID := uuid.New().String()
	transaction := &dbinit.WalletTransaction{
		ID:            orderID,
		UserID:        userID.(string),
		Type:          "purchase",
		Amount:        -plan.Price, // 负数表示扣款
		RelatedID:     plan.ID,
		RelatedType:   "plan",
		Status:        "pending",
		PaymentMethod: req.PaymentMethod,
		Description:   fmt.Sprintf("购买套餐：%s", plan.Name),
		CreatedAt:     time.Now(),
	}

	// 获取用户钱包
	wallet, err := h.app.DB.DB.SQLite.GetWalletByUserID(userID.(string))
	if err != nil || wallet == nil {
		response.BadRequest(c, "Wallet not found")
		return
	}
	transaction.WalletID = wallet.ID
	transaction.Balance = wallet.Balance

	if err := h.app.DB.DB.SQLite.CreateWalletTransaction(transaction); err != nil {
		logger.Error("创建套餐购买订单失败", zap.Error(err))
		response.InternalError(c, "Failed to create purchase order")
		return
	}

	logger.Info("创建套餐购买订单",
		zap.String("orderID", orderID),
		zap.String("userID", userID.(string)),
		zap.String("planID", plan.ID),
		zap.Float64("price", plan.Price))

	response.Success(c, gin.H{
		"order_id": orderID,
		"plan":     plan,
		"amount":   plan.Price,
		"status":   "pending",
	})
}

// ActivateSubscription 激活订阅（支付成功回调）
func (h *PlanHandler) ActivateSubscription(orderID, userID, planID string) error {
	// 获取套餐信息
	plan, err := h.app.DB.DB.SQLite.GetPlan(planID)
	if err != nil || plan == nil {
		return fmt.Errorf("plan not found")
	}

	// 获取用户当前订阅
	existingSub, err := h.app.DB.DB.SQLite.GetUserSubscription(userID)
	if err != nil {
		return err
	}

	now := time.Now()
	var expiresAt time.Time

	// 如果已有订阅且未过期，则延长有效期
	if existingSub != nil && existingSub.ExpiresAt.After(now) {
		expiresAt = existingSub.ExpiresAt.Add(time.Duration(plan.Duration) * 24 * time.Hour)
	} else {
		expiresAt = now.Add(time.Duration(plan.Duration) * 24 * time.Hour)
	}

	// 创建或更新订阅
	subscription := &dbinit.Subscription{
		ID:           uuid.New().String(),
		UserID:       userID,
		PlanID:       planID,
		Status:       "active",
		StartAt:      now,
		ExpiresAt:    expiresAt,
		Traffic:      plan.Traffic * 1024 * 1024 * 1024, // GB转Bytes
		UsedTraffic:  0,
		MaxTunnels:   plan.MaxTunnels,
		MaxBandwidth: plan.MaxBandwidth,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// 如果已有订阅，累加流量和隧道数
	if existingSub != nil {
		subscription.ID = existingSub.ID
		subscription.Traffic += existingSub.Traffic - existingSub.UsedTraffic // 累加剩余流量
		subscription.UsedTraffic = existingSub.UsedTraffic
		subscription.MaxTunnels += existingSub.MaxTunnels
		subscription.CreatedAt = existingSub.CreatedAt

		if err := h.app.DB.DB.SQLite.UpdateSubscription(subscription); err != nil {
			return err
		}
	} else {
		if err := h.app.DB.DB.SQLite.CreateSubscription(subscription); err != nil {
			return err
		}
	}

	logger.Info("激活订阅成功",
		zap.String("userID", userID),
		zap.String("planID", planID),
		zap.Time("expiresAt", expiresAt))

	return nil
}
