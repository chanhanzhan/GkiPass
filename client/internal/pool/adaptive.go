package pool

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/internal/transport"

	"go.uber.org/zap"
)

// ConnectionQuality 连接质量指标
type ConnectionQuality struct {
	RTT        time.Duration `json:"rtt"`         // 往返时间
	PacketLoss float64       `json:"packet_loss"` // 丢包率 (0-1)
	Throughput int64         `json:"throughput"`  // 吞吐量 bytes/sec
	ErrorRate  float64       `json:"error_rate"`  // 错误率 (0-1)
	LastUpdate time.Time     `json:"last_update"` // 最后更新时间
	Score      float64       `json:"score"`       // 质量评分 (0-100)
}

// PooledConnection 池化连接
type PooledConnection struct {
	conn         net.Conn
	id           string
	createdAt    time.Time
	lastUsed     time.Time
	quality      atomic.Value // *ConnectionQuality
	bytesIn      atomic.Int64
	bytesOut     atomic.Int64
	requestCount atomic.Int64
	errorCount   atomic.Int64
	inUse        atomic.Bool
	ctx          context.Context
	cancel       context.CancelFunc

	// RTT测量
	rttSamples []time.Duration
	rttMutex   sync.RWMutex

	// 包统计
	packetsSent atomic.Int64
	packetsLost atomic.Int64
	packetsRecv atomic.Int64
}

// AdaptivePool 自适应连接池
type AdaptivePool struct {
	// 配置
	targetAddr    string
	transportType transport.TransportType
	transportMgr  *transport.Manager

	// 池配置
	minConnections int
	maxConnections int
	maxIdleTime    time.Duration
	maxConnAge     time.Duration

	// 连接管理
	connections map[string]*PooledConnection
	connMutex   sync.RWMutex

	// 质量阈值
	minQualityScore float64
	maxRTT          time.Duration
	maxPacketLoss   float64
	maxErrorRate    float64

	// 自适应参数
	scaleUpThreshold   float64 // 扩容阈值
	scaleDownThreshold float64 // 缩容阈值
	adaptInterval      time.Duration

	// 负载均衡策略
	loadBalanceStrategy LoadBalanceStrategy
	roundRobinIndex     atomic.Int64 // 轮询索引
	loadBalancer        LoadBalancer // 高级负载均衡器

	// 连接预热
	preWarmingEnabled bool
	preWarmingTargets int
	trafficPrediction *TrafficPredictor

	// 状态
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *zap.Logger

	// 统计
	stats struct {
		totalConnections  atomic.Int64
		activeConnections atomic.Int64
		failedConnections atomic.Int64
		poolHits          atomic.Int64
		poolMisses        atomic.Int64
		scalingEvents     atomic.Int64
		averageQuality    atomic.Int64 // * 100 for precision
	}
}

// NewAdaptivePool 创建自适应连接池
func NewAdaptivePool(targetAddr string, transportType transport.TransportType, transportMgr *transport.Manager) *AdaptivePool {
	pool := &AdaptivePool{
		targetAddr:          targetAddr,
		transportType:       transportType,
		transportMgr:        transportMgr,
		minConnections:      2,
		maxConnections:      20,
		maxIdleTime:         5 * time.Minute,
		maxConnAge:          30 * time.Minute,
		minQualityScore:     60.0,
		maxRTT:              500 * time.Millisecond,
		maxPacketLoss:       0.05, // 5%
		maxErrorRate:        0.02, // 2%
		scaleUpThreshold:    80.0, // 平均质量低于80分时扩容
		scaleDownThreshold:  95.0, // 平均质量高于95分且连接数>min时缩容
		adaptInterval:       30 * time.Second,
		loadBalanceStrategy: StrategyQualityBased, // 默认使用质量优先策略
		preWarmingEnabled:   true,
		preWarmingTargets:   3,
		connections:         make(map[string]*PooledConnection),
		logger:              zap.L().Named("adaptive-pool"),
	}

	// 初始化流量预测器
	pool.trafficPrediction = NewTrafficPredictor()

	// 初始化高级负载均衡器
	pool.loadBalancer = NewAdvancedLoadBalancer(pool.loadBalanceStrategy)

	return pool
}

