package ws

import (
	"fmt"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// Handler WebSocket消息处理器
type Handler struct {
	client        *Client
	tunnelManager TunnelManager
}

// TunnelManager 隧道管理器接口
type TunnelManager interface {
	ApplyRules(rules []TunnelRule) error
	RemoveRule(tunnelID string) error
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

	// TODO: 重新加载配置文件

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
