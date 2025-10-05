//go:build linux
// +build linux

package core

import (
	"io"
	"net"
	"syscall"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// ZeroCopy 零拷贝转发（使用splice系统调用）
func ZeroCopy(dst, src net.Conn) (int64, error) {
	// 获取文件描述符
	srcTCP, srcOK := src.(*net.TCPConn)
	dstTCP, dstOK := dst.(*net.TCPConn)

	if !srcOK || !dstOK {
		// 回退到普通io.Copy
		return io.Copy(dst, src)
	}

	srcFile, err := srcTCP.File()
	if err != nil {
		return io.Copy(dst, src)
	}
	defer srcFile.Close()

	dstFile, err := dstTCP.File()
	if err != nil {
		return io.Copy(dst, src)
	}
	defer dstFile.Close()

	srcFd := int(srcFile.Fd())
	dstFd := int(dstFile.Fd())

	// 创建管道用于splice
	pipeFds := make([]int, 2)
	if err := syscall.Pipe(pipeFds); err != nil {
		logger.Warn("创建管道失败，回退到io.Copy", zap.Error(err))
		return io.Copy(dst, src)
	}
	defer syscall.Close(pipeFds[0])
	defer syscall.Close(pipeFds[1])

	var total int64
	const maxSpliceSize = 64 * 1024 // 64KB per splice

	for {
		// src → pipe
		n, err := syscall.Splice(srcFd, nil, pipeFds[1], nil, maxSpliceSize, 0)
		if n > 0 {
			// pipe → dst
			written := int64(0)
			for written < n {
				w, err := syscall.Splice(pipeFds[0], nil, dstFd, nil, int(n-written), 0)
				if w > 0 {
					written += w
				}
				if err != nil {
					return total + written, err
				}
			}
			total += n
		}
		if err != nil {
			if err == syscall.EAGAIN {
				continue
			}
			if err == io.EOF || err == syscall.EPIPE {
				return total, nil
			}
			return total, err
		}
		if n == 0 {
			return total, nil
		}
	}
}







