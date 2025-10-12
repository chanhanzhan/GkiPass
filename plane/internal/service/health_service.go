package service

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"gkipass/plane/db"
	"gkipass/plane/internal/protocol"
)

// NodeHealthStatus 节点健康状态
type NodeHealthStatus string

const (
	NodeHealthStatusHealthy   NodeHealthStatus = "healthy"   // 健康
	NodeHealthStatusDegraded  NodeHealthStatus = "degraded"  // 降级
	NodeHealthStatusUnhealthy NodeHealthStatus = "unhealthy" // 不健康
	NodeHealthStatusUnknown   NodeHealthStatus = "unknown"   // 未知
)

// NodeHealthReport 节点健康报告
type NodeHealthReport struct {
	NodeID       string                         `json:"node_id"`       // 节点ID
	Status       NodeHealthStatus               `json:"status"`        // 状态
	Components   map[string]NodeComponentHealth `json:"components"`    // 组件健康状态
	LastReported time.Time                      `json:"last_reported"` // 最后上报时间
	StartupTime  time.Time                      `json:"startup_time"`  // 启动时间
	Uptime       float64                        `json:"uptime"`        // 运行时长（秒）
	Details      map[string]interface{}         `json:"details,omitempty"`
}

// NodeComponentHealth 节点组件健康状态
type NodeComponentHealth struct {
	Name        string                 `json:"name"`
	Status      NodeHealthStatus       `json:"status"`
	Description string                 `json:"description,omitempty"`
	LastChecked time.Time              `json:"last_checked"`
	Latency     float64                `json:"latency,omitempty"` // 毫秒
	Details     map[string]interface{} `json:"details,omitempty"`
}

// HealthService 健康服务
type HealthService struct {
	db           *db.Manager
	nodeHealth   map[string]*NodeHealthReport // 节点ID -> 健康报告
	alertHistory map[string][]AlertEvent      // 节点ID -> 告警事件列表
	mu           sync.RWMutex
	logger       *zap.Logger

	// 配置选项
	alertThresholds map[string]float64 // 指标 -> 告警阈值
	checkInterval   time.Duration      // 检查间隔
}

// AlertEvent 告警事件
type AlertEvent struct {
	NodeID     string    `json:"node_id"`     // 节点ID
	Component  string    `json:"component"`   // 组件
	Status     string    `json:"status"`      // 状态
	Message    string    `json:"message"`     // 消息
	Timestamp  time.Time `json:"timestamp"`   // 时间戳
	Cleared    bool      `json:"cleared"`     // 是否已清除
	ClearedAt  time.Time `json:"cleared_at"`  // 清除时间
	AlertLevel string    `json:"alert_level"` // 告警级别
}

// NewHealthService 创建健康服务
func NewHealthService(dbManager *db.Manager) *HealthService {
	return &HealthService{
		db:           dbManager,
		nodeHealth:   make(map[string]*NodeHealthReport),
		alertHistory: make(map[string][]AlertEvent),
		logger:       zap.L().Named("health-service"),
		alertThresholds: map[string]float64{
			"cpu_usage":    80.0,  // CPU使用率阈值
			"memory_usage": 80.0,  // 内存使用率阈值
			"disk_usage":   90.0,  // 磁盘使用率阈值
			"error_rate":   5.0,   // 错误率阈值
			"latency":      200.0, // 延迟阈值（毫秒）
		},
		checkInterval: 1 * time.Minute, // 每分钟检查一次
	}
}

// Start 启动健康服务
func (s *HealthService) Start() {
	// 定期检查节点健康
	go func() {
		ticker := time.NewTicker(s.checkInterval)
		defer ticker.Stop()

		for range ticker.C {
			s.checkNodesHealth()
		}
	}()

	s.logger.Info("健康监控服务已启动")
}

// Stop 停止健康服务
func (s *HealthService) Stop() {
	s.logger.Info("健康监控服务已停止")
}

// checkNodesHealth 检查所有节点的健康状态
func (s *HealthService) checkNodesHealth() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()

	for nodeID, report := range s.nodeHealth {
		// 检查健康报告是否过期（超过5分钟未更新）
		if now.Sub(report.LastReported) > 5*time.Minute {
			// 更新状态为未知
			s.updateNodeHealthStatus(nodeID, NodeHealthStatusUnknown)

			// 生成告警
			s.createAlert(nodeID, "", "node_offline", "节点离线", "critical")
		}
	}
}

