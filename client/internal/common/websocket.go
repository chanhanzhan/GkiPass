package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WSMessageType WebSocket 消息类型
type WSMessageType string

const (
	// 认证和连接消息
	WSMessageTypeAuth       WSMessageType = "auth"
	WSMessageTypeAuthResult WSMessageType = "auth_result"
	WSMessageTypePing       WSMessageType = "ping"
	WSMessageTypePong       WSMessageType = "pong"
	WSMessageTypeDisconnect WSMessageType = "disconnect"

	// 规则和配置消息
	WSMessageTypeRuleSync        WSMessageType = "rule_sync"
	WSMessageTypeRuleSyncAck     WSMessageType = "rule_sync_ack"
	WSMessageTypeConfigUpdate    WSMessageType = "config_update"
	WSMessageTypeConfigUpdateAck WSMessageType = "config_update_ack"

	// 状态和监控消息
	WSMessageTypeStatusReport  WSMessageType = "status_report"
	WSMessageTypeMetricsReport WSMessageType = "metrics_report"
	WSMessageTypeLogReport     WSMessageType = "log_report"

	// 命令和控制消息
	WSMessageTypeCommand       WSMessageType = "command"
	WSMessageTypeCommandResult WSMessageType = "command_result"

	// 探测消息
	WSMessageTypeProbeRequest WSMessageType = "probe_request"
	WSMessageTypeProbeResult  WSMessageType = "probe_result"

	// 错误和通知消息
	WSMessageTypeError        WSMessageType = "error"
	WSMessageTypeNotification WSMessageType = "notification"
)

