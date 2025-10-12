package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/service"
)

// DiagnosticsHandler 诊断处理器
type DiagnosticsHandler struct {
	diagnosticsService *service.DiagnosticsService
	userService        *service.UserService
	logger             *zap.Logger
}

// NewDiagnosticsHandler 创建诊断处理器
func NewDiagnosticsHandler(diagnosticsService *service.DiagnosticsService, userService *service.UserService) *DiagnosticsHandler {
	return &DiagnosticsHandler{
		diagnosticsService: diagnosticsService,
		userService:        userService,
		logger:             zap.L().Named("diagnostics-handler"),
	}
}

// RegisterRoutes 注册路由
func (h *DiagnosticsHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/diagnostics", h.ListDiagnosticResults).Methods("GET")
	r.HandleFunc("/diagnostics/{id}", h.GetDiagnosticResult).Methods("GET")
	r.HandleFunc("/diagnostics/node-connection", h.NodeConnectionDiagnostic).Methods("POST")
	r.HandleFunc("/diagnostics/tunnel/{id}", h.TunnelDiagnostic).Methods("POST")
}

// ListDiagnosticResults 列出诊断结果
func (h *DiagnosticsHandler) ListDiagnosticResults(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	query := r.URL.Query()
	sourceID := query.Get("source_id")
	targetID := query.Get("target_id")
	limitStr := query.Get("limit")

	// 解析limit参数
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

	// 获取诊断结果
	results, err := h.diagnosticsService.ListDiagnosticResults(sourceID, targetID, limit)
	if err != nil {
		h.logger.Error("列出诊断结果失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取诊断结果列表失败", err)
		return
	}

	response.Success(w, results)
}

// GetDiagnosticResult 获取诊断结果
func (h *DiagnosticsHandler) GetDiagnosticResult(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 获取诊断结果
	result, err := h.diagnosticsService.GetDiagnosticResult(id)
	if err != nil {
		h.logger.Error("获取诊断结果失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusNotFound, "诊断结果不存在", err)
		return
	}

	response.Success(w, result)
}

// NodeConnectionDiagnostic 节点连接诊断
func (h *DiagnosticsHandler) NodeConnectionDiagnostic(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 解析请求体
	var req struct {
		SourceID string `json:"source_id"`
		TargetID string `json:"target_id"`
		Type     string `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 检查探测权限
	canProbe, err := h.userService.CanProbe(claims.UserID, req.SourceID)
	if err != nil {
		h.logger.Error("检查探测权限失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "检查探测权限失败", err)
		return
	}

	if !canProbe {
		response.Forbidden(w, "没有探测权限", nil)
		return
	}

	// 执行诊断
	result, err := h.diagnosticsService.NodeConnectionDiagnostic(req.SourceID, req.TargetID, req.Type)
	if err != nil {
		h.logger.Error("节点连接诊断失败",
			zap.String("source_id", req.SourceID),
			zap.String("target_id", req.TargetID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "节点连接诊断失败", err)
		return
	}

	response.Success(w, result)
}

// TunnelDiagnostic 隧道诊断
func (h *DiagnosticsHandler) TunnelDiagnostic(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// TODO: 检查隧道权限

	// 执行诊断
	result, err := h.diagnosticsService.TunnelDiagnostic(id)
	if err != nil {
		h.logger.Error("隧道诊断失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "隧道诊断失败", err)
		return
	}

	response.Success(w, result)
}
