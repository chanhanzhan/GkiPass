package pool

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// LoadBalancer 负载均衡器接口
type LoadBalancer interface {
	// SelectConnection 选择最佳连接
	SelectConnection(connections []*PooledConnection) *PooledConnection

	// UpdateStats 更新连接统计
	UpdateStats(conn *PooledConnection, responseTime time.Duration, success bool)

	// GetStrategy 获取当前策略
	GetStrategy() LoadBalanceStrategy

	// SetStrategy 设置策略
	SetStrategy(strategy LoadBalanceStrategy)

	// GetStats 获取负载均衡器统计
	GetStats() map[string]interface{}

	// CleanupStaleConnections 清理过期连接记录
	CleanupStaleConnections(activeConnIDs []string)

	// StartWeightUpdateLoop 启动权重更新循环
	StartWeightUpdateLoop(ctx context.Context)
}

// AdvancedLoadBalancer 高级负载均衡器
type AdvancedLoadBalancer struct {
	strategy      LoadBalanceStrategy
	mutex         sync.RWMutex
	roundRobinIdx atomic.Int64

	// 连接权重 (connection ID -> weight)
	weights      map[string]float64
	weightsMutex sync.RWMutex

	// 性能历史
	perfHistory map[string]*ConnectionPerformance
	perfMutex   sync.RWMutex

	// 自适应参数
	adaptiveConfig *AdaptiveConfig

	logger *zap.Logger
}

// ConnectionPerformance 连接性能记录
type ConnectionPerformance struct {
	ResponseTimes   []time.Duration `json:"response_times"`
	SuccessCount    atomic.Int64    `json:"success_count"`
	FailureCount    atomic.Int64    `json:"failure_count"`
	LastUpdate      time.Time       `json:"last_update"`
	Weight          float64         `json:"weight"`
	AvgResponseTime time.Duration   `json:"avg_response_time"`
	SuccessRate     float64         `json:"success_rate"`
	Momentum        float64         `json:"momentum"` // 动量因子，用于平滑权重变化
}

// AdaptiveConfig 自适应配置
type AdaptiveConfig struct {
	// 权重调整参数
	WeightDecayFactor float64 `json:"weight_decay_factor"` // 权重衰减因子
	LearningRate      float64 `json:"learning_rate"`       // 学习率
	MomentumFactor    float64 `json:"momentum_factor"`     // 动量因子

	// 性能窗口
	PerformanceWindow int           `json:"performance_window"` // 性能样本窗口大小
	UpdateInterval    time.Duration `json:"update_interval"`    // 权重更新间隔

	// 阈值设置
	MinWeight      float64 `json:"min_weight"`      // 最小权重
	MaxWeight      float64 `json:"max_weight"`      // 最大权重
	FailurePenalty float64 `json:"failure_penalty"` // 失败惩罚
	SuccessReward  float64 `json:"success_reward"`  // 成功奖励
}

// NewAdvancedLoadBalancer 创建高级负载均衡器
func NewAdvancedLoadBalancer(strategy LoadBalanceStrategy) *AdvancedLoadBalancer {
	return &AdvancedLoadBalancer{
		strategy:       strategy,
		weights:        make(map[string]float64),
		perfHistory:    make(map[string]*ConnectionPerformance),
		adaptiveConfig: DefaultAdaptiveConfig(),
		logger:         zap.L().Named("advanced-lb"),
	}
}

// DefaultAdaptiveConfig 默认自适应配置
func DefaultAdaptiveConfig() *AdaptiveConfig {
	return &AdaptiveConfig{
		WeightDecayFactor: 0.95, // 权重每次更新衰减5%
		LearningRate:      0.1,  // 10%的学习率
		MomentumFactor:    0.9,  // 90%的动量保持
		PerformanceWindow: 20,   // 保持最近20个性能样本
		UpdateInterval:    5 * time.Second,
		MinWeight:         0.1, // 最小权重10%
		MaxWeight:         2.0, // 最大权重200%
		FailurePenalty:    0.2, // 失败惩罚20%
		SuccessReward:     0.1, // 成功奖励10%
	}
}

