package service

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"gkipass/plane/db"
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// NodeCertManager 节点证书管理器
type NodeCertManager struct {
	db       *db.Manager
	caKey    *rsa.PrivateKey
	caCert   *x509.Certificate
	certsDir string
	nodeDir  string
}

// NewNodeCertManager 创建节点证书管理器
func NewNodeCertManager(dbManager *db.Manager, certsDir string) (*NodeCertManager, error) {
	manager := &NodeCertManager{
		db:       dbManager,
		certsDir: certsDir,
		nodeDir:  filepath.Join(certsDir, "nodes"),
	}

	// 创建节点证书目录
	if err := os.MkdirAll(manager.nodeDir, 0755); err != nil {
		return nil, fmt.Errorf("创建节点证书目录失败: %w", err)
	}

	// 加载 CA 证书和密钥
	if err := manager.loadCA(); err != nil {
		return nil, fmt.Errorf("加载 CA 证书失败: %w", err)
	}

	return manager, nil
}

// loadCA 加载 CA 证书和密钥
func (m *NodeCertManager) loadCA() error {
	// 读取 CA 证书
	caCertPath := filepath.Join(m.certsDir, "ca-cert.pem")
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return fmt.Errorf("读取 CA 证书失败: %w", err)
	}

	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return fmt.Errorf("解析 CA 证书失败")
	}

	m.caCert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("解析 CA 证书失败: %w", err)
	}

	// 读取 CA 私钥
	caKeyPath := filepath.Join(m.certsDir, "ca-key.pem")
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return fmt.Errorf("读取 CA 私钥失败: %w", err)
	}

	block, _ = pem.Decode(caKeyPEM)
	if block == nil {
		return fmt.Errorf("解析 CA 私钥失败")
	}

	m.caKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("解析 CA 私钥失败: %w", err)
	}

	logger.Info("✓ CA 证书已加载", zap.String("subject", m.caCert.Subject.CommonName))
	return nil
}

// GenerateNodeCert 为节点生成证书
func (m *NodeCertManager) GenerateNodeCert(nodeID, nodeName string) (*dbinit.Certificate, error) {
	logger.Info("生成节点证书", zap.String("nodeID", nodeID), zap.String("nodeName", nodeName))

	// 生成节点私钥
	nodeKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("生成节点私钥失败: %w", err)
	}

	// 创建证书模板
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	notAfter := now.AddDate(1, 0, 0) // 1年有效期

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("node-%s", nodeID),
			Organization: []string{"GKI Pass"},
			Country:      []string{"CN"},
		},
		NotBefore:             now,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// 使用 CA 签发证书
	certDER, err := x509.CreateCertificate(rand.Reader, template, m.caCert, &nodeKey.PublicKey, m.caKey)
	if err != nil {
		return nil, fmt.Errorf("签发节点证书失败: %w", err)
	}

	// 保存证书和私钥
	nodeDir := filepath.Join(m.nodeDir, nodeID)
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return nil, fmt.Errorf("创建节点证书目录失败: %w", err)
	}

	// 保存证书
	certPath := filepath.Join(nodeDir, "cert.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("保存节点证书失败: %w", err)
	}

	// 保存私钥
	keyPath := filepath.Join(nodeDir, "key.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(nodeKey),
	})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("保存节点私钥失败: %w", err)
	}

	// 保存 CA 证书（供节点验证）
	caCertPath := filepath.Join(nodeDir, "ca-cert.pem")
	caCertPEM, _ := os.ReadFile(filepath.Join(m.certsDir, "ca-cert.pem"))
	if err := os.WriteFile(caCertPath, caCertPEM, 0644); err != nil {
		return nil, fmt.Errorf("保存 CA 证书失败: %w", err)
	}

	// 计算 SPKI Pin
	pin := fmt.Sprintf("%x", x509.MarshalPKCS1PublicKey(&nodeKey.PublicKey))

	// 创建证书记录
	cert := &dbinit.Certificate{
		ID:          uuid.New().String(),
		Type:        "leaf",
		Name:        fmt.Sprintf("Node Certificate - %s", nodeName),
		CommonName:  template.Subject.CommonName,
		PublicKey:   string(certPEM),
		PrivateKey:  string(keyPEM),
		Pin:         pin,
		ParentID:    "", // 暂不关联 CA 记录
		NotBefore:   now,
		NotAfter:    notAfter,
		Revoked:     false,
		Description: fmt.Sprintf("自动生成的节点 %s 证书", nodeName),
	}

	// 保存到数据库
	if err := m.db.DB.SQLite.CreateCertificate(cert); err != nil {
		return nil, fmt.Errorf("保存证书记录失败: %w", err)
	}

	logger.Info("✓ 节点证书已生成",
		zap.String("nodeID", nodeID),
		zap.String("certID", cert.ID),
		zap.String("path", nodeDir),
		zap.Time("expires", notAfter))

	return cert, nil
}

// GetNodeCertPath 获取节点证书路径
func (m *NodeCertManager) GetNodeCertPath(nodeID string) (certPath, keyPath, caPath string) {
	nodeDir := filepath.Join(m.nodeDir, nodeID)
	return filepath.Join(nodeDir, "cert.pem"),
		filepath.Join(nodeDir, "key.pem"),
		filepath.Join(nodeDir, "ca-cert.pem")
}

// RevokeNodeCert 撤销节点证书
func (m *NodeCertManager) RevokeNodeCert(certID string) error {
	logger.Info("撤销节点证书", zap.String("certID", certID))

	// 更新数据库状态
	if err := m.db.DB.SQLite.RevokeCertificate(certID); err != nil {
		return fmt.Errorf("撤销证书失败: %w", err)
	}

	logger.Info("✓ 节点证书已撤销", zap.String("certID", certID))
	return nil
}

// RenewNodeCert 续期节点证书
func (m *NodeCertManager) RenewNodeCert(nodeID, nodeName, oldCertID string) (*dbinit.Certificate, error) {
	logger.Info("续期节点证书", zap.String("nodeID", nodeID))

	// 撤销旧证书
	if oldCertID != "" {
		if err := m.RevokeNodeCert(oldCertID); err != nil {
			logger.Warn("撤销旧证书失败", zap.Error(err))
		}
	}

	// 生成新证书
	newCert, err := m.GenerateNodeCert(nodeID, nodeName)
	if err != nil {
		return nil, fmt.Errorf("生成新证书失败: %w", err)
	}

	logger.Info("✓ 节点证书已续期", zap.String("nodeID", nodeID), zap.String("newCertID", newCert.ID))
	return newCert, nil
}

// CheckCertExpiry 检查证书是否即将过期（30天内）
func (m *NodeCertManager) CheckCertExpiry(cert *dbinit.Certificate) bool {
	return time.Until(cert.NotAfter) < 30*24*time.Hour
}

// CleanupExpiredCerts 清理过期证书文件
func (m *NodeCertManager) CleanupExpiredCerts() error {
	logger.Info("清理过期证书...")

	// 获取所有已撤销或过期的证书
	revoked := false
	certs, err := m.db.DB.SQLite.ListCertificates("leaf", &revoked)
	if err != nil {
		return fmt.Errorf("获取证书列表失败: %w", err)
	}

	cleaned := 0
	for _, cert := range certs {
		if cert.Revoked || time.Now().After(cert.NotAfter) {
			logger.Debug("发现过期/撤销证书", zap.String("certID", cert.ID))
			cleaned++
		}
	}

	logger.Info("✓ 证书清理完成", zap.Int("cleaned", cleaned))
	return nil
}
