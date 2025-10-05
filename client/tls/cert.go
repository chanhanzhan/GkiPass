package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// CertManager 证书管理器
type CertManager struct {
	certFile string
	keyFile  string
	caFile   string

	cert     *tls.Certificate
	caPool   *x509.CertPool
	loadTime time.Time
}

// NewCertManager 创建证书管理器
func NewCertManager(certFile, keyFile, caFile string) *CertManager {
	return &CertManager{
		certFile: certFile,
		keyFile:  keyFile,
		caFile:   caFile,
	}
}

// Load 加载证书
func (cm *CertManager) Load() error {
	// 加载客户端证书和私钥
	cert, err := tls.LoadX509KeyPair(cm.certFile, cm.keyFile)
	if err != nil {
		return fmt.Errorf("加载证书失败: %w", err)
	}
	cm.cert = &cert
	cm.loadTime = time.Now()

	// 解析证书检查有效期
	if len(cert.Certificate) > 0 {
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("解析证书失败: %w", err)
		}

		logger.Info("证书已加载",
			zap.String("subject", x509Cert.Subject.String()),
			zap.Time("not_before", x509Cert.NotBefore),
			zap.Time("not_after", x509Cert.NotAfter))

		// 检查证书是否即将过期
		if time.Until(x509Cert.NotAfter) < 7*24*time.Hour {
			logger.Warn("证书即将过期",
				zap.Duration("remaining", time.Until(x509Cert.NotAfter)))
		}
	}

	// 加载CA证书池
	if cm.caFile != "" {
		caData, err := os.ReadFile(cm.caFile)
		if err != nil {
			return fmt.Errorf("读取CA证书失败: %w", err)
		}

		cm.caPool = x509.NewCertPool()
		if !cm.caPool.AppendCertsFromPEM(caData) {
			return fmt.Errorf("解析CA证书失败")
		}

		logger.Info("CA证书已加载", zap.String("file", cm.caFile))
	}

	return nil
}

// GetTLSConfig 获取TLS配置
func (cm *CertManager) GetTLSConfig() *tls.Config {
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
	}

	if cm.cert != nil {
		config.Certificates = []tls.Certificate{*cm.cert}
	}

	if cm.caPool != nil {
		config.RootCAs = cm.caPool
	}

	return config
}

// NeedRenew 检查是否需要续期
func (cm *CertManager) NeedRenew() bool {
	if cm.cert == nil || len(cm.cert.Certificate) == 0 {
		return true
	}

	x509Cert, err := x509.ParseCertificate(cm.cert.Certificate[0])
	if err != nil {
		return true
	}

	// 提前7天续期
	return time.Until(x509Cert.NotAfter) < 7*24*time.Hour
}

// GetExpiry 获取证书过期时间
func (cm *CertManager) GetExpiry() (time.Time, error) {
	if cm.cert == nil || len(cm.cert.Certificate) == 0 {
		return time.Time{}, fmt.Errorf("证书未加载")
	}

	x509Cert, err := x509.ParseCertificate(cm.cert.Certificate[0])
	if err != nil {
		return time.Time{}, err
	}

	return x509Cert.NotAfter, nil
}






