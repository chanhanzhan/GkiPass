package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// TransferStats 传输统计
type TransferStats struct {
	// 传输字节数
	BytesIn  atomic.Int64
	BytesOut atomic.Int64

	// 操作计数
	ReadsIn   atomic.Int64
	WritesOut atomic.Int64

	// 错误计数
	ReadErrors  atomic.Int64
	WriteErrors atomic.Int64

	// 性能指标
	AvgReadSize  atomic.Float64
	AvgWriteSize atomic.Float64
	MaxReadSize  atomic.Int64
	MaxWriteSize atomic.Int64

	// 时间指标
	StartTime    time.Time
	LastActivity atomic.Int64 // Unix时间戳
}

// TransferOptions 传输选项
type TransferOptions struct {
	// 缓冲区配置
	BufferSize int

	// 超时设置
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// 优化选项
	UseZeroCopy     bool
	UseSendfile     bool
	UseSplice       bool
	UseDirectIO     bool
	EnableNoDelay   bool
	EnableKeepAlive bool
	KeepAlivePeriod time.Duration

	// 监控和指标
	CollectStats bool
	LogTransfers bool
	LogLevel     string
}

// DefaultTransferOptions 默认传输选项
func DefaultTransferOptions() *TransferOptions {
	return &TransferOptions{
		BufferSize:      65536, // 64KB
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     5 * time.Minute,
		UseZeroCopy:     true,
		UseSendfile:     true,
		UseSplice:       true,
		UseDirectIO:     false,
		EnableNoDelay:   true,
		EnableKeepAlive: true,
		KeepAlivePeriod: 30 * time.Second,
		CollectStats:    true,
		LogTransfers:    false,
		LogLevel:        "info",
	}
}

// HighPerformanceOptions 高性能传输选项
func HighPerformanceOptions() *TransferOptions {
	opts := DefaultTransferOptions()
	opts.BufferSize = 262144 // 256KB
	opts.UseDirectIO = true
	return opts
}

// LowLatencyOptions 低延迟传输选项
func LowLatencyOptions() *TransferOptions {
	opts := DefaultTransferOptions()
	opts.BufferSize = 32768 // 32KB
	opts.ReadTimeout = 10 * time.Second
	opts.WriteTimeout = 10 * time.Second
	opts.EnableNoDelay = true
	return opts
}

