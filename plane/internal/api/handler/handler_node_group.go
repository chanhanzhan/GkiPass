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

// NodeGroupHandler 节点组处理器
type NodeGroupHandler struct {
	nodeGroupService *service.NodeGroupService
	logger           *zap.Logger
}

// NewNodeGroupHandler 创建节点组处理器
func NewNodeGroupHandler(nodeGroupService *service.NodeGroupService) *NodeGroupHandler {
	return &NodeGroupHandler{
		nodeGroupService: nodeGroupService,
		logger:           zap.L().Named("node-group-handler"),
	}
}

// RegisterRoutes 注册路由
func (h *NodeGroupHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/api/v1/node-groups", h.ListNodeGroups).Methods("GET")
	r.HandleFunc("/api/v1/node-groups", h.CreateNodeGroup).Methods("POST")
	r.HandleFunc("/api/v1/node-groups/{id}", h.GetNodeGroup).Methods("GET")
	r.HandleFunc("/api/v1/node-groups/{id}", h.UpdateNodeGroup).Methods("PUT")
	r.HandleFunc("/api/v1/node-groups/{id}", h.DeleteNodeGroup).Methods("DELETE")
	r.HandleFunc("/api/v1/node-groups/{id}/nodes", h.GetNodeGroupNodes).Methods("GET")
	r.HandleFunc("/api/v1/node-groups/{id}/nodes/{nodeId}", h.AddNodeToGroup).Methods("PUT")
	r.HandleFunc("/api/v1/node-groups/{id}/nodes/{nodeId}", h.RemoveNodeFromGroup).Methods("DELETE")
	r.HandleFunc("/api/v1/nodes/{id}/groups", h.GetNodeGroups).Methods("GET")
	r.HandleFunc("/api/v1/node-groups/ingress", h.GetIngressGroups).Methods("GET")
	r.HandleFunc("/api/v1/node-groups/egress", h.GetEgressGroups).Methods("GET")
}

// ListNodeGroups 列出所有节点组
func (h *NodeGroupHandler) ListNodeGroups(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	query := r.URL.Query()
	role := query.Get("role")

	var groups []*model.NodeGroup
	var err error

	// 根据角色筛选
	if role != "" {
		switch role {
		case "ingress":
			groups, err = h.nodeGroupService.GetIngressGroups()
		case "egress":
			groups, err = h.nodeGroupService.GetEgressGroups()
		default:
			groups, err = h.nodeGroupService.ListNodeGroups()
		}
	} else {
		groups, err = h.nodeGroupService.ListNodeGroups()
	}

	if err != nil {
		h.logger.Error("列出节点组失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取节点组列表失败", err)
		return
	}

	response.JSON(w, http.StatusOK, groups)
}

// CreateNodeGroup 创建节点组
func (h *NodeGroupHandler) CreateNodeGroup(w http.ResponseWriter, r *http.Request) {
	// 解析请求体
	var group model.NodeGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 创建节点组
	if err := h.nodeGroupService.CreateNodeGroup(&group); err != nil {
		h.logger.Error("创建节点组失败",
			zap.String("name", group.Name),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "创建节点组失败", err)
		return
	}

	response.JSON(w, http.StatusCreated, group)
}

