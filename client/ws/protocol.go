package ws

import (
	"encoding/json"
	"time"
)

// MessageType 消息类型
type MessageType string

const (
	// 节点 -> 服务器
	MsgTypeNodeRegister  MessageType = "node_register"
	MsgTypeHeartbeat     MessageType = "heartbeat"
	MsgTypeTrafficReport MessageType = "traffic_report"
	MsgTypeNodeStatus    MessageType = "node_status"

	// 服务器 -> 节点
	MsgTypeRegisterAck MessageType = "register_ack"
	MsgTypeSyncRules   MessageType = "sync_rules"
	MsgTypeDeleteRule  MessageType = "delete_rule"
	MsgTypeReload      MessageType = "reload"
	MsgTypePing        MessageType = "ping"

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
