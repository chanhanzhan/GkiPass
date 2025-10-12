package model

import (
	"time"
)

// Tunnel 隧道
type Tunnel struct {
	// 基本信息
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Enabled     bool      `json:"enabled" db:"enabled"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	CreatedBy   string    `json:"created_by" db:"created_by"`

	// 节点配置
	IngressNodeID   string `json:"ingress_node_id" db:"ingress_node_id"`
	EgressNodeID    string `json:"egress_node_id" db:"egress_node_id"`
	IngressGroupID  string `json:"ingress_group_id" db:"ingress_group_id"`
	EgressGroupID   string `json:"egress_group_id" db:"egress_group_id"`
	IngressProtocol string `json:"ingress_protocol" db:"ingress_protocol"`
	EgressProtocol  string `json:"egress_protocol" db:"egress_protocol"`

	// 隧道配置
	ListenPort       int    `json:"listen_port" db:"listen_port"`
	TargetAddress    string `json:"target_address" db:"target_address"`
	TargetPort       int    `json:"target_port" db:"target_port"`
	EnableEncryption bool   `json:"enable_encryption" db:"enable_encryption"`

	// 高级选项
	RateLimitBPS   int64 `json:"rate_limit_bps" db:"rate_limit_bps"`
	MaxConnections int   `json:"max_connections" db:"max_connections"`
	IdleTimeout    int   `json:"idle_timeout" db:"idle_timeout"`

	// 关联规则
	RuleIDs []string `json:"rule_ids" db:"-"`

	// 统计信息
	ConnectionCount int64     `json:"connection_count" db:"-"`
	BytesIn         int64     `json:"bytes_in" db:"-"`
	BytesOut        int64     `json:"bytes_out" db:"-"`
	LastActive      time.Time `json:"last_active" db:"-"`
}

// ProbeResult 探测结果
type ProbeResult struct {
	TunnelID     string                 `json:"tunnel_id"`
	Success      bool                   `json:"success"`
	Message      string                 `json:"message"`
	ResponseTime int                    `json:"response_time"` // 毫秒
	Timestamp    time.Time              `json:"timestamp"`
	Details      map[string]interface{} `json:"details,omitempty"`
}

// TunnelStats 隧道统计信息
type TunnelStats struct {
	TunnelID        string    `json:"tunnel_id"`
	ConnectionCount int64     `json:"connection_count"`
	BytesIn         int64     `json:"bytes_in"`
	BytesOut        int64     `json:"bytes_out"`
	LastActive      time.Time `json:"last_active"`
}
