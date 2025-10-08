package user

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type PaymentHandler struct {
	app *types.App
}

func NewPaymentHandler(app *types.App) *PaymentHandler {
	return &PaymentHandler{app: app}
}

// CreateRechargeOrderRequest 创建充值订单请求
type CreateRechargeOrderRequest struct {
	Amount        float64 `json:"amount" binding:"required,min=10"`  // 充值金额，最低10元
	PaymentMethod string  `json:"payment_method" binding:"required"` // alipay/wechat/crypto
}

// CreateRechargeOrder 创建充值订单
func (h *PaymentHandler) CreateRechargeOrder(c *gin.Context) {
	var req CreateRechargeOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")

	// 验证支付方式
	validMethods := map[string]bool{
		"alipay": true,
		"wechat": true,
		"crypto": true,
		"usdt":   true,
		"manual": false, // 手动充值仅管理员可用
	}
	if !validMethods[req.PaymentMethod] {
		response.BadRequest(c, "Invalid payment method")
		return
	}

	// 获取用户钱包
	wallet, err := h.app.DB.DB.SQLite.GetWalletByUserID(userID.(string))
	if err != nil || wallet == nil {
		response.BadRequest(c, "Wallet not found")
		return
	}

	// 创建充值订单
	orderID := uuid.New().String()
	transaction := &dbinit.WalletTransaction{
		ID:            orderID,
		WalletID:      wallet.ID,
		UserID:        userID.(string),
		Type:          "recharge",
		Amount:        req.Amount,
		Balance:       wallet.Balance, // 当前余额，待支付成功后更新
		RelatedID:     "",
		RelatedType:   "recharge",
		Status:        "pending",
		PaymentMethod: req.PaymentMethod,
		Description:   fmt.Sprintf("充值 %.2f 元", req.Amount),
		CreatedAt:     time.Now(),
	}

	if err := h.app.DB.DB.SQLite.CreateWalletTransaction(transaction); err != nil {
		logger.Error("创建充值订单失败", zap.Error(err))
		response.InternalError(c, "Failed to create recharge order")
		return
	}

	// 生成支付参数
	var paymentData interface{}
	switch req.PaymentMethod {
	case "alipay":
		paymentData = h.generateAlipayParams(orderID, req.Amount)
	case "wechat":
		paymentData = h.generateWechatParams(orderID, req.Amount)
	case "crypto", "usdt":
		paymentData = h.generateCryptoParams(orderID, req.Amount)
	}

	logger.Info("创建充值订单",
		zap.String("orderID", orderID),
		zap.String("userID", userID.(string)),
		zap.Float64("amount", req.Amount),
		zap.String("method", req.PaymentMethod))

	response.Success(c, gin.H{
		"order_id":       orderID,
		"amount":         req.Amount,
		"payment_method": req.PaymentMethod,
		"status":         "pending",
		"payment_data":   paymentData,
		"created_at":     transaction.CreatedAt,
	})
}

// QueryOrderStatus 查询订单状态
func (h *PaymentHandler) QueryOrderStatus(c *gin.Context) {
	orderID := c.Param("id")
	userID, _ := c.Get("user_id")

	// 查询订单
	var transaction dbinit.WalletTransaction
	query := `SELECT * FROM wallet_transactions WHERE id = ? AND user_id = ?`
	err := h.app.DB.DB.SQLite.Get().QueryRow(query, orderID, userID).Scan(
		&transaction.ID, &transaction.WalletID, &transaction.UserID,
		&transaction.Type, &transaction.Amount, &transaction.Balance,
		&transaction.RelatedID, &transaction.RelatedType,
		&transaction.Status, &transaction.PaymentMethod,
		&transaction.Description, &transaction.CreatedAt,
	)
	if err != nil {
		response.NotFound(c, "Order not found")
		return
	}

	response.Success(c, transaction)
}

