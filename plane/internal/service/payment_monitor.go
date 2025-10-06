package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gkipass/plane/db"
	"gkipass/plane/db/sqlite"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// PaymentMonitorService 支付监听服务
type PaymentMonitorService struct {
	db     *db.DB
	ctx    context.Context
	cancel context.CancelFunc
}

// NewPaymentMonitorService 创建支付监听服务
func NewPaymentMonitorService(database *db.DB) *PaymentMonitorService {
	ctx, cancel := context.WithCancel(context.Background())
	return &PaymentMonitorService{
		db:     database,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动支付监听服务
func (s *PaymentMonitorService) Start() {
	logger.Info("支付监听服务启动")

	ticker := time.NewTicker(30 * time.Second) // 每30秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			logger.Info("支付监听服务停止")
			return
		case <-ticker.C:
			s.checkPendingPayments()
		}
	}
}

// Stop 停止支付监听服务
func (s *PaymentMonitorService) Stop() {
	s.cancel()
}

// checkPendingPayments 检查待确认的支付
func (s *PaymentMonitorService) checkPendingPayments() {
	monitors, err := s.db.SQLite.ListPendingMonitors()
	if err != nil {
		logger.Error("查询待监听订单失败", zap.Error(err))
		return
	}

	if len(monitors) == 0 {
		return
	}

	logger.Debug("检查待确认支付", zap.Int("count", len(monitors)))

	for _, monitor := range monitors {
		// 检查是否超时
		if time.Now().After(monitor.ExpiresAt) {
			s.handleTimeout(monitor)
			continue
		}

		// 根据支付类型检查
		switch monitor.PaymentType {
		case "crypto":
			s.checkCryptoPayment(monitor)
		case "epay":
			s.checkEpayPayment(monitor)
		}
	}
}

// checkCryptoPayment 检查加密货币支付
func (s *PaymentMonitorService) checkCryptoPayment(monitor *sqlite.PaymentMonitor) {
	// 获取加密货币配置
	config, err := s.db.SQLite.GetPaymentConfig("crypto_usdt")
	if err != nil || config == nil || !config.Enabled {
		return
	}

	var cryptoConfig struct {
		Network       string `json:"network"`
		Address       string `json:"address"`
		APIKey        string `json:"api_key"`
		CheckInterval int    `json:"check_interval"`
	}
	if err := json.Unmarshal([]byte(config.Config), &cryptoConfig); err != nil {
		logger.Error("解析加密货币配置失败", zap.Error(err))
		return
	}

	// 调用区块链浏览器API检查交易
	// 示例：TronScan API
	received, err := s.checkTRC20Balance(cryptoConfig.Address, cryptoConfig.APIKey)
	if err != nil {
		logger.Error("检查TRC20余额失败", zap.Error(err))
		return
	}

	// 检查是否收到足够金额
	if received >= monitor.ExpectedAmount {
		s.confirmPayment(monitor)
	}
}

