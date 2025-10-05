package models

import "time"

// NodeConfig 节点配置（下发给节点的完整配置）
type NodeConfig struct {
	NodeInfo     NodeInfo       `json:"node_info"`    // 节点基本信息
	Tunnels      []TunnelConfig `json:"tunnels"`      // 有效隧道列表
	PeerServers  []PeerServer   `json:"peer_servers"` // 对端服务器列表
	Capabilities NodeCapability `json:"capabilities"` // 节点能力配置
	Version      string         `json:"version"`      // 配置版本号
	UpdatedAt    time.Time      `json:"updated_at"`   // 配置更新时间
}

// NodeInfo 节点基本信息
type NodeInfo struct {
	NodeID    string            `json:"node_id"`    // 节点ID
	NodeName  string            `json:"node_name"`  // 节点名称
	NodeType  string            `json:"node_type"`  // entry/exit
	GroupID   string            `json:"group_id"`   // 节点组ID
	GroupName string            `json:"group_name"` // 节点组名称
	Region    string            `json:"region"`     // 地域
	Tags      map[string]string `json:"tags"`       // 标签
}

// TunnelConfig 隧道配置
type TunnelConfig struct {
	TunnelID          string                 `json:"tunnel_id"`          // 隧道ID
	Name              string                 `json:"name"`               // 隧道名称
	Protocol          string                 `json:"protocol"`           // tcp/udp/http/https
	LocalPort         int                    `json:"local_port"`         // 本地监听端口（入口节点）
	Targets           []TargetConfig         `json:"targets"`            // 目标列表（出口节点）
	Enabled           bool                   `json:"enabled"`            // 是否启用
	DisabledProtocols []string               `json:"disabled_protocols"` // 禁用的协议列表
	MaxBandwidth      int64                  `json:"max_bandwidth"`      // 最大带宽限制(bps)
	MaxConnections    int                    `json:"max_connections"`    // 最大连接数
	Options           map[string]interface{} `json:"options"`            // 其他选项
}

// TargetConfig 目标配置
type TargetConfig struct {
	Host           string `json:"host"`             // 域名或IP
	Port           int    `json:"port"`             // 端口
	Weight         int    `json:"weight"`           // 权重（负载均衡）
	Protocol       string `json:"protocol"`         // tcp/udp/http/https
	HealthCheck    bool   `json:"health_check"`     // 是否启用健康检查
	HealthCheckURL string `json:"health_check_url"` // 健康检查URL
	Timeout        int    `json:"timeout"`          // 超时时间(秒)
	MaxRetries     int    `json:"max_retries"`      // 最大重试次数
}

// PeerServer 对端服务器信息
type PeerServer struct {
	ServerID   string   `json:"server_id"`   // 服务器ID
	ServerName string   `json:"server_name"` // 服务器名称
	Host       string   `json:"host"`        // 主机地址
	Port       int      `json:"port"`        // 端口
	Type       string   `json:"type"`        // entry/exit
	Region     string   `json:"region"`      // 地域
	Priority   int      `json:"priority"`    // 优先级
	Protocols  []string `json:"protocols"`   // 支持的协议
}

// NodeCapability 节点能力配置
type NodeCapability struct {
	SupportedProtocols []string        `json:"supported_protocols"` // 支持的协议
	MaxTunnels         int             `json:"max_tunnels"`         // 最大隧道数
	MaxBandwidth       int64           `json:"max_bandwidth"`       // 最大带宽
	MaxConnections     int             `json:"max_connections"`     // 最大连接数
	Features           map[string]bool `json:"features"`            // 特性开关
	ReportInterval     int             `json:"report_interval"`     // 上报间隔(秒)
	HeartbeatInterval  int             `json:"heartbeat_interval"`  // 心跳间隔(秒)
}

// ConfigUpdateNotification 配置更新通知
type ConfigUpdateNotification struct {
	Type      string    `json:"type"`       // full/incremental
	Version   string    `json:"version"`    // 新版本号
	Changes   []string  `json:"changes"`    // 变更内容
	UpdatedAt time.Time `json:"updated_at"` // 更新时间
}

