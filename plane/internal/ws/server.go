package ws

import (
	"net/http"

	"gkipass/plane/db"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true 
		}
		
		// 允许的来源列表
		allowedOrigins := map[string]bool{
			"http://localhost:3000":  true,
			"https://localhost:3000": true,
			"http://127.0.0.1:3000":  true,
		}
		
		return allowedOrigins[origin]
	},
}

// Server WebSocket 服务器
type Server struct {
	manager *Manager
	handler *Handler
	db      *db.Manager
}

// NewServer 创建 WebSocket 服务器
func NewServer(dbManager *db.Manager) *Server {
	manager := NewManager()
	handler := NewHandler(manager, dbManager)

	return &Server{
		manager: manager,
		handler: handler,
		db:      dbManager,
	}
}

// Start 启动服务器
func (s *Server) Start() {
	go s.manager.Run()
	logger.Info("✓ WebSocket 服务器已启动")
}

// HandleWebSocket WebSocket 处理函数
func (s *Server) HandleWebSocket(c *gin.Context) {
	// 升级为 WebSocket 连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WebSocket 升级失败", zap.Error(err))
		return
	}

	// 处理连接
	s.handler.HandleConnection(conn)
}

// GetManager 获取管理器（用于外部调用）
func (s *Server) GetManager() *Manager {
	return s.manager
}

// GetHandler 获取处理器（用于外部调用）
func (s *Server) GetHandler() *Handler {
	return s.handler
}

// GetStats 获取统计信息
func (s *Server) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"online_nodes": s.manager.GetNodeCount(),
		"node_ids":     s.manager.GetAllNodeIDs(),
	}
}