// WSMessage WebSocket 消息
type WSMessage struct {
	Type      WSMessageType   `json:"type"`
	ID        string          `json:"id,omitempty"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     *WSError        `json:"error,omitempty"`
}

// WSError WebSocket 错误信息
type WSError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WSClient WebSocket 客户端配置
type WSClientConfig struct {
	URL               string           // WebSocket 服务端地址
	ReconnectInterval time.Duration    // 重连间隔
	MaxReconnect      int              // 最大重连次数
	PingInterval      time.Duration    // 心跳间隔
	PongTimeout       time.Duration    // 心跳响应超时
	WriteTimeout      time.Duration    // 写入超时
	ReadTimeout       time.Duration    // 读取超时
	HandshakeTimeout  time.Duration    // 握手超时
	EnableCompression bool             // 启用压缩
	BufferSize        int              // 消息缓冲区大小
	Logger            *zap.Logger      // 日志记录器
	OnOpen            func()           // 连接打开回调
	OnClose           func()           // 连接关闭回调
	OnError           func(error)      // 错误回调
	OnMessage         func(*WSMessage) // 消息回调
	OnReconnect       func(int)        // 重连回调
	Headers           http.Header      // 自定义HTTP头部
}

// DefaultWSClientConfig 默认 WebSocket 客户端配置
func DefaultWSClientConfig() *WSClientConfig {
	return &WSClientConfig{
		ReconnectInterval: 5 * time.Second,
		MaxReconnect:      -1, // 无限重连
		PingInterval:      30 * time.Second,
		PongTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		ReadTimeout:       10 * time.Second,
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: true,
		BufferSize:        256,
		Logger:            zap.L().Named("ws-client"),
		Headers:           make(http.Header),
	}
}

// WSClient WebSocket 客户端
type WSClient struct {
	config      *WSClientConfig
	conn        *websocket.Conn
	sendCh      chan *WSMessage
	done        chan struct{}
	reconnectCh chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	connected   bool
	closed      bool
	reconnects  int
	lastError   error
	dialer      *websocket.Dialer
}

// NewWSClient 创建 WebSocket 客户端
func NewWSClient(config *WSClientConfig) *WSClient {
	if config == nil {
		config = DefaultWSClientConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	dialer := &websocket.Dialer{
		HandshakeTimeout:  config.HandshakeTimeout,
		EnableCompression: config.EnableCompression,
		TLSClientConfig:   nil, // 可根据需要配置TLS
	}

	return &WSClient{
		config:      config,
		sendCh:      make(chan *WSMessage, config.BufferSize),
		done:        make(chan struct{}),
		reconnectCh: make(chan struct{}, 1),
		ctx:         ctx,
		cancel:      cancel,
		dialer:      dialer,
	}
}

// Connect 连接到 WebSocket 服务器
func (c *WSClient) Connect() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("client is closed")
	}

	if c.connected {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	// 建立连接
	c.config.Logger.Debug("正在连接到WebSocket服务器", zap.String("url", c.config.URL))
	conn, resp, err := c.dialer.Dial(c.config.URL, c.config.Headers)
	if err != nil {
		c.setLastError(err)

		if resp != nil {
			c.config.Logger.Error("WebSocket连接失败",
				zap.String("url", c.config.URL),
				zap.Int("status_code", resp.StatusCode),
				zap.Error(err))
		} else {
			c.config.Logger.Error("WebSocket连接失败",
				zap.String("url", c.config.URL),
				zap.Error(err))
		}

		go c.scheduleReconnect()
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.reconnects = 0
	c.mu.Unlock()

	c.config.Logger.Info("成功连接到WebSocket服务器", zap.String("url", c.config.URL))

	// 启动处理协程
	go c.readPump()
	go c.writePump()
	go c.pingPump()

	if c.config.OnOpen != nil {
		c.config.OnOpen()
	}

	return nil
}

// Send 发送消息
func (c *WSClient) Send(msgType WSMessageType, data interface{}) error {
	c.mu.RLock()
	if !c.connected || c.closed {
		c.mu.RUnlock()
		return errors.New("not connected or client closed")
	}
	c.mu.RUnlock()

	var rawData json.RawMessage
	if data != nil {
		bytes, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshal data failed: %w", err)
		}
		rawData = bytes
	}

	msg := &WSMessage{
		Type:      msgType,
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		Data:      rawData,
		ID:        String.RandomString(8), // 生成随机消息ID
	}

	select {
	case c.sendCh <- msg:
		return nil
	case <-c.done:
		return errors.New("client closed")
	default:
		return errors.New("send channel full")
	}
}

// Close 关闭连接
func (c *WSClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	c.connected = false

	// 取消上下文
	c.cancel()

	// 关闭通道
	close(c.done)

	// 关闭WebSocket连接
	if c.conn != nil {
		// 发送关闭消息
		msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client closing")
		_ = c.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
		c.conn.Close()
		c.conn = nil
	}

	c.config.Logger.Info("WebSocket客户端已关闭")

	if c.config.OnClose != nil {
		c.config.OnClose()
	}
}

// IsConnected 检查是否已连接
func (c *WSClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && !c.closed
}

// readPump 读取泵
func (c *WSClient) readPump() {
	defer func() {
		c.reconnect()
	}()

	if c.conn == nil {
		return
	}

	c.conn.SetReadLimit(65536) // 设置最大读取大小 64KB

	// 设置读超时处理
	c.conn.SetReadDeadline(time.Now().Add(c.config.PongTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(c.config.PongTimeout))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			c.setLastError(err)

			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseNormalClosure,
				websocket.CloseGoingAway,
				websocket.CloseNoStatusReceived) {
				c.config.Logger.Error("WebSocket读取错误", zap.Error(err))
			}

			// 如果客户端已关闭，不再重连
			c.mu.RLock()
			closed := c.closed
			c.mu.RUnlock()

			if closed {
				return
			}

			break
		}

		// 处理消息
		var wsMsg WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			c.config.Logger.Error("解析WebSocket消息失败", zap.Error(err))
			continue
		}

		// 处理特殊消息类型
		if wsMsg.Type == WSMessageTypePing {
			// 响应Ping
			c.Send(WSMessageTypePong, nil)
			continue
		}

		// 回调处理消息
		if c.config.OnMessage != nil {
			c.config.OnMessage(&wsMsg)
		}
	}
}

// writePump 写入泵
func (c *WSClient) writePump() {
	ticker := time.NewTicker(c.config.PingInterval)
	defer func() {
		ticker.Stop()
		c.reconnect()
	}()

	for {
		select {
		case message, ok := <-c.sendCh:
			if !ok {
				// 通道已关闭
				return
			}

			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				continue
			}

			// 设置写超时
			conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))

			// 序列化消息
			data, err := json.Marshal(message)
			if err != nil {
				c.config.Logger.Error("序列化消息失败", zap.Error(err))
				continue
			}

			// 发送消息
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				c.setLastError(err)
				c.config.Logger.Error("发送WebSocket消息失败", zap.Error(err))
				return
			}

		case <-ticker.C:
			// 发送Ping消息
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				continue
			}

			// 设置写超时
			conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.setLastError(err)
				c.config.Logger.Error("发送Ping消息失败", zap.Error(err))
				return
			}

		case <-c.ctx.Done():
			return
		}
	}
}

// pingPump 定期发送心跳
func (c *WSClient) pingPump() {
	ticker := time.NewTicker(c.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 发送应用层Ping
			err := c.Send(WSMessageTypePing, map[string]interface{}{"time": time.Now().UnixNano() / int64(time.Millisecond)})
			if err != nil {
				c.config.Logger.Debug("发送应用层Ping失败", zap.Error(err))
			}
		case <-c.ctx.Done():
			return
		}
	}
}

// reconnect 重新连接
func (c *WSClient) reconnect() {
	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()
		return
	}

	// 防止多次重连
	if !c.connected {
		c.mu.Unlock()
		return
	}

	// 标记为断开连接
	c.connected = false

	// 增加重连次数
	c.reconnects++
	reconnects := c.reconnects

	// 关闭旧连接
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.mu.Unlock()

	// 通知连接已关闭
	if c.config.OnClose != nil {
		c.config.OnClose()
	}

	// 检查最大重连次数
	if c.config.MaxReconnect >= 0 && reconnects > c.config.MaxReconnect {
		c.config.Logger.Warn("达到最大重连次数，停止重连",
			zap.Int("reconnects", reconnects),
			zap.Int("maxReconnect", c.config.MaxReconnect))
		return
	}

	// 调度重连
	select {
	case c.reconnectCh <- struct{}{}:
		// 触发重连
	default:
		// 已经有重连请求在队列中
	}
}

// scheduleReconnect 调度重连
func (c *WSClient) scheduleReconnect() {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return
	}
	reconnects := c.reconnects
	c.mu.RUnlock()

	// 通知重连事件
	if c.config.OnReconnect != nil {
		c.config.OnReconnect(reconnects)
	}

	// 计算重连延迟（可以实现指数退避）
	delay := c.config.ReconnectInterval

	c.config.Logger.Info("正在调度WebSocket重连",
		zap.Duration("delay", delay),
		zap.Int("attempt", reconnects+1))

	select {
	case <-time.After(delay):
		// 重新连接
		if err := c.Connect(); err != nil {
			c.config.Logger.Warn("重连失败", zap.Error(err))
			// 连接方法内部会处理重连
		}
	case <-c.ctx.Done():
		// 客户端已关闭，停止重连
		return
	}
}

// setLastError 设置最后一个错误
func (c *WSClient) setLastError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = err

	if c.config.OnError != nil {
		c.config.OnError(err)
	}
}

// StartReconnectLoop 启动重连循环
func (c *WSClient) StartReconnectLoop() {
	go func() {
		for {
			select {
			case <-c.reconnectCh:
				c.scheduleReconnect()
			case <-c.ctx.Done():
				return
			}
		}
	}()
}

// GetLastError 获取最后一个错误
func (c *WSClient) GetLastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError
}
