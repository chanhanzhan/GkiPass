package ws

import (
	"encoding/json"
	"time"

	"gkipass/plane/db"
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/modules/node"
	"gkipass/plane/internal/service"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Handler WebSocket 消息处理器
type Handler struct {
	manager       *Manager
	db            *db.Manager
	tunnelService *service.TunnelService
	nodeManager   *node.Manager
}

// NewHandler 创建处理器
func NewHandler(manager *Manager, dbManager *db.Manager) *Handler {
	// TODO: Pass correct services when available
	return &Handler{
		manager:       manager,
		db:            dbManager,
		tunnelService: service.NewTunnelService(nil, nil, nil, nil),
		nodeManager:   node.NewManager(dbManager),
	}
}

// HandleConnection 处理新连接
func (h *Handler) HandleConnection(conn *websocket.Conn) {
	var msg Message
	if err := conn.ReadJSON(&msg); err != nil {
		logger.Error("读取注册消息失败", zap.Error(err))
		conn.Close()
		return
	}

	if msg.Type != MsgTypeNodeRegister {
		h.sendError(conn, "INVALID_FIRST_MESSAGE", "首条消息必须是注册消息")
		conn.Close()
		return
	}
	var req NodeRegisterRequest
	if err := msg.ParseData(&req); err != nil {
		h.sendError(conn, "INVALID_REQUEST", "无效的注册请求")
		conn.Close()
		return
	}

	// 验证CK
	if !h.validateCK(req.CK, req.NodeID) {
		h.sendError(conn, "AUTH_FAILED", "CK验证失败")
		conn.Close()
		return
	}

	// 更新节点信息
	if err := h.updateNodeInfo(&req); err != nil {
		h.sendError(conn, "UPDATE_FAILED", "更新节点信息失败")
		conn.Close()
		return
	}

	nodeConn := &NodeConnection{
		NodeID:   req.NodeID,
		Conn:     conn,
		Send:     make(chan *Message, 256),
		LastSeen: time.Now(),
		IsAlive:  true,
	}
	h.manager.register <- nodeConn
	h.sendRegisterAck(nodeConn, true, "注册成功")
	go h.sendFullNodeConfig(req.NodeID)
	go h.readPump(nodeConn)
	go h.writePump(nodeConn)
}
func (h *Handler) readPump(conn *NodeConnection) {
	defer func() {
		h.manager.unregister <- conn
		conn.Close()
	}()

	if err := conn.Conn.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
		logger.Error("设置读取超时失败", zap.Error(err))
		return
	}
	conn.Conn.SetPongHandler(func(string) error {
		if err := conn.Conn.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
			logger.Error("设置读取超时失败", zap.Error(err))
		}
		conn.UpdateLastSeen()
		return nil
	})

	for {
		var msg Message
		err := conn.Conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error("WebSocket 读取错误",
					zap.String("nodeID", conn.NodeID),
					zap.Error(err))
			}
			break
		}

		conn.UpdateLastSeen()
		h.handleMessage(conn, &msg)
	}
}

// writePump 发送消息
func (h *Handler) writePump(conn *NodeConnection) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case msg, ok := <-conn.Send:
			if err := conn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				logger.Error("设置写入超时失败", zap.Error(err))
				return
			}
			if !ok {
				conn.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.Conn.WriteJSON(msg); err != nil {
				logger.Error("WebSocket 写入错误",
					zap.String("nodeID", conn.NodeID),
					zap.Error(err))
				return
			}

		case <-ticker.C:
			// 发送 Ping
			if err := conn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				logger.Error("设置写入超时失败", zap.Error(err))
				return
			}
			if err := conn.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage 处理消息
func (h *Handler) handleMessage(conn *NodeConnection, msg *Message) {
	logger.Debug("收到消息",
		zap.String("nodeID", conn.NodeID),
		zap.String("type", string(msg.Type)))

	switch msg.Type {
	case MsgTypeHeartbeat:
		h.handleHeartbeat(conn, msg)

	case MsgTypeTrafficReport:
		h.handleTrafficReport(conn, msg)

	case MsgTypeNodeStatus:
		h.handleNodeStatus(conn, msg)

	case MsgTypeMonitoringReport:
		h.handleMonitoringReport(conn, msg)

	case MsgTypePong:
		// Pong 消息已在 readPump 中处理

	default:
		logger.Warn("未知消息类型",
			zap.String("nodeID", conn.NodeID),
			zap.String("type", string(msg.Type)))
	}
}

