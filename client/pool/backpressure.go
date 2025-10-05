package pool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BackpressureController 回压控制器
type BackpressureController struct {
	readBufferSize  int
	writeBufferSize int
	readChan        chan []byte
	writeChan       chan []byte
	mu              sync.RWMutex
}

// NewBackpressureController 创建回压控制器
func NewBackpressureController(bufferSize int) *BackpressureController {
	return &BackpressureController{
		readBufferSize:  bufferSize,
		writeBufferSize: bufferSize,
		readChan:        make(chan []byte, bufferSize),
		writeChan:       make(chan []byte, bufferSize),
	}
}

// Read 读取数据（阻塞直到有数据）
func (bc *BackpressureController) Read() ([]byte, error) {
	select {
	case data := <-bc.readChan:
		return data, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("读超时")
	}
}

// ReadContext 带上下文的读取
func (bc *BackpressureController) ReadContext(ctx context.Context) ([]byte, error) {
	select {
	case data := <-bc.readChan:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Write 写入数据（如果缓冲区满则阻塞）
func (bc *BackpressureController) Write(data []byte) error {
	select {
	case bc.writeChan <- data:
		return nil
	case <-time.After(5 * time.Second):
		// 写队列满，触发回压
		return fmt.Errorf("回压：写缓冲区满")
	}
}

// WriteContext 带上下文的写入
func (bc *BackpressureController) WriteContext(ctx context.Context, data []byte) error {
	select {
	case bc.writeChan <- data:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetBufferUsage 获取缓冲区使用率
func (bc *BackpressureController) GetBufferUsage() (readUsage, writeUsage float64) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	readUsage = float64(len(bc.readChan)) / float64(bc.readBufferSize)
	writeUsage = float64(len(bc.writeChan)) / float64(bc.writeBufferSize)
	return
}

// IsReadCongested 读缓冲区是否拥塞
func (bc *BackpressureController) IsReadCongested() bool {
	readUsage, _ := bc.GetBufferUsage()
	return readUsage > 0.8 // 80%阈值
}

// IsWriteCongested 写缓冲区是否拥塞
func (bc *BackpressureController) IsWriteCongested() bool {
	_, writeUsage := bc.GetBufferUsage()
	return writeUsage > 0.8 // 80%阈值
}

// Close 关闭控制器
func (bc *BackpressureController) Close() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	close(bc.readChan)
	close(bc.writeChan)
}






