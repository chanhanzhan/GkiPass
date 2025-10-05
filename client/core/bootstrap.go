package core

import (
	"encoding/json"
	"fmt"
	"time"

	"gkipass/client/config"
	"gkipass/client/logger"
	"gkipass/client/ws"

	"go.uber.org/zap"
)

// NodeBootstrap 节点启动引导器
type NodeBootstrap struct {
	serverURL string
	token     string
	nodeType  string
	wsClient  *ws.Client
}

// NewNodeBootstrap 创建启动引导器
func NewNodeBootstrap(serverURL, token, nodeType string) *NodeBootstrap {
	return &NodeBootstrap{
		serverURL: serverURL,
		token:     token,
		nodeType:  nodeType,
	}
}

// Bootstrap 执行启动引导
// 1. 连接到Plane（使用0-RTT如果可用）
// 2. 发送注册请求（携带最少信息）
// 3. 等待后端下发完整配置
// 4. 应用配置并启动服务
func (nb *NodeBootstrap) Bootstrap() (*config.Config, *ws.Client, error) {
	logger.Info("🚀 节点启动引导开始",
		zap.String("server", nb.serverURL),
		zap.String("type", nb.nodeType))

	// 步骤1: 创建最小配置（仅用于连接）
	minimalCfg := &config.PlaneConfig{
		URL:                  nb.serverURL + "/ws/node",
		CK:                   nb.token,
		ReconnectInterval:    5,
		MaxReconnectAttempts: 0,
		Timeout:              30,
	}

	// 步骤2: 建立WebSocket连接
	wsClient := ws.NewClient(minimalCfg)

	logger.Info("📡 连接到Plane...")
	if err := wsClient.Connect(); err != nil {
		return nil, nil, fmt.Errorf("连接失败: %w", err)
	}

	logger.Info("✅ 连接成功，发送注册请求...")

	// 步骤3: 发送最小注册请求（仅标识符）
	registerReq := ws.NodeRegisterRequest{
		NodeID:   generateNodeID(),
		NodeType: nb.nodeType,
		CK:       nb.token,
		Version:  "1.0.0",
		Capabilities: map[string]bool{
			"tcp":             true,
			"udp":             true,
			"protocol_detect": true,
			"multiplex":       true,
			"rate_limit":      true,
			"load_balance":    true,
		},
	}

	registerMsg, _ := ws.NewMessage(ws.MsgTypeNodeRegister, registerReq)
	if err := wsClient.Send(registerMsg); err != nil {
		return nil, nil, fmt.Errorf("发送注册请求失败: %w", err)
	}

	logger.Info("⏳ 等待后端下发配置...")

	// 步骤4: 等待注册确认和配置下发
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-timeout.C:
			return nil, nil, fmt.Errorf("等待配置超时")
		default:
			msg, err := wsClient.Receive()
			if err != nil {
				continue
			}

			switch msg.Type {
			case ws.MsgTypeRegisterAck:
				var ack ws.NodeRegisterResponse
				if err := msg.ParseData(&ack); err != nil {
					continue
				}

				if !ack.Success {
					return nil, nil, fmt.Errorf("注册失败: %s", ack.Message)
				}

				logger.Info("✅ 注册成功", zap.String("node_id", ack.NodeID))

			case ws.MsgTypeSyncRules:
				// 接收到配置，构建完整config
				var syncReq ws.SyncRulesRequest
				if err := msg.ParseData(&syncReq); err != nil {
					continue
				}

				logger.Info("✅ 收到配置",
					zap.Int("rules", len(syncReq.Rules)))

				// 构建完整配置
				fullConfig := nb.buildFullConfig(registerReq.NodeID, syncReq)

				logger.Info("🎉 启动引导完成")
				return fullConfig, wsClient, nil
			}
		}
	}
}

// buildFullConfig 构建完整配置
func (nb *NodeBootstrap) buildFullConfig(nodeID string, syncReq ws.SyncRulesRequest) *config.Config {
	return &config.Config{
		Node: config.NodeConfig{
			ID:      nodeID,
			Name:    fmt.Sprintf("%s-node", nb.nodeType),
			Type:    nb.nodeType,
			GroupID: "", // 从规则中获取
		},
		Plane: config.PlaneConfig{
			URL:                  nb.serverURL + "/ws/node",
			CK:                   nb.token,
			ReconnectInterval:    5,
			MaxReconnectAttempts: 0,
			Timeout:              30,
		},
		Pool: config.PoolConfig{
			MinConns:          5,
			MaxConns:          100,
			IdleTimeout:       300,
			HeartbeatInterval: 30,
			AutoScale:         true,
		},
		RateLimit: config.RateLimitConfig{
			Enabled:          true,
			DefaultBandwidth: 100 * 1024 * 1024, // 100MB/s
			Burst:            50 * 1024 * 1024,
		},
		TLS: config.TLSConfig{
			Enabled:         false,
			PinVerification: false,
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "console",
			Output: "stdout",
		},
	}
}

// generateNodeID 生成节点ID
func generateNodeID() string {
	return fmt.Sprintf("node-%d", time.Now().UnixNano())
}

// ConfigResponse 后端下发的配置响应
type ConfigResponse struct {
	NodeConfig  NodeConfigData  `json:"node_config"`
	PoolConfig  PoolConfigData  `json:"pool_config"`
	TunnelRules []ws.TunnelRule `json:"tunnel_rules"`
	PeerNodes   []PeerNodeData  `json:"peer_nodes"`
}

// NodeConfigData 节点配置数据
type NodeConfigData struct {
	GroupID      string                 `json:"group_id"`
	Region       string                 `json:"region"`
	Tags         map[string]string      `json:"tags"`
	MaxBandwidth int64                  `json:"max_bandwidth"`
	Features     map[string]interface{} `json:"features"`
}

// PoolConfigData 连接池配置
type PoolConfigData struct {
	MinConns    int  `json:"min_conns"`
	MaxConns    int  `json:"max_conns"`
	IdleTimeout int  `json:"idle_timeout"`
	AutoScale   bool `json:"auto_scale"`
}

// PeerNodeData 对端节点数据
type PeerNodeData struct {
	NodeID   string `json:"node_id"`
	NodeType string `json:"node_type"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
}

// ParseConfigFromMessage 从消息解析配置
func ParseConfigFromMessage(msg *ws.Message) (*ConfigResponse, error) {
	var cfg ConfigResponse
	if err := json.Unmarshal(msg.Data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