// SelectConnection 选择最佳连接
func (alb *AdvancedLoadBalancer) SelectConnection(connections []*PooledConnection) *PooledConnection {
	if len(connections) == 0 {
		return nil
	}

	if len(connections) == 1 {
		return connections[0]
	}

	alb.mutex.RLock()
	strategy := alb.strategy
	alb.mutex.RUnlock()

	switch strategy {
	case StrategyQualityBased:
		return alb.selectByQualityWeighted(connections)
	case StrategyRoundRobin:
		return alb.selectByWeightedRoundRobin(connections)
	case StrategyLeastConn:
		return alb.selectByWeightedLeastConn(connections)
	case StrategyRandom:
		return alb.selectByWeightedRandom(connections)
	default:
		return alb.selectByQualityWeighted(connections)
	}
}

// selectByQualityWeighted 基于质量的加权选择
func (alb *AdvancedLoadBalancer) selectByQualityWeighted(connections []*PooledConnection) *PooledConnection {
	var bestConn *PooledConnection
	var bestScore float64 = -1

	for _, conn := range connections {
		// 获取连接质量分数
		quality := alb.getConnectionQuality(conn)

		// 获取权重
		weight := alb.getConnectionWeight(conn.id)

		// 计算加权分数
		score := quality.Score * weight

		if score > bestScore {
			bestScore = score
			bestConn = conn
		}
	}

	return bestConn
}

// selectByWeightedRoundRobin 加权轮询选择
func (alb *AdvancedLoadBalancer) selectByWeightedRoundRobin(connections []*PooledConnection) *PooledConnection {
	// 计算总权重
	var totalWeight float64
	weights := make([]float64, len(connections))

	for i, conn := range connections {
		weight := alb.getConnectionWeight(conn.id)
		weights[i] = weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		// 如果总权重为0，使用普通轮询
		idx := alb.roundRobinIdx.Add(1) - 1
		return connections[idx%int64(len(connections))]
	}

	// 加权轮询选择
	target := rand.Float64() * totalWeight
	var current float64

	for i, weight := range weights {
		current += weight
		if current >= target {
			return connections[i]
		}
	}

	// 兜底返回最后一个
	return connections[len(connections)-1]
}

// selectByWeightedLeastConn 加权最少连接选择
func (alb *AdvancedLoadBalancer) selectByWeightedLeastConn(connections []*PooledConnection) *PooledConnection {
	var bestConn *PooledConnection
	var bestRatio float64 = math.MaxFloat64

	for _, conn := range connections {
		connCount := float64(conn.requestCount.Load())
		weight := alb.getConnectionWeight(conn.id)

		// 计算连接数与权重的比值（权重越高，比值越小越好）
		ratio := connCount / math.Max(weight, 0.1)

		if ratio < bestRatio {
			bestRatio = ratio
			bestConn = conn
		}
	}

	return bestConn
}

// selectByWeightedRandom 加权随机选择
func (alb *AdvancedLoadBalancer) selectByWeightedRandom(connections []*PooledConnection) *PooledConnection {
	// 计算总权重
	var totalWeight float64
	for _, conn := range connections {
		weight := alb.getConnectionWeight(conn.id)
		totalWeight += weight
	}

	if totalWeight == 0 {
		// 如果总权重为0，使用普通随机
		return connections[rand.Intn(len(connections))]
	}

	// 加权随机选择
	target := rand.Float64() * totalWeight
	var current float64

	for _, conn := range connections {
		weight := alb.getConnectionWeight(conn.id)
		current += weight
		if current >= target {
			return conn
		}
	}

	// 兜底返回最后一个
	return connections[len(connections)-1]
}

