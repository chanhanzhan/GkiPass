package heartbeat

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// StatisticsWindow 统计窗口
type StatisticsWindow struct {
	measurements []RTTMeasurement
	maxSize      int
	mutex        sync.RWMutex
	logger       *zap.Logger
}

// NewStatisticsWindow 创建统计窗口
func NewStatisticsWindow(maxSize int) *StatisticsWindow {
	return &StatisticsWindow{
		measurements: make([]RTTMeasurement, 0, maxSize),
		maxSize:      maxSize,
		logger:       zap.L().Named("stats-window"),
	}
}

// AddMeasurement 添加测量值
func (sw *StatisticsWindow) AddMeasurement(measurement RTTMeasurement) {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()

	// 如果窗口已满，移除最旧的测量值
	if len(sw.measurements) >= sw.maxSize {
		sw.measurements = sw.measurements[1:]
	}

	sw.measurements = append(sw.measurements, measurement)

	sw.logger.Debug("添加RTT测量值",
		zap.String("ping_id", measurement.PingID),
		zap.Duration("rtt", measurement.RTT),
		zap.Bool("success", measurement.Success),
		zap.Int("window_size", len(sw.measurements)))
}

// GetRTTStatistics 获取RTT统计信息
func (sw *StatisticsWindow) GetRTTStatistics() RTTStatistics {
	sw.mutex.RLock()
	defer sw.mutex.RUnlock()

	if len(sw.measurements) == 0 {
		return RTTStatistics{}
	}

	// 收集成功的RTT测量值
	var rtts []time.Duration
	for _, m := range sw.measurements {
		if m.Success {
			rtts = append(rtts, m.RTT)
		}
	}

	if len(rtts) == 0 {
		return RTTStatistics{}
	}

	// 排序以计算中位数
	sort.Slice(rtts, func(i, j int) bool {
		return rtts[i] < rtts[j]
	})

	// 计算统计值
	min := rtts[0]
	max := rtts[len(rtts)-1]

	// 计算平均值
	var sum time.Duration
	for _, rtt := range rtts {
		sum += rtt
	}
	average := sum / time.Duration(len(rtts))

	// 计算中位数
	var median time.Duration
	if len(rtts)%2 == 0 {
		median = (rtts[len(rtts)/2-1] + rtts[len(rtts)/2]) / 2
	} else {
		median = rtts[len(rtts)/2]
	}

	// 计算标准差
	var variance float64
	for _, rtt := range rtts {
		diff := float64(rtt - average)
		variance += diff * diff
	}
	variance /= float64(len(rtts))
	stdDev := time.Duration(math.Sqrt(variance))

	return RTTStatistics{
		Min:     min,
		Max:     max,
		Average: average,
		Median:  median,
		StdDev:  stdDev,
		Count:   len(rtts),
	}
}

// GetJitterStatistics 获取抖动统计信息
func (sw *StatisticsWindow) GetJitterStatistics() JitterStatistics {
	sw.mutex.RLock()
	defer sw.mutex.RUnlock()

	if len(sw.measurements) < 2 {
		return JitterStatistics{}
	}

	// 计算相邻RTT的差值（抖动）
	var jitters []time.Duration
	var lastRTT time.Duration
	var hasLastRTT bool

	for _, m := range sw.measurements {
		if m.Success {
			if hasLastRTT {
				jitter := m.RTT - lastRTT
				if jitter < 0 {
					jitter = -jitter
				}
				jitters = append(jitters, jitter)
			}
			lastRTT = m.RTT
			hasLastRTT = true
		}
	}

	if len(jitters) == 0 {
		return JitterStatistics{}
	}

	// 排序
	sort.Slice(jitters, func(i, j int) bool {
		return jitters[i] < jitters[j]
	})

	// 计算统计值
	min := jitters[0]
	max := jitters[len(jitters)-1]

	// 计算平均值
	var sum time.Duration
	for _, jitter := range jitters {
		sum += jitter
	}
	average := sum / time.Duration(len(jitters))

	// 计算标准差
	var variance float64
	for _, jitter := range jitters {
		diff := float64(jitter - average)
		variance += diff * diff
	}
	variance /= float64(len(jitters))
	stdDev := time.Duration(math.Sqrt(variance))

	return JitterStatistics{
		Min:     min,
		Max:     max,
		Average: average,
		StdDev:  stdDev,
		Count:   len(jitters),
	}
}

