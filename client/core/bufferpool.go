package core

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// BufferSize 缓冲区大小常量
const (
	BufferSize4K   = 4 * 1024
	BufferSize8K   = 8 * 1024
	BufferSize16K  = 16 * 1024
	BufferSize32K  = 32 * 1024
	BufferSize64K  = 64 * 1024
	BufferSize128K = 128 * 1024
	BufferSize256K = 256 * 1024
)

// PooledBuffer 池化缓冲区
type PooledBuffer struct {
	data     []byte
	size     int
	refCount atomic.Int32
	pool     *ManagedBufferPool
}

// Bytes 返回底层字节切片
func (pb *PooledBuffer) Bytes() []byte {
	return pb.data
}

// Size 返回缓冲区大小
func (pb *PooledBuffer) Size() int {
	return pb.size
}

// Retain 增加引用计数
func (pb *PooledBuffer) Retain() {
	pb.refCount.Add(1)
}

// Release 释放缓冲区
func (pb *PooledBuffer) Release() {
	if pb.refCount.Add(-1) <= 0 {
		if pb.pool != nil {
			pb.pool.putBuffer(pb)
		}
	}
}

// ManagedBufferPool 托管缓冲池
type ManagedBufferPool struct {
	pools      map[int]*sync.Pool
	sizes      []int
	stats      BufferPoolStats
	maxMemory  int64 // 最大内存使用限制（字节）
	currentMem atomic.Int64
}

// BufferPoolStats 缓冲池统计信息
type BufferPoolStats struct {
	Gets      atomic.Int64
	Puts      atomic.Int64
	Allocs    atomic.Int64
	Reuses    atomic.Int64
	Overflows atomic.Int64
	LastReset atomic.Value // time.Time
}

var (
	// 全局托管缓冲池实例
	globalPool *ManagedBufferPool
	once       sync.Once
)

// InitBufferPool 初始化全局缓冲池
func InitBufferPool(maxMemoryMB int) {
	once.Do(func() {
		globalPool = NewManagedBufferPool(int64(maxMemoryMB) * 1024 * 1024)
		globalPool.WarmUp()
		logger.Info("缓冲池已初始化",
			zap.Int("max_memory_mb", maxMemoryMB),
			zap.Int("pool_sizes", len(globalPool.sizes)))
	})
}

// GetGlobalPool 获取全局缓冲池
func GetGlobalPool() *ManagedBufferPool {
	if globalPool == nil {
		InitBufferPool(512) // 默认512MB
	}
	return globalPool
}

// NewManagedBufferPool 创建托管缓冲池
func NewManagedBufferPool(maxMemory int64) *ManagedBufferPool {
	sizes := []int{
		BufferSize4K,
		BufferSize8K,
		BufferSize16K,
		BufferSize32K,
		BufferSize64K,
		BufferSize128K,
		BufferSize256K,
	}

	mbp := &ManagedBufferPool{
		pools:     make(map[int]*sync.Pool),
		sizes:     sizes,
		maxMemory: maxMemory,
	}
	mbp.stats.LastReset.Store(time.Now())

	// 为每个大小创建一个池
	for _, size := range sizes {
		s := size // 捕获循环变量
		mbp.pools[size] = &sync.Pool{
			New: func() interface{} {
				mbp.stats.Allocs.Add(1)
				buf := make([]byte, s)
				return &PooledBuffer{
					data: buf,
					size: s,
					pool: mbp,
				}
			},
		}
	}

	return mbp
}

// WarmUp 预热缓冲池（预分配一些缓冲区）
func (mbp *ManagedBufferPool) WarmUp() {
	warmupCounts := map[int]int{
		BufferSize4K:   100,
		BufferSize8K:   100,
		BufferSize16K:  50,
		BufferSize32K:  50,
		BufferSize64K:  20,
		BufferSize128K: 10,
		BufferSize256K: 5,
	}

	for size, count := range warmupCounts {
		pool := mbp.pools[size]
		if pool == nil {
			continue
		}
		// 预分配并放回池中
		bufs := make([]*PooledBuffer, count)
		for i := 0; i < count; i++ {
			bufs[i] = pool.Get().(*PooledBuffer)
		}
		for i := 0; i < count; i++ {
			pool.Put(bufs[i])
		}
	}
	logger.Info("缓冲池预热完成")
}

var (
	// 向后兼容：保留旧的池变量
	bufferPool32K = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 32*1024)
			return &buf
		},
	}

	bufferPool64K = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 64*1024)
			return &buf
		},
	}

	bufferPool128K = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 128*1024)
			return &buf
		},
	}
)

