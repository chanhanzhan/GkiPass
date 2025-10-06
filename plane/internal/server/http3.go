package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"gkipass/plane/pkg/logger"

	"github.com/quic-go/quic-go/http3"
	"go.uber.org/zap"
)

// HTTP3Server HTTP/3 服务器
type HTTP3Server struct {
	server *http3.Server
	addr   string
}

// NewHTTP3Server 创建 HTTP/3 服务器
func NewHTTP3Server(addr string, handler http.Handler, tlsConfig *tls.Config) *HTTP3Server {
	// 配置 ALPN for HTTP/3
	if tlsConfig.NextProtos == nil {
		tlsConfig.NextProtos = []string{"h3", "h3-29"}
	}

	server := &http3.Server{
		Addr:       addr,
		Handler:    handler,
		TLSConfig:  tlsConfig,
		QUICConfig: nil, // 使用默认 QUIC 配置
	}

	return &HTTP3Server{
		server: server,
		addr:   addr,
	}
}

// Start 启动 HTTP/3 服务器
func (s *HTTP3Server) Start() error {
	logger.Info("🚀 HTTP/3 服务器启动",
		zap.String("addr", s.addr),
		zap.String("protocol", "QUIC/HTTP3"))

	if err := s.server.ListenAndServe(); err != nil {
		return fmt.Errorf("HTTP/3 server error: %w", err)
	}

	return nil
}

// Shutdown 优雅关闭服务器
func (s *HTTP3Server) Shutdown(ctx context.Context) error {
	logger.Info("🛑 正在关闭 HTTP/3 服务器...")
	return s.server.Close()
}

// HTTP2Server HTTP/2 服务器 (使用标准 http.Server)
type HTTP2Server struct {
	server *http.Server
}

// NewHTTP2Server 创建 HTTP/2 服务器
func NewHTTP2Server(addr string, handler http.Handler, tlsConfig *tls.Config, readTimeout, writeTimeout time.Duration) *HTTP2Server {
	// 配置 ALPN for HTTP/2
	if tlsConfig != nil {
		if tlsConfig.NextProtos == nil {
			tlsConfig.NextProtos = []string{"h2", "http/1.1"}
		}
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		TLSConfig:    tlsConfig,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	return &HTTP2Server{
		server: server,
	}
}

// Start 启动 HTTP/2 服务器
func (s *HTTP2Server) Start(certFile, keyFile string) error {
	logger.Info("🚀 HTTP/2 服务器启动",
		zap.String("addr", s.server.Addr),
		zap.String("protocol", "TLS/HTTP2"))

	if err := s.server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP/2 server error: %w", err)
	}

	return nil
}

// StartInsecure 启动不安全的 HTTP 服务器（用于开发）
func (s *HTTP2Server) StartInsecure() error {
	logger.Warn("⚠️  HTTP 服务器启动 (非加密模式)",
		zap.String("addr", s.server.Addr))

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}

	return nil
}

// Shutdown 优雅关闭服务器
func (s *HTTP2Server) Shutdown(ctx context.Context) error {
	logger.Info("🛑 正在关闭 HTTP/2 服务器...")
	return s.server.Shutdown(ctx)
}
