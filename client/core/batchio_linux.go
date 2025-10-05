//go:build linux
// +build linux

package core

import (
	"net"
	"syscall"
	"unsafe"
)

// BatchReader 批量读取器
type BatchReader struct {
	fd     int
	iovecs []syscall.Iovec
	bufs   [][]byte
}

// NewBatchReader 创建批量读取器
func NewBatchReader(conn net.Conn, batchSize int) (*BatchReader, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, nil // 不支持batch，返回nil
	}

	file, err := tcpConn.File()
	if err != nil {
		return nil, err
	}

	br := &BatchReader{
		fd:     int(file.Fd()),
		iovecs: make([]syscall.Iovec, batchSize),
		bufs:   make([][]byte, batchSize),
	}

	// 初始化缓冲区
	for i := 0; i < batchSize; i++ {
		buf := GetBuffer(32 * 1024)
		br.bufs[i] = *buf
		br.iovecs[i].Base = &br.bufs[i][0]
		br.iovecs[i].SetLen(len(br.bufs[i]))
	}

	return br, nil
}

// ReadBatch 批量读取
func (br *BatchReader) ReadBatch() (int, error) {
	n, _, err := syscall.Syscall(
		syscall.SYS_READV,
		uintptr(br.fd),
		uintptr(unsafe.Pointer(&br.iovecs[0])),
		uintptr(len(br.iovecs)),
	)
	return int(n), err
}

// Close 关闭并释放缓冲区
func (br *BatchReader) Close() {
	for i := range br.bufs {
		PutBuffer(&br.bufs[i])
	}
}

// BatchWriter 批量写入器
type BatchWriter struct {
	fd     int
	iovecs []syscall.Iovec
}

// NewBatchWriter 创建批量写入器
func NewBatchWriter(conn net.Conn) (*BatchWriter, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, nil
	}

	file, err := tcpConn.File()
	if err != nil {
		return nil, err
	}

	return &BatchWriter{
		fd: int(file.Fd()),
	}, nil
}

// WriteBatch 批量写入
func (bw *BatchWriter) WriteBatch(buffers [][]byte) (int, error) {
	// 构建iovec
	iovecs := make([]syscall.Iovec, len(buffers))
	for i, buf := range buffers {
		iovecs[i].Base = &buf[0]
		iovecs[i].SetLen(len(buf))
	}

	n, _, err := syscall.Syscall(
		syscall.SYS_WRITEV,
		uintptr(bw.fd),
		uintptr(unsafe.Pointer(&iovecs[0])),
		uintptr(len(iovecs)),
	)
	return int(n), err
}