// Start 启动连接池
func (p *AdaptivePool) Start(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	p.logger.Info("🏊 启动自适应连接池",
		zap.String("target", p.targetAddr),
		zap.String("transport", string(p.transportType)),
		zap.Int("min_connections", p.minConnections),
		zap.Int("max_connections", p.maxConnections))

	// 预创建最小连接数
	for i := 0; i < p.minConnections; i++ {
		if _, err := p.createConnection(); err != nil {
			p.logger.Error("创建初始连接失败", zap.Error(err))
		}
	}

	// 连接预热
	if p.preWarmingEnabled {
		p.performPreWarming()
	}

	// 启动后台任务
	p.wg.Add(5)
	go p.adaptiveScaling()
	go p.qualityMonitoring()
	go p.connectionCleaner()
	go p.trafficMonitoring()
	go p.loadBalancer.StartWeightUpdateLoop(p.ctx)

	p.logger.Info("✅ 自适应连接池启动完成")
	return nil
}

// Stop 停止连接池
func (p *AdaptivePool) Stop() error {
	p.logger.Info("🔄 正在停止连接池...")

	if p.cancel != nil {
		p.cancel()
	}

	// 关闭所有连接
	p.connMutex.Lock()
	for _, conn := range p.connections {
		conn.cancel()
		conn.conn.Close()
	}
	p.connections = make(map[string]*PooledConnection)
	p.connMutex.Unlock()

	// 等待后台任务结束
	p.wg.Wait()

	p.logger.Info("✅ 连接池已停止")
	return nil
}

// GetConnection 获取一个连接
func (p *AdaptivePool) GetConnection() (*PooledConnection, error) {
	// 尝试从池中获取最佳连接
	if conn := p.getBestConnection(); conn != nil {
		p.stats.poolHits.Add(1)
		return conn, nil
	}

	p.stats.poolMisses.Add(1)

	// 池中没有可用连接，创建新连接
	return p.createConnection()
}

// getBestConnection 根据负载均衡策略获取最佳连接
func (p *AdaptivePool) getBestConnection() *PooledConnection {
	p.connMutex.RLock()
	defer p.connMutex.RUnlock()

	// 获取所有可用连接
	availableConns := make([]*PooledConnection, 0)
	for _, conn := range p.connections {
		// 跳过正在使用的连接
		if conn.inUse.Load() {
			continue
		}

		// 检查连接是否还有效
		if !p.isConnectionValid(conn) {
			continue
		}

		availableConns = append(availableConns, conn)
	}

	if len(availableConns) == 0 {
		return nil
	}

	// 根据负载均衡策略选择连接
	selectedConn := p.selectConnectionByStrategy(availableConns)

	if selectedConn != nil {
		selectedConn.inUse.Store(true)
		selectedConn.lastUsed = time.Now()
	}

	return selectedConn
}

// createConnection 创建新连接
func (p *AdaptivePool) createConnection() (*PooledConnection, error) {
	// 检查是否超过最大连接数
	p.connMutex.RLock()
	currentCount := len(p.connections)
	p.connMutex.RUnlock()

	if currentCount >= p.maxConnections {
		return nil, fmt.Errorf("连接池已满，当前连接数: %d", currentCount)
	}

	// 创建网络连接
	conn, err := p.transportMgr.DialContext(p.ctx, p.transportType, p.targetAddr)
	if err != nil {
		p.stats.failedConnections.Add(1)
		return nil, fmt.Errorf("创建连接失败: %w", err)
	}

	// 生成连接ID
	connID := fmt.Sprintf("conn-%d", time.Now().UnixNano())

	// 创建池化连接
	ctx, cancel := context.WithCancel(p.ctx)
	pooledConn := &PooledConnection{
		conn:       conn,
		id:         connID,
		createdAt:  time.Now(),
		lastUsed:   time.Now(),
		rttSamples: make([]time.Duration, 0, 10),
		ctx:        ctx,
		cancel:     cancel,
	}

	// 初始化质量指标
	initialQuality := &ConnectionQuality{
		RTT:        100 * time.Millisecond, // 默认RTT
		PacketLoss: 0.0,
		Throughput: 0,
		ErrorRate:  0.0,
		LastUpdate: time.Now(),
		Score:      100.0, // 新连接默认满分
	}
	pooledConn.quality.Store(initialQuality)
	pooledConn.inUse.Store(true)

	// 添加到池中
	p.connMutex.Lock()
	p.connections[connID] = pooledConn
	p.connMutex.Unlock()

	p.stats.totalConnections.Add(1)
	p.stats.activeConnections.Add(1)

	p.logger.Debug("创建新连接",
		zap.String("conn_id", connID),
		zap.String("target", p.targetAddr),
		zap.Int("pool_size", len(p.connections)))

	return pooledConn, nil
}

