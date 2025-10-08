package ws

import (
	"encoding/json"
	"time"
)

// MessageType 消息类型
type MessageType string

const (
	// 节点 -> 服务器
	MsgTypeNodeRegister     MessageType = "node_register"     // 节点注册
	MsgTypeHeartbeat        MessageType = "heartbeat"         // 心跳
	MsgTypeTrafficReport    MessageType = "traffic_report"    // 流量上报
	MsgTypeNodeStatus       MessageType = "node_status"       // 节点状态
	MsgTypeMonitoringReport MessageType = "monitoring_report" // 监控数据上报

	// 服务器 -> 节点
	MsgTypeRegisterAck   MessageType = "register_ack"   // 注册确认
	MsgTypeSyncRules     MessageType = "sync_rules"     // 同步规则
	MsgTypeDeleteRule    MessageType = "delete_rule"    // 删除规则
	MsgTypeReload        MessageType = "reload"         // 重载配置
	MsgTypePing          MessageType = "ping"           // Ping
	MsgTypeConfigUpdate  MessageType = "config_update"  // 配置更新
	MsgTypeMonitorConfig MessageType = "monitor_config" // 监控配置

	// 双向
	MsgTypePong  MessageType = "pong"  // Pong
	MsgTypeError MessageType = "error" // 错误消息
)

// Message WebSocket 消息结构
type Message struct {
	Type      MessageType     `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// NodeRegisterRequest 节点注册请求
type NodeRegisterRequest struct {
	NodeID       string          `json:"node_id"`      // 节点ID
	NodeName     string          `json:"node_name"`    // 节点名称
	NodeType     string          `json:"node_type"`    // entry/exit
	GroupID      string          `json:"group_id"`     // 节点组ID
	Version      string          `json:"version"`      // 节点版本
	IP           string          `json:"ip"`           // 节点IP
	Port         int             `json:"port"`         // 节点端口
	CK           string          `json:"ck"`           // Connection Key
	Capabilities map[string]bool `json:"capabilities"` // 节点能力
}

// NodeRegisterResponse 节点注册响应
type NodeRegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	NodeID  string `json:"node_id"`
	Token   string `json:"token,omitempty"` // 会话token（可选）
}

// HeartbeatRequest 心跳请求
type HeartbeatRequest struct {
	NodeID      string  `json:"node_id"`
	Status      string  `json:"status"`       // online/busy/offline
	CPUUsage    float64 `json:"cpu_usage"`    // CPU使用率 0-100
	MemoryUsage int64   `json:"memory_usage"` // 内存使用(bytes)
	Connections int     `json:"connections"`  // 当前连接数
}

// HeartbeatResponse 心跳响应
type HeartbeatResponse struct {
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
	Actions   []string  `json:"actions,omitempty"` // 需要执行的操作
}

// TrafficReportRequest 流量上报请求
type TrafficReportRequest struct {
	NodeID      string           `json:"node_id"`
	TunnelID    string           `json:"tunnel_id"`
	TrafficIn   int64            `json:"traffic_in"`        // 入站流量(bytes)
	TrafficOut  int64            `json:"traffic_out"`       // 出站流量(bytes)
	Connections int              `json:"connections"`       // 连接数
	Details     map[string]int64 `json:"details,omitempty"` // 详细统计
}

// TrafficReportResponse 流量上报响应
type TrafficReportResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
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
	MaxBandwidth int64          `json:"max_bandwidth,omitempty"` // 带宽限制
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
	Force bool         `json:"force"` // 是否强制同步
}

// SyncRulesResponse 同步规则响应
type SyncRulesResponse struct {
	Success      bool     `json:"success"`
	AppliedCount int      `json:"applied_count"`
	FailedRules  []string `json:"failed_rules,omitempty"`
	Message      string   `json:"message,omitempty"`
}

// DeleteRuleRequest 删除规则请求
type DeleteRuleRequest struct {
	TunnelID string `json:"tunnel_id"`
}

// DeleteRuleResponse 删除规则响应
type DeleteRuleResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
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
