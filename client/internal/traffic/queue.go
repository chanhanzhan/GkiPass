package traffic

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Priority 优先级类型
type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
	PriorityCritical
)

// String 返回优先级名称
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "LOW"
	case PriorityNormal:
		return "NORMAL"
	case PriorityHigh:
		return "HIGH"
	case PriorityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// Packet 数据包
type Packet struct {
	ID        string                 `json:"id"`
	Data      []byte                 `json:"data"`
	Priority  Priority               `json:"priority"`
	Timestamp time.Time              `json:"timestamp"`
	Deadline  time.Time              `json:"deadline"` // 截止时间
	Retries   int                    `json:"retries"`  // 重试次数
	Metadata  map[string]interface{} `json:"metadata"`
}

// NewPacket 创建数据包
func NewPacket(id string, data []byte, priority Priority) *Packet {
	now := time.Now()
	return &Packet{
		ID:        id,
		Data:      data,
		Priority:  priority,
		Timestamp: now,
		Deadline:  now.Add(30 * time.Second), // 默认30秒超时
		Retries:   0,
		Metadata:  make(map[string]interface{}),
	}
}

// IsExpired 检查是否过期
func (p *Packet) IsExpired() bool {
	return time.Now().After(p.Deadline)
}

// Size 获取数据包大小
func (p *Packet) Size() int {
	return len(p.Data)
}

// priorityQueueItem 优先级队列项
type priorityQueueItem struct {
	packet *Packet
	index  int // 在堆中的索引
}

// priorityQueue 实现heap.Interface的优先级队列
type priorityQueue []*priorityQueueItem

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// 优先级高的排在前面，如果优先级相同，则按时间戳排序
	if pq[i].packet.Priority != pq[j].packet.Priority {
		return pq[i].packet.Priority > pq[j].packet.Priority
	}
	return pq[i].packet.Timestamp.Before(pq[j].packet.Timestamp)
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*priorityQueueItem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// PriorityQueue 优先级队列
type PriorityQueue struct {
	queue    priorityQueue
	mutex    sync.Mutex
	notEmpty *sync.Cond

	// 配置
	maxSize     int
	maxWaitTime time.Duration

	// 统计
	totalEnqueued atomic.Int64
	totalDequeued atomic.Int64
	totalDropped  atomic.Int64
	totalExpired  atomic.Int64

	// 按优先级统计
	priorityStats map[Priority]*PriorityStats

	// 控制
	closed atomic.Bool

	logger *zap.Logger
}

// PriorityStats 优先级统计
type PriorityStats struct {
	Enqueued atomic.Int64
	Dequeued atomic.Int64
	Dropped  atomic.Int64
	Expired  atomic.Int64
}

// NewPriorityQueue 创建优先级队列
func NewPriorityQueue(maxSize int, maxWaitTime time.Duration) *PriorityQueue {
	pq := &PriorityQueue{
		queue:         make(priorityQueue, 0),
		maxSize:       maxSize,
		maxWaitTime:   maxWaitTime,
		priorityStats: make(map[Priority]*PriorityStats),
		logger:        zap.L().Named("priority-queue"),
	}

	pq.notEmpty = sync.NewCond(&pq.mutex)

	// 初始化优先级统计
	for _, priority := range []Priority{PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical} {
		pq.priorityStats[priority] = &PriorityStats{}
	}

	heap.Init(&pq.queue)
	return pq
}

// Enqueue 入队
func (pq *PriorityQueue) Enqueue(packet *Packet) error {
	if pq.closed.Load() {
		return fmt.Errorf("队列已关闭")
	}

	pq.mutex.Lock()
	defer pq.mutex.Unlock()

	// 检查队列是否已满
	if len(pq.queue) >= pq.maxSize {
		// 尝试丢弃低优先级的包
		if !pq.tryDropLowPriority(packet.Priority) {
			pq.totalDropped.Add(1)
			pq.priorityStats[packet.Priority].Dropped.Add(1)
			pq.logger.Debug("队列已满，丢弃数据包",
				zap.String("packet_id", packet.ID),
				zap.String("priority", packet.Priority.String()))
			return fmt.Errorf("队列已满")
		}
	}

	// 添加到队列
	item := &priorityQueueItem{packet: packet}
	heap.Push(&pq.queue, item)

	pq.totalEnqueued.Add(1)
	pq.priorityStats[packet.Priority].Enqueued.Add(1)

	pq.logger.Debug("数据包入队",
		zap.String("packet_id", packet.ID),
		zap.String("priority", packet.Priority.String()),
		zap.Int("queue_size", len(pq.queue)))

	// 通知等待的协程
	pq.notEmpty.Signal()

	return nil
}

