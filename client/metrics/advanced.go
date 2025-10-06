package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// AdvancedMetrics 高级指标收集器
type AdvancedMetrics struct {
	// 延迟统计
	latencySum   atomic.Int64
	latencyCount atomic.Int64
	latencyMin   atomic.Int64
	latencyMax   atomic.Int64

	// 丢包统计
	packetsSent atomic.Int64
	packetsLost atomic.Int64

	// 吞吐量统计
	throughputIn  atomic.Int64
	throughputOut atomic.Int64
	lastMeasure   atomic.Value // time.Time

	// 错误统计
	timeoutErrors atomic.Int64
	connErrors    atomic.Int64
	otherErrors   atomic.Int64

	// 连接质量
	rttSamples    []time.Duration
	jitterSamples []time.Duration
	mu            sync.RWMutex
}

// NewAdvancedMetrics 创建高级指标收集器
func NewAdvancedMetrics() *AdvancedMetrics {
	am := &AdvancedMetrics{
		rttSamples:    make([]time.Duration, 0, 100),
		jitterSamples: make([]time.Duration, 0, 100),
	}
	am.latencyMin.Store(int64(time.Hour)) // 初始化为一个大值
	am.lastMeasure.Store(time.Now())
	return am
}

// RecordLatency 记录延迟
func (am *AdvancedMetrics) RecordLatency(duration time.Duration) {
	latencyNs := duration.Nanoseconds()

	am.latencySum.Add(latencyNs)
	am.latencyCount.Add(1)

	// 更新最小值
	for {
		old := am.latencyMin.Load()
		if latencyNs >= old {
			break
		}
		if am.latencyMin.CompareAndSwap(old, latencyNs) {
			break
		}
	}

	// 更新最大值
	for {
		old := am.latencyMax.Load()
		if latencyNs <= old {
			break
		}
		if am.latencyMax.CompareAndSwap(old, latencyNs) {
			break
		}
	}
}

// RecordPacketLoss 记录丢包
func (am *AdvancedMetrics) RecordPacketLoss(sent, lost int64) {
	am.packetsSent.Add(sent)
	am.packetsLost.Add(lost)
}

// RecordThroughput 记录吞吐量
func (am *AdvancedMetrics) RecordThroughput(bytesIn, bytesOut int64) {
	am.throughputIn.Add(bytesIn)
	am.throughputOut.Add(bytesOut)
}

// RecordError 记录错误
func (am *AdvancedMetrics) RecordError(errType string) {
	switch errType {
	case "timeout":
		am.timeoutErrors.Add(1)
	case "connection":
		am.connErrors.Add(1)
	default:
		am.otherErrors.Add(1)
	}
}

// RecordRTT 记录往返时间
func (am *AdvancedMetrics) RecordRTT(rtt time.Duration) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if len(am.rttSamples) >= 100 {
		am.rttSamples = am.rttSamples[1:]
	}
	am.rttSamples = append(am.rttSamples, rtt)

	// 计算抖动（如果有前一个样本）
	if len(am.rttSamples) >= 2 {
		prev := am.rttSamples[len(am.rttSamples)-2]
		jitter := rtt - prev
		if jitter < 0 {
			jitter = -jitter
		}

		if len(am.jitterSamples) >= 100 {
			am.jitterSamples = am.jitterSamples[1:]
		}
		am.jitterSamples = append(am.jitterSamples, jitter)
	}
}

// GetMetricsSnapshot 获取指标快照
func (am *AdvancedMetrics) GetMetricsSnapshot() AdvancedMetricsSnapshot {
	am.mu.RLock()
	defer am.mu.RUnlock()

	count := am.latencyCount.Load()
	var avgLatency, minLatency, maxLatency time.Duration
	if count > 0 {
		avgLatency = time.Duration(am.latencySum.Load() / count)
		minLatency = time.Duration(am.latencyMin.Load())
		maxLatency = time.Duration(am.latencyMax.Load())
	}

	sent := am.packetsSent.Load()
	lost := am.packetsLost.Load()
	var lossRate float64
	if sent > 0 {
		lossRate = float64(lost) / float64(sent) * 100
	}

	lastMeasure := am.lastMeasure.Load().(time.Time)
	elapsed := time.Since(lastMeasure).Seconds()
	var throughputInMbps, throughputOutMbps float64
	if elapsed > 0 {
		throughputInMbps = float64(am.throughputIn.Load()*8) / elapsed / 1_000_000
		throughputOutMbps = float64(am.throughputOut.Load()*8) / elapsed / 1_000_000
	}

	// 计算平均RTT和抖动
	var avgRTT, avgJitter time.Duration
	if len(am.rttSamples) > 0 {
		var sum time.Duration
		for _, rtt := range am.rttSamples {
			sum += rtt
		}
		avgRTT = sum / time.Duration(len(am.rttSamples))
	}
	if len(am.jitterSamples) > 0 {
		var sum time.Duration
		for _, jitter := range am.jitterSamples {
			sum += jitter
		}
		avgJitter = sum / time.Duration(len(am.jitterSamples))
	}

	return AdvancedMetricsSnapshot{
		AvgLatency:        avgLatency,
		MinLatency:        minLatency,
		MaxLatency:        maxLatency,
		PacketLossRate:    lossRate,
		ThroughputInMbps:  throughputInMbps,
		ThroughputOutMbps: throughputOutMbps,
		AvgRTT:            avgRTT,
		AvgJitter:         avgJitter,
		TimeoutErrors:     am.timeoutErrors.Load(),
		ConnErrors:        am.connErrors.Load(),
		OtherErrors:       am.otherErrors.Load(),
		Timestamp:         time.Now(),
	}
}