// GetNodeGroup 获取节点组
func (h *NodeGroupHandler) GetNodeGroup(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 获取节点组
	group, err := h.nodeGroupService.GetNodeGroup(id)
	if err != nil {
		h.logger.Error("获取节点组失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusNotFound, "节点组不存在", err)
		return
	}

	response.JSON(w, http.StatusOK, group)
}

// UpdateNodeGroup 更新节点组
func (h *NodeGroupHandler) UpdateNodeGroup(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 解析请求体
	var group model.NodeGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 确保ID一致
	group.ID = id

	// 更新节点组
	if err := h.nodeGroupService.UpdateNodeGroup(&group); err != nil {
		h.logger.Error("更新节点组失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "更新节点组失败", err)
		return
	}

	response.JSON(w, http.StatusOK, group)
}

// DeleteNodeGroup 删除节点组
func (h *NodeGroupHandler) DeleteNodeGroup(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 删除节点组
	if err := h.nodeGroupService.DeleteNodeGroup(id); err != nil {
		h.logger.Error("删除节点组失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "删除节点组失败", err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"message": "节点组已删除",
		"id":      id,
	})
}

// GetNodeGroupNodes 获取节点组中的节点
func (h *NodeGroupHandler) GetNodeGroupNodes(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 获取节点组
	group, err := h.nodeGroupService.GetNodeGroup(id)
	if err != nil {
		h.logger.Error("获取节点组失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusNotFound, "节点组不存在", err)
		return
	}

	// TODO: 获取节点详情

	response.JSON(w, http.StatusOK, group.Nodes)
}

// AddNodeToGroup 将节点添加到组
func (h *NodeGroupHandler) AddNodeToGroup(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	groupID := vars["id"]
	nodeID := vars["nodeId"]

	// 添加节点到组
	if err := h.nodeGroupService.AddNodeToGroup(groupID, nodeID); err != nil {
		h.logger.Error("添加节点到组失败",
			zap.String("group_id", groupID),
			zap.String("node_id", nodeID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "添加节点到组失败", err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"message":  "节点已添加到组",
		"group_id": groupID,
		"node_id":  nodeID,
	})
}

// RemoveNodeFromGroup 从组中移除节点
func (h *NodeGroupHandler) RemoveNodeFromGroup(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	groupID := vars["id"]
	nodeID := vars["nodeId"]

	// 从组中移除节点
	if err := h.nodeGroupService.RemoveNodeFromGroup(groupID, nodeID); err != nil {
		h.logger.Error("从组中移除节点失败",
			zap.String("group_id", groupID),
			zap.String("node_id", nodeID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "从组中移除节点失败", err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"message":  "节点已从组中移除",
		"group_id": groupID,
		"node_id":  nodeID,
	})
}

// GetNodeGroups 获取节点所属的组
func (h *NodeGroupHandler) GetNodeGroups(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	nodeID := vars["id"]

	// 获取节点所属的组
	groups, err := h.nodeGroupService.GetNodeGroups(nodeID)
	if err != nil {
		h.logger.Error("获取节点所属组失败",
			zap.String("node_id", nodeID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取节点所属组失败", err)
		return
	}

	response.JSON(w, http.StatusOK, groups)
}

// GetIngressGroups 获取所有入口组
func (h *NodeGroupHandler) GetIngressGroups(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	query := r.URL.Query()
	limitStr := query.Get("limit")

	limit := 0
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			h.logger.Error("解析limit参数失败", zap.Error(err))
			response.Error(w, http.StatusBadRequest, "无效的limit参数", err)
			return
		}
	}

	// 获取入口组
	groups, err := h.nodeGroupService.GetIngressGroups()
	if err != nil {
		h.logger.Error("获取入口组失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取入口组失败", err)
		return
	}

	// 限制结果数量
	if limit > 0 && limit < len(groups) {
		groups = groups[:limit]
	}

	response.JSON(w, http.StatusOK, groups)
}

// GetEgressGroups 获取所有出口组
func (h *NodeGroupHandler) GetEgressGroups(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	query := r.URL.Query()
	limitStr := query.Get("limit")

	limit := 0
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			h.logger.Error("解析limit参数失败", zap.Error(err))
			response.Error(w, http.StatusBadRequest, "无效的limit参数", err)
			return
		}
	}

	// 获取出口组
	groups, err := h.nodeGroupService.GetEgressGroups()
	if err != nil {
		h.logger.Error("获取出口组失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取出口组失败", err)
		return
	}

	// 限制结果数量
	if limit > 0 && limit < len(groups) {
		groups = groups[:limit]
	}

	response.JSON(w, http.StatusOK, groups)
}