// ReleaseConnection 释放连接回池中
func (p *AdaptivePool) ReleaseConnection(conn *PooledConnection) {
	if conn == nil {
		return
	}

	conn.inUse.Store(false)
	conn.lastUsed = time.Now()

	// 更新连接质量
	p.updateConnectionQuality(conn)

	// 清理负载均衡器中的过期记录
	if p.loadBalancer != nil {
		activeIDs := make([]string, 0, len(p.connections))
		for id := range p.connections {
			activeIDs = append(activeIDs, id)
		}
		p.loadBalancer.CleanupStaleConnections(activeIDs)
	}

	p.logger.Debug("释放连接", zap.String("conn_id", conn.id))
}

// isConnectionValid 检查连接是否有效
func (p *AdaptivePool) isConnectionValid(conn *PooledConnection) bool {
	// 检查连接年龄
	if time.Since(conn.createdAt) > p.maxConnAge {
		return false
	}

	// 检查空闲时间
	if time.Since(conn.lastUsed) > p.maxIdleTime {
		return false
	}

	// 检查质量
	quality := p.getConnectionQuality(conn)
	if quality.Score < p.minQualityScore {
		return false
	}

	// 检查RTT
	if quality.RTT > p.maxRTT {
		return false
	}

	// 检查丢包率
	if quality.PacketLoss > p.maxPacketLoss {
		return false
	}

	// 检查错误率
	if quality.ErrorRate > p.maxErrorRate {
		return false
	}

	return true
}

// getConnectionQuality 获取连接质量
func (p *AdaptivePool) getConnectionQuality(conn *PooledConnection) *ConnectionQuality {
	if quality := conn.quality.Load(); quality != nil {
		return quality.(*ConnectionQuality)
	}

	// 返回默认质量
	return &ConnectionQuality{
		RTT:        1 * time.Second,
		PacketLoss: 0.0,
		Throughput: 0,
		ErrorRate:  0.0,
		LastUpdate: time.Time{},
		Score:      0.0,
	}
}

// updateConnectionQuality 更新连接质量（增强版）
func (p *AdaptivePool) updateConnectionQuality(conn *PooledConnection) {
	// 计算RTT平均值和方差
	conn.rttMutex.RLock()
	var avgRTT time.Duration
	var rttJitter time.Duration
	if len(conn.rttSamples) > 0 {
		var total time.Duration
		for _, rtt := range conn.rttSamples {
			total += rtt
		}
		avgRTT = total / time.Duration(len(conn.rttSamples))

		// 计算RTT抖动（标准差）
		if len(conn.rttSamples) > 1 {
			var variance float64
			avgMs := float64(avgRTT.Nanoseconds()) / 1e6
			for _, rtt := range conn.rttSamples {
				rttMs := float64(rtt.Nanoseconds()) / 1e6
				variance += math.Pow(rttMs-avgMs, 2)
			}
			variance /= float64(len(conn.rttSamples) - 1)
			stdDev := math.Sqrt(variance)
			rttJitter = time.Duration(stdDev * 1e6) // 转换为纳秒
		}
	} else {
		avgRTT = 100 * time.Millisecond // 默认值
	}
	conn.rttMutex.RUnlock()

	// 计算丢包率
	packetsSent := conn.packetsSent.Load()
	packetsLost := conn.packetsLost.Load()
	var packetLoss float64
	if packetsSent > 0 {
		packetLoss = float64(packetsLost) / float64(packetsSent)
	}

	// 计算错误率
	requestCount := conn.requestCount.Load()
	errorCount := conn.errorCount.Load()
	var errorRate float64
	if requestCount > 0 {
		errorRate = float64(errorCount) / float64(requestCount)
	}

	// 计算吞吐量 (基于最近时间窗口)
	bytesTransferred := conn.bytesIn.Load() + conn.bytesOut.Load()
	timeElapsed := time.Since(conn.createdAt).Seconds()
	var throughput int64
	if timeElapsed > 0 {
		throughput = int64(float64(bytesTransferred) / timeElapsed)
	}

	// 计算质量评分 (0-100)
	score := p.calculateQualityScore(avgRTT, packetLoss, errorRate, throughput)

	// 连接年龄因子调整
	age := time.Since(conn.createdAt)
	if age < 1*time.Minute {
		score += 2 // 新连接奖励
	} else if age > 10*time.Minute {
		score -= 3 // 老连接小幅惩罚
	}

	// RTT抖动惩罚
	if rttJitter > 50*time.Millisecond {
		jitterPenalty := math.Min(10.0, float64(rttJitter.Milliseconds())/10.0)
		score -= jitterPenalty
	}

	// 确保分数在合理范围内
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	// 更新质量指标
	quality := &ConnectionQuality{
		RTT:        avgRTT,
		PacketLoss: packetLoss,
		Throughput: throughput,
		ErrorRate:  errorRate,
		LastUpdate: time.Now(),
		Score:      score,
	}

	conn.quality.Store(quality)

	// 更新流量预测器
	if p.trafficPrediction != nil {
		p.trafficPrediction.AddSample(
			len(p.connections),
			throughput,
			avgRTT,
		)
	}
}

