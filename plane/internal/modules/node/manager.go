package node

import (
	"encoding/json"
	"fmt"

	"gkipass/plane/db"
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/models"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// Manager 节点管理器
type Manager struct {
	db *db.Manager
}

// NewManager 创建节点管理器
func NewManager(dbManager *db.Manager) *Manager {
	return &Manager{db: dbManager}
}

// GetNodeConfig 获取节点完整配置
func (m *Manager) GetNodeConfig(nodeID string) (*models.NodeConfig, error) {
	// 1. 获取节点信息
	node, err := m.db.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		return nil, fmt.Errorf("节点不存在: %s", nodeID)
	}

	// 2. 获取节点组信息
	group, err := m.db.DB.SQLite.GetNodeGroup(node.GroupID)
	if err != nil || group == nil {
		return nil, fmt.Errorf("节点组不存在: %s", node.GroupID)
	}

	// 3. 获取该节点组的所有隧道
	tunnels, err := m.db.DB.SQLite.GetTunnelsByGroupID(node.GroupID, node.Type)
	if err != nil {
		logger.Error("获取隧道列表失败", zap.Error(err))
		tunnels = []*dbinit.Tunnel{} // 返回空列表
	}

	// 4. 构建节点信息
	nodeInfo := models.NodeInfo{
		NodeID:    node.ID,
		NodeName:  node.Name,
		NodeType:  node.Type,
		GroupID:   node.GroupID,
		GroupName: group.Name,
		Region:    node.Tags, // 可以从 tags 中解析
		Tags:      make(map[string]string),
	}

	// 5. 构建隧道配置列表
	tunnelConfigs := make([]models.TunnelConfig, 0, len(tunnels))
	for _, tunnel := range tunnels {
		if !tunnel.Enabled {
			continue // 跳过未启用的隧道
		}

		// 解析目标列表
		targets, err := m.parseTargets(tunnel.Targets)
		if err != nil {
			logger.Warn("解析目标列表失败",
				zap.String("tunnelID", tunnel.ID),
				zap.Error(err))
			continue
		}

		tunnelConfig := models.TunnelConfig{
			TunnelID:          tunnel.ID,
			Name:              tunnel.Name,
			Protocol:          tunnel.Protocol,
			LocalPort:         tunnel.LocalPort,
			Targets:           targets,
			Enabled:           tunnel.Enabled,
			DisabledProtocols: []string{}, // 可以从数据库扩展
			MaxBandwidth:      0,          // 可以从套餐限制获取
			MaxConnections:    0,          // 可以从套餐限制获取
			Options:           make(map[string]interface{}),
		}

		tunnelConfigs = append(tunnelConfigs, tunnelConfig)
	}

	// 6. 获取对端服务器列表
	peerServers := m.getPeerServers(node.Type, node.GroupID)

	// 7. 构建节点能力配置
	capability := models.NodeCapability{
		SupportedProtocols: []string{"tcp", "udp", "http", "https"},
		MaxTunnels:         100,
		MaxBandwidth:       1000000000, // 1Gbps
		MaxConnections:     10000,
		Features: map[string]bool{
			"load_balance":  true,
			"health_check":  true,
			"traffic_stats": true,
		},
		ReportInterval:    60, // 60秒上报一次流量
		HeartbeatInterval: 30, // 30秒心跳一次
	}

	// 8. 组装完整配置
	config := &models.NodeConfig{
		NodeInfo:     nodeInfo,
		Tunnels:      tunnelConfigs,
		PeerServers:  peerServers,
		Capabilities: capability,
		Version:      "1.0.0",
		UpdatedAt:    node.UpdatedAt,
	}

	logger.Info("生成节点配置",
		zap.String("nodeID", nodeID),
		zap.Int("tunnelCount", len(tunnelConfigs)))

	return config, nil
}

// parseTargets 解析目标列表
func (m *Manager) parseTargets(targetsJSON string) ([]models.TargetConfig, error) {
	if targetsJSON == "" {
		return []models.TargetConfig{}, nil
	}

	// 从 JSON 解析
	var dbTargets []dbinit.TunnelTarget
	if err := json.Unmarshal([]byte(targetsJSON), &dbTargets); err != nil {
		return nil, fmt.Errorf("解析目标列表失败: %w", err)
	}
	
	targets := make([]models.TargetConfig, 0, len(dbTargets))
	for _, t := range dbTargets {
		target := models.TargetConfig{
			Host:           t.Host,
			Port:           t.Port,
			Weight:         t.Weight,
			Protocol:       "tcp", // 默认TCP
			HealthCheck:    false,
			HealthCheckURL: "",
			Timeout:        30,
			MaxRetries:     3,
		}
		targets = append(targets, target)
	}

	return targets, nil
}

// getPeerServers 获取对端服务器列表
func (m *Manager) getPeerServers(nodeType, groupID string) []models.PeerServer {
	// 根据节点类型返回对端服务器
	// 入口节点需要知道所有出口节点
	// 出口节点需要知道所有入口节点

	var targetType string
	if nodeType == "entry" {
		targetType = "exit"
	} else {
		targetType = "entry"
	}

	// 获取对端类型的所有节点组
	groups, err := m.db.DB.SQLite.ListNodeGroups("", targetType)
	if err != nil {
		logger.Error("获取节点组失败", zap.Error(err))
		return []models.PeerServer{}
	}

	peerServers := make([]models.PeerServer, 0)
	for _, group := range groups {
		// 获取该组的所有在线节点
		nodes, _ := m.db.DB.SQLite.ListNodes(group.ID, "online", 0, 100)
		for _, node := range nodes {

			peer := models.PeerServer{
				ServerID:   node.ID,
				ServerName: node.Name,
				Host:       node.IP,
				Port:       node.Port,
				Type:       node.Type,
				Region:     node.Tags,
				Priority:   1,
				Protocols:  []string{"tcp", "udp", "http", "https"},
			}
			peerServers = append(peerServers, peer)
		}
	}

	return peerServers
}
