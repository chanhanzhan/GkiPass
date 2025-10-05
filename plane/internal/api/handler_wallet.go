package api

import (
	"strconv"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// WalletHandler 钱包处理器
type WalletHandler struct {
	app *App
}

// NewWalletHandler 创建钱包处理器
func NewWalletHandler(app *App) *WalletHandler {
	return &WalletHandler{app: app}
}

// GetBalance 获取余额
func (h *WalletHandler) GetBalance(c *gin.Context) {
	userID, _ := c.Get("user_id")

	wallet, err := h.app.DB.DB.SQLite.GetOrCreateWallet(userID.(string))
	if err != nil {
		logger.Error("获取钱包失败", zap.Error(err))
		response.InternalError(c, "Failed to get wallet")
		return
	}

	response.Success(c, gin.H{
		"balance": wallet.Balance,
		"frozen":  wallet.Frozen,
	})
}

// RechargeRequest 充值请求
type RechargeRequest struct {
	Amount float64 `json:"amount" binding:"required,gt=0"`
}

// Recharge 充值 当前为debug版本，后续完善实现
func (h *WalletHandler) Recharge(c *gin.Context) {
	var req RechargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")

	wallet, err := h.app.DB.DB.SQLite.GetOrCreateWallet(userID.(string))
	if err != nil {
		response.InternalError(c, "Failed to get wallet")
		return
	}

	// 更新余额
	newBalance := wallet.Balance + req.Amount
	if err := h.app.DB.DB.SQLite.UpdateWalletBalance(wallet.ID, newBalance, wallet.Frozen); err != nil {
		response.InternalError(c, "Failed to update balance")
		return
	}

	// 创建交易记录
	tx := &dbinit.WalletTransaction{
		ID:            uuid.New().String(),
		WalletID:      wallet.ID,
		UserID:        userID.(string),
		Type:          "recharge",
		Amount:        req.Amount,
		Balance:       newBalance,
		Status:        "completed",
		PaymentMethod: "admin",
		Description:   "充值",
	}
	if err := h.app.DB.DB.SQLite.CreateWalletTransaction(tx); err != nil {
		logger.Error("创建交易记录失败", zap.Error(err))
	}

	response.Success(c, gin.H{
		"balance":        newBalance,
		"transaction_id": tx.ID,
	})
}

// ListTransactions 获取交易记录
func (h *WalletHandler) ListTransactions(c *gin.Context) {
	userID, _ := c.Get("user_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	transactions, total, err := h.app.DB.DB.SQLite.ListWalletTransactions(userID.(string), page, limit)
	if err != nil {
		response.InternalError(c, "Failed to list transactions")
		return
	}

	response.Success(c, gin.H{
		"data":        transactions,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": (total + limit - 1) / limit,
	})
}
