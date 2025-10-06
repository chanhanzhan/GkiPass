package core

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/logger"
	"gkipass/client/ws"

	"go.uber.org/zap"
)

// TunnelManager 隧道管理器
type TunnelManager struct {
	rules       map[string]*ws.TunnelRule // tunnelID -> rule
	listeners   map[int]net.Listener      // port -> listener
	nodeType    string                    // entry/exit
	stats       map[string]*TunnelStats   // tunnelID -> stats
	mu          sync.RWMutex
	stopChan    chan struct{}
	connections atomic.Int64 // 总连接数
}

// TunnelStats 隧道统计
type TunnelStats struct {
	TrafficIn   atomic.Int64
	TrafficOut  atomic.Int64
	Connections atomic.Int32
	Errors      atomic.Int32
}

// NewTunnelManager 创建隧道管理器
func NewTunnelManager(nodeType string) *TunnelManager {
	return &TunnelManager{
		rules:     make(map[string]*ws.TunnelRule),
		listeners: make(map[int]net.Listener),
		nodeType:  nodeType,
		stats:     make(map[string]*TunnelStats),
		stopChan:  make(chan struct{}),
	}
}

// ApplyRules 应用隧道规则
func (tm *TunnelManager) ApplyRules(rules []ws.TunnelRule) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	logger.Info("应用隧道规则", zap.Int("count", len(rules)))

	// 构建新规则映射
	newRules := make(map[string]*ws.TunnelRule)
	for i := range rules {
		newRules[rules[i].TunnelID] = &rules[i]
	}

	// 删除不存在的规则
	for tunnelID := range tm.rules {
		if _, exists := newRules[tunnelID]; !exists {
			if err := tm.stopTunnelLocked(tunnelID); err != nil {
				logger.Error("停止隧道失败",
					zap.String("tunnel_id", tunnelID),
					zap.Error(err))
			}
		}
	}

	// 添加或更新规则
	for _, rule := range rules {
		if rule.Enabled {
			// 检查是否已存在且端口未变
			if existing, exists := tm.rules[rule.TunnelID]; exists {
				if existing.LocalPort == rule.LocalPort {
					// 仅更新规则，不重启监听器
					tm.rules[rule.TunnelID] = &rule
					logger.Info("更新隧道规则",
						zap.String("tunnel_id", rule.TunnelID),
						zap.String("name", rule.Name))
					continue
				}
				// 端口变化，需要重启
				tm.stopTunnelLocked(rule.TunnelID)
			}

			// 启动新隧道
			if err := tm.startTunnelLocked(&rule); err != nil {
				logger.Error("启动隧道失败",
					zap.String("tunnel_id", rule.TunnelID),
					zap.Error(err))
				continue
			}
		} else {
			// 禁用的规则，如果存在则停止
			if _, exists := tm.rules[rule.TunnelID]; exists {
				tm.stopTunnelLocked(rule.TunnelID)
			}
		}
	}

	return nil
}

// startTunnelLocked 启动隧道（需要持有锁）
func (tm *TunnelManager) startTunnelLocked(rule *ws.TunnelRule) error {
	// 检查端口是否被占用
	if _, exists := tm.listeners[rule.LocalPort]; exists {
		return fmt.Errorf("端口 %d 已被占用", rule.LocalPort)
	}

	// 仅Entry节点需要监听端口
	if tm.nodeType == "entry" {
		addr := fmt.Sprintf(":%d", rule.LocalPort)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("绑定端口失败: %w", err)
		}

		tm.listeners[rule.LocalPort] = listener
		go tm.acceptConnections(listener, rule)
	}

	// 保存规则和初始化统计
	tm.rules[rule.TunnelID] = rule
	tm.stats[rule.TunnelID] = &TunnelStats{}

	logger.Info("隧道已启动",
		zap.String("tunnel_id", rule.TunnelID),
		zap.String("name", rule.Name),
		zap.String("protocol", rule.Protocol),
		zap.Int("port", rule.LocalPort),
		zap.Int("targets", len(rule.Targets)))

	return nil
}

