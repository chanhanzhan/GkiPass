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

// NodeHandler 节点处理器
type NodeHandler struct {
	nodeService *service.NodeService
	ruleService *service.RuleService
	logger      *zap.Logger
}

// NewNodeHandler 创建节点处理器
func NewNodeHandler(nodeService *service.NodeService, ruleService *service.RuleService) *NodeHandler {
	return &NodeHandler{
		nodeService: nodeService,
		ruleService: ruleService,
		logger:      zap.L().Named("node-handler"),
	}
}

// RegisterRoutes 注册路由
func (h *NodeHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/nodes", h.ListNodes).Methods("GET")
	r.HandleFunc("/nodes", h.CreateNode).Methods("POST")
	r.HandleFunc("/nodes/{id}", h.GetNode).Methods("GET")
	r.HandleFunc("/nodes/{id}", h.UpdateNode).Methods("PUT")
	r.HandleFunc("/nodes/{id}", h.DeleteNode).Methods("DELETE")
	r.HandleFunc("/nodes/{id}/status", h.UpdateNodeStatus).Methods("PUT")
	r.HandleFunc("/nodes/{id}/groups", h.GetNodeGroups).Methods("GET")
}

// ListNodes 列出所有节点
func (h *NodeHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	query := r.URL.Query()
	status := query.Get("status")
	role := query.Get("role")

	// 获取节点
	nodes, err := h.nodeService.ListNodes()
	if err != nil {
		h.logger.Error("列出节点失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取节点列表失败", err)
		return
	}

	// 过滤节点
	var filteredNodes []*model.Node
	for _, node := range nodes {
		// 过滤状态
		if status != "" && string(node.Status) != status {
			continue
		}

		// 过滤角色
		if role != "" && string(node.Role) != role {
			continue
		}

		filteredNodes = append(filteredNodes, node)
	}

	response.Success(w, filteredNodes)
}

// CreateNode 创建节点
func (h *NodeHandler) CreateNode(w http.ResponseWriter, r *http.Request) {
	// 解析请求体
	var node model.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 创建节点
	if err := h.nodeService.CreateNode(&node); err != nil {
		h.logger.Error("创建节点失败",
			zap.String("name", node.Name),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "创建节点失败", err)
		return
	}

	response.Created(w, node)
}

// GetNode 获取节点
func (h *NodeHandler) GetNode(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 获取节点
	node, err := h.nodeService.GetNode(id)
	if err != nil {
		h.logger.Error("获取节点失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusNotFound, "节点不存在", err)
		return
	}

	// 构建响应
	nodeResp := map[string]interface{}{
		"node": node,
	}

	// 获取节点所属的组
	groups, err := h.nodeService.GetNodeGroups(id)
	if err != nil {
		h.logger.Warn("获取节点所属组失败",
			zap.String("id", id),
			zap.Error(err))
	} else {
		// 收集组ID
		groupIDs := make([]string, len(groups))
		for i, group := range groups {
			groupIDs[i] = group.ID
		}
		nodeResp["groups"] = groupIDs
	}

	response.Success(w, nodeResp)
}

// UpdateNode 更新节点
func (h *NodeHandler) UpdateNode(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 解析请求体
	var node model.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 确保ID一致
	node.ID = id

	// 更新节点
	if err := h.nodeService.UpdateNode(&node); err != nil {
		h.logger.Error("更新节点失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "更新节点失败", err)
		return
	}

	response.Success(w, node)
}

// DeleteNode 删除节点
func (h *NodeHandler) DeleteNode(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 删除节点
	if err := h.nodeService.DeleteNode(id); err != nil {
		h.logger.Error("删除节点失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "删除节点失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "节点已删除",
		"id":      id,
	})
}

// UpdateNodeStatus 更新节点状态
func (h *NodeHandler) UpdateNodeStatus(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 解析请求体
	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 更新节点状态
	if err := h.nodeService.UpdateNodeStatus(id, req.Status); err != nil {
		h.logger.Error("更新节点状态失败",
			zap.String("id", id),
			zap.String("status", req.Status),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "更新节点状态失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "节点状态已更新",
		"id":      id,
		"status":  req.Status,
	})
}

// GetNodeGroups 获取节点所属的组
func (h *NodeHandler) GetNodeGroups(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 获取节点所属的组
	groups, err := h.nodeService.GetNodeGroups(id)
	if err != nil {
		h.logger.Error("获取节点所属组失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取节点所属组失败", err)
		return
	}

	response.Success(w, groups)
}
