package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"gkipass/plane/internal/config"
)

// createTLSConfig 创建 TLS 配置
// 注意：优先使用 TLS 1.3 以缓解 Terrapin 攻击风险
// TLS 1.3 的 AEAD 加密套件（如 ChaCha20-Poly1305）不受 Terrapin 攻击影响
func createTLSConfig(cfg *config.Config) *tls.Config {
	tlsConfig := &tls.Config{
		// 优先使用 TLS 1.3 以获得更好的安全性
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.X25519, // 最安全和最快的曲线
			tls.CurveP256,
			tls.CurveP384,
		},
		// TLS 1.3 加密套件（按安全性和性能排序）
		CipherSuites: []uint16{
			// TLS 1.3 套件（更安全，不受 Terrapin 攻击影响）
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256, // TLS 1.3 中安全
			// TLS 1.2 回退套件（如果必须支持旧客户端）
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		PreferServerCipherSuites: true,
		SessionTicketsDisabled:   false,                // 启用会话恢复以提高性能
		Renegotiation:            tls.RenegotiateNever, // 禁用重协商以提高安全性
	}

	// 允许降级到 TLS 1.2 以兼容旧客户端（如果配置允许）
	if cfg.TLS.MinVersion == "TLS 1.2" {
		tlsConfig.MinVersion = tls.VersionTLS12
		tlsConfig.MaxVersion = 0 // 0 表示使用最高可用版本
	}

	// 启用 ALPN (Application-Layer Protocol Negotiation)
	if cfg.TLS.EnableALPN {
		tlsConfig.NextProtos = []string{
			"h3",       // HTTP/3
			"h3-29",    // HTTP/3 draft 29
			"h2",       // HTTP/2
			"http/1.1", // HTTP/1.1
		}

	}

	// 加载 CA 证书（如果提供）
	if cfg.TLS.CAFile != "" {
		caCert, err := os.ReadFile(cfg.TLS.CAFile)
		if err == nil {
			caCertPool := x509.NewCertPool()
			if caCertPool.AppendCertsFromPEM(caCert) {
				tlsConfig.RootCAs = caCertPool
			}
		} else {
		}
	}

	return tlsConfig
}