// HandleHealthReport 处理节点健康报告
func (s *HealthService) HandleHealthReport(nodeID string, data []byte) error {
	var report NodeHealthReport
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}

	// 设置节点ID和上报时间
	report.NodeID = nodeID
	report.LastReported = time.Now()

	// 保存健康报告
	s.mu.Lock()
	s.nodeHealth[nodeID] = &report
	s.mu.Unlock()

	// 处理健康状态变化
	s.handleStatusChange(nodeID, &report)

	return nil
}

// handleStatusChange 处理状态变化
func (s *HealthService) handleStatusChange(nodeID string, report *NodeHealthReport) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查组件状态
	for name, component := range report.Components {
		if component.Status == NodeHealthStatusUnhealthy || component.Status == NodeHealthStatusDegraded {
			// 生成告警
			alertLevel := "warning"
			if component.Status == NodeHealthStatusUnhealthy {
				alertLevel = "critical"
			}

			s.createAlert(nodeID, name, string(component.Status), component.Description, alertLevel)

			// 检查具体指标是否超过阈值
			s.checkComponentThresholds(nodeID, name, component)
		} else if component.Status == NodeHealthStatusHealthy {
			// 清除该组件的告警
			s.clearAlerts(nodeID, name)
		}
	}
}

// checkComponentThresholds 检查组件指标阈值
func (s *HealthService) checkComponentThresholds(nodeID, component string, health NodeComponentHealth) {
	// 检查各项指标是否超过阈值
	for metric, value := range health.Details {
		if threshold, exists := s.alertThresholds[metric]; exists {
			// 尝试将值转换为浮点数
			var floatValue float64
			switch v := value.(type) {
			case float64:
				floatValue = v
			case float32:
				floatValue = float64(v)
			case int:
				floatValue = float64(v)
			case int64:
				floatValue = float64(v)
			default:
				continue // 无法比较的类型
			}

			// 判断是否超过阈值
			if floatValue > threshold {
				message := fmt.Sprintf("%s超过阈值 %.2f > %.2f", metric, floatValue, threshold)
				s.createAlert(nodeID, component, metric+"_threshold_exceeded", message, "warning")
			}
		}
	}
}

// createAlert 创建告警
func (s *HealthService) createAlert(nodeID, component, status, message, level string) {
	// 检查是否已有相同的未清除告警
	alerts := s.alertHistory[nodeID]
	for _, alert := range alerts {
		if !alert.Cleared && alert.Component == component && alert.Status == status {
			// 已有相同告警，不重复创建
			return
		}
	}

	// 创建新告警
	alert := AlertEvent{
		NodeID:     nodeID,
		Component:  component,
		Status:     status,
		Message:    message,
		Timestamp:  time.Now(),
		Cleared:    false,
		AlertLevel: level,
	}

	// 添加到历史记录
	s.alertHistory[nodeID] = append(s.alertHistory[nodeID], alert)

	// 记录告警日志
	s.logger.Warn("节点告警",
		zap.String("node_id", nodeID),
		zap.String("component", component),
		zap.String("status", status),
		zap.String("message", message),
		zap.String("level", level))

	// 存储到数据库
	s.saveAlertToDB(alert)

	// 发送告警通知
	s.sendAlertNotification(alert)
}

// clearAlerts 清除告警
func (s *HealthService) clearAlerts(nodeID, component string) {
	alerts := s.alertHistory[nodeID]
	now := time.Now()

	for i := range alerts {
		if !alerts[i].Cleared && (component == "" || alerts[i].Component == component) {
			// 清除告警
			alerts[i].Cleared = true
			alerts[i].ClearedAt = now

			// 记录日志
			s.logger.Info("告警已清除",
				zap.String("node_id", nodeID),
				zap.String("component", alerts[i].Component),
				zap.String("status", alerts[i].Status))

			// 更新数据库
			s.updateAlertInDB(alerts[i])
		}
	}

	s.alertHistory[nodeID] = alerts
}

// updateNodeHealthStatus 更新节点健康状态
func (s *HealthService) updateNodeHealthStatus(nodeID string, status NodeHealthStatus) {
	report, exists := s.nodeHealth[nodeID]
	if !exists {
		// 节点不存在
		return
	}

	// 更新状态
	report.Status = status

	// 记录日志
	s.logger.Info("节点健康状态更新",
		zap.String("node_id", nodeID),
		zap.String("status", string(status)))
}

