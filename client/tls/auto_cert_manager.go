package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// AutoCertManager 自动证书管理器
type AutoCertManager struct {
	nodeID        string
	planeURL      string
	connectionKey string
	certDir       string

	// 证书信息
	currentCert *tls.Certificate
	caCert      *x509.Certificate
	certExpiry  time.Time

	// HTTP客户端
	httpClient *http.Client

	// 控制字段
	stopChan chan struct{}
	started  bool
	mu       sync.RWMutex
}

// CertificateInfo 证书信息
type CertificateInfo struct {
	ID         string    `json:"id"`
	CommonName string    `json:"common_name"`
	NotBefore  time.Time `json:"not_before"`
	NotAfter   time.Time `json:"not_after"`
	PublicKey  string    `json:"public_key"`
	PrivateKey string    `json:"private_key"`
}

// NewAutoCertManager 创建自动证书管理器
func NewAutoCertManager(nodeID, planeURL, connectionKey, certDir string) *AutoCertManager {
	return &AutoCertManager{
		nodeID:        nodeID,
		planeURL:      planeURL,
		connectionKey: connectionKey,
		certDir:       certDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		stopChan: make(chan struct{}),
	}
}

// Start 启动证书管理器
func (cm *AutoCertManager) Start() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.started {
		return nil
	}

	cm.started = true

	// 确保证书目录存在
	if err := os.MkdirAll(cm.certDir, 0755); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}

	// 初始化证书
	if err := cm.initializeCertificates(); err != nil {
		logger.Error("初始化证书失败", zap.Error(err))
		// 不返回错误，继续运行，稍后重试
	}

	// 启动证书检查和续期协程
	go cm.certMaintenanceLoop()

	logger.Info("自动证书管理器已启动",
		zap.String("nodeID", cm.nodeID),
		zap.String("certDir", cm.certDir))

	return nil
}

// Stop 停止证书管理器
func (cm *AutoCertManager) Stop() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !cm.started {
		return
	}

	close(cm.stopChan)
	cm.started = false

	logger.Info("自动证书管理器已停止")
}

// GetTLSConfig 获取TLS配置
func (cm *AutoCertManager) GetTLSConfig() *tls.Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.currentCert == nil {
		return nil
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*cm.currentCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}
}

// initializeCertificates 初始化证书
func (cm *AutoCertManager) initializeCertificates() error {
	// 1. 检查本地证书
	if err := cm.loadLocalCertificates(); err == nil {
		logger.Info("本地证书加载成功")
		return nil
	}

	// 2. 从Plane获取证书
	if err := cm.fetchCertificatesFromPlane(); err == nil {
		logger.Info("从Plane获取证书成功")
		return nil
	}

	// 3. 请求Plane生成新证书
	if err := cm.requestCertificateGeneration(); err != nil {
		return fmt.Errorf("请求证书生成失败: %w", err)
	}

	logger.Info("证书初始化完成")
	return nil
}

// loadLocalCertificates 加载本地证书
func (cm *AutoCertManager) loadLocalCertificates() error {
	certPath := filepath.Join(cm.certDir, "cert.pem")
	keyPath := filepath.Join(cm.certDir, "key.pem")

	// 检查文件是否存在
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return fmt.Errorf("证书文件不存在")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("私钥文件不存在")
	}

	// 加载证书
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf("加载证书失败: %w", err)
	}

	// 解析证书以检查有效期
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("解析证书失败: %w", err)
	}

	// 检查证书是否即将过期（7天内）
	if time.Until(x509Cert.NotAfter) < 7*24*time.Hour {
		logger.Warn("本地证书即将过期", zap.Time("expires", x509Cert.NotAfter))
		return fmt.Errorf("证书即将过期")
	}

	cm.currentCert = &cert
	cm.certExpiry = x509Cert.NotAfter

	// 加载CA证书
	cm.loadCACertificate()

	logger.Info("本地证书已加载",
		zap.String("subject", x509Cert.Subject.CommonName),
		zap.Time("expires", x509Cert.NotAfter))

	return nil
}

