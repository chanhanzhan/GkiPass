package pool

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

var (
	ErrPoolClosed = errors.New("连接池已关闭")
	ErrPoolFull   = errors.New("连接池已满")
	ErrTimeout    = errors.New("获取连接超时")
)

// Conn 连接包装
type Conn struct {
	net.Conn
	ID        string
	CreatedAt time.Time
	LastUsed  time.Time
	UseCount  int
	IsHealthy bool
	pool      *Pool
}

// Close 归还连接到池
func (c *Conn) Close() error {
	if c.pool != nil {
		c.pool.Put(c)
		return nil
	}
	return c.Conn.Close()
}

// Pool 连接池
type Pool struct {
	target      string
	minConns    int
	maxConns    int
	idleTimeout time.Duration

	idleConns   chan *Conn
	activeConns map[string]*Conn
	mu          sync.RWMutex

	stopChan chan struct{}
	closed   bool
}

// NewPool 创建连接池
func NewPool(target string, minConns, maxConns int, idleTimeout time.Duration) *Pool {
	if minConns < 0 {
		minConns = 0
	}
	if maxConns < minConns {
		maxConns = minConns
	}

	pool := &Pool{
		target:      target,
		minConns:    minConns,
		maxConns:    maxConns,
		idleTimeout: idleTimeout,
		idleConns:   make(chan *Conn, maxConns),
		activeConns: make(map[string]*Conn),
		stopChan:    make(chan struct{}),
	}

	return pool
}

// Init 初始化连接池
func (p *Pool) Init() error {
	// 预创建最小连接数
	for i := 0; i < p.minConns; i++ {
		conn, err := p.createConnection()
		if err != nil {
			logger.Warn("创建初始连接失败",
				zap.Int("index", i),
				zap.Error(err))
			continue
		}
		p.idleConns <- conn
	}

	// 启动维护协程
	go p.maintainLoop()

	logger.Info("连接池已初始化",
		zap.String("target", p.target),
		zap.Int("min", p.minConns),
		zap.Int("max", p.maxConns))

	return nil
}

// Get 获取连接
func (p *Pool) Get() (*Conn, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrPoolClosed
	}
	p.mu.RUnlock()

	// 尝试从空闲队列获取
	select {
	case conn := <-p.idleConns:
		// 检查连接是否仍然健康
		if p.isConnHealthy(conn) {
			conn.LastUsed = time.Now()
			conn.UseCount++
			p.moveToActive(conn)
			return conn, nil
		}
		// 连接不健康，关闭并创建新的
		conn.Conn.Close()
	default:
	}

	// 空闲队列为空，尝试创建新连接
	if p.canCreate() {
		conn, err := p.createConnection()
		if err != nil {
			return nil, err
		}
		p.moveToActive(conn)
		return conn, nil
	}

	// 达到最大连接数，等待或返回错误
	return nil, ErrPoolFull
}

// Put 归还连接
func (p *Pool) Put(conn *Conn) {
	if conn == nil {
		return
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		conn.Conn.Close()
		return
	}

	// 从活跃列表移除
	delete(p.activeConns, conn.ID)
	p.mu.Unlock()

	// 检查连接健康
	if !p.isConnHealthy(conn) {
		conn.Conn.Close()
		return
	}

	// 放回空闲队列
	select {
	case p.idleConns <- conn:
	default:
		// 队列已满，关闭连接
		conn.Conn.Close()
	}
}

// createConnection 创建新连接
func (p *Pool) createConnection() (*Conn, error) {
	rawConn, err := net.DialTimeout("tcp", p.target, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}

	conn := &Conn{
		Conn:      rawConn,
		ID:        generateConnID(),
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		IsHealthy: true,
		pool:      p,
	}

	return conn, nil
}

// isConnHealthy 检查连接是否健康
func (p *Pool) isConnHealthy(conn *Conn) bool {
	if conn == nil || conn.Conn == nil {
		return false
	}

	// 设置读超时测试连接
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	one := make([]byte, 1)
	_, err := conn.Read(one)
	conn.SetReadDeadline(time.Time{})

	// 如果是超时错误，说明连接可能正常（只是没有数据）
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return true
		}
		return false
	}

	return true
}

// canCreate 是否可以创建新连接
func (p *Pool) canCreate() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.activeConns) < p.maxConns
}

// moveToActive 移动到活跃列表
func (p *Pool) moveToActive(conn *Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activeConns[conn.ID] = conn
}

// maintainLoop 维护循环
func (p *Pool) maintainLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanup()
		case <-p.stopChan:
			return
		}
	}
}

// cleanup 清理过期连接
func (p *Pool) cleanup() {
	// 清理空闲超时的连接
	idleConns := make([]*Conn, 0)

loop:
	for {
		select {
		case conn := <-p.idleConns:
			if time.Since(conn.LastUsed) > p.idleTimeout {
				conn.Conn.Close()
				logger.Debug("关闭空闲超时连接", zap.String("conn_id", conn.ID))
			} else {
				idleConns = append(idleConns, conn)
			}
		default:
			break loop
		}
	}

	// 放回未超时的连接
	for _, conn := range idleConns {
		p.idleConns <- conn
	}

	// 确保最小连接数
	currentIdle := len(p.idleConns)
	if currentIdle < p.minConns {
		for i := 0; i < p.minConns-currentIdle; i++ {
			conn, err := p.createConnection()
			if err != nil {
				logger.Warn("补充连接失败", zap.Error(err))
				break
			}
			p.idleConns <- conn
		}
	}
}

// Close 关闭连接池
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.stopChan)
	p.mu.Unlock()

	// 关闭所有空闲连接
	close(p.idleConns)
	for conn := range p.idleConns {
		conn.Conn.Close()
	}

	// 关闭所有活跃连接
	p.mu.Lock()
	for _, conn := range p.activeConns {
		conn.Conn.Close()
	}
	p.activeConns = nil
	p.mu.Unlock()

	logger.Info("连接池已关闭", zap.String("target", p.target))
	return nil
}

// Stats 连接池统计
func (p *Pool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return PoolStats{
		Target:      p.target,
		IdleCount:   len(p.idleConns),
		ActiveCount: len(p.activeConns),
		MinConns:    p.minConns,
		MaxConns:    p.maxConns,
	}
}

// PoolStats 连接池统计信息
type PoolStats struct {
	Target      string
	IdleCount   int
	ActiveCount int
	MinConns    int
	MaxConns    int
}

// generateConnID 生成连接ID
func generateConnID() string {
	return fmt.Sprintf("conn-%d", time.Now().UnixNano())
}