// GetNodeHealthReport 获取节点健康报告
func (s *HealthService) GetNodeHealthReport(nodeID string) *NodeHealthReport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report, exists := s.nodeHealth[nodeID]
	if !exists {
		return nil
	}

	// 返回副本，避免外部修改
	reportCopy := *report
	return &reportCopy
}

// GetAllNodeHealthReports 获取所有节点的健康报告
func (s *HealthService) GetAllNodeHealthReports() map[string]*NodeHealthReport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 创建副本
	reports := make(map[string]*NodeHealthReport, len(s.nodeHealth))
	for nodeID, report := range s.nodeHealth {
		reportCopy := *report
		reports[nodeID] = &reportCopy
	}

	return reports
}

// GetNodeAlerts 获取节点告警
func (s *HealthService) GetNodeAlerts(nodeID string, includeCleared bool) []AlertEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	alerts := s.alertHistory[nodeID]
	if !includeCleared {
		// 过滤已清除的告警
		activeAlerts := make([]AlertEvent, 0, len(alerts))
		for _, alert := range alerts {
			if !alert.Cleared {
				activeAlerts = append(activeAlerts, alert)
			}
		}
		return activeAlerts
	}

	// 返回所有告警
	return append([]AlertEvent{}, alerts...)
}

// GetAllAlerts 获取所有告警
func (s *HealthService) GetAllAlerts(includeCleared bool) []AlertEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var allAlerts []AlertEvent

	for _, alerts := range s.alertHistory {
		for _, alert := range alerts {
			if includeCleared || !alert.Cleared {
				allAlerts = append(allAlerts, alert)
			}
		}
	}

	return allAlerts
}

// saveAlertToDB 保存告警到数据库
func (s *HealthService) saveAlertToDB(alert AlertEvent) {
	// TODO: 实现告警存储
}

// updateAlertInDB 更新数据库中的告警
func (s *HealthService) updateAlertInDB(alert AlertEvent) {
	// TODO: 实现告警更新
}

// sendAlertNotification 发送告警通知
func (s *HealthService) sendAlertNotification(alert AlertEvent) {
	// 构造通知消息
	_ = protocol.Notification{
		Type:      "alert",
		Title:     fmt.Sprintf("节点告警: %s", alert.NodeID),
		Message:   alert.Message,
		Level:     alert.AlertLevel,
		Timestamp: time.Now().UnixMilli(),
		Details: map[string]interface{}{
			"node_id":   alert.NodeID,
			"component": alert.Component,
			"status":    alert.Status,
			"timestamp": alert.Timestamp.Format(time.RFC3339),
			"alert_id":  alert.NodeID + "-" + alert.Component + "-" + alert.Timestamp.Format("20060102150405"),
		},
	}

	// TODO: 发送WebSocket通知
}

// GetNodeStatus 获取节点状态
func (s *HealthService) GetNodeStatus(nodeID string) (NodeHealthStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report, exists := s.nodeHealth[nodeID]
	if !exists {
		return NodeHealthStatusUnknown, fmt.Errorf("未找到节点: %s", nodeID)
	}

	return report.Status, nil
}

// GetClusterHealth 获取集群健康状态
func (s *HealthService) GetClusterHealth() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalNodes := len(s.nodeHealth)
	healthyNodes := 0
	degradedNodes := 0
	unhealthyNodes := 0
	unknownNodes := 0

	for _, report := range s.nodeHealth {
		switch report.Status {
		case NodeHealthStatusHealthy:
			healthyNodes++
		case NodeHealthStatusDegraded:
			degradedNodes++
		case NodeHealthStatusUnhealthy:
			unhealthyNodes++
		case NodeHealthStatusUnknown:
			unknownNodes++
		}
	}

	return map[string]interface{}{
		"total_nodes":     totalNodes,
		"healthy_nodes":   healthyNodes,
		"degraded_nodes":  degradedNodes,
		"unhealthy_nodes": unhealthyNodes,
		"unknown_nodes":   unknownNodes,
		"health_percent":  calculateHealthPercent(totalNodes, healthyNodes, degradedNodes),
	}
}

// calculateHealthPercent 计算健康百分比
func calculateHealthPercent(total, healthy, degraded int) float64 {
	if total == 0 {
		return 100.0
	}

	// 健康节点算满分，降级节点算半分
	return float64(healthy*100+degraded*50) / float64(total)
}
