package model

import (
	"time"
)

// NodeStatus 节点状态
type NodeStatus string

const (
	NodeStatusOnline     NodeStatus = "online"
	NodeStatusOffline    NodeStatus = "offline"
	NodeStatusError      NodeStatus = "error"
	NodeStatusConnecting NodeStatus = "connecting"
	NodeStatusDisabled   NodeStatus = "disabled"
)

// NodeRole 节点角色
type NodeRole string

const (
	NodeRoleIngress NodeRole = "ingress" // 入口节点
	NodeRoleEgress  NodeRole = "egress"  // 出口节点
	NodeRoleBoth    NodeRole = "both"    // 双向节点
)

// Node 节点
type Node struct {
	// 基本信息
	ID          string     `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	Status      NodeStatus `json:"status" db:"status"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	LastOnline  time.Time  `json:"last_online" db:"last_online"`

	// 硬件信息
	HardwareID string `json:"hardware_id" db:"hardware_id"`
	SystemInfo string `json:"system_info" db:"system_info"`
	IPAddress  string `json:"ip_address" db:"ip_address"`
	Version    string `json:"version" db:"version"`

	// 角色信息
	Role NodeRole `json:"role" db:"role"`

	// 关联组
	Groups []string `json:"groups" db:"-"`
}

// NodeMetrics 节点指标
type NodeMetrics struct {
	NodeID      string    `json:"node_id"`
	Timestamp   time.Time `json:"timestamp"`
	CPUUsage    float64   `json:"cpu_usage"`    // 百分比
	MemoryUsage float64   `json:"memory_usage"` // 百分比
	DiskUsage   float64   `json:"disk_usage"`   // 百分比
	NetworkIn   int64     `json:"network_in"`   // 字节/秒
	NetworkOut  int64     `json:"network_out"`  // 字节/秒
	Connections int       `json:"connections"`  // 连接数
	Goroutines  int       `json:"goroutines"`   // Go协程数
	GCPause     int64     `json:"gc_pause"`     // 纳秒
}
