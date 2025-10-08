package ws

import (
	"encoding/json"
	"time"
)

// MessageType 消息类型
type MessageType string

const (
	// 节点 -> 服务器
	MsgTypeNodeRegister     MessageType = "node_register"
	MsgTypeHeartbeat        MessageType = "heartbeat"
	MsgTypeTrafficReport    MessageType = "traffic_report"
	MsgTypeNodeStatus       MessageType = "node_status"
	MsgTypeMonitoringReport MessageType = "monitoring_report" // 监控数据上报

	// 服务器 -> 节点
	MsgTypeRegisterAck   MessageType = "register_ack"
	MsgTypeSyncRules     MessageType = "sync_rules"
	MsgTypeDeleteRule    MessageType = "delete_rule"
	MsgTypeReload        MessageType = "reload"
	MsgTypePing          MessageType = "ping"
	MsgTypeConfigUpdate  MessageType = "config_update"  // 配置更新
	MsgTypeMonitorConfig MessageType = "monitor_config" // 监控配置

	// 双向
	MsgTypePong  MessageType = "pong"
	MsgTypeError MessageType = "error"
)

// Message WebSocket消息结构
type Message struct {
	Type      MessageType     `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// NodeRegisterRequest 节点注册请求
type NodeRegisterRequest struct {
	NodeID       string          `json:"node_id"`
	NodeName     string          `json:"node_name"`
	NodeType     string          `json:"node_type"` // entry/exit
	GroupID      string          `json:"group_id"`
	Version      string          `json:"version"`
	IP           string          `json:"ip"`
	Port         int             `json:"port"`
	CK           string          `json:"ck"`
	Capabilities map[string]bool `json:"capabilities"`
}

// NodeRegisterResponse 节点注册响应
type NodeRegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	NodeID  string `json:"node_id"`
	Token   string `json:"token,omitempty"`
}

// HeartbeatRequest 心跳请求
type HeartbeatRequest struct {
	NodeID      string  `json:"node_id"`
	Status      string  `json:"status"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage int64   `json:"memory_usage"`
	Connections int     `json:"connections"`
}

// TrafficReportRequest 流量上报请求
type TrafficReportRequest struct {
	NodeID      string `json:"node_id"`
	TunnelID    string `json:"tunnel_id"`
	TrafficIn   int64  `json:"traffic_in"`
	TrafficOut  int64  `json:"traffic_out"`
	Connections int    `json:"connections"`
}

// TunnelRule 隧道规则
type TunnelRule struct {
	TunnelID     string         `json:"tunnel_id"`
	Name         string         `json:"name"`
	Protocol     string         `json:"protocol"`
	LocalPort    int            `json:"local_port"`
	Targets      []TunnelTarget `json:"targets"`
	Enabled      bool           `json:"enabled"`
	UserID       string         `json:"user_id"`
	MaxBandwidth int64          `json:"max_bandwidth,omitempty"`
	EntryGroupID string         `json:"entry_group_id"`
	ExitGroupID  string         `json:"exit_group_id"`
}

// TunnelTarget 隧道目标
type TunnelTarget struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Weight int    `json:"weight"`
}

// SyncRulesRequest 同步规则请求
type SyncRulesRequest struct {
	Rules []TunnelRule `json:"rules"`
	Force bool         `json:"force"`
}

// DeleteRuleRequest 删除规则请求
type DeleteRuleRequest struct {
	TunnelID string `json:"tunnel_id"`
}

// ErrorMessage 错误消息
type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// MonitoringReportRequest 监控数据上报请求
type MonitoringReportRequest struct {
	NodeID        string       `json:"node_id"`
	Timestamp     time.Time    `json:"timestamp"`
	SystemInfo    SystemInfo   `json:"system_info"`
	NetworkStats  NetworkStats `json:"network_stats"`
	TunnelStats   TunnelStats  `json:"tunnel_stats"`
	Performance   Performance  `json:"performance"`
	AppInfo       AppInfo      `json:"app_info"`
	ConfigVersion string       `json:"config_version"`
}

