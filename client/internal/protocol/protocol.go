package protocol

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MessageType 消息类型
type MessageType string

const (
	// 认证和连接消息
	MessageTypeAuth       MessageType = "auth"
	MessageTypeAuthResult MessageType = "auth_result"
	MessageTypePing       MessageType = "ping"
	MessageTypePong       MessageType = "pong"
	MessageTypeDisconnect MessageType = "disconnect"

	// 规则和配置消息
	MessageTypeRuleSync        MessageType = "rule_sync"
	MessageTypeRuleSyncAck     MessageType = "rule_sync_ack"
	MessageTypeConfigUpdate    MessageType = "config_update"
	MessageTypeConfigUpdateAck MessageType = "config_update_ack"

	// 状态和监控消息
	MessageTypeStatusReport  MessageType = "status_report"
	MessageTypeMetricsReport MessageType = "metrics_report"
	MessageTypeLogReport     MessageType = "log_report"

	// 命令和控制消息
	MessageTypeCommand       MessageType = "command"
	MessageTypeCommandResult MessageType = "command_result"

	// 探测消息
	MessageTypeProbeRequest MessageType = "probe_request"
	MessageTypeProbeResult  MessageType = "probe_result"

	// 错误和通知消息
	MessageTypeError        MessageType = "error"
	MessageTypeNotification MessageType = "notification"
)

// Message WebSocket消息
type Message struct {
	Type      MessageType     `json:"type"`            // 消息类型
	ID        string          `json:"id,omitempty"`    // 消息ID，用于跟踪请求/响应
	Timestamp int64           `json:"timestamp"`       // 时间戳（毫秒）
	Data      json.RawMessage `json:"data,omitempty"`  // 消息数据
	Error     *ErrorInfo      `json:"error,omitempty"` // 错误信息
}

// ErrorInfo 错误信息
type ErrorInfo struct {
	Code    string `json:"code"`    // 错误代码
	Message string `json:"message"` // 错误消息
}

// NewMessage 创建新消息
func NewMessage(msgType MessageType, data interface{}) (*Message, error) {
	var rawData json.RawMessage
	if data != nil {
		bytes, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("marshal data failed: %w", err)
		}
		rawData = bytes
	}

	return &Message{
		Type:      msgType,
		ID:        generateID(),
		Timestamp: time.Now().UnixMilli(),
		Data:      rawData,
	}, nil
}

// NewErrorMessage 创建错误消息
func NewErrorMessage(code, message string) (*Message, error) {
	errorInfo := &ErrorInfo{
		Code:    code,
		Message: message,
	}

	return &Message{
		Type:      MessageTypeError,
		ID:        generateID(),
		Timestamp: time.Now().UnixMilli(),
		Error:     errorInfo,
	}, nil
}

// NewResponseMessage 创建响应消息
func NewResponseMessage(requestID string, msgType MessageType, data interface{}) (*Message, error) {
	var rawData json.RawMessage
	if data != nil {
		bytes, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("marshal data failed: %w", err)
		}
		rawData = bytes
	}

	return &Message{
		Type:      msgType,
		ID:        requestID,
		Timestamp: time.Now().UnixMilli(),
		Data:      rawData,
	}, nil
}

// ParseData 解析消息数据
func (m *Message) ParseData(v interface{}) error {
	if m.Data == nil {
		return fmt.Errorf("no data to parse")
	}
	return json.Unmarshal(m.Data, v)
}

// generateID 生成唯一消息ID
func generateID() string {
	return uuid.New().String()[:8]
}

// AuthRequest 认证请求
type AuthRequest struct {
	NodeID     string            `json:"node_id"`     // 节点ID
	Token      string            `json:"token"`       // 认证令牌
	NodeName   string            `json:"node_name"`   // 节点名称
	HardwareID string            `json:"hardware_id"` // 硬件ID
	SystemInfo map[string]string `json:"system_info"` // 系统信息
	Timestamp  int64             `json:"timestamp"`   // 时间戳
}

// AuthResponse 认证响应
type AuthResponse struct {
	Success  bool     `json:"success"`             // 是否成功
	Message  string   `json:"message,omitempty"`   // 消息
	Role     string   `json:"role,omitempty"`      // 角色
	Groups   []string `json:"groups,omitempty"`    // 组
	NodeName string   `json:"node_name,omitempty"` // 节点名称
}

// RuleSyncRequest 规则同步请求
type RuleSyncRequest struct {
	NodeID   string `json:"node_id"`   // 节点ID
	Version  int64  `json:"version"`   // 规则版本
	FullSync bool   `json:"full_sync"` // 是否全量同步
}

// RuleSyncResponse 规则同步响应
type RuleSyncResponse struct {
	Rules    json.RawMessage `json:"rules"`     // 规则数据
	Version  int64           `json:"version"`   // 规则版本
	FullSync bool            `json:"full_sync"` // 是否全量同步
}

