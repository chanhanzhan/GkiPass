package ws

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/config"
	"gkipass/client/logger"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var (
	ErrNotConnected = errors.New("未连接到服务器")
	ErrTimeout      = errors.New("操作超时")
)

// Client WebSocket客户端
type Client struct {
	config    *config.PlaneConfig
	conn      *websocket.Conn
	sendChan  chan *Message
	recvChan  chan *Message
	stopChan  chan struct{}
	connected atomic.Bool
	mu        sync.RWMutex

	reconnectCount int
	maxReconnect   int

	// 事件回调
	onConnected    func()
	onDisconnected func()
	onMessage      func(*Message)
}

// NewClient 创建WebSocket客户端
func NewClient(cfg *config.PlaneConfig) *Client {
	return &Client{
		config:       cfg,
		sendChan:     make(chan *Message, 256),
		recvChan:     make(chan *Message, 256),
		stopChan:     make(chan struct{}),
		maxReconnect: cfg.MaxReconnectAttempts,
	}
}

// Connect 连接到服务器
func (c *Client) Connect() error {
	u, err := url.Parse(c.config.URL)
	if err != nil {
		return fmt.Errorf("无效的URL: %w", err)
	}

	// 添加CK参数
	if c.config.CK != "" {
		q := u.Query()
		q.Set("ck", c.config.CK)
		u.RawQuery = q.Encode()
	}

	logger.Info("连接到Plane", zap.String("url", u.String()))

	dialer := websocket.Dialer{
		HandshakeTimeout: time.Duration(c.config.Timeout) * time.Second,
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected.Store(true)
	c.reconnectCount = 0
	c.mu.Unlock()

	logger.Info("连接成功")

	// 启动读写循环
	go c.readLoop()
	go c.writeLoop()
	go c.heartbeatLoop()

	if c.onConnected != nil {
		c.onConnected()
	}

	return nil
}

// Send 发送消息
func (c *Client) Send(msg *Message) error {
	if !c.connected.Load() {
		return ErrNotConnected
	}

	select {
	case c.sendChan <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return ErrTimeout
	}
}

// Receive 接收消息
func (c *Client) Receive() (*Message, error) {
	select {
	case msg := <-c.recvChan:
		return msg, nil
	case <-c.stopChan:
		return nil, errors.New("客户端已关闭")
	}
}

// Close 关闭连接
func (c *Client) Close() error {
	c.connected.Store(false)
	close(c.stopChan)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		logger.Info("关闭WebSocket连接")
		return c.conn.Close()
	}

	return nil
}

// IsConnected 是否已连接
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// SetOnConnected 设置连接成功回调
func (c *Client) SetOnConnected(fn func()) {
	c.onConnected = fn
}

// SetOnDisconnected 设置断开连接回调
func (c *Client) SetOnDisconnected(fn func()) {
	c.onDisconnected = fn
}

// SetOnMessage 设置消息回调
func (c *Client) SetOnMessage(fn func(*Message)) {
	c.onMessage = fn
}

// readLoop 读取消息循环
func (c *Client) readLoop() {
	defer func() {
		c.connected.Store(false)
		if c.onDisconnected != nil {
			c.onDisconnected()
		}
		go c.reconnect()
	}()

	for {
		select {
		case <-c.stopChan:
			return
		default:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				return
			}

			var msg Message
			if err := conn.ReadJSON(&msg); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logger.Error("读取消息失败", zap.Error(err))
				}
				return
			}

			// 分发消息
			if c.onMessage != nil {
				c.onMessage(&msg)
			}

			select {
			case c.recvChan <- &msg:
			case <-c.stopChan:
				return
			}
		}
	}
}

// writeLoop 写入消息循环
func (c *Client) writeLoop() {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case msg := <-c.sendChan:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				continue
			}

			if err := conn.WriteJSON(msg); err != nil {
				logger.Error("发送消息失败", zap.Error(err))
				return
			}

		case <-ticker.C:
			// 发送ping保持连接
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn != nil {
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					logger.Error("发送ping失败", zap.Error(err))
					return
				}
			}
		}
	}
}

// heartbeatLoop 心跳循环
func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			if !c.connected.Load() {
				continue
			}

			pingMsg, _ := NewMessage(MsgTypePing, nil)
			if err := c.Send(pingMsg); err != nil {
				logger.Error("发送心跳失败", zap.Error(err))
			}
		}
	}
}

// reconnect 重连
func (c *Client) reconnect() {
	if c.maxReconnect > 0 && c.reconnectCount >= c.maxReconnect {
		logger.Error("达到最大重连次数，停止重连", zap.Int("count", c.reconnectCount))
		return
	}

	// 指数退避策略
	backoff := time.Duration(c.config.ReconnectInterval) * time.Second
	if c.reconnectCount > 0 {
		backoff = time.Duration(1<<uint(c.reconnectCount)) * time.Second
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}

	c.reconnectCount++
	logger.Info("准备重连",
		zap.Int("attempt", c.reconnectCount),
		zap.Duration("after", backoff))

	time.Sleep(backoff)

	if err := c.Connect(); err != nil {
		logger.Error("重连失败", zap.Error(err))
		go c.reconnect()
	}
}

// WaitForMessage 等待特定类型的消息
func (c *Client) WaitForMessage(ctx context.Context, msgType MessageType) (*Message, error) {
	for {
		select {
		case msg := <-c.recvChan:
			if msg.Type == msgType {
				return msg, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}