// calculateQualityScore 计算质量评分（增强版）
func (p *AdaptivePool) calculateQualityScore(rtt time.Duration, packetLoss, errorRate float64, throughput int64) float64 {
	score := 100.0

	// RTT影响 (0-35分) - 使用指数函数提高对延迟的敏感度
	rttMs := float64(rtt.Nanoseconds()) / 1e6
	if rttMs > 1000 {
		score -= 35
	} else {
		// 使用指数衰减函数: score_penalty = 35 * (1 - e^(-rtt/200))
		penalty := 35.0 * (1.0 - math.Exp(-rttMs/200.0))
		score -= penalty
	}

	// 丢包率影响 (0-25分) - 使用对数函数
	if packetLoss > 0.15 {
		score -= 25
	} else if packetLoss > 0 {
		// 使用对数函数增强对丢包的敏感度
		penalty := 25.0 * math.Log10(1.0+packetLoss*99.0) / 2.0
		score -= penalty
	}

	// 错误率影响 (0-20分)
	if errorRate > 0.1 {
		score -= 20
	} else if errorRate > 0 {
		penalty := 20.0 * math.Sqrt(errorRate*10.0)
		score -= penalty
	}

	// 吞吐量奖励 (0-15分) - 基于对数增长
	if throughput > 0 {
		// 将吞吐量转换为MB/s
		throughputMBps := float64(throughput) / (1024 * 1024)
		if throughputMBps > 100 {
			score += 15
		} else {
			bonus := 15.0 * math.Log10(1.0+throughputMBps) / 2.0
			score += bonus
		}
	}

	// 连接年龄因子将在updateConnectionQuality中处理

	// 确保分数在合理范围内
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

// adaptiveScaling 自适应扩缩容
func (p *AdaptivePool) adaptiveScaling() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.adaptInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performScaling()
		case <-p.ctx.Done():
			return
		}
	}
}

// performScaling 执行扩缩容逻辑
func (p *AdaptivePool) performScaling() {
	p.connMutex.RLock()
	currentSize := len(p.connections)

	// 计算平均质量
	var totalScore float64
	var validConnections int
	for _, conn := range p.connections {
		if p.isConnectionValid(conn) {
			quality := p.getConnectionQuality(conn)
			totalScore += quality.Score
			validConnections++
		}
	}
	p.connMutex.RUnlock()

	if validConnections == 0 {
		return
	}

	avgQuality := totalScore / float64(validConnections)
	p.stats.averageQuality.Store(int64(avgQuality * 100))

	p.logger.Debug("连接池质量评估",
		zap.Int("pool_size", currentSize),
		zap.Int("valid_connections", validConnections),
		zap.Float64("avg_quality", avgQuality))

	// 扩容逻辑
	if avgQuality < p.scaleUpThreshold && currentSize < p.maxConnections {
		// 需要扩容
		newConnections := min(2, p.maxConnections-currentSize)
		p.logger.Info("🔼 连接池扩容",
			zap.Float64("avg_quality", avgQuality),
			zap.Int("current_size", currentSize),
			zap.Int("new_connections", newConnections))

		for i := 0; i < newConnections; i++ {
			if _, err := p.createConnection(); err != nil {
				p.logger.Error("扩容时创建连接失败", zap.Error(err))
				break
			}
		}
		p.stats.scalingEvents.Add(1)
	}

	// 缩容逻辑
	if avgQuality > p.scaleDownThreshold && currentSize > p.minConnections {
		// 可以缩容，移除质量最差的连接
		p.removeWorstConnection()
		p.logger.Info("🔽 连接池缩容",
			zap.Float64("avg_quality", avgQuality),
			zap.Int("pool_size", currentSize-1))
		p.stats.scalingEvents.Add(1)
	}
}

