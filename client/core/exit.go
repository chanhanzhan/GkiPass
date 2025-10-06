package core

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"gkipass/client/logger"
	"gkipass/client/pool"
	"gkipass/client/ws"

	"go.uber.org/zap"
)

// ExitNode Exit节点（目标访问侧）
type ExitNode struct {
	targetConns map[string]net.Conn // sessionID → target conn
	mu          sync.RWMutex
}

// NewExitNode 创建Exit节点
func NewExitNode() *ExitNode {
	return &ExitNode{
		targetConns: make(map[string]net.Conn),
	}
}

// HandleStream 处理来自Entry的stream
func (ex *ExitNode) HandleStream(stream *pool.Stream, sessionID string) {
	defer stream.Close()

	// 读取初始化帧
	initData := make([]byte, 1024)
	n, err := stream.Read(initData)
	if err != nil {
		logger.Error("读取初始化帧失败", zap.Error(err))
		return
	}

	// 解析目标信息
	targets, err := ex.parseInitFrame(initData[:n])
	if err != nil {
		logger.Error("解析初始化帧失败", zap.Error(err))
		return
	}

	if len(targets) == 0 {
		logger.Error("没有目标服务器")
		return
	}

	// 选择目标（简单选择第一个）
	target := targets[0]
	targetAddr := net.JoinHostPort(target.Host, fmt.Sprintf("%d", target.Port))

	// 连接目标服务器
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		logger.Error("连接目标失败",
			zap.String("target", targetAddr),
			zap.Error(err))
		return
	}
	defer targetConn.Close()

	// 保存连接
	ex.mu.Lock()
	ex.targetConns[sessionID] = targetConn
	ex.mu.Unlock()

	defer func() {
		ex.mu.Lock()
		delete(ex.targetConns, sessionID)
		ex.mu.Unlock()
	}()

	logger.Info("Exit已连接目标",
		zap.String("session_id", sessionID),
		zap.String("target", targetAddr))

	// 双向转发
	ex.forwardBidirectional(stream, targetConn, sessionID)
}

// parseInitFrame 解析初始化帧
func (ex *ExitNode) parseInitFrame(data []byte) ([]ws.TunnelTarget, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("初始化帧太短")
	}

	// 跳过TunnelID (16字节)
	pos := 16

	// 读取Target数量
	targetCount := binary.BigEndian.Uint16(data[pos:])
	pos += 2

	targets := make([]ws.TunnelTarget, 0, targetCount)

	for i := 0; i < int(targetCount); i++ {
		if pos >= len(data) {
			break
		}

		// Host长度
		hostLen := int(data[pos])
		pos++

		if pos+hostLen+4 > len(data) {
			break
		}

		// Host
		host := string(data[pos : pos+hostLen])
		pos += hostLen

		// Port
		port := int(binary.BigEndian.Uint16(data[pos:]))
		pos += 2

		// Weight
		weight := int(binary.BigEndian.Uint16(data[pos:]))
		pos += 2

		targets = append(targets, ws.TunnelTarget{
			Host:   host,
			Port:   port,
			Weight: weight,
		})
	}

	return targets, nil
}

// forwardBidirectional 双向转发
func (ex *ExitNode) forwardBidirectional(stream *pool.Stream, targetConn net.Conn, sessionID string) {
	errChan := make(chan error, 2)

	// Stream → Target
	go func() {
		_, err := io.Copy(targetConn, stream)
		errChan <- err
	}()

	// Target → Stream
	go func() {
		_, err := io.Copy(stream, targetConn)
		errChan <- err
	}()

	// 等待任意方向结束
	<-errChan
	<-errChan

	logger.Debug("Exit转发结束", zap.String("session_id", sessionID))
}
