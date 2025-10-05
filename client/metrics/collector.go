package metrics

import (
	"runtime"
	"time"

	"gkipass/client/core"
	"gkipass/client/logger"
	"gkipass/client/ws"

	"go.uber.org/zap"
)

// Collector 指标收集器
type Collector struct {
	nodeID        string
	wsClient      *ws.Client
	tunnelManager *core.TunnelManager
	monitor       *SystemMonitor
	reportTicker  *time.Ticker
	stopChan      chan struct{}
}

// NewCollector 创建收集器
func NewCollector(nodeID string, wsClient *ws.Client, tunnelManager *core.TunnelManager) *Collector {
	monitor := NewSystemMonitor()
	monitor.Start()

	return &Collector{
		nodeID:        nodeID,
		wsClient:      wsClient,
		tunnelManager: tunnelManager,
		monitor:       monitor,
		stopChan:      make(chan struct{}),
	}
}

// Start 启动收集器
func (c *Collector) Start() {
	// 流量上报：60秒
	c.reportTicker = time.NewTicker(60 * time.Second)
	go c.reportLoop()

	logger.Info("指标收集器已启动")
}

// Stop 停止收集器
func (c *Collector) Stop() {
	close(c.stopChan)
	if c.reportTicker != nil {
		c.reportTicker.Stop()
	}
	logger.Info("指标收集器已停止")
}

// reportLoop 流量上报循环
func (c *Collector) reportLoop() {
	for {
		select {
		case <-c.reportTicker.C:
			c.reportTraffic()
		case <-c.stopChan:
			return
		}
	}
}

// reportTraffic 上报流量
func (c *Collector) reportTraffic() {
	if !c.wsClient.IsConnected() {
		return
	}

	allStats := c.tunnelManager.GetAllStats()

	reportCount := 0
	for tunnelID, stats := range allStats {
		trafficIn := stats.TrafficIn.Load()
		trafficOut := stats.TrafficOut.Load()
		connections := int(stats.Connections.Load())

		// 跳过无流量的隧道
		if trafficIn == 0 && trafficOut == 0 {
			continue
		}

		req := ws.TrafficReportRequest{
			NodeID:      c.nodeID,
			TunnelID:    tunnelID,
			TrafficIn:   trafficIn,
			TrafficOut:  trafficOut,
			Connections: connections,
		}

		msg, err := ws.NewMessage(ws.MsgTypeTrafficReport, req)
		if err != nil {
			logger.Error("创建流量上报消息失败",
				zap.String("tunnel_id", tunnelID),
				zap.Error(err))
			continue
		}

		if err := c.wsClient.Send(msg); err != nil {
			logger.Error("上报流量失败",
				zap.String("tunnel_id", tunnelID),
				zap.Error(err))
			continue
		}

		reportCount++

		// 重置统计（或者不重置，看业务需求）
		// stats.TrafficIn.Store(0)
		// stats.TrafficOut.Store(0)
	}

	if reportCount > 0 {
		logger.Debug("流量上报完成", zap.Int("count", reportCount))
	}
}

// GetCPUUsage 获取CPU使用率（通过监控器）
func (c *Collector) GetCPUUsage() float64 {
	if c.monitor != nil {
		return c.monitor.GetCPUUsage()
	}
	return 0.0
}

// GetMemoryUsage 获取内存使用（通过监控器）
func (c *Collector) GetMemoryUsage() int64 {
	if c.monitor != nil {
		return c.monitor.GetMemoryUsage()
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.Alloc)
}

// GetMonitorSnapshot 获取完整监控快照
func (c *Collector) GetMonitorSnapshot() MonitorSnapshot {
	if c.monitor != nil {
		snapshot := c.monitor.GetSnapshot()
		snapshot.Connections = c.tunnelManager.GetConnectionCount()
		return snapshot
	}
	return MonitorSnapshot{}
}
