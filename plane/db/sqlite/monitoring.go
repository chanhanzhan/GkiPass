package sqlite

import (
	"database/sql"
	"time"

	dbinit "gkipass/plane/db/init"

	"github.com/google/uuid"
)

// === 监控配置操作 ===

// CreateNodeMonitoringConfig 创建节点监控配置
func (s *SQLiteDB) CreateNodeMonitoringConfig(config *dbinit.NodeMonitoringConfig) error {
	if config.ID == "" {
		config.ID = uuid.New().String()
	}
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	query := `
		INSERT INTO node_monitoring_config 
		(id, node_id, monitoring_enabled, report_interval, collect_system_info, 
		 collect_network_stats, collect_tunnel_stats, collect_performance, data_retention_days,
		 alert_cpu_threshold, alert_memory_threshold, alert_disk_threshold, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		config.ID, config.NodeID, config.MonitoringEnabled, config.ReportInterval,
		config.CollectSystemInfo, config.CollectNetworkStats, config.CollectTunnelStats,
		config.CollectPerformance, config.DataRetentionDays, config.AlertCPUThreshold,
		config.AlertMemoryThreshold, config.AlertDiskThreshold, config.CreatedAt, config.UpdatedAt)
	return err
}

// GetNodeMonitoringConfig 获取节点监控配置
func (s *SQLiteDB) GetNodeMonitoringConfig(nodeID string) (*dbinit.NodeMonitoringConfig, error) {
	config := &dbinit.NodeMonitoringConfig{}
	query := `
		SELECT id, node_id, monitoring_enabled, report_interval, collect_system_info, 
		       collect_network_stats, collect_tunnel_stats, collect_performance, data_retention_days,
		       alert_cpu_threshold, alert_memory_threshold, alert_disk_threshold, created_at, updated_at
		FROM node_monitoring_config WHERE node_id = ?
	`
	err := s.db.QueryRow(query, nodeID).Scan(
		&config.ID, &config.NodeID, &config.MonitoringEnabled, &config.ReportInterval,
		&config.CollectSystemInfo, &config.CollectNetworkStats, &config.CollectTunnelStats,
		&config.CollectPerformance, &config.DataRetentionDays, &config.AlertCPUThreshold,
		&config.AlertMemoryThreshold, &config.AlertDiskThreshold, &config.CreatedAt, &config.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return config, err
}

// UpdateNodeMonitoringConfig 更新节点监控配置
func (s *SQLiteDB) UpdateNodeMonitoringConfig(config *dbinit.NodeMonitoringConfig) error {
	config.UpdatedAt = time.Now()
	query := `
		UPDATE node_monitoring_config 
		SET monitoring_enabled = ?, report_interval = ?, collect_system_info = ?,
		    collect_network_stats = ?, collect_tunnel_stats = ?, collect_performance = ?,
		    data_retention_days = ?, alert_cpu_threshold = ?, alert_memory_threshold = ?,
		    alert_disk_threshold = ?, updated_at = ?
		WHERE node_id = ?
	`
	_, err := s.db.Exec(query,
		config.MonitoringEnabled, config.ReportInterval, config.CollectSystemInfo,
		config.CollectNetworkStats, config.CollectTunnelStats, config.CollectPerformance,
		config.DataRetentionDays, config.AlertCPUThreshold, config.AlertMemoryThreshold,
		config.AlertDiskThreshold, config.UpdatedAt, config.NodeID)
	return err
}

// UpsertNodeMonitoringConfig 创建或更新节点监控配置
func (s *SQLiteDB) UpsertNodeMonitoringConfig(config *dbinit.NodeMonitoringConfig) error {
	existing, err := s.GetNodeMonitoringConfig(config.NodeID)
	if err != nil {
		return err
	}

	if existing == nil {
		return s.CreateNodeMonitoringConfig(config)
	}

	config.ID = existing.ID
	config.CreatedAt = existing.CreatedAt
	return s.UpdateNodeMonitoringConfig(config)
}

// === 监控数据操作 ===

// CreateNodeMonitoringData 创建节点监控数据
func (s *SQLiteDB) CreateNodeMonitoringData(data *dbinit.NodeMonitoringData) error {
	if data.ID == "" {
		data.ID = uuid.New().String()
	}
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	query := `
		INSERT INTO node_monitoring_data 
		(id, node_id, timestamp, system_uptime, boot_time, cpu_usage, cpu_load_1m, cpu_load_5m, 
		 cpu_load_15m, cpu_cores, memory_total, memory_used, memory_available, memory_usage_percent,
		 disk_total, disk_used, disk_available, disk_usage_percent, network_interfaces,
		 bandwidth_in, bandwidth_out, tcp_connections, udp_connections, active_tunnels,
		 total_connections, traffic_in_bytes, traffic_out_bytes, packets_in, packets_out,
		 connection_errors, tunnel_errors, avg_response_time, max_response_time, min_response_time,
		 app_version, go_version, os_info, node_config_version, last_config_update)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		data.ID, data.NodeID, data.Timestamp, data.SystemUptime, data.BootTime,
		data.CPUUsage, data.CPULoad1m, data.CPULoad5m, data.CPULoad15m, data.CPUCores,
		data.MemoryTotal, data.MemoryUsed, data.MemoryAvailable, data.MemoryUsagePercent,
		data.DiskTotal, data.DiskUsed, data.DiskAvailable, data.DiskUsagePercent,
		data.NetworkInterfaces, data.BandwidthIn, data.BandwidthOut,
		data.TCPConnections, data.UDPConnections, data.ActiveTunnels, data.TotalConnections,
		data.TrafficInBytes, data.TrafficOutBytes, data.PacketsIn, data.PacketsOut,
		data.ConnectionErrors, data.TunnelErrors, data.AvgResponseTime,
		data.MaxResponseTime, data.MinResponseTime, data.AppVersion,
		data.GoVersion, data.OSInfo, data.NodeConfigVersion, data.LastConfigUpdate)
	return err
}