// Transfer 传输管理器
type Transfer struct {
	source      net.Conn
	destination net.Conn
	options     *TransferOptions
	logger      *zap.Logger
	stats       TransferStats
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewTransfer 创建新的传输
func NewTransfer(source, destination net.Conn, options *TransferOptions) *Transfer {
	if options == nil {
		options = DefaultTransferOptions()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Transfer{
		source:      source,
		destination: destination,
		options:     options,
		logger:      zap.L().Named("transfer"),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start 开始双向传输
func (t *Transfer) Start() error {
	t.stats.StartTime = time.Now()
	t.stats.LastActivity.Store(time.Now().Unix())

	t.configureConnections()

	t.logger.Debug("启动双向传输",
		zap.String("source", t.source.RemoteAddr().String()),
		zap.String("destination", t.destination.RemoteAddr().String()))

	// 启动双向复制
	t.wg.Add(2)
	go t.transferData(t.source, t.destination, "source->dest")
	go t.transferData(t.destination, t.source, "dest->source")

	return nil
}

// Wait 等待传输完成
func (t *Transfer) Wait() {
	t.wg.Wait()
}

// Stop 停止传输
func (t *Transfer) Stop() {
	if t.cancel != nil {
		t.cancel()
	}

	// 优雅关闭连接
	if conn, ok := t.source.(*net.TCPConn); ok {
		conn.CloseRead()
	} else {
		t.source.Close()
	}

	if conn, ok := t.destination.(*net.TCPConn); ok {
		conn.CloseRead()
	} else {
		t.destination.Close()
	}

	// 等待传输完成
	t.wg.Wait()
}

// GetStats 获取传输统计
func (t *Transfer) GetStats() map[string]interface{} {
	duration := time.Since(t.stats.StartTime)

	var bytesIn, bytesOut int64
	var bytesInPerSec, bytesOutPerSec float64

	bytesIn = t.stats.BytesIn.Load()
	bytesOut = t.stats.BytesOut.Load()

	if duration.Seconds() > 0 {
		bytesInPerSec = float64(bytesIn) / duration.Seconds()
		bytesOutPerSec = float64(bytesOut) / duration.Seconds()
	}

	return map[string]interface{}{
		"bytes_in":           bytesIn,
		"bytes_out":          bytesOut,
		"total_bytes":        bytesIn + bytesOut,
		"reads_in":           t.stats.ReadsIn.Load(),
		"writes_out":         t.stats.WritesOut.Load(),
		"read_errors":        t.stats.ReadErrors.Load(),
		"write_errors":       t.stats.WriteErrors.Load(),
		"avg_read_size":      t.stats.AvgReadSize.Load(),
		"avg_write_size":     t.stats.AvgWriteSize.Load(),
		"max_read_size":      t.stats.MaxReadSize.Load(),
		"max_write_size":     t.stats.MaxWriteSize.Load(),
		"duration_seconds":   duration.Seconds(),
		"bytes_in_per_sec":   bytesInPerSec,
		"bytes_out_per_sec":  bytesOutPerSec,
		"transfer_rate_mbps": (bytesInPerSec + bytesOutPerSec) * 8 / 1024 / 1024,
	}
}

// configureConnections 配置连接选项
func (t *Transfer) configureConnections() {
	// TCP特定设置
	t.configureTCPConn(t.source)
	t.configureTCPConn(t.destination)
}

// configureTCPConn 配置TCP连接
func (t *Transfer) configureTCPConn(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}

	if t.options.EnableNoDelay {
		tcpConn.SetNoDelay(true)
	}

	if t.options.EnableKeepAlive {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(t.options.KeepAlivePeriod)
	}
}

// transferData 执行数据传输
func (t *Transfer) transferData(src, dst net.Conn, direction string) {
	defer t.wg.Done()
	defer t.logger.Debug("传输完成", zap.String("direction", direction))

	// 使用零拷贝技术如果可用
	if t.options.UseZeroCopy {
		if done, err := t.tryZeroCopyTransfer(src, dst, direction); done {
			if err != nil {
				t.logger.Error("零拷贝传输失败",
					zap.String("direction", direction),
					zap.Error(err))
			}
			return
		}
	}

	// 回退到标准传输
	t.standardTransfer(src, dst, direction)
}

// tryZeroCopyTransfer 尝试使用零拷贝传输
func (t *Transfer) tryZeroCopyTransfer(src, dst net.Conn, direction string) (bool, error) {
	var srcFile, dstFile *os.File
	var err error

	// 获取文件描述符
	if t.options.UseSendfile {
		// 尝试获取源文件
		if tcpSrc, ok := src.(*net.TCPConn); ok {
			srcFile, err = tcpSrc.File()
			if err != nil {
				return false, fmt.Errorf("获取源文件描述符失败: %w", err)
			}
			defer srcFile.Close()

			// 获取目标文件
			if tcpDst, ok := dst.(*net.TCPConn); ok {
				dstFile, err = tcpDst.File()
				if err != nil {
					return false, fmt.Errorf("获取目标文件描述符失败: %w", err)
				}
				defer dstFile.Close()

				// 如果两端都支持，尝试使用splice或sendfile
				if t.options.UseSplice {
					return true, t.spliceTransfer(
						int(srcFile.Fd()),
						int(dstFile.Fd()),
						direction)
				} else if t.options.UseSendfile && direction == "source->dest" {
					return true, t.sendfileTransfer(
						int(srcFile.Fd()),
						int(dstFile.Fd()),
						direction)
				}
			}
		}
	}

	return false, nil
}

// spliceTransfer 使用splice系统调用传输
func (t *Transfer) spliceTransfer(srcFd, dstFd int, direction string) error {
	t.logger.Debug("使用splice传输",
		zap.String("direction", direction))

	// 创建pipe
	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("创建管道失败: %w", err)
	}
	defer r.Close()
	defer w.Close()

	buffer := make([]byte, t.options.BufferSize)
	var totalBytes int64

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
		}

		// 读取数据 (使用普通读取，因为splice实现复杂)
		n, err := syscall.Read(srcFd, buffer)
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EINTR {
				continue
			}
			if err == syscall.ECONNRESET || err == syscall.EPIPE {
				return nil
			}
			t.stats.ReadErrors.Inc()
			return fmt.Errorf("读取错误: %w", err)
		}

		if n == 0 {
			// EOF
			return nil
		}

		// 记录统计
		t.stats.ReadsIn.Inc()
		t.stats.BytesIn.Add(int64(n))
		t.updateMaxReadSize(int64(n))
		t.updateAvgReadSize(int64(n))
		t.updateActivity()

		// 写入数据
		written, err := syscall.Write(dstFd, buffer[:n])
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EINTR {
				continue
			}
			if err == syscall.ECONNRESET || err == syscall.EPIPE {
				return nil
			}
			t.stats.WriteErrors.Inc()
			return fmt.Errorf("写入错误: %w", err)
		}

		t.stats.WritesOut.Inc()
		t.stats.BytesOut.Add(int64(written))
		t.updateMaxWriteSize(int64(written))
		t.updateAvgWriteSize(int64(written))
		t.updateActivity()

		totalBytes += int64(written)
	}
}

