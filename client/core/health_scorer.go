package core

import (
	"sync/atomic"
	"time"
)

// HealthScorer 节点健康评分系统
type HealthScorer struct {
	latency      atomic.Int64 // 延迟(ms)
	stability    atomic.Int64 // 稳定性得分(0-100)
	load         atomic.Int64 // 负载(0-100)
	errorCount   atomic.Int64 // 错误计数
	successCount atomic.Int64 // 成功计数
	lastUpdate   atomic.Value // time.Time
}

// NewHealthScorer 创建健康评分器
func NewHealthScorer() *HealthScorer {
	hs := &HealthScorer{}
	hs.stability.Store(100)
	hs.lastUpdate.Store(time.Now())
	return hs
}

// RecordLatency 记录延迟
func (hs *HealthScorer) RecordLatency(latency time.Duration) {
	hs.latency.Store(latency.Milliseconds())
	hs.lastUpdate.Store(time.Now())
}

// RecordSuccess 记录成功
func (hs *HealthScorer) RecordSuccess() {
	hs.successCount.Add(1)
	hs.updateStability()
}

// RecordError 记录错误
func (hs *HealthScorer) RecordError() {
	hs.errorCount.Add(1)
	hs.updateStability()
}

// SetLoad 设置负载
func (hs *HealthScorer) SetLoad(load int64) {
	if load < 0 {
		load = 0
	}
	if load > 100 {
		load = 100
	}
	hs.load.Store(load)
}

// updateStability 更新稳定性得分
func (hs *HealthScorer) updateStability() {
	total := hs.successCount.Load() + hs.errorCount.Load()
	if total == 0 {
		return
	}

	successRate := float64(hs.successCount.Load()) / float64(total)
	stability := int64(successRate * 100)
	hs.stability.Store(stability)
}

// CalculateScore 计算综合健康评分
func (hs *HealthScorer) CalculateScore() int {
	score := 100

	// 延迟影响 (0-30分)
	latency := hs.latency.Load()
	if latency > 500 {
		score -= 30
	} else if latency > 200 {
		score -= 20
	} else if latency > 100 {
		score -= 10
	}

	// 稳定性影响 (0-40分)
	stability := hs.stability.Load()
	score -= int((100 - stability) * 40 / 100)

	// 负载影响 (0-30分)
	load := hs.load.Load()
	if load > 90 {
		score -= 30
	} else if load > 70 {
		score -= 20
	} else if load > 50 {
		score -= 10
	}

	if score < 0 {
		score = 0
	}

	return score
}

// GetMetrics 获取指标
func (hs *HealthScorer) GetMetrics() HealthMetrics {
	return HealthMetrics{
		Latency:      time.Duration(hs.latency.Load()) * time.Millisecond,
		Stability:    hs.stability.Load(),
		Load:         hs.load.Load(),
		ErrorCount:   hs.errorCount.Load(),
		SuccessCount: hs.successCount.Load(),
		Score:        hs.CalculateScore(),
		LastUpdate:   hs.lastUpdate.Load().(time.Time),
	}
}

// HealthMetrics 健康指标
type HealthMetrics struct {
	Latency      time.Duration
	Stability    int64
	Load         int64
	ErrorCount   int64
	SuccessCount int64
	Score        int
	LastUpdate   time.Time
}

