package core

import (
	"sync/atomic"
	"unsafe"
)

// LockFreeQueue 无锁队列（MPMC - 多生产者多消费者）
type LockFreeQueue struct {
	head    unsafe.Pointer
	tail    unsafe.Pointer
	size    atomic.Int64
	maxSize int64
}

type node struct {
	value interface{}
	next  unsafe.Pointer
}

// NewLockFreeQueue 创建无锁队列
func NewLockFreeQueue(maxSize int64) *LockFreeQueue {
	n := unsafe.Pointer(&node{})
	return &LockFreeQueue{
		head:    n,
		tail:    n,
		maxSize: maxSize,
	}
}

// Enqueue 入队（返回false表示队列已满）
func (q *LockFreeQueue) Enqueue(value interface{}) bool {
	// 检查队列大小
	if q.maxSize > 0 && q.size.Load() >= q.maxSize {
		return false
	}

	n := &node{value: value}
	nPtr := unsafe.Pointer(n)

	for {
		tail := atomic.LoadPointer(&q.tail)
		next := atomic.LoadPointer(&(*node)(tail).next)

		if tail == atomic.LoadPointer(&q.tail) {
			if next == nil {
				if atomic.CompareAndSwapPointer(&(*node)(tail).next, next, nPtr) {
					atomic.CompareAndSwapPointer(&q.tail, tail, nPtr)
					q.size.Add(1)
					return true
				}
			} else {
				atomic.CompareAndSwapPointer(&q.tail, tail, next)
			}
		}
	}
}

// Dequeue 出队（返回nil表示队列为空）
func (q *LockFreeQueue) Dequeue() interface{} {
	for {
		head := atomic.LoadPointer(&q.head)
		tail := atomic.LoadPointer(&q.tail)
		next := atomic.LoadPointer(&(*node)(head).next)

		if head == atomic.LoadPointer(&q.head) {
			if head == tail {
				if next == nil {
					return nil // 队列为空
				}
				atomic.CompareAndSwapPointer(&q.tail, tail, next)
			} else {
				value := (*node)(next).value
				if atomic.CompareAndSwapPointer(&q.head, head, next) {
					q.size.Add(-1)
					return value
				}
			}
		}
	}
}

// Size 获取队列大小
func (q *LockFreeQueue) Size() int64 {
	return q.size.Load()
}

// IsEmpty 是否为空
func (q *LockFreeQueue) IsEmpty() bool {
	return q.Size() == 0
}

// AtomicStats 原子统计计数器
type AtomicStats struct {
	TotalConnections  atomic.Int64
	ActiveConnections atomic.Int64
	TotalBytes        atomic.Int64
	TotalPackets      atomic.Int64
	Errors            atomic.Int64
}

// NewAtomicStats 创建原子统计
func NewAtomicStats() *AtomicStats {
	return &AtomicStats{}
}

// IncrConnection 增加连接
func (as *AtomicStats) IncrConnection() {
	as.TotalConnections.Add(1)
	as.ActiveConnections.Add(1)
}

// DecrConnection 减少连接
func (as *AtomicStats) DecrConnection() {
	as.ActiveConnections.Add(-1)
}

// AddBytes 添加字节数
func (as *AtomicStats) AddBytes(n int64) {
	as.TotalBytes.Add(n)
}

// AddPackets 添加包数
func (as *AtomicStats) AddPackets(n int64) {
	as.TotalPackets.Add(n)
}

// IncrError 增加错误计数
func (as *AtomicStats) IncrError() {
	as.Errors.Add(1)
}

// Snapshot 获取快照
func (as *AtomicStats) Snapshot() StatsSnapshot {
	return StatsSnapshot{
		TotalConnections:  as.TotalConnections.Load(),
		ActiveConnections: as.ActiveConnections.Load(),
		TotalBytes:        as.TotalBytes.Load(),
		TotalPackets:      as.TotalPackets.Load(),
		Errors:            as.Errors.Load(),
	}
}

// StatsSnapshot 统计快照
type StatsSnapshot struct {
	TotalConnections  int64
	ActiveConnections int64
	TotalBytes        int64
	TotalPackets      int64
	Errors            int64
}