// removeWorstConnection 移除质量最差的连接
func (p *AdaptivePool) removeWorstConnection() {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	var worstConn *PooledConnection
	var worstScore float64 = 101 // 大于最大分数

	for _, conn := range p.connections {
		if conn.inUse.Load() {
			continue // 跳过正在使用的连接
		}

		quality := p.getConnectionQuality(conn)
		if quality.Score < worstScore {
			worstScore = quality.Score
			worstConn = conn
		}
	}

	if worstConn != nil {
		// 关闭并移除连接
		worstConn.cancel()
		worstConn.conn.Close()
		delete(p.connections, worstConn.id)
		p.stats.activeConnections.Add(-1)

		p.logger.Debug("移除低质量连接",
			zap.String("conn_id", worstConn.id),
			zap.Float64("quality_score", worstScore))
	}
}

// qualityMonitoring 质量监控
func (p *AdaptivePool) qualityMonitoring() {
	defer p.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performQualityCheck()
		case <-p.ctx.Done():
			return
		}
	}
}

// performQualityCheck 执行质量检查
func (p *AdaptivePool) performQualityCheck() {
	p.connMutex.RLock()
	connections := make([]*PooledConnection, 0, len(p.connections))
	for _, conn := range p.connections {
		connections = append(connections, conn)
	}
	p.connMutex.RUnlock()

	for _, conn := range connections {
		if !conn.inUse.Load() {
			// 对空闲连接进行RTT测试
			p.performRTTTest(conn)
		}

		// 更新连接质量
		p.updateConnectionQuality(conn)
	}
}

// performRTTTest 执行RTT测试
func (p *AdaptivePool) performRTTTest(conn *PooledConnection) {
	start := time.Now()

	// 简单的连接活性测试 (设置很短的截止时间)
	conn.conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	buffer := make([]byte, 1)
	_, err := conn.conn.Read(buffer)
	conn.conn.SetReadDeadline(time.Time{}) // 清除截止时间

	rtt := time.Since(start)

	// 记录RTT样本
	conn.rttMutex.Lock()
	conn.rttSamples = append(conn.rttSamples, rtt)
	if len(conn.rttSamples) > 10 {
		// 保持最近10个样本
		conn.rttSamples = conn.rttSamples[1:]
	}
	conn.rttMutex.Unlock()

	// 根据测试结果更新统计
	if err != nil {
		// 测试失败可能表示连接问题
		conn.errorCount.Add(1)
	}
	conn.requestCount.Add(1)
}

// connectionCleaner 连接清理器
func (p *AdaptivePool) connectionCleaner() {
	defer p.wg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanupConnections()
		case <-p.ctx.Done():
			return
		}
	}
}

