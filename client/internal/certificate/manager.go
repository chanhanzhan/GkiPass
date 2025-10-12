package certificate

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// 证书有效期
	CACertValidDays   = 3650 // CA证书10年
	LeafCertValidDays = 30   // Leaf证书30天

	// 密钥长度
	RSAKeySize = 2048

	// 证书文件名
	CACertFile   = "ca.crt"
	CAKeyFile    = "ca.key"
	NodeCertFile = "node.crt"
	NodeKeyFile  = "node.key"

	// 证书更新阈值（剩余天数）
	RenewalThresholdDays = 7
)

// CertificateInfo 证书信息
type CertificateInfo struct {
	Subject      string    `json:"subject"`
	Issuer       string    `json:"issuer"`
	NotBefore    time.Time `json:"not_before"`
	NotAfter     time.Time `json:"not_after"`
	SerialNumber string    `json:"serial_number"`
	Fingerprint  string    `json:"fingerprint"`
	SPKI         string    `json:"spki"` // Subject Public Key Info
}

// Manager 证书管理器
type Manager struct {
	nodeID  string
	certDir string
	logger  *zap.Logger

	// 证书缓存
	caCert    *x509.Certificate
	caKey     *rsa.PrivateKey
	nodeCert  *x509.Certificate
	nodeKey   *rsa.PrivateKey
	tlsConfig *tls.Config

	// 状态管理
	ctx        context.Context
	cancel     context.CancelFunc
	renewTimer *time.Timer
	mutex      sync.RWMutex

	// PIN验证列表 (SPKI hashes)
	trustedPins map[string]bool
}

// New 创建证书管理器
func New(nodeID, certDir string) (*Manager, error) {
	manager := &Manager{
		nodeID:      nodeID,
		certDir:     certDir,
		logger:      zap.L().Named("certificate"),
		trustedPins: make(map[string]bool),
	}

	// 确保证书目录存在
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, fmt.Errorf("创建证书目录失败: %w", err)
	}

	// 加载或生成证书
	if err := manager.initializeCertificates(); err != nil {
		return nil, fmt.Errorf("初始化证书失败: %w", err)
	}

	return manager, nil
}

// initializeCertificates 初始化证书
func (m *Manager) initializeCertificates() error {
	m.logger.Info("🔐 初始化证书系统...")

	// 1. 加载或生成CA证书
	if err := m.loadOrGenerateCA(); err != nil {
		return fmt.Errorf("处理CA证书失败: %w", err)
	}

	// 2. 加载或生成节点证书
	if err := m.loadOrGenerateNodeCert(); err != nil {
		return fmt.Errorf("处理节点证书失败: %w", err)
	}

	// 3. 构建TLS配置
	if err := m.buildTLSConfig(); err != nil {
		return fmt.Errorf("构建TLS配置失败: %w", err)
	}

	m.logger.Info("✅ 证书系统初始化完成",
		zap.String("ca_fingerprint", m.getCertFingerprint(m.caCert)),
		zap.String("node_fingerprint", m.getCertFingerprint(m.nodeCert)),
		zap.Time("node_expires", m.nodeCert.NotAfter))

	return nil
}

// loadOrGenerateCA 加载或生成CA证书
func (m *Manager) loadOrGenerateCA() error {
	caCertPath := filepath.Join(m.certDir, CACertFile)
	caKeyPath := filepath.Join(m.certDir, CAKeyFile)

	// 尝试加载现有CA证书
	if m.loadCA(caCertPath, caKeyPath) == nil {
		// 验证CA证书有效性
		if time.Until(m.caCert.NotAfter) > 365*24*time.Hour {
			m.logger.Info("使用现有CA证书", zap.Time("expires", m.caCert.NotAfter))
			return nil
		}
		m.logger.Warn("CA证书即将过期，重新生成", zap.Time("expires", m.caCert.NotAfter))
	}

	// 生成新的CA证书
	m.logger.Info("生成新的CA证书...")
	return m.generateCA(caCertPath, caKeyPath)
}

