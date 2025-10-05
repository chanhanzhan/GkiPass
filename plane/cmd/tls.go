package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"gkipass/plane/internal/config"
)

// createTLSConfig 创建 TLS 配置
func createTLSConfig(cfg *config.Config) *tls.Config {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
		PreferServerCipherSuites: true,
	}

	// 设置 TLS 最低版本
	if cfg.TLS.MinVersion == "TLS 1.3" {
		tlsConfig.MinVersion = tls.VersionTLS13
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