// checkTRC20Balance 检查TRC20地址余额（示例）
func (s *PaymentMonitorService) checkTRC20Balance(address, apiKey string) (float64, error) {
	// 这里是示例实现，实际应该调用 TronGrid 或 TronScan API
	url := fmt.Sprintf("https://api.trongrid.io/v1/accounts/%s/transactions/trc20", address)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	if apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// 解析响应并计算收到的金额
	var result struct {
		Data []struct {
			Value string `json:"value"`
			Type  string `json:"type"`
			To    string `json:"to"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	// 这里需要根据实际API响应格式解析
	// 示例：简单返回0，实际需要累加所有收款
	return 0, nil
}

// checkEpayPayment 检查易支付订单
func (s *PaymentMonitorService) checkEpayPayment(monitor *sqlite.PaymentMonitor) {
	// 获取易支付配置
	config, err := s.db.SQLite.GetPaymentConfig("epay_default")
	if err != nil || config == nil || !config.Enabled {
		return
	}

	var epayConfig struct {
		APIURL      string `json:"api_url"`
		MerchantID  string `json:"merchant_id"`
		MerchantKey string `json:"merchant_key"`
	}
	if err := json.Unmarshal([]byte(config.Config), &epayConfig); err != nil {
		logger.Error("解析易支付配置失败", zap.Error(err))
		return
	}

	// 调用易支付查询接口
	status, err := s.queryEpayOrder(monitor.TransactionID, epayConfig)
	if err != nil {
		logger.Error("查询易支付订单失败", zap.Error(err))
		return
	}

	if status == "success" {
		s.confirmPayment(monitor)
	}
}

// queryEpayOrder 查询易支付订单状态
func (s *PaymentMonitorService) queryEpayOrder(orderID string, config struct {
	APIURL      string `json:"api_url"`
	MerchantID  string `json:"merchant_id"`
	MerchantKey string `json:"merchant_key"`
}) (string, error) {
	// 构建查询参数
	params := map[string]string{
		"act":          "order",
		"pid":          config.MerchantID,
		"out_trade_no": orderID,
	}

	// 生成签名
	sign := s.generateEpaySign(params, config.MerchantKey)
	params["sign"] = sign
	params["sign_type"] = "MD5"

	// 发送请求
	url := fmt.Sprintf("%s/api.php?act=order&pid=%s&out_trade_no=%s&sign=%s&sign_type=MD5",
		config.APIURL, config.MerchantID, orderID, sign)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Code   int    `json:"code"`
		Msg    string `json:"msg"`
		Status string `json:"status"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	return result.Status, nil
}

// generateEpaySign 生成易支付签名
func (s *PaymentMonitorService) generateEpaySign(params map[string]string, key string) string {
	// 按照易支付规则生成签名
	str := fmt.Sprintf("act=%s&out_trade_no=%s&pid=%s%s",
		params["act"], params["out_trade_no"], params["pid"], key)

	hash := md5.Sum([]byte(str))
	return hex.EncodeToString(hash[:])
}

// confirmPayment 确认支付
func (s *PaymentMonitorService) confirmPayment(monitor *sqlite.PaymentMonitor) {
	logger.Info("支付确认",
		zap.String("transactionID", monitor.TransactionID),
		zap.String("paymentType", monitor.PaymentType),
		zap.Float64("amount", monitor.ExpectedAmount))

	// 更新监听状态
	if err := s.db.SQLite.UpdatePaymentMonitor(monitor.ID, "confirmed", monitor.ConfirmCount+1); err != nil {
		logger.Error("更新监听状态失败", zap.Error(err))
		return
	}

	// 获取交易记录
	var transaction struct {
		WalletID string
		UserID   string
	}
	query := `SELECT wallet_id, user_id FROM wallet_transactions WHERE id = ?`
	err := s.db.SQLite.Get().QueryRow(query, monitor.TransactionID).Scan(
		&transaction.WalletID, &transaction.UserID,
	)
	if err != nil {
		logger.Error("查询交易记录失败", zap.Error(err))
		return
	}

	// 获取钱包
	wallet, err := s.db.SQLite.GetWalletByUserID(transaction.UserID)
	if err != nil || wallet == nil {
		logger.Error("获取钱包失败", zap.Error(err))
		return
	}

	// 更新钱包余额
	newBalance := wallet.Balance + monitor.ExpectedAmount
	if err := s.db.SQLite.UpdateWalletBalance(wallet.ID, newBalance, wallet.Frozen); err != nil {
		logger.Error("更新钱包余额失败", zap.Error(err))
		return
	}

	// 更新交易状态
	updateQuery := `UPDATE wallet_transactions 
		SET status = 'completed', balance = ? 
		WHERE id = ?`
	if _, err := s.db.SQLite.Get().Exec(updateQuery, newBalance, monitor.TransactionID); err != nil {
		logger.Error("更新交易状态失败", zap.Error(err))
		return
	}

	logger.Info("充值成功",
		zap.String("userID", transaction.UserID),
		zap.Float64("amount", monitor.ExpectedAmount),
		zap.Float64("newBalance", newBalance))
}

// handleTimeout 处理超时订单
func (s *PaymentMonitorService) handleTimeout(monitor *sqlite.PaymentMonitor) {
	logger.Warn("支付超时",
		zap.String("transactionID", monitor.TransactionID),
		zap.String("paymentType", monitor.PaymentType))

	// 更新状态为超时
	if err := s.db.SQLite.UpdatePaymentMonitor(monitor.ID, "timeout", monitor.ConfirmCount); err != nil {
		logger.Error("更新监听状态失败", zap.Error(err))
		return
	}

	// 更新交易状态为失败
	query := `UPDATE wallet_transactions SET status = 'failed' WHERE id = ?`
	if _, err := s.db.SQLite.Get().Exec(query, monitor.TransactionID); err != nil {
		logger.Error("更新交易状态失败", zap.Error(err))
	}
}