// UpdateStats 更新连接统计
func (alb *AdvancedLoadBalancer) UpdateStats(conn *PooledConnection, responseTime time.Duration, success bool) {
	alb.perfMutex.Lock()
	defer alb.perfMutex.Unlock()

	perf, exists := alb.perfHistory[conn.id]
	if !exists {
		perf = &ConnectionPerformance{
			ResponseTimes: make([]time.Duration, 0, alb.adaptiveConfig.PerformanceWindow),
			Weight:        1.0, // 初始权重
			Momentum:      0.0, // 初始动量
		}
		alb.perfHistory[conn.id] = perf
	}

	// 更新响应时间
	perf.ResponseTimes = append(perf.ResponseTimes, responseTime)
	if len(perf.ResponseTimes) > alb.adaptiveConfig.PerformanceWindow {
		perf.ResponseTimes = perf.ResponseTimes[1:]
	}

	// 更新成功/失败计数
	if success {
		perf.SuccessCount.Add(1)
	} else {
		perf.FailureCount.Add(1)
	}

	perf.LastUpdate = time.Now()

	// 计算平均响应时间
	if len(perf.ResponseTimes) > 0 {
		var total time.Duration
		for _, rt := range perf.ResponseTimes {
			total += rt
		}
		perf.AvgResponseTime = total / time.Duration(len(perf.ResponseTimes))
	}

	// 计算成功率
	totalRequests := perf.SuccessCount.Load() + perf.FailureCount.Load()
	if totalRequests > 0 {
		perf.SuccessRate = float64(perf.SuccessCount.Load()) / float64(totalRequests)
	}

	// 更新权重
	alb.updateWeight(conn.id, perf)
}

// updateWeight 更新连接权重
func (alb *AdvancedLoadBalancer) updateWeight(connID string, perf *ConnectionPerformance) {
	cfg := alb.adaptiveConfig

	// 计算基础性能分数 (0-1)
	var perfScore float64 = 0.5 // 默认中等分数

	// 响应时间因子 (越快分数越高)
	if perf.AvgResponseTime > 0 {
		// 假设100ms是理想响应时间
		idealResponseTime := 100 * time.Millisecond
		rtFactor := math.Max(0.1, math.Min(1.0, float64(idealResponseTime)/float64(perf.AvgResponseTime)))
		perfScore *= rtFactor
	}

	// 成功率因子
	perfScore *= perf.SuccessRate

	// 计算权重变化
	targetWeight := perfScore * 2.0 // 目标权重范围 0-2

	// 使用动量更新权重
	weightDelta := cfg.LearningRate * (targetWeight - perf.Weight)
	perf.Momentum = cfg.MomentumFactor*perf.Momentum + (1.0-cfg.MomentumFactor)*weightDelta
	newWeight := perf.Weight + perf.Momentum

	// 应用权重衰减
	newWeight *= cfg.WeightDecayFactor

	// 限制权重范围
	newWeight = math.Max(cfg.MinWeight, math.Min(cfg.MaxWeight, newWeight))

	perf.Weight = newWeight

	// 更新权重映射
	alb.weightsMutex.Lock()
	alb.weights[connID] = newWeight
	alb.weightsMutex.Unlock()

	alb.logger.Debug("🔄 更新连接权重",
		zap.String("conn_id", connID),
		zap.Float64("old_weight", perf.Weight),
		zap.Float64("new_weight", newWeight),
		zap.Float64("perf_score", perfScore),
		zap.Float64("success_rate", perf.SuccessRate),
		zap.Duration("avg_rt", perf.AvgResponseTime))
}

// getConnectionWeight 获取连接权重
func (alb *AdvancedLoadBalancer) getConnectionWeight(connID string) float64 {
	alb.weightsMutex.RLock()
	defer alb.weightsMutex.RUnlock()

	if weight, exists := alb.weights[connID]; exists {
		return weight
	}
	return 1.0 // 默认权重
}

// getConnectionQuality 获取连接质量 (这里需要与AdaptivePool集成)
func (alb *AdvancedLoadBalancer) getConnectionQuality(conn *PooledConnection) *ConnectionQuality {
	if quality := conn.quality.Load(); quality != nil {
		return quality.(*ConnectionQuality)
	}

	// 返回默认质量
	return &ConnectionQuality{
		RTT:        100 * time.Millisecond,
		PacketLoss: 0.0,
		Throughput: 0,
		ErrorRate:  0.0,
		LastUpdate: time.Time{},
		Score:      50.0, // 默认中等分数
	}
}

