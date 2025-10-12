package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/model"
	"gkipass/plane/internal/service"
)

// RuleHandler 规则处理器
type RuleHandler struct {
	ruleService *service.RuleService
	logger      *zap.Logger
}

// NewRuleHandler 创建规则处理器
func NewRuleHandler(ruleService *service.RuleService) *RuleHandler {
	return &RuleHandler{
		ruleService: ruleService,
		logger:      zap.L().Named("rule-handler"),
	}
}

// RegisterRoutes 注册路由
func (h *RuleHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/rules", h.ListRules).Methods("GET")
	r.HandleFunc("/rules", h.CreateRule).Methods("POST")
	r.HandleFunc("/rules/{id}", h.GetRule).Methods("GET")
	r.HandleFunc("/rules/{id}", h.UpdateRule).Methods("PUT")
	r.HandleFunc("/rules/{id}", h.DeleteRule).Methods("DELETE")
	r.HandleFunc("/rules/{id}/stats", h.GetRuleStats).Methods("GET")
	r.HandleFunc("/rules/{id}/stats/reset", h.ResetRuleStats).Methods("POST")
	r.HandleFunc("/nodes/{id}/rules", h.GetNodeRules).Methods("GET")
	r.HandleFunc("/nodes/{id}/rules/sync", h.SyncRulesToNode).Methods("POST")
}

// ListRules 列出所有规则
func (h *RuleHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	query := r.URL.Query()
	enabled := query.Get("enabled")
	protocol := query.Get("protocol")

	// 获取规则
	rules, err := h.ruleService.ListRules()
	if err != nil {
		h.logger.Error("列出规则失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取规则列表失败", err)
		return
	}

	// 过滤规则
	var filteredRules []*model.Rule
	for _, rule := range rules {
		// 过滤启用状态
		if enabled != "" {
			enabledBool, err := strconv.ParseBool(enabled)
			if err == nil && rule.Enabled != enabledBool {
				continue
			}
		}

		// 过滤协议
		if protocol != "" && rule.Protocol != protocol {
			continue
		}

		filteredRules = append(filteredRules, rule)
	}

	response.Success(w, filteredRules)
}

// CreateRule 创建规则
func (h *RuleHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	// 获取用户ID
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 解析请求体
	var rule model.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 设置创建者
	rule.CreatedBy = claims.UserID

	// 创建规则
	if err := h.ruleService.CreateRule(&rule); err != nil {
		h.logger.Error("创建规则失败",
			zap.String("name", rule.Name),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "创建规则失败", err)
		return
	}

	response.Created(w, rule)
}

// GetRule 获取规则
func (h *RuleHandler) GetRule(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 获取规则
	rule, err := h.ruleService.GetRule(id)
	if err != nil {
		h.logger.Error("获取规则失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusNotFound, "规则不存在", err)
		return
	}

	response.Success(w, rule)
}

// UpdateRule 更新规则
func (h *RuleHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 解析请求体
	var rule model.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 确保ID一致
	rule.ID = id

	// 更新规则
	if err := h.ruleService.UpdateRule(&rule); err != nil {
		h.logger.Error("更新规则失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "更新规则失败", err)
		return
	}

	response.Success(w, rule)
}

// DeleteRule 删除规则
func (h *RuleHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 删除规则
	if err := h.ruleService.DeleteRule(id); err != nil {
		h.logger.Error("删除规则失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "删除规则失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "规则已删除",
		"id":      id,
	})
}

// GetRuleStats 获取规则统计信息
func (h *RuleHandler) GetRuleStats(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 获取规则
	rule, err := h.ruleService.GetRule(id)
	if err != nil {
		h.logger.Error("获取规则失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusNotFound, "规则不存在", err)
		return
	}

	// 构建统计信息
	stats := map[string]interface{}{
		"rule_id":          rule.ID,
		"rule_name":        rule.Name,
		"connection_count": rule.ConnectionCount,
		"bytes_in":         rule.BytesIn,
		"bytes_out":        rule.BytesOut,
		"total_bytes":      rule.BytesIn + rule.BytesOut,
		"last_active":      rule.LastActive,
		"enabled":          rule.Enabled,
	}

	response.Success(w, stats)
}

// ResetRuleStats 重置规则统计信息
func (h *RuleHandler) ResetRuleStats(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 重置规则统计信息
	if err := h.ruleService.ResetRuleStats(id); err != nil {
		h.logger.Error("重置规则统计信息失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "重置规则统计信息失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "规则统计信息已重置",
		"id":      id,
	})
}

// GetNodeRules 获取节点的规则
func (h *RuleHandler) GetNodeRules(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	nodeID := vars["id"]

	// 获取节点的规则
	rules, err := h.ruleService.GetNodeRules(nodeID)
	if err != nil {
		h.logger.Error("获取节点规则失败",
			zap.String("node_id", nodeID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取节点规则失败", err)
		return
	}

	response.Success(w, rules)
}

// SyncRulesToNode 同步规则到节点
func (h *RuleHandler) SyncRulesToNode(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	nodeID := vars["id"]

	// 同步规则到节点
	if err := h.ruleService.SyncRulesToNode(nodeID); err != nil {
		h.logger.Error("同步规则到节点失败",
			zap.String("node_id", nodeID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "同步规则到节点失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "规则已同步到节点",
		"node_id": nodeID,
	})
}
