package plane

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"gkipass/client/internal/auth"
	"gkipass/client/internal/identity"
)

// ConnectionStatus 连接状态
type ConnectionStatus string

const (
	StatusDisconnected ConnectionStatus = "disconnected"
	StatusConnecting   ConnectionStatus = "connecting"
	StatusConnected    ConnectionStatus = "connected"
	StatusReconnecting ConnectionStatus = "reconnecting"
	StatusError        ConnectionStatus = "error"
)

// ConnectionConfig 连接配置
type ConnectionConfig struct {
	URL                  string        `json:"url"`                    // Plane服务器地址
	APIKey               string        `json:"api_key"`                // API密钥
	ConnectTimeout       time.Duration `json:"connect_timeout"`        // 连接超时
	ReconnectInterval    time.Duration `json:"reconnect_interval"`     // 重连间隔
	MaxReconnectAttempts int           `json:"max_reconnect_attempts"` // 最大重连次数
	HeartbeatInterval    time.Duration `json:"heartbeat_interval"`     // 心跳间隔
	WriteTimeout         time.Duration `json:"write_timeout"`          // 写超时
	ReadTimeout          time.Duration `json:"read_timeout"`           // 读超时
	TLSVerify            bool          `json:"tls_verify"`             // 验证TLS
	TLSCertFile          string        `json:"tls_cert_file"`          // TLS证书文件
	TLSKeyFile           string        `json:"tls_key_file"`           // TLS密钥文件
	TLSCAFile            string        `json:"tls_ca_file"`            // TLS CA文件
}

// DefaultConnectionConfig 默认连接配置
func DefaultConnectionConfig() *ConnectionConfig {
	return &ConnectionConfig{
		ConnectTimeout:       30 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 0, // 无限重连
		HeartbeatInterval:    30 * time.Second,
		WriteTimeout:         10 * time.Second,
		ReadTimeout:          60 * time.Second,
		TLSVerify:            true,
	}
}

// Connection Plane连接
type Connection struct {
	config          *ConnectionConfig
	authManager     *auth.Manager
	identityManager *identity.Manager
	logger          *zap.Logger

	conn           *websocket.Conn
	status         ConnectionStatus
	statusMu       sync.RWMutex
	reconnectCount int
	lastError      error
	lastActivity   time.Time

	handlers   map[string]MessageHandler
	handlersMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// MessageHandler 消息处理器
type MessageHandler func(msg *Message) error

// Message Plane消息
type Message struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
	Error     *MessageError   `json:"error,omitempty"`
}

// MessageError 消息错误
type MessageError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewConnection 创建Plane连接
func NewConnection(config *ConnectionConfig, authManager *auth.Manager, identityManager *identity.Manager) (*Connection, error) {
	if config == nil {
		config = DefaultConnectionConfig()
	}

	if authManager == nil {
		return nil, fmt.Errorf("auth manager is required")
	}

	if identityManager == nil {
		return nil, fmt.Errorf("identity manager is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Connection{
		config:          config,
		authManager:     authManager,
		identityManager: identityManager,
		logger:          zap.L().Named("plane-connection"),
		status:          StatusDisconnected,
		handlers:        make(map[string]MessageHandler),
		ctx:             ctx,
		cancel:          cancel,
	}

	// 注册内置处理器
	c.RegisterHandler("ping", c.handlePing)
	c.RegisterHandler("auth_result", c.handleAuthResult)
	c.RegisterHandler("config_update", c.handleConfigUpdate)
	c.RegisterHandler("rule_update", c.handleRuleUpdate)
	c.RegisterHandler("command", c.handleCommand)

	return c, nil
}

// Start 启动连接
func (c *Connection) Start() error {
	c.logger.Info("启动Plane连接")

	// 启动连接协程
	c.wg.Add(1)
	go c.connectionLoop()

	return nil
}

// Stop 停止连接
func (c *Connection) Stop() error {
	c.logger.Info("停止Plane连接")

	// 取消上下文
	c.cancel()

	// 关闭连接
	c.closeConnection()

	// 等待协程结束
	c.wg.Wait()

	return nil
}

// GetStatus 获取连接状态
func (c *Connection) GetStatus() ConnectionStatus {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.status
}

// RegisterHandler 注册消息处理器
func (c *Connection) RegisterHandler(msgType string, handler MessageHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.handlers[msgType] = handler
}

// SendMessage 发送消息
func (c *Connection) SendMessage(msgType string, data interface{}) error {
	c.statusMu.RLock()
	if c.status != StatusConnected {
		c.statusMu.RUnlock()
		return fmt.Errorf("连接未建立，当前状态: %s", c.status)
	}
	c.statusMu.RUnlock()

	msg := &Message{
		Type:      msgType,
		ID:        generateMessageID(),
		Timestamp: time.Now().Unix(),
	}

	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("序列化消息数据失败: %w", err)
		}
		msg.Data = jsonData
	}

	// 设置写超时
	if c.config.WriteTimeout > 0 {
		c.conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
	}

	// 发送消息
	if err := c.conn.WriteJSON(msg); err != nil {
		c.setLastError(err)
		return fmt.Errorf("发送消息失败: %w", err)
	}

	// 更新最后活动时间
	c.lastActivity = time.Now()

	return nil
}

