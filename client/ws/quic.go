package ws

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"gkipass/client/config"
	"gkipass/client/logger"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

// QUICClient QUIC客户端（支持0-RTT）
type QUICClient struct {
	config     *config.PlaneConfig
	quicConn   quic.Connection
	stream     quic.Stream
	tlsConfig  *tls.Config
	enable0RTT bool
}

// NewQUICClient 创建QUIC客户端
func NewQUICClient(cfg *config.PlaneConfig, tlsConfig *tls.Config) *QUICClient {
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"gkipass-v1"},
		}
	}

	return &QUICClient{
		config:     cfg,
		tlsConfig:  tlsConfig,
		enable0RTT: true,
	}
}

// ConnectWithEarlyData 使用0-RTT连接
func (qc *QUICClient) ConnectWithEarlyData(earlyData []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(qc.config.Timeout)*time.Second)
	defer cancel()

	quicConfig := &quic.Config{
		MaxIdleTimeout:  300 * time.Second,
		KeepAlivePeriod: 30 * time.Second,
		EnableDatagrams: true,
		Allow0RTT:       true, // 启用0-RTT
	}

	logger.Info("QUIC连接中（0-RTT）",
		zap.String("server", qc.config.URL),
		zap.Bool("0-rtt", qc.enable0RTT))

	// 建立QUIC连接
	conn, err := quic.DialAddr(ctx, qc.config.URL, qc.tlsConfig, quicConfig)
	if err != nil {
		return fmt.Errorf("QUIC连接失败: %w", err)
	}

	qc.quicConn = conn

	// QUIC原生支持0-RTT
	logger.Info("✅ QUIC连接已建立（原生支持0-RTT）")

	// 打开双向流
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("打开流失败: %w", err)
	}

	qc.stream = stream

	// 如果有early data，立即发送
	if len(earlyData) > 0 {
		if _, err := stream.Write(earlyData); err != nil {
			logger.Warn("发送early data失败", zap.Error(err))
		} else {
			logger.Info("Early data已发送", zap.Int("bytes", len(earlyData)))
		}
	}

	return nil
}

// Send 发送消息
func (qc *QUICClient) Send(data []byte) error {
	if qc.stream == nil {
		return fmt.Errorf("stream未建立")
	}

	_, err := qc.stream.Write(data)
	return err
}

// Receive 接收消息
func (qc *QUICClient) Receive(buf []byte) (int, error) {
	if qc.stream == nil {
		return 0, fmt.Errorf("stream未建立")
	}

	return qc.stream.Read(buf)
}

// Close 关闭连接
func (qc *QUICClient) Close() error {
	if qc.stream != nil {
		qc.stream.Close()
	}

	if qc.quicConn != nil {
		return qc.quicConn.CloseWithError(0, "client closing")
	}

	return nil
}

// Is0RTTUsed QUIC原生支持0-RTT
func (qc *QUICClient) Is0RTTUsed() bool {
	return qc.quicConn != nil
}