// loadCA 加载CA证书
func (m *Manager) loadCA(certPath, keyPath string) error {
	// 加载证书
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return fmt.Errorf("解析CA证书失败")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return err
	}

	// 加载私钥
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("解析CA私钥失败")
	}

	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return err
	}

	m.caCert = cert
	m.caKey = key

	return nil
}

// generateCA 生成CA证书
func (m *Manager) generateCA(certPath, keyPath string) error {
	// 生成私钥
	key, err := rsa.GenerateKey(rand.Reader, RSAKeySize)
	if err != nil {
		return fmt.Errorf("生成CA私钥失败: %w", err)
	}

	// 证书模板
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"GKIPass"},
			Country:       []string{"CN"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    fmt.Sprintf("GKIPass-Node-CA-%s", m.nodeID[:8]),
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, CACertValidDays),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	// 自签名
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("创建CA证书失败: %w", err)
	}

	// 解析证书
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("解析CA证书失败: %w", err)
	}

	// 保存证书
	if err := m.saveCertificate(certPath, certDER); err != nil {
		return fmt.Errorf("保存CA证书失败: %w", err)
	}

	// 保存私钥
	if err := m.savePrivateKey(keyPath, key); err != nil {
		return fmt.Errorf("保存CA私钥失败: %w", err)
	}

	m.caCert = cert
	m.caKey = key

	m.logger.Info("CA证书生成成功",
		zap.String("subject", cert.Subject.String()),
		zap.Time("expires", cert.NotAfter))

	return nil
}

// loadOrGenerateNodeCert 加载或生成节点证书
func (m *Manager) loadOrGenerateNodeCert() error {
	nodeCertPath := filepath.Join(m.certDir, NodeCertFile)
	nodeKeyPath := filepath.Join(m.certDir, NodeKeyFile)

	// 尝试加载现有节点证书
	if m.loadNodeCert(nodeCertPath, nodeKeyPath) == nil {
		// 检查证书是否需要更新
		if m.needsRenewal(m.nodeCert) {
			m.logger.Info("节点证书需要更新", zap.Time("expires", m.nodeCert.NotAfter))
		} else {
			m.logger.Info("使用现有节点证书", zap.Time("expires", m.nodeCert.NotAfter))
			return nil
		}
	}

	// 生成新的节点证书
	m.logger.Info("生成新的节点证书...")
	return m.generateNodeCert(nodeCertPath, nodeKeyPath)
}

// loadNodeCert 加载节点证书
func (m *Manager) loadNodeCert(certPath, keyPath string) error {
	// 加载证书
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return fmt.Errorf("解析节点证书失败")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return err
	}

	// 验证证书是否由我们的CA签发
	if err := cert.CheckSignatureFrom(m.caCert); err != nil {
		return fmt.Errorf("节点证书签名验证失败: %w", err)
	}

	// 加载私钥
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("解析节点私钥失败")
	}

	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return err
	}

	m.nodeCert = cert
	m.nodeKey = key

	return nil
}

// generateNodeCert 生成节点证书
func (m *Manager) generateNodeCert(certPath, keyPath string) error {
	// 生成私钥
	key, err := rsa.GenerateKey(rand.Reader, RSAKeySize)
	if err != nil {
		return fmt.Errorf("生成节点私钥失败: %w", err)
	}

	// 生成序列号
	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Errorf("生成序列号失败: %w", err)
	}

	// 证书模板
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:  []string{"GKIPass"},
			Country:       []string{"CN"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    fmt.Sprintf("GKIPass-Node-%s", m.nodeID),
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 0, LeafCertValidDays),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		SubjectKeyId: m.calculateSKI(&key.PublicKey),
	}

	// 添加SAN (Subject Alternative Names)
	template.DNSNames = []string{
		m.nodeID,
		fmt.Sprintf("node-%s", m.nodeID),
		"localhost",
	}

	// 由CA签名
	certDER, err := x509.CreateCertificate(rand.Reader, &template, m.caCert, &key.PublicKey, m.caKey)
	if err != nil {
		return fmt.Errorf("创建节点证书失败: %w", err)
	}

	// 解析证书
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("解析节点证书失败: %w", err)
	}

	// 保存证书
	if err := m.saveCertificate(certPath, certDER); err != nil {
		return fmt.Errorf("保存节点证书失败: %w", err)
	}

	// 保存私钥
	if err := m.savePrivateKey(keyPath, key); err != nil {
		return fmt.Errorf("保存节点私钥失败: %w", err)
	}

	m.nodeCert = cert
	m.nodeKey = key

	m.logger.Info("节点证书生成成功",
		zap.String("subject", cert.Subject.String()),
		zap.Time("expires", cert.NotAfter))

	return nil
}