// connectionLoop 连接循环
func (c *Connection) connectionLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// 尝试连接
			if err := c.connect(); err != nil {
				c.setLastError(err)
				c.logger.Error("连接Plane服务器失败",
					zap.Error(err),
					zap.Int("reconnect_count", c.reconnectCount))

				// 检查是否达到最大重连次数
				if c.config.MaxReconnectAttempts > 0 && c.reconnectCount >= c.config.MaxReconnectAttempts {
					c.logger.Error("达到最大重连次数，停止重连")
					c.setStatus(StatusError)
					return
				}

				// 等待重连
				c.setStatus(StatusReconnecting)
				select {
				case <-c.ctx.Done():
					return
				case <-time.After(c.config.ReconnectInterval):
					// 继续重连
				}
				continue
			}

			// 连接成功，启动读写协程
			c.setStatus(StatusConnected)
			c.reconnectCount = 0 // 重置重连计数

			// 启动心跳协程
			heartbeatCtx, heartbeatCancel := context.WithCancel(c.ctx)
			heartbeatDone := make(chan struct{})
			go func() {
				defer close(heartbeatDone)
				c.heartbeatLoop(heartbeatCtx)
			}()

			// 读取消息
			c.readLoop()

			// 停止心跳
			heartbeatCancel()
			<-heartbeatDone

			// 连接已断开，尝试重连
			c.reconnectCount++
		}
	}
}

// connect 连接到Plane服务器
func (c *Connection) connect() error {
	c.setStatus(StatusConnecting)

	// 解析URL
	u, err := url.Parse(c.config.URL)
	if err != nil {
		return fmt.Errorf("解析URL失败: %w", err)
	}

	// 确保URL是WebSocket URL
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// 已经是WebSocket URL，不需要修改
	default:
		return fmt.Errorf("不支持的URL协议: %s", u.Scheme)
	}

	// 确保路径以/ws结尾
	if !strings.HasSuffix(u.Path, "/ws") {
		if u.Path == "" || u.Path == "/" {
			u.Path = "/ws"
		} else {
			u.Path = strings.TrimSuffix(u.Path, "/") + "/ws"
		}
	}

	// 添加查询参数
	query := u.Query()
	nodeID := c.identityManager.GetNodeID()
	if nodeID != "" {
		query.Set("node_id", nodeID)
	}
	if c.config.APIKey != "" {
		query.Set("api_key", c.config.APIKey)
	}
	u.RawQuery = query.Encode()

	// 创建HTTP头部
	header := http.Header{}
	header.Set("User-Agent", "GKiPass-Client/1.0")

	// 获取认证令牌
	authResult := c.authManager.GetAuthResult()
	if authResult != nil && authResult.Success && authResult.Token != "" {
		header.Set("Authorization", fmt.Sprintf("Bearer %s", authResult.Token))
	}

	// 创建WebSocket拨号器
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: c.config.ConnectTimeout,
	}

	// 设置TLS配置
	if u.Scheme == "wss" {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: !c.config.TLSVerify,
			MinVersion:         tls.VersionTLS12,
		}
		dialer.TLSClientConfig = tlsConfig
	}

	// 连接到WebSocket服务器
	c.logger.Debug("连接到Plane服务器",
		zap.String("url", u.String()))

	conn, resp, err := dialer.Dial(u.String(), header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("连接失败: HTTP状态码 %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("连接失败: %w", err)
	}

	c.conn = conn
	c.lastActivity = time.Now()

	c.logger.Info("已连接到Plane服务器",
		zap.String("url", u.String()),
		zap.String("local_addr", conn.LocalAddr().String()),
		zap.String("remote_addr", conn.RemoteAddr().String()))

	// 发送认证消息
	if err := c.sendAuthMessage(); err != nil {
		c.closeConnection()
		return fmt.Errorf("发送认证消息失败: %w", err)
	}

	return nil
}

