package handler

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/service"
)

// WebSocketHandler WebSocket处理器
type WebSocketHandler struct {
	wsService *service.WebSocketService
	logger    *zap.Logger
	upgrader  websocket.Upgrader
}

// NewWebSocketHandler 创建WebSocket处理器
func NewWebSocketHandler(wsService *service.WebSocketService) *WebSocketHandler {
	return &WebSocketHandler{
		wsService: wsService,
		logger:    zap.L().Named("websocket-handler"),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// 在生产环境中，应该检查Origin头
				return true
			},
		},
	}
}

// RegisterRoutes 注册路由
func (h *WebSocketHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/ws/node", h.HandleNodeConnection).Methods("GET")
	r.HandleFunc("/ws/admin", h.HandleAdminConnection).Methods("GET")
}

// HandleNodeConnection 处理节点WebSocket连接
func (h *WebSocketHandler) HandleNodeConnection(w http.ResponseWriter, r *http.Request) {
	// 获取节点ID
	nodeID := r.URL.Query().Get("node_id")
	if nodeID == "" {
		response.BadRequest(w, "缺少node_id参数", nil)
		return
	}

	// 升级HTTP连接为WebSocket连接
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("升级WebSocket连接失败",
			zap.Error(err),
			zap.String("node_id", nodeID))
		return
	}

	// 处理WebSocket连接
	h.wsService.HandleNodeConnection(conn, nodeID)
}

// HandleAdminConnection 处理管理员WebSocket连接
func (h *WebSocketHandler) HandleAdminConnection(w http.ResponseWriter, r *http.Request) {
	// 获取用户信息
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		response.Forbidden(w, "需要管理员权限", nil)
		return
	}

	// 升级HTTP连接为WebSocket连接
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("升级WebSocket连接失败",
			zap.Error(err),
			zap.String("user_id", claims.UserID))
		return
	}

	// 处理WebSocket连接
	h.wsService.HandleAdminConnection(conn, claims.UserID)
}