// buildTLSConfig 构建TLS配置
func (m *Manager) buildTLSConfig() error {
	// 创建证书对
	cert := tls.Certificate{
		Certificate: [][]byte{m.nodeCert.Raw, m.caCert.Raw},
		PrivateKey:  m.nodeKey,
		Leaf:        m.nodeCert,
	}

	// 创建CA证书池
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(m.caCert)

	// 构建TLS配置
	m.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
		RootCAs:      caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ServerName:   m.nodeID,
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},
		// 自定义证书验证
		VerifyConnection: m.verifyConnection,
	}

	return nil
}

// verifyConnection 自定义证书验证（PIN验证）
func (m *Manager) verifyConnection(cs tls.ConnectionState) error {
	if len(cs.PeerCertificates) == 0 {
		return fmt.Errorf("没有对等证书")
	}

	// 计算证书的SPKI hash
	cert := cs.PeerCertificates[0]
	spkiHash := m.calculateSPKI(cert)

	// 如果有PIN列表，进行PIN验证
	m.mutex.RLock()
	hasPins := len(m.trustedPins) > 0
	isPinned := m.trustedPins[spkiHash]
	m.mutex.RUnlock()

	if hasPins && !isPinned {
		return fmt.Errorf("证书PIN验证失败: %s", spkiHash)
	}

	m.logger.Debug("证书验证通过",
		zap.String("subject", cert.Subject.String()),
		zap.String("spki_hash", spkiHash),
		zap.Bool("pinned", isPinned))

	return nil
}

// Start 启动证书管理器
func (m *Manager) Start() error {
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// 启动证书自动更新
	m.scheduleRenewal()

	m.logger.Info("证书管理器启动")
	return nil
}

// Stop 停止证书管理器
func (m *Manager) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}

	if m.renewTimer != nil {
		m.renewTimer.Stop()
	}

	m.logger.Info("证书管理器停止")
	return nil
}

// scheduleRenewal 调度证书更新
func (m *Manager) scheduleRenewal() {
	if m.needsRenewal(m.nodeCert) {
		// 立即更新
		go m.renewNodeCert()
		return
	}

	// 计算到期前7天的时间
	renewTime := m.nodeCert.NotAfter.AddDate(0, 0, -RenewalThresholdDays)
	duration := time.Until(renewTime)

	if duration > 0 {
		m.renewTimer = time.AfterFunc(duration, func() {
			m.renewNodeCert()
		})

		m.logger.Info("证书自动更新已调度",
			zap.Time("renewal_time", renewTime),
			zap.Duration("duration", duration))
	}
}

// renewNodeCert 更新节点证书
func (m *Manager) renewNodeCert() {
	m.logger.Info("开始自动更新节点证书...")

	nodeCertPath := filepath.Join(m.certDir, NodeCertFile)
	nodeKeyPath := filepath.Join(m.certDir, NodeKeyFile)

	if err := m.generateNodeCert(nodeCertPath, nodeKeyPath); err != nil {
		m.logger.Error("自动更新节点证书失败", zap.Error(err))
		// 重新调度（1小时后重试）
		m.renewTimer = time.AfterFunc(time.Hour, func() {
			m.renewNodeCert()
		})
		return
	}

	// 重新构建TLS配置
	if err := m.buildTLSConfig(); err != nil {
		m.logger.Error("重新构建TLS配置失败", zap.Error(err))
		return
	}

	m.logger.Info("节点证书自动更新成功", zap.Time("expires", m.nodeCert.NotAfter))

	// 调度下次更新
	m.scheduleRenewal()
}