// PaymentCallback 支付回调
func (h *PaymentHandler) PaymentCallback(c *gin.Context) {
	var req struct {
		OrderID       string  `json:"order_id" binding:"required"`
		PaymentMethod string  `json:"payment_method" binding:"required"`
		Amount        float64 `json:"amount" binding:"required"`
		TransactionID string  `json:"transaction_id"` // 第三方交易号
		Sign          string  `json:"sign"`           // 签名
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 验证签名
	if !h.verifyPaymentSign(req.OrderID, req.PaymentMethod, req.Amount, req.TransactionID, req.Sign) {
		response.BadRequest(c, "Invalid signature")
		return
	}

	// 查询订单
	var transaction dbinit.WalletTransaction
	query := `SELECT * FROM wallet_transactions WHERE id = ?`
	err := h.app.DB.DB.SQLite.Get().QueryRow(query, req.OrderID).Scan(
		&transaction.ID, &transaction.WalletID, &transaction.UserID,
		&transaction.Type, &transaction.Amount, &transaction.Balance,
		&transaction.RelatedID, &transaction.RelatedType,
		&transaction.Status, &transaction.PaymentMethod,
		&transaction.Description, &transaction.CreatedAt,
	)
	if err != nil {
		response.NotFound(c, "Order not found")
		return
	}

	// 检查订单状态
	if transaction.Status != "pending" {
		response.Success(c, gin.H{"message": "Order already processed"})
		return
	}

	// 验证金额
	if transaction.Amount != req.Amount {
		logger.Warn("充值金额不匹配",
			zap.String("orderID", req.OrderID),
			zap.Float64("expected", transaction.Amount),
			zap.Float64("actual", req.Amount))
		response.BadRequest(c, "Amount mismatch")
		return
	}

	// 获取钱包
	wallet, err := h.app.DB.DB.SQLite.GetWalletByUserID(transaction.UserID)
	if err != nil || wallet == nil {
		response.InternalError(c, "Wallet not found")
		return
	}

	// 更新钱包余额
	newBalance := wallet.Balance + req.Amount
	if err := h.app.DB.DB.SQLite.UpdateWalletBalance(wallet.ID, newBalance, wallet.Frozen); err != nil {
		logger.Error("更新钱包余额失败", zap.Error(err))
		response.InternalError(c, "Failed to update wallet balance")
		return
	}

	// 更新订单状态
	updateQuery := `UPDATE wallet_transactions 
		SET status = 'completed', balance = ?, related_id = ?
		WHERE id = ?`
	_, err = h.app.DB.DB.SQLite.Get().Exec(updateQuery,
		newBalance, req.TransactionID, req.OrderID)
	if err != nil {
		logger.Error("更新订单状态失败", zap.Error(err))
		response.InternalError(c, "Failed to update order status")
		return
	}

	// 如果是购买套餐，激活订阅
	if transaction.Type == "purchase" && transaction.RelatedType == "plan" {
		// Import the handler package for accessing PlanHandler from main handler package
		// This requires implementing the ActivateSubscription in the service layer instead
		// For now, just log this - this should be handled via service layer
		logger.Info("套餐购买成功，需要激活订阅",
			zap.String("orderID", req.OrderID),
			zap.String("userID", transaction.UserID),
			zap.String("planID", transaction.RelatedID))
	}

	logger.Info("支付成功",
		zap.String("orderID", req.OrderID),
		zap.String("userID", transaction.UserID),
		zap.String("type", transaction.Type),
		zap.Float64("amount", req.Amount),
		zap.Float64("newBalance", newBalance))

	response.Success(c, gin.H{
		"message":     "Payment successful",
		"order_id":    req.OrderID,
		"type":        transaction.Type,
		"new_balance": newBalance,
	})
}

// ManualRecharge 管理员手动充值
func (h *PaymentHandler) ManualRecharge(c *gin.Context) {
	var req struct {
		UserID      string  `json:"user_id" binding:"required"`
		Amount      float64 `json:"amount" binding:"required"`
		Description string  `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	adminID, _ := c.Get("user_id")

	// 获取用户钱包
	wallet, err := h.app.DB.DB.SQLite.GetWalletByUserID(req.UserID)
	if err != nil || wallet == nil {
		response.BadRequest(c, "User wallet not found")
		return
	}

	// 更新余额
	newBalance := wallet.Balance + req.Amount
	if err := h.app.DB.DB.SQLite.UpdateWalletBalance(wallet.ID, newBalance, wallet.Frozen); err != nil {
		logger.Error("更新钱包余额失败", zap.Error(err))
		response.InternalError(c, "Failed to update wallet balance")
		return
	}

	// 记录交易
	description := req.Description
	if description == "" {
		description = fmt.Sprintf("管理员充值 %.2f 元", req.Amount)
	}
	transaction := &dbinit.WalletTransaction{
		ID:            uuid.New().String(),
		WalletID:      wallet.ID,
		UserID:        req.UserID,
		Type:          "recharge",
		Amount:        req.Amount,
		Balance:       newBalance,
		RelatedID:     adminID.(string),
		RelatedType:   "manual",
		Status:        "completed",
		PaymentMethod: "admin",
		Description:   description,
		CreatedAt:     time.Now(),
	}

	if err := h.app.DB.DB.SQLite.CreateWalletTransaction(transaction); err != nil {
		logger.Warn("记录交易失败", zap.Error(err))
	}

	logger.Info("管理员手动充值",
		zap.String("adminID", adminID.(string)),
		zap.String("userID", req.UserID),
		zap.Float64("amount", req.Amount),
		zap.Float64("newBalance", newBalance))

	response.Success(c, gin.H{
		"user_id":     req.UserID,
		"amount":      req.Amount,
		"new_balance": newBalance,
	})
}

// 生成支付宝支付参数
func (h *PaymentHandler) generateAlipayParams(orderID string, amount float64) map[string]string {
	return map[string]string{
		"type":        "alipay",
		"order_id":    orderID,
		"amount":      fmt.Sprintf("%.2f", amount),
		"qr_code":     fmt.Sprintf("alipay://pay?order_id=%s&amount=%.2f", orderID, amount),
		"payment_url": fmt.Sprintf("/payment/alipay?order_id=%s", orderID),
		"expires_in":  "900", // 15分钟
	}
}

// 生成微信支付参数
func (h *PaymentHandler) generateWechatParams(orderID string, amount float64) map[string]string {
	return map[string]string{
		"type":        "wechat",
		"order_id":    orderID,
		"amount":      fmt.Sprintf("%.2f", amount),
		"qr_code":     fmt.Sprintf("weixin://wxpay/bizpayurl?order_id=%s&amount=%.2f", orderID, amount),
		"payment_url": fmt.Sprintf("/payment/wechat?order_id=%s", orderID),
		"expires_in":  "900",
	}
}

// 生成加密货币支付参数
func (h *PaymentHandler) generateCryptoParams(orderID string, amount float64) map[string]string {
	// 生成唯一的USDT-TRC20地址（示例）
	address := h.generateCryptoAddress(orderID)

	return map[string]string{
		"type":        "crypto",
		"order_id":    orderID,
		"amount":      fmt.Sprintf("%.2f", amount),
		"crypto":      "USDT-TRC20",
		"address":     address,
		"rate":        "7.2", // 汇率: 1 USDT = 7.2 CNY
		"usdt_amount": fmt.Sprintf("%.2f", amount/7.2),
		"expires_in":  "1800", // 30分钟
	}
}

// 生成加密货币地址（示例）
func (h *PaymentHandler) generateCryptoAddress(orderID string) string {
	hash := md5.Sum([]byte(orderID + "gkipass_secret"))
	return "T" + hex.EncodeToString(hash[:])[:33] // TRC20地址格式
}

// verifyPaymentSign 验证支付回调签名
func (h *PaymentHandler) verifyPaymentSign(orderID, method string, amount float64, transactionID, sign string) bool {
	// 构造签名字符串
	signStr := fmt.Sprintf("%s|%s|%.2f|%s|%s", orderID, method, amount, transactionID, "gkipass_payment_secret")

	// 计算MD5签名
	hash := md5.Sum([]byte(signStr))
	expectedSign := hex.EncodeToString(hash[:])

	// 比较签名
	return sign == expectedSign
}
