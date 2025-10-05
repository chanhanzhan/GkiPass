package ws

import (
	"sync"
	"time"

	"gkipass/plane/pkg/logger"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// NodeConnection 节点连接
type NodeConnection struct {
	NodeID   string
	Conn     *websocket.Conn
	Send     chan *Message
	LastSeen time.Time
	IsAlive  bool
	mu       sync.RWMutex
}

// Manager WebSocket 连接管理器
type Manager struct {
	connections map[string]*NodeConnection // nodeID -> connection
	register    chan *NodeConnection
	unregister  chan *NodeConnection
	broadcast   chan *Message
	mu          sync.RWMutex
}

// NewManager 创建连接管理器
func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]*NodeConnection),
		register:    make(chan *NodeConnection, 10),
		unregister:  make(chan *NodeConnection, 10),
		broadcast:   make(chan *Message, 100),
	}
}

// Run 运行管理器
func (m *Manager) Run() {
	logger.Info("WebSocket 管理器已启动")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case conn := <-m.register:
			m.registerNode(conn)

		case conn := <-m.unregister:
			m.unregisterNode(conn)

		case msg := <-m.broadcast:
			m.broadcastMessage(msg)

		case <-ticker.C:
			m.checkDeadConnections()
		}
	}
}

// registerNode 注册节点
func (m *Manager) registerNode(conn *NodeConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果节点已存在，先关闭旧连接
	if oldConn, exists := m.connections[conn.NodeID]; exists {
		close(oldConn.Send)
		oldConn.Conn.Close()
	}

	m.connections[conn.NodeID] = conn

	logger.Info("节点已连接",
		zap.String("nodeID", conn.NodeID),
		zap.Int("totalNodes", len(m.connections)))
}

// unregisterNode 注销节点
func (m *Manager) unregisterNode(conn *NodeConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.connections[conn.NodeID]; exists {
		delete(m.connections, conn.NodeID)
		close(conn.Send)

		logger.Info("节点已断开",
			zap.String("nodeID", conn.NodeID),
			zap.Int("totalNodes", len(m.connections)))
	}
}

// SendToNode 发送消息到指定节点
func (m *Manager) SendToNode(nodeID string, msg *Message) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[nodeID]
	if !exists {
		return ErrNodeNotConnected
	}

	select {
	case conn.Send <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return ErrSendTimeout
	}
}

// SendToGroup 发送消息到节点组
func (m *Manager) SendToGroup(nodeIDs []string, msg *Message) map[string]error {
	results := make(map[string]error)

	for _, nodeID := range nodeIDs {
		err := m.SendToNode(nodeID, msg)
		if err != nil {
			results[nodeID] = err
		}
	}

	return results
}

// BroadcastToAll 广播消息到所有节点
func (m *Manager) BroadcastToAll(msg *Message) {
	m.broadcast <- msg
}

// broadcastMessage 执行广播
func (m *Manager) broadcastMessage(msg *Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for nodeID, conn := range m.connections {
		select {
		case conn.Send <- msg:
		default:
			logger.Warn("发送广播消息失败（通道已满）",
				zap.String("nodeID", nodeID))
		}
	}
}

// GetConnection 获取节点连接
func (m *Manager) GetConnection(nodeID string) (*NodeConnection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[nodeID]
	return conn, exists
}

// GetAllNodeIDs 获取所有在线节点ID
func (m *Manager) GetAllNodeIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodeIDs := make([]string, 0, len(m.connections))
	for nodeID := range m.connections {
		nodeIDs = append(nodeIDs, nodeID)
	}

	return nodeIDs
}

// GetNodeCount 获取在线节点数量
func (m *Manager) GetNodeCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.connections)
}

// checkDeadConnections 检查死连接
func (m *Manager) checkDeadConnections() {
	m.mu.RLock()
	deadNodes := make([]*NodeConnection, 0)
	now := time.Now()

	for _, conn := range m.connections {
		conn.mu.RLock()
		if now.Sub(conn.LastSeen) > 90*time.Second {
			deadNodes = append(deadNodes, conn)
		}
		conn.mu.RUnlock()
	}
	m.mu.RUnlock()

	// 清理死连接
	for _, conn := range deadNodes {
		logger.Warn("节点超时，断开连接",
			zap.String("nodeID", conn.NodeID),
			zap.Duration("timeout", now.Sub(conn.LastSeen)))
		m.unregister <- conn
	}
}

// UpdateLastSeen 更新最后心跳时间
func (nc *NodeConnection) UpdateLastSeen() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.LastSeen = time.Now()
	nc.IsAlive = true
}

// Close 关闭连接
func (nc *NodeConnection) Close() {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if nc.Conn != nil {
		nc.Conn.Close()
	}
	nc.IsAlive = false
}

// Errors
var (
	ErrNodeNotConnected = &WSError{Code: "NODE_NOT_CONNECTED", Message: "节点未连接"}
	ErrSendTimeout      = &WSError{Code: "SEND_TIMEOUT", Message: "发送消息超时"}
)

// WSError WebSocket 错误
type WSError struct {
	Code    string
	Message string
}

func (e *WSError) Error() string {
	return e.Message
}

