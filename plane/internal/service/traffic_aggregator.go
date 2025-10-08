package service

import (
	"sync"
	"time"
)

// TrafficAggregator 流量统计聚合器
type TrafficAggregator struct {
	hourlyStats  map[string]*TrafficStats
	dailyStats   map[string]*TrafficStats
	monthlyStats map[string]*TrafficStats
	mu           sync.RWMutex
}

// TrafficStats 流量统计
type TrafficStats struct {
	TunnelID    string
	BytesIn     int64
	BytesOut    int64
	Connections int64
	Timestamp   time.Time
}

// NewTrafficAggregator 创建流量聚合器
func NewTrafficAggregator() *TrafficAggregator {
	return &TrafficAggregator{
		hourlyStats:  make(map[string]*TrafficStats),
		dailyStats:   make(map[string]*TrafficStats),
		monthlyStats: make(map[string]*TrafficStats),
	}
}

// Record 记录流量
func (ta *TrafficAggregator) Record(tunnelID string, bytesIn, bytesOut int64) {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	now := time.Now()

	// 小时统计
	hourKey := tunnelID + ":" + now.Format("2006010215")
	if stats, exists := ta.hourlyStats[hourKey]; exists {
		stats.BytesIn += bytesIn
		stats.BytesOut += bytesOut
		stats.Connections++
	} else {
		ta.hourlyStats[hourKey] = &TrafficStats{
			TunnelID:    tunnelID,
			BytesIn:     bytesIn,
			BytesOut:    bytesOut,
			Connections: 1,
			Timestamp:   now,
		}
	}

	// 日统计
	dayKey := tunnelID + ":" + now.Format("20060102")
	if stats, exists := ta.dailyStats[dayKey]; exists {
		stats.BytesIn += bytesIn
		stats.BytesOut += bytesOut
		stats.Connections++
	} else {
		ta.dailyStats[dayKey] = &TrafficStats{
			TunnelID:    tunnelID,
			BytesIn:     bytesIn,
			BytesOut:    bytesOut,
			Connections: 1,
			Timestamp:   now,
		}
	}

	// 月统计
	monthKey := tunnelID + ":" + now.Format("200601")
	if stats, exists := ta.monthlyStats[monthKey]; exists {
		stats.BytesIn += bytesIn
		stats.BytesOut += bytesOut
		stats.Connections++
	} else {
		ta.monthlyStats[monthKey] = &TrafficStats{
			TunnelID:    tunnelID,
			BytesIn:     bytesIn,
			BytesOut:    bytesOut,
			Connections: 1,
			Timestamp:   now,
		}
	}
}

// GetHourlyStats 获取小时统计
func (ta *TrafficAggregator) GetHourlyStats(tunnelID string, hour time.Time) *TrafficStats {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	key := tunnelID + ":" + hour.Format("2006010215")
	return ta.hourlyStats[key]
}

// GetDailyStats 获取日统计
func (ta *TrafficAggregator) GetDailyStats(tunnelID string, day time.Time) *TrafficStats {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	key := tunnelID + ":" + day.Format("20060102")
	return ta.dailyStats[key]
}