// RuleSyncAck 规则同步确认
type RuleSyncAck struct {
	Status    string `json:"status"`            // 状态：success/failed
	Message   string `json:"message,omitempty"` // 消息
	Version   int64  `json:"version"`           // 规则版本
	Timestamp int64  `json:"timestamp"`         // 时间戳
}

// ConfigUpdateRequest 配置更新请求
type ConfigUpdateRequest struct {
	NodeID   string          `json:"node_id"`   // 节点ID
	Config   json.RawMessage `json:"config"`    // 配置数据
	Version  int64           `json:"version"`   // 配置版本
	FullSync bool            `json:"full_sync"` // 是否全量同步
}

// ConfigUpdateAck 配置更新确认
type ConfigUpdateAck struct {
	Status    string `json:"status"`            // 状态：success/failed
	Message   string `json:"message,omitempty"` // 消息
	Version   int64  `json:"version"`           // 配置版本
	Timestamp int64  `json:"timestamp"`         // 时间戳
}

// StatusReport 状态报告
type StatusReport struct {
	NodeID    string                 `json:"node_id"`           // 节点ID
	Status    string                 `json:"status"`            // 状态：online/busy/offline
	Uptime    int64                  `json:"uptime"`            // 运行时长（秒）
	Version   string                 `json:"version"`           // 版本
	Timestamp int64                  `json:"timestamp"`         // 时间戳
	Details   map[string]interface{} `json:"details,omitempty"` // 详细信息
}

// MetricsReport 指标报告
type MetricsReport struct {
	NodeID    string                 `json:"node_id"`   // 节点ID
	Timestamp int64                  `json:"timestamp"` // 时间戳
	Metrics   map[string]interface{} `json:"metrics"`   // 指标数据
}

// LogReport 日志报告
type LogReport struct {
	NodeID    string                 `json:"node_id"`           // 节点ID
	Timestamp int64                  `json:"timestamp"`         // 时间戳
	Level     string                 `json:"level"`             // 级别：debug/info/warn/error
	Message   string                 `json:"message"`           // 消息
	Source    string                 `json:"source"`            // 来源
	Details   map[string]interface{} `json:"details,omitempty"` // 详细信息
}

// Command 命令
type Command struct {
	Command   string                 `json:"command"`           // 命令
	Params    map[string]interface{} `json:"params,omitempty"`  // 参数
	Timeout   int                    `json:"timeout,omitempty"` // 超时（秒）
	Timestamp int64                  `json:"timestamp"`         // 时间戳
}

// CommandResult 命令结果
type CommandResult struct {
	Command   string                 `json:"command"`           // 命令
	Status    string                 `json:"status"`            // 状态：success/failed
	Message   string                 `json:"message,omitempty"` // 消息
	Result    map[string]interface{} `json:"result,omitempty"`  // 结果
	Timestamp int64                  `json:"timestamp"`         // 时间戳
}

// ProbeRequest 探测请求
type ProbeRequest struct {
	ProbeID       string   `json:"probe_id"`                 // 探测ID
	Type          string   `json:"type"`                     // 类型：ping/tcp/http
	Target        string   `json:"target"`                   // 目标
	Count         int      `json:"count"`                    // 次数
	Timeout       int      `json:"timeout"`                  // 超时（秒）
	SourceNodeID  string   `json:"source_node_id"`           // 源节点ID
	TargetNodeID  string   `json:"target_node_id,omitempty"` // 目标节点ID
	TargetAddress string   `json:"target_address,omitempty"` // 目标地址
	Protocol      string   `json:"protocol"`                 // 协议
	Options       []string `json:"options,omitempty"`        // 选项
}

// ProbeResult 探测结果
type ProbeResult struct {
	ProbeID       string                   `json:"probe_id"`                 // 探测ID
	Type          string                   `json:"type"`                     // 类型
	Target        string                   `json:"target"`                   // 目标
	SourceNodeID  string                   `json:"source_node_id"`           // 源节点ID
	TargetNodeID  string                   `json:"target_node_id,omitempty"` // 目标节点ID
	TargetAddress string                   `json:"target_address,omitempty"` // 目标地址
	Protocol      string                   `json:"protocol"`                 // 协议
	Success       bool                     `json:"success"`                  // 是否成功
	Message       string                   `json:"message,omitempty"`        // 消息
	Results       []map[string]interface{} `json:"results"`                  // 结果列表
	Summary       map[string]interface{}   `json:"summary"`                  // 摘要
	Timestamp     int64                    `json:"timestamp"`                // 时间戳
}

// Notification 通知
type Notification struct {
	Type      string                 `json:"type"`              // 类型：system/security/update
	Title     string                 `json:"title"`             // 标题
	Message   string                 `json:"message"`           // 消息
	Level     string                 `json:"level"`             // 级别：info/warn/error
	Timestamp int64                  `json:"timestamp"`         // 时间戳
	Details   map[string]interface{} `json:"details,omitempty"` // 详细信息
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
