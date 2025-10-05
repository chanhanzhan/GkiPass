package core

import (
	"context"
	"net"
	"syscall"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// OptimizeTCPConn 优化TCP连接参数
func OptimizeTCPConn(conn net.Conn) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}

	// 禁用Nagle算法（减少延迟）
	if err := tcpConn.SetNoDelay(true); err != nil {
		logger.Warn("设置TCP_NODELAY失败", zap.Error(err))
	}

	// 启用Keepalive
	if err := tcpConn.SetKeepAlive(true); err != nil {
		logger.Warn("设置TCP Keepalive失败", zap.Error(err))
	}

	// Keepalive间隔30秒
	// Note: SetKeepAlivePeriod需要time.Duration
	// if err := tcpConn.SetKeepAlivePeriod(30 * time.Second); err != nil {
	// 	logger.Warn("设置Keepalive间隔失败", zap.Error(err))
	// }

	// 设置大缓冲区（Linux系统）
	if rawConn, err := tcpConn.SyscallConn(); err == nil {
		rawConn.Control(func(fd uintptr) {
			// 设置发送缓冲区为1MB
			syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, 1024*1024)
			// 设置接收缓冲区为1MB
			syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 1024*1024)
			// 启用TCP快速打开（TFO）
			syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, 23, 1) // TCP_FASTOPEN
		})
	}

	logger.Debug("TCP连接已优化", zap.String("remote", tcpConn.RemoteAddr().String()))
	return nil
}

// EnableReusePort 启用SO_REUSEPORT（多核负载均衡）
func EnableReusePort(network, address string) (net.Listener, error) {
	// SO_REUSEPORT值（Linux）
	const SO_REUSEPORT = 15

	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				// SO_REUSEPORT允许多个进程/线程绑定同一端口
				err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, SO_REUSEPORT, 1)
			})
			return err
		},
	}

	return lc.Listen(context.Background(), network, address)
}
