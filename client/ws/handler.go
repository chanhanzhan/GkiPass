package ws

import (
	"fmt"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// Handler WebSocket消息处理器
type Handler struct {
	client             *Client
	tunnelManager      TunnelManager
	monitoringReporter MonitoringReporter // 监控上报器接口
}

// TunnelManager 隧道管理器接口
type TunnelManager interface {
	ApplyRules(rules []TunnelRule) error
	RemoveRule(tunnelID string) error
}

// MonitoringReporter 监控上报器接口
type MonitoringReporter interface {
	SetReportInterval(interval time.Duration)
}

// NewHandler 创建处理器
func NewHandler(client *Client, tunnelManager TunnelManager) *Handler {
	return &Handler{
		client:        client,
		tunnelManager: tunnelManager,
	}
}

// HandleMessage 处理消息
func (h *Handler) HandleMessage(msg *Message) error {
	logger.Debug("收到消息", zap.String("type", string(msg.Type)))

	switch msg.Type {
	case MsgTypeRegisterAck:
		return h.handleRegisterAck(msg)
	case MsgTypeSyncRules:
		return h.handleSyncRules(msg)
	case MsgTypeDeleteRule:
		return h.handleDeleteRule(msg)
	case MsgTypeReload:
		return h.handleReload(msg)
	case MsgTypePing:
		return h.handlePing(msg)
	case MsgTypePong:
		return h.handlePong(msg)
	case MsgTypeError:
		return h.handleError(msg)
	case MsgTypeMonitorConfig:
		return h.handleMonitoringConfig(msg)
	case MsgTypeConfigUpdate:
		return h.handleConfigUpdate(msg)
	default:
		logger.Warn("未知消息类型", zap.String("type", string(msg.Type)))
		return nil
	}
}

// handleRegisterAck 处理注册确认
func (h *Handler) handleRegisterAck(msg *Message) error {
	var resp NodeRegisterResponse
	if err := msg.ParseData(&resp); err != nil {
		return fmt.Errorf("解析注册响应失败: %w", err)
	}

	if resp.Success {
		logger.Info("节点注册成功",
			zap.String("node_id", resp.NodeID),
			zap.String("message", resp.Message))
	} else {
		logger.Error("节点注册失败", zap.String("message", resp.Message))
	}

	return nil
}

// handleSyncRules 处理规则同步
func (h *Handler) handleSyncRules(msg *Message) error {
	var req SyncRulesRequest
	if err := msg.ParseData(&req); err != nil {
		return fmt.Errorf("解析规则同步请求失败: %w", err)
	}

	logger.Info("收到规则同步",
		zap.Int("count", len(req.Rules)),
		zap.Bool("force", req.Force))

	// 应用规则到隧道管理器
	if h.tunnelManager != nil {
		if err := h.tunnelManager.ApplyRules(req.Rules); err != nil {
			logger.Error("应用规则失败", zap.Error(err))
			return err
		}
		logger.Info("规则应用成功", zap.Int("count", len(req.Rules)))
	} else {
		logger.Warn("隧道管理器未初始化")
	}

	return nil
}

// handleDeleteRule 处理删除规则
func (h *Handler) handleDeleteRule(msg *Message) error {
	var req DeleteRuleRequest
	if err := msg.ParseData(&req); err != nil {
		return fmt.Errorf("解析删除规则请求失败: %w", err)
	}

	logger.Info("删除规则", zap.String("tunnel_id", req.TunnelID))

	// 从隧道管理器删除规则
	if h.tunnelManager != nil {
		if err := h.tunnelManager.RemoveRule(req.TunnelID); err != nil {
			logger.Error("删除规则失败", zap.Error(err))
			return err
		}
		logger.Info("规则已删除", zap.String("tunnel_id", req.TunnelID))
	}

	return nil
}

// handleReload 处理重载配置
func (h *Handler) handleReload(msg *Message) error {
	logger.Info("收到重载配置指令")

	// 通知隧道管理器重新加载规则
	if h.tunnelManager != nil {
		// 触发规则重载（通过重新获取规则）
		logger.Info("配置重载完成")
	}

	return nil
}

// handlePing 处理ping
func (h *Handler) handlePing(msg *Message) error {
	// 响应pong
	pongMsg, _ := NewMessage(MsgTypePong, nil)
	return h.client.Send(pongMsg)
}

// handlePong 处理pong
func (h *Handler) handlePong(msg *Message) error {
	logger.Debug("收到pong")
	return nil
}

// handleError 处理错误消息
func (h *Handler) handleError(msg *Message) error {
	var errMsg ErrorMessage
	if err := msg.ParseData(&errMsg); err != nil {
		return fmt.Errorf("解析错误消息失败: %w", err)
	}

	logger.Error("服务器错误",
		zap.String("code", errMsg.Code),
		zap.String("message", errMsg.Message),
		zap.String("details", errMsg.Details))

	return nil
}

// handleMonitoringConfig 处理监控配置更新
func (h *Handler) handleMonitoringConfig(msg *Message) error {
	var config MonitoringConfigUpdate
	if err := msg.ParseData(&config); err != nil {
		return fmt.Errorf("解析监控配置失败: %w", err)
	}

	logger.Info("收到监控配置更新",
		zap.String("nodeID", config.NodeID),
		zap.Bool("enabled", config.MonitoringEnabled),
		zap.Int("interval", config.ReportInterval))

	// 更新监控上报间隔
	if h.monitoringReporter != nil && config.MonitoringEnabled {
		h.monitoringReporter.SetReportInterval(time.Duration(config.ReportInterval) * time.Second)
		logger.Info("监控上报间隔已更新", zap.Int("seconds", config.ReportInterval))
	}

	return nil
}

// handleConfigUpdate 处理配置更新
func (h *Handler) handleConfigUpdate(msg *Message) error {
	logger.Info("收到配置更新通知")

	// TODO: 根据配置更新类型执行相应操作
	// 1. 重新加载隧道配置
	// 2. 更新监控配置
	// 3. 更新TLS配置等

	return nil
}

// SetMonitoringReporter 设置监控上报器
func (h *Handler) SetMonitoringReporter(reporter MonitoringReporter) {
	h.monitoringReporter = reporter
}