// handleHeartbeat 处理心跳
func (h *Handler) handleHeartbeat(conn *NodeConnection, msg *Message) {
	var req HeartbeatRequest
	if err := msg.ParseData(&req); err != nil {
		logger.Error("解析心跳请求失败", zap.Error(err))
		return
	}

	// 更新节点状态到数据库
	node, err := h.db.DB.SQLite.GetNode(req.NodeID)
	if err != nil || node == nil {
		return
	}

	node.Status = req.Status
	node.LastSeen = time.Now()
	h.db.DB.SQLite.UpdateNode(node)

	// 发送心跳响应
	resp := &HeartbeatResponse{
		Success:   true,
		Timestamp: time.Now(),
	}

	respMsg, _ := NewMessage(MsgTypeHeartbeat, resp)
	conn.Send <- respMsg
}

// handleTrafficReport 处理流量上报
func (h *Handler) handleTrafficReport(conn *NodeConnection, msg *Message) {
	var req TrafficReportRequest
	if err := msg.ParseData(&req); err != nil {
		logger.Error("解析流量上报失败", zap.Error(err))
		return
	}

	// 更新隧道流量统计
	if err := h.tunnelService.UpdateTraffic(req.TunnelID, req.TrafficIn, req.TrafficOut); err != nil {
		logger.Error("更新流量统计失败",
			zap.String("tunnelID", req.TunnelID),
			zap.Error(err))
	}

	// 发送响应
	resp := &TrafficReportResponse{
		Success: true,
	}

	respMsg, _ := NewMessage(MsgTypeTrafficReport, resp)
	conn.Send <- respMsg
}

// handleNodeStatus 处理节点状态
func (h *Handler) handleNodeStatus(conn *NodeConnection, msg *Message) {
}

// handleMonitoringReport 处理监控数据上报
func (h *Handler) handleMonitoringReport(conn *NodeConnection, msg *Message) {
	var req MonitoringReportRequest
	if err := msg.ParseData(&req); err != nil {
		logger.Error("解析监控数据失败", zap.Error(err))
		return
	}

	logger.Debug("收到监控数据",
		zap.String("nodeID", req.NodeID),
		zap.Float64("cpuUsage", req.SystemInfo.CPUUsage),
		zap.Float64("memUsage", req.SystemInfo.MemoryUsagePercent),
		zap.Int("tunnels", req.TunnelStats.ActiveTunnels))

	// 创建监控服务实例（如果不存在）
	monitoringService := service.NewNodeMonitoringService(h.db)

	// 转换数据结构
	reportData := &service.NodeMonitoringReportData{
		SystemInfo: service.SystemInfo{
			Uptime:             req.SystemInfo.Uptime,
			BootTime:           req.SystemInfo.BootTime,
			CPUUsage:           req.SystemInfo.CPUUsage,
			CPULoad1m:          req.SystemInfo.CPULoad1m,
			CPULoad5m:          req.SystemInfo.CPULoad5m,
			CPULoad15m:         req.SystemInfo.CPULoad15m,
			CPUCores:           req.SystemInfo.CPUCores,
			MemoryTotal:        req.SystemInfo.MemoryTotal,
			MemoryUsed:         req.SystemInfo.MemoryUsed,
			MemoryAvailable:    req.SystemInfo.MemoryAvailable,
			MemoryUsagePercent: req.SystemInfo.MemoryUsagePercent,
			DiskTotal:          req.SystemInfo.DiskTotal,
			DiskUsed:           req.SystemInfo.DiskUsed,
			DiskAvailable:      req.SystemInfo.DiskAvailable,
			DiskUsagePercent:   req.SystemInfo.DiskUsagePercent,
		},
		NetworkStats: service.NetworkStats{
			Interfaces:       convertNetworkInterfaces(req.NetworkStats.Interfaces),
			BandwidthIn:      req.NetworkStats.BandwidthIn,
			BandwidthOut:     req.NetworkStats.BandwidthOut,
			TCPConnections:   req.NetworkStats.TCPConnections,
			UDPConnections:   req.NetworkStats.UDPConnections,
			TotalConnections: req.NetworkStats.TotalConnections,
			TrafficInBytes:   req.NetworkStats.TrafficInBytes,
			TrafficOutBytes:  req.NetworkStats.TrafficOutBytes,
			PacketsIn:        req.NetworkStats.PacketsIn,
			PacketsOut:       req.NetworkStats.PacketsOut,
			ConnectionErrors: req.NetworkStats.ConnectionErrors,
		},
		TunnelStats: service.TunnelStats{
			ActiveTunnels: req.TunnelStats.ActiveTunnels,
			TunnelErrors:  req.TunnelStats.TunnelErrors,
			TunnelList:    convertTunnelStatusList(req.TunnelStats.TunnelList),
		},
		Performance: service.Performance{
			AvgResponseTime: req.Performance.AvgResponseTime,
			MaxResponseTime: req.Performance.MaxResponseTime,
			MinResponseTime: req.Performance.MinResponseTime,
			RequestsPerSec:  req.Performance.RequestsPerSec,
			ErrorRate:       req.Performance.ErrorRate,
		},
		AppInfo: service.AppInfo{
			Version:      req.AppInfo.Version,
			GoVersion:    req.AppInfo.GoVersion,
			OSInfo:       req.AppInfo.OSInfo,
			Architecture: req.AppInfo.Architecture,
			StartTime:    req.AppInfo.StartTime,
		},
		ConfigVersion: req.ConfigVersion,
	}

	// 处理监控数据
	if err := monitoringService.ReportMonitoringData(req.NodeID, reportData); err != nil {
		logger.Error("处理监控数据失败",
			zap.String("nodeID", req.NodeID),
			zap.Error(err))
		return
	}

	// 发送确认响应
	resp := gin.H{
		"success":   true,
		"timestamp": time.Now(),
		"message":   "监控数据已接收",
	}

	respMsg, _ := NewMessage("monitoring_ack", resp)
	conn.Send <- respMsg
}

