package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ForwarderConfig 转发器配置
type ForwarderConfig struct {
	BufferSize        int           // 缓冲区大小
	IdleTimeout       time.Duration // 空闲超时
	MaxConnections    int           // 最大连接数
	EnableCompression bool          // 启用压缩
	RateLimitBPS      int64         // 速率限制（字节/秒）
	Logger            *zap.Logger   // 日志记录器
}

// DefaultForwarderConfig 默认转发器配置
func DefaultForwarderConfig() *ForwarderConfig {
	return &ForwarderConfig{
		BufferSize:        32 * 1024,
		IdleTimeout:       5 * time.Minute,
		MaxConnections:    1000,
		EnableCompression: false,
		RateLimitBPS:      0, // 不限速
		Logger:            zap.L().Named("forwarder"),
	}
}

// ForwarderStats 转发器统计信息
type ForwarderStats struct {
	ActiveConnections int64
	TotalConnections  int64
	BytesIn           int64
	BytesOut          int64
	StartTime         time.Time
}

// Forwarder 流量转发器
type Forwarder struct {
	config     *ForwarderConfig
	classifier *Classifier
	stats      ForwarderStats
	sessions   sync.Map
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *zap.Logger
}

// ForwardSession 转发会话
type ForwardSession struct {
	ID             string
	ClientConn     net.Conn
	TargetConn     net.Conn
	TrafficType    TrafficType
	BytesIn        int64
	BytesOut       int64
	StartTime      time.Time
	LastActiveTime time.Time
	closed         bool
	mutex          sync.Mutex
}