// stopTunnelLocked 停止隧道（需要持有锁）
func (tm *TunnelManager) stopTunnelLocked(tunnelID string) error {
	rule, exists := tm.rules[tunnelID]
	if !exists {
		return nil
	}

	// 关闭监听器
	if listener, ok := tm.listeners[rule.LocalPort]; ok {
		listener.Close()
		delete(tm.listeners, rule.LocalPort)
	}

	// 删除规则和统计
	delete(tm.rules, tunnelID)
	delete(tm.stats, tunnelID)

	logger.Info("隧道已停止", zap.String("tunnel_id", tunnelID))
	return nil
}

// RemoveRule 移除规则
func (tm *TunnelManager) RemoveRule(tunnelID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.stopTunnelLocked(tunnelID)
}

// GetStats 获取隧道统计
func (tm *TunnelManager) GetStats(tunnelID string) *TunnelStats {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.stats[tunnelID]
}

// GetAllStats 获取所有隧道统计
func (tm *TunnelManager) GetAllStats() map[string]*TunnelStats {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make(map[string]*TunnelStats)
	for tunnelID, stats := range tm.stats {
		result[tunnelID] = stats
	}
	return result
}

// GetConnectionCount 获取总连接数
func (tm *TunnelManager) GetConnectionCount() int64 {
	return tm.connections.Load()
}

// acceptConnections 接收连接（Entry节点）
func (tm *TunnelManager) acceptConnections(listener net.Listener, rule *ws.TunnelRule) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("accept协程panic", zap.Any("panic", r))
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			// 监听器关闭时退出
			select {
			case <-tm.stopChan:
				return
			default:
				if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
					return
				}
				logger.Error("接受连接失败",
					zap.String("tunnel_id", rule.TunnelID),
					zap.Error(err))
				continue
			}
		}

		// 增加连接计数
		tm.connections.Add(1)
		tm.mu.RLock()
		stats := tm.stats[rule.TunnelID]
		tm.mu.RUnlock()
		if stats != nil {
			stats.Connections.Add(1)
		}

		// 异步处理连接
		go tm.handleEntryConnection(conn, rule)
	}
}

// handleEntryConnection 处理Entry节点连接
func (tm *TunnelManager) handleEntryConnection(userConn net.Conn, rule *ws.TunnelRule) {
	defer func() {
		userConn.Close()
		tm.connections.Add(-1)

		tm.mu.RLock()
		stats := tm.stats[rule.TunnelID]
		tm.mu.RUnlock()
		if stats != nil {
			stats.Connections.Add(-1)
		}

		if r := recover(); r != nil {
			logger.Error("连接处理panic", zap.Any("panic", r))
		}
	}()

	remoteAddr := userConn.RemoteAddr().String()
	logger.Debug("新连接",
		zap.String("tunnel_id", rule.TunnelID),
		zap.String("remote", remoteAddr),
		zap.String("protocol", rule.Protocol))

	// 根据协议类型处理
	switch rule.Protocol {
	case "tcp":
		tm.handleTCPConnection(userConn, rule)
	case "udp":
		// UDP在监听器层面已处理
		logger.Warn("UDP不应该通过TCP连接处理")
	default:
		// 默认按TCP处理
		tm.handleTCPConnection(userConn, rule)
	}
}

// handleTCPConnection 处理TCP连接
func (tm *TunnelManager) handleTCPConnection(userConn net.Conn, rule *ws.TunnelRule) {
	// 选择目标
	if len(rule.Targets) == 0 {
		logger.Error("没有可用的目标", zap.String("tunnel_id", rule.TunnelID))
		return
	}

	target := tm.selectTarget(rule.Targets)
	targetAddr := net.JoinHostPort(target.Host, fmt.Sprintf("%d", target.Port))

	// 连接目标服务器
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		logger.Error("连接目标失败",
			zap.String("tunnel_id", rule.TunnelID),
			zap.String("target", targetAddr),
			zap.Error(err))

		tm.mu.RLock()
		stats := tm.stats[rule.TunnelID]
		tm.mu.RUnlock()
		if stats != nil {
			stats.Errors.Add(1)
		}
		return
	}
	defer targetConn.Close()

	logger.Info("已连接目标",
		zap.String("tunnel_id", rule.TunnelID),
		zap.String("target", targetAddr))

	// 双向转发
	tm.forwardBidirectional(userConn, targetConn, rule.TunnelID)
}

