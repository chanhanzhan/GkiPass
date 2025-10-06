//go:build !linux
// +build !linux

package core

import (
	"io"
	"net"
	"sync"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

const (
	// 缓冲区大小配置
	minBufferSize     = 16 * 1024  // 16KB
	defaultBufferSize = 64 * 1024  // 64KB
	maxBufferSize     = 256 * 1024 // 256KB

	// 性能监控阈值
	slowCopyThreshold = 100 * time.Millisecond
)

// bufferPool 为非Linux平台提供缓冲区池
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, defaultBufferSize)
		return &buf
	},
}

// ZeroCopy 非Linux平台的优化实现
// 使用缓冲池和智能缓冲区大小，提供比标准io.Copy更好的性能
func ZeroCopy(dst, src net.Conn) (int64, error) {
	// 尝试使用 ReadFrom 优化（如果连接支持）
	if rf, ok := dst.(io.ReaderFrom); ok {
		return zeroCopyWithReadFrom(rf, src)
	}

	// 使用优化的缓冲复制
	return optimizedBufferCopy(dst, src)
}

// zeroCopyWithReadFrom 使用 ReadFrom 接口优化
func zeroCopyWithReadFrom(dst io.ReaderFrom, src net.Conn) (int64, error) {
	startTime := time.Now()
	written, err := dst.ReadFrom(src)
	duration := time.Since(startTime)

	// 记录慢拷贝
	if duration > slowCopyThreshold && written > 0 {
		speed := float64(written) / duration.Seconds() / (1024 * 1024) // MB/s
		logger.Debug("零拷贝传输完成 (ReadFrom)",
			zap.Int64("bytes", written),
			zap.Duration("duration", duration),
			zap.Float64("speed_mbps", speed))
	}

	return written, err
}

// optimizedBufferCopy 使用优化的缓冲区复制
func optimizedBufferCopy(dst net.Conn, src net.Conn) (int64, error) {
	// 从池中获取缓冲区
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	buf := *bufPtr
	var written int64
	startTime := time.Now()
	var reads, writes int64

	for {
		// 读取数据
		nr, er := src.Read(buf)
		reads++

		if nr > 0 {
			// 写入数据
			nw, ew := dst.Write(buf[0:nr])
			writes++

			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = io.ErrShortWrite
				}
			}
			written += int64(nw)

			if ew != nil {
				logTransferStats(written, reads, writes, time.Since(startTime))
				return written, ew
			}

			if nr != nw {
				logTransferStats(written, reads, writes, time.Since(startTime))
				return written, io.ErrShortWrite
			}
		}

		if er != nil {
			if er != io.EOF {
				logTransferStats(written, reads, writes, time.Since(startTime))
			}
			return written, er
		}
	}
}

// logTransferStats 记录传输统计信息
func logTransferStats(written, reads, writes int64, duration time.Duration) {
	if duration > slowCopyThreshold && written > 0 {
		speed := float64(written) / duration.Seconds() / (1024 * 1024) // MB/s
		logger.Debug("零拷贝传输统计 (缓冲模式)",
			zap.Int64("bytes", written),
			zap.Int64("reads", reads),
			zap.Int64("writes", writes),
			zap.Duration("duration", duration),
			zap.Float64("speed_mbps", speed))
	}
}

// AdaptiveZeroCopy 自适应零拷贝，根据传输大小动态调整缓冲区
func AdaptiveZeroCopy(dst, src net.Conn, estimatedSize int64) (int64, error) {
	// 根据预期大小选择缓冲区大小
	bufferSize := defaultBufferSize

	if estimatedSize > 0 {
		if estimatedSize < int64(minBufferSize) {
			bufferSize = minBufferSize
		} else if estimatedSize > int64(maxBufferSize) {
			bufferSize = maxBufferSize
		} else {
			// 使用预期大小的一半作为缓冲区大小
			bufferSize = int(estimatedSize / 2)
			if bufferSize < minBufferSize {
				bufferSize = minBufferSize
			}
		}
	}

	// 尝试使用 ReadFrom 优化
	if rf, ok := dst.(io.ReaderFrom); ok {
		return zeroCopyWithReadFrom(rf, src)
	}

	// 使用自定义大小的缓冲区
	buf := make([]byte, bufferSize)
	return io.CopyBuffer(dst, src, buf)
}

// GetZeroCopyBufferStats 获取零拷贝缓冲池统计信息（用于监控）
type ZeroCopyBufferStats struct {
	BufferSize    int
	MinBufferSize int
	MaxBufferSize int
}

func GetZeroCopyBufferStats() ZeroCopyBufferStats {
	return ZeroCopyBufferStats{
		BufferSize:    defaultBufferSize,
		MinBufferSize: minBufferSize,
		MaxBufferSize: maxBufferSize,
	}
}