// convertNetworkInterfaces 转换网络接口数据结构
func convertNetworkInterfaces(interfaces []NetworkInterface) []service.NetworkInterface {
	result := make([]service.NetworkInterface, len(interfaces))
	for i, iface := range interfaces {
		result[i] = service.NetworkInterface{
			Name:      iface.Name,
			IP:        iface.IP,
			MAC:       iface.MAC,
			Status:    iface.Status,
			Speed:     iface.Speed,
			RxBytes:   iface.RxBytes,
			TxBytes:   iface.TxBytes,
			RxPackets: iface.RxPackets,
			TxPackets: iface.TxPackets,
		}
	}
	return result
}

// convertTunnelStatusList 转换隧道状态列表
func convertTunnelStatusList(tunnels []TunnelStatusInfo) []service.TunnelStatusInfo {
	result := make([]service.TunnelStatusInfo, len(tunnels))
	for i, tunnel := range tunnels {
		result[i] = service.TunnelStatusInfo{
			TunnelID:        tunnel.TunnelID,
			Name:            tunnel.Name,
			Protocol:        tunnel.Protocol,
			LocalPort:       tunnel.LocalPort,
			Status:          tunnel.Status,
			Connections:     tunnel.Connections,
			TrafficIn:       tunnel.TrafficIn,
			TrafficOut:      tunnel.TrafficOut,
			AvgResponseTime: tunnel.AvgResponseTime,
			ErrorCount:      tunnel.ErrorCount,
		}
	}
	return result
}

// syncRulesToNode 同步规则到节点
func (h *Handler) syncRulesToNode(nodeID, groupID, nodeType string) {
	logger.Info("同步隧道规则到节点",
		zap.String("nodeID", nodeID),
		zap.String("groupID", groupID))

	// 获取该节点组的所有隧道
	tunnels, err := h.tunnelService.GetTunnelsByGroupID(groupID)
	if err != nil {
		logger.Error("获取隧道规则失败", zap.Error(err))
		return
	}

	// 转换为规则格式
	rules := make([]TunnelRule, 0, len(tunnels))
	for _, tunnel := range tunnels {
		// 构建单个目标
		wsTargets := []TunnelTarget{
			{
				Host:   tunnel.TargetAddress,
				Port:   tunnel.TargetPort,
				Weight: 1,
			},
		}

		// 根据节点类型选择协议
		protocol := tunnel.IngressProtocol
		if nodeType == "egress" || nodeType == "exit" {
			protocol = tunnel.EgressProtocol
		}

		rule := TunnelRule{
			TunnelID:  tunnel.ID,
			Name:      tunnel.Name,
			Protocol:  protocol,
			LocalPort: tunnel.ListenPort,
			Targets:   wsTargets,
			Enabled:   tunnel.Enabled,
			UserID:    tunnel.CreatedBy,
		}

		rules = append(rules, rule)
	}

	// 发送同步请求
	syncReq := &SyncRulesRequest{
		Rules: rules,
		Force: true,
	}

	msg, err := NewMessage(MsgTypeSyncRules, syncReq)
	if err != nil {
		logger.Error("创建同步消息失败", zap.Error(err))
		return
	}

	if err := h.manager.SendToNode(nodeID, msg); err != nil {
		logger.Error("发送同步消息失败",
			zap.String("nodeID", nodeID),
			zap.Error(err))
	}
}

