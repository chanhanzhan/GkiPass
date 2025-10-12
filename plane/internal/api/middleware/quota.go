package middleware

import (
	"gkipass/plane/db"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/service"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// QuotaCheck 配额检查中间件
func QuotaCheck(dbManager *db.Manager) gin.HandlerFunc {
	planService := service.NewPlanService(dbManager)

	return func(c *gin.Context) {
		// 管理员跳过检查
		role, exists := c.Get("role")
		if exists && role == "admin" {
			c.Next()
			return
		}

		// 获取用户ID
		userID, exists := c.Get("user_id")
		if !exists {
			response.GinUnauthorized(c, "User not authenticated")
			c.Abort()
			return
		}

		// 检查订阅
		sub, plan, err := planService.GetUserSubscription(userID.(string))
		if err != nil {
			response.InternalError(c, "Failed to check subscription")
			c.Abort()
			return
		}
		if sub != nil && plan != nil {
			c.Set("subscription", sub)
			c.Set("plan", plan)
		}

		c.Next()
	}
}

// RuleQuotaCheck 规则配额检查（用于创建隧道）
func RuleQuotaCheck(dbManager *db.Manager) gin.HandlerFunc {
	planService := service.NewPlanService(dbManager)

	return func(c *gin.Context) {
		// 管理员跳过检查
		role, exists := c.Get("role")
		if exists && role == "admin" {
			c.Next()
			return
		}

		// 获取用户ID
		userID, exists := c.Get("user_id")
		if !exists {
			response.GinUnauthorized(c, "User not authenticated")
			c.Abort()
			return
		}

		// 检查订阅和规则配额
		sub, plan, err := planService.GetUserSubscription(userID.(string))
		if err != nil {
			response.InternalError(c, "Failed to check subscription")
			c.Abort()
			return
		}

		// 无订阅时禁止创建
		if sub == nil || plan == nil {
			response.GinForbidden(c, "No active subscription. Please subscribe to a plan to create tunnels.")
			c.Abort()
			return
		}

		// 检查规则配额
		if err := planService.CheckQuota(userID.(string), "rules"); err != nil {
			logger.Warn("规则配额不足",
				zap.String("userID", userID.(string)),
				zap.Error(err))
			response.GinForbidden(c, err.Error())
			c.Abort()
			return
		}

		c.Next()
	}
}

// TrafficQuotaCheck 流量配额检查
func TrafficQuotaCheck(dbManager *db.Manager) gin.HandlerFunc {
	planService := service.NewPlanService(dbManager)

	return func(c *gin.Context) {
		// 管理员跳过检查
		role, exists := c.Get("role")
		if exists && role == "admin" {
			c.Next()
			return
		}

		// 获取用户ID
		userID, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}

		// 检查流量配额
		if err := planService.CheckQuota(userID.(string), "traffic"); err != nil {
			logger.Warn("流量配额不足",
				zap.String("userID", userID.(string)),
				zap.Error(err))
			response.GinForbidden(c, err.Error())
			c.Abort()
			return
		}

		c.Next()
	}
}
