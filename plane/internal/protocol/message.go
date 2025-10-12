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
		ID:        generateMessageID(),
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

// generateMessageID 生成消息ID
func generateMessageID() string {
	return uuid.New().String()[:8]
}
