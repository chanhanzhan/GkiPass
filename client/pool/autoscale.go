package pool

import (
	"sync"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// PoolMetrics 连接池性能指标
type PoolMetrics struct {
	RTT        time.Duration
	LossRate   float64
	ActiveRate float64 // 活跃连接比率
}

// AutoScaler 自适应伸缩器
type AutoScaler struct {
	pool       *Pool
	rttSamples []time.Duration
	maxSamples int
	mu         sync.RWMutex
	stopChan   chan struct{}
}

// NewAutoScaler 创建自适应伸缩器
func NewAutoScaler(pool *Pool) *AutoScaler {
	return &AutoScaler{
		pool:       pool,
		rttSamples: make([]time.Duration, 0, 100),
		maxSamples: 100,
		stopChan:   make(chan struct{}),
	}
}

// Start 启动自适应伸缩
func (as *AutoScaler) Start() {
	go as.scaleLoop()
	logger.Info("自适应伸缩器已启动")
}

// Stop 停止自适应伸缩
func (as *AutoScaler) Stop() {
	close(as.stopChan)
	logger.Info("自适应伸缩器已停止")
}

// AddRTTSample 添加RTT样本
func (as *AutoScaler) AddRTTSample(rtt time.Duration) {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.rttSamples = append(as.rttSamples, rtt)
	if len(as.rttSamples) > as.maxSamples {
		// 保留最近的样本
		as.rttSamples = as.rttSamples[len(as.rttSamples)-as.maxSamples:]
	}
}

// GetMetrics 获取性能指标
func (as *AutoScaler) GetMetrics() PoolMetrics {
	as.mu.RLock()
	defer as.mu.RUnlock()

	metrics := PoolMetrics{}

	// 计算平均RTT
	if len(as.rttSamples) > 0 {
		var total time.Duration
		for _, rtt := range as.rttSamples {
			total += rtt
		}
		metrics.RTT = total / time.Duration(len(as.rttSamples))
	}

	// 获取连接池统计
	stats := as.pool.Stats()
	if stats.IdleCount+stats.ActiveCount > 0 {
		metrics.ActiveRate = float64(stats.ActiveCount) / float64(stats.IdleCount+stats.ActiveCount)
	}

	return metrics
}

// scaleLoop 伸缩循环
func (as *AutoScaler) scaleLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			as.scale()
		case <-as.stopChan:
			return
		}
	}
}

// scale 执行伸缩决策
func (as *AutoScaler) scale() {
	metrics := as.GetMetrics()
	stats := as.pool.Stats()

	shouldScaleUp := false
	shouldScaleDown := false

	// RTT 过高（超过200ms）
	if metrics.RTT > 200*time.Millisecond && stats.ActiveCount < stats.MaxConns {
		shouldScaleUp = true
		logger.Info("RTT过高，扩容连接池",
			zap.Duration("rtt", metrics.RTT),
			zap.Int("current", stats.ActiveCount))
	}

	// 活跃连接比率过高（超过80%）
	if metrics.ActiveRate > 0.8 && stats.ActiveCount < stats.MaxConns {
		shouldScaleUp = true
		logger.Info("活跃率过高，扩容连接池",
			zap.Float64("active_rate", metrics.ActiveRate),
			zap.Int("current", stats.ActiveCount))
	}

	// 空闲连接过多（空闲连接 > 最小连接数 * 2）
	if stats.IdleCount > stats.MinConns*2 {
		shouldScaleDown = true
		logger.Info("空闲连接过多，缩容连接池",
			zap.Int("idle", stats.IdleCount),
			zap.Int("min", stats.MinConns))
	}

	if shouldScaleUp {
		// 暂时不实现自动扩容，因为连接池会按需创建
		logger.Debug("建议扩容")
	} else if shouldScaleDown {
		// 暂时不实现自动缩容，由cleanup处理
		logger.Debug("建议缩容")
	}
}

// MeasureRTT 测量RTT
func (as *AutoScaler) MeasureRTT(target string) time.Duration {
	start := time.Now()
	conn, err := as.pool.Get()
	if err != nil {
		return 0
	}
	rtt := time.Since(start)
	as.pool.Put(conn)

	as.AddRTTSample(rtt)
	return rtt
}






