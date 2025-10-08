package handler

import (
	"time"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/auth"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CKHandler Connection Key 处理器
type CKHandler struct {
	app *types.App
}

// NewCKHandler 创建 CK 处理器
func NewCKHandler(app *types.App) *CKHandler {
	return &CKHandler{app: app}
}

// GenerateNodeCK 为节点生成 CK
func (h *CKHandler) GenerateNodeCK(c *gin.Context) {
	nodeID := c.Param("id")

	// 验证节点存在
	node, err := h.app.DB.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 检查权限（管理员或节点所有者）
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	if role != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	// 生成 CK（30天有效期）
	ck := auth.CreateNodeCK(nodeID, 30*24*time.Hour)

	// 保存到数据库
	if err := h.app.DB.DB.SQLite.CreateConnectionKey(ck); err != nil {
		logger.Error("创建CK失败", zap.Error(err))
		response.InternalError(c, "Failed to create connection key")
		return
	}

	logger.Info("为节点生成CK",
		zap.String("nodeID", nodeID),
		zap.String("ckID", ck.ID))

	response.SuccessWithMessage(c, "Connection key created successfully", gin.H{
		"connection_key": ck.Key,
		"node_id":        ck.NodeID,
		"expires_at":     ck.ExpiresAt,
		"usage":          "使用方法: ./client --token " + ck.Key,
	})
}

// ListNodeCKs 列出节点的所有 CK
func (h *CKHandler) ListNodeCKs(c *gin.Context) {
	nodeID := c.Param("id")

	// 验证节点存在
	node, err := h.app.DB.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 检查权限
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	if role != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	// 获取CK列表
	cks, err := h.app.DB.DB.SQLite.GetConnectionKeysByNodeID(nodeID)
	if err != nil {
		response.InternalError(c, "Failed to list connection keys")
		return
	}

	// 隐藏完整 key，只显示前10位
	for _, ck := range cks {
		if len(ck.Key) > 10 {
			ck.Key = ck.Key[:10] + "..." + ck.Key[len(ck.Key)-4:]
		}
	}

	response.Success(c, cks)
}

// RevokeCK 撤销 CK
func (h *CKHandler) RevokeCK(c *gin.Context) {
	ckID := c.Param("ck_id")

	if err := h.app.DB.DB.SQLite.DeleteConnectionKey(ckID); err != nil {
		response.InternalError(c, "Failed to revoke connection key")
		return
	}

	logger.Info("CK已撤销", zap.String("ckID", ckID))
	response.SuccessWithMessage(c, "Connection key revoked successfully", nil)
}
