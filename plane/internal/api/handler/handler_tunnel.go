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

// TunnelHandler 隧道处理器
type TunnelHandler struct {
	tunnelService *service.TunnelService
	logger        *zap.Logger
}

// NewTunnelHandler 创建隧道处理器
func NewTunnelHandler(tunnelService *service.TunnelService) *TunnelHandler {
	return &TunnelHandler{
		tunnelService: tunnelService,
		logger:        zap.L().Named("tunnel-handler"),
	}
}

// RegisterRoutes 注册路由
func (h *TunnelHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/tunnels", h.ListTunnels).Methods("GET")
	r.HandleFunc("/tunnels", h.CreateTunnel).Methods("POST")
	r.HandleFunc("/tunnels/{id}", h.GetTunnel).Methods("GET")
	r.HandleFunc("/tunnels/{id}", h.UpdateTunnel).Methods("PUT")
	r.HandleFunc("/tunnels/{id}", h.DeleteTunnel).Methods("DELETE")
	r.HandleFunc("/tunnels/{id}/probe", h.ProbeTunnel).Methods("POST")
}

// ListTunnels 列出所有隧道
func (h *TunnelHandler) ListTunnels(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	query := r.URL.Query()
	enabled := query.Get("enabled")

	// 获取隧道
	tunnels, err := h.tunnelService.ListTunnels()
	if err != nil {
		h.logger.Error("列出隧道失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取隧道列表失败", err)
		return
	}

	// 过滤隧道
	var filteredTunnels []*model.Tunnel
	for _, tunnel := range tunnels {
		// 过滤启用状态
		if enabled != "" {
			enabledBool, err := strconv.ParseBool(enabled)
			if err == nil && tunnel.Enabled != enabledBool {
				continue
			}
		}

		filteredTunnels = append(filteredTunnels, tunnel)
	}

	response.Success(w, filteredTunnels)
}

// CreateTunnel 创建隧道
func (h *TunnelHandler) CreateTunnel(w http.ResponseWriter, r *http.Request) {
	// 获取用户ID
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 解析请求体
	var tunnel model.Tunnel
	if err := json.NewDecoder(r.Body).Decode(&tunnel); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 创建隧道
	if err := h.tunnelService.CreateTunnel(&tunnel, claims.UserID); err != nil {
		h.logger.Error("创建隧道失败",
			zap.String("name", tunnel.Name),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "创建隧道失败", err)
		return
	}

	response.Created(w, tunnel)
}

// GetTunnel 获取隧道
func (h *TunnelHandler) GetTunnel(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 获取隧道
	tunnel, err := h.tunnelService.GetTunnel(id)
	if err != nil {
		h.logger.Error("获取隧道失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusNotFound, "隧道不存在", err)
		return
	}

	response.Success(w, tunnel)
}

// UpdateTunnel 更新隧道
func (h *TunnelHandler) UpdateTunnel(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 解析请求体
	var tunnel model.Tunnel
	if err := json.NewDecoder(r.Body).Decode(&tunnel); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 确保ID一致
	tunnel.ID = id

	// 更新隧道
	if err := h.tunnelService.UpdateTunnel(&tunnel); err != nil {
		h.logger.Error("更新隧道失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "更新隧道失败", err)
		return
	}

	response.Success(w, tunnel)
}

// DeleteTunnel 删除隧道
func (h *TunnelHandler) DeleteTunnel(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 删除隧道
	if err := h.tunnelService.DeleteTunnel(id); err != nil {
		h.logger.Error("删除隧道失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "删除隧道失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "隧道已删除",
		"id":      id,
	})
}

// ProbeTunnel 探测隧道
func (h *TunnelHandler) ProbeTunnel(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 探测隧道
	result, err := h.tunnelService.ProbeTunnel(id)
	if err != nil {
		h.logger.Error("探测隧道失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "探测隧道失败", err)
		return
	}

	response.Success(w, result)
}
