package service

import (
	"encoding/json"
	"fmt"
	"time"

	"gkipass/plane/db"
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/models"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// NodeSyncService 节点同步服务
type NodeSyncService struct {
	db *db.Manager
}

// NewNodeSyncService 创建节点同步服务
func NewNodeSyncService(dbManager *db.Manager) *NodeSyncService {
	return &NodeSyncService{
		db: dbManager,
	}
}

// SyncTunnelsToNode 同步隧道到指定节点
func (s *NodeSyncService) SyncTunnelsToNode(nodeID string) error {
	node, err := s.db.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		return fmt.Errorf("节点 %s 不存在", nodeID)
	}

	// 获取节点所在组的隧道
	tunnels, err := s.db.DB.SQLite.GetTunnelsByGroupID(node.GroupID, node.Type)
	if err != nil {
		return fmt.Errorf("获取隧道失败: %w", err)
	}

	logger.Info("同步隧道到节点",
		zap.String("nodeID", nodeID),
		zap.String("groupID", node.GroupID),
		zap.Int("tunnelCount", len(tunnels)))

	return nil
}

// SyncTunnelsToGroup 同步隧道到节点组的所有节点
func (s *NodeSyncService) SyncTunnelsToGroup(groupID string) error {
	nodes, err := s.db.DB.SQLite.ListNodes(groupID, "", 0, 1000)
	if err != nil {
		return fmt.Errorf("获取节点列表失败: %w", err)
	}

	for _, node := range nodes {
		if err := s.SyncTunnelsToNode(node.ID); err != nil {
			logger.Error("同步隧道到节点失败",
				zap.String("nodeID", node.ID),
				zap.Error(err))
		}
	}

	return nil
}

// BuildNodeConfig 构建节点配置
func (s *NodeSyncService) BuildNodeConfig(nodeID string) (*models.NodeConfig, error) {
	node, err := s.db.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		return nil, fmt.Errorf("节点不存在")
	}

	// 获取节点组信息
	group, err := s.db.DB.SQLite.GetNodeGroup(node.GroupID)
	if err != nil || group == nil {
		return nil, fmt.Errorf("节点组不存在")
	}

	// 获取该节点组的所有隧道
	tunnels, err := s.db.DB.SQLite.GetTunnelsByGroupID(node.GroupID, node.Type)
	if err != nil {
		return nil, fmt.Errorf("获取隧道失败: %w", err)
	}

	// 构建隧道配置列表
	tunnelConfigs := make([]models.TunnelConfig, 0, len(tunnels))
	for _, tunnel := range tunnels {
		if !tunnel.Enabled {
			continue // 跳过未启用的隧道
		}

		// 解析目标列表
		var targets []dbinit.TunnelTarget
		if err := json.Unmarshal([]byte(tunnel.Targets), &targets); err != nil {
			logger.Warn("解析隧道目标失败", zap.String("tunnelID", tunnel.ID), zap.Error(err))
			continue
		}

		// 转换为models.TargetConfig
		targetConfigs := make([]models.TargetConfig, len(targets))
		for i, t := range targets {
			targetConfigs[i] = models.TargetConfig{
				Host:           t.Host,
				Port:           t.Port,
				Weight:         t.Weight,
				Protocol:       tunnel.Protocol,
				HealthCheck:    false,
				HealthCheckURL: "",
				Timeout:        30,
				MaxRetries:     3,
			}
		}

		tunnelConfigs = append(tunnelConfigs, models.TunnelConfig{
			TunnelID:          tunnel.ID,
			Name:              tunnel.Name,
			Protocol:          tunnel.Protocol,
			LocalPort:         tunnel.LocalPort,
			Targets:           targetConfigs,
			Enabled:           tunnel.Enabled,
			DisabledProtocols: []string{},
			MaxBandwidth:      0, // 0=无限制
			MaxConnections:    0, // 0=无限制
			Options:           make(map[string]interface{}),
		})
	}

	// 获取对端服务器信息
	peerServers := s.getPeerServers(node, group)

	// 构建节点配置
	configVersion := fmt.Sprintf("%d", time.Now().Unix())
	// 解析节点 Tags（JSON格式存储）
	nodeTags := make(map[string]string)
	if node.Tags != "" {
		_ = json.Unmarshal([]byte(node.Tags), &nodeTags)
	}

	// 从Tags中获取region，如果没有则使用默认值
	region := nodeTags["region"]
	if region == "" {
		region = "default"
	}

	config := &models.NodeConfig{
		NodeInfo: models.NodeInfo{
			NodeID:    node.ID,
			NodeName:  node.Name,
			NodeType:  node.Type,
			GroupID:   node.GroupID,
			GroupName: group.Name,
			Region:    region,
			Tags:      nodeTags,
		},
		Tunnels:     tunnelConfigs,
		PeerServers: peerServers,
		Capabilities: models.NodeCapability{
			SupportedProtocols: []string{"tcp", "udp", "http", "https"},
			MaxTunnels:         1000,
			MaxBandwidth:       1000 * 1024 * 1024, // 1Gbps
			MaxConnections:     10000,
			Features:           make(map[string]bool),
			ReportInterval:     60,
			HeartbeatInterval:  30,
		},
		Version:   configVersion,
		UpdatedAt: time.Now(),
	}

	return config, nil
}

// getPeerServers 获取对端服务器列表
func (s *NodeSyncService) getPeerServers(node *dbinit.Node, group *dbinit.NodeGroup) []models.PeerServer {
	var peerGroupType string
	if node.Type == "entry" {
		peerGroupType = "exit"
	} else {
		peerGroupType = "entry"
	}

	// 获取对端节点组
	peerGroups, err := s.db.DB.SQLite.ListNodeGroups(group.UserID, peerGroupType)
	if err != nil {
		logger.Error("获取对端节点组失败", zap.Error(err))
		return []models.PeerServer{}
	}

	peerServers := make([]models.PeerServer, 0)
	for _, peerGroup := range peerGroups {
		// 获取该组的所有在线节点
		nodes, err := s.db.DB.SQLite.ListNodes(peerGroup.ID, "online", 0, 100)
		if err != nil {
			continue
		}

		for _, peerNode := range nodes {
			peerServers = append(peerServers, models.PeerServer{
				ServerID:   peerNode.ID,
				ServerName: peerNode.Name,
				Host:       peerNode.IP,
				Port:       peerNode.Port,
			})
		}
	}

	return peerServers
}
func (s *NodeSyncService) NotifyConfigUpdate(nodeID string, config *models.NodeConfig) error {
	logger.Info("通知节点配置更新",
		zap.String("nodeID", nodeID),
		zap.String("configVersion", config.Version),
		zap.Int("tunnelCount", len(config.Tunnels)))

	return nil
}

func (s *NodeSyncService) UpdateNodeConfig(tunnelID string) error {
	// 获取隧道信息
	tunnel, err := s.db.DB.SQLite.GetTunnel(tunnelID)
	if err != nil {
		return err
	}

	// 同步到入口组
	if err := s.SyncTunnelsToGroup(tunnel.EntryGroupID); err != nil {
		logger.Error("同步入口组失败", zap.Error(err))
	}

	// 同步到出口组
	if err := s.SyncTunnelsToGroup(tunnel.ExitGroupID); err != nil {
		logger.Error("同步出口组失败", zap.Error(err))
	}

	return nil
}
