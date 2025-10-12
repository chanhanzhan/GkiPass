package heartbeat

import (
	"encoding/json"
	"fmt"
	"time"

	"crypto/rand"
	"encoding/hex"
)

// MessageType 心跳消息类型
type MessageType int

const (
	MessageTypePing MessageType = iota
	MessageTypePong
	MessageTypeProbe    // 网络探测
	MessageTypeProbeAck // 探测应答
)

// String 返回消息类型名称
func (mt MessageType) String() string {
	switch mt {
	case MessageTypePing:
		return "PING"
	case MessageTypePong:
		return "PONG"
	case MessageTypeProbe:
		return "PROBE"
	case MessageTypeProbeAck:
		return "PROBE_ACK"
	default:
		return "UNKNOWN"
	}
}

// Message 心跳消息
type Message struct {
	Type      MessageType            `json:"type"`
	ID        string                 `json:"id"`
	Timestamp int64                  `json:"timestamp"` // Unix timestamp (nanoseconds)
	Sequence  uint64                 `json:"sequence"`  // 序列号
	NodeID    string                 `json:"node_id"`   // 发送节点ID
	Payload   []byte                 `json:"payload"`   // 载荷数据
	Metadata  map[string]interface{} `json:"metadata"`  // 元数据
}

// NewMessage 创建心跳消息
func NewMessage(msgType MessageType, nodeID string, sequence uint64) *Message {
	return &Message{
		Type:      msgType,
		ID:        generateMessageID(),
		Timestamp: time.Now().UnixNano(),
		Sequence:  sequence,
		NodeID:    nodeID,
		Payload:   nil,
		Metadata:  make(map[string]interface{}),
	}
}

// NewPingMessage 创建Ping消息
func NewPingMessage(nodeID string, sequence uint64) *Message {
	msg := NewMessage(MessageTypePing, nodeID, sequence)
	msg.Metadata["probe_size"] = 0
	return msg
}

// NewPongMessage 创建Pong消息
func NewPongMessage(nodeID string, pingMessage *Message) *Message {
	msg := NewMessage(MessageTypePong, nodeID, pingMessage.Sequence)
	msg.Metadata["ping_id"] = pingMessage.ID
	msg.Metadata["ping_timestamp"] = pingMessage.Timestamp
	return msg
}

// NewProbeMessage 创建探测消息
func NewProbeMessage(nodeID string, sequence uint64, probeSize int) *Message {
	msg := NewMessage(MessageTypeProbe, nodeID, sequence)

	// 创建指定大小的探测载荷
	if probeSize > 0 {
		payload := make([]byte, probeSize)
		rand.Read(payload)
		msg.Payload = payload
	}

	msg.Metadata["probe_size"] = probeSize
	return msg
}

// NewProbeAckMessage 创建探测应答消息
func NewProbeAckMessage(nodeID string, probeMessage *Message) *Message {
	msg := NewMessage(MessageTypeProbeAck, nodeID, probeMessage.Sequence)
	msg.Metadata["probe_id"] = probeMessage.ID
	msg.Metadata["probe_timestamp"] = probeMessage.Timestamp
	msg.Metadata["probe_size"] = len(probeMessage.Payload)
	return msg
}

// GetAge 获取消息年龄
func (m *Message) GetAge() time.Duration {
	return time.Duration(time.Now().UnixNano() - m.Timestamp)
}

// IsExpired 检查消息是否过期
func (m *Message) IsExpired(timeout time.Duration) bool {
	return m.GetAge() > timeout
}

// GetSize 获取消息大小（字节）
func (m *Message) GetSize() int {
	data, _ := json.Marshal(m)
	return len(data)
}

// Encode 编码消息
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// Decode 解码消息
func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("解码心跳消息失败: %w", err)
	}
	return &msg, nil
}

// generateMessageID 生成消息ID
func generateMessageID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// RTTMeasurement RTT测量结果
type RTTMeasurement struct {
	PingID    string        `json:"ping_id"`
	Sequence  uint64        `json:"sequence"`
	RTT       time.Duration `json:"rtt"`
	Timestamp time.Time     `json:"timestamp"`
	ProbeSize int           `json:"probe_size"`
	Success   bool          `json:"success"`
}

// NewRTTMeasurement 创建RTT测量结果
func NewRTTMeasurement(pingID string, sequence uint64, rtt time.Duration, probeSize int, success bool) *RTTMeasurement {
	return &RTTMeasurement{
		PingID:    pingID,
		Sequence:  sequence,
		RTT:       rtt,
		Timestamp: time.Now(),
		ProbeSize: probeSize,
		Success:   success,
	}
}

// PacketLossMeasurement 丢包测量结果
type PacketLossMeasurement struct {
	WindowStart     time.Time `json:"window_start"`
	WindowEnd       time.Time `json:"window_end"`
	SentPackets     int       `json:"sent_packets"`
	ReceivedPackets int       `json:"received_packets"`
	LossRate        float64   `json:"loss_rate"`
}