// NewForwarder 创建流量转发器
func NewForwarder(config *ForwarderConfig) *Forwarder {
	if config == nil {
		config = DefaultForwarderConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	classifierConfig := DefaultClassifierConfig()
	classifierConfig.Logger = config.Logger.Named("classifier")

	forwarder := &Forwarder{
		config:     config,
		classifier: NewClassifier(classifierConfig),
		stats: ForwarderStats{
			StartTime: time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
		logger: config.Logger,
	}

	// 启动清理过期会话的协程
	go forwarder.cleanupSessions()

	return forwarder
}

// Start 启动转发器
func (f *Forwarder) Start() error {
	f.logger.Info("流量转发器启动")
	return nil
}

// Stop 停止转发器
func (f *Forwarder) Stop() error {
	f.cancel()
	f.logger.Info("流量转发器停止")
	return nil
}

// Forward 转发连接
func (f *Forwarder) Forward(clientConn net.Conn, targetAddr string) error {
	// 检查最大连接数
	if f.config.MaxConnections > 0 && atomic.LoadInt64(&f.stats.ActiveConnections) >= int64(f.config.MaxConnections) {
		f.logger.Warn("达到最大连接数限制",
			zap.Int("max_connections", f.config.MaxConnections))
		return fmt.Errorf("达到最大连接数限制: %d", f.config.MaxConnections)
	}

	// 创建会话ID
	sessionID := fmt.Sprintf("%s-%d", clientConn.RemoteAddr().String(), time.Now().UnixNano())

	// 分类流量
	result, err := f.classifier.Classify(clientConn)
	if err != nil {
		f.logger.Error("流量分类失败",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return err
	}

	f.logger.Info("流量分类结果",
		zap.String("session_id", sessionID),
		zap.String("traffic_type", string(result.TrafficType)),
		zap.String("protocol", string(result.Protocol)),
		zap.Float64("confidence", result.Confidence))

	// 连接目标
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		f.logger.Error("连接目标失败",
			zap.String("session_id", sessionID),
			zap.String("target_addr", targetAddr),
			zap.Error(err))
		return err
	}

	// 创建会话
	session := &ForwardSession{
		ID:             sessionID,
		ClientConn:     clientConn,
		TargetConn:     targetConn,
		TrafficType:    result.TrafficType,
		StartTime:      time.Now(),
		LastActiveTime: time.Now(),
	}

	// 保存会话
	f.sessions.Store(sessionID, session)

	// 更新统计信息
	atomic.AddInt64(&f.stats.ActiveConnections, 1)
	atomic.AddInt64(&f.stats.TotalConnections, 1)

	f.logger.Info("开始转发",
		zap.String("session_id", sessionID),
		zap.String("client_addr", clientConn.RemoteAddr().String()),
		zap.String("target_addr", targetAddr),
		zap.String("traffic_type", string(result.TrafficType)))

	// 如果有缓冲区数据，先发送
	if len(result.Buffer) > 0 {
		if _, err := targetConn.Write(result.Buffer); err != nil {
			f.logger.Error("发送缓冲区数据失败",
				zap.String("session_id", sessionID),
				zap.Error(err))
			f.closeSession(session)
			return err
		}
		atomic.AddInt64(&session.BytesOut, int64(len(result.Buffer)))
		atomic.AddInt64(&f.stats.BytesOut, int64(len(result.Buffer)))
	}

	// 双向转发
	errCh := make(chan error, 2)

	// 客户端 -> 目标
	go func() {
		err := f.pipe(session, clientConn, targetConn, &session.BytesIn, &f.stats.BytesIn)
		errCh <- err
	}()

	// 目标 -> 客户端
	go func() {
		err := f.pipe(session, targetConn, clientConn, &session.BytesOut, &f.stats.BytesOut)
		errCh <- err
	}()

	// 等待任一方向出错或关闭
	err = <-errCh
	f.closeSession(session)

	if err != nil && err != io.EOF {
		f.logger.Error("转发出错",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return err
	}

	f.logger.Info("转发结束",
		zap.String("session_id", sessionID),
		zap.Int64("bytes_in", atomic.LoadInt64(&session.BytesIn)),
		zap.Int64("bytes_out", atomic.LoadInt64(&session.BytesOut)),
		zap.Duration("duration", time.Since(session.StartTime)))

	return nil
}

// pipe 管道传输数据
func (f *Forwarder) pipe(session *ForwardSession, src, dst net.Conn, byteCounter *int64, totalCounter *int64) error {
	buffer := make([]byte, f.config.BufferSize)

	for {
		// 设置读取超时
		if f.config.IdleTimeout > 0 {
			src.SetReadDeadline(time.Now().Add(f.config.IdleTimeout))
		}

		// 读取数据
		n, err := src.Read(buffer)
		if err != nil {
			if err == io.EOF {
				return err
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				f.logger.Debug("连接读取超时",
					zap.String("session_id", session.ID))
				return fmt.Errorf("连接读取超时: %w", err)
			}
			return err
		}

		// 更新活动时间
		session.LastActiveTime = time.Now()

		// 应用速率限制
		if f.config.RateLimitBPS > 0 {
			// 简单的令牌桶实现
			// 计算需要等待的时间
			bytesPerSecond := float64(f.config.RateLimitBPS)
			waitTime := time.Duration(float64(n) / bytesPerSecond * float64(time.Second))
			time.Sleep(waitTime)
		}

		// 写入数据
		_, err = dst.Write(buffer[:n])
		if err != nil {
			return err
		}

		// 更新计数器
		atomic.AddInt64(byteCounter, int64(n))
		atomic.AddInt64(totalCounter, int64(n))
	}
}

// closeSession 关闭会话
func (f *Forwarder) closeSession(session *ForwardSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	if session.closed {
		return
	}

	session.ClientConn.Close()
	session.TargetConn.Close()
	session.closed = true

	f.sessions.Delete(session.ID)
	atomic.AddInt64(&f.stats.ActiveConnections, -1)
}

// cleanupSessions 清理过期会话
func (f *Forwarder) cleanupSessions() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			var expiredCount int

			f.sessions.Range(func(key, value interface{}) bool {
				session := value.(*ForwardSession)

				// 如果设置了空闲超时并且会话已超时
				if f.config.IdleTimeout > 0 && now.Sub(session.LastActiveTime) > f.config.IdleTimeout {
					f.logger.Debug("关闭空闲会话",
						zap.String("session_id", session.ID),
						zap.Duration("idle_time", now.Sub(session.LastActiveTime)))
					f.closeSession(session)
					expiredCount++
				}

				return true
			})

			if expiredCount > 0 {
				f.logger.Info("清理过期会话",
					zap.Int("expired_count", expiredCount))
			}
		}
	}
}

// GetStats 获取统计信息
func (f *Forwarder) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"active_connections": atomic.LoadInt64(&f.stats.ActiveConnections),
		"total_connections":  atomic.LoadInt64(&f.stats.TotalConnections),
		"bytes_in":           atomic.LoadInt64(&f.stats.BytesIn),
		"bytes_out":          atomic.LoadInt64(&f.stats.BytesOut),
		"uptime":             time.Since(f.stats.StartTime).Seconds(),
	}
}

// CreateForwarderFromConfig 从配置创建转发器
func CreateForwarderFromConfig(cfg interface{}) *Forwarder {
	forwarderConfig := &ForwarderConfig{
		BufferSize:        32 * 1024,
		IdleTimeout:       5 * time.Minute,
		MaxConnections:    1000,
		EnableCompression: false,
		RateLimitBPS:      0, // 不限速
		Logger:            zap.L().Named("forwarder"),
	}

	return NewForwarder(forwarderConfig)
}