// tryDropLowPriority 尝试丢弃低优先级的包为新包让位
func (pq *PriorityQueue) tryDropLowPriority(newPriority Priority) bool {
	// 从队列末尾找低优先级的包
	for i := len(pq.queue) - 1; i >= 0; i-- {
		if pq.queue[i].packet.Priority < newPriority {
			// 找到可以丢弃的低优先级包
			item := heap.Remove(&pq.queue, i).(*priorityQueueItem)
			pq.totalDropped.Add(1)
			pq.priorityStats[item.packet.Priority].Dropped.Add(1)

			pq.logger.Debug("丢弃低优先级数据包",
				zap.String("dropped_id", item.packet.ID),
				zap.String("dropped_priority", item.packet.Priority.String()),
				zap.String("new_priority", newPriority.String()))

			return true
		}
	}

	return false
}

// Dequeue 出队
func (pq *PriorityQueue) Dequeue(ctx context.Context) (*Packet, error) {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()

	for {
		// 检查队列是否关闭
		if pq.closed.Load() && len(pq.queue) == 0 {
			return nil, fmt.Errorf("队列已关闭")
		}

		// 清理过期的包
		pq.cleanupExpired()

		// 如果队列不为空，返回最高优先级的包
		if len(pq.queue) > 0 {
			item := heap.Pop(&pq.queue).(*priorityQueueItem)
			packet := item.packet

			pq.totalDequeued.Add(1)
			pq.priorityStats[packet.Priority].Dequeued.Add(1)

			pq.logger.Debug("数据包出队",
				zap.String("packet_id", packet.ID),
				zap.String("priority", packet.Priority.String()),
				zap.Int("queue_size", len(pq.queue)))

			return packet, nil
		}

		// 队列为空，等待新数据或超时
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				pq.mutex.Lock()
				pq.notEmpty.Signal() // 唤醒等待的协程
				pq.mutex.Unlock()
			case <-done:
			}
		}()

		pq.notEmpty.Wait()
		close(done)

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
}

// TryDequeue 非阻塞出队
func (pq *PriorityQueue) TryDequeue() (*Packet, bool) {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()

	// 清理过期的包
	pq.cleanupExpired()

	if len(pq.queue) == 0 {
		return nil, false
	}

	item := heap.Pop(&pq.queue).(*priorityQueueItem)
	packet := item.packet

	pq.totalDequeued.Add(1)
	pq.priorityStats[packet.Priority].Dequeued.Add(1)

	pq.logger.Debug("数据包出队（非阻塞）",
		zap.String("packet_id", packet.ID),
		zap.String("priority", packet.Priority.String()),
		zap.Int("queue_size", len(pq.queue)))

	return packet, true
}

// cleanupExpired 清理过期的包（需要在锁内调用）
func (pq *PriorityQueue) cleanupExpired() {
	var expiredIndices []int

	// 收集过期的包的索引
	for i, item := range pq.queue {
		if item.packet.IsExpired() {
			expiredIndices = append(expiredIndices, i)
		}
	}

	// 从后往前删除，避免索引变化
	for i := len(expiredIndices) - 1; i >= 0; i-- {
		idx := expiredIndices[i]
		item := heap.Remove(&pq.queue, idx).(*priorityQueueItem)

		pq.totalExpired.Add(1)
		pq.priorityStats[item.packet.Priority].Expired.Add(1)

		pq.logger.Debug("清理过期数据包",
			zap.String("packet_id", item.packet.ID),
			zap.String("priority", item.packet.Priority.String()),
			zap.Time("deadline", item.packet.Deadline))
	}
}

// Size 获取队列大小
func (pq *PriorityQueue) Size() int {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()

	return len(pq.queue)
}

// IsEmpty 检查队列是否为空
func (pq *PriorityQueue) IsEmpty() bool {
	return pq.Size() == 0
}

// IsFull 检查队列是否已满
func (pq *PriorityQueue) IsFull() bool {
	return pq.Size() >= pq.maxSize
}

