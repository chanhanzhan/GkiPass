//go:build !linux
// +build !linux

package core

import (
	"io"
	"net"
)

// ZeroCopy 非Linux平台回退到普通io.Copy
func ZeroCopy(dst, src net.Conn) (int64, error) {
	return io.Copy(dst, src)
}







