package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/model"
	"gkipass/plane/internal/service"
)

// ACLHandler ACL处理器
type ACLHandler struct {
	aclService  *service.ACLService
	ruleService *service.RuleService
	logger      *zap.Logger
}

// NewACLHandler 创建ACL处理器
func NewACLHandler(aclService *service.ACLService, ruleService *service.RuleService) *ACLHandler {
	return &ACLHandler{
		aclService:  aclService,
		ruleService: ruleService,
		logger:      zap.L().Named("acl-handler"),
	}
}

// RegisterRoutes 注册路由
func (h *ACLHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/rules/{rule_id}/acl", h.GetACLRules).Methods("GET")
	r.HandleFunc("/rules/{rule_id}/acl", h.CreateACLRule).Methods("POST")
	r.HandleFunc("/rules/{rule_id}/acl/{id}", h.UpdateACLRule).Methods("PUT")
	r.HandleFunc("/rules/{rule_id}/acl/{id}", h.DeleteACLRule).Methods("DELETE")
	r.HandleFunc("/rules/{rule_id}/acl/check", h.CheckAccess).Methods("POST")
}

// GetACLRules 获取规则的ACL规则
func (h *ACLHandler) GetACLRules(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	ruleID := vars["rule_id"]

	// 检查规则是否存在
	_, err := h.ruleService.GetRule(ruleID)
	if err != nil {
		h.logger.Error("获取规则失败",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		response.NotFound(w, "规则不存在", err)
		return
	}

	// 获取ACL规则
	rules, err := h.aclService.GetACLRules(ruleID)
	if err != nil {
		h.logger.Error("获取ACL规则失败",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取ACL规则失败", err)
		return
	}

	response.Success(w, rules)
}

// CreateACLRule 创建ACL规则
func (h *ACLHandler) CreateACLRule(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	ruleID := vars["rule_id"]

	// 检查规则是否存在
	_, err := h.ruleService.GetRule(ruleID)
	if err != nil {
		h.logger.Error("获取规则失败",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		response.NotFound(w, "规则不存在", err)
		return
	}

	// 解析请求体
	var acl model.ACLRule
	if err := json.NewDecoder(r.Body).Decode(&acl); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 设置规则ID
	acl.RuleID = ruleID

	// 创建ACL规则
	if err := h.aclService.CreateACLRule(&acl); err != nil {
		h.logger.Error("创建ACL规则失败",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "创建ACL规则失败", err)
		return
	}

	response.Created(w, acl)
}

// UpdateACLRule 更新ACL规则
func (h *ACLHandler) UpdateACLRule(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	ruleID := vars["rule_id"]
	id := vars["id"]

	// 检查规则是否存在
	_, err := h.ruleService.GetRule(ruleID)
	if err != nil {
		h.logger.Error("获取规则失败",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		response.NotFound(w, "规则不存在", err)
		return
	}

	// 解析请求体
	var acl model.ACLRule
	if err := json.NewDecoder(r.Body).Decode(&acl); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 设置ID和规则ID
	acl.ID = id
	acl.RuleID = ruleID

	// 更新ACL规则
	if err := h.aclService.UpdateACLRule(&acl); err != nil {
		h.logger.Error("更新ACL规则失败",
			zap.String("id", id),
			zap.String("rule_id", ruleID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "更新ACL规则失败", err)
		return
	}

	response.Success(w, acl)
}

// DeleteACLRule 删除ACL规则
func (h *ACLHandler) DeleteACLRule(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	ruleID := vars["rule_id"]
	id := vars["id"]

	// 删除ACL规则
	if err := h.aclService.DeleteACLRule(id, ruleID); err != nil {
		h.logger.Error("删除ACL规则失败",
			zap.String("id", id),
			zap.String("rule_id", ruleID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "删除ACL规则失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "ACL规则已删除",
		"id":      id,
		"rule_id": ruleID,
	})
}

// CheckAccess 检查访问权限
func (h *ACLHandler) CheckAccess(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	ruleID := vars["rule_id"]

	// 检查规则是否存在
	_, err := h.ruleService.GetRule(ruleID)
	if err != nil {
		h.logger.Error("获取规则失败",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		response.NotFound(w, "规则不存在", err)
		return
	}

	// 解析请求体
	var req struct {
		SourceIP string `json:"source_ip"`
		DestIP   string `json:"dest_ip"`
		Protocol string `json:"protocol"`
		Port     int    `json:"port"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 检查访问权限
	allowed, err := h.aclService.CheckAccess(ruleID, req.SourceIP, req.DestIP, req.Protocol, req.Port)
	if err != nil {
		h.logger.Error("检查访问权限失败",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "检查访问权限失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"allowed": allowed,
		"rule_id": ruleID,
		"request": req,
	})
}