// closeConnection 关闭连接
func (c *Connection) closeConnection() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.setStatus(StatusDisconnected)
}

// readLoop 读取消息循环
func (c *Connection) readLoop() {
	for {
		// 设置读超时
		if c.config.ReadTimeout > 0 {
			c.conn.SetReadDeadline(time.Now().Add(c.config.ReadTimeout))
		}

		// 读取消息
		var msg Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			c.setLastError(err)
			c.logger.Error("读取消息失败", zap.Error(err))
			break
		}

		// 更新最后活动时间
		c.lastActivity = time.Now()

		// 处理消息
		if err := c.handleMessage(&msg); err != nil {
			c.logger.Error("处理消息失败",
				zap.String("type", msg.Type),
				zap.Error(err))
		}
	}

	// 连接已断开
	c.closeConnection()
}

// heartbeatLoop 心跳循环
func (c *Connection) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(c.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 发送心跳
			if err := c.SendMessage("ping", map[string]interface{}{
				"timestamp": time.Now().Unix(),
			}); err != nil {
				c.logger.Error("发送心跳失败", zap.Error(err))
			}
		}
	}
}

// handleMessage 处理消息
func (c *Connection) handleMessage(msg *Message) error {
	c.logger.Debug("收到消息",
		zap.String("type", msg.Type),
		zap.String("id", msg.ID))

	// 查找处理器
	c.handlersMu.RLock()
	handler, ok := c.handlers[msg.Type]
	c.handlersMu.RUnlock()

	if !ok {
		return fmt.Errorf("未知的消息类型: %s", msg.Type)
	}

	// 调用处理器
	return handler(msg)
}

// setStatus 设置连接状态
func (c *Connection) setStatus(status ConnectionStatus) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.status = status
}

// setLastError 设置最后一个错误
func (c *Connection) setLastError(err error) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.lastError = err
}

// sendAuthMessage 发送认证消息
func (c *Connection) sendAuthMessage() error {
	// 获取节点ID
	nodeID := c.identityManager.GetNodeID()
	if nodeID == "" {
		return fmt.Errorf("节点ID为空")
	}

	// 获取认证令牌
	token := c.authManager.GetAuthResult()
	if token == nil || !token.Success {
		return fmt.Errorf("未认证")
	}

	// 获取身份信息
	identity := c.identityManager.GetIdentity()
	if identity == nil {
		return fmt.Errorf("身份信息为空")
	}

	// 构建认证消息
	authData := map[string]interface{}{
		"node_id":     nodeID,
		"token":       token.Token,
		"node_name":   identity.NodeName,
		"hardware_id": identity.HardwareID,
		"system_info": identity.SystemInfo,
		"timestamp":   time.Now().Unix(),
	}

	// 发送认证消息
	return c.SendMessage("auth", authData)
}

// handlePing 处理ping消息
func (c *Connection) handlePing(msg *Message) error {
	// 回复pong
	return c.SendMessage("pong", map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"echo":      msg.Data,
	})
}

