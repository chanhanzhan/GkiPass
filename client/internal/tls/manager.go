package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Config TLS配置
type Config struct {
	CertFile            string        `json:"cert_file"`
	KeyFile             string        `json:"key_file"`
	CAFile              string        `json:"ca_file"`
	ServerName          string        `json:"server_name"`
	InsecureSkipVerify  bool          `json:"insecure_skip_verify"`
	CertRefreshInterval time.Duration `json:"cert_refresh_interval"`
}

// Manager TLS管理器
type Manager struct {
	config      *Config
	certPool    *x509.CertPool
	certificate *tls.Certificate
	tlsConfig   *tls.Config
	mutex       sync.RWMutex
	logger      *zap.Logger
}

// NewManager 创建TLS管理器
func NewManager(config *Config) *Manager {
	if config == nil {
		config = &Config{
			CertRefreshInterval: 24 * time.Hour,
		}
	}

	return &Manager{
		config: config,
		logger: zap.L().Named("tls-manager"),
	}
}

// Start 启动TLS管理器
func (m *Manager) Start() error {
	m.logger.Info("启动TLS管理器")

	// 初始化TLS配置
	if err := m.initTLSConfig(); err != nil {
		return fmt.Errorf("初始化TLS配置失败: %w", err)
	}

	// 如果设置了证书刷新间隔，启动定时刷新
	if m.config.CertRefreshInterval > 0 {
		go m.refreshCertificates()
	}

	return nil
}

// Stop 停止TLS管理器
func (m *Manager) Stop() error {
	m.logger.Info("停止TLS管理器")
	return nil
}

// GetTLSConfig 获取TLS配置
func (m *Manager) GetTLSConfig() *tls.Config {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.tlsConfig
}

// GetStatus 获取状态
func (m *Manager) GetStatus() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	status := map[string]interface{}{
		"has_certificate": m.certificate != nil,
		"has_cert_pool":   m.certPool != nil,
	}

	if m.certificate != nil && len(m.certificate.Certificate) > 0 {
		leaf, err := x509.ParseCertificate(m.certificate.Certificate[0])
		if err == nil {
			status["cert_subject"] = leaf.Subject.CommonName
			status["cert_issuer"] = leaf.Issuer.CommonName
			status["cert_not_before"] = leaf.NotBefore.Format(time.RFC3339)
			status["cert_not_after"] = leaf.NotAfter.Format(time.RFC3339)
		}
	}

	return status
}

// initTLSConfig 初始化TLS配置
func (m *Manager) initTLSConfig() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 创建基本的TLS配置
	m.tlsConfig = &tls.Config{
		InsecureSkipVerify: m.config.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	// 如果指定了服务器名称
	if m.config.ServerName != "" {
		m.tlsConfig.ServerName = m.config.ServerName
	}

	// 如果指定了CA文件，加载CA证书
	if m.config.CAFile != "" {
		caPool, err := m.loadCACertificates(m.config.CAFile)
		if err != nil {
			m.logger.Error("加载CA证书失败", zap.Error(err))
			return fmt.Errorf("加载CA证书失败: %w", err)
		}
		m.certPool = caPool
		m.tlsConfig.RootCAs = caPool
	}

	// 如果指定了证书和密钥文件，加载客户端证书
	if m.config.CertFile != "" && m.config.KeyFile != "" {
		cert, err := m.loadCertificate(m.config.CertFile, m.config.KeyFile)
		if err != nil {
			m.logger.Error("加载客户端证书失败", zap.Error(err))
			return fmt.Errorf("加载客户端证书失败: %w", err)
		}
		m.certificate = cert
		m.tlsConfig.Certificates = []tls.Certificate{*cert}
	}

	return nil
}

// loadCACertificates 加载CA证书
func (m *Manager) loadCACertificates(caFile string) (*x509.CertPool, error) {
	caData, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("读取CA证书文件失败: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("解析CA证书失败")
	}

	m.logger.Info("CA证书加载成功")
	return caPool, nil
}

// loadCertificate 加载证书
func (m *Manager) loadCertificate(certFile, keyFile string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("加载证书和密钥失败: %w", err)
	}

	m.logger.Info("客户端证书加载成功")
	return &cert, nil
}

// refreshCertificates 定期刷新证书
func (m *Manager) refreshCertificates() {
	ticker := time.NewTicker(m.config.CertRefreshInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.logger.Info("刷新证书")
		if err := m.initTLSConfig(); err != nil {
			m.logger.Error("刷新证书失败", zap.Error(err))
		}
	}
}
