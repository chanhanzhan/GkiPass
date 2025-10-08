package handler

import (
	"encoding/json"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PolicyHandler 策略处理器
type PolicyHandler struct {
	app *types.App
}

// NewPolicyHandler 创建策略处理器
func NewPolicyHandler(app *types.App) *PolicyHandler {
	return &PolicyHandler{app: app}
}

// CreatePolicyRequest 创建策略请求
type CreatePolicyRequest struct {
	Name        string              `json:"name" binding:"required"`
	Type        string              `json:"type" binding:"required,oneof=protocol acl routing"`
	Priority    int                 `json:"priority"`
	Enabled     bool                `json:"enabled"`
	Config      dbinit.PolicyConfig `json:"config" binding:"required"`
	NodeIDs     []string            `json:"node_ids"`
	Description string              `json:"description"`
}

// Create 创建策略
func (h *PolicyHandler) Create(c *gin.Context) {
	var req CreatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 序列化配置和节点ID列表
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		response.BadRequest(c, "Invalid config format")
		return
	}

	nodeIDsJSON, err := json.Marshal(req.NodeIDs)
	if err != nil {
		response.BadRequest(c, "Invalid node_ids format")
		return
	}

	policy := &dbinit.Policy{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Type:        req.Type,
		Priority:    req.Priority,
		Enabled:     req.Enabled,
		Config:      string(configJSON),
		NodeIDs:     string(nodeIDsJSON),
		Description: req.Description,
	}

	if err := h.app.DB.DB.SQLite.CreatePolicy(policy); err != nil {
		response.InternalError(c, "Failed to create policy: "+err.Error())
		return
	}

	// 使相关节点的策略缓存失效
	if h.app.DB.HasCache() {
		for _, nodeID := range req.NodeIDs {
			_ = h.app.DB.Cache.Redis.InvalidatePolicyCache(nodeID)
		}
	}

	response.SuccessWithMessage(c, "Policy created successfully", policy)
}

// List 列出策略
func (h *PolicyHandler) List(c *gin.Context) {
	policyType := c.Query("type")
	enabledStr := c.Query("enabled")

	var enabled *bool
	if enabledStr != "" {
		val := enabledStr == "true"
		enabled = &val
	}

	policies, err := h.app.DB.DB.SQLite.ListPolicies(policyType, enabled)
	if err != nil {
		response.InternalError(c, "Failed to list policies: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"policies": policies,
		"total":    len(policies),
	})
}

// Get 获取策略详情
func (h *PolicyHandler) Get(c *gin.Context) {
	id := c.Param("id")

	policy, err := h.app.DB.DB.SQLite.GetPolicy(id)
	if err != nil {
		response.InternalError(c, "Failed to get policy: "+err.Error())
		return
	}

	if policy == nil {
		response.NotFound(c, "Policy not found")
		return
	}

	response.Success(c, policy)
}

// UpdatePolicyRequest 更新策略请求
type UpdatePolicyRequest struct {
	Name        string               `json:"name"`
	Priority    int                  `json:"priority"`
	Enabled     *bool                `json:"enabled"`
	Config      *dbinit.PolicyConfig `json:"config"`
	NodeIDs     []string             `json:"node_ids"`
	Description string               `json:"description"`
}

// Update 更新策略
func (h *PolicyHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req UpdatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	policy, err := h.app.DB.DB.SQLite.GetPolicy(id)
	if err != nil {
		response.InternalError(c, "Failed to get policy: "+err.Error())
		return
	}

	if policy == nil {
		response.NotFound(c, "Policy not found")
		return
	}

	// 更新字段
	if req.Name != "" {
		policy.Name = req.Name
	}
	if req.Priority > 0 {
		policy.Priority = req.Priority
	}
	if req.Enabled != nil {
		policy.Enabled = *req.Enabled
	}
	if req.Config != nil {
		configJSON, _ := json.Marshal(req.Config)
		policy.Config = string(configJSON)
	}
	if req.NodeIDs != nil {
		nodeIDsJSON, _ := json.Marshal(req.NodeIDs)
		policy.NodeIDs = string(nodeIDsJSON)
	}
	if req.Description != "" {
		policy.Description = req.Description
	}

	if err := h.app.DB.DB.SQLite.UpdatePolicy(policy); err != nil {
		response.InternalError(c, "Failed to update policy: "+err.Error())
		return
	}

	// 使相关节点的策略缓存失效
	if h.app.DB.HasCache() {
		var nodeIDs []string
		json.Unmarshal([]byte(policy.NodeIDs), &nodeIDs)
		for _, nodeID := range nodeIDs {
			_ = h.app.DB.Cache.Redis.InvalidatePolicyCache(nodeID)
		}
	}

	response.SuccessWithMessage(c, "Policy updated successfully", policy)
}

// Delete 删除策略
func (h *PolicyHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	policy, _ := h.app.DB.DB.SQLite.GetPolicy(id)
	if err := h.app.DB.DB.SQLite.DeletePolicy(id); err != nil {
		response.InternalError(c, "Failed to delete policy: "+err.Error())
		return
	}

	// 使相关节点的策略缓存失效
	if h.app.DB.HasCache() && policy != nil {
		var nodeIDs []string
		json.Unmarshal([]byte(policy.NodeIDs), &nodeIDs)
		for _, nodeID := range nodeIDs {
			_ = h.app.DB.Cache.Redis.InvalidatePolicyCache(nodeID)
		}
	}

	response.SuccessWithMessage(c, "Policy deleted successfully", nil)
}

// Deploy 部署策略到节点
func (h *PolicyHandler) Deploy(c *gin.Context) {
	id := c.Param("id")

	policy, err := h.app.DB.DB.SQLite.GetPolicy(id)
	if err != nil {
		response.InternalError(c, "Failed to get policy: "+err.Error())
		return
	}

	if policy == nil {
		response.NotFound(c, "Policy not found")
		return
	}

	// 这里实现策略下发逻辑
	// 在实际应用中，可以通过WebSocket或HTTP推送给节点
	// 目前只是更新缓存

	if h.app.DB.HasCache() {
		var nodeIDs []string
		json.Unmarshal([]byte(policy.NodeIDs), &nodeIDs)
		for _, nodeID := range nodeIDs {
			_ = h.app.DB.Cache.Redis.SetPolicyForNode(nodeID, []string{policy.ID})
		}
	}

	response.SuccessWithMessage(c, "Policy deployed successfully", gin.H{
		"policy_id": id,
		"status":    "deployed",
	})
}