// GetLatestNodeMonitoringData 获取节点最新监控数据
func (s *SQLiteDB) GetLatestNodeMonitoringData(nodeID string) (*dbinit.NodeMonitoringData, error) {
	data := &dbinit.NodeMonitoringData{}
	query := `
		SELECT id, node_id, timestamp, system_uptime, boot_time, cpu_usage, cpu_load_1m, cpu_load_5m, 
		       cpu_load_15m, cpu_cores, memory_total, memory_used, memory_available, memory_usage_percent,
		       disk_total, disk_used, disk_available, disk_usage_percent, network_interfaces,
		       bandwidth_in, bandwidth_out, tcp_connections, udp_connections, active_tunnels,
		       total_connections, traffic_in_bytes, traffic_out_bytes, packets_in, packets_out,
		       connection_errors, tunnel_errors, avg_response_time, max_response_time, min_response_time,
		       app_version, go_version, os_info, node_config_version, last_config_update
		FROM node_monitoring_data 
		WHERE node_id = ? 
		ORDER BY timestamp DESC 
		LIMIT 1
	`
	err := s.db.QueryRow(query, nodeID).Scan(
		&data.ID, &data.NodeID, &data.Timestamp, &data.SystemUptime, &data.BootTime,
		&data.CPUUsage, &data.CPULoad1m, &data.CPULoad5m, &data.CPULoad15m, &data.CPUCores,
		&data.MemoryTotal, &data.MemoryUsed, &data.MemoryAvailable, &data.MemoryUsagePercent,
		&data.DiskTotal, &data.DiskUsed, &data.DiskAvailable, &data.DiskUsagePercent,
		&data.NetworkInterfaces, &data.BandwidthIn, &data.BandwidthOut,
		&data.TCPConnections, &data.UDPConnections, &data.ActiveTunnels, &data.TotalConnections,
		&data.TrafficInBytes, &data.TrafficOutBytes, &data.PacketsIn, &data.PacketsOut,
		&data.ConnectionErrors, &data.TunnelErrors, &data.AvgResponseTime,
		&data.MaxResponseTime, &data.MinResponseTime, &data.AppVersion,
		&data.GoVersion, &data.OSInfo, &data.NodeConfigVersion, &data.LastConfigUpdate,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}

// ListNodeMonitoringData 获取节点监控数据列表
func (s *SQLiteDB) ListNodeMonitoringData(nodeID string, from, to time.Time, limit int) ([]*dbinit.NodeMonitoringData, error) {
	query := `
		SELECT id, node_id, timestamp, system_uptime, boot_time, cpu_usage, cpu_load_1m, cpu_load_5m, 
		       cpu_load_15m, cpu_cores, memory_total, memory_used, memory_available, memory_usage_percent,
		       disk_total, disk_used, disk_available, disk_usage_percent, network_interfaces,
		       bandwidth_in, bandwidth_out, tcp_connections, udp_connections, active_tunnels,
		       total_connections, traffic_in_bytes, traffic_out_bytes, packets_in, packets_out,
		       connection_errors, tunnel_errors, avg_response_time, max_response_time, min_response_time,
		       app_version, go_version, os_info, node_config_version, last_config_update
		FROM node_monitoring_data 
		WHERE node_id = ? AND timestamp BETWEEN ? AND ?
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, nodeID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dataList []*dbinit.NodeMonitoringData
	for rows.Next() {
		data := &dbinit.NodeMonitoringData{}
		err := rows.Scan(
			&data.ID, &data.NodeID, &data.Timestamp, &data.SystemUptime, &data.BootTime,
			&data.CPUUsage, &data.CPULoad1m, &data.CPULoad5m, &data.CPULoad15m, &data.CPUCores,
			&data.MemoryTotal, &data.MemoryUsed, &data.MemoryAvailable, &data.MemoryUsagePercent,
			&data.DiskTotal, &data.DiskUsed, &data.DiskAvailable, &data.DiskUsagePercent,
			&data.NetworkInterfaces, &data.BandwidthIn, &data.BandwidthOut,
			&data.TCPConnections, &data.UDPConnections, &data.ActiveTunnels, &data.TotalConnections,
			&data.TrafficInBytes, &data.TrafficOutBytes, &data.PacketsIn, &data.PacketsOut,
			&data.ConnectionErrors, &data.TunnelErrors, &data.AvgResponseTime,
			&data.MaxResponseTime, &data.MinResponseTime, &data.AppVersion,
			&data.GoVersion, &data.OSInfo, &data.NodeConfigVersion, &data.LastConfigUpdate,
		)
		if err != nil {
			return nil, err
		}
		dataList = append(dataList, data)
	}

	return dataList, rows.Err()
}

// DeleteOldMonitoringData 删除过期的监控数据
func (s *SQLiteDB) DeleteOldMonitoringData(nodeID string, retentionDays int) error {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	query := `DELETE FROM node_monitoring_data WHERE node_id = ? AND timestamp < ?`
	result, err := s.db.Exec(query, nodeID, cutoffTime)
	if err != nil {
		return err
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		// Log cleanup operation
	}

	return nil
}

// === 监控权限操作 ===

// CreateMonitoringPermission 创建监控权限
func (s *SQLiteDB) CreateMonitoringPermission(perm *dbinit.MonitoringPermission) error {
	if perm.ID == "" {
		perm.ID = uuid.New().String()
	}
	perm.CreatedAt = time.Now()
	perm.UpdatedAt = time.Now()

	query := `
		INSERT INTO monitoring_permissions 
		(id, user_id, node_id, permission_type, enabled, created_by, created_at, updated_at, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		perm.ID, perm.UserID, perm.NodeID, perm.PermissionType, perm.Enabled,
		perm.CreatedBy, perm.CreatedAt, perm.UpdatedAt, perm.Description)
	return err
}

// GetMonitoringPermission 获取监控权限
func (s *SQLiteDB) GetMonitoringPermission(userID, nodeID string) (*dbinit.MonitoringPermission, error) {
	perm := &dbinit.MonitoringPermission{}
	query := `
		SELECT id, user_id, node_id, permission_type, enabled, created_by, created_at, updated_at, description
		FROM monitoring_permissions 
		WHERE user_id = ? AND (node_id = ? OR node_id IS NULL)
		ORDER BY node_id DESC -- 具体节点权限优先于全局权限
		LIMIT 1
	`
	err := s.db.QueryRow(query, userID, nodeID).Scan(
		&perm.ID, &perm.UserID, &perm.NodeID, &perm.PermissionType, &perm.Enabled,
		&perm.CreatedBy, &perm.CreatedAt, &perm.UpdatedAt, &perm.Description,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return perm, err
}

// ListMonitoringPermissions 列出监控权限
func (s *SQLiteDB) ListMonitoringPermissions(userID string) ([]*dbinit.MonitoringPermission, error) {
	var query string
	var args []interface{}

	if userID != "" {
		query = `
			SELECT id, user_id, node_id, permission_type, enabled, created_by, created_at, updated_at, description
			FROM monitoring_permissions 
			WHERE user_id = ?
			ORDER BY created_at DESC
		`
		args = []interface{}{userID}
	} else {
		query = `
			SELECT id, user_id, node_id, permission_type, enabled, created_by, created_at, updated_at, description
			FROM monitoring_permissions 
			ORDER BY created_at DESC
		`
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []*dbinit.MonitoringPermission
	for rows.Next() {
		perm := &dbinit.MonitoringPermission{}
		err := rows.Scan(
			&perm.ID, &perm.UserID, &perm.NodeID, &perm.PermissionType, &perm.Enabled,
			&perm.CreatedBy, &perm.CreatedAt, &perm.UpdatedAt, &perm.Description,
		)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, perm)
	}

	return permissions, rows.Err()
}

// === 告警规则操作 ===

// CreateNodeAlertRule 创建节点告警规则
func (s *SQLiteDB) CreateNodeAlertRule(rule *dbinit.NodeAlertRule) error {
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	query := `
		INSERT INTO node_alert_rules 
		(id, node_id, rule_name, metric_type, operator, threshold_value, duration_seconds,
		 severity, enabled, notification_channels, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		rule.ID, rule.NodeID, rule.RuleName, rule.MetricType, rule.Operator,
		rule.ThresholdValue, rule.DurationSeconds, rule.Severity, rule.Enabled,
		rule.NotificationChannels, rule.CreatedAt, rule.UpdatedAt)
	return err
}

// ListNodeAlertRules 列出节点告警规则
func (s *SQLiteDB) ListNodeAlertRules(nodeID string) ([]*dbinit.NodeAlertRule, error) {
	query := `
		SELECT id, node_id, rule_name, metric_type, operator, threshold_value, duration_seconds,
		       severity, enabled, notification_channels, created_at, updated_at
		FROM node_alert_rules 
		WHERE node_id = ? OR node_id IS NULL -- 包括全局规则
		ORDER BY severity DESC, created_at DESC
	`

	rows, err := s.db.Query(query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*dbinit.NodeAlertRule
	for rows.Next() {
		rule := &dbinit.NodeAlertRule{}
		err := rows.Scan(
			&rule.ID, &rule.NodeID, &rule.RuleName, &rule.MetricType, &rule.Operator,
			&rule.ThresholdValue, &rule.DurationSeconds, &rule.Severity, &rule.Enabled,
			&rule.NotificationChannels, &rule.CreatedAt, &rule.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

// CreateNodeAlertHistory 创建告警历史记录
func (s *SQLiteDB) CreateNodeAlertHistory(alert *dbinit.NodeAlertHistory) error {
	if alert.ID == "" {
		alert.ID = uuid.New().String()
	}
	if alert.TriggeredAt.IsZero() {
		alert.TriggeredAt = time.Now()
	}

	query := `
		INSERT INTO node_alert_history 
		(id, rule_id, node_id, alert_type, severity, message, metric_value, threshold_value,
		 status, triggered_at, acknowledged_at, resolved_at, acknowledged_by, details)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		alert.ID, alert.RuleID, alert.NodeID, alert.AlertType, alert.Severity,
		alert.Message, alert.MetricValue, alert.ThresholdValue, alert.Status,
		alert.TriggeredAt, alert.AcknowledgedAt, alert.ResolvedAt,
		alert.AcknowledgedBy, alert.Details)
	return err
}

// ListNodeAlertHistory 获取节点告警历史
func (s *SQLiteDB) ListNodeAlertHistory(nodeID string, limit int) ([]*dbinit.NodeAlertHistory, error) {
	query := `
		SELECT id, rule_id, node_id, alert_type, severity, message, metric_value, threshold_value,
		       status, triggered_at, acknowledged_at, resolved_at, acknowledged_by, details
		FROM node_alert_history 
		WHERE node_id = ?
		ORDER BY triggered_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*dbinit.NodeAlertHistory
	for rows.Next() {
		alert := &dbinit.NodeAlertHistory{}
		err := rows.Scan(
			&alert.ID, &alert.RuleID, &alert.NodeID, &alert.AlertType, &alert.Severity,
			&alert.Message, &alert.MetricValue, &alert.ThresholdValue, &alert.Status,
			&alert.TriggeredAt, &alert.AcknowledgedAt, &alert.ResolvedAt,
			&alert.AcknowledgedBy, &alert.Details,
		)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}

	return alerts, rows.Err()
}

// === 性能历史数据操作 ===

// CreateNodePerformanceHistory 创建性能历史数据
func (s *SQLiteDB) CreateNodePerformanceHistory(history *dbinit.NodePerformanceHistory) error {
	if history.ID == "" {
		history.ID = uuid.New().String()
	}
	history.CreatedAt = time.Now()

	query := `
		INSERT INTO node_performance_history 
		(id, node_id, date, aggregation_type, aggregation_time, avg_cpu_usage, avg_memory_usage,
		 avg_disk_usage, avg_bandwidth_in, avg_bandwidth_out, avg_connections, avg_response_time,
		 max_cpu_usage, max_memory_usage, max_connections, max_response_time,
		 total_traffic_in, total_traffic_out, total_packets_in, total_packets_out,
		 total_errors, uptime_seconds, downtime_seconds, availability_percent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		history.ID, history.NodeID, history.Date, history.AggregationType, history.AggregationTime,
		history.AvgCPUUsage, history.AvgMemoryUsage, history.AvgDiskUsage,
		history.AvgBandwidthIn, history.AvgBandwidthOut, history.AvgConnections, history.AvgResponseTime,
		history.MaxCPUUsage, history.MaxMemoryUsage, history.MaxConnections, history.MaxResponseTime,
		history.TotalTrafficIn, history.TotalTrafficOut, history.TotalPacketsIn, history.TotalPacketsOut,
		history.TotalErrors, history.UptimeSeconds, history.DowntimeSeconds, history.AvailabilityPercent,
		history.CreatedAt)
	return err
}

// GetNodePerformanceHistory 获取节点性能历史数据
func (s *SQLiteDB) GetNodePerformanceHistory(nodeID string, aggregationType string, from, to time.Time) ([]*dbinit.NodePerformanceHistory, error) {
	query := `
		SELECT id, node_id, date, aggregation_type, aggregation_time, avg_cpu_usage, avg_memory_usage,
		       avg_disk_usage, avg_bandwidth_in, avg_bandwidth_out, avg_connections, avg_response_time,
		       max_cpu_usage, max_memory_usage, max_connections, max_response_time,
		       total_traffic_in, total_traffic_out, total_packets_in, total_packets_out,
		       total_errors, uptime_seconds, downtime_seconds, availability_percent, created_at
		FROM node_performance_history 
		WHERE node_id = ? AND aggregation_type = ? AND date BETWEEN ? AND ?
		ORDER BY aggregation_time DESC
	`

	rows, err := s.db.Query(query, nodeID, aggregationType, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var historyList []*dbinit.NodePerformanceHistory
	for rows.Next() {
		history := &dbinit.NodePerformanceHistory{}
		err := rows.Scan(
			&history.ID, &history.NodeID, &history.Date, &history.AggregationType, &history.AggregationTime,
			&history.AvgCPUUsage, &history.AvgMemoryUsage, &history.AvgDiskUsage,
			&history.AvgBandwidthIn, &history.AvgBandwidthOut, &history.AvgConnections, &history.AvgResponseTime,
			&history.MaxCPUUsage, &history.MaxMemoryUsage, &history.MaxConnections, &history.MaxResponseTime,
			&history.TotalTrafficIn, &history.TotalTrafficOut, &history.TotalPacketsIn, &history.TotalPacketsOut,
			&history.TotalErrors, &history.UptimeSeconds, &history.DowntimeSeconds, &history.AvailabilityPercent,
			&history.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		historyList = append(historyList, history)
	}

	return historyList, rows.Err()
}