// Close 关闭队列
func (pq *PriorityQueue) Close() {
	pq.closed.Store(true)

	pq.mutex.Lock()
	pq.notEmpty.Broadcast() // 唤醒所有等待的协程
	pq.mutex.Unlock()

	pq.logger.Info("优先级队列已关闭")
}

// GetStats 获取队列统计信息
func (pq *PriorityQueue) GetStats() map[string]interface{} {
	pq.mutex.Lock()
	queueSize := len(pq.queue)
	pq.mutex.Unlock()

	stats := map[string]interface{}{
		"queue_size":     queueSize,
		"max_size":       pq.maxSize,
		"utilization":    float64(queueSize) / float64(pq.maxSize) * 100,
		"total_enqueued": pq.totalEnqueued.Load(),
		"total_dequeued": pq.totalDequeued.Load(),
		"total_dropped":  pq.totalDropped.Load(),
		"total_expired":  pq.totalExpired.Load(),
		"closed":         pq.closed.Load(),
		"priority_stats": make(map[string]interface{}),
	}

	// 优先级统计
	priorityStats := stats["priority_stats"].(map[string]interface{})
	for priority, stat := range pq.priorityStats {
		priorityStats[priority.String()] = map[string]interface{}{
			"enqueued": stat.Enqueued.Load(),
			"dequeued": stat.Dequeued.Load(),
			"dropped":  stat.Dropped.Load(),
			"expired":  stat.Expired.Load(),
		}
	}

	return stats
}

// MultiQueue 多优先级队列管理器
type MultiQueue struct {
	queues map[string]*PriorityQueue
	mutex  sync.RWMutex

	defaultMaxSize     int
	defaultMaxWaitTime time.Duration

	logger *zap.Logger
}

// NewMultiQueue 创建多队列管理器
func NewMultiQueue(defaultMaxSize int, defaultMaxWaitTime time.Duration) *MultiQueue {
	return &MultiQueue{
		queues:             make(map[string]*PriorityQueue),
		defaultMaxSize:     defaultMaxSize,
		defaultMaxWaitTime: defaultMaxWaitTime,
		logger:             zap.L().Named("multi-queue"),
	}
}

// GetQueue 获取或创建队列
func (mq *MultiQueue) GetQueue(name string) *PriorityQueue {
	mq.mutex.RLock()
	queue, exists := mq.queues[name]
	mq.mutex.RUnlock()

	if exists {
		return queue
	}

	mq.mutex.Lock()
	defer mq.mutex.Unlock()

	// 双重检查
	if queue, exists := mq.queues[name]; exists {
		return queue
	}

	queue = NewPriorityQueue(mq.defaultMaxSize, mq.defaultMaxWaitTime)
	mq.queues[name] = queue

	mq.logger.Debug("创建新队列",
		zap.String("name", name),
		zap.Int("max_size", mq.defaultMaxSize))

	return queue
}

// RemoveQueue 移除队列
func (mq *MultiQueue) RemoveQueue(name string) {
	mq.mutex.Lock()
	defer mq.mutex.Unlock()

	if queue, exists := mq.queues[name]; exists {
		queue.Close()
		delete(mq.queues, name)

		mq.logger.Debug("移除队列", zap.String("name", name))
	}
}

// CloseAll 关闭所有队列
func (mq *MultiQueue) CloseAll() {
	mq.mutex.Lock()
	defer mq.mutex.Unlock()

	for name, queue := range mq.queues {
		queue.Close()
		mq.logger.Debug("关闭队列", zap.String("name", name))
	}

	mq.queues = make(map[string]*PriorityQueue)
	mq.logger.Info("所有队列已关闭")
}

// GetAllStats 获取所有队列统计
func (mq *MultiQueue) GetAllStats() map[string]interface{} {
	mq.mutex.RLock()
	defer mq.mutex.RUnlock()

	stats := map[string]interface{}{
		"queue_count":      len(mq.queues),
		"default_max_size": mq.defaultMaxSize,
		"default_max_wait": mq.defaultMaxWaitTime.String(),
		"queues":           make(map[string]interface{}),
	}

	queuesStats := stats["queues"].(map[string]interface{})
	for name, queue := range mq.queues {
		queuesStats[name] = queue.GetStats()
	}

	return stats
}





