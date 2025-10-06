package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/ws"
)

// LoadBalancerStrategy 负载均衡策略
type LoadBalancerStrategy string

const (
	StrategyWeightedRoundRobin LoadBalancerStrategy = "weighted_round_robin" // 加权轮询
	StrategyLeastConnections   LoadBalancerStrategy = "least_connections"    // 最小连接数
	StrategyFastestResponse    LoadBalancerStrategy = "fastest_response"     // 响应时间优先
	StrategyRandom             LoadBalancerStrategy = "random"               // 随机
)

// AdvancedLoadBalancer 高级负载均衡器
type AdvancedLoadBalancer struct {
	targets   []*TargetWithMetrics
	strategy  LoadBalancerStrategy
	mu        sync.RWMutex
	rrCounter atomic.Uint64 // 轮询计数器
}

// TargetWithMetrics 带指标的目标
type TargetWithMetrics struct {
	ws.TunnelTarget
	Connections atomic.Int64 // 当前连接数
	TotalConns  atomic.Int64 // 总连接数
	Failures    atomic.Int64 // 失败次数
	AvgRTT      atomic.Int64 // 平均RTT (纳秒)
	Healthy     atomic.Bool  // 健康状态
	LastCheck   atomic.Value // time.Time
	LastUsed    atomic.Value // time.Time
}

// NewAdvancedLoadBalancer 创建高级负载均衡器
func NewAdvancedLoadBalancer(targets []ws.TunnelTarget, strategy LoadBalancerStrategy) *AdvancedLoadBalancer {
	alb := &AdvancedLoadBalancer{
		targets:  make([]*TargetWithMetrics, len(targets)),
		strategy: strategy,
	}

	for i, target := range targets {
		tm := &TargetWithMetrics{
			TunnelTarget: target,
		}
		tm.Healthy.Store(true)
		tm.LastCheck.Store(time.Now())
		tm.LastUsed.Store(time.Time{})
		alb.targets[i] = tm
	}

	return alb
}

// SelectTarget 选择目标
func (alb *AdvancedLoadBalancer) SelectTarget() (*TargetWithMetrics, error) {
	alb.mu.RLock()
	defer alb.mu.RUnlock()

	// 过滤健康的目标
	healthy := make([]*TargetWithMetrics, 0, len(alb.targets))
	for _, t := range alb.targets {
		if t.Healthy.Load() {
			healthy = append(healthy, t)
		}
	}

	if len(healthy) == 0 {
		// 所有目标都不健康，尝试使用任意目标
		if len(alb.targets) > 0 {
			return alb.targets[0], nil
		}
		return nil, ErrNoHealthyTarget
	}

	switch alb.strategy {
	case StrategyWeightedRoundRobin:
		return alb.weightedRoundRobin(healthy), nil
	case StrategyLeastConnections:
		return alb.leastConnections(healthy), nil
	case StrategyFastestResponse:
		return alb.fastestResponse(healthy), nil
	case StrategyRandom:
		return alb.random(healthy), nil
	default:
		return alb.weightedRoundRobin(healthy), nil
	}
}

// weightedRoundRobin 加权轮询
func (alb *AdvancedLoadBalancer) weightedRoundRobin(targets []*TargetWithMetrics) *TargetWithMetrics {
	// 计算总权重
	totalWeight := 0
	for _, t := range targets {
		if t.Weight <= 0 {
			t.Weight = 1
		}
		totalWeight += t.Weight
	}

	// 获取当前计数并递增
	count := alb.rrCounter.Add(1) - 1
	position := int(count % uint64(totalWeight))

	// 根据权重选择
	for _, t := range targets {
		if position < t.Weight {
			return t
		}
		position -= t.Weight
	}

	return targets[0]
}

// leastConnections 最小连接数
func (alb *AdvancedLoadBalancer) leastConnections(targets []*TargetWithMetrics) *TargetWithMetrics {
	minConns := int64(-1)
	var selected *TargetWithMetrics

	for _, t := range targets {
		conns := t.Connections.Load()
		if minConns == -1 || conns < minConns {
			minConns = conns
			selected = t
		}
	}

	return selected
}

// fastestResponse 响应时间优先
func (alb *AdvancedLoadBalancer) fastestResponse(targets []*TargetWithMetrics) *TargetWithMetrics {
	minRTT := int64(-1)
	var selected *TargetWithMetrics

	for _, t := range targets {
		rtt := t.AvgRTT.Load()
		if rtt == 0 {
			// 未测量RTT的目标，给予机会
			return t
		}
		if minRTT == -1 || rtt < minRTT {
			minRTT = rtt
			selected = t
		}
	}

	if selected == nil && len(targets) > 0 {
		return targets[0]
	}

	return selected
}

// random 随机选择
func (alb *AdvancedLoadBalancer) random(targets []*TargetWithMetrics) *TargetWithMetrics {
	idx := time.Now().UnixNano() % int64(len(targets))
	return targets[idx]
}

// RecordConnection 记录连接
func (alb *AdvancedLoadBalancer) RecordConnection(target *TargetWithMetrics, success bool) {
	if success {
		target.Connections.Add(1)
		target.TotalConns.Add(1)
		target.LastUsed.Store(time.Now())
	} else {
		target.Failures.Add(1)
	}
}

// ReleaseConnection 释放连接
func (alb *AdvancedLoadBalancer) ReleaseConnection(target *TargetWithMetrics) {
	target.Connections.Add(-1)
}

// UpdateRTT 更新RTT
func (alb *AdvancedLoadBalancer) UpdateRTT(target *TargetWithMetrics, rtt time.Duration) {
	// 使用指数移动平均
	oldRTT := target.AvgRTT.Load()
	if oldRTT == 0 {
		target.AvgRTT.Store(rtt.Nanoseconds())
	} else {
		// EMA: new = 0.9 * old + 0.1 * current
		newRTT := int64(float64(oldRTT)*0.9 + float64(rtt.Nanoseconds())*0.1)
		target.AvgRTT.Store(newRTT)
	}
}

// UpdateHealth 更新健康状态
func (alb *AdvancedLoadBalancer) UpdateHealth(target *TargetWithMetrics, healthy bool) {
	target.Healthy.Store(healthy)
	target.LastCheck.Store(time.Now())
}

// GetStats 获取统计信息
func (alb *AdvancedLoadBalancer) GetStats() []TargetStats {
	alb.mu.RLock()
	defer alb.mu.RUnlock()

	stats := make([]TargetStats, len(alb.targets))
	for i, t := range alb.targets {
		stats[i] = TargetStats{
			Host:        t.Host,
			Port:        t.Port,
			Weight:      t.Weight,
			Connections: t.Connections.Load(),
			TotalConns:  t.TotalConns.Load(),
			Failures:    t.Failures.Load(),
			AvgRTT:      time.Duration(t.AvgRTT.Load()),
			Healthy:     t.Healthy.Load(),
		}
	}

	return stats
}

// TargetStats 目标统计
type TargetStats struct {
	Host        string
	Port        int
	Weight      int
	Connections int64
	TotalConns  int64
	Failures    int64
	AvgRTT      time.Duration
	Healthy     bool
}

var ErrNoHealthyTarget = fmt.Errorf("no healthy target available")

