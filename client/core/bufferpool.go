package core

import (
	"io"
	"sync"
)

var (
	// 32KB缓冲区池（用于常规转发）
	bufferPool32K = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 32*1024)
			return &buf
		},
	}

	// 64KB缓冲区池（用于大流量转发）
	bufferPool64K = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 64*1024)
			return &buf
		},
	}

	// 128KB缓冲区池（用于超大流量转发）
	bufferPool128K = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 128*1024)
			return &buf
		},
	}
)

// GetBuffer 获取缓冲区
func GetBuffer(size int) *[]byte {
	switch {
	case size <= 32*1024:
		return bufferPool32K.Get().(*[]byte)
	case size <= 64*1024:
		return bufferPool64K.Get().(*[]byte)
	default:
		return bufferPool128K.Get().(*[]byte)
	}
}

// PutBuffer 归还缓冲区
func PutBuffer(buf *[]byte) {
	if buf == nil {
		return
	}

	size := cap(*buf)
	switch {
	case size == 32*1024:
		bufferPool32K.Put(buf)
	case size == 64*1024:
		bufferPool64K.Put(buf)
	case size == 128*1024:
		bufferPool128K.Put(buf)
	}
}

// CopyWithBuffer 使用内存池的拷贝
func CopyWithBuffer(dst, src interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
}) (int64, error) {
	buf := GetBuffer(64 * 1024) // 使用64KB缓冲
	defer PutBuffer(buf)

	var total int64
	for {
		nr, err := src.Read(*buf)
		if nr > 0 {
			nw, werr := dst.Write((*buf)[:nr])
			if nw > 0 {
				total += int64(nw)
			}
			if werr != nil {
				return total, werr
			}
			if nr != nw {
				return total, io.ErrShortWrite
			}
		}
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
}