type SystemInfo struct {
	Uptime             int64     `json:"uptime"`
	BootTime           time.Time `json:"boot_time"`
	CPUUsage           float64   `json:"cpu_usage"`
	CPULoad1m          float64   `json:"cpu_load_1m"`
	CPULoad5m          float64   `json:"cpu_load_5m"`
	CPULoad15m         float64   `json:"cpu_load_15m"`
	CPUCores           int       `json:"cpu_cores"`
	MemoryTotal        int64     `json:"memory_total"`
	MemoryUsed         int64     `json:"memory_used"`
	MemoryAvailable    int64     `json:"memory_available"`
	MemoryUsagePercent float64   `json:"memory_usage_percent"`
	DiskTotal          int64     `json:"disk_total"`
	DiskUsed           int64     `json:"disk_used"`
	DiskAvailable      int64     `json:"disk_available"`
	DiskUsagePercent   float64   `json:"disk_usage_percent"`
}

type NetworkStats struct {
	Interfaces       []NetworkInterface `json:"interfaces"`
	BandwidthIn      int64              `json:"bandwidth_in"`
	BandwidthOut     int64              `json:"bandwidth_out"`
	TCPConnections   int                `json:"tcp_connections"`
	UDPConnections   int                `json:"udp_connections"`
	TotalConnections int                `json:"total_connections"`
	TrafficInBytes   int64              `json:"traffic_in_bytes"`
	TrafficOutBytes  int64              `json:"traffic_out_bytes"`
	PacketsIn        int64              `json:"packets_in"`
	PacketsOut       int64              `json:"packets_out"`
	ConnectionErrors int                `json:"connection_errors"`
}

type NetworkInterface struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Status    string `json:"status"`
	Speed     int64  `json:"speed"`
	RxBytes   int64  `json:"rx_bytes"`
	TxBytes   int64  `json:"tx_bytes"`
	RxPackets int64  `json:"rx_packets"`
	TxPackets int64  `json:"tx_packets"`
	RxErrors  int64  `json:"rx_errors"`
	TxErrors  int64  `json:"tx_errors"`
}

type TunnelStats struct {
	ActiveTunnels int                `json:"active_tunnels"`
	TunnelErrors  int                `json:"tunnel_errors"`
	TunnelList    []TunnelStatusInfo `json:"tunnel_list"`
}

type TunnelStatusInfo struct {
	TunnelID        string  `json:"tunnel_id"`
	Name            string  `json:"name"`
	Protocol        string  `json:"protocol"`
	LocalPort       int     `json:"local_port"`
	Status          string  `json:"status"`
	Connections     int     `json:"connections"`
	TrafficIn       int64   `json:"traffic_in"`
	TrafficOut      int64   `json:"traffic_out"`
	AvgResponseTime float64 `json:"avg_response_time"`
	ErrorCount      int     `json:"error_count"`
}

type Performance struct {
	AvgResponseTime float64 `json:"avg_response_time"`
	MaxResponseTime float64 `json:"max_response_time"`
	MinResponseTime float64 `json:"min_response_time"`
	RequestsPerSec  int     `json:"requests_per_sec"`
	ErrorRate       float64 `json:"error_rate"`
}

type AppInfo struct {
	Version      string    `json:"version"`
	GoVersion    string    `json:"go_version"`
	OSInfo       string    `json:"os_info"`
	Architecture string    `json:"architecture"`
	StartTime    time.Time `json:"start_time"`
}

// MonitoringConfigUpdate 监控配置更新
type MonitoringConfigUpdate struct {
	NodeID               string  `json:"node_id"`
	MonitoringEnabled    bool    `json:"monitoring_enabled"`
	ReportInterval       int     `json:"report_interval"`
	CollectSystemInfo    bool    `json:"collect_system_info"`
	CollectNetworkStats  bool    `json:"collect_network_stats"`
	CollectTunnelStats   bool    `json:"collect_tunnel_stats"`
	CollectPerformance   bool    `json:"collect_performance"`
	AlertCPUThreshold    float64 `json:"alert_cpu_threshold"`
	AlertMemoryThreshold float64 `json:"alert_memory_threshold"`
	AlertDiskThreshold   float64 `json:"alert_disk_threshold"`
}

// NewMessage 创建新消息
func NewMessage(msgType MessageType, data interface{}) (*Message, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Data:      dataBytes,
	}, nil
}

// ParseData 解析消息数据
func (m *Message) ParseData(v interface{}) error {
	return json.Unmarshal(m.Data, v)
}