// cleanupConnections 清理无效连接
func (p *AdaptivePool) cleanupConnections() {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	var toRemove []string
	for id, conn := range p.connections {
		if !p.isConnectionValid(conn) && !conn.inUse.Load() {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		conn := p.connections[id]
		conn.cancel()
		conn.conn.Close()
		delete(p.connections, id)
		p.stats.activeConnections.Add(-1)

		p.logger.Debug("清理无效连接", zap.String("conn_id", id))
	}

	if len(toRemove) > 0 {
		p.logger.Info("清理完成", zap.Int("removed", len(toRemove)))
	}
}

// GetStats 获取连接池统计
func (p *AdaptivePool) GetStats() map[string]interface{} {
	p.connMutex.RLock()
	poolSize := len(p.connections)
	var inUseCount int
	for _, conn := range p.connections {
		if conn.inUse.Load() {
			inUseCount++
		}
	}
	p.connMutex.RUnlock()

	return map[string]interface{}{
		"pool_size":          poolSize,
		"connections_in_use": inUseCount,
		"total_connections":  p.stats.totalConnections.Load(),
		"active_connections": p.stats.activeConnections.Load(),
		"failed_connections": p.stats.failedConnections.Load(),
		"pool_hits":          p.stats.poolHits.Load(),
		"pool_misses":        p.stats.poolMisses.Load(),
		"scaling_events":     p.stats.scalingEvents.Load(),
		"average_quality":    float64(p.stats.averageQuality.Load()) / 100.0,
		"target_addr":        p.targetAddr,
		"transport_type":     string(p.transportType),
		"load_balancer":      p.getLoadBalancerStats(),
		"traffic_prediction": p.getTrafficPredictionStats(),
	}
}

// selectConnectionByStrategy 根据策略选择连接（使用高级负载均衡器）
func (p *AdaptivePool) selectConnectionByStrategy(connections []*PooledConnection) *PooledConnection {
	if len(connections) == 0 {
		return nil
	}

	// 使用高级负载均衡器
	if p.loadBalancer != nil {
		return p.loadBalancer.SelectConnection(connections)
	}

	// 备用方案：使用简单策略
	switch p.loadBalanceStrategy {
	case StrategyQualityBased:
		return p.selectByQuality(connections)
	case StrategyRoundRobin:
		return p.selectByRoundRobin(connections)
	case StrategyLeastConn:
		return p.selectByLeastConn(connections)
	case StrategyRandom:
		return p.selectByRandom(connections)
	default:
		return p.selectByQuality(connections)
	}
}

// selectByQuality 基于质量选择连接
func (p *AdaptivePool) selectByQuality(connections []*PooledConnection) *PooledConnection {
	var bestConn *PooledConnection
	var bestScore float64 = -1

	for _, conn := range connections {
		quality := p.getConnectionQuality(conn)
		if quality.Score > bestScore {
			bestScore = quality.Score
			bestConn = conn
		}
	}

	return bestConn
}

// selectByRoundRobin 轮询选择连接
func (p *AdaptivePool) selectByRoundRobin(connections []*PooledConnection) *PooledConnection {
	if len(connections) == 0 {
		return nil
	}

	index := p.roundRobinIndex.Add(1) - 1
	return connections[index%int64(len(connections))]
}

// selectByLeastConn 选择连接数最少的连接
func (p *AdaptivePool) selectByLeastConn(connections []*PooledConnection) *PooledConnection {
	var leastConn *PooledConnection
	var leastCount int64 = math.MaxInt64

	for _, conn := range connections {
		count := conn.requestCount.Load()
		if count < leastCount {
			leastCount = count
			leastConn = conn
		}
	}

	return leastConn
}

// selectByRandom 随机选择连接
func (p *AdaptivePool) selectByRandom(connections []*PooledConnection) *PooledConnection {
	if len(connections) == 0 {
		return nil
	}

	index := rand.Intn(len(connections))
	return connections[index]
}

// SetLoadBalanceStrategy 设置负载均衡策略
func (p *AdaptivePool) SetLoadBalanceStrategy(strategy LoadBalanceStrategy) {
	p.loadBalanceStrategy = strategy

	// 同时更新高级负载均衡器的策略
	if p.loadBalancer != nil {
		p.loadBalancer.SetStrategy(strategy)
	}

	p.logger.Info("🔄 切换负载均衡策略", zap.String("strategy", strategy.String()))
}

// TrafficPredictor 流量预测器
type TrafficPredictor struct {
	historyWindow []TrafficSample
	mutex         sync.RWMutex
	maxSamples    int
}

// TrafficSample 流量样本
type TrafficSample struct {
	Timestamp    time.Time
	Connections  int
	Throughput   int64
	ResponseTime time.Duration
}

// NewTrafficPredictor 创建流量预测器
func NewTrafficPredictor() *TrafficPredictor {
	return &TrafficPredictor{
		historyWindow: make([]TrafficSample, 0),
		maxSamples:    100, // 保持最近100个样本
	}
}

// AddSample 添加流量样本
func (tp *TrafficPredictor) AddSample(connections int, throughput int64, responseTime time.Duration) {
	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	sample := TrafficSample{
		Timestamp:    time.Now(),
		Connections:  connections,
		Throughput:   throughput,
		ResponseTime: responseTime,
	}

	tp.historyWindow = append(tp.historyWindow, sample)
	if len(tp.historyWindow) > tp.maxSamples {
		tp.historyWindow = tp.historyWindow[1:]
	}
}

// PredictOptimalConnections 预测最优连接数
func (tp *TrafficPredictor) PredictOptimalConnections() int {
	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	if len(tp.historyWindow) < 3 {
		return 3 // 默认值
	}

	// 简单的加权平均预测（越近期权重越大）
	var weightSum float64
	var valueSum float64

	for i, sample := range tp.historyWindow {
		weight := float64(i+1) / float64(len(tp.historyWindow))
		weightSum += weight
		valueSum += weight * float64(sample.Connections)
	}

	predicted := int(valueSum / weightSum)

	// 限制在合理范围内
	if predicted < 1 {
		predicted = 1
	}
	if predicted > 50 {
		predicted = 50
	}

	return predicted
}

// performPreWarming 执行连接预热
func (p *AdaptivePool) performPreWarming() {
	p.logger.Info("🔥 开始连接预热", zap.Int("targets", p.preWarmingTargets))

	for i := 0; i < p.preWarmingTargets; i++ {
		go func(index int) {
			if conn, err := p.createConnection(); err != nil {
				p.logger.Warn("预热连接创建失败",
					zap.Int("index", index),
					zap.Error(err))
			} else {
				// 立即释放连接供后续使用
				p.ReleaseConnection(conn)
				p.logger.Debug("预热连接成功",
					zap.Int("index", index),
					zap.String("conn_id", conn.id))
			}
		}(i)
	}
}

// trafficMonitoring 流量监控任务
func (p *AdaptivePool) trafficMonitoring() {
	defer p.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.analyzeTrafficPattern()
		case <-p.ctx.Done():
			return
		}
	}
}