// GetStrategy 获取当前策略
func (alb *AdvancedLoadBalancer) GetStrategy() LoadBalanceStrategy {
	alb.mutex.RLock()
	defer alb.mutex.RUnlock()
	return alb.strategy
}

// SetStrategy 设置策略
func (alb *AdvancedLoadBalancer) SetStrategy(strategy LoadBalanceStrategy) {
	alb.mutex.Lock()
	defer alb.mutex.Unlock()

	if alb.strategy != strategy {
		alb.logger.Info("🔄 切换负载均衡策略",
			zap.String("old", alb.strategy.String()),
			zap.String("new", strategy.String()))
		alb.strategy = strategy
	}
}

// GetStats 获取负载均衡器统计
func (alb *AdvancedLoadBalancer) GetStats() map[string]interface{} {
	alb.perfMutex.RLock()
	defer alb.perfMutex.RUnlock()

	connections := make(map[string]interface{})
	var totalWeight float64
	var avgSuccessRate float64
	var avgResponseTime time.Duration
	count := 0

	for connID, perf := range alb.perfHistory {
		totalWeight += perf.Weight
		avgSuccessRate += perf.SuccessRate
		avgResponseTime += perf.AvgResponseTime
		count++

		connections[connID] = map[string]interface{}{
			"weight":            perf.Weight,
			"success_rate":      perf.SuccessRate,
			"avg_response_time": perf.AvgResponseTime,
			"success_count":     perf.SuccessCount.Load(),
			"failure_count":     perf.FailureCount.Load(),
			"last_update":       perf.LastUpdate,
			"momentum":          perf.Momentum,
		}
	}

	if count > 0 {
		avgSuccessRate /= float64(count)
		avgResponseTime /= time.Duration(count)
	}

	return map[string]interface{}{
		"strategy":          alb.strategy.String(),
		"connection_count":  count,
		"total_weight":      totalWeight,
		"avg_success_rate":  avgSuccessRate,
		"avg_response_time": avgResponseTime,
		"connections":       connections,
		"adaptive_config":   alb.adaptiveConfig,
	}
}

// CleanupStaleConnections 清理过期连接记录
func (alb *AdvancedLoadBalancer) CleanupStaleConnections(activeConnIDs []string) {
	alb.perfMutex.Lock()
	defer alb.perfMutex.Unlock()

	activeSet := make(map[string]bool)
	for _, id := range activeConnIDs {
		activeSet[id] = true
	}

	var removedCount int
	for connID := range alb.perfHistory {
		if !activeSet[connID] {
			delete(alb.perfHistory, connID)
			removedCount++
		}
	}

	// 同时清理权重映射
	alb.weightsMutex.Lock()
	for connID := range alb.weights {
		if !activeSet[connID] {
			delete(alb.weights, connID)
		}
	}
	alb.weightsMutex.Unlock()

	if removedCount > 0 {
		alb.logger.Info("🧹 清理过期连接记录", zap.Int("removed", removedCount))
	}
}

// StartWeightUpdateLoop 启动权重更新循环
func (alb *AdvancedLoadBalancer) StartWeightUpdateLoop(ctx context.Context) {
	ticker := time.NewTicker(alb.adaptiveConfig.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			alb.periodicWeightUpdate()
		case <-ctx.Done():
			return
		}
	}
}

// periodicWeightUpdate 定期权重更新
func (alb *AdvancedLoadBalancer) periodicWeightUpdate() {
	alb.perfMutex.Lock()
	defer alb.perfMutex.Unlock()

	now := time.Now()
	var updatedCount int

	for connID, perf := range alb.perfHistory {
		// 只更新最近有活动的连接
		if now.Sub(perf.LastUpdate) < 30*time.Second {
			alb.updateWeight(connID, perf)
			updatedCount++
		}
	}

	if updatedCount > 0 {
		alb.logger.Debug("📊 定期权重更新", zap.Int("updated", updatedCount))
	}
}
