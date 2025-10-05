package pool

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// PoolManager 连接池管理器
type PoolManager struct {
	pools   map[string]*MultiplexConn // exitNodeID → mux conn
	tlsCfg  *tls.Config
	mu      sync.RWMutex
	enabled bool
}

// NewPoolManager 创建连接池管理器
func NewPoolManager(tlsEnabled bool, tlsCfg *tls.Config) *PoolManager {
	return &PoolManager{
		pools:   make(map[string]*MultiplexConn),
		tlsCfg:  tlsCfg,
		enabled: tlsEnabled,
	}
}

// GetMultiplexConn 获取多路复用连接
func (pm *PoolManager) GetMultiplexConn(exitNodeID string) (*MultiplexConn, error) {
	pm.mu.RLock()
	if conn, exists := pm.pools[exitNodeID]; exists {
		pm.mu.RUnlock()
		return conn, nil
	}
	pm.mu.RUnlock()

	// 创建新的多路复用连接
	return pm.createMultiplexConn(exitNodeID)
}

// createMultiplexConn 创建多路复用连接
func (pm *PoolManager) createMultiplexConn(exitNodeID string) (*MultiplexConn, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 双重检查
	if conn, exists := pm.pools[exitNodeID]; exists {
		return conn, nil
	}

	// TODO: 从节点发现服务获取Exit节点地址
	// 当前简化：假设exitNodeID就是地址
	addr := exitNodeID

	logger.Info("创建Exit连接", zap.String("exit", addr))

	// 建立TCP连接
	rawConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("连接Exit失败: %w", err)
	}

	// 如果启用TLS，升级连接
	var conn net.Conn = rawConn
	if pm.enabled && pm.tlsCfg != nil {
		tlsConn := tls.Client(rawConn, pm.tlsCfg)
		if err := tlsConn.Handshake(); err != nil {
			rawConn.Close()
			return nil, fmt.Errorf("TLS握手失败: %w", err)
		}
		conn = tlsConn
		logger.Info("TLS加密已建立", zap.String("exit", addr))
	}

	// 创建多路复用连接
	muxConn := NewMultiplexConn(conn)
	pm.pools[exitNodeID] = muxConn

	logger.Info("多路复用连接已建立",
		zap.String("exit", addr),
		zap.Bool("tls", pm.enabled))

	return muxConn, nil
}

// CloseAll 关闭所有连接
func (pm *PoolManager) CloseAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for exitNodeID, muxConn := range pm.pools {
		muxConn.Close()
		logger.Info("关闭Exit连接", zap.String("exit", exitNodeID))
	}

	pm.pools = make(map[string]*MultiplexConn)
}

// GetStats 获取连接池统计
func (pm *PoolManager) GetStats() map[string]int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make(map[string]int)
	for exitNodeID, muxConn := range pm.pools {
		stats[exitNodeID] = muxConn.StreamCount()
	}
	return stats
}






