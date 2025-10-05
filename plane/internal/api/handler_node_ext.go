package api

import (
	"encoding/json"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GetAvailableNodes 获取用户可用的节点（基于套餐）
func (h *NodeHandler) GetAvailableNodes(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	// 管理员可以看到所有节点
	if role == "admin" {
		nodes, err := h.app.DB.DB.SQLite.ListNodes("", "", 1000, 0)
		if err != nil {
			response.InternalError(c, "Failed to get nodes")
			return
		}
		response.Success(c, nodes)
		return
	}

	// 普通用户：检查订阅状态
	sub, err := h.app.DB.DB.SQLite.GetActiveSubscriptionByUserID(userID.(string))
	if err != nil {
		response.InternalError(c, "Failed to check subscription")
		return
	}

	// 如果没有订阅，返回空列表
	if sub == nil {
		response.Success(c, gin.H{
			"nodes":            []interface{}{},
			"has_subscription": false,
			"message":          "请先购买套餐以查看可用节点",
		})
		return
	}

	// 获取套餐信息
	plan, err := h.app.DB.DB.SQLite.GetPlan(sub.PlanID)
	if err != nil || plan == nil {
		response.Success(c, gin.H{
			"nodes":            []interface{}{},
			"has_subscription": false,
			"message":          "请先购买套餐以查看可用节点",
		})
		return
	}

	// 获取所有节点
	allNodes, err := h.app.DB.DB.SQLite.ListNodes("", "", 1000, 0)
	if err != nil {
		response.InternalError(c, "Failed to get nodes")
		return
	}

	// 如果套餐的allowed_node_ids为空或"[]"，则允许使用所有节点
	if plan.AllowedNodeIDs == "" || plan.AllowedNodeIDs == "[]" {
		response.Success(c, gin.H{
			"nodes":            allNodes,
			"has_subscription": true,
			"plan_name":        plan.Name,
		})
		return
	}

	// 解析允许的节点ID列表
	var allowedNodeIDs []string
	if err := json.Unmarshal([]byte(plan.AllowedNodeIDs), &allowedNodeIDs); err != nil {
		logger.Error("解析套餐节点ID失败", zap.Error(err))
		response.InternalError(c, "Failed to parse plan node IDs")
		return
	}

	// 过滤节点
	allowedNodesMap := make(map[string]bool)
	for _, id := range allowedNodeIDs {
		allowedNodesMap[id] = true
	}

	var availableNodes []interface{}
	for _, node := range allNodes {
		if allowedNodesMap[node.ID] {
			availableNodes = append(availableNodes, node)
		}
	}

	response.Success(c, gin.H{
		"nodes":            availableNodes,
		"has_subscription": true,
		"plan_name":        plan.Name,
		"total_allowed":    len(allowedNodeIDs),
		"available_count":  len(availableNodes),
	})
}
