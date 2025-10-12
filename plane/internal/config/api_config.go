package config

import (
	"time"
)

// APIConfig API服务器配置
type APIConfig struct {
	// 监听配置
	ListenAddr string `json:"listen_addr" yaml:"listen_addr"`
	ListenPort int    `json:"listen_port" yaml:"listen_port"`

	// 超时配置
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout" yaml:"idle_timeout"`

	// TLS配置
	EnableTLS     bool   `json:"enable_tls" yaml:"enable_tls"`
	TLSCertFile   string `json:"tls_cert_file" yaml:"tls_cert_file"`
	TLSKeyFile    string `json:"tls_key_file" yaml:"tls_key_file"`
	TLSClientCA   string `json:"tls_client_ca" yaml:"tls_client_ca"`
	TLSMinVersion string `json:"tls_min_version" yaml:"tls_min_version"`

	// CORS配置
	CORSEnabled        bool     `json:"cors_enabled" yaml:"cors_enabled"`
	CORSAllowedOrigins []string `json:"cors_allowed_origins" yaml:"cors_allowed_origins"`

	// 日志配置
	LogRequests bool   `json:"log_requests" yaml:"log_requests"`
	LogLevel    string `json:"log_level" yaml:"log_level"`

	// 限流配置
	RateLimitEnabled bool `json:"rate_limit_enabled" yaml:"rate_limit_enabled"`
	RateLimit        int  `json:"rate_limit" yaml:"rate_limit"`
	RateBurst        int  `json:"rate_burst" yaml:"rate_burst"`
	RateLimitByIP    bool `json:"rate_limit_by_ip" yaml:"rate_limit_by_ip"`
}

// DefaultAPIConfig 默认API服务器配置
func DefaultAPIConfig() *APIConfig {
	return &APIConfig{
		ListenAddr:         "0.0.0.0",
		ListenPort:         8080,
		ReadTimeout:        15 * time.Second,
		WriteTimeout:       15 * time.Second,
		IdleTimeout:        60 * time.Second,
		EnableTLS:          false,
		TLSMinVersion:      "1.2",
		CORSEnabled:        true,
		CORSAllowedOrigins: []string{"*"},
		LogRequests:        true,
		LogLevel:           "info",
		RateLimitEnabled:   true,
		RateLimit:          100, // 每秒请求数
		RateBurst:          200, // 突发请求数
		RateLimitByIP:      true,
	}
}