// needsRenewal 检查证书是否需要更新
func (m *Manager) needsRenewal(cert *x509.Certificate) bool {
	if cert == nil {
		return true
	}

	threshold := time.Now().AddDate(0, 0, RenewalThresholdDays)
	return cert.NotAfter.Before(threshold)
}

// GetTLSConfig 获取TLS配置
func (m *Manager) GetTLSConfig() *tls.Config {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.tlsConfig == nil {
		return &tls.Config{}
	}

	// 返回副本以防止并发修改
	return m.tlsConfig.Clone()
}

// GetCACertificate 获取CA证书
func (m *Manager) GetCACertificate() *x509.Certificate {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.caCert
}

// GetNodeCertificate 获取节点证书
func (m *Manager) GetNodeCertificate() *x509.Certificate {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.nodeCert
}

// GetCertificateInfo 获取证书信息
func (m *Manager) GetCertificateInfo() (*CertificateInfo, *CertificateInfo) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var caInfo, nodeInfo *CertificateInfo

	if m.caCert != nil {
		caInfo = &CertificateInfo{
			Subject:      m.caCert.Subject.String(),
			Issuer:       m.caCert.Issuer.String(),
			NotBefore:    m.caCert.NotBefore,
			NotAfter:     m.caCert.NotAfter,
			SerialNumber: m.caCert.SerialNumber.String(),
			Fingerprint:  m.getCertFingerprint(m.caCert),
			SPKI:         m.calculateSPKI(m.caCert),
		}
	}

	if m.nodeCert != nil {
		nodeInfo = &CertificateInfo{
			Subject:      m.nodeCert.Subject.String(),
			Issuer:       m.nodeCert.Issuer.String(),
			NotBefore:    m.nodeCert.NotBefore,
			NotAfter:     m.nodeCert.NotAfter,
			SerialNumber: m.nodeCert.SerialNumber.String(),
			Fingerprint:  m.getCertFingerprint(m.nodeCert),
			SPKI:         m.calculateSPKI(m.nodeCert),
		}
	}

	return caInfo, nodeInfo
}

// AddTrustedPin 添加信任的证书PIN
func (m *Manager) AddTrustedPin(spkiHash string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.trustedPins[spkiHash] = true
	m.logger.Info("添加信任PIN", zap.String("spki_hash", spkiHash))
}

// RemoveTrustedPin 移除信任的证书PIN
func (m *Manager) RemoveTrustedPin(spkiHash string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.trustedPins, spkiHash)
	m.logger.Info("移除信任PIN", zap.String("spki_hash", spkiHash))
}

// GetTrustedPins 获取所有信任的PIN
func (m *Manager) GetTrustedPins() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	pins := make([]string, 0, len(m.trustedPins))
	for pin := range m.trustedPins {
		pins = append(pins, pin)
	}
	return pins
}

// 辅助方法

// saveCertificate 保存证书到文件
func (m *Manager) saveCertificate(path string, certDER []byte) error {
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return os.WriteFile(path, certPEM, 0644)
}

// savePrivateKey 保存私钥到文件
func (m *Manager) savePrivateKey(path string, key *rsa.PrivateKey) error {
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyDER,
	})

	return os.WriteFile(path, keyPEM, 0600)
}

// getCertFingerprint 获取证书指纹
func (m *Manager) getCertFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return fmt.Sprintf("%x", hash)
}

// calculateSPKI 计算证书的SPKI hash
func (m *Manager) calculateSPKI(cert *x509.Certificate) string {
	spkiDER, _ := x509.MarshalPKIXPublicKey(cert.PublicKey)
	hash := sha256.Sum256(spkiDER)
	return base64.StdEncoding.EncodeToString(hash[:])
}

// calculateSKI 计算Subject Key Identifier
func (m *Manager) calculateSKI(pubKey *rsa.PublicKey) []byte {
	spkiDER, _ := x509.MarshalPKIXPublicKey(pubKey)
	hash := sha256.Sum256(spkiDER)
	return hash[:]
}
