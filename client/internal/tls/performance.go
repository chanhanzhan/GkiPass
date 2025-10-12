package tls

import (
	"crypto/tls"
	"sync"

	"go.uber.org/zap"
)

// PerformanceConfig TLS性能配置
type PerformanceConfig struct {
	SessionCacheSize           int           `json:"session_cache_size"`
	SessionTicketsDisabled     bool          `json:"session_tickets_disabled"`
	PreferServerCipherSuites   bool          `json:"prefer_server_cipher_suites"`
	CurvePreferences           []tls.CurveID `json:"curve_preferences"`
	PreferredCipherSuites      []uint16      `json:"preferred_cipher_suites"`
	DynamicRecordSizingEnabled bool          `json:"dynamic_record_sizing_enabled"`
}

// DefaultPerformanceConfig 默认TLS性能配置
func DefaultPerformanceConfig() *PerformanceConfig {
	return &PerformanceConfig{
		SessionCacheSize:           64,
		SessionTicketsDisabled:     false,
		PreferServerCipherSuites:   true,
		CurvePreferences:           []tls.CurveID{tls.X25519, tls.CurveP256},
		PreferredCipherSuites:      defaultCipherSuites(),
		DynamicRecordSizingEnabled: true,
	}
}

// HighSecurityPerformanceConfig 高安全性TLS性能配置
func HighSecurityPerformanceConfig() *PerformanceConfig {
	return &PerformanceConfig{
		SessionCacheSize:           32,
		SessionTicketsDisabled:     true,
		PreferServerCipherSuites:   true,
		CurvePreferences:           []tls.CurveID{tls.X25519, tls.CurveP256},
		PreferredCipherSuites:      securityFocusedCipherSuites(),
		DynamicRecordSizingEnabled: false,
	}
}

// HighPerformanceConfig 高性能TLS性能配置
func HighPerformanceConfig() *PerformanceConfig {
	return &PerformanceConfig{
		SessionCacheSize:           128,
		SessionTicketsDisabled:     false,
		PreferServerCipherSuites:   false,
		CurvePreferences:           []tls.CurveID{tls.X25519, tls.CurveP256},
		PreferredCipherSuites:      performanceFocusedCipherSuites(),
		DynamicRecordSizingEnabled: true,
	}
}

// PerformanceOptimizer TLS性能优化器
type PerformanceOptimizer struct {
	config       *PerformanceConfig
	sessionCache tls.ClientSessionCache
	mutex        sync.RWMutex
	logger       *zap.Logger
}

// NewPerformanceOptimizer 创建TLS性能优化器
func NewPerformanceOptimizer(config *PerformanceConfig) *PerformanceOptimizer {
	if config == nil {
		config = DefaultPerformanceConfig()
	}

	return &PerformanceOptimizer{
		config:       config,
		sessionCache: tls.NewLRUClientSessionCache(config.SessionCacheSize),
		logger:       zap.L().Named("tls-optimizer"),
	}
}

// OptimizeTLSConfig 优化TLS配置
func (o *PerformanceOptimizer) OptimizeTLSConfig(config *tls.Config) {
	o.mutex.RLock()
	defer o.mutex.RUnlock()

	// 设置会话缓存
	config.ClientSessionCache = o.sessionCache

	// 设置会话票证
	config.SessionTicketsDisabled = o.config.SessionTicketsDisabled

	// 设置密码套件偏好
	config.PreferServerCipherSuites = o.config.PreferServerCipherSuites

	// 设置曲线偏好
	if len(o.config.CurvePreferences) > 0 {
		config.CurvePreferences = o.config.CurvePreferences
	}

	// 设置密码套件
	if len(o.config.PreferredCipherSuites) > 0 {
		config.CipherSuites = o.config.PreferredCipherSuites
	}
}

// UpdateConfig 更新配置
func (o *PerformanceOptimizer) UpdateConfig(config *PerformanceConfig) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	// 如果会话缓存大小变化，创建新的缓存
	if config.SessionCacheSize != o.config.SessionCacheSize {
		o.sessionCache = tls.NewLRUClientSessionCache(config.SessionCacheSize)
	}

	o.config = config
	o.logger.Info("TLS性能配置已更新")
}

// GetStats 获取统计信息
func (o *PerformanceOptimizer) GetStats() map[string]interface{} {
	o.mutex.RLock()
	defer o.mutex.RUnlock()

	return map[string]interface{}{
		"session_cache_size":            o.config.SessionCacheSize,
		"session_tickets_disabled":      o.config.SessionTicketsDisabled,
		"prefer_server_cipher_suites":   o.config.PreferServerCipherSuites,
		"dynamic_record_sizing_enabled": o.config.DynamicRecordSizingEnabled,
		"curve_preferences_count":       len(o.config.CurvePreferences),
		"cipher_suites_count":           len(o.config.PreferredCipherSuites),
	}
}

// 默认密码套件
func defaultCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}
}

// 安全优先密码套件
func securityFocusedCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	}
}

// 性能优先密码套件
func performanceFocusedCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}
}
