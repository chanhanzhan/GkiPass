package service

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// WebSocketService WebSocket服务
type WebSocketService struct {
	logger      *zap.Logger
	connections sync.Map // nodeID -> WebSocketConnection
}

// WebSocketConnection WebSocket连接
type WebSocketConnection interface {
	Send(message interface{}) error
	Close() error
}

// NewWebSocketService 创建WebSocket服务
func NewWebSocketService() *WebSocketService {
	return &WebSocketService{
		logger: zap.L().Named("websocket-service"),
	}
}

// RegisterConnection 注册连接
func (s *WebSocketService) RegisterConnection(nodeID string, conn WebSocketConnection) {
	s.connections.Store(nodeID, conn)
	s.logger.Info("注册WebSocket连接", zap.String("node_id", nodeID))
}

// UnregisterConnection 注销连接
func (s *WebSocketService) UnregisterConnection(nodeID string) {
	if conn, ok := s.connections.LoadAndDelete(nodeID); ok {
		if wsConn, ok := conn.(WebSocketConnection); ok {
			wsConn.Close()
		}
		s.logger.Info("注销WebSocket连接", zap.String("node_id", nodeID))
	}
}

// SendMessageToNode 发送消息到节点
func (s *WebSocketService) SendMessageToNode(nodeID string, message interface{}) error {
	conn, ok := s.connections.Load(nodeID)
	if !ok {
		return fmt.Errorf("节点未连接: %s", nodeID)
	}

	wsConn, ok := conn.(WebSocketConnection)
	if !ok {
		return fmt.Errorf("无效的WebSocket连接")
	}

	if err := wsConn.Send(message); err != nil {
		s.logger.Error("发送消息失败",
			zap.String("node_id", nodeID),
			zap.Error(err))
		return fmt.Errorf("发送消息失败: %w", err)
	}

	return nil
}

// BroadcastMessage 广播消息
func (s *WebSocketService) BroadcastMessage(message interface{}) {
	s.connections.Range(func(key, value interface{}) bool {
		nodeID := key.(string)
		conn := value.(WebSocketConnection)

		if err := conn.Send(message); err != nil {
			s.logger.Error("广播消息失败",
				zap.String("node_id", nodeID),
				zap.Error(err))
		}

		return true
	})
}

// GetConnectedNodes 获取已连接节点
func (s *WebSocketService) GetConnectedNodes() []string {
	var nodes []string

	s.connections.Range(func(key, _ interface{}) bool {
		nodeID := key.(string)
		nodes = append(nodes, nodeID)
		return true
	})

	return nodes
}

// IsNodeConnected 节点是否已连接
func (s *WebSocketService) IsNodeConnected(nodeID string) bool {
	_, ok := s.connections.Load(nodeID)
	return ok
}

// GetConnectionCount 获取连接数量
func (s *WebSocketService) GetConnectionCount() int {
	count := 0
	s.connections.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// HandleNodeConnection 处理节点WebSocket连接
func (s *WebSocketService) HandleNodeConnection(conn interface{}, nodeID string) {
	// TODO: Implement node WebSocket connection handling
	// This should handle incoming messages from nodes and manage the connection lifecycle
	s.logger.Info("处理节点WebSocket连接", zap.String("node_id", nodeID))
}

// HandleAdminConnection 处理管理员WebSocket连接
func (s *WebSocketService) HandleAdminConnection(conn interface{}, userID string) {
	// TODO: Implement admin WebSocket connection handling
	// This should handle admin monitoring and control connections
	s.logger.Info("处理管理员WebSocket连接", zap.String("user_id", userID))
}