// GetPacketLossStatistics 获取丢包统计信息
func (sw *StatisticsWindow) GetPacketLossStatistics() PacketLossStatistics {
	sw.mutex.RLock()
	defer sw.mutex.RUnlock()

	if len(sw.measurements) == 0 {
		return PacketLossStatistics{}
	}

	totalSent := len(sw.measurements)
	totalReceived := 0
	maxBurstLoss := 0
	currentBurstLoss := 0

	for _, m := range sw.measurements {
		if m.Success {
			totalReceived++
			// 重置连续丢包计数
			if currentBurstLoss > maxBurstLoss {
				maxBurstLoss = currentBurstLoss
			}
			currentBurstLoss = 0
		} else {
			currentBurstLoss++
		}
	}

	// 检查最后的连续丢包
	if currentBurstLoss > maxBurstLoss {
		maxBurstLoss = currentBurstLoss
	}

	totalLost := totalSent - totalReceived
	lossRate := 0.0
	if totalSent > 0 {
		lossRate = float64(totalLost) / float64(totalSent) * 100
	}

	return PacketLossStatistics{
		TotalSent:     totalSent,
		TotalReceived: totalReceived,
		TotalLost:     totalLost,
		LossRate:      lossRate,
		BurstLoss:     maxBurstLoss,
	}
}

// Clear 清空统计窗口
func (sw *StatisticsWindow) Clear() {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()

	sw.measurements = sw.measurements[:0]
	sw.logger.Debug("清空统计窗口")
}

// GetSize 获取窗口大小
func (sw *StatisticsWindow) GetSize() int {
	sw.mutex.RLock()
	defer sw.mutex.RUnlock()

	return len(sw.measurements)
}

// QualityCalculator 网络质量计算器
type QualityCalculator struct {
	logger *zap.Logger
}

// NewQualityCalculator 创建质量计算器
func NewQualityCalculator() *QualityCalculator {
	return &QualityCalculator{
		logger: zap.L().Named("quality-calculator"),
	}
}

// CalculateNetworkQuality 计算网络质量
func (qc *QualityCalculator) CalculateNetworkQuality(
	rttStats RTTStatistics,
	lossStats PacketLossStatistics,
	jitterStats JitterStatistics,
	samplePeriod time.Duration,
) *NetworkQuality {

	// 如果没有足够的数据，返回默认质量
	if rttStats.Count == 0 {
		return &NetworkQuality{
			Score:        50.0,
			Grade:        QualityGradeFair,
			RTTStats:     rttStats,
			LossStats:    lossStats,
			JitterStats:  jitterStats,
			Timestamp:    time.Now(),
			SamplePeriod: samplePeriod,
		}
	}

	// 计算RTT分数 (0-40分)
	rttScore := qc.calculateRTTScore(rttStats)

	// 计算丢包分数 (0-35分)
	lossScore := qc.calculateLossScore(lossStats)

	// 计算抖动分数 (0-25分)
	jitterScore := qc.calculateJitterScore(jitterStats)

	// 总分
	totalScore := rttScore + lossScore + jitterScore

	// 确保分数在0-100范围内
	if totalScore > 100 {
		totalScore = 100
	}
	if totalScore < 0 {
		totalScore = 0
	}

	quality := &NetworkQuality{
		Score:        totalScore,
		Grade:        GetGradeFromScore(totalScore),
		RTTStats:     rttStats,
		LossStats:    lossStats,
		JitterStats:  jitterStats,
		Timestamp:    time.Now(),
		SamplePeriod: samplePeriod,
	}

	qc.logger.Debug("计算网络质量",
		zap.Float64("total_score", totalScore),
		zap.Float64("rtt_score", rttScore),
		zap.Float64("loss_score", lossScore),
		zap.Float64("jitter_score", jitterScore),
		zap.String("grade", quality.Grade.String()))

	return quality
}

// calculateRTTScore 计算RTT分数
func (qc *QualityCalculator) calculateRTTScore(stats RTTStatistics) float64 {
	if stats.Count == 0 {
		return 0
	}

	avgRTTMs := float64(stats.Average.Nanoseconds()) / 1e6 // 转换为毫秒

	// RTT评分标准（毫秒）
	// < 20ms: 满分40
	// 20-50ms: 35-40
	// 50-100ms: 25-35
	// 100-200ms: 15-25
	// 200-500ms: 5-15
	// > 500ms: 0-5

	var score float64
	switch {
	case avgRTTMs < 20:
		score = 40
	case avgRTTMs < 50:
		score = 35 + (50-avgRTTMs)/(50-20)*5
	case avgRTTMs < 100:
		score = 25 + (100-avgRTTMs)/(100-50)*10
	case avgRTTMs < 200:
		score = 15 + (200-avgRTTMs)/(200-100)*10
	case avgRTTMs < 500:
		score = 5 + (500-avgRTTMs)/(500-200)*10
	default:
		score = math.Max(0, 5-avgRTTMs/1000)
	}

	return math.Max(0, math.Min(40, score))
}