// fetchCertificatesFromPlane 从Plane获取证书
func (cm *AutoCertManager) fetchCertificatesFromPlane() error {
	url := fmt.Sprintf("%s/api/v1/nodes/%s/cert/info", cm.planeURL, cm.nodeID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Connection-Key", cm.connectionKey)

	resp, err := cm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求证书信息失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("节点证书不存在")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求失败: %d", resp.StatusCode)
	}

	var certInfo struct {
		HasCert    bool      `json:"has_cert"`
		FileExists bool      `json:"file_exists"`
		CertID     string    `json:"cert_id"`
		NotAfter   time.Time `json:"not_after"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&certInfo); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if !certInfo.HasCert {
		return fmt.Errorf("节点无证书")
	}

	// 下载证书文件
	return cm.downloadCertificates()
}

// downloadCertificates 下载证书文件
func (cm *AutoCertManager) downloadCertificates() error {
	url := fmt.Sprintf("%s/api/v1/nodes/%s/cert/download", cm.planeURL, cm.nodeID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Connection-Key", cm.connectionKey)

	resp, err := cm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载证书失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: %d", resp.StatusCode)
	}

	// 解析并保存证书（这里简化处理，实际应该解析ZIP文件）
	// TODO: 实现ZIP文件解析和证书文件保存

	logger.Info("证书下载完成")

	// 重新加载证书
	return cm.loadLocalCertificates()
}

// requestCertificateGeneration 请求证书生成
func (cm *AutoCertManager) requestCertificateGeneration() error {
	url := fmt.Sprintf("%s/api/v1/nodes/%s/cert/generate", cm.planeURL, cm.nodeID)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Connection-Key", cm.connectionKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求证书生成失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("证书生成请求失败: %d", resp.StatusCode)
	}

	logger.Info("证书生成请求已发送，等待生成完成...")

	// 等待证书生成完成后下载
	time.Sleep(5 * time.Second)
	return cm.downloadCertificates()
}

// loadCACertificate 加载CA证书
func (cm *AutoCertManager) loadCACertificate() {
	caCertPath := filepath.Join(cm.certDir, "ca-cert.pem")

	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		logger.Warn("加载CA证书失败", zap.Error(err))
		return
	}

	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		logger.Warn("解析CA证书失败")
		return
	}

	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		logger.Warn("解析CA证书失败", zap.Error(err))
		return
	}

	cm.caCert = caCert
	logger.Info("CA证书已加载", zap.String("subject", caCert.Subject.CommonName))
}

// certMaintenanceLoop 证书维护循环
func (cm *AutoCertManager) certMaintenanceLoop() {
	// 每小时检查一次证书状态
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.checkAndRenewCertificate()
		case <-cm.stopChan:
			return
		}
	}
}

// checkAndRenewCertificate 检查并续期证书
func (cm *AutoCertManager) checkAndRenewCertificate() {
	cm.mu.RLock()
	expiry := cm.certExpiry
	cm.mu.RUnlock()

	// 如果证书在7天内过期，进行续期
	if time.Until(expiry) < 7*24*time.Hour {
		logger.Info("证书即将过期，开始续期",
			zap.String("nodeID", cm.nodeID),
			zap.Time("expires", expiry))

		if err := cm.renewCertificate(); err != nil {
			logger.Error("证书续期失败", zap.Error(err))
		}
	}
}

// renewCertificate 续期证书
func (cm *AutoCertManager) renewCertificate() error {
	url := fmt.Sprintf("%s/api/v1/nodes/%s/cert/renew", cm.planeURL, cm.nodeID)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Connection-Key", cm.connectionKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求证书续期失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("证书续期请求失败: %d", resp.StatusCode)
	}

	logger.Info("证书续期请求已发送")

	// 等待续期完成后重新下载
	time.Sleep(3 * time.Second)
	if err := cm.downloadCertificates(); err != nil {
		return fmt.Errorf("下载新证书失败: %w", err)
	}

	logger.Info("证书续期完成")
	return nil
}

// generateSelfSignedCert 生成自签名证书（应急使用）
func (cm *AutoCertManager) generateSelfSignedCert() error {
	logger.Warn("生成自签名证书作为应急方案")

	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("生成私钥失败: %w", err)
	}

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("node-%s", cm.nodeID),
			Organization: []string{"GKI Pass Self-Signed"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(24 * time.Hour), // 24小时有效期
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:    []string{"localhost"},
	}

	// 自签名
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("创建证书失败: %w", err)
	}

	// 保存证书
	certPath := filepath.Join(cm.certDir, "cert.pem")
	certFile, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("创建证书文件失败: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("写入证书失败: %w", err)
	}

	// 保存私钥
	keyPath := filepath.Join(cm.certDir, "key.pem")
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return fmt.Errorf("创建私钥文件失败: %w", err)
	}
	defer keyFile.Close()

	privateKeyDER := x509.MarshalPKCS1PrivateKey(privateKey)
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyDER}); err != nil {
		return fmt.Errorf("写入私钥失败: %w", err)
	}

	// 重新加载证书
	return cm.loadLocalCertificates()
}

// GetCertificateExpiry 获取证书过期时间
func (cm *AutoCertManager) GetCertificateExpiry() time.Time {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.certExpiry
}

// HasValidCertificate 检查是否有有效证书
func (cm *AutoCertManager) HasValidCertificate() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.currentCert != nil && time.Now().Before(cm.certExpiry)
}

// GetCertificateInfo 获取证书信息
func (cm *AutoCertManager) GetCertificateInfo() *CertificateInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.currentCert == nil {
		return nil
	}

	// 解析第一个证书
	x509Cert, err := x509.ParseCertificate(cm.currentCert.Certificate[0])
	if err != nil {
		return nil
	}

	return &CertificateInfo{
		CommonName: x509Cert.Subject.CommonName,
		NotBefore:  x509Cert.NotBefore,
		NotAfter:   x509Cert.NotAfter,
	}
}

// ForceRenewal 强制续期证书
func (cm *AutoCertManager) ForceRenewal() error {
	logger.Info("强制续期证书", zap.String("nodeID", cm.nodeID))
	return cm.renewCertificate()
}

// OnCertificateUpdated 证书更新回调接口
type OnCertificateUpdatedCallback func(*tls.Config)

// SetCertificateUpdateCallback 设置证书更新回调
func (cm *AutoCertManager) SetCertificateUpdateCallback(callback OnCertificateUpdatedCallback) {
	// TODO: 实现证书更新回调机制
	// 当证书更新时，通知TunnelManager重新配置TLS
}

// Metrics 获取证书管理器指标
func (cm *AutoCertManager) Metrics() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	metrics := map[string]interface{}{
		"has_certificate":    cm.currentCert != nil,
		"certificate_expiry": cm.certExpiry,
		"days_until_expiry":  int(time.Until(cm.certExpiry).Hours() / 24),
	}

	if cm.currentCert != nil {
		x509Cert, err := x509.ParseCertificate(cm.currentCert.Certificate[0])
		if err == nil {
			metrics["common_name"] = x509Cert.Subject.CommonName
			metrics["issuer"] = x509Cert.Issuer.CommonName
		}
	}

	return metrics
}