// analyzeTrafficPattern 分析流量模式
func (p *AdaptivePool) analyzeTrafficPattern() {
	if p.trafficPrediction == nil {
		return
	}

	analysis := p.trafficPrediction.GetTrendAnalysis()
	optimalConns := p.trafficPrediction.PredictOptimalConnections()

	p.connMutex.RLock()
	currentSize := len(p.connections)
	p.connMutex.RUnlock()

	p.logger.Debug("📈 流量分析",
		zap.Any("analysis", analysis),
		zap.Int("optimal_conns", optimalConns),
		zap.Int("current_size", currentSize))

	// 根据预测调整连接池大小
	if trend, ok := analysis["trend"].(string); ok {
		switch trend {
		case "increasing":
			if currentSize < optimalConns && currentSize < p.maxConnections {
				p.proactiveScaleUp(optimalConns - currentSize)
			}
		case "decreasing":
			if currentSize > optimalConns && currentSize > p.minConnections {
				p.proactiveScaleDown(currentSize - optimalConns)
			}
		}
	}

	// 动态调整负载均衡策略
	p.adaptLoadBalanceStrategy(analysis)
}

// proactiveScaleUp 主动扩容
func (p *AdaptivePool) proactiveScaleUp(count int) {
	p.logger.Info("🔼 主动扩容", zap.Int("count", count))

	for i := 0; i < count && i < 3; i++ { // 每次最多扩容3个
		go func() {
			if _, err := p.createConnection(); err != nil {
				p.logger.Error("主动扩容失败", zap.Error(err))
			}
		}()
	}
}

// proactiveScaleDown 主动缩容
func (p *AdaptivePool) proactiveScaleDown(count int) {
	p.logger.Info("🔽 主动缩容", zap.Int("count", count))

	for i := 0; i < count && i < 2; i++ { // 每次最多缩容2个
		p.removeWorstConnection()
	}
}

// adaptLoadBalanceStrategy 自适应负载均衡策略
func (p *AdaptivePool) adaptLoadBalanceStrategy(analysis map[string]interface{}) {
	confidence, _ := analysis["confidence"].(float64)

	// 在高置信度下调整策略
	if confidence > 0.7 {
		connTrend, _ := analysis["conn_trend"].(float64)
		throughputTrend, _ := analysis["throughput_trend"].(float64)

		// 高并发场景：优先质量
		if connTrend > 5 && p.loadBalanceStrategy != StrategyQualityBased {
			p.SetLoadBalanceStrategy(StrategyQualityBased)
			return
		}

		// 稳定场景：使用轮询
		if math.Abs(connTrend) < 1 && math.Abs(throughputTrend) < 1000 &&
			p.loadBalanceStrategy != StrategyRoundRobin {
			p.SetLoadBalanceStrategy(StrategyRoundRobin)
			return
		}

		// 低负载场景：最少连接
		if connTrend < -2 && p.loadBalanceStrategy != StrategyLeastConn {
			p.SetLoadBalanceStrategy(StrategyLeastConn)
			return
		}
	}
}

