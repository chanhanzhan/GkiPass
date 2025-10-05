package ws

import (
	"encoding/json"
	"time"

	"gkipass/plane/db"
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/modules/node"
	"gkipass/plane/internal/service"
	"gkipass/plane/pkg/logger"

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
	planService := service.NewPlanService(dbManager)
	return &Handler{
		manager:       manager,
		db:            dbManager,
		tunnelService: service.NewTunnelService(dbManager, planService),
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

	conn.Conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.Conn.SetPongHandler(func(string) error {
		conn.Conn.SetReadDeadline(time.Now().Add(90 * time.Second))
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
			conn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
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
			conn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
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

// syncRulesToNode 同步规则到节点
func (h *Handler) syncRulesToNode(nodeID, groupID, nodeType string) {
	logger.Info("同步隧道规则到节点",
		zap.String("nodeID", nodeID),
		zap.String("groupID", groupID))

	// 获取该节点组的所有隧道
	tunnels, err := h.tunnelService.GetTunnelsByGroupID(groupID, nodeType)
	if err != nil {
		logger.Error("获取隧道规则失败", zap.Error(err))
		return
	}

	// 转换为规则格式
	rules := make([]TunnelRule, 0, len(tunnels))
	for _, tunnel := range tunnels {
		// 解析目标列表
		targets, err := service.ParseTargetsFromString(tunnel.Targets)
		if err != nil {
			logger.Warn("解析目标列表失败",
				zap.String("tunnelID", tunnel.ID),
				zap.Error(err))
			continue
		}

		// 转换目标格式
		wsTargets := make([]TunnelTarget, len(targets))
		for i, t := range targets {
			wsTargets[i] = TunnelTarget{
				Host:   t.Host,
				Port:   t.Port,
				Weight: t.Weight,
			}
		}

		rule := TunnelRule{
			TunnelID:  tunnel.ID,
			Name:      tunnel.Name,
			Protocol:  tunnel.Protocol,
			LocalPort: tunnel.LocalPort,
			Targets:   wsTargets,
			Enabled:   tunnel.Enabled,
			UserID:    tunnel.UserID,
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