// calculateLossScore 计算丢包分数
func (qc *QualityCalculator) calculateLossScore(stats PacketLossStatistics) float64 {
	if stats.TotalSent == 0 {
		return 35 // 没有数据时给予中等分数
	}

	lossRate := stats.LossRate

	// 丢包率评分标准
	// 0%: 满分35
	// 0-1%: 30-35
	// 1-3%: 20-30
	// 3-5%: 10-20
	// 5-10%: 5-10
	// > 10%: 0-5

	var score float64
	switch {
	case lossRate == 0:
		score = 35
	case lossRate < 1:
		score = 30 + (1-lossRate)/1*5
	case lossRate < 3:
		score = 20 + (3-lossRate)/(3-1)*10
	case lossRate < 5:
		score = 10 + (5-lossRate)/(5-3)*10
	case lossRate < 10:
		score = 5 + (10-lossRate)/(10-5)*5
	default:
		score = math.Max(0, 5-lossRate/10)
	}

	// 考虑突发丢包的影响
	if stats.BurstLoss > 3 {
		score *= 0.8 // 突发丢包降低20%分数
	}

	return math.Max(0, math.Min(35, score))
}

// calculateJitterScore 计算抖动分数
func (qc *QualityCalculator) calculateJitterScore(stats JitterStatistics) float64 {
	if stats.Count == 0 {
		return 25 // 没有数据时给予满分
	}

	avgJitterMs := float64(stats.Average.Nanoseconds()) / 1e6 // 转换为毫秒

	// 抖动评分标准（毫秒）
	// < 5ms: 满分25
	// 5-10ms: 20-25
	// 10-20ms: 15-20
	// 20-50ms: 10-15
	// 50-100ms: 5-10
	// > 100ms: 0-5

	var score float64
	switch {
	case avgJitterMs < 5:
		score = 25
	case avgJitterMs < 10:
		score = 20 + (10-avgJitterMs)/(10-5)*5
	case avgJitterMs < 20:
		score = 15 + (20-avgJitterMs)/(20-10)*5
	case avgJitterMs < 50:
		score = 10 + (50-avgJitterMs)/(50-20)*5
	case avgJitterMs < 100:
		score = 5 + (100-avgJitterMs)/(100-50)*5
	default:
		score = math.Max(0, 5-avgJitterMs/100)
	}

	return math.Max(0, math.Min(25, score))
}

// HealthEvaluator 健康评估器
type HealthEvaluator struct {
	logger *zap.Logger
}

// NewHealthEvaluator 创建健康评估器
func NewHealthEvaluator() *HealthEvaluator {
	return &HealthEvaluator{
		logger: zap.L().Named("health-evaluator"),
	}
}

// EvaluateHealth 评估连接健康状态
func (he *HealthEvaluator) EvaluateHealth(
	quality *NetworkQuality,
	lastHeartbeat time.Time,
	consecutiveFails int,
	heartbeatInterval time.Duration,
) *HealthStatus {

	status := NewHealthStatus()
	status.NetworkQuality = quality
	status.LastHeartbeat = lastHeartbeat
	status.ConsecutiveFails = consecutiveFails

	// 基础健康分数从网络质量开始
	healthScore := quality.Score

	// 检查心跳超时
	timeSinceLastHeartbeat := time.Since(lastHeartbeat)
	if timeSinceLastHeartbeat > heartbeatInterval*3 {
		healthScore *= 0.5 // 心跳超时严重影响健康分数
		status.AddIssue("心跳超时")
		status.AddRecommendation("检查网络连接")
	}

	// 检查连续失败次数
	if consecutiveFails > 0 {
		failPenalty := math.Min(0.8, float64(consecutiveFails)*0.1)
		healthScore *= (1.0 - failPenalty)
		status.AddIssue(fmt.Sprintf("连续失败 %d 次", consecutiveFails))

		if consecutiveFails >= 5 {
			status.AddRecommendation("考虑重新连接")
		}
	}

	// 根据网络质量添加具体问题和建议
	if quality.LossStats.LossRate > 5 {
		status.AddIssue(fmt.Sprintf("丢包率过高: %.2f%%", quality.LossStats.LossRate))
		status.AddRecommendation("检查网络稳定性")
	}

	if quality.RTTStats.Average > 200*time.Millisecond {
		status.AddIssue(fmt.Sprintf("延迟过高: %v", quality.RTTStats.Average))
		status.AddRecommendation("优化网络路径")
	}

	if quality.JitterStats.Average > 50*time.Millisecond {
		status.AddIssue(fmt.Sprintf("抖动过大: %v", quality.JitterStats.Average))
		status.AddRecommendation("检查网络质量")
	}

	status.UpdateHealthScore(healthScore)

	he.logger.Debug("评估连接健康状态",
		zap.Float64("health_score", healthScore),
		zap.Bool("is_healthy", status.IsHealthy),
		zap.Int("consecutive_fails", consecutiveFails),
		zap.Duration("time_since_heartbeat", timeSinceLastHeartbeat),
		zap.Strings("issues", status.Issues))

	return status
}