// handleAuthResult 处理认证结果消息
func (c *Connection) handleAuthResult(msg *Message) error {
	var result struct {
		Success  bool     `json:"success"`
		Message  string   `json:"message"`
		Role     string   `json:"role"`
		Groups   []string `json:"groups"`
		NodeName string   `json:"node_name"`
	}

	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return fmt.Errorf("解析认证结果失败: %w", err)
	}

	if !result.Success {
		c.logger.Error("Plane认证失败",
			zap.String("message", result.Message))
		return fmt.Errorf("认证失败: %s", result.Message)
	}

	c.logger.Info("Plane认证成功",
		zap.String("role", result.Role),
		zap.Strings("groups", result.Groups))

	return nil
}

// handleConfigUpdate 处理配置更新消息
func (c *Connection) handleConfigUpdate(msg *Message) error {
	// 解析配置更新
	var configUpdate map[string]interface{}
	if err := json.Unmarshal(msg.Data, &configUpdate); err != nil {
		return fmt.Errorf("解析配置更新失败: %w", err)
	}

	c.logger.Info("收到配置更新",
		zap.Any("config", configUpdate))

	// TODO: 应用配置更新

	// 回复确认
	return c.SendMessage("config_update_ack", map[string]interface{}{
		"status":    "success",
		"message":   "配置已更新",
		"timestamp": time.Now().Unix(),
	})
}

// handleRuleUpdate 处理规则更新消息
func (c *Connection) handleRuleUpdate(msg *Message) error {
	// 解析规则更新
	var ruleUpdate struct {
		Rules    []interface{} `json:"rules"`
		Version  int64         `json:"version"`
		FullSync bool          `json:"full_sync"`
	}
	if err := json.Unmarshal(msg.Data, &ruleUpdate); err != nil {
		return fmt.Errorf("解析规则更新失败: %w", err)
	}

	c.logger.Info("收到规则更新",
		zap.Int("rule_count", len(ruleUpdate.Rules)),
		zap.Int64("version", ruleUpdate.Version),
		zap.Bool("full_sync", ruleUpdate.FullSync))

	// TODO: 应用规则更新

	// 回复确认
	return c.SendMessage("rule_update_ack", map[string]interface{}{
		"status":    "success",
		"message":   "规则已更新",
		"version":   ruleUpdate.Version,
		"timestamp": time.Now().Unix(),
	})
}

// handleCommand 处理命令消息
func (c *Connection) handleCommand(msg *Message) error {
	// 解析命令
	var command struct {
		Command string                 `json:"command"`
		Params  map[string]interface{} `json:"params"`
	}
	if err := json.Unmarshal(msg.Data, &command); err != nil {
		return fmt.Errorf("解析命令失败: %w", err)
	}

	c.logger.Info("收到命令",
		zap.String("command", command.Command),
		zap.Any("params", command.Params))

	// TODO: 执行命令

	// 回复确认
	return c.SendMessage("command_ack", map[string]interface{}{
		"status":    "success",
		"message":   "命令已执行",
		"command":   command.Command,
		"timestamp": time.Now().Unix(),
	})
}

// generateMessageID 生成消息ID
func generateMessageID() string {
	// 生成随机字节
	buf := make([]byte, 4)
	_, err := rand.Read(buf)
	if err != nil {
		// 如果随机数生成失败，使用时间戳
		return fmt.Sprintf("%d-%x", time.Now().UnixNano(), time.Now().Unix()%1000000)
	}

	// 结合时间戳和随机数
	return fmt.Sprintf("%d-%x", time.Now().UnixNano(), buf)
}

// GetStats 获取统计信息
func (c *Connection) GetStats() map[string]interface{} {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()

	var lastErrorStr string
	if c.lastError != nil {
		lastErrorStr = c.lastError.Error()
	}

	return map[string]interface{}{
		"status":          string(c.status),
		"reconnect_count": c.reconnectCount,
		"last_activity":   c.lastActivity,
		"last_error":      lastErrorStr,
		"connected":       c.status == StatusConnected,
	}
}
