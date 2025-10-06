package api

import (
	"database/sql"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/service"
	"gkipass/plane/pkg/logger"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// PlanHandler 套餐处理器
type PlanHandler struct {
	app         *App
	planService *service.PlanService
}

// NewPlanHandler 创建套餐处理器
func NewPlanHandler(app *App) *PlanHandler {
	return &PlanHandler{
		app:         app,
		planService: service.NewPlanService(app.DB),
	}
}

// planToResponse 将数据库Plan转换为API响应格式
func planToResponse(plan *dbinit.Plan) map[string]interface{} {
	allowedNodeIDs := ""
	if plan.AllowedNodeIDs.Valid {
		allowedNodeIDs = plan.AllowedNodeIDs.String
	}

	return map[string]interface{}{
		"id":               plan.ID,
		"name":             plan.Name,
		"max_rules":        plan.MaxRules,
		"max_traffic":      plan.MaxTraffic,
		"traffic":          plan.Traffic,
		"max_tunnels":      plan.MaxTunnels,
		"max_bandwidth":    plan.MaxBandwidth,
		"max_connections":  plan.MaxConnections,
		"max_connect_ips":  plan.MaxConnectIPs,
		"allowed_node_ids": allowedNodeIDs,
		"billing_cycle":    plan.BillingCycle,
		"duration":         plan.Duration,
		"price":            plan.Price,
		"enabled":          plan.Enabled,
		"created_at":       plan.CreatedAt,
		"updated_at":       plan.UpdatedAt,
		"description":      plan.Description,
	}
}

// CreatePlanRequest 创建套餐请求
type CreatePlanRequest struct {
	Name           string  `json:"name" binding:"required"`
	MaxRules       int     `json:"max_rules"`
	MaxTraffic     int64   `json:"max_traffic"`
	MaxBandwidth   int64   `json:"max_bandwidth"`
	MaxConnections int     `json:"max_connections"`
	MaxConnectIPs  int     `json:"max_connect_ips"`
	AllowedNodeIDs string  `json:"allowed_node_ids"`
	BillingCycle   string  `json:"billing_cycle" binding:"required,oneof=monthly yearly permanent"`
	Price          float64 `json:"price"`
	Enabled        bool    `json:"enabled"`
	Description    string  `json:"description"`
}

// Create 创建套餐（仅管理员）
func (h *PlanHandler) Create(c *gin.Context) {
	var req CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	plan := &dbinit.Plan{
		Name:           req.Name,
		MaxRules:       req.MaxRules,
		MaxTraffic:     req.MaxTraffic,
		MaxBandwidth:   req.MaxBandwidth,
		MaxConnections: req.MaxConnections,
		MaxConnectIPs:  req.MaxConnectIPs,
		AllowedNodeIDs: sql.NullString{String: req.AllowedNodeIDs, Valid: req.AllowedNodeIDs != ""},
		BillingCycle:   req.BillingCycle,
		Price:          req.Price,
		Enabled:        req.Enabled,
		Description:    req.Description,
	}

	if err := h.planService.CreatePlan(plan); err != nil {
		logger.Error("创建套餐失败", zap.Error(err))
		response.InternalError(c, "Failed to create plan")
		return
	}

	response.SuccessWithMessage(c, "Plan created successfully", planToResponse(plan))
}

// List 列出套餐
func (h *PlanHandler) List(c *gin.Context) {
	var enabled *bool
	if enabledStr := c.Query("enabled"); enabledStr != "" {
		val := enabledStr == "true"
		enabled = &val
	}

	plans, err := h.planService.ListPlans(enabled)
	if err != nil {
		logger.Error("列出套餐失败", zap.Error(err))
		response.InternalError(c, "Failed to list plans")
		return
	}

	// Convert plans to response format
	responsePlans := make([]map[string]interface{}, len(plans))
	for i, plan := range plans {
		responsePlans[i] = planToResponse(plan)
	}

	response.Success(c, responsePlans)
}

// Get 获取套餐详情
func (h *PlanHandler) Get(c *gin.Context) {
	id := c.Param("id")

	plan, err := h.planService.GetPlan(id)
	if err != nil {
		response.InternalError(c, "Failed to get plan")
		return
	}
	if plan == nil {
		response.NotFound(c, "Plan not found")
		return
	}

	response.Success(c, planToResponse(plan))
}

// Update 更新套餐（仅管理员）
func (h *PlanHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 获取现有套餐
	plan, err := h.planService.GetPlan(id)
	if err != nil || plan == nil {
		response.NotFound(c, "Plan not found")
		return
	}

	// 更新字段
	plan.Name = req.Name
	plan.MaxRules = req.MaxRules
	plan.MaxTraffic = req.MaxTraffic
	plan.MaxBandwidth = req.MaxBandwidth
	plan.MaxConnections = req.MaxConnections
	plan.MaxConnectIPs = req.MaxConnectIPs
	plan.AllowedNodeIDs = sql.NullString{String: req.AllowedNodeIDs, Valid: req.AllowedNodeIDs != ""}
	plan.BillingCycle = req.BillingCycle
	plan.Price = req.Price
	plan.Enabled = req.Enabled
	plan.Description = req.Description

	if err := h.planService.UpdatePlan(plan); err != nil {
		logger.Error("更新套餐失败", zap.Error(err))
		response.InternalError(c, "Failed to update plan")
		return
	}

	response.SuccessWithMessage(c, "Plan updated successfully", planToResponse(plan))
}

// Delete 删除套餐（仅管理员）
func (h *PlanHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.planService.DeletePlan(id); err != nil {
		logger.Error("删除套餐失败", zap.String("id", id), zap.Error(err))
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "Plan deleted successfully", nil)
}

// Subscribe 订阅套餐
func (h *PlanHandler) Subscribe(c *gin.Context) {
	planID := c.Param("id")
	userID, _ := c.Get("user_id")

	// 获取订阅月数（默认1个月）
	months := 1
	if monthsStr := c.Query("months"); monthsStr != "" {
		if m, err := strconv.Atoi(monthsStr); err == nil && m > 0 {
			months = m
		}
	}

	sub, err := h.planService.SubscribeUserToPlan(userID.(string), planID, months)
	if err != nil {
		logger.Error("订阅套餐失败",
			zap.String("userID", userID.(string)),
			zap.String("planID", planID),
			zap.Error(err))
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "Subscribed successfully", sub)
}

// MySubscription 获取我的订阅
func (h *PlanHandler) MySubscription(c *gin.Context) {
	userID, _ := c.Get("user_id")

	sub, plan, err := h.planService.GetUserSubscription(userID.(string))
	if err != nil {
		response.InternalError(c, "Failed to get subscription")
		return
	}

	// 用户无订阅，返回null而不是错误
	if sub == nil || plan == nil {
		response.Success(c, nil)
		return
	}

	response.Success(c, gin.H{
		"id":               sub.ID,
		"plan_id":          sub.PlanID,
		"status":           sub.Status,
		"used_rules":       sub.UsedRules,
		"used_traffic":     sub.UsedTraffic,
		"used_bandwidth":   0,
		"used_connections": 0,
		"start_date":       sub.StartDate,
		"end_date":         sub.EndDate,
		"plan": gin.H{
			"id":              plan.ID,
			"name":            plan.Name,
			"max_rules":       plan.MaxRules,
			"max_traffic":     plan.MaxTraffic,
			"max_bandwidth":   plan.MaxBandwidth,
			"max_connections": plan.MaxConnections,
		},
	})
}