// selectTarget 选择目标（加权随机）
func (tm *TunnelManager) selectTarget(targets []ws.TunnelTarget) ws.TunnelTarget {
	if len(targets) == 1 {
		return targets[0]
	}

	// 计算总权重
	totalWeight := 0
	for i := range targets {
		if targets[i].Weight <= 0 {
			targets[i].Weight = 1
		}
		totalWeight += targets[i].Weight
	}

	// 加权随机选择
	rand := tm.randomInt(totalWeight)
	for _, t := range targets {
		rand -= t.Weight
		if rand < 0 {
			return t
		}
	}

	// 兜底：返回第一个
	return targets[0]
}

// randomInt 生成随机数 [0, max)
func (tm *TunnelManager) randomInt(max int) int {
	if max <= 0 {
		return 0
	}
	// 使用时间作为种子的简单随机
	return int(time.Now().UnixNano() % int64(max))
}

// forwardBidirectional 双向转发
func (tm *TunnelManager) forwardBidirectional(src, dst net.Conn, tunnelID string) {
	errChan := make(chan error, 2)

	// 上行：用户 → 目标
	go func() {
		written, err := tm.copyWithStats(dst, src, tunnelID, "upstream")
		logger.Debug("上行结束",
			zap.String("tunnel_id", tunnelID),
			zap.Int64("bytes", written))
		errChan <- err
	}()

	// 下行：目标 → 用户
	go func() {
		written, err := tm.copyWithStats(src, dst, tunnelID, "downstream")
		logger.Debug("下行结束",
			zap.String("tunnel_id", tunnelID),
			zap.Int64("bytes", written))
		errChan <- err
	}()

	// 等待任意方向结束
	<-errChan
	<-errChan
}

// copyWithStats 带统计的数据拷贝
func (tm *TunnelManager) copyWithStats(dst, src net.Conn, tunnelID, direction string) (int64, error) {
	buf := make([]byte, 32*1024) // 32KB buffer
	var total int64

	for {
		nr, err := src.Read(buf)
		if nr > 0 {
			nw, werr := dst.Write(buf[0:nr])
			if nw > 0 {
				total += int64(nw)
				// 更新统计
				tm.updateStats(tunnelID, direction, int64(nw))
			}
			if werr != nil {
				return total, werr
			}
			if nr != nw {
				return total, fmt.Errorf("short write")
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
}

// updateStats 更新统计
func (tm *TunnelManager) updateStats(tunnelID, direction string, bytes int64) {
	tm.mu.RLock()
	stats := tm.stats[tunnelID]
	tm.mu.RUnlock()

	if stats != nil {
		if direction == "upstream" {
			stats.TrafficIn.Add(bytes)
		} else {
			stats.TrafficOut.Add(bytes)
		}
	}
}

// Stop 停止管理器
func (tm *TunnelManager) Stop() {
	close(tm.stopChan)

	tm.mu.Lock()
	defer tm.mu.Unlock()

	logger.Info("停止所有隧道")

	// 关闭所有监听器
	for port, listener := range tm.listeners {
		listener.Close()
		delete(tm.listeners, port)
	}

	// 清空规则
	tm.rules = make(map[string]*ws.TunnelRule)
	tm.stats = make(map[string]*TunnelStats)
}

// ListRules 列出所有规则
func (tm *TunnelManager) ListRules() []*ws.TunnelRule {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	rules := make([]*ws.TunnelRule, 0, len(tm.rules))
	for _, rule := range tm.rules {
		rules = append(rules, rule)
	}
	return rules
}