// Reset 重置指标
func (am *AdvancedMetrics) Reset() {
	am.latencySum.Store(0)
	am.latencyCount.Store(0)
	am.latencyMin.Store(int64(time.Hour))
	am.latencyMax.Store(0)
	am.packetsSent.Store(0)
	am.packetsLost.Store(0)
	am.throughputIn.Store(0)
	am.throughputOut.Store(0)
	am.timeoutErrors.Store(0)
	am.connErrors.Store(0)
	am.otherErrors.Store(0)
	am.lastMeasure.Store(time.Now())

	am.mu.Lock()
	am.rttSamples = am.rttSamples[:0]
	am.jitterSamples = am.jitterSamples[:0]
	am.mu.Unlock()
}

// AdvancedMetricsSnapshot 高级指标快照
type AdvancedMetricsSnapshot struct {
	AvgLatency        time.Duration `json:"avg_latency"`
	MinLatency        time.Duration `json:"min_latency"`
	MaxLatency        time.Duration `json:"max_latency"`
	PacketLossRate    float64       `json:"packet_loss_rate"`
	ThroughputInMbps  float64       `json:"throughput_in_mbps"`
	ThroughputOutMbps float64       `json:"throughput_out_mbps"`
	AvgRTT            time.Duration `json:"avg_rtt"`
	AvgJitter         time.Duration `json:"avg_jitter"`
	TimeoutErrors     int64         `json:"timeout_errors"`
	ConnErrors        int64         `json:"conn_errors"`
	OtherErrors       int64         `json:"other_errors"`
	Timestamp         time.Time     `json:"timestamp"`
}

// LogSnapshot 记录快照到日志
func (s *AdvancedMetricsSnapshot) LogSnapshot(tunnelID string) {
	logger.Info("高级指标快照",
		zap.String("tunnel_id", tunnelID),
		zap.Duration("avg_latency", s.AvgLatency),
		zap.Duration("min_latency", s.MinLatency),
		zap.Duration("max_latency", s.MaxLatency),
		zap.Float64("packet_loss_rate", s.PacketLossRate),
		zap.Float64("throughput_in_mbps", s.ThroughputInMbps),
		zap.Float64("throughput_out_mbps", s.ThroughputOutMbps),
		zap.Duration("avg_rtt", s.AvgRTT),
		zap.Duration("avg_jitter", s.AvgJitter),
		zap.Int64("timeout_errors", s.TimeoutErrors),
		zap.Int64("conn_errors", s.ConnErrors),
		zap.Int64("other_errors", s.OtherErrors))
}

// QualityScore 计算连接质量评分（0-100）
func (s *AdvancedMetricsSnapshot) QualityScore() float64 {
	score := 100.0

	// 延迟影响（0-25分）
	if s.AvgLatency > 0 {
		latencyMs := float64(s.AvgLatency.Milliseconds())
		if latencyMs > 200 {
			score -= 25
		} else if latencyMs > 100 {
			score -= 15
		} else if latencyMs > 50 {
			score -= 5
		}
	}

	// 丢包率影响（0-30分）
	if s.PacketLossRate > 10 {
		score -= 30
	} else if s.PacketLossRate > 5 {
		score -= 20
	} else if s.PacketLossRate > 1 {
		score -= 10
	}

	// 抖动影响（0-20分）
	if s.AvgJitter > 0 {
		jitterMs := float64(s.AvgJitter.Milliseconds())
		if jitterMs > 50 {
			score -= 20
		} else if jitterMs > 20 {
			score -= 10
		} else if jitterMs > 10 {
			score -= 5
		}
	}

	// 错误影响（0-25分）
	totalErrors := s.TimeoutErrors + s.ConnErrors + s.OtherErrors
	if totalErrors > 100 {
		score -= 25
	} else if totalErrors > 50 {
		score -= 15
	} else if totalErrors > 10 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}

	return score
}