// NewPacketLossMeasurement 创建丢包测量结果
func NewPacketLossMeasurement(start, end time.Time, sent, received int) *PacketLossMeasurement {
	lossRate := 0.0
	if sent > 0 {
		lossRate = float64(sent-received) / float64(sent) * 100
	}

	return &PacketLossMeasurement{
		WindowStart:     start,
		WindowEnd:       end,
		SentPackets:     sent,
		ReceivedPackets: received,
		LossRate:        lossRate,
	}
}

// NetworkQuality 网络质量评估
type NetworkQuality struct {
	Score        float64              `json:"score"`        // 0-100分
	Grade        QualityGrade         `json:"grade"`        // 质量等级
	RTTStats     RTTStatistics        `json:"rtt_stats"`    // RTT统计
	LossStats    PacketLossStatistics `json:"loss_stats"`   // 丢包统计
	JitterStats  JitterStatistics     `json:"jitter_stats"` // 抖动统计
	Timestamp    time.Time            `json:"timestamp"`
	SamplePeriod time.Duration        `json:"sample_period"` // 采样周期
}

// QualityGrade 质量等级
type QualityGrade int

const (
	QualityGradeExcellent QualityGrade = iota // 90-100分
	QualityGradeGood                          // 75-89分
	QualityGradeFair                          // 60-74分
	QualityGradePoor                          // 40-59分
	QualityGradeBad                           // 0-39分
)

// String 返回质量等级名称
func (qg QualityGrade) String() string {
	switch qg {
	case QualityGradeExcellent:
		return "EXCELLENT"
	case QualityGradeGood:
		return "GOOD"
	case QualityGradeFair:
		return "FAIR"
	case QualityGradePoor:
		return "POOR"
	case QualityGradeBad:
		return "BAD"
	default:
		return "UNKNOWN"
	}
}

// GetGradeFromScore 根据分数获取等级
func GetGradeFromScore(score float64) QualityGrade {
	switch {
	case score >= 90:
		return QualityGradeExcellent
	case score >= 75:
		return QualityGradeGood
	case score >= 60:
		return QualityGradeFair
	case score >= 40:
		return QualityGradePoor
	default:
		return QualityGradeBad
	}
}

// RTTStatistics RTT统计信息
type RTTStatistics struct {
	Min     time.Duration `json:"min"`
	Max     time.Duration `json:"max"`
	Average time.Duration `json:"average"`
	Median  time.Duration `json:"median"`
	StdDev  time.Duration `json:"std_dev"`
	Count   int           `json:"count"`
}

// PacketLossStatistics 丢包统计信息
type PacketLossStatistics struct {
	TotalSent     int     `json:"total_sent"`
	TotalReceived int     `json:"total_received"`
	TotalLost     int     `json:"total_lost"`
	LossRate      float64 `json:"loss_rate"`  // 百分比
	BurstLoss     int     `json:"burst_loss"` // 连续丢包数
}

// JitterStatistics 抖动统计信息
type JitterStatistics struct {
	Min     time.Duration `json:"min"`
	Max     time.Duration `json:"max"`
	Average time.Duration `json:"average"`
	StdDev  time.Duration `json:"std_dev"`
	Count   int           `json:"count"`
}

// HealthStatus 健康状态
type HealthStatus struct {
	IsHealthy        bool            `json:"is_healthy"`
	HealthScore      float64         `json:"health_score"` // 0-100
	LastHeartbeat    time.Time       `json:"last_heartbeat"`
	ConsecutiveFails int             `json:"consecutive_fails"`
	NetworkQuality   *NetworkQuality `json:"network_quality"`
	Issues           []string        `json:"issues"`          // 健康问题列表
	Recommendations  []string        `json:"recommendations"` // 建议
}

// NewHealthStatus 创建健康状态
func NewHealthStatus() *HealthStatus {
	return &HealthStatus{
		IsHealthy:        true,
		HealthScore:      100.0,
		LastHeartbeat:    time.Now(),
		ConsecutiveFails: 0,
		Issues:           make([]string, 0),
		Recommendations:  make([]string, 0),
	}
}

// AddIssue 添加健康问题
func (hs *HealthStatus) AddIssue(issue string) {
	hs.Issues = append(hs.Issues, issue)
}

// AddRecommendation 添加建议
func (hs *HealthStatus) AddRecommendation(recommendation string) {
	hs.Recommendations = append(hs.Recommendations, recommendation)
}

// UpdateHealthScore 更新健康分数
func (hs *HealthStatus) UpdateHealthScore(score float64) {
	hs.HealthScore = score
	hs.IsHealthy = score >= 60.0 // 60分以上认为健康
}

// GetHealthGrade 获取健康等级
func (hs *HealthStatus) GetHealthGrade() QualityGrade {
	return GetGradeFromScore(hs.HealthScore)
}





