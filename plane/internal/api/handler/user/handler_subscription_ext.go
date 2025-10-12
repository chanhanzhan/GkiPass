package user

import (
	"strconv"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// parseInt 解析整数参数
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

// SubscriptionHandler 订阅处理器
type SubscriptionHandler struct {
	app *types.App
}

// NewSubscriptionHandler 创建订阅处理器
func NewSubscriptionHandler(app *types.App) *SubscriptionHandler {
	return &SubscriptionHandler{app: app}
}

// GetCurrentSubscription 获取当前用户订阅
func (h *SubscriptionHandler) GetCurrentSubscription(c *gin.Context) {
	userID, _ := c.Get("user_id")

	// 获取用户订阅
	subscription, err := h.app.DB.DB.SQLite.GetUserSubscription(userID.(string))
	if err != nil {
		logger.Error("获取用户订阅失败", zap.Error(err))
		response.InternalError(c, "Failed to get subscription")
		return
	}

	if subscription == nil {
		response.GinSuccess(c, nil)
		return
	}

	// 获取套餐信息
	plan, err := h.app.DB.DB.SQLite.GetPlan(subscription.PlanID)
	if err == nil && plan != nil {
		response.GinSuccess(c, gin.H{
			"id":            subscription.ID,
			"user_id":       subscription.UserID,
			"plan_id":       subscription.PlanID,
			"plan_name":     plan.Name,
			"status":        subscription.Status,
			"start_at":      subscription.StartAt,
			"expires_at":    subscription.ExpiresAt,
			"traffic":       subscription.Traffic,
			"used_traffic":  subscription.UsedTraffic,
			"max_tunnels":   subscription.MaxTunnels,
			"max_bandwidth": subscription.MaxBandwidth,
			"created_at":    subscription.CreatedAt,
			"updated_at":    subscription.UpdatedAt,
		})
		return
	}

	response.GinSuccess(c, subscription)
}

// ListSubscriptions 列出所有订阅（管理员）
func (h *SubscriptionHandler) ListSubscriptions(c *gin.Context) {
	// 分页参数
	page := 1
	limit := 20
	if p := c.Query("page"); p != "" {
		if val, err := parseInt(p); err == nil && val > 0 {
			page = val
		}
	}
	if l := c.Query("limit"); l != "" {
		if val, err := parseInt(l); err == nil && val > 0 {
			limit = val
		}
	}

	offset := (page - 1) * limit

	// 查询订阅列表
	subscriptions, total, err := h.app.DB.DB.SQLite.ListSubscriptions(limit, offset)
	if err != nil {
		logger.Error("查询订阅列表失败", zap.Error(err))
		response.InternalError(c, "Failed to list subscriptions")
		return
	}

	response.GinSuccess(c, gin.H{
		"data":  subscriptions,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}