// GetTrendAnalysis 获取趋势分析
func (tp *TrafficPredictor) GetTrendAnalysis() map[string]interface{} {
	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	if len(tp.historyWindow) < 2 {
		return map[string]interface{}{
			"trend":      "stable",
			"confidence": 0.0,
			"samples":    len(tp.historyWindow),
		}
	}

	// 计算趋势
	recentSamples := tp.historyWindow[len(tp.historyWindow)-5:]
	if len(recentSamples) < 2 {
		recentSamples = tp.historyWindow
	}

	first := recentSamples[0]
	last := recentSamples[len(recentSamples)-1]

	connTrend := float64(last.Connections-first.Connections) / float64(len(recentSamples))
	throughputTrend := float64(last.Throughput-first.Throughput) / float64(len(recentSamples))

	var trend string
	if connTrend > 1 {
		trend = "increasing"
	} else if connTrend < -1 {
		trend = "decreasing"
	} else {
		trend = "stable"
	}

	// 简单的置信度计算（基于数据数量和方差）
	confidence := math.Min(1.0, float64(len(recentSamples))/10.0)

	return map[string]interface{}{
		"trend":            trend,
		"confidence":       confidence,
		"conn_trend":       connTrend,
		"throughput_trend": throughputTrend,
		"samples":          len(tp.historyWindow),
		"recent_samples":   len(recentSamples),
	}
}

// getLoadBalancerStats 获取负载均衡器统计
func (p *AdaptivePool) getLoadBalancerStats() map[string]interface{} {
	if p.loadBalancer != nil {
		return p.loadBalancer.GetStats()
	}
	return map[string]interface{}{
		"enabled":  false,
		"strategy": p.loadBalanceStrategy.String(),
	}
}

// getTrafficPredictionStats 获取流量预测统计
func (p *AdaptivePool) getTrafficPredictionStats() map[string]interface{} {
	if p.trafficPrediction != nil {
		analysis := p.trafficPrediction.GetTrendAnalysis()
		optimal := p.trafficPrediction.PredictOptimalConnections()
		analysis["predicted_optimal_connections"] = optimal
		return analysis
	}
	return map[string]interface{}{
		"enabled": false,
	}
}

// UpdateConnectionStats 更新连接统计（用于负载均衡器）
func (p *AdaptivePool) UpdateConnectionStats(conn *PooledConnection, responseTime time.Duration, success bool) {
	if p.loadBalancer != nil {
		p.loadBalancer.UpdateStats(conn, responseTime, success)
	}
}

// GetConnectionById 根据ID获取连接
func (p *AdaptivePool) GetConnectionById(id string) (*PooledConnection, bool) {
	p.connMutex.RLock()
	defer p.connMutex.RUnlock()
	conn, exists := p.connections[id]
	return conn, exists
}

// GetActiveConnectionsCount 获取活跃连接数
func (p *AdaptivePool) GetActiveConnectionsCount() int {
	p.connMutex.RLock()
	defer p.connMutex.RUnlock()

	var activeCount int
	for _, conn := range p.connections {
		if conn.inUse.Load() {
			activeCount++
		}
	}
	return activeCount
}

// GetConnectionsInfo 获取所有连接信息
func (p *AdaptivePool) GetConnectionsInfo() []map[string]interface{} {
	p.connMutex.RLock()
	defer p.connMutex.RUnlock()

	infos := make([]map[string]interface{}, 0, len(p.connections))
	for _, conn := range p.connections {
		info := map[string]interface{}{
			"id":         conn.id,
			"created_at": conn.createdAt,
			"last_used":  conn.lastUsed,
			"in_use":     conn.inUse.Load(),
			"bytes_in":   conn.bytesIn.Load(),
			"bytes_out":  conn.bytesOut.Load(),
			"requests":   conn.requestCount.Load(),
			"errors":     conn.errorCount.Load(),
			"quality":    p.getConnectionQuality(conn),
		}
		infos = append(infos, info)
	}
	return infos
}

// 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