// selectPoolSize 选择合适的池大小
func (mbp *ManagedBufferPool) selectPoolSize(requested int) int {
	for _, size := range mbp.sizes {
		if size >= requested {
			return size
		}
	}
	return mbp.sizes[len(mbp.sizes)-1] // 返回最大的
}

// GetBuffer2 获取缓冲区（新接口）
func (mbp *ManagedBufferPool) GetBuffer2(size int) *PooledBuffer {
	poolSize := mbp.selectPoolSize(size)
	pool, ok := mbp.pools[poolSize]
	if !ok {
		// 回退到最大池
		poolSize = mbp.sizes[len(mbp.sizes)-1]
		pool = mbp.pools[poolSize]
	}

	mbp.stats.Gets.Add(1)

	// 检查内存限制
	if mbp.maxMemory > 0 {
		current := mbp.currentMem.Load()
		if current+int64(poolSize) > mbp.maxMemory {
			mbp.stats.Overflows.Add(1)
			// 仍然分配，但记录溢出
			logger.Warn("缓冲池内存接近限制",
				zap.Int64("current_mb", current/(1024*1024)),
				zap.Int64("max_mb", mbp.maxMemory/(1024*1024)))
		}
	}

	buf := pool.Get().(*PooledBuffer)
	buf.refCount.Store(1)
	mbp.currentMem.Add(int64(poolSize))

	return buf
}

// putBuffer 归还缓冲区（内部方法）
func (mbp *ManagedBufferPool) putBuffer(buf *PooledBuffer) {
	if buf == nil {
		return
	}

	pool, ok := mbp.pools[buf.size]
	if !ok {
		return
	}

	mbp.stats.Puts.Add(1)
	mbp.stats.Reuses.Add(1)
	mbp.currentMem.Add(-int64(buf.size))

	// 重置引用计数
	buf.refCount.Store(0)

	pool.Put(buf)
}

// GetStats 获取统计信息
func (mbp *ManagedBufferPool) GetStats() BufferPoolStatsSnapshot {
	lastReset := mbp.stats.LastReset.Load().(time.Time)
	return BufferPoolStatsSnapshot{
		Gets:       mbp.stats.Gets.Load(),
		Puts:       mbp.stats.Puts.Load(),
		Allocs:     mbp.stats.Allocs.Load(),
		Reuses:     mbp.stats.Reuses.Load(),
		Overflows:  mbp.stats.Overflows.Load(),
		CurrentMem: mbp.currentMem.Load(),
		MaxMemory:  mbp.maxMemory,
		ReuseRate:  float64(mbp.stats.Reuses.Load()) / float64(mbp.stats.Gets.Load()+1) * 100,
		Since:      time.Since(lastReset),
	}
}

// BufferPoolStatsSnapshot 缓冲池统计快照
type BufferPoolStatsSnapshot struct {
	Gets       int64
	Puts       int64
	Allocs     int64
	Reuses     int64
	Overflows  int64
	CurrentMem int64
	MaxMemory  int64
	ReuseRate  float64 // 复用率（百分比）
	Since      time.Duration
}

// String 格式化输出统计信息
func (s BufferPoolStatsSnapshot) String() string {
	return fmt.Sprintf("Gets=%d Puts=%d Allocs=%d Reuses=%d ReuseRate=%.2f%% CurrentMem=%dMB MaxMem=%dMB Overflows=%d",
		s.Gets, s.Puts, s.Allocs, s.Reuses, s.ReuseRate,
		s.CurrentMem/(1024*1024), s.MaxMemory/(1024*1024), s.Overflows)
}

// ResetStats 重置统计信息
func (mbp *ManagedBufferPool) ResetStats() {
	mbp.stats.Gets.Store(0)
	mbp.stats.Puts.Store(0)
	mbp.stats.Allocs.Store(0)
	mbp.stats.Reuses.Store(0)
	mbp.stats.Overflows.Store(0)
	mbp.stats.LastReset.Store(time.Now())
}

// GetBuffer 获取缓冲区（向后兼容）
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

// PutBuffer 归还缓冲区（向后兼容）
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

// GetManagedBuffer 获取托管缓冲区（推荐使用）
func GetManagedBuffer(size int) *PooledBuffer {
	return GetGlobalPool().GetBuffer2(size)
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

// CopyWithManagedBuffer 使用托管缓冲池的拷贝（推荐使用）
func CopyWithManagedBuffer(dst, src interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
}, size int) (int64, error) {
	buf := GetManagedBuffer(size)
	defer buf.Release()

	var total int64
	data := buf.Bytes()
	for {
		nr, err := src.Read(data)
		if nr > 0 {
			nw, werr := dst.Write(data[:nr])
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
