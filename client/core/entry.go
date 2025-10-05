package core

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"gkipass/client/logger"
	"gkipass/client/pool"
	"gkipass/client/ws"

	"go.uber.org/zap"
)

// EntryNode Entry节点（用户接入侧）
type EntryNode struct {
	tunnelMgr    *TunnelManager
	poolMgr      *pool.PoolManager
	rateLimiters map[string]*pool.RateLimiter // tunnelID → limiter
	connections  atomic.Int64
}

// NewEntryNode 创建Entry节点
func NewEntryNode(tunnelMgr *TunnelManager, poolMgr *pool.PoolManager) *EntryNode {
	return &EntryNode{
		tunnelMgr:    tunnelMgr,
		poolMgr:      poolMgr,
		rateLimiters: make(map[string]*pool.RateLimiter),
	}
}

// HandleConnection 处理用户连接
func (en *EntryNode) HandleConnection(userConn net.Conn, rule *ws.TunnelRule) {
	defer userConn.Close()
	defer en.connections.Add(-1)

	en.connections.Add(1)

	logger.Debug("Entry新连接",
		zap.String("tunnel_id", rule.TunnelID),
		zap.String("remote", userConn.RemoteAddr().String()))

	// 选择Exit节点
	exitNode := en.selectExitNode(rule.ExitGroupID)
	if exitNode == "" {
		logger.Error("没有可用的Exit节点", zap.String("exit_group", rule.ExitGroupID))
		return
	}

	// 从连接池获取到Exit的多路复用连接
	muxConn, err := en.poolMgr.GetMultiplexConn(exitNode)
	if err != nil {
		logger.Error("获取Exit连接失败", zap.Error(err))
		return
	}

	// 打开新stream
	sessionID := generateSessionID()
	stream, err := muxConn.OpenStream(sessionID)
	if err != nil {
		logger.Error("打开stream失败", zap.Error(err))
		return
	}
	defer muxConn.CloseStream(sessionID)

	// 发送初始化帧（告诉Exit转发到哪里）
	if err := en.sendInitFrame(stream, rule, sessionID); err != nil {
		logger.Error("发送初始化帧失败", zap.Error(err))
		return
	}

	// 获取限速器（单隧道限速）
	limiter := en.getRateLimiter(rule)

	// 双向转发（带限速）
	en.forwardWithRateLimit(userConn, stream, rule.TunnelID, limiter)
}

// sendInitFrame 发送初始化帧
func (en *EntryNode) sendInitFrame(stream *pool.Stream, rule *ws.TunnelRule, sessionID string) error {
	// 帧格式: TunnelID(16B) | TargetCount(2B) | Targets...
	// Target格式: HostLen(1B) | Host | Port(2B) | Weight(2B)

	buf := make([]byte, 0, 1024)

	// TunnelID (固定16字节，不足补0)
	tunnelIDBytes := make([]byte, 16)
	copy(tunnelIDBytes, rule.TunnelID)
	buf = append(buf, tunnelIDBytes...)

	// Target数量
	targetCount := uint16(len(rule.Targets))
	buf = binary.BigEndian.AppendUint16(buf, targetCount)

	// 各个Target
	for _, target := range rule.Targets {
		// Host长度
		buf = append(buf, byte(len(target.Host)))
		// Host
		buf = append(buf, []byte(target.Host)...)
		// Port
		buf = binary.BigEndian.AppendUint16(buf, uint16(target.Port))
		// Weight
		buf = binary.BigEndian.AppendUint16(buf, uint16(target.Weight))
	}

	_, err := stream.Write(buf)
	return err
}

// forwardWithRateLimit 带限速的双向转发
func (en *EntryNode) forwardWithRateLimit(userConn net.Conn, stream *pool.Stream, tunnelID string, limiter *pool.RateLimiter) {
	errChan := make(chan error, 2)

	// 上行: 用户 → Exit (通过stream)
	go func() {
		written := int64(0)
		buf := make([]byte, 32*1024)

		for {
			n, err := userConn.Read(buf)
			if n > 0 {
				// 限速
				if limiter != nil {
					limiter.Wait(int64(n))
				}

				if _, werr := stream.Write(buf[:n]); werr != nil {
					errChan <- werr
					return
				}
				written += int64(n)
				en.tunnelMgr.updateStats(tunnelID, "upstream", int64(n))
			}
			if err != nil {
				errChan <- err
				return
			}
		}
	}()

	// 下行: Exit → 用户 (从stream读取)
	go func() {
		written := int64(0)
		buf := make([]byte, 32*1024)

		for {
			n, err := stream.Read(buf)
			if n > 0 {
				// 限速
				if limiter != nil {
					limiter.Wait(int64(n))
				}

				if _, werr := userConn.Write(buf[:n]); werr != nil {
					errChan <- werr
					return
				}
				written += int64(n)
				en.tunnelMgr.updateStats(tunnelID, "downstream", int64(n))
			}
			if err != nil {
				errChan <- err
				return
			}
		}
	}()

	<-errChan
	<-errChan
}

// getRateLimiter 获取限速器（单隧道）
func (en *EntryNode) getRateLimiter(rule *ws.TunnelRule) *pool.RateLimiter {
	if rule.MaxBandwidth <= 0 {
		return nil // 不限速
	}

	// 检查是否已存在
	if limiter, exists := en.rateLimiters[rule.TunnelID]; exists {
		// 更新速率
		limiter.SetRate(rule.MaxBandwidth)
		return limiter
	}

	// 创建新限速器
	limiter := pool.NewRateLimiter(rule.MaxBandwidth, rule.MaxBandwidth*2) // burst=2倍
	en.rateLimiters[rule.TunnelID] = limiter

	logger.Info("创建限速器",
		zap.String("tunnel_id", rule.TunnelID),
		zap.Int64("bandwidth", rule.MaxBandwidth))

	return limiter
}

// selectExitNode 选择Exit节点
func (en *EntryNode) selectExitNode(exitGroupID string) string {
	// TODO: 从节点发现服务获取Exit节点列表
	// 当前简化：直接返回groupID
	return exitGroupID
}

// GetConnectionCount 获取连接数
func (en *EntryNode) GetConnectionCount() int64 {
	return en.connections.Load()
}

func generateSessionID() string {
	return fmt.Sprintf("sess-%d", time.Now().UnixNano())
}
