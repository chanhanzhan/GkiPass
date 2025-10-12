package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AutoCertConfig 自动证书配置
type AutoCertConfig struct {
	CertDir        string        `json:"cert_dir"`
	CommonName     string        `json:"common_name"`
	DNSNames       []string      `json:"dns_names"`
	IPAddresses    []string      `json:"ip_addresses"`
	ValidDuration  time.Duration `json:"valid_duration"`
	RenewThreshold time.Duration `json:"renew_threshold"`
}

// DefaultAutoCertConfig 默认自动证书配置
func DefaultAutoCertConfig() *AutoCertConfig {
	return &AutoCertConfig{
		CertDir:        "data/certs",
		CommonName:     "gkipass-client",
		DNSNames:       []string{"localhost"},
		IPAddresses:    []string{"127.0.0.1"},
		ValidDuration:  365 * 24 * time.Hour, // 1年
		RenewThreshold: 30 * 24 * time.Hour,  // 30天
	}
}

// AutoCertManager 自动证书管理器
type AutoCertManager struct {
	config      *AutoCertConfig
	certFile    string
	keyFile     string
	certificate *tls.Certificate
	generator   *CertificateGenerator
	renewTimer  *time.Timer
	mutex       sync.RWMutex
	logger      *zap.Logger
}

// NewAutoCertManager 创建自动证书管理器
func NewAutoCertManager(config *AutoCertConfig) *AutoCertManager {
	if config == nil {
		config = DefaultAutoCertConfig()
	}

	certFile := filepath.Join(config.CertDir, "cert.pem")
	keyFile := filepath.Join(config.CertDir, "key.pem")

	return &AutoCertManager{
		config:    config,
		certFile:  certFile,
		keyFile:   keyFile,
		generator: NewCertificateGenerator(),
		logger:    zap.L().Named("auto-cert-manager"),
	}
}

// Start 启动自动证书管理器
func (m *AutoCertManager) Start() error {
	m.logger.Info("启动自动证书管理器")

	// 确保证书目录存在
	if err := os.MkdirAll(m.config.CertDir, 0755); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}

	// 检查证书是否存在，如果不存在或即将过期则生成
	if err := m.ensureCertificate(); err != nil {
		return fmt.Errorf("确保证书存在失败: %w", err)
	}

	// 启动证书更新定时器
	m.scheduleRenewal()

	return nil
}

// Stop 停止自动证书管理器
func (m *AutoCertManager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 停止更新定时器
	if m.renewTimer != nil {
		m.renewTimer.Stop()
		m.renewTimer = nil
	}

	m.logger.Info("停止自动证书管理器")
	return nil
}

// GetCertificate 获取证书
func (m *AutoCertManager) GetCertificate() (*tls.Certificate, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.certificate == nil {
		return nil, fmt.Errorf("证书未初始化")
	}

	return m.certificate, nil
}

// ensureCertificate 确保证书存在且有效
func (m *AutoCertManager) ensureCertificate() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查证书文件是否存在
	certExists := fileExists(m.certFile) && fileExists(m.keyFile)

	// 如果证书存在，检查是否即将过期
	if certExists {
		cert, err := tls.LoadX509KeyPair(m.certFile, m.keyFile)
		if err != nil {
			m.logger.Warn("加载现有证书失败，将重新生成", zap.Error(err))
			certExists = false
		} else {
			// 解析证书以获取过期时间
			x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				m.logger.Warn("解析证书失败，将重新生成", zap.Error(err))
				certExists = false
			} else {
				// 检查是否即将过期
				timeUntilExpiry := x509Cert.NotAfter.Sub(time.Now())
				if timeUntilExpiry <= m.config.RenewThreshold {
					m.logger.Info("证书即将过期，将重新生成",
						zap.Time("expiry", x509Cert.NotAfter),
						zap.Duration("time_until_expiry", timeUntilExpiry))
					certExists = false
				} else {
					m.certificate = &cert
					m.logger.Info("使用现有证书",
						zap.Time("expiry", x509Cert.NotAfter),
						zap.Duration("time_until_expiry", timeUntilExpiry))
				}
			}
		}
	}

	// 如果证书不存在或无效，生成新证书
	if !certExists {
		// 解析IP地址
		var ips []net.IP
		for _, ipStr := range m.config.IPAddresses {
			ip := net.ParseIP(ipStr)
			if ip != nil {
				ips = append(ips, ip)
			} else {
				m.logger.Warn("无效的IP地址", zap.String("ip", ipStr))
			}
		}

		// 生成证书
		err := m.generator.GenerateSelfSignedCert(
			m.config.CommonName,
			ips,
			m.config.DNSNames,
			m.config.ValidDuration,
			m.certFile,
			m.keyFile,
		)
		if err != nil {
			return fmt.Errorf("生成自签名证书失败: %w", err)
		}

		// 加载新生成的证书
		cert, err := tls.LoadX509KeyPair(m.certFile, m.keyFile)
		if err != nil {
			return fmt.Errorf("加载新生成的证书失败: %w", err)
		}

		m.certificate = &cert
		m.logger.Info("生成并加载了新证书")
	}

	return nil
}

// scheduleRenewal 安排证书更新
func (m *AutoCertManager) scheduleRenewal() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 停止现有定时器
	if m.renewTimer != nil {
		m.renewTimer.Stop()
	}

	// 如果证书未初始化，立即返回
	if m.certificate == nil {
		m.logger.Warn("无法安排证书更新，证书未初始化")
		return
	}

	// 解析证书以获取过期时间
	x509Cert, err := x509.ParseCertificate(m.certificate.Certificate[0])
	if err != nil {
		m.logger.Error("解析证书失败，无法安排更新", zap.Error(err))
		return
	}

	// 计算更新时间
	timeUntilExpiry := x509Cert.NotAfter.Sub(time.Now())
	renewTime := timeUntilExpiry - m.config.RenewThreshold
	if renewTime < 0 {
		renewTime = 0
	}

	// 创建定时器
	m.renewTimer = time.AfterFunc(renewTime, func() {
		if err := m.ensureCertificate(); err != nil {
			m.logger.Error("证书更新失败", zap.Error(err))
		} else {
			m.logger.Info("证书已更新")
		}
		m.scheduleRenewal()
	})

	m.logger.Info("证书更新已安排",
		zap.Time("expiry", x509Cert.NotAfter),
		zap.Duration("time_until_renewal", renewTime))
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