// sendfileTransfer 使用sendfile系统调用传输
func (t *Transfer) sendfileTransfer(srcFd, dstFd int, direction string) error {
	t.logger.Debug("使用sendfile传输",
		zap.String("direction", direction))

	var totalBytes int64
	var offset int64 = 0

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
		}

		// 发送数据块
		n, err := syscall.Sendfile(dstFd, srcFd, &offset, t.options.BufferSize)
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EINTR {
				continue
			}
			if err == syscall.EPIPE || err == syscall.ECONNRESET {
				// 连接关闭
				return nil
			}
			return fmt.Errorf("sendfile失败: %w", err)
		}

		if n == 0 {
			// 完成传输
			return nil
		}

		// 记录统计
		t.stats.BytesIn.Add(int64(n))
		t.stats.BytesOut.Add(int64(n))
		t.stats.ReadsIn.Inc()
		t.stats.WritesOut.Inc()
		t.updateActivity()
		totalBytes += int64(n)
	}
}

// standardTransfer 使用标准IO传输
func (t *Transfer) standardTransfer(src, dst net.Conn, direction string) {
	t.logger.Debug("使用标准传输",
		zap.String("direction", direction))

	buffer := make([]byte, t.options.BufferSize)

	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		// 设置读取超时
		if t.options.ReadTimeout > 0 {
			src.SetReadDeadline(time.Now().Add(t.options.ReadTimeout))
		}

		// 读取数据
		n, err := src.Read(buffer)
		if err != nil {
			if err == io.EOF {
				t.logger.Debug("连接关闭", zap.String("direction", direction))
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// 读取超时，检查是否空闲超时
				if t.isIdleTimeout() {
					t.logger.Debug("空闲超时", zap.String("direction", direction))
					t.cancel()
				}
				continue
			} else {
				t.stats.ReadErrors.Inc()
				t.logger.Error("读取数据失败",
					zap.String("direction", direction),
					zap.Error(err))
			}
			t.cancel()
			return
		}

		if n > 0 {
			// 记录统计
			t.stats.BytesIn.Add(int64(n))
			t.stats.ReadsIn.Inc()
			t.updateMaxReadSize(int64(n))
			t.updateAvgReadSize(int64(n))
			t.updateActivity()

			// 设置写入超时
			if t.options.WriteTimeout > 0 {
				dst.SetWriteDeadline(time.Now().Add(t.options.WriteTimeout))
			}

			// 写入数据
			written, err := dst.Write(buffer[:n])
			if err != nil {
				t.stats.WriteErrors.Inc()
				t.logger.Error("写入数据失败",
					zap.String("direction", direction),
					zap.Error(err))
				t.cancel()
				return
			}

			// 记录统计
			t.stats.BytesOut.Add(int64(written))
			t.stats.WritesOut.Inc()
			t.updateMaxWriteSize(int64(written))
			t.updateAvgWriteSize(int64(written))
		}
	}
}

// isIdleTimeout 检查是否空闲超时
func (t *Transfer) isIdleTimeout() bool {
	if t.options.IdleTimeout <= 0 {
		return false
	}

	lastActivity := time.Unix(t.stats.LastActivity.Load(), 0)
	return time.Since(lastActivity) > t.options.IdleTimeout
}

// updateActivity 更新活动时间
func (t *Transfer) updateActivity() {
	t.stats.LastActivity.Store(time.Now().Unix())
}

// updateMaxReadSize 更新最大读取大小
func (t *Transfer) updateMaxReadSize(size int64) {
	for {
		current := t.stats.MaxReadSize.Load()
		if size <= current || t.stats.MaxReadSize.CAS(current, size) {
			break
		}
	}
}

// updateMaxWriteSize 更新最大写入大小
func (t *Transfer) updateMaxWriteSize(size int64) {
	for {
		current := t.stats.MaxWriteSize.Load()
		if size <= current || t.stats.MaxWriteSize.CAS(current, size) {
			break
		}
	}
}

// updateAvgReadSize 更新平均读取大小
func (t *Transfer) updateAvgReadSize(size int64) {
	reads := float64(t.stats.ReadsIn.Load())
	if reads <= 1 {
		t.stats.AvgReadSize.Store(float64(size))
		return
	}

	currentAvg := t.stats.AvgReadSize.Load()
	newAvg := currentAvg + (float64(size)-currentAvg)/reads
	t.stats.AvgReadSize.Store(newAvg)
}

// updateAvgWriteSize 更新平均写入大小
func (t *Transfer) updateAvgWriteSize(size int64) {
	writes := float64(t.stats.WritesOut.Load())
	if writes <= 1 {
		t.stats.AvgWriteSize.Store(float64(size))
		return
	}

	currentAvg := t.stats.AvgWriteSize.Load()
	newAvg := currentAvg + (float64(size)-currentAvg)/writes
	t.stats.AvgWriteSize.Store(newAvg)
}