// validateCK 验证 Connection Key
func (h *Handler) validateCK(ck, nodeID string) bool {
	// 1. 验证 CK 格式
	if len(ck) < 10 {
		return false
	}

	// 2. 从数据库查询 CK
	ckRecord, err := h.db.DB.SQLite.GetConnectionKeyByKey(ck)
	if err != nil {
		logger.Error("查询CK失败", zap.Error(err))
		return false
	}

	if ckRecord == nil {
		logger.Warn("CK不存在", zap.String("ck", ck[:10]+"..."))
		return false
	}

	// 3. 检查是否过期
	if time.Now().After(ckRecord.ExpiresAt) {
		logger.Warn("CK已过期", zap.String("nodeID", nodeID))
		return false
	}

	// 4. 验证节点ID是否匹配
	if ckRecord.NodeID != nodeID {
		logger.Warn("CK与节点ID不匹配",
			zap.String("ckNodeID", ckRecord.NodeID),
			zap.String("requestNodeID", nodeID))
		return false
	}

	// 5. 验证类型
	if ckRecord.Type != "node" {
		logger.Warn("CK类型错误", zap.String("type", ckRecord.Type))
		return false
	}

	logger.Info("CK验证通过",
		zap.String("nodeID", nodeID),
		zap.String("ck", ck[:10]+"..."))

	return true
}

// updateNodeInfo 更新节点信息
func (h *Handler) updateNodeInfo(req *NodeRegisterRequest) error {
	node, err := h.db.DB.SQLite.GetNode(req.NodeID)
	if err != nil {
		return err
	}

	if node == nil {
		// 节点不存在，返回错误
		return &WSError{Code: "NODE_NOT_FOUND", Message: "节点不存在"}
	}

	// 更新节点信息
	node.Name = req.NodeName
	node.Status = "online"
	node.IP = req.IP
	node.Port = req.Port
	node.Version = req.Version
	node.LastSeen = time.Now()

	return h.db.DB.SQLite.UpdateNode(node)
}

// sendRegisterAck 发送注册确认
func (h *Handler) sendRegisterAck(conn *NodeConnection, success bool, message string) {
	resp := &NodeRegisterResponse{
		Success: success,
		Message: message,
		NodeID:  conn.NodeID,
	}

	msg, _ := NewMessage(MsgTypeRegisterAck, resp)
	conn.Send <- msg
}

// sendError 发送错误消息
func (h *Handler) sendError(conn *websocket.Conn, code, message string) {
	errMsg := &ErrorMessage{
		Code:    code,
		Message: message,
	}

	msg, _ := NewMessage(MsgTypeError, errMsg)
	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)
}

// NotifyRuleChange 通知规则变更（外部调用）
func (h *Handler) NotifyRuleChange(groupID, nodeType string, tunnel *dbinit.Tunnel) {
	logger.Info("通知规则变更",
		zap.String("groupID", groupID),
		zap.String("tunnelID", tunnel.ID))

	// 获取该组的所有在线节点
	nodeIDs := h.getOnlineNodesInGroup(groupID)

	// 通知组内所有节点
	for _, nodeID := range nodeIDs {
		go h.sendFullNodeConfig(nodeID)
	}

	logger.Info("规则变更已通知",
		zap.String("groupID", groupID),
		zap.Int("nodeCount", len(nodeIDs)))
}

// getOnlineNodesInGroup 获取组内所有在线节点ID
func (h *Handler) getOnlineNodesInGroup(groupID string) []string {
	// 从在线连接中筛选
	allNodeIDs := h.manager.GetAllNodeIDs()
	groupNodeIDs := make([]string, 0)

	for _, nodeID := range allNodeIDs {
		node, err := h.db.DB.SQLite.GetNode(nodeID)
		if err != nil || node == nil {
			continue
		}

		if node.GroupID == groupID {
			groupNodeIDs = append(groupNodeIDs, nodeID)
		}
	}

	return groupNodeIDs
}

// syncRulesToAllNodes 同步规则到所有节点
func (h *Handler) syncRulesToAllNodes() {
	nodeIDs := h.manager.GetAllNodeIDs()
	for _, nodeID := range nodeIDs {
		go h.sendFullNodeConfig(nodeID)
	}
}

// sendFullNodeConfig 发送完整节点配置
func (h *Handler) sendFullNodeConfig(nodeID string) {
	logger.Info("下发完整节点配置", zap.String("nodeID", nodeID))

	// 获取完整节点配置
	config, err := h.nodeManager.GetNodeConfig(nodeID)
	if err != nil {
		logger.Error("获取节点配置失败",
			zap.String("nodeID", nodeID),
			zap.Error(err))
		return
	}

	// 发送配置
	msg, err := NewMessage("full_config", config)
	if err != nil {
		logger.Error("创建配置消息失败", zap.Error(err))
		return
	}

	if err := h.manager.SendToNode(nodeID, msg); err != nil {
		logger.Error("发送配置失败",
			zap.String("nodeID", nodeID),
			zap.Error(err))
		return
	}

	logger.Info("节点配置已下发",
		zap.String("nodeID", nodeID),
		zap.Int("tunnelCount", len(config.Tunnels)),
		zap.Int("peerCount", len(config.PeerServers)))
}
